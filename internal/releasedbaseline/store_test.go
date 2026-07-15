package releasedbaseline

import (
	"path/filepath"
	"testing"
	"time"
)

func TestHashFieldsIsStableAcrossMapKeyOrder(t *testing.T) {
	first, err := HashFields(map[string]any{"z": float64(1), "nested": map[string]any{"b": true, "a": "value"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashFields(map[string]any{"nested": map[string]any{"a": "value", "b": true}, "z": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first[:7] != "sha256:" {
		t.Fatalf("hashes = %q, %q", first, second)
	}
}

func TestStoreSavesEnvironmentBaseline(t *testing.T) {
	store := Open(filepath.Join(t.TempDir(), "released-baseline"))
	want := Document{EnvironmentID: "development", ReleaseID: "rel_test", ReleasedAt: time.Date(2026, 7, 15, 3, 20, 0, 0, time.UTC), SourceRevision: "src_test", Entities: map[string]string{"entity:pack/v1:setting:home": "sha256:test"}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.Load("development")
	if err != nil || !found || got.EnvironmentID != want.EnvironmentID || got.ReleaseID != want.ReleaseID || got.SourceRevision != want.SourceRevision || got.Entities["entity:pack/v1:setting:home"] != "sha256:test" {
		t.Fatalf("baseline = %#v, found=%v, err=%v", got, found, err)
	}
}
