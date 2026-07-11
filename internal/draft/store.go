package draft

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type SourceProvider func() (SourceSnapshot, error)

type Store struct {
	mu     sync.Mutex
	path   string
	source SourceProvider
	state  State
}

// Open uses .conflow/draft.json only until Spec 005 defines the managed-file
// format. SourceProvider deliberately remains separate: draft replacements are
// not a source adapter and must not become the source of truth.
func Open(path string, source SourceProvider) (*Store, error) {
	if source == nil {
		return nil, errors.New("draft source provider is required")
	}
	store := &Store{path: path, source: source, state: State{Revision: 1, EnvironmentOverrides: map[string]map[string]any{}}}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read draft store: %w", err)
	}
	var persisted persistedState
	if err := json.Unmarshal(content, &persisted); err != nil {
		return nil, fmt.Errorf("parse draft store: %w", err)
	}
	if persisted.Revision == 0 {
		return nil, errors.New("draft store revision must be positive")
	}
	store.state = State{Revision: persisted.Revision, EnvironmentOverrides: persisted.EnvironmentOverrides}
	if store.state.EnvironmentOverrides == nil {
		store.state.EnvironmentOverrides = map[string]map[string]any{}
	}
	if persisted.Baseline != nil {
		store.state.BaselinePresent = true
		store.state.Baseline = *persisted.Baseline
	}
	return store, nil
}

func NewMemory(source SourceSnapshot, state State) *Store {
	if state.Revision == 0 {
		state.Revision = 1
	}
	if state.EnvironmentOverrides == nil {
		state.EnvironmentOverrides = map[string]map[string]any{}
	}
	return &Store{source: func() (SourceSnapshot, error) { return cloneSource(source), nil }, state: cloneState(state)}
}

func (s *Store) View(schema Schema, environments []Environment, environmentID string) (View, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.viewLocked(schema, environments, environmentID)
}

type Mutation struct {
	ExpectedRevision       uint64
	ExpectedSourceRevision string
	Scope                  string
	Action                 string
	Configuration          json.RawMessage
	// Prepare runs under the same lock and source snapshot as precondition
	// checks. It lets higher-level resources turn a focused operation into the
	// complete replacement required by the draft model.
	Prepare func(View) (json.RawMessage, error)
	// Validate runs after structural validation and before the replacement is
	// committed. Domain services use it for rules outside the Pack structure.
	Validate func(map[string]any) error
}

type ConflictError struct {
	Code                  string
	CurrentRevision       uint64
	CurrentSourceRevision string
	ConflictScope         string
	CurrentState          View
}

func (e *ConflictError) Error() string { return e.Code }

type ValidationError struct{ Details []StructuralError }

func (e *ValidationError) Error() string { return "draft replacement validation failed" }

func (s *Store) Mutate(schema Schema, environments []Environment, environmentID string, mutation Mutation) (View, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ValidScope(mutation.Scope) {
		return View{}, 0, ErrInvalidScope
	}
	// The comparison, conflict state, and post-commit state intentionally use
	// one source snapshot. Spec 005 will replace this provider with a managed
	// file transaction, but must preserve this atomic observation boundary.
	source, err := s.source()
	if err != nil {
		return View{}, 0, err
	}
	view, err := BuildView(schema, environments, source, cloneState(s.state), environmentID)
	revision := s.state.Revision
	if err != nil {
		return View{}, 0, err
	}
	if mutation.ExpectedRevision != revision {
		return View{}, 0, &ConflictError{Code: "revision_mismatch", CurrentRevision: revision, CurrentSourceRevision: view.SourceRevision, ConflictScope: mutation.Scope, CurrentState: view}
	}
	if mutation.ExpectedSourceRevision != view.SourceRevision {
		return View{}, 0, &ConflictError{Code: "source_revision_mismatch", CurrentRevision: revision, CurrentSourceRevision: view.SourceRevision, ConflictScope: mutation.Scope, CurrentState: view}
	}
	var replacement map[string]any
	if mutation.Action == "put" {
		if mutation.Prepare != nil {
			mutation.Configuration, err = mutation.Prepare(view)
			if err != nil {
				return View{}, 0, err
			}
		}
		var details []StructuralError
		replacement, details = ValidateReplacement(schema, mutation.Scope, mutation.Configuration)
		if len(details) > 0 {
			return View{}, 0, &ValidationError{Details: details}
		}
		if mutation.Validate != nil {
			if err := mutation.Validate(replacement); err != nil {
				return View{}, 0, err
			}
		}
	}
	next := cloneState(s.state)
	switch mutation.Action {
	case "put":
		s.install(&next, environmentID, mutation.Scope, replacement)
	case "reset":
		s.install(&next, environmentID, mutation.Scope, map[string]any{})
	case "discard":
		s.remove(&next, environmentID, mutation.Scope)
	default:
		return View{}, 0, fmt.Errorf("invalid draft mutation action %q", mutation.Action)
	}
	next.Revision++
	if err := s.persistLocked(next); err != nil {
		return View{}, 0, err
	}
	s.state = next
	view, err = BuildView(schema, environments, source, cloneState(s.state), environmentID)
	revision = s.state.Revision
	return view, revision, err
}

func (s *Store) viewLocked(schema Schema, environments []Environment, environmentID string) (View, uint64, error) {
	source, err := s.source()
	if err != nil {
		return View{}, 0, err
	}
	view, err := BuildView(schema, environments, source, cloneState(s.state), environmentID)
	return view, s.state.Revision, err
}

func (s *Store) install(state *State, environmentID, scope string, replacement map[string]any) {
	if scope == ScopeBaseline {
		state.BaselinePresent = true
		state.Baseline = cloneMap(replacement)
		return
	}
	state.EnvironmentOverrides[environmentID] = cloneMap(replacement)
}

func (s *Store) remove(state *State, environmentID, scope string) {
	if scope == ScopeBaseline {
		state.BaselinePresent = false
		state.Baseline = nil
		return
	}
	delete(state.EnvironmentOverrides, environmentID)
}

type persistedState struct {
	Revision             uint64                    `json:"revision"`
	Baseline             *map[string]any           `json:"baseline,omitempty"`
	EnvironmentOverrides map[string]map[string]any `json:"environment_overrides"`
}

func (s *Store) persistLocked(state State) error {
	if s.path == "" {
		return nil
	}
	persisted := persistedState{Revision: state.Revision, EnvironmentOverrides: cloneOverrides(state.EnvironmentOverrides)}
	if state.BaselinePresent {
		baseline := cloneMap(state.Baseline)
		persisted.Baseline = &baseline
	}
	content, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode draft store: %w", err)
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create draft store directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".draft-*.tmp")
	if err != nil {
		return fmt.Errorf("create draft store temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace draft store: %w", err)
	}
	return nil
}

func cloneSource(source SourceSnapshot) SourceSnapshot {
	return SourceSnapshot{Revision: source.Revision, Baseline: cloneMap(source.Baseline), EnvironmentOverrides: cloneOverrides(source.EnvironmentOverrides)}
}
