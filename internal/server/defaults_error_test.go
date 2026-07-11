package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadDefaultsErrorMappings(t *testing.T) {
	handler, _ := newTestHandler(t)
	bad := newAPIRequest(t, http.MethodGet, "/api/v1/environments/development/defaults?format=exe", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, bad)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("bad format status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	missing := newAPIRequest(t, http.MethodGet, "/api/v1/environments/development/defaults?format=json", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, missing)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing snapshot status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
