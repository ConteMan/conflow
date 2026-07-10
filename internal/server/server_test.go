package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ConteMan/conflow/internal/project"
)

func TestHealthEndpoint(t *testing.T) {
	handler := New(project.Manifest{Project: project.Project{ID: "photo-editor"}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
}
