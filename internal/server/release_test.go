package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/plan"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/provider"
)

type releaseFirebase struct {
	template provider.Template
	puts     int
}

func (f *releaseFirebase) Connect(context.Context) error { return nil }
func (f *releaseFirebase) Status(context.Context) provider.Status {
	return provider.Status{Status: "connected", Capabilities: f.Capabilities()}
}
func (f *releaseFirebase) Pull(context.Context) (provider.Template, error) { return f.template, nil }
func (f *releaseFirebase) Validate(context.Context, []byte) error          { return nil }
func (f *releaseFirebase) Capabilities() provider.Capabilities {
	return provider.Capabilities{Pull: true, Validate: true, Publish: true}
}
func (f *releaseFirebase) Publish(_ context.Context, raw []byte, etag string) (provider.Template, error) {
	if etag != f.template.ETag {
		return provider.Template{}, provider.ErrETagMismatch
	}
	f.puts++
	f.template = provider.Template{Raw: raw, ETag: "etag-after", Version: "2", ObservedAt: time.Now().UTC()}
	return f.template, nil
}

func TestReleaseHTTPReturnsOperationAndETagConflictPayload(t *testing.T) {
	service, firebase := releaseHTTPService(t)
	plan := readyReleasePlan(t, service)
	handler := New(service)
	body, _ := json.Marshal(map[string]any{"plan_id": plan.PlanID, "expected_draft_revision": plan.DraftRevision, "expected_remote_etag": *plan.RemoteETag, "confirmation": map[string]any{"acknowledged": true, "acknowledged_risk_item_ids": []string{}}})
	request := newAPIRequest(t, http.MethodPost, "/api/v1/environments/development/releases", body)
	request.Header.Set("Idempotency-Key", "release-http-key-0001")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("release=%d %s", recorder.Code, recorder.Body.String())
	}
	var started struct {
		Data struct {
			OperationID string `json:"operation_id"`
		} `json:"data"`
	}
	decodeResponse(t, recorder, &started)
	for deadline := time.Now().Add(time.Second); ; {
		op, err := service.Operation(context.Background(), started.Data.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("release timeout")
		}
		time.Sleep(time.Millisecond)
	}
	if firebase.puts != 1 {
		t.Fatalf("puts=%d", firebase.puts)
	}

	secondPlan := readyReleasePlan(t, service)
	firebase.template.ETag = "etag-external-change"
	body, _ = json.Marshal(map[string]any{"plan_id": secondPlan.PlanID, "expected_draft_revision": secondPlan.DraftRevision, "expected_remote_etag": *secondPlan.RemoteETag, "confirmation": map[string]any{"acknowledged": true, "acknowledged_risk_item_ids": []string{}}})
	request = newAPIRequest(t, http.MethodPost, "/api/v1/environments/development/releases", body)
	request.Header.Set("Idempotency-Key", "release-http-key-0002")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPreconditionFailed || firebase.puts != 1 {
		t.Fatalf("mismatch=%d puts=%d %s", recorder.Code, firebase.puts, recorder.Body.String())
	}
	var mismatch remoteETagMismatchEnvelope
	decodeResponse(t, recorder, &mismatch)
	if mismatch.Error.Code != "remote_etag_mismatch" || mismatch.Error.CurrentRemote.RemoteETag != "etag-external-change" || mismatch.Error.Rebuild.PlanEndpoint != "/api/v1/drafts/development:plan" {
		t.Fatalf("mismatch=%#v", mismatch)
	}
	encoded := recorder.Body.String()
	for _, forbidden := range []string{"current_revision", "current_source_revision", "current_state"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("mismatch leaked local field %s: %s", forbidden, encoded)
		}
	}
}

func releaseHTTPService(t *testing.T) (*app.Service, *releaseFirebase) {
	t.Helper()
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	firebase := &releaseFirebase{template: provider.Template{Raw: []byte(`{"parameters":{"unmanaged":{"defaultValue":{"value":"keep"}}}}`), ETag: "etag-before", Version: "1", ObservedAt: time.Now().UTC()}}
	service, err := app.OpenWithPacksAndProviderFactory(workspace, packs.BuiltinRegistry(), func(project.Environment) (provider.Adapter, error) { return firebase, nil })
	if err != nil {
		t.Fatal(err)
	}
	return service, firebase
}
func readyReleasePlan(t *testing.T, service *app.Service) plan.Plan {
	t.Helper()
	started, err := service.StartPlan(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Second); ; {
		op, err := service.Operation(context.Background(), started.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status == "succeeded" {
			p, err := service.GetPlan(context.Background(), op.Result.ResourceID)
			if err != nil {
				t.Fatal(err)
			}
			return p
		}
		if time.Now().After(deadline) {
			t.Fatalf("plan=%#v", op)
		}
		time.Sleep(time.Millisecond)
	}
}
