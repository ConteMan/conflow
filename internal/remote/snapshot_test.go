package remote

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProtectedSnapshotPersistsTemplateWithoutLeakingMetadata(t *testing.T) {
	store := OpenFileStore(t.TempDir())
	template := []byte(`{"version":{"versionNumber":"3"},"parameters":{"ad_frequency_cap":{"defaultValue":{"value":"30000"},"conditionalValues":{"country":{"value":"60000"}}}},"conditions":[{}]}`)
	snapshot, err := SnapshotFromTemplate(template, "etag-test", "3", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Summary.HasUnmodeledConditions {
		t.Fatal("conditional values were not detected")
	}
	if err := store.Save("development", snapshot); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(store.root, "development.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	loaded, err := store.Current("development")
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded.Template) != string(template) || loaded.Parameters["ad_frequency_cap"] != "30000" {
		t.Fatalf("loaded=%#v", loaded)
	}
	metadata, _ := json.Marshal(loaded)
	if strings.Contains(string(metadata), "60000") || strings.Contains(string(metadata), "parameters") {
		t.Fatalf("protected template leaked through JSON: %s", metadata)
	}
}

func TestFailedPullDoesNotReplaceSnapshot(t *testing.T) {
	store := OpenFileStore(t.TempDir())
	before, _ := SnapshotFromTemplate([]byte(`{"parameters":{}}`), "old", "1", time.Now())
	if err := store.Save("development", before); err != nil {
		t.Fatal(err)
	}
	if _, err := SnapshotFromTemplate([]byte(`not-json`), "new", "2", time.Now()); err == nil {
		t.Fatal("invalid pull accepted")
	}
	after, err := store.Current("development")
	if err != nil {
		t.Fatal(err)
	}
	if after.RemoteETag != "old" {
		t.Fatalf("cache changed after failure: %#v", after)
	}
}
