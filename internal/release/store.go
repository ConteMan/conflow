// Package release owns the local durable boundary for release idempotency,
// audit records, protected version snapshots, and rollback previews.
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
var ErrNotFound = errors.New("release not found")
var ErrPreviewNotFound = errors.New("rollback preview not found")
var ErrCorrupt = errors.New("release audit store is corrupt")

type AuditState struct {
	RemoteETag string          `json:"remote_etag"`
	Version    string          `json:"version"`
	ObservedAt time.Time       `json:"observed_at"`
	Summary    *remote.Summary `json:"summary"`
}

type Release struct {
	ReleaseID           string             `json:"release_id"`
	EnvironmentID       string             `json:"environment_id"`
	Kind                string             `json:"kind"`
	Outcome             string             `json:"outcome"`
	CreatedAt           time.Time          `json:"created_at"`
	CompletedAt         time.Time          `json:"completed_at"`
	OperationID         string             `json:"operation_id"`
	RemoteState         string             `json:"remote_state"`
	SemanticSummary     string             `json:"semantic_summary"`
	RiskSummary         string             `json:"risk_summary"`
	PlanID              string             `json:"plan_id,omitempty"`
	RollbackOfReleaseID string             `json:"rollback_of_release_id,omitempty"`
	SourceDigest        string             `json:"source_digest,omitempty"`
	PlanDigest          string             `json:"plan_digest,omitempty"`
	RemoteBefore        AuditState         `json:"remote_before"`
	RemoteAfter         *AuditState        `json:"remote_after,omitempty"`
	Failure             *operation.Failure `json:"failure,omitempty"`
}

// RollbackPreview is the durable immutable read model consumed by the
// rollback confirmation endpoint. Its target template is held separately in
// disk.PrivateTemplates and is never included here.
type RollbackPreview struct {
	RollbackPreviewID        string          `json:"rollback_preview_id"`
	EnvironmentID            string          `json:"environment_id"`
	TargetReleaseID          string          `json:"target_release_id"`
	TargetRemoteVersion      string          `json:"target_remote_version"`
	Status                   string          `json:"status"`
	ExpectedRemoteETag       string          `json:"expected_remote_etag"`
	CreatedAt                time.Time       `json:"created_at"`
	ExpiresAt                time.Time       `json:"expires_at"`
	InvalidationReason       string          `json:"invalidation_reason,omitempty"`
	CurrentRemote            AuditState      `json:"current_remote"`
	SemanticChanges          json.RawMessage `json:"semantic_changes"`
	RemoteParameterChanges   json.RawMessage `json:"remote_parameter_changes"`
	Severity                 string          `json:"severity"`
	RiskItems                json.RawMessage `json:"risk_items"`
	BlockingReasons          json.RawMessage `json:"blocking_reasons"`
	ConfirmationRequirements json.RawMessage `json:"confirmation_requirements"`
}

type idempotencyRecord struct {
	Digest      string `json:"digest"`
	OperationID string `json:"operation_id"`
}
type disk struct {
	Idempotency map[string]idempotencyRecord `json:"idempotency"`
	Releases    map[string]Release           `json:"releases"`
	Previews    map[string]RollbackPreview   `json:"previews,omitempty"`
	// PrivateTemplates contains protected Firebase templates keyed by Release
	// ID. The enclosing file is mode 0600, and this field is never returned by
	// a public read model.
	PrivateTemplates map[string]json.RawMessage `json:"private_templates,omitempty"`
	// PrivateConflowState holds the Conflow effective desired state at the time
	// of each successful publish, keyed by Release ID. It is used as the
	// baseline for subsequent plan builds so that the plan diff reflects only
	// changes made since the last publish rather than the full config vs. empty
	// schema default. Never exposed by any public read model.
	PrivateConflowState map[string]json.RawMessage `json:"private_conflow_state,omitempty"`
}
type Store struct {
	mu      sync.Mutex
	path    string
	disk    disk
	corrupt error
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, disk: disk{Idempotency: map[string]idempotencyRecord{}, Releases: map[string]Release{}, Previews: map[string]RollbackPreview{}, PrivateTemplates: map[string]json.RawMessage{}, PrivateConflowState: map[string]json.RawMessage{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read releases: %w", err)
	}
	if err := json.Unmarshal(b, &s.disk); err != nil {
		s.corrupt = fmt.Errorf("%w: parse releases: %v", ErrCorrupt, err)
		return s, nil
	}
	if s.disk.Idempotency == nil {
		s.disk.Idempotency = map[string]idempotencyRecord{}
	}
	if s.disk.Releases == nil {
		s.disk.Releases = map[string]Release{}
	}
	if s.disk.Previews == nil {
		s.disk.Previews = map[string]RollbackPreview{}
	}
	if s.disk.PrivateTemplates == nil {
		s.disk.PrivateTemplates = map[string]json.RawMessage{}
	}
	if s.disk.PrivateConflowState == nil {
		s.disk.PrivateConflowState = map[string]json.RawMessage{}
	}
	if err := validateDisk(s.disk); err != nil {
		s.corrupt = err
	}
	return s, nil
}

func validateDisk(value disk) error {
	for id, record := range value.Releases {
		if id == "" || id != record.ReleaseID || record.EnvironmentID == "" || (record.Kind != "publish" && record.Kind != "rollback") || (record.Outcome != "succeeded" && record.Outcome != "failed") || record.OperationID == "" || record.CreatedAt.IsZero() {
			return fmt.Errorf("%w: invalid release record %q", ErrCorrupt, id)
		}
		if record.Outcome == "succeeded" && record.RemoteAfter == nil {
			// Spec 010 records did not have a pointer remote_after. Decode the old
			// zero value as absent only when it genuinely carries no audit state.
			return fmt.Errorf("%w: succeeded release %q has no remote_after", ErrCorrupt, id)
		}
	}
	for id, preview := range value.Previews {
		if id == "" || id != preview.RollbackPreviewID || preview.EnvironmentID == "" || preview.TargetReleaseID == "" || preview.ExpectedRemoteETag == "" || (preview.Status != "ready" && preview.Status != "invalidated" && preview.Status != "expired") || preview.CreatedAt.IsZero() || preview.ExpiresAt.IsZero() {
			return fmt.Errorf("%w: invalid rollback preview %q", ErrCorrupt, id)
		}
	}
	return nil
}

func (s *Store) healthyLocked() error {
	if s.corrupt != nil {
		return s.corrupt
	}
	return nil
}

// Reserve maps one environment/action/key to one durable Operation. The caller
// serializes operation creation with this call, making the mapping atomic at
// the application boundary without exposing credentials or request bodies.
func (s *Store) Reserve(environmentID, key, digest, operationID string) (string, error) {
	return s.ReserveAction(environmentID, "publish", key, digest, operationID)
}

func (s *Store) ReserveAction(environmentID, action, key, digest, operationID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return "", err
	}
	storageKey := environmentID + "|" + action + "|" + key
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
	return s.LookupAction(environmentID, "publish", key, digest)
}

func (s *Store) LookupAction(environmentID, action, key, digest string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return "", false, err
	}
	existing, ok := s.disk.Idempotency[environmentID+"|"+action+"|"+key]
	if !ok {
		return "", false, nil
	}
	if existing.Digest != digest {
		return "", true, ErrIdempotencyConflict
	}
	return existing.OperationID, true, nil
}

func (s *Store) Save(value Release) error {
	return s.SaveWithTemplate(value, nil)
}
func (s *Store) SaveWithTemplate(value Release, template []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return err
	}
	s.disk.Releases[value.ReleaseID] = value
	if len(template) > 0 {
		s.disk.PrivateTemplates[value.ReleaseID] = append(json.RawMessage(nil), template...)
	}
	return s.persistLocked()
}
func (s *Store) Get(id string) (Release, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return Release{}, false, err
	}
	value, ok := s.disk.Releases[id]
	return value, ok, nil
}
func (s *Store) List(environmentID string, limit int, cursor string) ([]Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return nil, err
	}
	result := []Release{}
	for _, value := range s.disk.Releases {
		if value.EnvironmentID == environmentID {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ReleaseID > result[j].ReleaseID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	if cursor != "" {
		start := len(result)
		for i := range result {
			if result[i].ReleaseID == cursor {
				start = i + 1
				break
			}
		}
		result = result[start:]
	}
	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (s *Store) Template(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return nil, err
	}
	value, ok := s.disk.PrivateTemplates[id]
	if !ok || len(value) == 0 {
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}
// SaveConflowState persists the Conflow effective desired state that was
// published in the given release. Errors are non-critical; callers may ignore
// them without affecting release outcome correctness.
func (s *Store) SaveConflowState(releaseID string, state json.RawMessage) error {
	if releaseID == "" || len(state) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return err
	}
	s.disk.PrivateConflowState[releaseID] = append(json.RawMessage(nil), state...)
	return s.persistLocked()
}

// ConflowState returns the stored Conflow effective desired state for a release.
func (s *Store) ConflowState(releaseID string) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return nil, err
	}
	value, ok := s.disk.PrivateConflowState[releaseID]
	if !ok || len(value) == 0 {
		return nil, ErrNotFound
	}
	return append(json.RawMessage(nil), value...), nil
}

// LatestSucceeded returns the most recent succeeded release for the given
// environment, or (Release{}, false, nil) if none exists.
func (s *Store) LatestSucceeded(environmentID string) (Release, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return Release{}, false, err
	}
	var result Release
	for _, r := range s.disk.Releases {
		if r.EnvironmentID != environmentID || r.Outcome != "succeeded" {
			continue
		}
		if result.ReleaseID == "" || r.CreatedAt.After(result.CreatedAt) {
			result = r
		}
	}
	if result.ReleaseID == "" {
		return Release{}, false, nil
	}
	return result, true, nil
}

func (s *Store) SavePreview(value RollbackPreview) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return err
	}
	s.disk.Previews[value.RollbackPreviewID] = value
	return s.persistLocked()
}
func (s *Store) Preview(environmentID, targetID string) (RollbackPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return RollbackPreview{}, err
	}
	var result RollbackPreview
	for _, preview := range s.disk.Previews {
		if preview.EnvironmentID == environmentID && preview.TargetReleaseID == targetID && (result.CreatedAt.IsZero() || preview.CreatedAt.After(result.CreatedAt)) {
			result = preview
		}
	}
	if result.RollbackPreviewID == "" {
		return RollbackPreview{}, ErrPreviewNotFound
	}
	if result.Status == "ready" && !time.Now().UTC().Before(result.ExpiresAt) {
		result.Status, result.InvalidationReason = "expired", "ttl_expired"
		s.disk.Previews[result.RollbackPreviewID] = result
		if err := s.persistLocked(); err != nil {
			return RollbackPreview{}, err
		}
	}
	return result, nil
}
func (s *Store) PreviewByID(id string) (RollbackPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return RollbackPreview{}, err
	}
	value, ok := s.disk.Previews[id]
	if !ok {
		return RollbackPreview{}, ErrPreviewNotFound
	}
	if value.Status == "ready" && !time.Now().UTC().Before(value.ExpiresAt) {
		value.Status, value.InvalidationReason = "expired", "ttl_expired"
		s.disk.Previews[id] = value
		if err := s.persistLocked(); err != nil {
			return RollbackPreview{}, err
		}
	}
	return value, nil
}
func (s *Store) InvalidatePreview(id, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.healthyLocked(); err != nil {
		return err
	}
	value, ok := s.disk.Previews[id]
	if !ok {
		return ErrPreviewNotFound
	}
	value.Status, value.InvalidationReason = "invalidated", reason
	s.disk.Previews[id] = value
	return s.persistLocked()
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
