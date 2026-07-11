package source

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/goccy/go-yaml"
)

var (
	ErrGitWorkspaceDirty = errors.New("git workspace is dirty")
	ErrRoundTripBlocked  = errors.New("git-json round-trip is blocked")
)

// GitWorkspace is deliberately limited to safe, user-visible repository state.
// The adapter only invokes read-only git commands.
type GitWorkspace struct {
	Root   string `json:"root"`
	Branch string `json:"branch"`
	Dirty  bool   `json:"dirty"`
}

type MappingDiagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type InspectResult struct {
	Workspace   GitWorkspace        `json:"workspace"`
	ProfilePath string              `json:"profile_path"`
	Matched     bool                `json:"matched"`
	Diagnostics []MappingDiagnostic `json:"diagnostics"`
}

type Preview struct {
	Digest string        `json:"source_digest"`
	Files  []PreviewFile `json:"files"`
}

type PreviewFile struct {
	Path    string `json:"path"`
	Diff    string `json:"diff"`
	Changed bool   `json:"changed"`
}

// GitJSONProfile is a declarative bridge from a repository-owned document to
// the source-neutral configuration shape. Paths are RFC 6901 JSON pointers.
type GitJSONProfile struct {
	Version  int              `yaml:"version" json:"version"`
	Files    []GitJSONFile    `yaml:"files" json:"files"`
	Mappings []GitJSONMapping `yaml:"mappings" json:"mappings"`
}

type GitJSONFile struct {
	Path   string `yaml:"path" json:"path"`
	Format string `yaml:"format" json:"format"`
}

type GitJSONMapping struct {
	Name              string                  `yaml:"name" json:"name"`
	Collection        string                  `yaml:"collection" json:"collection"`
	Scope             string                  `yaml:"scope" json:"scope"`
	File              string                  `yaml:"file" json:"file"`
	RecordsPath       string                  `yaml:"records_path" json:"records_path"`
	IDPath            string                  `yaml:"id_path" json:"id_path"`
	EnvironmentIDPath string                  `yaml:"environment_id_path" json:"environment_id_path"`
	Fields            map[string]GitJSONField `yaml:"fields" json:"fields"`
}

type GitJSONField struct {
	Path      string `yaml:"path" json:"path"`
	Transform string `yaml:"transform" json:"transform"`
}

type GitJSON struct {
	mu          sync.Mutex
	workspace   string
	profilePath string
	profile     GitJSONProfile
	lastDigest  string
}

func OpenGitJSON(workspace, profilePath string) (*GitJSON, error) {
	if profilePath == "" {
		return nil, errors.New("git-json profile path is required")
	}
	root, err := gitWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	resolved, err := safePath(root.Root, profilePath)
	if err != nil {
		return nil, fmt.Errorf("resolve git-json profile: %w", err)
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read git-json profile: %w", err)
	}
	var profile GitJSONProfile
	if err := yaml.Unmarshal(content, &profile); err != nil {
		return nil, fmt.Errorf("parse git-json profile: %w", err)
	}
	if err := validateProfile(profile); err != nil {
		return nil, err
	}
	relative, _ := filepath.Rel(root.Root, resolved)
	return &GitJSON{workspace: root.Root, profilePath: filepath.ToSlash(relative), profile: profile}, nil
}

func (g *GitJSON) Capabilities() Capabilities { return Capabilities{Read: true, Save: true} }

func (g *GitJSON) Load() (Snapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	snapshot, _, _, err := g.loadLocked()
	if err != nil {
		return Snapshot{}, err
	}
	g.lastDigest = snapshot.Revision
	return snapshot, nil
}

func (g *GitJSON) Status() (Status, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	snapshot, _, paths, err := g.loadLocked()
	if err != nil {
		return Status{}, err
	}
	workspace, err := gitWorkspace(g.workspace)
	if err != nil {
		return Status{}, err
	}
	return Status{Type: "git-json", Digest: snapshot.Revision, ExternalModified: g.lastDigest != "" && g.lastDigest != snapshot.Revision, Paths: paths, Git: &workspace}, nil
}

func (g *GitJSON) Inspect() (InspectResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	workspace, err := gitWorkspace(g.workspace)
	if err != nil {
		return InspectResult{}, err
	}
	_, _, _, loadErr := g.loadLocked()
	diagnostics := []MappingDiagnostic{}
	if loadErr != nil {
		diagnostics = diagnosticsFromRoundTripError(loadErr)
		if len(diagnostics) == 0 {
			diagnostics = append(diagnostics, MappingDiagnostic{Code: "profile_mismatch", Message: "映射 profile 无法读取当前源文件"})
		}
	}
	return InspectResult{Workspace: workspace, ProfilePath: g.profilePath, Matched: loadErr == nil && len(diagnostics) == 0, Diagnostics: diagnostics}, nil
}

func (g *GitJSON) Preview(input SaveInput) (Preview, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, docs, _, err := g.loadLocked()
	if err != nil {
		return Preview{}, err
	}
	changes, diagnostics, err := g.renderSave(docs, input)
	if err != nil {
		return Preview{}, err
	}
	if len(diagnostics) > 0 {
		return Preview{}, roundTripError(diagnostics)
	}
	files := make([]PreviewFile, 0, len(changes))
	for _, change := range changes {
		files = append(files, PreviewFile{Path: change.path, Changed: !bytes.Equal(change.before, change.after), Diff: unifiedDiff(change.path, change.before, change.after)})
	}
	return Preview{Digest: sourceDigest(joinDocuments(docs)), Files: files}, nil
}

func (g *GitJSON) Save(input SaveInput) (Snapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	workspace, err := gitWorkspace(g.workspace)
	if err != nil {
		return Snapshot{}, err
	}
	if workspace.Dirty {
		return Snapshot{}, ErrGitWorkspaceDirty
	}
	snapshot, docs, _, err := g.loadLocked()
	if err != nil {
		return Snapshot{}, err
	}
	if input.ExpectedRevision != snapshot.Revision {
		return Snapshot{}, ErrRevisionMismatch
	}
	changes, diagnostics, err := g.renderSave(docs, input)
	if err != nil {
		return Snapshot{}, err
	}
	if len(diagnostics) > 0 {
		return Snapshot{}, roundTripError(diagnostics)
	}
	// Re-read immediately before replacing files so a source edit observed
	// after the draft snapshot cannot be overwritten by this process.
	currentDocs, _, err := g.readDocuments()
	if err != nil {
		return Snapshot{}, err
	}
	if sourceDigest(joinDocuments(currentDocs)) != snapshot.Revision {
		return Snapshot{}, ErrRevisionMismatch
	}
	for _, change := range changes {
		if bytes.Equal(change.before, change.after) {
			continue
		}
		path, pathErr := safePath(g.workspace, change.path)
		if pathErr != nil {
			return Snapshot{}, pathErr
		}
		if err := atomicWrite(path, change.after); err != nil {
			return Snapshot{}, fmt.Errorf("write git-json source %s: %w", change.path, err)
		}
	}
	next, _, _, err := g.loadLocked()
	if err != nil {
		return Snapshot{}, err
	}
	g.lastDigest = next.Revision
	return next, nil
}

type sourceDocument struct {
	path, format string
	raw          []byte
	value        map[string]any
}
type renderedDocument struct {
	path          string
	before, after []byte
}

func (g *GitJSON) loadLocked() (Snapshot, []sourceDocument, []string, error) {
	docs, paths, err := g.readDocuments()
	if err != nil {
		return Snapshot{}, nil, nil, err
	}
	baseline := map[string]any{}
	overrides := map[string]map[string]any{}
	diagnostics := []MappingDiagnostic{}
	for _, mapping := range g.profile.Mappings {
		doc, ok := findDocument(docs, mapping.File)
		if !ok {
			diagnostics = append(diagnostics, MappingDiagnostic{Code: "profile_mismatch", Path: mapping.File, Message: "mapping file is not declared"})
			continue
		}
		records, ok := pointerGet(doc.value, mapping.RecordsPath).([]any)
		if !ok {
			diagnostics = append(diagnostics, MappingDiagnostic{Code: "profile_mismatch", Path: mapping.RecordsPath, Message: "records_path must resolve to an array"})
			continue
		}
		for index, item := range records {
			record, ok := item.(map[string]any)
			if !ok {
				diagnostics = append(diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: fmt.Sprintf("%s/%d", mapping.RecordsPath, index), Message: "mapped record must be an object"})
				continue
			}
			if hasConditionalValue(record) {
				diagnostics = append(diagnostics, MappingDiagnostic{Code: "conditional_value", Path: fmt.Sprintf("%s/%d", mapping.RecordsPath, index), Message: "条件值不能安全 round-trip"})
				continue
			}
			adapted, environmentID, ds := mapRecord(mapping, record, index)
			diagnostics = append(diagnostics, ds...)
			if adapted == nil {
				continue
			}
			if mapping.Scope == "environment_override" {
				if environmentID == "" {
					continue
				}
				if overrides[environmentID] == nil {
					overrides[environmentID] = map[string]any{}
				}
				overrides[environmentID][mapping.Collection] = append(recordsOf(overrides[environmentID][mapping.Collection]), adapted)
			} else {
				baseline[mapping.Collection] = append(recordsOf(baseline[mapping.Collection]), adapted)
			}
		}
	}
	if len(diagnostics) > 0 {
		return Snapshot{}, docs, paths, roundTripError(diagnostics)
	}
	digest := sourceDigest(joinDocuments(docs))
	return Snapshot{Revision: digest, Baseline: baseline, EnvironmentOverrides: overrides}, docs, paths, nil
}

func (g *GitJSON) readDocuments() ([]sourceDocument, []string, error) {
	docs := make([]sourceDocument, 0, len(g.profile.Files))
	paths := make([]string, 0, len(g.profile.Files))
	for _, file := range g.profile.Files {
		path, err := safePath(g.workspace, file.Path)
		if err != nil {
			return nil, nil, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("read git-json source %s: %w", file.Path, err)
		}
		value, err := decodeDocument(file.Format, raw)
		if err != nil {
			return nil, nil, fmt.Errorf("parse git-json source %s: %w", file.Path, err)
		}
		docs = append(docs, sourceDocument{path: filepath.ToSlash(normalizePath(file.Path)), format: file.Format, raw: raw, value: value})
		paths = append(paths, filepath.ToSlash(normalizePath(file.Path)))
	}
	sort.Strings(paths)
	return docs, paths, nil
}

func (g *GitJSON) renderSave(docs []sourceDocument, input SaveInput) ([]renderedDocument, []MappingDiagnostic, error) {
	working := cloneDocuments(docs)
	diagnostics := []MappingDiagnostic{}
	for _, mapping := range g.profile.Mappings {
		doc, ok := findDocument(working, mapping.File)
		if !ok {
			diagnostics = append(diagnostics, MappingDiagnostic{Code: "profile_mismatch", Path: mapping.File, Message: "mapping file is not declared"})
			continue
		}
		var desired any
		if mapping.Scope == "environment_override" {
			if input.EnvironmentOverride != nil {
				desired = input.EnvironmentOverride[mapping.Collection]
			}
		} else {
			desired = input.Baseline[mapping.Collection]
		}
		if err := applyMapping(mapping, doc.value, input.EnvironmentID, recordsOf(desired), &diagnostics); err != nil {
			return nil, diagnostics, err
		}
	}
	result := make([]renderedDocument, 0, len(working))
	for _, doc := range working {
		encoded, err := encodeDocument(doc.format, doc.value)
		if err != nil {
			return nil, diagnostics, err
		}
		result = append(result, renderedDocument{path: doc.path, before: docs[len(result)].raw, after: encoded})
	}
	return result, diagnostics, nil
}

func applyMapping(mapping GitJSONMapping, root map[string]any, environmentID string, desired []any, diagnostics *[]MappingDiagnostic) error {
	value := pointerGet(root, mapping.RecordsPath)
	records, ok := value.([]any)
	if !ok {
		return fmt.Errorf("records_path %s must resolve to an array", mapping.RecordsPath)
	}
	desiredByID := map[string]map[string]any{}
	for _, item := range desired {
		record, ok := item.(map[string]any)
		if !ok {
			*diagnostics = append(*diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: mapping.Collection, Message: "desired record is not an object"})
			continue
		}
		id, _ := record["id"].(string)
		fields, _ := record["fields"].(map[string]any)
		if id == "" || fields == nil {
			*diagnostics = append(*diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: mapping.Collection, Message: "desired record lacks id or fields"})
			continue
		}
		desiredByID[id] = fields
	}
	updated := make([]any, 0, len(records)+len(desiredByID))
	for _, item := range records {
		record, ok := item.(map[string]any)
		if !ok {
			updated = append(updated, item)
			continue
		}
		if mapping.Scope == "environment_override" {
			current, _ := pointerGet(record, mapping.EnvironmentIDPath).(string)
			if current != environmentID {
				updated = append(updated, record)
				continue
			}
		}
		id, _ := pointerGet(record, mapping.IDPath).(string)
		fields, exists := desiredByID[id]
		if !exists {
			continue
		}
		for target, spec := range mapping.Fields {
			field, exists := fields[target]
			if !exists {
				*diagnostics = append(*diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: mapping.Collection + "/" + id + "/" + target, Message: "mapped field is absent from desired configuration"})
				continue
			}
			converted, err := reverseTransform(spec.Transform, field)
			if err != nil {
				*diagnostics = append(*diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: mapping.Collection + "/" + id + "/" + target, Message: err.Error()})
				continue
			}
			if !pointerSet(record, spec.Path, converted) {
				return fmt.Errorf("set mapping path %s", spec.Path)
			}
		}
		delete(desiredByID, id)
		updated = append(updated, record)
	}
	newIDs := make([]string, 0, len(desiredByID))
	for id := range desiredByID {
		newIDs = append(newIDs, id)
	}
	sort.Strings(newIDs)
	for _, id := range newIDs {
		fields := desiredByID[id]
		record := map[string]any{}
		if !pointerSet(record, mapping.IDPath, id) {
			return fmt.Errorf("set mapping id path %s", mapping.IDPath)
		}
		if mapping.Scope == "environment_override" && !pointerSet(record, mapping.EnvironmentIDPath, environmentID) {
			return fmt.Errorf("set mapping environment path %s", mapping.EnvironmentIDPath)
		}
		for target, spec := range mapping.Fields {
			field, exists := fields[target]
			if !exists {
				*diagnostics = append(*diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: mapping.Collection + "/" + id + "/" + target, Message: "new record lacks a mapped field"})
				continue
			}
			converted, err := reverseTransform(spec.Transform, field)
			if err != nil {
				*diagnostics = append(*diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: mapping.Collection + "/" + id + "/" + target, Message: err.Error()})
				continue
			}
			_ = pointerSet(record, spec.Path, converted)
		}
		updated = append(updated, record)
	}
	if !pointerSet(root, mapping.RecordsPath, updated) {
		return fmt.Errorf("set records path %s", mapping.RecordsPath)
	}
	return nil
}

func mapRecord(mapping GitJSONMapping, record map[string]any, index int) (map[string]any, string, []MappingDiagnostic) {
	path := fmt.Sprintf("%s/%d", mapping.RecordsPath, index)
	diagnostics := []MappingDiagnostic{}
	id, ok := pointerGet(record, mapping.IDPath).(string)
	if !ok || id == "" {
		return nil, "", []MappingDiagnostic{{Code: "unmapped_value", Path: path, Message: "mapping id_path must resolve to a non-empty string"}}
	}
	fields := map[string]any{}
	for target, spec := range mapping.Fields {
		value, exists := pointerLookup(record, spec.Path)
		if !exists {
			diagnostics = append(diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: path + spec.Path, Message: "mapped field is missing"})
			continue
		}
		converted, err := transform(spec.Transform, value)
		if err != nil {
			diagnostics = append(diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: path + spec.Path, Message: err.Error()})
			continue
		}
		fields[target] = converted
	}
	if len(diagnostics) > 0 {
		return nil, "", diagnostics
	}
	environmentID := ""
	if mapping.Scope == "environment_override" {
		environmentID, _ = pointerGet(record, mapping.EnvironmentIDPath).(string)
		if environmentID == "" {
			diagnostics = append(diagnostics, MappingDiagnostic{Code: "unmapped_value", Path: path, Message: "environment binding requires environment_id_path"})
		}
	}
	return map[string]any{"id": id, "fields": fields}, environmentID, diagnostics
}

func validateProfile(profile GitJSONProfile) error {
	if profile.Version != 1 {
		return errors.New("git-json profile version must be 1")
	}
	files := map[string]struct{}{}
	for _, file := range profile.Files {
		if file.Path == "" || (file.Format != "json" && file.Format != "yaml") {
			return errors.New("git-json profile files require path and json or yaml format")
		}
		files[filepath.ToSlash(normalizePath(file.Path))] = struct{}{}
	}
	if len(files) == 0 || len(profile.Mappings) == 0 {
		return errors.New("git-json profile requires files and mappings")
	}
	for _, m := range profile.Mappings {
		if m.Collection == "" || (m.Scope != "baseline" && m.Scope != "environment_override") || m.RecordsPath == "" || m.IDPath == "" || len(m.Fields) == 0 {
			return fmt.Errorf("invalid git-json mapping %q", m.Name)
		}
		if _, ok := files[filepath.ToSlash(normalizePath(m.File))]; !ok {
			return fmt.Errorf("mapping %q references undeclared file", m.Name)
		}
		if m.Scope == "environment_override" && m.EnvironmentIDPath == "" {
			return fmt.Errorf("mapping %q requires environment_id_path", m.Name)
		}
	}
	return nil
}

func gitWorkspace(workspace string) (GitWorkspace, error) {
	root, err := gitOutput(workspace, "rev-parse", "--show-toplevel")
	if err != nil {
		return GitWorkspace{}, fmt.Errorf("detect git workspace: %w", err)
	}
	branch, err := gitOutput(workspace, "branch", "--show-current")
	if err != nil {
		return GitWorkspace{}, fmt.Errorf("detect git branch: %w", err)
	}
	status, err := gitOutput(workspace, "status", "--porcelain")
	if err != nil {
		return GitWorkspace{}, fmt.Errorf("detect git status: %w", err)
	}
	return GitWorkspace{Root: root, Branch: branch, Dirty: strings.TrimSpace(status) != ""}, nil
}

func gitOutput(workspace string, args ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", workspace}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
func safePath(root, candidate string) (string, error) {
	normalized := normalizePath(candidate)
	if filepath.IsAbs(normalized) || hasWindowsVolumePath(candidate) {
		return "", errors.New("absolute path is not allowed")
	}
	result := filepath.Join(root, normalized)
	relative, err := filepath.Rel(root, result)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes git workspace")
	}
	return result, nil
}

func hasWindowsVolumePath(value string) bool {
	return len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}
func normalizePath(value string) string {
	return filepath.Clean(filepath.FromSlash(strings.ReplaceAll(value, "\\", "/")))
}
func decodeDocument(format string, raw []byte) (map[string]any, error) {
	var value any
	var err error
	if format == "json" {
		err = json.Unmarshal(raw, &value)
	} else {
		err = yaml.Unmarshal(raw, &value)
	}
	if err != nil {
		return nil, err
	}
	result, ok := normalize(value).(map[string]any)
	if !ok {
		return nil, errors.New("source document must be an object")
	}
	return result, nil
}
func encodeDocument(format string, value map[string]any) ([]byte, error) {
	if format == "json" {
		content, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(content, '\n'), nil
	}
	return canonicalYAML(value)
}
func findDocument(docs []sourceDocument, path string) (*sourceDocument, bool) {
	path = filepath.ToSlash(normalizePath(path))
	for i := range docs {
		if docs[i].path == path {
			return &docs[i], true
		}
	}
	return nil, false
}
func cloneDocuments(docs []sourceDocument) []sourceDocument {
	result := make([]sourceDocument, len(docs))
	for i, doc := range docs {
		result[i] = sourceDocument{path: doc.path, format: doc.format, raw: append([]byte{}, doc.raw...), value: cloneMap(doc.value)}
	}
	return result
}
func joinDocuments(docs []sourceDocument) []byte {
	var result bytes.Buffer
	sorted := append([]sourceDocument{}, docs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })
	for _, doc := range sorted {
		result.WriteString(doc.path)
		result.WriteByte(0)
		result.Write(doc.raw)
		result.WriteByte(0)
	}
	return result.Bytes()
}
func recordsOf(value any) []any { records, _ := value.([]any); return records }
func pointerGet(value any, pointer string) any {
	result, _ := pointerLookup(value, pointer)
	return result
}

func pointerLookup(value any, pointer string) (any, bool) {
	if pointer == "" || pointer == "/" {
		return value, true
	}
	current := value
	for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		var exists bool
		current, exists = object[strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")]
		if !exists {
			return nil, false
		}
	}
	return current, true
}
func pointerSet(root map[string]any, pointer string, value any) bool {
	tokens := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	if pointer == "" || len(tokens) == 0 {
		return false
	}
	current := root
	for _, token := range tokens[:len(tokens)-1] {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		next, ok := current[token].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[token] = next
		}
		current = next
	}
	last := strings.ReplaceAll(strings.ReplaceAll(tokens[len(tokens)-1], "~1", "/"), "~0", "~")
	current[last] = value
	return true
}
func transform(name string, value any) (any, error) {
	switch name {
	case "", "identity":
		return value, nil
	case "seconds_to_milliseconds":
		number, ok := value.(float64)
		if !ok {
			return nil, errors.New("seconds_to_milliseconds requires a number")
		}
		return number * 1000, nil
	case "string_to_boolean":
		text, ok := value.(string)
		if !ok {
			return nil, errors.New("string_to_boolean requires a string")
		}
		if text == "true" {
			return true, nil
		}
		if text == "false" {
			return false, nil
		}
		return nil, errors.New("string_to_boolean accepts true or false")
	}
	return nil, fmt.Errorf("unsupported transform %q", name)
}
func reverseTransform(name string, value any) (any, error) {
	switch name {
	case "", "identity":
		return value, nil
	case "seconds_to_milliseconds":
		number, ok := value.(float64)
		if !ok {
			return nil, errors.New("milliseconds value must be a number")
		}
		return number / 1000, nil
	case "string_to_boolean":
		boolean, ok := value.(bool)
		if !ok {
			return nil, errors.New("boolean value is required")
		}
		if boolean {
			return "true", nil
		}
		return "false", nil
	}
	return nil, fmt.Errorf("unsupported transform %q", name)
}
func hasConditionalValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			if normalized == "conditionalvalues" || normalized == "conditionalvalue" {
				return true
			}
			if hasConditionalValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasConditionalValue(child) {
				return true
			}
		}
	}
	return false
}
func roundTripError(diagnostics []MappingDiagnostic) error {
	raw, _ := json.Marshal(diagnostics)
	return fmt.Errorf("%w: %s", ErrRoundTripBlocked, raw)
}

func diagnosticsFromRoundTripError(err error) []MappingDiagnostic {
	if !errors.Is(err, ErrRoundTripBlocked) {
		return nil
	}
	start := strings.Index(err.Error(), "[")
	if start < 0 {
		return nil
	}
	var diagnostics []MappingDiagnostic
	if json.Unmarshal([]byte(err.Error()[start:]), &diagnostics) != nil {
		return nil
	}
	return diagnostics
}
func unifiedDiff(path string, before, after []byte) string {
	if bytes.Equal(before, after) {
		return ""
	}
	return "--- a/" + path + "\n+++ b/" + path + "\n-" + strings.TrimSuffix(string(before), "\n") + "\n+" + strings.TrimSuffix(string(after), "\n") + "\n"
}
