package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

const manifestRelativePath = ".conflow/project.yaml"

var projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)

type Manifest struct {
	Version      int           `yaml:"version"`
	Project      Project       `yaml:"project"`
	Pack         PackReference `yaml:"pack"`
	Source       Source        `yaml:"source"`
	Environments []Environment `yaml:"environments"`
}

type Project struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type PackReference struct {
	ID string `yaml:"id"`
}

type Source struct {
	Type string `yaml:"type"`
}

type Environment struct {
	ID       string   `yaml:"id"`
	Provider Provider `yaml:"provider"`
	Publish  Publish  `yaml:"publish"`
}

type Provider struct {
	Type      string `yaml:"type"`
	ProjectID string `yaml:"project_id"`
}

type Publish struct {
	RequiresConfirmation bool `yaml:"requires_confirmation"`
}

func ManifestPath(workspace string) string {
	return filepath.Join(workspace, manifestRelativePath)
}

func Load(workspace string) (Manifest, error) {
	path := ManifestPath(workspace)
	content, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read project manifest %s: %w", path, err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse project manifest %s: %w", path, err)
	}
	return manifest, nil
}

func Validate(manifest Manifest) error {
	var validationErrors []error
	if manifest.Version != 1 {
		validationErrors = append(validationErrors, errors.New("version must be 1"))
	}
	if !projectIDPattern.MatchString(manifest.Project.ID) {
		validationErrors = append(validationErrors, errors.New("project.id must be kebab-case and 2-63 characters"))
	}
	if strings.TrimSpace(manifest.Project.Name) == "" {
		validationErrors = append(validationErrors, errors.New("project.name is required"))
	}
	if manifest.Pack.ID == "" {
		validationErrors = append(validationErrors, errors.New("pack.id is required"))
	}
	if manifest.Source.Type != "managed-file" && manifest.Source.Type != "git-json" {
		validationErrors = append(validationErrors, errors.New("source.type must be managed-file or git-json"))
	}
	if len(manifest.Environments) == 0 {
		validationErrors = append(validationErrors, errors.New("at least one environment is required"))
	}

	environmentIDs := map[string]struct{}{}
	for _, environment := range manifest.Environments {
		if !projectIDPattern.MatchString(environment.ID) {
			validationErrors = append(validationErrors, fmt.Errorf("environment.id %q is invalid", environment.ID))
		}
		if _, exists := environmentIDs[environment.ID]; exists {
			validationErrors = append(validationErrors, fmt.Errorf("environment.id %q is duplicated", environment.ID))
		}
		environmentIDs[environment.ID] = struct{}{}
		if environment.Provider.Type != "firebase-remote-config" {
			validationErrors = append(validationErrors, fmt.Errorf("environment %q provider.type must be firebase-remote-config", environment.ID))
		}
		if strings.TrimSpace(environment.Provider.ProjectID) == "" {
			validationErrors = append(validationErrors, fmt.Errorf("environment %q provider.project_id is required", environment.ID))
		}
	}
	return errors.Join(validationErrors...)
}

func CreateExample(workspace string) (string, error) {
	path := ManifestPath(workspace)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("project manifest already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	manifest := Manifest{
		Version: 1,
		Project: Project{ID: "photo-editor", Name: "Photo Editor"},
		Pack:    PackReference{ID: "mobile-ad-monetization/v1"},
		Source:  Source{Type: "managed-file"},
		Environments: []Environment{
			{ID: "development", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-dev"}},
			{ID: "production", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-prod"}, Publish: Publish{RequiresConfirmation: true}},
		},
	}
	content, err := yaml.Marshal(manifest)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
