package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/project"
)

func TestDraftHandlerValidationOrder(t *testing.T) {
	handler, _ := newTestHandlerWithPacks(t, packs.MustNewRegistry(draftHTTPPack()))
	valid := []byte(`{"expected_source_revision":"ignored","write_scope":"baseline","configuration":{"setting":{"enabled":true}}}`)
	invalid := []byte(`{`)
	tests := []struct {
		name        string
		path        string
		ifMatch     string
		body        []byte
		contentType string
		origin      string
		status      int
		code        string
	}{
		{name: "content type before If-Match", path: "/api/v1/drafts/development", body: valid, contentType: "text/plain", status: http.StatusUnsupportedMediaType, code: "unsupported_media_type"},
		{name: "origin before If-Match", path: "/api/v1/drafts/development", body: valid, contentType: "application/json", origin: "http://example.test", status: http.StatusForbidden, code: "invalid_origin"},
		{name: "missing If-Match before body decode", path: "/api/v1/drafts/development", body: invalid, contentType: "application/json", status: http.StatusPreconditionRequired, code: "precondition_required"},
		{name: "malformed If-Match before body decode", path: "/api/v1/drafts/development", ifMatch: "1", body: invalid, contentType: "application/json", status: http.StatusBadRequest, code: "invalid_request"},
		{name: "malformed body", path: "/api/v1/drafts/development", ifMatch: `"1"`, body: invalid, contentType: "application/json", status: http.StatusBadRequest, code: "malformed_json"},
		{name: "unknown body field", path: "/api/v1/drafts/development", ifMatch: `"1"`, body: []byte(`{"expected_source_revision":"x","write_scope":"baseline","configuration":{},"unknown":true}`), contentType: "application/json", status: http.StatusBadRequest, code: "malformed_json"},
		{name: "missing required body field", path: "/api/v1/drafts/development", ifMatch: `"1"`, body: []byte(`{"write_scope":"baseline","configuration":{}}`), contentType: "application/json", status: http.StatusBadRequest, code: "malformed_json"},
		{name: "unknown environment before conflicts", path: "/api/v1/drafts/missing", ifMatch: `"9"`, body: valid, contentType: "application/json", status: http.StatusNotFound, code: "environment_not_found"},
		{name: "invalid write scope before conflicts", path: "/api/v1/drafts/development", ifMatch: `"9"`, body: []byte(`{"expected_source_revision":"x","write_scope":"other","configuration":{}}`), contentType: "application/json", status: http.StatusBadRequest, code: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := newAPIRequest(t, http.MethodPut, test.path, test.body)
			request.Header.Set("Content-Type", test.contentType)
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.ifMatch != "" {
				request.Header.Set("If-Match", test.ifMatch)
			}
			recorder := executeDraftRequest(handler, request)
			assertDraftError(t, recorder, test.status, test.code)
		})
	}
	t.Run("missing Pack before conflicts", func(t *testing.T) {
		handler, workspace := newTestHandlerWithPacks(t, packs.MustNewRegistry(draftHTTPPack()))
		path := project.ManifestPath(workspace)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(content), "mobile-ad-monetization/v1", "missing-pack/v1", 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		recorder := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/development", `"9"`, valid)
		assertDraftError(t, recorder, http.StatusNotFound, "pack_not_found")
	})
}

func TestDraftHandlerConflictsAndStructuralErrors(t *testing.T) {
	handler, _ := newTestHandlerWithPacks(t, packs.MustNewRegistry(draftHTTPPack()))
	initial := getDraftForTest(t, handler, "development")

	staleBody := []byte(`{"expected_source_revision":"wrong","write_scope":"baseline","configuration":{"setting":{"enabled":null}}}`)
	stale := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/development", `"2"`, staleBody)
	assertDraftError(t, stale, http.StatusPreconditionFailed, "revision_mismatch")
	var revisionConflict draftConflictEnvelope
	decodeResponse(t, stale, &revisionConflict)
	if revisionConflict.Error.CurrentRevision != 1 || revisionConflict.Error.CurrentSourceRevision != initial.SourceRevision || revisionConflict.Error.ConflictScope != "baseline" || stale.Header().Get("ETag") != `"1"` || revisionConflict.Error.CurrentState.EnvironmentID != "development" {
		t.Fatalf("revision conflict = %#v", revisionConflict)
	}

	sourceBody := []byte(`{"expected_source_revision":"wrong","write_scope":"baseline","configuration":{"setting":{"enabled":null}}}`)
	source := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/development", `"1"`, sourceBody)
	assertDraftError(t, source, http.StatusPreconditionFailed, "source_revision_mismatch")
	var sourceConflict draftConflictEnvelope
	decodeResponse(t, source, &sourceConflict)
	if sourceConflict.Error.CurrentRevision != 1 || sourceConflict.Error.CurrentSourceRevision != initial.SourceRevision || sourceConflict.Error.ConflictScope != "baseline" || source.Header().Get("ETag") != `"1"` {
		t.Fatalf("source conflict = %#v", sourceConflict)
	}

	body := []byte(`{"expected_source_revision":"` + initial.SourceRevision + `","write_scope":"baseline","configuration":{"setting":{"tags":"not-an-array","label":"","enabled":null}}}`)
	validation := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/development", `"1"`, body)
	assertDraftError(t, validation, http.StatusUnprocessableEntity, "validation_failed")
	var response draftValidationEnvelope
	decodeResponse(t, validation, &response)
	if len(response.Error.Details) != 3 {
		t.Fatalf("details = %#v", response.Error.Details)
	}
	for index, want := range []struct{ code, path string }{{"explicit_null_forbidden", "/setting/enabled"}, {"value_not_allowed", "/setting/label"}, {"field_type_mismatch", "/setting/tags"}} {
		got := response.Error.Details[index]
		if got.Code != want.code || got.Path != want.path || got.Scope != "baseline" || got.Message == "" {
			t.Fatalf("detail %d = %#v", index, got)
		}
	}
}

func TestDraftHandlerSharedRevisionDoesNotPolluteCurrentView(t *testing.T) {
	handler, _ := newTestHandlerWithPacks(t, packs.MustNewRegistry(draftHTTPPack()))
	development := getDraftForTest(t, handler, "development")
	body := []byte(`{"expected_source_revision":"` + development.SourceRevision + `","write_scope":"environment_override","configuration":{"setting":{"enabled":true}}}`)
	updated := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/production", `"1"`, body)
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"2"` {
		t.Fatalf("write status = %d, body = %s", updated.Code, updated.Body.String())
	}
	view := getDraftForTest(t, handler, "development")
	if view.Dirty || len(view.DirtyScopes) != 0 || len(view.AffectedEnvironments) != 0 {
		t.Fatalf("development view polluted: %#v", view)
	}
	stale := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/development", `"1"`, body)
	assertDraftError(t, stale, http.StatusPreconditionFailed, "revision_mismatch")
}

func TestDraftHandlerResetAndDiscard(t *testing.T) {
	handler, _ := newTestHandlerWithPacks(t, packs.MustNewRegistry(draftHTTPPack()))
	initial := getDraftForTest(t, handler, "development")
	body := []byte(`{"expected_source_revision":"` + initial.SourceRevision + `","write_scope":"baseline"}`)
	reset := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development:reset", `"1"`, body)
	if reset.Code != http.StatusOK || reset.Header().Get("ETag") != `"2"` {
		t.Fatalf("reset status = %d, body = %s", reset.Code, reset.Body.String())
	}
	var resetResponse struct {
		Data draftViewDTO `json:"data"`
	}
	decodeResponse(t, reset, &resetResponse)
	if !resetResponse.Data.Baseline.Draft.Present || len(resetResponse.Data.Baseline.Draft.Value) != 0 || !resetResponse.Data.Dirty {
		t.Fatalf("reset view = %#v", resetResponse.Data)
	}

	discard := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development:discard", `"2"`, body)
	if discard.Code != http.StatusOK || discard.Header().Get("ETag") != `"3"` {
		t.Fatalf("discard status = %d, body = %s", discard.Code, discard.Body.String())
	}
	var discardResponse struct {
		Data draftViewDTO `json:"data"`
	}
	decodeResponse(t, discard, &discardResponse)
	if discardResponse.Data.Baseline.Draft.Present || discardResponse.Data.Dirty {
		t.Fatalf("discard view = %#v", discardResponse.Data)
	}
}

func TestDraftSaveWritesManagedFileAndClearsVisibleDraft(t *testing.T) {
	handler, workspace := newTestHandlerWithPacks(t, packs.MustNewRegistry(draftHTTPPack()))
	initial := getDraftForTest(t, handler, "development")
	put := []byte(`{"expected_source_revision":"` + initial.SourceRevision + `","write_scope":"baseline","configuration":{"setting":{"enabled":true}}}`)
	changed := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/development", `"1"`, put)
	if changed.Code != http.StatusOK {
		t.Fatalf("draft write status = %d: %s", changed.Code, changed.Body.String())
	}
	var changedResponse struct {
		Data draftViewDTO `json:"data"`
		Meta responseMeta `json:"meta"`
	}
	decodeResponse(t, changed, &changedResponse)
	save := []byte(`{"expected_source_revision":"` + changedResponse.Data.SourceRevision + `"}`)
	saved := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development:save", `"2"`, save)
	if saved.Code != http.StatusOK || saved.Header().Get("ETag") != `"3"` {
		t.Fatalf("save status = %d, etag = %q, body = %s", saved.Code, saved.Header().Get("ETag"), saved.Body.String())
	}
	var savedResponse struct {
		Data draftViewDTO `json:"data"`
	}
	decodeResponse(t, saved, &savedResponse)
	if savedResponse.Data.Dirty || savedResponse.Data.Baseline.Draft.Present || savedResponse.Data.SourceRevision == changedResponse.Data.SourceRevision {
		t.Fatalf("saved draft = %#v", savedResponse.Data)
	}
	content, err := os.ReadFile(filepath.Join(workspace, ".conflow", "data", "base.yaml"))
	if err != nil || !strings.Contains(string(content), "enabled: true") {
		t.Fatalf("managed base = %q, err = %v", content, err)
	}
}

func TestDraftSaveExternalSourceChangeReturnsConflict(t *testing.T) {
	handler, workspace := newTestHandlerWithPacks(t, packs.MustNewRegistry(draftHTTPPack()))
	initial := getDraftForTest(t, handler, "development")
	put := []byte(`{"expected_source_revision":"` + initial.SourceRevision + `","write_scope":"baseline","configuration":{"setting":{"enabled":true}}}`)
	if response := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/development", `"1"`, put); response.Code != http.StatusOK {
		t.Fatalf("draft write = %d: %s", response.Code, response.Body.String())
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".conflow", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".conflow", "data", "base.yaml"), []byte("setting:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	conflict := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development:save", `"2"`, []byte(`{"expected_source_revision":"`+initial.SourceRevision+`"}`))
	assertDraftError(t, conflict, http.StatusPreconditionFailed, "source_revision_mismatch")
}

func getDraftForTest(t *testing.T, handler http.Handler, environmentID string) draftViewDTO {
	t.Helper()
	recorder := executeRequest(t, handler, http.MethodGet, "/api/v1/drafts/"+environmentID, "", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data draftViewDTO `json:"data"`
	}
	decodeResponse(t, recorder, &response)
	return response.Data
}

func executeDraftRequest(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertDraftError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d, body = %s", recorder.Code, status, recorder.Body.String())
	}
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if response.Error.Code != code {
		t.Fatalf("code = %q, want %q; body = %s", response.Error.Code, code, recorder.Body.String())
	}
}

func draftHTTPPack() packs.Definition {
	minLength := 1
	return packs.Definition{
		Metadata: packs.Metadata{
			Name: "mobile-ad-monetization", Version: "v1", Description: "Draft HTTP test Pack.",
			EntityTypes: []packs.EntityMetadata{{Name: "setting", Label: "Setting", Description: "A setting.", IDRule: packs.IDRule{Pattern: "^[a-z]+$", MinLength: 1, MaxLength: 63}, DeletionPolicy: packs.DeletionPolicyRestrict, EnvironmentOverrideFields: []string{"enabled", "label", "tags"}}},
		},
		Schema: packs.Schema{Version: 1, Entities: []packs.EntitySchema{{Name: "setting", Fields: []packs.FieldSchema{
			{Name: "enabled", Type: packs.FieldTypeBoolean, Required: true, Nullable: false, Default: json.RawMessage("false"), Sensitivity: packs.SensitivityPublic, UI: packs.FieldUI{Label: "Enabled", Description: "", Control: "switch", Group: "general"}, Validation: packs.FieldValidation{Enum: []json.RawMessage{}}},
			{Name: "label", Type: packs.FieldTypeString, Required: true, Nullable: false, Default: json.RawMessage(`"default"`), Sensitivity: packs.SensitivityPublic, UI: packs.FieldUI{Label: "Label", Description: "", Control: "input", Group: "general"}, Validation: packs.FieldValidation{Enum: []json.RawMessage{}, MinLength: &minLength}},
			{Name: "tags", Type: packs.FieldTypeArray, Required: true, Nullable: false, Default: json.RawMessage(`[]`), Sensitivity: packs.SensitivityPublic, UI: packs.FieldUI{Label: "Tags", Description: "", Control: "input", Group: "general"}, Validation: packs.FieldValidation{Enum: []json.RawMessage{}}},
		}}}},
	}
}
