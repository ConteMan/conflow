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

	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/plan"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/provider"
	"github.com/ConteMan/conflow/internal/release"
	"github.com/ConteMan/conflow/internal/releasedbaseline"
	"github.com/ConteMan/conflow/internal/remote"
)

func TestReleaseSuccessReplayConflictAndAudit(t *testing.T) {
	s, fake := releaseService(t)
	p := buildReadyPlan(t, s)
	request := releaseRequest(p)
	started, err := s.StartRelease(context.Background(), "development", "publish-key-0000001", request)
	if err != nil {
		t.Fatal(err)
	}
	completed := waitAppOperation(t, s, started.OperationID)
	if completed.Status != "succeeded" || completed.RemoteState != "changed" || completed.Result == nil {
		t.Fatalf("operation=%#v", completed)
	}
	if len(fake.published) == 0 {
		t.Fatal("provider did not receive publish")
	}
	if records := s.Releases(context.Background(), "development"); len(records) != 1 || records[0].OperationID != started.OperationID || records[0].Outcome != "succeeded" {
		t.Fatalf("audit=%#v", records)
	}
	baseline, found, err := s.baselines.Load("development")
	if err != nil || !found || baseline.ReleaseID != completed.Result.ResourceID || baseline.SourceRevision == "" || baseline.Entities == nil {
		t.Fatalf("baseline=%#v found=%v err=%v", baseline, found, err)
	}
	replayed, err := s.StartRelease(context.Background(), "development", "publish-key-0000001", request)
	if err != nil || replayed.OperationID != started.OperationID || replayed.Status != "succeeded" {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	request.ExpectedDraftRevision++
	_, err = s.StartRelease(context.Background(), "development", "publish-key-0000001", request)
	if !errors.Is(err, release.ErrIdempotencyConflict) {
		t.Fatalf("conflict=%v", err)
	}
}

func TestReleaseETagMismatchDoesNotPublish(t *testing.T) {
	s, fake := releaseService(t)
	p := buildReadyPlan(t, s)
	fake.template.ETag = "etag-current"
	_, err := s.StartRelease(context.Background(), "development", "publish-key-0000002", releaseRequest(p))
	var mismatch *RemoteETagMismatchError
	if !errors.As(err, &mismatch) || mismatch.Current.RemoteETag != "etag-current" || len(fake.published) != 0 {
		t.Fatalf("err=%#v published=%s", err, fake.published)
	}
}

func TestReleaseTimeoutMakesRemoteStateUnknown(t *testing.T) {
	s, fake := releaseService(t)
	fake.publishErr = context.DeadlineExceeded
	p := buildReadyPlan(t, s)
	started, err := s.StartRelease(context.Background(), "development", "publish-key-0000003", releaseRequest(p))
	if err != nil {
		t.Fatal(err)
	}
	completed := waitAppOperation(t, s, started.OperationID)
	if completed.Status != "failed" || completed.RemoteState != "unknown" || completed.Failure == nil || completed.Failure.Code != "provider_response_unknown" {
		t.Fatalf("operation=%#v", completed)
	}
	records := s.Releases(context.Background(), "development")
	if len(records) != 1 || records[0].Outcome != "failed" || records[0].RemoteState != "unknown" || records[0].RemoteAfter != nil || records[0].Failure == nil || records[0].Failure.Code != "provider_response_unknown" {
		t.Fatalf("failed audit=%#v", records)
	}
	if _, found, err := s.baselines.Load("development"); err != nil || found {
		t.Fatalf("failed release baseline found=%v err=%v", found, err)
	}
}

func TestEntityChangeStatusUsesReleasedBaseline(t *testing.T) {
	s, _ := releaseService(t)
	view, revision, err := s.GetDraft(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	record := EntityRecord{ID: "inter_global_cap", Fields: map[string]any{"cooldown_ms": float64(30000), "interval_ms": float64(300000), "max_count": float64(3), "shift_count": float64(1), "positions": []any{"open_document"}}}
	created, revision, err := s.MutateEntity(context.Background(), "development", EntityMutation{ExpectedRevision: revision, ExpectedSourceRevision: view.SourceRevision, Scope: "baseline", EntityType: "frequency_policy", EntityID: record.ID, Entity: &record, Action: "create"})
	if err != nil || created.ChangeStatus != "created" {
		t.Fatalf("created=%#v revision=%d err=%v", created, revision, err)
	}
	draftView, _, err := s.GetDraft(context.Background(), "development")
	if err != nil || draftView.ChangedEntityCount != 1 {
		t.Fatalf("draft=%#v err=%v", draftView, err)
	}
	hash, err := releasedbaseline.HashFields(record.Fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.baselines.Save(releasedbaseline.Document{EnvironmentID: "development", ReleaseID: "rel_test", ReleasedAt: time.Now().UTC(), SourceRevision: draftView.SourceRevision, Entities: map[string]string{created.EntityRef: hash}}); err != nil {
		t.Fatal(err)
	}
	unchanged, revision, err := s.GetEntity(context.Background(), "development", "frequency_policy", record.ID)
	if err != nil || unchanged.ChangeStatus != "unchanged" {
		t.Fatalf("unchanged=%#v revision=%d err=%v", unchanged, revision, err)
	}
	draftView, _, err = s.GetDraft(context.Background(), "development")
	if err != nil || draftView.ChangedEntityCount != 0 {
		t.Fatalf("draft=%#v err=%v", draftView, err)
	}
	changed := cloneRecord(record)
	changed.Fields["cooldown_ms"] = float64(60000)
	modified, _, err := s.MutateEntity(context.Background(), "development", EntityMutation{ExpectedRevision: revision, ExpectedSourceRevision: draftView.SourceRevision, Scope: "baseline", EntityType: "frequency_policy", EntityID: record.ID, Entity: &changed, Action: "replace"})
	if err != nil || modified.ChangeStatus != "modified" {
		t.Fatalf("modified=%#v err=%v", modified, err)
	}
	draftView, _, err = s.GetDraft(context.Background(), "development")
	if err != nil || draftView.ChangedEntityCount != 1 {
		t.Fatalf("draft=%#v err=%v", draftView, err)
	}
}

func TestReleaseBlockingPlanCannotBeConfirmedAway(t *testing.T) {
	s, fake := releaseService(t)
	p := buildReadyPlan(t, s)
	p.BlockingReasons = []plan.BlockingReason{{ReasonCode: "unmodeled_remote_condition", Summary: "blocked"}}
	if err := s.plans.Update(p); err != nil {
		t.Fatal(err)
	}
	_, err := s.StartRelease(context.Background(), "development", "publish-key-0000004", releaseRequest(p))
	var invalidated *PlanInvalidatedError
	if !errors.As(err, &invalidated) || len(fake.published) != 0 {
		t.Fatalf("err=%#v published=%s", err, fake.published)
	}
}

func TestReleaseConfirmationPolicyAndRiskSets(t *testing.T) {
	base := plan.Plan{ConfirmationRequirements: plan.ConfirmationRequirements{RequiresAcknowledgement: true, RequiredRiskItemIDs: []string{}}}
	production := project.Environment{ID: "production", Kind: "production"}
	lowAcknowledged := ReleaseConfirmation{Acknowledged: true}
	if err := validateReleaseConfirmation(base, production, project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "acknowledgement"}, lowAcknowledged); err != nil {
		t.Fatalf("low acknowledgement=%v", err)
	}
	if err := validateReleaseConfirmation(base, production, project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "environment_id"}, lowAcknowledged); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("low environment id=%v", err)
	}
	base.ConfirmationRequirements.RequiredRiskItemIDs = []string{"risk_high"}
	if err := validateReleaseConfirmation(base, production, project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "acknowledgement"}, ReleaseConfirmation{Acknowledged: true, AcknowledgedRiskItemIDs: []string{"risk_high"}}); err != nil {
		t.Fatalf("high risk=%v", err)
	}
	if err := validateReleaseConfirmation(base, production, project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "acknowledgement"}, ReleaseConfirmation{Acknowledged: true, AcknowledgedRiskItemIDs: []string{"unknown"}}); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("unknown risk=%v", err)
	}
}

func TestReleaseContractFixtureScenariosAreConsumed(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v1", "plan-risk-operation-rollback.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Scenarios []struct {
			ID                     string `json:"id"`
			ExpectedPublishRequest struct {
				IdempotencyKey string `json:"idempotency_key"`
				Confirmation   struct {
					AcknowledgedRiskItemIDs []string `json:"acknowledged_risk_item_ids"`
				} `json:"confirmation"`
			} `json:"expected_publish_request"`
			Expected struct {
				ErrorCode        string `json:"error_code"`
				RemoteUpdateSent bool   `json:"remote_update_sent"`
			} `json:"expected"`
			ExpectedPreview struct {
				RollbackPreviewID  string `json:"rollback_preview_id"`
				ExpectedRemoteETag string `json:"expected_remote_etag"`
			} `json:"expected_preview"`
			ExpectedRollbackRequest struct {
				IdempotencyKey string `json:"idempotency_key"`
			} `json:"expected_rollback_request"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Scenarios) < 4 || fixture.Scenarios[0].ExpectedPublishRequest.IdempotencyKey == "" || len(fixture.Scenarios[0].ExpectedPublishRequest.Confirmation.AcknowledgedRiskItemIDs) == 0 {
		t.Fatalf("publish fixture=%#v", fixture.Scenarios)
	}
	if fixture.Scenarios[1].Expected.ErrorCode != "remote_etag_mismatch" || fixture.Scenarios[1].Expected.RemoteUpdateSent {
		t.Fatalf("etag fixture=%#v", fixture.Scenarios[1])
	}
	rollback := fixture.Scenarios[3]
	if rollback.ExpectedPreview.RollbackPreviewID == "" || rollback.ExpectedPreview.ExpectedRemoteETag == "" || rollback.ExpectedRollbackRequest.IdempotencyKey == "" {
		t.Fatalf("rollback fixture=%#v", rollback)
	}
}

func TestRollbackPreviewETagRecoveryAndNewRelease(t *testing.T) {
	s, fake := releaseService(t)
	p := buildReadyPlan(t, s)
	published, err := s.StartRelease(context.Background(), "development", "publish-key-rollback-0001", releaseRequest(p))
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, published.OperationID); completed.Status != "succeeded" {
		t.Fatalf("publish=%#v", completed)
	}
	targets := s.Releases(context.Background(), "development")
	if len(targets) != 1 || targets[0].RemoteAfter == nil {
		t.Fatalf("targets=%#v", targets)
	}
	target := targets[0]
	fake.template.ETag, fake.template.Version = "etag-current-rollback", "3"
	previewOperation, err := s.CreateRollbackPreview(context.Background(), "development", target.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, previewOperation.OperationID); completed.Status != "succeeded" || completed.Result == nil || completed.Result.ResourceType != "rollback_preview" {
		t.Fatalf("preview=%#v", completed)
	}
	preview, err := s.RollbackPreview(context.Background(), "development", target.ReleaseID)
	if err != nil || preview.Status != "ready" || preview.ExpectedRemoteETag != "etag-current-rollback" {
		t.Fatalf("preview=%#v err=%v", preview, err)
	}
	fake.template.ETag = "etag-raced"
	_, err = s.StartRollback(context.Background(), "development", target.ReleaseID, "rollback-key-0001", RollbackRequest{RollbackPreviewID: preview.RollbackPreviewID, ExpectedRemoteETag: preview.ExpectedRemoteETag, Confirmation: ReleaseConfirmation{Acknowledged: true}})
	var mismatch *RemoteETagMismatchError
	if !errors.As(err, &mismatch) || mismatch.Current.RemoteETag != "etag-raced" {
		t.Fatalf("mismatch=%v", err)
	}
	previewOperation, err = s.CreateRollbackPreview(context.Background(), "development", target.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, previewOperation.OperationID); completed.Status != "succeeded" {
		t.Fatalf("rebuild=%#v", completed)
	}
	preview, err = s.RollbackPreview(context.Background(), "development", target.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := s.StartRollback(context.Background(), "development", target.ReleaseID, "rollback-key-0002", RollbackRequest{RollbackPreviewID: preview.RollbackPreviewID, ExpectedRemoteETag: preview.ExpectedRemoteETag, Confirmation: ReleaseConfirmation{Acknowledged: true}})
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, rollback.OperationID); completed.Status != "succeeded" || completed.Result == nil {
		t.Fatalf("rollback=%#v", completed)
	}
	result, ok := s.Release(context.Background(), waitAppOperation(t, s, rollback.OperationID).Result.ResourceID)
	if !ok || result.Kind != "rollback" || result.Outcome != "succeeded" || result.RollbackOfReleaseID != target.ReleaseID {
		t.Fatalf("rollback release=%#v", result)
	}
}

func TestDefaultsExportFormatsCarrySourceDigest(t *testing.T) {
	s, _ := releaseService(t)
	p := buildReadyPlan(t, s)
	started, err := s.StartRelease(context.Background(), "development", "publish-key-defaults-0001", releaseRequest(p))
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, started.OperationID); completed.Status != "succeeded" {
		t.Fatalf("release=%#v", completed)
	}
	for _, format := range []string{"json", "xml", "plist"} {
		content, filename, _, err := s.Defaults(context.Background(), "development", format)
		if err != nil || filename == "" || !strings.Contains(string(content), "sha256:") || !strings.Contains(string(content), "unmanaged") {
			t.Fatalf("format=%s filename=%s err=%v content=%s", format, filename, err, content)
		}
	}
}

func TestDefaultsGoldenFormatsAndDigest(t *testing.T) {
	s, _ := releaseService(t)
	snapshot, err := remote.SnapshotFromTemplate([]byte(`{"parameters":{"alpha":{"defaultValue":{"value":"on"}},"beta":{"defaultValue":{"value":"2"}}}}`), "etag-golden", "7", time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.remote.(*remote.FileStore).Save("development", snapshot); err != nil {
		t.Fatal(err)
	}
	const digest = "sha256:2e069d117170e22fac69a1e9367509a40b5cad1e856c5b3da692d2105514246a"
	wants := map[string]string{
		"json":  "{\n  \"defaults\": {\n    \"alpha\": \"on\",\n    \"beta\": \"2\"\n  },\n  \"metadata\": {\n    \"digest\": \"" + digest + "\",\n    \"source_etag\": \"etag-golden\",\n    \"source_version\": \"7\"\n  }\n}\n",
		"xml":   "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<defaults source_version=\"7\" source_etag=\"etag-golden\" digest=\"" + digest + "\">\n  <entry key=\"alpha\" value=\"on\"/>\n  <entry key=\"beta\" value=\"2\"/>\n</defaults>\n",
		"plist": "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n<key>_conflow_metadata</key><dict><key>source_version</key><string>7</string><key>source_etag</key><string>etag-golden</string><key>digest</key><string>" + digest + "</string></dict>\n<key>alpha</key><string>on</string>\n<key>beta</key><string>2</string>\n</dict></plist>\n",
	}
	for format, want := range wants {
		content, _, _, err := s.Defaults(context.Background(), "development", format)
		if err != nil || string(content) != want {
			t.Fatalf("format=%s err=%v\nwant:\n%s\ngot:\n%s", format, err, want, content)
		}
	}
}

func TestFailedRollbackLeavesFailedAudit(t *testing.T) {
	s, fake := releaseService(t)
	p := buildReadyPlan(t, s)
	started, err := s.StartRelease(context.Background(), "development", "publish-key-failed-rollback-0001", releaseRequest(p))
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, started.OperationID); completed.Status != "succeeded" {
		t.Fatalf("publish=%#v", completed)
	}
	target := s.Releases(context.Background(), "development")[0]
	previewOperation, err := s.CreateRollbackPreview(context.Background(), "development", target.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, previewOperation.OperationID); completed.Status != "succeeded" {
		t.Fatalf("preview=%#v", completed)
	}
	preview, err := s.RollbackPreview(context.Background(), "development", target.ReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	fake.validateErr = provider.ErrValidation
	rollback, err := s.StartRollback(context.Background(), "development", target.ReleaseID, "rollback-key-failed-0001", RollbackRequest{RollbackPreviewID: preview.RollbackPreviewID, ExpectedRemoteETag: preview.ExpectedRemoteETag, Confirmation: ReleaseConfirmation{Acknowledged: true}})
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, rollback.OperationID); completed.Status != "failed" {
		t.Fatalf("rollback=%#v", completed)
	}
	var failed release.Release
	for _, item := range s.Releases(context.Background(), "development") {
		if item.OperationID == rollback.OperationID {
			failed = item
			break
		}
	}
	if failed.ReleaseID == "" || failed.Kind != "rollback" || failed.Outcome != "failed" || failed.RollbackOfReleaseID != target.ReleaseID || failed.Failure == nil || failed.RemoteAfter != nil {
		t.Fatalf("failed rollback audit=%#v", failed)
	}
}

func TestReleaseHistoryRemainsReadableWithoutLocalCredentials(t *testing.T) {
	s, _ := releaseService(t)
	p := buildReadyPlan(t, s)
	started, err := s.StartRelease(context.Background(), "development", "publish-key-history-0001", releaseRequest(p))
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitAppOperation(t, s, started.OperationID); completed.Status != "succeeded" {
		t.Fatalf("release=%#v", completed)
	}
	if err := os.RemoveAll(filepath.Join(s.workspace, ".conflow", "providers")); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.workspace)
	if err != nil {
		t.Fatal(err)
	}
	items, err := reopened.ReleasesPage(context.Background(), "development", 0, "")
	if err != nil || len(items) != 1 || items[0].Outcome != "succeeded" {
		t.Fatalf("history=%#v err=%v", items, err)
	}
}

func releaseService(t *testing.T) (*Service, *fakeFirebase) {
	t.Helper()
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	fake := &fakeFirebase{template: providerTemplate("etag-before")}
	s, err := OpenWithPacksAndProviderFactory(workspace, packs.BuiltinRegistry(), func(project.Environment) (provider.Adapter, error) { return fake, nil })
	if err != nil {
		t.Fatal(err)
	}
	return s, fake
}

func providerTemplate(etag string) provider.Template {
	return provider.Template{ETag: etag, Version: "1", ObservedAt: time.Now().UTC(), Raw: []byte(`{"parameters":{"unmanaged":{"defaultValue":{"value":"keep"}}}}`)}
}
func releaseRequest(p plan.Plan) ReleaseRequest {
	return ReleaseRequest{PlanID: p.PlanID, ExpectedDraftRevision: p.DraftRevision, ExpectedRemoteETag: *p.RemoteETag, Confirmation: ReleaseConfirmation{Acknowledged: true, AcknowledgedRiskItemIDs: append([]string(nil), p.ConfirmationRequirements.RequiredRiskItemIDs...)}}
}
