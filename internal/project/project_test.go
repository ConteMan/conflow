package project

import "testing"

func TestValidateAcceptsExampleManifest(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Project: Project{ID: "photo-editor", Name: "Photo Editor"},
		Pack:    PackReference{ID: "mobile-ad-monetization/v1"},
		Source:  Source{Type: "managed-file"},
		Environments: []Environment{
			{ID: "development", Name: "Development", Kind: "development", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-dev"}},
		},
	}
	if err := Validate(manifest); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsDuplicateEnvironment(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Project: Project{ID: "photo-editor", Name: "Photo Editor"},
		Pack:    PackReference{ID: "mobile-ad-monetization/v1"},
		Source:  Source{Type: "managed-file"},
		Environments: []Environment{
			{ID: "production", Name: "Production", Kind: "production", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-prod"}},
			{ID: "production", Name: "Production 2", Kind: "production", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-prod"}},
		},
	}
	if err := Validate(manifest); err == nil {
		t.Fatal("Validate() error = nil, want duplicate environment error")
	}
}

func TestValidateRequiresEnvironmentNameAndKind(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Project: Project{ID: "photo-editor", Name: "Photo Editor"},
		Pack:    PackReference{ID: "mobile-ad-monetization/v1"},
		Source:  Source{Type: "managed-file"},
		Environments: []Environment{{
			ID: "development", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-dev"},
		}},
	}
	if err := Validate(manifest); err == nil {
		t.Fatal("Validate() error = nil, want missing name and kind error")
	}
}

func TestValidateAllowsEmptyFirebaseProjectIDDuringInitialization(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Project: Project{ID: "photo-editor", Name: "Photo Editor"},
		Pack:    PackReference{ID: "mobile-ad-monetization/v1"},
		Source:  Source{Type: "managed-file"},
		Environments: []Environment{{
			ID: "development", Name: "Development", Kind: "development",
			Provider: Provider{Type: "firebase-remote-config"},
		}},
	}
	if err := Validate(manifest); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
