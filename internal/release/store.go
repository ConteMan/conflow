// Package release owns the local durable boundary for publish idempotency and
// the minimal successful-release metadata consumed by Spec 011.
package release

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/ConteMan/conflow/internal/operation"
	"github.com/ConteMan/conflow/internal/remote"
)

var ErrIdempotencyConflict = errors.New("idempotency key conflicts with a different request")

type AuditState struct {
	RemoteETag string          `json:"remote_etag"`
	Version    string          `json:"version"`
	ObservedAt time.Time       `json:"observed_at"`
	Summary    *remote.Summary `json:"summary"`
}

type Release struct {
	ReleaseID       string             `json:"release_id"`
	EnvironmentID   string             `json:"environment_id"`
	Kind            string             `json:"kind"`
	Outcome         string             `json:"outcome"`
	CreatedAt       time.Time          `json:"created_at"`
	CompletedAt     time.Time          `json:"completed_at"`
	OperationID     string             `json:"operation_id"`
	RemoteState     string             `json:"remote_state"`
	SemanticSummary string             `json:"semantic_summary"`
	RiskSummary     string             `json:"risk_summary"`
	PlanID          string             `json:"plan_id,omitempty"`
	SourceDigest    string             `json:"source_digest,omitempty"`
	PlanDigest      string             `json:"plan_digest,omitempty"`
	RemoteBefore    AuditState         `json:"remote_before"`
	RemoteAfter     AuditState         `json:"remote_after"`
	Failure         *operation.Failure `json:"failure,omitempty"`
}

type idempotencyRecord struct {
	Digest      string `json:"digest"`
	OperationID string `json:"operation_id"`
}
type disk struct {
	Idempotency map[string]idempotencyRecord `json:"idempotency"`
	Releases    map[string]Release           `json:"releases"`
}
type Store struct {
	mu   sync.Mutex
	path string
	disk disk
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, disk: disk{Idempotency: map[string]idempotencyRecord{}, Releases: map[string]Release{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read releases: %w", err)
	}
	if err := json.Unmarshal(b, &s.disk); err != nil {
		return nil, fmt.Errorf("parse releases: %w", err)
	}
	if s.disk.Idempotency == nil {
		s.disk.Idempotency = map[string]idempotencyRecord{}
	}
	if s.disk.Releases == nil {
		s.disk.Releases = map[string]Release{}
	}
	return s, nil
}

// Reserve maps one environment/action/key to one durable Operation. The caller
// serializes operation creation with this call, making the mapping atomic at
// the application boundary without exposing credentials or request bodies.
func (s *Store) Reserve(environmentID, key, digest, operationID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	storageKey := environmentID + "|publish|" + key
	if existing, ok := s.disk.Idempotency[storageKey]; ok {
		if existing.Digest != digest {
			return "", ErrIdempotencyConflict
		}
		return existing.OperationID, nil
	}
	s.disk.Idempotency[storageKey] = idempotencyRecord{Digest: digest, OperationID: operationID}
	return operationID, s.persistLocked()
}

func (s *Store) Lookup(environmentID, key, digest string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.disk.Idempotency[environmentID+"|publish|"+key]
	if !ok {
		return "", false, nil
	}
	if existing.Digest != digest {
		return "", true, ErrIdempotencyConflict
	}
	return existing.OperationID, true, nil
}

func (s *Store) Save(value Release) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disk.Releases[value.ReleaseID] = value
	return s.persistLocked()
}
func (s *Store) Get(id string) (Release, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.disk.Releases[id]
	return value, ok
}
func (s *Store) List(environmentID string) []Release {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []Release{}
	for _, value := range s.disk.Releases {
		if value.EnvironmentID == environmentID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}
func NewID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("rel_%x", time.Now().UnixNano())
	}
	return "rel_" + hex.EncodeToString(b)
}
func (s *Store) persistLocked() error {
	b, err := json.MarshalIndent(s.disk, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".releases-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
}
