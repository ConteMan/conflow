// Package operation provides the durable read model shared by asynchronous
// ConfigOps workflows. It intentionally stores only progress and resource
// references; workflow-specific input remains with its owning use case.
package operation

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrNotFound = errors.New("operation not found")
var ErrInvalidStage = errors.New("operation stage is invalid for operation type")

// AllowedStages freezes the Spec 009 workflow boundaries. Other operation
// types remain extensible for later Specs, while read workflows cannot be
// accidentally routed through a publishing stage.
var AllowedStages = map[string]map[string]bool{
	"remote_pull":      {"queued": true, "reading_remote": true, "snapshotting": true, "completed": true},
	"remote_validate":  {"queued": true, "validating_remote": true, "completed": true},
	"plan":             {"queued": true, "reading_remote": true, "compiling": true, "analyzing": true, "completed": true},
	"release":          {"queued": true, "validating_remote": true, "submitting": true, "verifying": true, "recording_audit": true, "completed": true},
	"publish":          {"queued": true, "validating_remote": true, "submitting": true, "verifying": true, "recording_audit": true, "completed": true},
	"rollback_preview": {"queued": true, "reading_remote": true, "analyzing": true, "completed": true},
	"rollback":         {"queued": true, "validating_remote": true, "submitting": true, "verifying": true, "recording_audit": true, "completed": true},
}

type Failure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	Stage     string `json:"stage"`
}

type Result struct {
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Href         string `json:"href"`
}

type Operation struct {
	OperationID   string    `json:"operation_id"`
	OperationType string    `json:"operation_type"`
	Status        string    `json:"status"`
	Stage         string    `json:"stage"`
	RemoteState   string    `json:"remote_state"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Failure       *Failure  `json:"failure,omitempty"`
	Result        *Result   `json:"result,omitempty"`
}

type Store struct {
	mu    sync.RWMutex
	path  string
	items map[string]Operation
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, items: map[string]Operation{}}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read operations: %w", err)
	}
	if err := json.Unmarshal(content, &s.items); err != nil {
		return nil, fmt.Errorf("parse operations: %w", err)
	}
	if s.items == nil {
		s.items = map[string]Operation{}
	}
	return s, nil
}

func (s *Store) Create(operationType string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	op := Operation{OperationID: "op_" + randomID(), OperationType: operationType, Status: "pending", Stage: "queued", RemoteState: "unchanged", CreatedAt: now, UpdatedAt: now}
	s.items[op.OperationID] = op
	return op, s.persistLocked()
}

func (s *Store) Get(id string) (Operation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	op, ok := s.items[id]
	if !ok {
		return Operation{}, ErrNotFound
	}
	return op, nil
}

func (s *Store) Update(id, status, stage string, failure *Failure, result *Result, remoteState string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op, ok := s.items[id]
	if !ok {
		return Operation{}, ErrNotFound
	}
	if stage != "" && AllowedStages[op.OperationType] != nil && !AllowedStages[op.OperationType][stage] {
		return Operation{}, fmt.Errorf("%w: %s/%s", ErrInvalidStage, op.OperationType, stage)
	}
	if status != "" {
		op.Status = status
	}
	if stage != "" {
		op.Stage = stage
	}
	if remoteState != "" {
		op.RemoteState = remoteState
	}
	op.Failure, op.Result, op.UpdatedAt = failure, result, time.Now().UTC()
	s.items[id] = op
	return op, s.persistLocked()
}

func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	content, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".operations-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
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

func randomID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
