package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/source"
	"github.com/ConteMan/conflow/internal/validation"
)

var (
	ErrLastEnvironment         = errors.New("cannot delete the last environment")
	ErrPackRegistryUnavailable = errors.New("pack registry is unavailable")
)

type Service struct {
	projects     *project.Store
	packRegistry *packs.Registry
	drafts       *draft.Store
	source       source.Adapter
	validations  *validation.Store
}

func Initialize(workspace string) (string, error) {
	return project.CreateExample(workspace)
}

func Open(workspace string) (*Service, error) {
	return OpenWithPacks(workspace, packs.BuiltinRegistry())
}

func OpenWithPacks(workspace string, registry *packs.Registry) (*Service, error) {
	if registry == nil {
		return nil, ErrPackRegistryUnavailable
	}
	store, err := project.Open(workspace)
	if err != nil {
		return nil, err
	}
	manifest, err := store.Snapshot()
	if err != nil {
		return nil, err
	}
	if manifest.Manifest.Source.Type != "managed-file" {
		return nil, fmt.Errorf("unsupported source adapter %q", manifest.Manifest.Source.Type)
	}
	managed := source.OpenManagedFile(workspace)
	draftStore, err := draft.Open(filepath.Join(workspace, ".conflow", "draft.json"), func() (draft.SourceSnapshot, error) {
		snapshot, snapshotErr := managed.Load()
		if snapshotErr != nil {
			return draft.SourceSnapshot{}, snapshotErr
		}
		return draftSourceSnapshot(snapshot), nil
	})
	if err != nil {
		return nil, err
	}
	validationStore, err := validation.Open(filepath.Join(workspace, ".conflow", "validation-results.json"))
	if err != nil {
		return nil, err
	}
	return &Service{projects: store, packRegistry: registry, drafts: draftStore, source: managed, validations: validationStore}, nil
}

func (s *Service) Snapshot(_ context.Context) (project.Snapshot, error) {
	return s.projects.Snapshot()
}

func (s *Service) UpdateProject(_ context.Context, expectedRevision uint64, metadata project.Project) (project.Snapshot, error) {
	return s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		manifest.Project = metadata
		return nil
	})
}

func (s *Service) CreateEnvironment(_ context.Context, expectedRevision uint64, environment project.Environment) (project.Snapshot, project.Environment, error) {
	snapshot, err := s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		if slices.ContainsFunc(manifest.Environments, func(current project.Environment) bool {
			return current.ID == environment.ID
		}) {
			return project.ErrAlreadyExists
		}
		manifest.Environments = append(manifest.Environments, environment)
		return nil
	})
	return snapshot, environment, err
}

func (s *Service) GetEnvironment(ctx context.Context, environmentID string) (project.Snapshot, project.Environment, error) {
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return project.Snapshot{}, project.Environment{}, err
	}
	for _, environment := range snapshot.Manifest.Environments {
		if environment.ID == environmentID {
			return snapshot, environment, nil
		}
	}
	return snapshot, project.Environment{}, project.ErrNotFound
}

func (s *Service) UpdateEnvironment(_ context.Context, expectedRevision uint64, environmentID string, replacement project.Environment) (project.Snapshot, project.Environment, error) {
	snapshot, err := s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		for index := range manifest.Environments {
			if manifest.Environments[index].ID == environmentID {
				replacement.ID = environmentID
				replacement.Kind = manifest.Environments[index].Kind
				manifest.Environments[index] = replacement
				return nil
			}
		}
		return project.ErrNotFound
	})
	if err != nil {
		return snapshot, project.Environment{}, err
	}
	return snapshot, replacement, nil
}

func (s *Service) DeleteEnvironment(_ context.Context, expectedRevision uint64, environmentID string) (project.Snapshot, error) {
	return s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		if len(manifest.Environments) == 1 && manifest.Environments[0].ID == environmentID {
			return ErrLastEnvironment
		}
		for index := range manifest.Environments {
			if manifest.Environments[index].ID == environmentID {
				manifest.Environments = slices.Delete(manifest.Environments, index, index+1)
				return nil
			}
		}
		return project.ErrNotFound
	})
}

func (s *Service) ListPacks(_ context.Context) packs.Snapshot {
	return s.packRegistry.List()
}

func (s *Service) GetPack(_ context.Context, name, version string) (packs.Definition, uint64, error) {
	return s.packRegistry.Get(name, version)
}

func (s *Service) GetPackSchema(_ context.Context, name, version string, requestedVersion *uint64) (packs.Schema, uint64, error) {
	return s.packRegistry.Schema(name, version, requestedVersion)
}

func (s *Service) GetDraft(_ context.Context, environmentID string) (draft.View, uint64, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return draft.View{}, 0, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return draft.View{}, 0, err
	}
	return s.drafts.View(schema, environments, environmentID)
}

func (s *Service) MutateDraft(_ context.Context, environmentID string, mutation draft.Mutation) (draft.View, uint64, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return draft.View{}, 0, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return draft.View{}, 0, err
	}
	return s.drafts.Mutate(schema, environments, environmentID, mutation)
}

type SourceInfo struct {
	Type         string
	Capabilities source.Capabilities
	Status       source.Status
}

func (s *Service) SourceInfo(_ context.Context) (SourceInfo, error) {
	status, err := s.source.Status()
	if err != nil {
		return SourceInfo{}, err
	}
	return SourceInfo{Type: status.Type, Capabilities: s.source.Capabilities(), Status: status}, nil
}

// SaveDraft writes the resolved source replacement for the selected view and
// then consumes that view's draft replacements. The draft Store retains the
// existing lock/snapshot boundary from Spec 004.
func (s *Service) SaveDraft(_ context.Context, environmentID string, expectedRevision uint64, expectedSourceRevision string) (draft.View, uint64, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return draft.View{}, 0, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return draft.View{}, 0, err
	}
	return s.drafts.SaveToSource(schema, environments, environmentID, draft.Save{
		ExpectedRevision: expectedRevision, ExpectedSourceRevision: expectedSourceRevision,
		Commit: func(current draft.SourceSnapshot, state draft.State) (draft.SourceSnapshot, error) {
			baseline := current.Baseline
			if state.BaselinePresent {
				baseline = state.Baseline
			}
			override, present := current.EnvironmentOverrides[environmentID]
			if replacement, exists := state.EnvironmentOverrides[environmentID]; exists {
				override, present = replacement, true
			}
			if !present {
				override = nil
			}
			next, saveErr := s.source.Save(source.SaveInput{ExpectedRevision: current.Revision, EnvironmentID: environmentID, Baseline: baseline, EnvironmentOverride: override})
			if errors.Is(saveErr, source.ErrRevisionMismatch) {
				return draft.SourceSnapshot{}, draft.ErrSourceRevisionChanged
			}
			if saveErr != nil {
				return draft.SourceSnapshot{}, saveErr
			}
			return draftSourceSnapshot(next), nil
		},
	})
}

func draftSourceSnapshot(snapshot source.Snapshot) draft.SourceSnapshot {
	return draft.SourceSnapshot{Revision: snapshot.Revision, Baseline: snapshot.Baseline, EnvironmentOverrides: snapshot.EnvironmentOverrides}
}

// ValidateDraft runs the complete Spec 007 validation against one captured
// DraftView and stores the resulting project-level draft revision.
func (s *Service) ValidateDraft(_ context.Context, environmentID string) (validation.Result, uint64, error) {
	view, revision, environment, err := s.validationContext(environmentID)
	if err != nil {
		return validation.Result{}, 0, err
	}
	result := validation.Result{
		EnvironmentID:          environmentID,
		ValidatedDraftRevision: revision,
		ValidatedAt:            time.Now().UTC(),
		Status:                 validation.StatusFresh,
		Diagnostics: validation.Validate(validation.Input{
			PackRef:         view.PackRef,
			EnvironmentID:   environmentID,
			EnvironmentKind: environment.Kind,
			Effective:       view.Effective,
		}),
	}
	result.Readiness = validation.ReadinessFor(result.Diagnostics)
	if err := s.validations.Save(result); err != nil {
		return validation.Result{}, 0, err
	}
	// A mutation may have completed while validation was executing. Preserve
	// the captured revision and expose the result's actual freshness.
	_, currentRevision, err := s.GetDraft(context.Background(), environmentID)
	if err != nil {
		return validation.Result{}, 0, err
	}
	result, err = s.validations.Get(environmentID, currentRevision)
	return result, currentRevision, err
}

// Diagnostics returns the latest stored complete-validation result.
func (s *Service) Diagnostics(ctx context.Context, environmentID string) (validation.Result, uint64, error) {
	_, revision, err := s.GetDraft(ctx, environmentID)
	if err != nil {
		return validation.Result{}, 0, err
	}
	result, err := s.validations.Get(environmentID, revision)
	return result, revision, err
}

func (s *Service) validationContext(environmentID string) (draft.View, uint64, project.Environment, error) {
	view, revision, err := s.GetDraft(context.Background(), environmentID)
	if err != nil {
		return draft.View{}, 0, project.Environment{}, err
	}
	_, environment, err := s.GetEnvironment(context.Background(), environmentID)
	if err != nil {
		return draft.View{}, 0, project.Environment{}, err
	}
	return view, revision, environment, nil
}

func (s *Service) draftSchema(manifest project.Manifest) (draft.Schema, []draft.Environment, error) {
	definition, _, err := s.packRegistry.Resolve(manifest.Pack.ID)
	if err != nil {
		return draft.Schema{}, nil, err
	}
	allowed := make(map[string]map[string]bool, len(definition.Metadata.EntityTypes))
	for _, entity := range definition.Metadata.EntityTypes {
		fields := make(map[string]bool, len(entity.EnvironmentOverrideFields))
		for _, field := range entity.EnvironmentOverrideFields {
			fields[field] = true
		}
		allowed[entity.Name] = fields
	}
	schema := draft.Schema{PackRef: manifest.Pack.ID, Defaults: map[string]any{}, Fields: []draft.Field{}}
	for _, entity := range definition.Schema.Entities {
		metadata := entityMetadata(definition.Metadata.EntityTypes, entity.Name)
		if metadata.Collection != "" {
			schema.Defaults[metadata.Collection] = []any{}
			schema.Fields = append(schema.Fields, draft.Field{Path: "/" + pointerToken(metadata.Collection), Type: "array", EnvironmentOverrideAllowed: len(metadata.EnvironmentOverrideFields) > 0, Default: []any{}})
			continue
		}
		entityDefaults := map[string]any{}
		for _, field := range entity.Fields {
			var defaultValue any
			if err := json.Unmarshal(field.Default, &defaultValue); err != nil {
				return draft.Schema{}, nil, err
			}
			entityDefaults[field.Name] = defaultValue
			schema.Fields = append(schema.Fields, draft.Field{Path: "/" + pointerToken(entity.Name) + "/" + pointerToken(field.Name), Type: string(field.Type), Nullable: field.Nullable, EnvironmentOverrideAllowed: allowed[entity.Name][field.Name], Required: field.Required, Default: defaultValue, Enum: rawValues(field.Validation.Enum), MinLength: field.Validation.MinLength, MaxLength: field.Validation.MaxLength, Minimum: field.Validation.Minimum, Maximum: field.Validation.Maximum})
		}
		if len(entityDefaults) > 0 {
			schema.Defaults[entity.Name] = entityDefaults
		}
	}
	environments := make([]draft.Environment, len(manifest.Environments))
	for index, environment := range manifest.Environments {
		environments[index] = draft.Environment{ID: environment.ID, Name: environment.Name, Kind: environment.Kind}
	}
	return schema, environments, nil
}

func entityMetadata(entities []packs.EntityMetadata, name string) packs.EntityMetadata {
	for _, entity := range entities {
		if entity.Name == name {
			return entity
		}
	}
	return packs.EntityMetadata{}
}

func rawValues(values []json.RawMessage) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		var decoded any
		if json.Unmarshal(value, &decoded) == nil {
			result = append(result, decoded)
		}
	}
	return result
}

func pointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}
