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
	Version      int           `yaml:"version" json:"version"`
	Project      Project       `yaml:"project" json:"project"`
	Pack         PackReference `yaml:"pack" json:"pack"`
	Source       Source        `yaml:"source" json:"source"`
	Environments []Environment `yaml:"environments" json:"environments"`
}

type Project struct {
	ID                        string                    `yaml:"id" json:"id"`
	Name                      string                    `yaml:"name" json:"name"`
	ReleaseConfirmationPolicy ReleaseConfirmationPolicy `yaml:"release_confirmation_policy" json:"release_confirmation_policy"`
}

// ReleaseConfirmationPolicy is project-level by ADR-006. Empty values from
// older manifests resolve to environment_id during plan construction.
type ReleaseConfirmationPolicy struct {
	ProductionLowRiskMode string `yaml:"production_low_risk_mode" json:"production_low_risk_mode"`
}

type PackReference struct {
	ID string `yaml:"id" json:"id"`
}

type Source struct {
	Type    string `yaml:"type" json:"type"`
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
}

type Environment struct {
	ID       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Kind     string   `yaml:"kind" json:"kind"`
	Provider Provider `yaml:"provider" json:"provider"`
	Publish  Publish  `yaml:"publish" json:"publish"`
}

type Provider struct {
	Type      string `yaml:"type" json:"type"`
	ProjectID string `yaml:"project_id" json:"project_id"`
}

type Publish struct {
	RequiresConfirmation bool `yaml:"requires_confirmation" json:"requires_confirmation"`
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
	normalize(&manifest)
	return manifest, nil
}

func normalize(manifest *Manifest) {
	if manifest.Project.ReleaseConfirmationPolicy.ProductionLowRiskMode == "" {
		manifest.Project.ReleaseConfirmationPolicy.ProductionLowRiskMode = "environment_id"
	}
}

func Validate(manifest Manifest) error {
	var validationErrors []error
	if manifest.Version != 1 {
		validationErrors = append(validationErrors, errors.New("version must be 1"))
	}
	if !projectIDPattern.MatchString(manifest.Project.ID) {
		validationErrors = append(validationErrors, errors.New("project.id must be kebab-case and 2-63 characters"))
	}
	projectName := strings.TrimSpace(manifest.Project.Name)
	if projectName == "" {
		validationErrors = append(validationErrors, errors.New("project.name is required"))
	} else if len([]rune(projectName)) > 120 {
		validationErrors = append(validationErrors, errors.New("project.name must be at most 120 characters"))
	}
	if mode := manifest.Project.ReleaseConfirmationPolicy.ProductionLowRiskMode; mode != "" && mode != "environment_id" && mode != "acknowledgement" {
		validationErrors = append(validationErrors, errors.New("project.release_confirmation_policy.production_low_risk_mode must be environment_id or acknowledgement"))
	}
	if manifest.Pack.ID == "" {
		validationErrors = append(validationErrors, errors.New("pack.id is required"))
	}
	if manifest.Source.Type != "managed-file" && manifest.Source.Type != "git-json" {
		validationErrors = append(validationErrors, errors.New("source.type must be managed-file or git-json"))
	}
	if manifest.Source.Type == "git-json" && strings.TrimSpace(manifest.Source.Profile) == "" {
		validationErrors = append(validationErrors, errors.New("source.profile is required for git-json"))
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
		environmentName := strings.TrimSpace(environment.Name)
		if environmentName == "" {
			validationErrors = append(validationErrors, fmt.Errorf("environment %q name is required", environment.ID))
		} else if len([]rune(environmentName)) > 120 {
			validationErrors = append(validationErrors, fmt.Errorf("environment %q name must be at most 120 characters", environment.ID))
		}
		switch environment.Kind {
		case "development", "staging", "production", "custom":
		default:
			validationErrors = append(validationErrors, fmt.Errorf("environment %q kind must be development, staging, production, or custom", environment.ID))
		}
		if environment.Provider.Type != "firebase-remote-config" {
			validationErrors = append(validationErrors, fmt.Errorf("environment %q provider.type must be firebase-remote-config", environment.ID))
		}
		providerProjectID := strings.TrimSpace(environment.Provider.ProjectID)
		if providerProjectID != "" && len([]rune(providerProjectID)) > 128 {
			validationErrors = append(validationErrors, fmt.Errorf("environment %q provider.project_id must be at most 128 characters", environment.ID))
		}
	}
	return errors.Join(validationErrors...)
}

// Create initializes the manifest and its managed-file source state. Provider
// project IDs may intentionally be empty during onboarding.
func Create(workspace string, manifest Manifest) (string, error) {
	path := ManifestPath(workspace)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("project manifest already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	normalize(&manifest)
	if err := Validate(manifest); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidManifest, err)
	}
	content, err := marshalInitialManifest(manifest)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}
	dataDirectory := filepath.Join(workspace, ".conflow", "data", "environments")
	if err := os.MkdirAll(dataDirectory, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(dataDirectory), "base.yaml"), []byte("{}\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func marshalInitialManifest(manifest Manifest) ([]byte, error) {
	content, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	annotated := "# Conflow 项目清单。\n# 清单格式版本。\n" + string(content)
	for _, section := range []struct{ key, comment string }{
		{"project:", "# 项目元数据：稳定 ID、显示名称与发布确认策略。"},
		{"pack:", "# 配置包：定义可管理的业务实体和规则。"},
		{"source:", "# 配置来源：保存和读取项目配置的适配器。"},
		{"environments:", "# 环境：各发布目标的 Firebase 项目与发布策略。"},
	} {
		annotated = strings.Replace(annotated, "\n"+section.key, "\n"+section.comment+"\n"+section.key, 1)
	}
	return []byte(annotated), nil
}

func CreateExample(workspace string) (string, error) {
	return Create(workspace, Manifest{
		Version: 1,
		Project: Project{ID: "photo-editor", Name: "Photo Editor", ReleaseConfirmationPolicy: ReleaseConfirmationPolicy{ProductionLowRiskMode: "environment_id"}},
		Pack:    PackReference{ID: "mobile-ad-monetization/v1"},
		Source:  Source{Type: "managed-file"},
		Environments: []Environment{
			{ID: "development", Name: "Development", Kind: "development", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-dev"}},
			{ID: "production", Name: "Production", Kind: "production", Provider: Provider{Type: "firebase-remote-config", ProjectID: "photo-editor-prod"}, Publish: Publish{RequiresConfirmation: true}},
		},
	})
}
