// Package releasedbaseline persists the entity hashes from the last successful
// release of each environment.
package releasedbaseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Document struct {
	EnvironmentID  string            `json:"environment_id"`
	ReleaseID      string            `json:"release_id"`
	ReleasedAt     time.Time         `json:"released_at"`
	SourceRevision string            `json:"source_revision"`
	Entities       map[string]string `json:"entities"`
}

type Store struct {
	root string
	mu   sync.Mutex
}

func Open(root string) *Store { return &Store{root: root} }

func (s *Store) Load(environmentID string) (Document, bool, error) {
	path, err := s.path(environmentID)
	if err != nil {
		return Document{}, false, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Document{}, false, nil
	}
	if err != nil {
		return Document{}, false, fmt.Errorf("read released baseline: %w", err)
	}
	var document Document
	if err := json.Unmarshal(content, &document); err != nil {
		return Document{}, false, fmt.Errorf("parse released baseline: %w", err)
	}
	if document.EnvironmentID != environmentID || document.ReleaseID == "" || document.ReleasedAt.IsZero() || document.Entities == nil {
		return Document{}, false, errors.New("released baseline is invalid")
	}
	return document, true, nil
}

func (s *Store) Save(document Document) error {
	path, err := s.path(document.EnvironmentID)
	if err != nil {
		return err
	}
	if document.ReleaseID == "" || document.ReleasedAt.IsZero() || document.Entities == nil {
		return errors.New("released baseline is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	content, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode released baseline: %w", err)
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create released baseline directory: %w", err)
	}
	temporary, err := os.CreateTemp(s.root, ".released-baseline-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(content, '\n')); err != nil {
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace released baseline: %w", err)
	}
	return nil
}

// Clear removes the baseline so that entity change markers fall back to the
// no-baseline behaviour until the next successful release rebuilds it. Used
// when rolling back to a release that predates baseline capture.
func (s *Store) Clear(environmentID string) error {
	path, err := s.path(environmentID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear released baseline: %w", err)
	}
	return nil
}

func HashFields(fields map[string]any) (string, error) {
	// encoding/json sorts map keys and emits compact JSON, which is the
	// canonical representation required for entity field hashes.
	content, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (s *Store) path(environmentID string) (string, error) {
	if environmentID == "" || filepath.Base(environmentID) != environmentID {
		return "", errors.New("environment ID is invalid")
	}
	return filepath.Join(s.root, environmentID+".json"), nil
}
