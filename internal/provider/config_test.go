package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveCredentialReferenceKeepsOnlyLocalPathReference(t *testing.T) {
	workspace := t.TempDir()
	if err := SaveCredentialReference(workspace, "development", "/private/keys/firebase.json"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, ".conflow", "provider", "development.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %#o, want 0600", info.Mode().Perm())
	}
	stored, err := LoadCredentialReference(workspace, "development")
	if err != nil || stored != "/private/keys/firebase.json" {
		t.Fatalf("stored = %q, err = %v", stored, err)
	}
	if got := CredentialPathDisplay(stored); got != "…/firebase.json" {
		t.Fatalf("display = %q", got)
	}
}
