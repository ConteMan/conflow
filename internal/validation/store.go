package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrNotFound = errors.New("validation result not found")

// Store persists complete-validation results next to draft state. This is a
// temporary Spec 005 boundary: source configuration persistence is not owned
// by this file and validation records remain local under .conflow.
type Store struct {
	mu      sync.Mutex
	path    string
	results map[string]Result
}

func Open(path string) (*Store, error) {
	store := &Store{path: path, results: map[string]Result{}}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read validation results: %w", err)
	}
	if err := json.Unmarshal(content, &store.results); err != nil {
		return nil, fmt.Errorf("parse validation results: %w", err)
	}
	if store.results == nil {
		store.results = map[string]Result{}
	}
	return store, nil
}

func NewMemory() *Store { return &Store{results: map[string]Result{}} }

func (s *Store) Save(result Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result.Status = StatusFresh
	result.Diagnostics = append([]Diagnostic{}, result.Diagnostics...)
	if result.Diagnostics == nil {
		result.Diagnostics = []Diagnostic{}
	}
	s.results[result.EnvironmentID] = result
	return s.persistLocked()
}

func (s *Store) Get(environmentID string, currentDraftRevision uint64) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, ok := s.results[environmentID]
	if !ok {
		return Result{}, ErrNotFound
	}
	result.Diagnostics = append([]Diagnostic{}, result.Diagnostics...)
	if result.Diagnostics == nil {
		result.Diagnostics = []Diagnostic{}
	}
	if result.ValidatedDraftRevision == currentDraftRevision {
		result.Status = StatusFresh
	} else {
		result.Status = StatusStale
	}
	return result, nil
}

func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	content, err := json.MarshalIndent(s.results, "", "  ")
	if err != nil {
		return fmt.Errorf("encode validation results: %w", err)
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create validation result directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".validation-*.tmp")
	if err != nil {
		return fmt.Errorf("create validation result temp file: %w", err)
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
		return fmt.Errorf("replace validation results: %w", err)
	}
	return nil
}
