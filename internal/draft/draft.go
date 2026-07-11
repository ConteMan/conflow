// Package draft owns the targeted replacement semantics introduced by Spec 004.
package draft

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	ScopeBaseline            = "baseline"
	ScopeEnvironmentOverride = "environment_override"
)

var (
	ErrEnvironmentNotFound = errors.New("draft environment not found")
	ErrInvalidScope        = errors.New("invalid draft write scope")
)

type Environment struct {
	ID   string `json:"environment_id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type Field struct {
	Path                       string
	Type                       string
	Nullable                   bool
	EnvironmentOverrideAllowed bool
	Required                   bool
	Default                    any
	Enum                       []any
	MinLength                  *int
	MaxLength                  *int
	Minimum                    *float64
	Maximum                    *float64
}

type Schema struct {
	PackRef  string
	Defaults map[string]any
	Fields   []Field
}

type SourceSnapshot struct {
	Revision             string
	Baseline             map[string]any
	EnvironmentOverrides map[string]map[string]any
}

type State struct {
	Revision             uint64
	Baseline             map[string]any
	BaselinePresent      bool
	EnvironmentOverrides map[string]map[string]any
}

type ConfigurationPresence struct {
	Present bool           `json:"present"`
	Value   map[string]any `json:"value,omitempty"`
}

// MarshalJSON preserves a present null-free empty object while omitting value
// when the replacement is missing.
func (p ConfigurationPresence) MarshalJSON() ([]byte, error) {
	if !p.Present {
		return []byte(`{"present":false}`), nil
	}
	return json.Marshal(struct {
		Present bool           `json:"present"`
		Value   map[string]any `json:"value"`
	}{Present: true, Value: p.Value})
}

type FieldValuePresence struct {
	Present bool `json:"present"`
	Value   any  `json:"value,omitempty"`
}

func (p FieldValuePresence) MarshalJSON() ([]byte, error) {
	if !p.Present {
		return []byte(`{"present":false}`), nil
	}
	return json.Marshal(struct {
		Present bool `json:"present"`
		Value   any  `json:"value"`
	}{Present: true, Value: p.Value})
}

type LayerState struct {
	Source   ConfigurationPresence `json:"source"`
	Draft    ConfigurationPresence `json:"draft"`
	Resolved ConfigurationPresence `json:"resolved"`
	Dirty    bool                  `json:"dirty"`
}

type FieldState struct {
	Path                       string             `json:"path"`
	PackDefault                FieldValuePresence `json:"pack_default"`
	Baseline                   FieldValuePresence `json:"baseline"`
	DraftBaseline              FieldValuePresence `json:"draft_baseline"`
	EnvironmentOverride        FieldValuePresence `json:"environment_override"`
	DraftEnvironmentOverride   FieldValuePresence `json:"draft_environment_override"`
	Effective                  FieldValuePresence `json:"effective"`
	Origin                     string             `json:"origin"`
	EnvironmentOverrideAllowed bool               `json:"environment_override_allowed"`
	IsEnvironmentOverridden    bool               `json:"is_environment_overridden"`
	SourceRevision             string             `json:"source_revision"`
	Nullable                   bool               `json:"nullable"`
}

type View struct {
	EnvironmentID        string         `json:"environment_id"`
	PackRef              string         `json:"pack_ref"`
	SourceRevision       string         `json:"source_revision"`
	Dirty                bool           `json:"dirty"`
	DirtyScopes          []string       `json:"dirty_scopes"`
	Baseline             LayerState     `json:"baseline"`
	EnvironmentOverride  LayerState     `json:"environment_override"`
	Effective            map[string]any `json:"effective"`
	FieldStates          []FieldState   `json:"field_states"`
	AffectedEnvironments []Environment  `json:"affected_environments"`
}

type StructuralError struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Scope   string `json:"scope"`
	Message string `json:"message"`
}

func ValidScope(scope string) bool {
	return scope == ScopeBaseline || scope == ScopeEnvironmentOverride
}

func BuildView(schema Schema, environments []Environment, source SourceSnapshot, state State, environmentID string) (View, error) {
	if !slices.ContainsFunc(environments, func(environment Environment) bool { return environment.ID == environmentID }) {
		return View{}, ErrEnvironmentNotFound
	}
	if source.Revision == "" {
		return View{}, errors.New("source revision is required")
	}
	fields := slices.Clone(schema.Fields)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
	baselineDraft := ConfigurationPresence{Present: state.BaselinePresent, Value: cloneMap(state.Baseline)}
	baselineSource := ConfigurationPresence{Present: source.Baseline != nil, Value: cloneMap(source.Baseline)}
	baselineResolved := baselineSource
	if baselineDraft.Present {
		baselineResolved = baselineDraft
	}
	environmentSourceValue, environmentSourcePresent := source.EnvironmentOverrides[environmentID]
	environmentDraftValue, environmentDraftPresent := state.EnvironmentOverrides[environmentID]
	environmentSource := ConfigurationPresence{Present: environmentSourcePresent, Value: cloneMap(environmentSourceValue)}
	environmentDraft := ConfigurationPresence{Present: environmentDraftPresent, Value: cloneMap(environmentDraftValue)}
	environmentResolved := environmentSource
	if environmentDraft.Present {
		environmentResolved = environmentDraft
	}
	dirtyScopes := make([]string, 0, 2)
	if baselineDraft.Present {
		dirtyScopes = append(dirtyScopes, ScopeBaseline)
	}
	if environmentDraft.Present {
		dirtyScopes = append(dirtyScopes, ScopeEnvironmentOverride)
	}
	view := View{
		EnvironmentID: environmentID, PackRef: schema.PackRef, SourceRevision: source.Revision,
		Dirty: len(dirtyScopes) > 0, DirtyScopes: dirtyScopes,
		Baseline:             LayerState{Source: baselineSource, Draft: baselineDraft, Resolved: baselineResolved, Dirty: baselineDraft.Present},
		EnvironmentOverride:  LayerState{Source: environmentSource, Draft: environmentDraft, Resolved: environmentResolved, Dirty: environmentDraft.Present},
		Effective:            merge(schema.Defaults, baselineResolved.Value, environmentResolved.Value),
		FieldStates:          make([]FieldState, 0, len(fields)),
		AffectedEnvironments: affected(schema, environments, source, state, environmentID),
	}
	for _, field := range fields {
		packDefault := valueAt(schema.Defaults, field.Path)
		baseline := valueAt(source.Baseline, field.Path)
		draftBaseline := valueAt(state.Baseline, field.Path)
		environmentOverride := valueAt(environmentSourceValue, field.Path)
		draftEnvironmentOverride := valueAt(environmentDraftValue, field.Path)
		effective := valueAt(view.Effective, field.Path)
		origin := "pack_default"
		isEnvironmentOverridden := false
		if baselineResolved.Present {
			if valueAt(baselineResolved.Value, field.Path).Present {
				if baselineDraft.Present {
					origin = "draft_baseline"
				} else {
					origin = "baseline"
				}
			}
		}
		if environmentResolved.Present && valueAt(environmentResolved.Value, field.Path).Present {
			isEnvironmentOverridden = true
			if environmentDraft.Present {
				origin = "draft_environment_override"
			} else {
				origin = "environment_override"
			}
		}
		view.FieldStates = append(view.FieldStates, FieldState{Path: field.Path, PackDefault: packDefault, Baseline: baseline, DraftBaseline: draftBaseline, EnvironmentOverride: environmentOverride, DraftEnvironmentOverride: draftEnvironmentOverride, Effective: effective, Origin: origin, EnvironmentOverrideAllowed: field.EnvironmentOverrideAllowed, IsEnvironmentOverridden: isEnvironmentOverridden, SourceRevision: source.Revision, Nullable: field.Nullable})
	}
	return view, nil
}

func ValidateReplacement(schema Schema, scope string, raw json.RawMessage) (map[string]any, []StructuralError) {
	var configuration any
	if err := json.Unmarshal(raw, &configuration); err != nil {
		return nil, []StructuralError{{Code: "invalid_config_shape", Path: "", Scope: scope, Message: "configuration must be valid JSON"}}
	}
	replacement, ok := configuration.(map[string]any)
	if !ok {
		return nil, []StructuralError{{Code: "invalid_config_shape", Path: "", Scope: scope, Message: "configuration must be an object"}}
	}
	known := make(map[string]Field, len(schema.Fields))
	for _, field := range schema.Fields {
		known[field.Path] = field
	}
	var result []StructuralError
	walkConfiguration(replacement, "", known, func(path string, value any) {
		field, exists := known[path]
		if !exists {
			result = append(result, StructuralError{Code: "invalid_config_shape", Path: path, Scope: scope, Message: "configuration field is not declared by the Pack"})
			return
		}
		if scope == ScopeEnvironmentOverride && !field.EnvironmentOverrideAllowed {
			result = append(result, StructuralError{Code: "environment_override_forbidden", Path: path, Scope: scope, Message: "field cannot be overridden by an environment"})
		}
		if value == nil {
			if !field.Nullable {
				result = append(result, StructuralError{Code: "explicit_null_forbidden", Path: path, Scope: scope, Message: "field does not allow explicit null"})
			}
			return
		}
		if !matchesType(value, field.Type) {
			result = append(result, StructuralError{Code: "field_type_mismatch", Path: path, Scope: scope, Message: "field value has the wrong JSON type"})
			return
		}
		if !allowsValue(value, field) {
			result = append(result, StructuralError{Code: "value_not_allowed", Path: path, Scope: scope, Message: "field value violates Pack constraints"})
		}
	})
	for _, field := range schema.Fields {
		if field.Required && !valueAt(replacement, field.Path).Present {
			// Required describes a full Pack configuration. Targeted replacements may
			// omit it, so defaults make the effective document complete.
			continue
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return cmp.Or(strings.Compare(result[i].Scope, result[j].Scope), strings.Compare(result[i].Path, result[j].Path), strings.Compare(result[i].Code, result[j].Code)) < 0
	})
	return replacement, result
}

func affected(schema Schema, environments []Environment, source SourceSnapshot, state State, selected string) []Environment {
	if !state.BaselinePresent {
		if _, exists := state.EnvironmentOverrides[selected]; !exists {
			return []Environment{}
		}
	}
	without := cloneState(state)
	without.BaselinePresent = false
	without.Baseline = nil
	delete(without.EnvironmentOverrides, selected)
	result := make([]Environment, 0, len(environments))
	for _, environment := range environments {
		if !maps.EqualFunc(effectiveFor(schema, source, state, environment.ID), effectiveFor(schema, source, without, environment.ID), func(left, right any) bool { return jsonEqual(left, right) }) {
			result = append(result, environment)
		}
	}
	return result
}

func effectiveFor(schema Schema, source SourceSnapshot, state State, environmentID string) map[string]any {
	baseline := source.Baseline
	if state.BaselinePresent {
		baseline = state.Baseline
	}
	override := source.EnvironmentOverrides[environmentID]
	if draft, exists := state.EnvironmentOverrides[environmentID]; exists {
		override = draft
	}
	return merge(schema.Defaults, baseline, override)
}

func merge(layers ...map[string]any) map[string]any {
	result := map[string]any{}
	for _, layer := range layers {
		mergeInto(result, layer)
	}
	return result
}

func mergeInto(target, layer map[string]any) {
	for key, value := range layer {
		if object, ok := value.(map[string]any); ok {
			if existing, ok := target[key].(map[string]any); ok {
				mergeInto(existing, object)
				continue
			}
			target[key] = cloneMap(object)
			continue
		}
		target[key] = clone(value)
	}
}

func valueAt(root map[string]any, pointer string) FieldValuePresence {
	if root == nil {
		return FieldValuePresence{}
	}
	value := any(root)
	for _, token := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		object, ok := value.(map[string]any)
		if !ok {
			return FieldValuePresence{}
		}
		var exists bool
		value, exists = object[unescape(token)]
		if !exists {
			return FieldValuePresence{}
		}
	}
	return FieldValuePresence{Present: true, Value: clone(value)}
}

func walkConfiguration(value any, path string, known map[string]Field, visit func(string, any)) {
	if path != "" {
		if _, exists := known[path]; exists {
			visit(path, value)
			return
		}
	}
	if object, ok := value.(map[string]any); ok {
		if len(object) == 0 {
			return
		}
		for key, child := range object {
			walkConfiguration(child, path+"/"+escape(key), known, visit)
		}
		return
	}
	visit(path, value)
}

func escape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}
func unescape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~1", "/"), "~0", "~")
}

func matchesType(value any, fieldType string) bool {
	switch fieldType {
	case "string", "reference":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == float64(int64(number))
	case "number":
		_, ok := value.(float64)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	default:
		return false
	}
}

func allowsValue(value any, field Field) bool {
	if len(field.Enum) > 0 && !slices.ContainsFunc(field.Enum, func(allowed any) bool { return jsonEqual(allowed, value) }) {
		return false
	}
	if text, ok := value.(string); ok {
		length := len([]rune(text))
		if field.MinLength != nil && length < *field.MinLength {
			return false
		}
		if field.MaxLength != nil && length > *field.MaxLength {
			return false
		}
	}
	if number, ok := value.(float64); ok {
		if field.Minimum != nil && number < *field.Minimum {
			return false
		}
		if field.Maximum != nil && number > *field.Maximum {
			return false
		}
	}
	return true
}

func cloneState(state State) State {
	return State{Revision: state.Revision, Baseline: cloneMap(state.Baseline), BaselinePresent: state.BaselinePresent, EnvironmentOverrides: cloneOverrides(state.EnvironmentOverrides)}
}
func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, child := range value {
		cloned[key] = clone(child)
	}
	return cloned
}
func cloneOverrides(value map[string]map[string]any) map[string]map[string]any {
	cloned := make(map[string]map[string]any, len(value))
	for key, child := range value {
		cloned[key] = cloneMap(child)
	}
	return cloned
}
func clone(value any) any {
	raw, _ := json.Marshal(value)
	var cloned any
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}
func jsonEqual(left, right any) bool {
	leftRaw, _ := json.Marshal(left)
	rightRaw, _ := json.Marshal(right)
	return bytes.Equal(leftRaw, rightRaw)
}
func FormatRevision(revision uint64) string { return strconv.Quote(strconv.FormatUint(revision, 10)) }
func ParseRevision(etag string) (uint64, error) {
	if len(etag) < 3 || etag[0] != '"' || etag[len(etag)-1] != '"' {
		return 0, errors.New("invalid draft revision ETag")
	}
	revision, err := strconv.ParseUint(etag[1:len(etag)-1], 10, 64)
	if err != nil || revision == 0 {
		return 0, errors.New("invalid draft revision ETag")
	}
	return revision, nil
}
func EnsureSchema(schema Schema) error {
	if schema.PackRef == "" {
		return fmt.Errorf("draft schema pack ref is required")
	}
	return nil
}
