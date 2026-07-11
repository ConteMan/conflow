package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateReleaseUnknownPlanReturnsNotFound(t *testing.T) {
	handler, _ := newTestHandler(t)
	body := []byte(`{"plan_id":"plan_missing","expected_draft_revision":1,"expected_remote_etag":"e","confirmation":{"acknowledged":true,"acknowledged_risk_item_ids":[]}}`)
	request := newAPIRequest(t, http.MethodPost, "/api/v1/environments/development/releases", body)
	request.Header.Set("Idempotency-Key", "k-missing-plan")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
