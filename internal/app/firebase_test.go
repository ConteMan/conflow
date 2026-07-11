package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/operation"
	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/provider"
)

type fakeFirebase struct {
	template                         provider.Template
	pullErr, validateErr, publishErr error
	validated                        []byte
	published                        []byte
}

func (f *fakeFirebase) Connect(context.Context) error { return f.pullErr }
func (f *fakeFirebase) Status(context.Context) provider.Status {
	return provider.Status{Status: "connected", Capabilities: f.Capabilities()}
}
func (f *fakeFirebase) Pull(context.Context) (provider.Template, error) { return f.template, f.pullErr }
func (f *fakeFirebase) Validate(_ context.Context, input []byte) error {
	f.validated = append([]byte(nil), input...)
	return f.validateErr
}
func (f *fakeFirebase) Publish(_ context.Context, input []byte, expectedETag string) (provider.Template, error) {
	if f.publishErr != nil {
		return provider.Template{}, f.publishErr
	}
	if expectedETag != f.template.ETag {
		return provider.Template{}, provider.ErrETagMismatch
	}
	f.published = append([]byte(nil), input...)
	f.template = provider.Template{Raw: append([]byte(nil), input...), ETag: "etag-after", Version: "2", ObservedAt: time.Now().UTC()}
	return f.template, nil
}
func (f *fakeFirebase) Capabilities() provider.Capabilities {
	return provider.Capabilities{Pull: true, Validate: true, Publish: true}
}

func TestPullValidateAndConditionRiskUseProviderBoundary(t *testing.T) {
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	fake := &fakeFirebase{template: provider.Template{ETag: "etag-safe", Version: "4", ObservedAt: time.Now(), Raw: []byte(`{"parameters":{"ad_frequency_inter_global_cap":{"defaultValue":{"value":"30000"},"conditionalValues":{"country":{"value":"60000"}}}}}`)}}
	service, err := OpenWithPacksAndProviderFactory(workspace, packs.BuiltinRegistry(), func(project.Environment) (provider.Adapter, error) { return fake, nil })
	if err != nil {
		t.Fatal(err)
	}
	pull, err := service.StartPull(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	pull = waitAppOperation(t, service, pull.OperationID)
	if pull.Status != "succeeded" || pull.Stage != "completed" || pull.RemoteState != "unchanged" || pull.Result == nil || pull.Result.ResourceType != "remote_snapshot" {
		t.Fatalf("pull=%#v", pull)
	}
	snapshotBytes, err := os.ReadFile(filepath.Join(workspace, ".conflow", "remote-snapshots", "development.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Join(workspace, ".conflow", "remote-snapshots", "development.json")); info.Mode().Perm() != 0o600 {
		t.Fatal("snapshot not protected")
	}
	if strings.Contains(string(snapshotBytes), "Authorization") || strings.Contains(string(snapshotBytes), "test-access-token") {
		t.Fatal("credential material entered snapshot")
	}
	planOp, err := service.StartPlan(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	planOp = waitAppOperation(t, service, planOp.OperationID)
	if planOp.Status != "succeeded" {
		t.Fatalf("plan=%#v", planOp)
	}
	p, err := service.GetPlan(context.Background(), planOp.Result.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Severity != "blocking" || len(p.BlockingReasons) == 0 || p.BlockingReasons[0].ReasonCode != "unmodeled_remote_condition" {
		t.Fatalf("condition did not block: %#v", p.BlockingReasons)
	}
	tokenBefore := p.SnapshotToken
	validate, err := service.StartRemoteValidate(context.Background(), "development", p.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	validate = waitAppOperation(t, service, validate.OperationID)
	if validate.Status != "succeeded" || validate.RemoteState != "unchanged" || len(fake.validated) == 0 {
		t.Fatalf("validate=%#v input=%s", validate, fake.validated)
	}
	after, err := service.GetPlan(context.Background(), p.PlanID)
	if err != nil || after.SnapshotToken != tokenBefore {
		t.Fatalf("validate mutated plan snapshot token: before=%q after=%#v err=%v", tokenBefore, after, err)
	}
}

func TestProviderFailureIsStableAndDoesNotLeakUpstreamText(t *testing.T) {
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	fake := &fakeFirebase{pullErr: errors.New("Bearer test-access-token private_key test-private-key")}
	service, err := OpenWithPacksAndProviderFactory(workspace, packs.BuiltinRegistry(), func(project.Environment) (provider.Adapter, error) { return fake, nil })
	if err != nil {
		t.Fatal(err)
	}
	op, err := service.StartPull(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	op = waitAppOperation(t, service, op.OperationID)
	if op.Status != "failed" || op.Failure == nil || op.Failure.Code != "provider_unavailable" {
		t.Fatalf("op=%#v", op)
	}
	if strings.Contains(op.Failure.Message, "test-access-token") || strings.Contains(op.Failure.Message, "test-private-key") {
		t.Fatalf("leaked failure: %#v", op.Failure)
	}
}

func TestRemoteProjectionMapsEditorFieldsAndRedactsUnitIDs(t *testing.T) {
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	fake := &fakeFirebase{template: provider.Template{ETag: "etag-caption", Version: "2", ObservedAt: time.Now(), Raw: []byte(`{"parameters":{"ad_frequency_inter_global_cap":{"defaultValue":{"value":"30000"}},"unit_binding_banner_unit_id_ref":{"defaultValue":{"value":"real-unit-id-must-not-escape"}}}}`)}}
	service, err := OpenWithPacksAndProviderFactory(workspace, packs.BuiltinRegistry(), func(project.Environment) (provider.Adapter, error) { return fake, nil })
	if err != nil {
		t.Fatal(err)
	}
	view, revision, err := service.GetDraft(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	configuration := []byte(`{"frequency_policies":[{"id":"inter_global_cap","cooldown_ms":120000}],"unit_bindings":[{"id":"banner","unit_id_ref":"secret-ref"}]}`)
	if _, _, err := service.MutateDraft(context.Background(), "development", draft.Mutation{ExpectedRevision: revision, ExpectedSourceRevision: view.SourceRevision, Scope: draft.ScopeBaseline, Action: "put", Configuration: configuration}); err != nil {
		t.Fatal(err)
	}
	op, err := service.StartPull(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	_ = waitAppOperation(t, service, op.OperationID)
	projection, err := service.RemoteProjection(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(projection)
	if strings.Contains(string(encoded), "real-unit-id-must-not-escape") || strings.Contains(string(encoded), "secret-ref") {
		t.Fatalf("projection leaked unit value: %s", encoded)
	}
	var foundAvailable, foundRedacted bool
	for _, item := range projection.Projections {
		if item.ParameterKey == "ad_frequency_inter_global_cap" && item.EntityRef != "" && item.FieldPath == "/cooldown_ms" && item.Availability == "available" && item.ValueSummary == "30 seconds" {
			foundAvailable = true
		}
		if item.ParameterKey == "unit_binding_banner_unit_id_ref" && item.Availability == "redacted" && item.Redacted {
			foundRedacted = true
		}
	}
	if !foundAvailable || !foundRedacted {
		t.Fatalf("projection=%#v", projection)
	}
}

func waitAppOperation(t *testing.T, service *Service, id string) operation.Operation {
	t.Helper()
	for deadline := time.Now().Add(time.Second); ; {
		op, err := service.Operation(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status == "succeeded" || op.Status == "failed" {
			return op
		}
		if time.Now().After(deadline) {
			t.Fatal("operation timeout")
		}
		time.Sleep(time.Millisecond)
	}
}
