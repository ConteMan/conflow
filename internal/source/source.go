// Package source owns configuration source adapters. It deliberately has no
// dependency on Pack or draft semantics.
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/goccy/go-yaml"
)

var ErrRevisionMismatch = errors.New("source revision mismatch")

type Snapshot struct {
	Revision             string
	Baseline             map[string]any
	EnvironmentOverrides map[string]map[string]any
}

type Capabilities struct {
	Read bool `json:"read"`
	Save bool `json:"save"`
}

type Status struct {
	Type             string   `json:"type"`
	Digest           string   `json:"digest"`
	ExternalModified bool     `json:"external_modified"`
	Paths            []string `json:"paths"`
}

type Adapter interface {
	Load() (Snapshot, error)
	Save(SaveInput) (Snapshot, error)
	Status() (Status, error)
	Capabilities() Capabilities
}

// SaveInput replaces the two source layers visible to one draft save. A nil
// environment override removes the managed file for that environment.
type SaveInput struct {
	ExpectedRevision    string
	EnvironmentID       string
	Baseline            map[string]any
	EnvironmentOverride map[string]any
}

type ManagedFile struct {
	mu         sync.Mutex
	root       string
	lastDigest string
	writeFile  func(string, []byte) error
}

func OpenManagedFile(workspace string) *ManagedFile {
	return &ManagedFile{root: filepath.Join(workspace, ".conflow", "data"), writeFile: atomicWrite}
}

func (m *ManagedFile) Capabilities() Capabilities { return Capabilities{Read: true, Save: true} }

func (m *ManagedFile) Load() (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot, digest, _, err := m.loadLocked()
	if err != nil {
		return Snapshot{}, err
	}
	m.lastDigest = digest
	return snapshot, nil
}

func (m *ManagedFile) Status() (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, digest, paths, err := m.loadLocked()
	if err != nil {
		return Status{}, err
	}
	return Status{Type: "managed-file", Digest: digest, ExternalModified: m.lastDigest != "" && m.lastDigest != digest, Paths: paths}, nil
}

func (m *ManagedFile) Save(input SaveInput) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, digest, _, err := m.loadLocked()
	if err != nil {
		return Snapshot{}, err
	}
	if input.ExpectedRevision != digest {
		return Snapshot{}, ErrRevisionMismatch
	}
	base, err := canonicalYAML(input.Baseline)
	if err != nil {
		return Snapshot{}, err
	}
	if err := m.writeFile(m.basePath(), base); err != nil {
		return Snapshot{}, fmt.Errorf("write managed base: %w", err)
	}
	if input.EnvironmentOverride == nil {
		if err := os.Remove(m.environmentPath(input.EnvironmentID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, fmt.Errorf("remove managed environment override: %w", err)
		}
	} else {
		override, err := canonicalYAML(input.EnvironmentOverride)
		if err != nil {
			return Snapshot{}, err
		}
		if err := m.writeFile(m.environmentPath(input.EnvironmentID), override); err != nil {
			return Snapshot{}, fmt.Errorf("write managed environment override: %w", err)
		}
	}
	snapshot, nextDigest, _, err := m.loadLocked()
	if err != nil {
		return Snapshot{}, err
	}
	m.lastDigest = nextDigest
	return snapshot, nil
}

func (m *ManagedFile) basePath() string { return filepath.Join(m.root, "base.yaml") }
func (m *ManagedFile) environmentPath(id string) string {
	return filepath.Join(m.root, "environments", id+".yaml")
}

func (m *ManagedFile) loadLocked() (Snapshot, string, []string, error) {
	baseline, baseExists, err := readConfiguration(m.basePath())
	if err != nil {
		return Snapshot{}, "", nil, err
	}
	if !baseExists {
		baseline = map[string]any{}
	}
	entries, err := os.ReadDir(filepath.Join(m.root, "environments"))
	if errors.Is(err, os.ErrNotExist) {
		entries = nil
	} else if err != nil {
		return Snapshot{}, "", nil, fmt.Errorf("read managed environment directory: %w", err)
	}
	overrides := map[string]map[string]any{}
	paths := []string{".conflow/data/base.yaml"}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".yaml")]
		value, exists, readErr := readConfiguration(filepath.Join(m.root, "environments", entry.Name()))
		if readErr != nil {
			return Snapshot{}, "", nil, readErr
		}
		if exists {
			overrides[id] = value
			paths = append(paths, filepath.ToSlash(filepath.Join(".conflow", "data", "environments", entry.Name())))
		}
	}
	sort.Strings(paths)
	canonical, err := canonicalSource(baseline, overrides)
	if err != nil {
		return Snapshot{}, "", nil, err
	}
	digest := sourceDigest(canonical)
	return Snapshot{Revision: digest, Baseline: cloneMap(baseline), EnvironmentOverrides: cloneOverrides(overrides)}, digest, paths, nil
}

func readConfiguration(path string) (map[string]any, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read managed file %s: %w", path, err)
	}
	var value any
	if err := yaml.Unmarshal(content, &value); err != nil {
		return nil, false, fmt.Errorf("parse managed file %s: %w", path, err)
	}
	configuration, ok := normalize(value).(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("managed file %s must contain a YAML object", path)
	}
	return configuration, true, nil
}

func canonicalSource(baseline map[string]any, overrides map[string]map[string]any) ([]byte, error) {
	return canonicalYAML(map[string]any{"baseline": baseline, "environment_overrides": overrides})
}

func canonicalYAML(value any) ([]byte, error) {
	content, err := yaml.MarshalWithOptions(sorted(value), yaml.Indent(2), yaml.IndentSequence(true))
	if err != nil {
		return nil, fmt.Errorf("encode managed file: %w", err)
	}
	return content, nil
}

func sorted(value any) any {
	switch typed := normalize(value).(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make(yaml.MapSlice, 0, len(keys))
		for _, key := range keys {
			result = append(result, yaml.MapItem{Key: key, Value: sorted(typed[key])})
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = sorted(typed[index])
		}
		return result
	default:
		return typed
	}
}

func sourceDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return "src-" + hex.EncodeToString(sum[:])
}

func atomicWrite(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".managed-*.tmp")
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

func normalize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = normalize(child)
		}
		return result
	case map[any]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[fmt.Sprint(key)] = normalize(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = normalize(child)
		}
		return result
	case int:
		return float64(typed)
	case int8:
		return float64(typed)
	case int16:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint8:
		return float64(typed)
	case uint16:
		return float64(typed)
	case uint32:
		return float64(typed)
	case uint64:
		return float64(typed)
	case float32:
		return float64(typed)
	default:
		return typed
	}
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	result := make(map[string]any, len(value))
	for key, child := range value {
		result[key] = normalize(child)
	}
	return result
}
func cloneOverrides(value map[string]map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any, len(value))
	for key, child := range value {
		result[key] = cloneMap(child)
	}
	return result
}
