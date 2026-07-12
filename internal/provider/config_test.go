package provider

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestValidateFirebaseServiceAccountClassifiesLocalInputWithoutEchoingPrivateKey(t *testing.T) {
	directory := t.TempDir()
	validPath := filepath.Join(directory, "service-account.json")
	if err := os.WriteFile(validPath, []byte(`{"type":"service_account","client_email":"bot@example.invalid","private_key":"PRIVATE KEY MUST NOT ESCAPE","project_id":"test-project"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFirebaseServiceAccount(validPath); err != nil {
		t.Fatalf("valid credential: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		content string
		want    error
	}{
		{name: "missing", path: filepath.Join(directory, "nope.json"), want: ErrCredentialFileMissing},
		{name: "invalid JSON", path: filepath.Join(directory, "bad.json"), content: `{`, want: ErrCredentialJSONInvalid},
		{name: "wrong type", path: filepath.Join(directory, "wrong-type.json"), content: `{"type":"authorized_user"}`, want: ErrCredentialServiceAccount},
		{name: "missing fields", path: filepath.Join(directory, "partial.json"), content: `{"type":"service_account"}`, want: ErrCredentialFieldsMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.content != "" {
				if err := os.WriteFile(test.path, []byte(test.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err := ValidateFirebaseServiceAccount(test.path)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if err != nil && (strings.Contains(err.Error(), "PRIVATE KEY") || strings.Contains(err.Error(), "private_key\":\"")) {
				t.Fatalf("credential content leaked: %v", err)
			}
		})
	}
}
