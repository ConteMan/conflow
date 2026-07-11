// Package remote owns protected remote snapshot access. Provider adapters will
// replace FileStore in Spec 009 without changing Plan's input contract.
package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Summary struct {
	ParameterCount        int `json:"parameter_count"`
	ManagedParameterCount int `json:"managed_parameter_count"`
	ConditionCount        int `json:"condition_count"`
	// HasUnmodeledConditions is set by the snapshot reader until Spec 009
	// supplies condition-level Firebase Remote Config data for comparison.
	HasUnmodeledConditions bool   `json:"has_unmodeled_conditions"`
	ContentDigest          string `json:"content_digest"`
}
type UnavailableReason string

const (
	ProviderUnavailable   UnavailableReason = "provider_unavailable"
	ProviderUnauthorized  UnavailableReason = "provider_unauthorized"
	SnapshotMissing       UnavailableReason = "remote_snapshot_missing"
	CapabilityUnavailable UnavailableReason = "provider_capability_unavailable"
)

type Snapshot struct {
	Status            string            `json:"status"`
	RemoteETag        string            `json:"remote_etag,omitempty"`
	Version           string            `json:"version,omitempty"`
	ObservedAt        time.Time         `json:"observed_at,omitempty"`
	Summary           *Summary          `json:"summary,omitempty"`
	UnavailableReason UnavailableReason `json:"unavailable_reason,omitempty"`
	Parameters        map[string]any    `json:"-"`
}
type Store interface {
	Current(environmentID string) (Snapshot, error)
}
type FileStore struct {
	root string
	mu   sync.RWMutex
}

func OpenFileStore(workspace string) *FileStore {
	return &FileStore{root: filepath.Join(workspace, ".conflow", "remote-snapshots")}
}
func (s *FileStore) Current(environmentID string) (Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	content, err := os.ReadFile(filepath.Join(s.root, environmentID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{Status: "unavailable", UnavailableReason: SnapshotMissing}, nil
	}
	if err != nil {
		return Snapshot{Status: "unavailable", UnavailableReason: ProviderUnavailable}, nil
	}
	var raw struct {
		RemoteETag string         `json:"remote_etag"`
		Version    string         `json:"version"`
		ObservedAt time.Time      `json:"observed_at"`
		Summary    Summary        `json:"summary"`
		Parameters map[string]any `json:"parameters"`
	}
	if err := json.Unmarshal(content, &raw); err != nil {
		return Snapshot{}, fmt.Errorf("parse remote snapshot: %w", err)
	}
	return Snapshot{Status: "available", RemoteETag: raw.RemoteETag, Version: raw.Version, ObservedAt: raw.ObservedAt, Summary: &raw.Summary, Parameters: raw.Parameters}, nil
}
