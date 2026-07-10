package project

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/goccy/go-yaml"
)

var (
	ErrRevisionMismatch = errors.New("project revision mismatch")
	ErrInvalidManifest  = errors.New("invalid project manifest")
	ErrNotFound         = errors.New("project resource not found")
	ErrAlreadyExists    = errors.New("project resource already exists")
)

type RevisionMismatchError struct {
	Expected uint64
	Current  uint64
	Snapshot Snapshot
}

func (e *RevisionMismatchError) Error() string {
	return fmt.Sprintf("%v: expected %d, current %d", ErrRevisionMismatch, e.Expected, e.Current)
}

func (e *RevisionMismatchError) Unwrap() error {
	return ErrRevisionMismatch
}

type Snapshot struct {
	Manifest Manifest
	Revision uint64
	Digest   string
}

type Store struct {
	mu         sync.Mutex
	path       string
	manifest   Manifest
	revision   uint64
	digest     string
	refreshErr error
}

func Open(workspace string) (*Store, error) {
	path := ManifestPath(workspace)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read project manifest %s: %w", path, err)
	}
	manifest, err := parseManifest(content)
	if err != nil {
		return nil, fmt.Errorf("open project manifest %s: %w", path, err)
	}
	return &Store{
		path:     path,
		manifest: manifest,
		revision: 1,
		digest:   digest(content),
	}, nil
}

func (s *Store) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return Snapshot{}, err
	}
	return s.snapshotLocked(), nil
}

func (s *Store) Update(expectedRevision uint64, mutate func(*Manifest) error) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return Snapshot{}, err
	}
	if expectedRevision != s.revision {
		current := s.snapshotLocked()
		return Snapshot{}, &RevisionMismatchError{Expected: expectedRevision, Current: current.Revision, Snapshot: current}
	}

	next := cloneManifest(s.manifest)
	if err := mutate(&next); err != nil {
		return Snapshot{}, err
	}
	if err := Validate(next); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	content, err := yaml.Marshal(next)
	if err != nil {
		return Snapshot{}, fmt.Errorf("encode project manifest: %w", err)
	}
	if err := atomicWrite(s.path, content); err != nil {
		return Snapshot{}, fmt.Errorf("write project manifest: %w", err)
	}

	s.manifest = next
	s.digest = digest(content)
	s.refreshErr = nil
	s.revision++
	return s.snapshotLocked(), nil
}

func (s *Store) refreshLocked() error {
	content, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read project manifest %s: %w", s.path, err)
	}
	currentDigest := digest(content)
	if currentDigest == s.digest {
		return s.refreshErr
	}

	s.digest = currentDigest
	s.revision++
	manifest, parseErr := parseManifest(content)
	if parseErr != nil {
		s.refreshErr = fmt.Errorf("refresh project manifest %s: %w", s.path, parseErr)
		return s.refreshErr
	}
	s.manifest = manifest
	s.refreshErr = nil
	return nil
}

func (s *Store) snapshotLocked() Snapshot {
	return Snapshot{
		Manifest: cloneManifest(s.manifest),
		Revision: s.revision,
		Digest:   s.digest,
	}
}

func parseManifest(content []byte) (Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(content, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if err := Validate(manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	return manifest, nil
}

func cloneManifest(manifest Manifest) Manifest {
	clone := manifest
	clone.Environments = append([]Environment(nil), manifest.Environments...)
	return clone
}

func digest(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

func atomicWrite(path string, content []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".project-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o644); err != nil {
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
	return os.Rename(temporaryPath, path)
}
