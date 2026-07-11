// Package remote owns protected remote snapshot access. Provider adapters feed
// FileStore in Spec 009 without changing Plan's input contract.
package remote

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

type Summary struct {
	ParameterCount        int `json:"parameter_count"`
	ManagedParameterCount int `json:"managed_parameter_count"`
	ConditionCount        int `json:"condition_count"`
	// HasUnmodeledConditions is set by the snapshot reader until Spec 009
	// supplies condition-level Firebase Remote Config data for comparison.
	HasUnmodeledConditions bool `json:"-"`
	// These observations are provider-internal comparison facts. They are not
	// serialized in the public RemoteSummary contract and never contain values.
	HasUnknownParameters    bool   `json:"-"`
	HasStructuralDifference bool   `json:"-"`
	ContentDigest           string `json:"content_digest"`
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
	// Template is protected provider data. It is persisted with mode 0600 and
	// deliberately excluded from normal API and Plan response serialization.
	Template json.RawMessage `json:"-"`
}

// SnapshotStore is the stable Plan-facing remote snapshot contract.
type SnapshotStore interface {
	Current(environmentID string) (Snapshot, error)
}
type Store = SnapshotStore
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
		RemoteETag             string          `json:"remote_etag"`
		Version                string          `json:"version"`
		ObservedAt             time.Time       `json:"observed_at"`
		Summary                Summary         `json:"summary"`
		HasUnmodeledConditions bool            `json:"has_unmodeled_conditions"`
		Parameters             map[string]any  `json:"parameters"`
		Template               json.RawMessage `json:"template"`
	}
	if err := json.Unmarshal(content, &raw); err != nil {
		return Snapshot{}, fmt.Errorf("parse remote snapshot: %w", err)
	}
	raw.Summary.HasUnmodeledConditions = raw.HasUnmodeledConditions
	return Snapshot{Status: "available", RemoteETag: raw.RemoteETag, Version: raw.Version, ObservedAt: raw.ObservedAt, Summary: &raw.Summary, Parameters: raw.Parameters, Template: raw.Template}, nil
}

// Save writes an atomically replaced protected file. Callers only invoke it
// after a complete successful pull, so a failed read never changes the cache.
func (s *FileStore) Save(environmentID string, snapshot Snapshot) error {
	if snapshot.Status != "available" || len(snapshot.Template) == 0 {
		return errors.New("cannot persist unavailable remote snapshot")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	raw := struct {
		RemoteETag             string          `json:"remote_etag"`
		Version                string          `json:"version"`
		ObservedAt             time.Time       `json:"observed_at"`
		Summary                Summary         `json:"summary"`
		HasUnmodeledConditions bool            `json:"has_unmodeled_conditions"`
		Parameters             map[string]any  `json:"parameters"`
		Template               json.RawMessage `json:"template"`
	}{snapshot.RemoteETag, snapshot.Version, snapshot.ObservedAt, *snapshot.Summary, snapshot.Summary.HasUnmodeledConditions, snapshot.Parameters, snapshot.Template}
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".remote-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
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
	return os.Rename(tmpName, filepath.Join(s.root, environmentID+".json"))
}

// SnapshotFromTemplate derives the provider-neutral data Plan requires without
// exposing the Firebase template through normal read models.
func SnapshotFromTemplate(template []byte, etag, version string, observedAt time.Time) (Snapshot, error) {
	var document struct {
		Parameters map[string]struct {
			DefaultValue      json.RawMessage            `json:"defaultValue"`
			ConditionalValues map[string]json.RawMessage `json:"conditionalValues"`
		} `json:"parameters"`
		Conditions []json.RawMessage `json:"conditions"`
	}
	if err := json.Unmarshal(template, &document); err != nil {
		return Snapshot{}, fmt.Errorf("parse firebase template: %w", err)
	}
	parameters := make(map[string]any, len(document.Parameters))
	unmodeled, structural := false, false
	for key, parameter := range document.Parameters {
		if len(parameter.ConditionalValues) > 0 {
			unmodeled = true
		}
		if len(parameter.DefaultValue) == 0 {
			// A parameter without Firebase's expected defaultValue shape is a
			// structural observation, not a value to expose.
			structural = true
			continue
		}
		var defaultValue struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(parameter.DefaultValue, &defaultValue) == nil {
			parameters[key] = defaultValue.Value
		}
	}
	digest := sha256.Sum256(template)
	return Snapshot{Status: "available", RemoteETag: etag, Version: version, ObservedAt: observedAt, Summary: &Summary{ParameterCount: len(document.Parameters), ManagedParameterCount: len(document.Parameters), ConditionCount: len(document.Conditions), HasUnmodeledConditions: unmodeled, HasStructuralDifference: structural, ContentDigest: "sha256:" + hex.EncodeToString(digest[:])}, Parameters: parameters, Template: append(json.RawMessage(nil), template...)}, nil
}
