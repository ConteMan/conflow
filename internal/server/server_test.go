package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/project"
)

func TestHealthEndpoint(t *testing.T) {
	handler, _ := newTestHandler(t)
	recorder := executeRequest(t, handler, http.MethodGet, "/api/v1/health", "", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q, want no-store", got)
	}
}

func TestFrontendServesIndexWithoutRedirect(t *testing.T) {
	handler, _ := newTestHandler(t)
	for _, requestPath := range []string{"/", "/environments"} {
		t.Run(requestPath, func(t *testing.T) {
			recorder := executeRequest(t, handler, http.MethodGet, requestPath, "", nil)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, location = %q", recorder.Code, recorder.Header().Get("Location"))
			}
			if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
				t.Fatalf("content type = %q, want text/html", got)
			}
			if !strings.Contains(recorder.Body.String(), `<div id="root"></div>`) {
				t.Fatalf("response does not contain React root: %s", recorder.Body.String())
			}
		})
	}
}

func TestBootstrapReturnsRevisionAndRequestID(t *testing.T) {
	handler, _ := newTestHandler(t)
	recorder := executeRequest(t, handler, http.MethodGet, "/api/v1/bootstrap", "", nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("ETag") != `"1"` {
		t.Fatalf("etag = %q", recorder.Header().Get("ETag"))
	}
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID")
	}
	var response struct {
		Data bootstrapData `json:"data"`
		Meta responseMeta  `json:"meta"`
	}
	decodeResponse(t, recorder, &response)
	if response.Data.Project.ID != "photo-editor" || len(response.Data.Environments) != 2 {
		t.Fatalf("bootstrap = %#v", response.Data)
	}
	if response.Data.Environments[1].Name != "Production" || response.Data.Environments[1].Kind != "production" {
		t.Fatalf("production environment = %#v", response.Data.Environments[1])
	}
	if response.Meta.RequestID == "" || response.Meta.Revision != 1 {
		t.Fatalf("meta = %#v", response.Meta)
	}
	if response.Meta.RequestID != recorder.Header().Get("X-Request-ID") {
		t.Fatalf("meta request ID = %q, header = %q", response.Meta.RequestID, recorder.Header().Get("X-Request-ID"))
	}
}

func TestSourceEndpointsExposeManagedFileStatusWithoutAbsolutePaths(t *testing.T) {
	handler, _ := newTestHandler(t)
	source := executeRequest(t, handler, http.MethodGet, "/api/v1/source", "", nil)
	if source.Code != http.StatusOK {
		t.Fatalf("source status = %d: %s", source.Code, source.Body.String())
	}
	var sourceResponse struct {
		Data sourceDTO `json:"data"`
	}
	decodeResponse(t, source, &sourceResponse)
	if sourceResponse.Data.Type != "managed-file" || !sourceResponse.Data.Capabilities.Load || !sourceResponse.Data.Capabilities.Save {
		t.Fatalf("source = %#v", sourceResponse.Data)
	}
	status := executeRequest(t, handler, http.MethodGet, "/api/v1/source/status", "", nil)
	if status.Code != http.StatusOK {
		t.Fatalf("source status = %d: %s", status.Code, status.Body.String())
	}
	var statusResponse struct {
		Data sourceStatusDTO `json:"data"`
	}
	decodeResponse(t, status, &statusResponse)
	if statusResponse.Data.Digest == "" || len(statusResponse.Data.Paths) != 1 || statusResponse.Data.Paths[0] != ".conflow/data/base.yaml" {
		t.Fatalf("status = %#v", statusResponse.Data)
	}
}

func TestProjectUpdateAndRevisionConflict(t *testing.T) {
	handler, _ := newTestHandler(t)
	body := `{"id":"photo-editor","name":"Updated Photo Editor"}`
	updated := executeRequest(t, handler, http.MethodPut, "/api/v1/project", `"1"`, []byte(body))
	if updated.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", updated.Code, updated.Body.String())
	}
	if updated.Header().Get("ETag") != `"2"` {
		t.Fatalf("etag = %q", updated.Header().Get("ETag"))
	}

	stale := executeRequest(t, handler, http.MethodPut, "/api/v1/project", `"1"`, []byte(body))
	if stale.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, body = %s", stale.Code, stale.Body.String())
	}
	var response manifestRevisionMismatchEnvelope
	decodeResponse(t, stale, &response)
	if response.Error.Code != "revision_mismatch" || response.Error.CurrentRevision != 2 {
		t.Fatalf("error = %#v", response.Error)
	}
	if stale.Header().Get("ETag") != `"2"` || response.Error.CurrentState.Project.Name != "Updated Photo Editor" || len(response.Error.CurrentState.Environments) != 2 {
		t.Fatalf("conflict state = %#v, etag = %q", response.Error.CurrentState, stale.Header().Get("ETag"))
	}
}

func TestEnvironmentLifecycle(t *testing.T) {
	handler, _ := newTestHandler(t)
	createBody := `{"id":"staging","name":"Staging","kind":"staging","provider":{"type":"firebase-remote-config","project_id":"photo-editor-staging"},"publish":{"requires_confirmation":true}}`
	created := executeRequest(t, handler, http.MethodPost, "/api/v1/environments", `"1"`, []byte(createBody))
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", created.Code, created.Body.String())
	}

	got := executeRequest(t, handler, http.MethodGet, "/api/v1/environments/staging", "", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", got.Code, got.Body.String())
	}

	updateBody := `{"name":"QA Staging","provider":{"type":"firebase-remote-config","project_id":"photo-editor-staging-2"},"publish":{"requires_confirmation":false}}`
	updated := executeRequest(t, handler, http.MethodPut, "/api/v1/environments/staging", `"2"`, []byte(updateBody))
	if updated.Code != http.StatusOK || updated.Header().Get("ETag") != `"3"` {
		t.Fatalf("update status = %d, etag = %q, body = %s", updated.Code, updated.Header().Get("ETag"), updated.Body.String())
	}
	var updatedResponse struct {
		Data environmentDTO `json:"data"`
	}
	decodeResponse(t, updated, &updatedResponse)
	if updatedResponse.Data.Name != "QA Staging" || updatedResponse.Data.Kind != "staging" {
		t.Fatalf("updated environment = %#v", updatedResponse.Data)
	}

	deleted := executeRequest(t, handler, http.MethodDelete, "/api/v1/environments/staging", `"3"`, nil)
	if deleted.Code != http.StatusOK || deleted.Header().Get("ETag") != `"4"` {
		t.Fatalf("delete status = %d, body = %s", deleted.Code, deleted.Body.String())
	}
}

func TestMutationGuards(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*http.Request)
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing if-match",
			configure:  func(request *http.Request) {},
			wantStatus: http.StatusPreconditionRequired,
			wantCode:   "precondition_required",
		},
		{
			name: "invalid origin",
			configure: func(request *http.Request) {
				request.Header.Set("If-Match", `"1"`)
				request.Header.Set("Origin", "https://evil.example")
			},
			wantStatus: http.StatusForbidden,
			wantCode:   "invalid_origin",
		},
		{
			name: "invalid host",
			configure: func(request *http.Request) {
				request.Host = "attacker.example"
				request.Header.Set("If-Match", `"1"`)
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_host",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, _ := newTestHandler(t)
			request := newAPIRequest(t, http.MethodPut, "/api/v1/project", []byte(`{"id":"photo-editor","name":"Updated"}`))
			test.configure(request)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			var response errorEnvelope
			decodeResponse(t, recorder, &response)
			if response.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", response.Error.Code, test.wantCode)
			}
		})
	}
}

func TestUnknownJSONFieldIsRejected(t *testing.T) {
	handler, _ := newTestHandler(t)
	recorder := executeRequest(t, handler, http.MethodPut, "/api/v1/project", `"1"`, []byte(`{"id":"photo-editor","name":"Updated","unknown":true}`))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response errorEnvelope
	decodeResponse(t, recorder, &response)
	if response.Error.Code != "malformed_json" {
		t.Fatalf("code = %q", response.Error.Code)
	}
}

func TestDuplicateEnvironmentReturnsConflict(t *testing.T) {
	handler, _ := newTestHandler(t)
	body := `{"id":"development","name":"Development copy","kind":"development","provider":{"type":"firebase-remote-config","project_id":"duplicate"},"publish":{"requires_confirmation":false}}`
	recorder := executeRequest(t, handler, http.MethodPost, "/api/v1/environments", `"1"`, []byte(body))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response errorEnvelope
	decodeResponse(t, recorder, &response)
	if response.Error.Code != "environment_already_exists" {
		t.Fatalf("code = %q", response.Error.Code)
	}
}

func TestInvalidProjectReturnsValidationError(t *testing.T) {
	handler, _ := newTestHandler(t)
	recorder := executeRequest(t, handler, http.MethodPut, "/api/v1/project", `"1"`, []byte(`{"id":"INVALID","name":"Updated"}`))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response errorEnvelope
	decodeResponse(t, recorder, &response)
	if response.Error.Code != "validation_failed" {
		t.Fatalf("code = %q", response.Error.Code)
	}
}

func TestEnvironmentRequiresExplicitPublishConfirmation(t *testing.T) {
	handler, _ := newTestHandler(t)
	body := `{"id":"staging","name":"Staging","kind":"staging","provider":{"type":"firebase-remote-config","project_id":"photo-editor-staging"},"publish":{}}`
	recorder := executeRequest(t, handler, http.MethodPost, "/api/v1/environments", `"1"`, []byte(body))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response errorEnvelope
	decodeResponse(t, recorder, &response)
	if response.Error.Code != "validation_failed" {
		t.Fatalf("code = %q", response.Error.Code)
	}
}

func TestMutationRequiresJSONContentType(t *testing.T) {
	handler, _ := newTestHandler(t)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/project", strings.NewReader(`{"id":"photo-editor","name":"Updated"}`))
	request.Host = "127.0.0.1:9010"
	request.Header.Set("If-Match", `"1"`)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestHealthReflectsProjectIDUpdate(t *testing.T) {
	handler, _ := newTestHandler(t)
	updated := executeRequest(t, handler, http.MethodPut, "/api/v1/project", `"1"`, []byte(`{"id":"renamed-project","name":"Renamed"}`))
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updated.Code, updated.Body.String())
	}
	health := executeRequest(t, handler, http.MethodGet, "/api/v1/health", "", nil)
	var response healthResponse
	decodeResponse(t, health, &response)
	if response.ProjectID != "renamed-project" {
		t.Fatalf("project ID = %q", response.ProjectID)
	}
}

func TestHealthReportsUnavailableProject(t *testing.T) {
	handler, workspace := newTestHandler(t)
	if err := os.WriteFile(project.ManifestPath(workspace), []byte("version: 1\nproject: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recorder := executeRequest(t, handler, http.MethodGet, "/api/v1/health", "", nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response errorEnvelope
	decodeResponse(t, recorder, &response)
	if response.Error.Code != "project_unavailable" {
		t.Fatalf("code = %q", response.Error.Code)
	}
}

func TestPackEndpointsReturnDeclarativeMetadataAndSchema(t *testing.T) {
	handler, _ := newTestHandlerWithPacks(t, packs.MustNewRegistry(testPackDefinition()))

	listed := executeRequest(t, handler, http.MethodGet, "/api/v1/packs", "", nil)
	if listed.Code != http.StatusOK || listed.Header().Get("Cache-Control") != "no-store" || listed.Header().Get("ETag") != `"1"` {
		t.Fatalf("list status = %d, headers = %#v, body = %s", listed.Code, listed.Header(), listed.Body.String())
	}
	var listResponse struct {
		Data []packSummaryDTO `json:"data"`
		Meta responseMeta     `json:"meta"`
	}
	decodeResponse(t, listed, &listResponse)
	if len(listResponse.Data) != 1 || listResponse.Data[0].Ref != "test-pack/v1" || listResponse.Data[0].Name != "test-pack" || listResponse.Meta.RequestID == "" {
		t.Fatalf("list response = %#v", listResponse)
	}

	metadata := executeRequest(t, handler, http.MethodGet, "/api/v1/packs/test-pack/versions/v1", "", nil)
	if metadata.Code != http.StatusOK || metadata.Header().Get("ETag") != `"1"` {
		t.Fatalf("metadata status = %d, body = %s", metadata.Code, metadata.Body.String())
	}
	var metadataResponse struct {
		Data packMetadataDTO `json:"data"`
		Meta responseMeta    `json:"meta"`
	}
	decodeResponse(t, metadata, &metadataResponse)
	if metadataResponse.Meta.Revision != 1 || metadataResponse.Data.Ref != "test-pack/v1" || metadataResponse.Data.SchemaVersion != 1 || metadataResponse.Data.EntityTypes[0].DeletionPolicy != "restrict" || metadataResponse.Data.EntityTypes[0].EnvironmentOverrideFields[0] != "enabled" {
		t.Fatalf("metadata = %#v", metadataResponse.Data)
	}

	schema := executeRequest(t, handler, http.MethodGet, "/api/v1/packs/test-pack/versions/v1/schema", "", nil)
	if schema.Code != http.StatusOK || schema.Header().Get("ETag") != `"1"` {
		t.Fatalf("schema status = %d, body = %s", schema.Code, schema.Body.String())
	}
	var schemaResponse struct {
		Data packSchemaDTO `json:"data"`
		Meta responseMeta  `json:"meta"`
	}
	decodeResponse(t, schema, &schemaResponse)
	field := schemaResponse.Data.Entities[0].Fields[0]
	if schemaResponse.Meta.Revision != 1 || string(field.Default) != "false" || field.UI.Control != "switch" || field.Sensitivity != "public" {
		t.Fatalf("field = %#v", field)
	}
}

func TestPackEndpointsMapLookupAndSchemaErrors(t *testing.T) {
	handler, _ := newTestHandlerWithPacks(t, packs.MustNewRegistry(testPackDefinition()))
	tests := []struct {
		path       string
		wantStatus int
		wantCode   string
	}{
		{path: "/api/v1/packs/missing-pack/versions/v1", wantStatus: http.StatusNotFound, wantCode: "pack_not_found"},
		{path: "/api/v1/packs/test-pack/versions/v2", wantStatus: http.StatusNotFound, wantCode: "pack_version_not_found"},
		{path: "/api/v1/packs/test-pack/versions/v1/schema?schema_version=2", wantStatus: http.StatusUnprocessableEntity, wantCode: "schema_incompatible"},
		{path: "/api/v1/packs/test-pack/versions/v1/schema?schema_version=invalid", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{path: "/api/v1/packs/test-pack/versions/v1/schema?schema_version=", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{path: "/api/v1/packs/test-pack/versions/v1/schema?schema_version=1&schema_version=1", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.wantCode, func(t *testing.T) {
			recorder := executeRequest(t, handler, http.MethodGet, test.path, "", nil)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			var response errorEnvelope
			decodeResponse(t, recorder, &response)
			if response.Error.Code != test.wantCode || response.Error.RequestID == "" {
				t.Fatalf("error = %#v", response.Error)
			}
		})
	}
}

func TestExternalManifestChangeInvalidatesRevision(t *testing.T) {
	handler, workspace := newTestHandler(t)
	path := project.ManifestPath(workspace)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "Photo Editor", "Changed outside Conflow", 1))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	recorder := executeRequest(t, handler, http.MethodPut, "/api/v1/project", `"1"`, []byte(`{"id":"photo-editor","name":"Stale update"}`))
	if recorder.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func newTestHandler(t *testing.T) (http.Handler, string) {
	return newTestHandlerWithPacks(t, packs.BuiltinRegistry())
}

func newTestHandlerWithPacks(t *testing.T, registry *packs.Registry) (http.Handler, string) {
	t.Helper()
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	service, err := app.OpenWithPacks(workspace, registry)
	if err != nil {
		t.Fatal(err)
	}
	return New(service), workspace
}

func testPackDefinition() packs.Definition {
	return packs.Definition{
		Metadata: packs.Metadata{
			Name:         "test-pack",
			Version:      "v1",
			Description:  "A declarative HTTP test Pack.",
			Capabilities: []string{"environment_overrides"},
			EntityTypes: []packs.EntityMetadata{{
				Name:                      "setting",
				Label:                     "Setting",
				Description:               "A test setting.",
				IDRule:                    packs.IDRule{Pattern: "^[a-z][a-z0-9-]{0,62}$", MinLength: 1, MaxLength: 63},
				DeletionPolicy:            packs.DeletionPolicyRestrict,
				EnvironmentOverrideFields: []string{"enabled"},
			}},
		},
		Schema: packs.Schema{
			Version: 1,
			Entities: []packs.EntitySchema{{
				Name: "setting",
				Fields: []packs.FieldSchema{{
					Name:        "enabled",
					Type:        packs.FieldTypeBoolean,
					Required:    true,
					Default:     json.RawMessage("false"),
					Sensitivity: packs.SensitivityPublic,
					UI:          packs.FieldUI{Label: "Enabled", Description: "Whether the setting is enabled.", Control: "switch", Group: "General", Order: 0},
					Validation:  packs.FieldValidation{Enum: []json.RawMessage{}},
				}},
			}},
			Migrations: []packs.SchemaMigration{},
		},
	}
}

func executeRequest(t *testing.T, handler http.Handler, method, path, ifMatch string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := newAPIRequest(t, method, path, body)
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func newAPIRequest(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Host = "127.0.0.1:9010"
	if method == http.MethodPost || method == http.MethodPut {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://127.0.0.1:9010")
	}
	return request
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, recorder.Body.String())
	}
}
