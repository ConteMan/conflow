package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/operation"
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
	return provider.Capabilities{Pull: true, Validate: true, Publish: true, Rollback: true}
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

func TestReleaseRollbackAndDefaultsHTTPMappings(t *testing.T) {
	service, _ := releaseHTTPService(t)
	p := readyReleasePlan(t, service)
	started, err := service.StartRelease(context.Background(), "development", "release-http-rollback-0001", app.ReleaseRequest{PlanID: p.PlanID, ExpectedDraftRevision: p.DraftRevision, ExpectedRemoteETag: *p.RemoteETag, Confirmation: app.ReleaseConfirmation{Acknowledged: true, AcknowledgedRiskItemIDs: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitReleaseOperation(t, service, started.OperationID); completed.Status != "succeeded" || completed.Result == nil {
		t.Fatalf("release=%#v", completed)
	}
	releaseID := waitReleaseOperation(t, service, started.OperationID).Result.ResourceID
	handler := New(service)
	for _, test := range []struct {
		method, path string
		body         []byte
		status       int
	}{
		{http.MethodGet, "/api/v1/environments/development/releases", nil, http.StatusOK},
		{http.MethodGet, "/api/v1/environments/development/releases/rel_missing", nil, http.StatusNotFound},
		{http.MethodPost, "/api/v1/environments/development/releases/rel_missing:rollback-preview", []byte(`{}`), http.StatusNotFound},
		{http.MethodGet, "/api/v1/environments/development/releases/" + releaseID + "/rollback-preview", nil, http.StatusNotFound},
		{http.MethodGet, "/api/v1/environments/development/defaults?format=json", nil, http.StatusOK},
		{http.MethodGet, "/api/v1/environments/development/defaults?format=ini", nil, http.StatusBadRequest},
	} {
		request := newAPIRequest(t, test.method, test.path, test.body)
		if test.method == http.MethodPost {
			request.Header.Set("Content-Type", "application/json")
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("%s %s status=%d body=%s", test.method, test.path, recorder.Code, recorder.Body.String())
		}
	}
	request := newAPIRequest(t, http.MethodPost, "/api/v1/environments/development/releases/"+releaseID+":rollback-preview", []byte(`{}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("preview=%d %s", recorder.Code, recorder.Body.String())
	}
	var previewStarted struct {
		Data struct {
			OperationID string `json:"operation_id"`
		} `json:"data"`
	}
	decodeResponse(t, recorder, &previewStarted)
	if completed := waitReleaseOperation(t, service, previewStarted.Data.OperationID); completed.Status != "succeeded" {
		t.Fatalf("preview operation=%#v", completed)
	}
	request = newAPIRequest(t, http.MethodGet, "/api/v1/environments/development/releases/"+releaseID+"/rollback-preview", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("get preview=%d %s", recorder.Code, recorder.Body.String())
	}
	var preview struct {
		Data struct {
			RollbackPreviewID  string `json:"rollback_preview_id"`
			ExpectedRemoteETag string `json:"expected_remote_etag"`
		} `json:"data"`
	}
	decodeResponse(t, recorder, &preview)
	request = newAPIRequest(t, http.MethodPost, "/api/v1/environments/development/releases/"+releaseID+":rollback", []byte(`{"rollback_preview_id":"rbp_missing","expected_remote_etag":"x","confirmation":{"acknowledged":true,"acknowledged_risk_item_ids":[]}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "rollback-http-missing-preview")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing preview=%d %s", recorder.Code, recorder.Body.String())
	}
	body, _ := json.Marshal(map[string]any{"rollback_preview_id": preview.Data.RollbackPreviewID, "expected_remote_etag": preview.Data.ExpectedRemoteETag, "confirmation": map[string]any{"acknowledged": true, "acknowledged_risk_item_ids": []string{}}})
	request = newAPIRequest(t, http.MethodPost, "/api/v1/environments/development/releases/"+releaseID+":rollback", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "rollback-http-success-0001")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("rollback=%d %s", recorder.Code, recorder.Body.String())
	}
	var rollbackStarted struct {
		Data struct {
			OperationID string `json:"operation_id"`
		} `json:"data"`
	}
	decodeResponse(t, recorder, &rollbackStarted)
	if completed := waitReleaseOperation(t, service, rollbackStarted.Data.OperationID); completed.Status != "succeeded" {
		t.Fatalf("rollback operation=%#v", completed)
	}
}

func TestRollbackExpiredPreviewHTTPMapsToConflict(t *testing.T) {
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	firebase := &releaseFirebase{template: provider.Template{Raw: []byte(`{"parameters":{"unmanaged":{"defaultValue":{"value":"keep"}}}}`), ETag: "etag-before", Version: "1", ObservedAt: time.Now().UTC()}}
	newService := func() *app.Service {
		service, err := app.OpenWithPacksAndProviderFactory(workspace, packs.BuiltinRegistry(), func(project.Environment) (provider.Adapter, error) { return firebase, nil })
		if err != nil {
			t.Fatal(err)
		}
		return service
	}
	service := newService()
	p := readyReleasePlan(t, service)
	started, err := service.StartRelease(context.Background(), "development", "expired-preview-publish-0001", app.ReleaseRequest{PlanID: p.PlanID, ExpectedDraftRevision: p.DraftRevision, ExpectedRemoteETag: *p.RemoteETag, Confirmation: app.ReleaseConfirmation{Acknowledged: true, AcknowledgedRiskItemIDs: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitReleaseOperation(t, service, started.OperationID); completed.Status != "succeeded" || completed.Result == nil {
		t.Fatalf("release=%#v", completed)
	}
	releaseID := waitReleaseOperation(t, service, started.OperationID).Result.ResourceID
	previewOp, err := service.CreateRollbackPreview(context.Background(), "development", releaseID)
	if err != nil {
		t.Fatal(err)
	}
	if completed := waitReleaseOperation(t, service, previewOp.OperationID); completed.Status != "succeeded" {
		t.Fatalf("preview=%#v", completed)
	}
	preview, err := service.RollbackPreview(context.Background(), "development", releaseID)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, ".conflow", "releases.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]any
	if err := json.Unmarshal(content, &disk); err != nil {
		t.Fatal(err)
	}
	previews := disk["previews"].(map[string]any)
	raw := previews[preview.RollbackPreviewID].(map[string]any)
	raw["expires_at"] = "2000-01-01T00:00:00Z"
	content, _ = json.Marshal(disk)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	service = newService()
	body, _ := json.Marshal(map[string]any{"rollback_preview_id": preview.RollbackPreviewID, "expected_remote_etag": preview.ExpectedRemoteETag, "confirmation": map[string]any{"acknowledged": true, "acknowledged_risk_item_ids": []string{}}})
	request := newAPIRequest(t, http.MethodPost, "/api/v1/environments/development/releases/"+releaseID+":rollback", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "expired-preview-rollback-0001")
	recorder := httptest.NewRecorder()
	New(service).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "rollback_preview_invalid") {
		t.Fatalf("expired=%d %s", recorder.Code, recorder.Body.String())
	}
}

func waitReleaseOperation(t *testing.T, service *app.Service, operationID string) operation.Operation {
	t.Helper()
	for deadline := time.Now().Add(time.Second); ; {
		op, err := service.Operation(context.Background(), operationID)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status == "succeeded" || op.Status == "failed" {
			return op
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation timeout=%#v", op)
		}
		time.Sleep(time.Millisecond)
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
