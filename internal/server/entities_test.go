package server

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestEntityHandlerCRUDReferencesAndScope(t *testing.T) {
	handler, _ := newTestHandler(t)
	sourceRevision := getDraftForTest(t, handler, "development").SourceRevision

	policy := entityBody(sourceRevision, "baseline", "frequency_policy", "inter_global_cap", map[string]any{
		"cooldown_ms": 30000, "interval_ms": 300000, "max_count": 3, "shift_count": 1, "positions": []any{"open_document"},
	})
	created := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development/entities", `"1"`, policy)
	if created.Code != http.StatusCreated || created.Header().Get("ETag") != `"2"` {
		t.Fatalf("create policy = %d %s", created.Code, created.Body.String())
	}

	placement := entityBody(sourceRevision, "baseline", "placement", "ad_interstitial_001", map[string]any{
		"key": "interstitial_open_document", "ad_type": "interstitial", "enabled": true, "network_mode": "hybrid", "frequency_policy_id": "inter_global_cap", "load_timeout_ms": 5000, "cache_policy": "memory", "fallback_behavior": "open_document",
	})
	created = executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development/entities", `"2"`, placement)
	if created.Code != http.StatusCreated || created.Header().Get("ETag") != `"3"` {
		t.Fatalf("create placement = %d %s", created.Code, created.Body.String())
	}

	list := executeRequest(t, handler, http.MethodGet, "/api/v1/drafts/development/entities?entity_type=placement", "", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	var listed struct {
		Data []struct {
			EntityID string `json:"entity_id"`
		} `json:"data"`
	}
	decodeResponse(t, list, &listed)
	if len(listed.Data) != 1 || listed.Data[0].EntityID != "ad_interstitial_001" {
		t.Fatalf("filtered list = %#v", listed.Data)
	}

	referencedBy := executeRequest(t, handler, http.MethodGet, "/api/v1/drafts/development/entities/frequency_policy/inter_global_cap/referenced-by", "", nil)
	if referencedBy.Code != http.StatusOK {
		t.Fatalf("referenced-by = %d %s", referencedBy.Code, referencedBy.Body.String())
	}
	var references struct {
		Data struct {
			ReferencedBy []struct {
				EntityRef string `json:"entity_ref"`
				Path      string `json:"path"`
			} `json:"referenced_by"`
		} `json:"data"`
	}
	decodeResponse(t, referencedBy, &references)
	if len(references.Data.ReferencedBy) != 1 || references.Data.ReferencedBy[0].Path != "/frequency_policy_id" {
		t.Fatalf("references = %#v", references.Data.ReferencedBy)
	}

	deleteBody := []byte(`{"expected_source_revision":"` + sourceRevision + `","write_scope":"baseline"}`)
	deleteRequest := newAPIRequest(t, http.MethodDelete, "/api/v1/drafts/development/entities/frequency_policy/inter_global_cap", deleteBody)
	deleteRequest.Header.Set("Content-Type", "application/json")
	deleteRequest.Header.Set("If-Match", `"3"`)
	blocked := executeDraftRequest(handler, deleteRequest)
	if blocked.Code != http.StatusConflict || blocked.Header().Get("ETag") != `"3"` {
		t.Fatalf("restricted delete = %d %s", blocked.Code, blocked.Body.String())
	}
	var conflict entityReferencedEnvelope
	decodeResponse(t, blocked, &conflict)
	if conflict.Error.Code != "entity_referenced" || conflict.Error.CurrentRevision != 3 || len(conflict.Error.References) != 1 || conflict.Error.References[0].EntityRef != "entity:mobile-ad-monetization/v1:placement:ad_interstitial_001" {
		t.Fatalf("conflict = %#v", conflict)
	}

	binding := entityBody(sourceRevision, "environment_override", "unit_binding", "ub_development_ios_ad_interstitial_001", map[string]any{
		"placement_id": "ad_interstitial_001", "environment_id": "development", "platform": "ios", "unit_id_ref": "ios_dev_008", "status": "configured",
	})
	created = executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development/entities", `"3"`, binding)
	if created.Code != http.StatusCreated || created.Header().Get("ETag") != `"4"` {
		t.Fatalf("create binding override = %d %s", created.Code, created.Body.String())
	}

	forbidden := entityBody(sourceRevision, "environment_override", "placement", "another_placement", map[string]any{
		"key": "another", "ad_type": "native", "enabled": true, "network_mode": "hybrid", "frequency_policy_id": "inter_global_cap", "load_timeout_ms": 4000, "cache_policy": "memory", "fallback_behavior": "continue",
	})
	response := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development/entities", `"4"`, forbidden)
	assertDraftError(t, response, http.StatusBadRequest, "invalid_request")
	baselineBinding := entityBody(sourceRevision, "baseline", "unit_binding", "ub_development_android_ad_interstitial_001", map[string]any{
		"placement_id": "ad_interstitial_001", "environment_id": "development", "platform": "android", "unit_id_ref": "android_dev_008", "status": "configured",
	})
	response = executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development/entities", `"4"`, baselineBinding)
	assertDraftError(t, response, http.StatusBadRequest, "invalid_request")
}

func TestEntityHandlerRejectsInvalidAndMissingResources(t *testing.T) {
	handler, _ := newTestHandler(t)
	sourceRevision := getDraftForTest(t, handler, "development").SourceRevision
	invalidID := entityBody(sourceRevision, "baseline", "frequency_policy", "invalid id", map[string]any{
		"cooldown_ms": 0, "interval_ms": 1, "max_count": 1, "shift_count": 0, "positions": []any{},
	})
	response := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development/entities", `"1"`, invalidID)
	assertDraftError(t, response, http.StatusBadRequest, "invalid_request")
	stale := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development/entities", `"2"`, invalidID)
	assertDraftError(t, stale, http.StatusPreconditionFailed, "revision_mismatch")

	missing := executeRequest(t, handler, http.MethodGet, "/api/v1/drafts/development/entities/frequency_policy/missing", "", nil)
	assertDraftError(t, missing, http.StatusNotFound, "entity_not_found")
	unknown := executeRequest(t, handler, http.MethodGet, "/api/v1/drafts/development/entities/unknown/missing", "", nil)
	assertDraftError(t, unknown, http.StatusBadRequest, "invalid_request")
	deleteWithoutJSON := newAPIRequest(t, http.MethodDelete, "/api/v1/drafts/development/entities/frequency_policy/missing", []byte(`{}`))
	deleteWithoutJSON.Header.Set("If-Match", `"1"`)
	recorder := executeDraftRequest(handler, deleteWithoutJSON)
	assertDraftError(t, recorder, http.StatusUnsupportedMediaType, "unsupported_media_type")
}

func entityBody(sourceRevision, scope, entityType, id string, fields map[string]any) []byte {
	body, _ := json.Marshal(map[string]any{
		"expected_source_revision": sourceRevision,
		"write_scope":              scope,
		"entity_type":              entityType,
		"entity": map[string]any{
			"id":     id,
			"fields": fields,
		},
	})
	return body
}
