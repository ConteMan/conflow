package release

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreReadsLegacyRecordsAndDetectsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "releases.json")
	legacy := `{"idempotency":{},"releases":{"rel_legacy":{"release_id":"rel_legacy","environment_id":"development","kind":"publish","outcome":"succeeded","created_at":"2026-07-11T10:00:00Z","completed_at":"2026-07-11T10:00:01Z","operation_id":"op_legacy","remote_state":"changed","semantic_summary":"one","risk_summary":"low","remote_before":{"remote_etag":"before","version":"1","observed_at":"2026-07-11T10:00:00Z","summary":{"parameter_count":0,"managed_parameter_count":0,"condition_count":0,"content_digest":"sha256:before"}},"remote_after":{"remote_etag":"after","version":"2","observed_at":"2026-07-11T10:00:01Z","summary":{"parameter_count":0,"managed_parameter_count":0,"condition_count":0,"content_digest":"sha256:after"}}}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	items, err := store.List("development", 0, "")
	if err != nil || len(items) != 1 || items[0].RemoteAfter == nil {
		t.Fatalf("legacy=%#v err=%v", items, err)
	}
	if err := os.WriteFile(path, []byte(`{"releases":`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.List("development", 0, "")
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("corruption error=%v", err)
	}
}

func TestStorePaginatesNewestFirst(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "releases.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"rel_old", "rel_new"} {
		now := time.Date(2026, 7, 11, 10, i, 0, 0, time.UTC)
		after := AuditState{RemoteETag: id, Version: "1", ObservedAt: now}
		if err := store.Save(Release{ReleaseID: id, EnvironmentID: "development", Kind: "publish", Outcome: "succeeded", CreatedAt: now, CompletedAt: now, OperationID: "op_" + id, RemoteState: "changed", SemanticSummary: "", RiskSummary: "low", RemoteAfter: &after}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.List("development", 1, "")
	if err != nil || len(items) != 1 || items[0].ReleaseID != "rel_new" {
		t.Fatalf("page=%#v err=%v", items, err)
	}
	items, err = store.List("development", 1, "rel_new")
	if err != nil || len(items) != 1 || items[0].ReleaseID != "rel_old" {
		t.Fatalf("cursor=%#v err=%v", items, err)
	}
}
