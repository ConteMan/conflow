package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestPlanHTTPReturnsOperationAndRecoversResult(t *testing.T) {
	handler, _ := newTestHandler(t)
	created := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development:plan", "", nil)
	if created.Code != http.StatusAccepted {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	var response struct {
		Data struct {
			OperationID string `json:"operation_id"`
		} `json:"data"`
	}
	decodeResponse(t, created, &response)
	var operation struct {
		Data struct {
			Status string `json:"status"`
			Result *struct {
				ResourceID string `json:"resource_id"`
			} `json:"result"`
		} `json:"data"`
	}
	deadline := time.Now().Add(time.Second)
	for {
		result := executeRequest(t, handler, http.MethodGet, "/api/v1/operations/"+response.Data.OperationID, "", nil)
		if result.Code != http.StatusOK {
			t.Fatalf("operation status = %d", result.Code)
		}
		decodeResponse(t, result, &operation)
		if operation.Data.Status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("operation did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if operation.Data.Result == nil {
		t.Fatal("operation result missing")
	}
	planResponse := executeRequest(t, handler, http.MethodGet, "/api/v1/plans/"+operation.Data.Result.ResourceID, "", nil)
	if planResponse.Code != http.StatusOK {
		t.Fatalf("plan status = %d: %s", planResponse.Code, planResponse.Body.String())
	}
	var p struct {
		Data struct {
			Status           string `json:"status"`
			ArtifactMetadata []struct {
				ArtifactName string `json:"artifact_name"`
			} `json:"artifact_metadata"`
		} `json:"data"`
	}
	if err := json.Unmarshal(planResponse.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Data.Status != "preview_only" {
		t.Fatalf("status = %q", p.Data.Status)
	}
	artifact := executeRequest(t, handler, http.MethodGet, "/api/v1/plans/"+operation.Data.Result.ResourceID+"/artifacts/review.json", "", nil)
	if artifact.Code != http.StatusOK {
		t.Fatalf("artifact status = %d", artifact.Code)
	}
	missing := executeRequest(t, handler, http.MethodGet, "/api/v1/plans/missing", "", nil)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d", missing.Code)
	}
}

func TestInvalidatedPlanArtifactUsesConflictDomain(t *testing.T) {
	handler, _ := newTestHandler(t)
	created := executeRequest(t, handler, http.MethodPost, "/api/v1/drafts/development:plan", "", nil)
	var started struct {
		Data struct {
			OperationID string `json:"operation_id"`
		} `json:"data"`
	}
	decodeResponse(t, created, &started)
	var op struct {
		Data struct {
			Status string `json:"status"`
			Result *struct {
				ResourceID string `json:"resource_id"`
			} `json:"result"`
		} `json:"data"`
	}
	deadline := time.Now().Add(time.Second)
	for {
		r := executeRequest(t, handler, http.MethodGet, "/api/v1/operations/"+started.Data.OperationID, "", nil)
		decodeResponse(t, r, &op)
		if op.Data.Status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("operation timeout")
		}
		time.Sleep(time.Millisecond)
	}
	draft := getDraftForTest(t, handler, "development")
	body := []byte(`{"expected_source_revision":"` + draft.SourceRevision + `","write_scope":"baseline","configuration":{}}`)
	updated := executeRequest(t, handler, http.MethodPut, "/api/v1/drafts/development", `"`+"1"+`"`, body)
	if updated.Code != http.StatusOK {
		t.Fatalf("mutate = %d %s", updated.Code, updated.Body.String())
	}
	artifact := executeRequest(t, handler, http.MethodGet, "/api/v1/plans/"+op.Data.Result.ResourceID+"/artifacts/review.json", "", nil)
	if artifact.Code != http.StatusConflict {
		t.Fatalf("artifact conflict = %d", artifact.Code)
	}
}
