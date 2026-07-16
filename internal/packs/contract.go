package packs

import (
	"context"
	"encoding/json"
)

// Definition is the immutable, declarative contract for one Pack version.
// It contains no executable rules; the runtime extension points below are
// intentionally separate from the data exposed to clients.
type Definition struct {
	Metadata Metadata `json:"metadata"`
	Schema   Schema   `json:"schema"`
}

type Metadata struct {
	Name         string           `json:"name"`
	Version      string           `json:"version"`
	Description  string           `json:"description"`
	Capabilities []string         `json:"capabilities"`
	EntityTypes  []EntityMetadata `json:"entity_types"`
}

type EntityMetadata struct {
	Name string `json:"name"`
	// Collection is the configuration field that stores the entity records. It
	// is intentionally internal until the public Pack metadata contract gains a
	// collection member.
	Collection                string         `json:"-"`
	Label                     string         `json:"label"`
	Description               string         `json:"description"`
	IDRule                    IDRule         `json:"id_rule"`
	DeletionPolicy            DeletionPolicy `json:"deletion_policy"`
	EnvironmentOverrideFields []string       `json:"environment_override_fields"`
}

type IDRule struct {
	Pattern   string `json:"pattern"`
	MinLength int    `json:"min_length"`
	MaxLength int    `json:"max_length"`
}

type DeletionPolicy string

const (
	DeletionPolicyRestrict DeletionPolicy = "restrict"
	DeletionPolicyCascade  DeletionPolicy = "cascade"
	DeletionPolicyAllow    DeletionPolicy = "allow"
)

type Schema struct {
	Version    uint64            `json:"version"`
	Entities   []EntitySchema    `json:"entities"`
	Migrations []SchemaMigration `json:"migrations"`
}

type SchemaMigration struct {
	FromVersion uint64 `json:"from_version"`
	ToVersion   uint64 `json:"to_version"`
	Description string `json:"description"`
}

type EntitySchema struct {
	Name   string        `json:"name"`
	Fields []FieldSchema `json:"fields"`
}

type FieldSchema struct {
	Name        string          `json:"name"`
	Type        FieldType       `json:"type"`
	Required    bool            `json:"required"`
	Nullable    bool            `json:"nullable"`
	Default     json.RawMessage `json:"default"`
	Sensitivity Sensitivity     `json:"sensitivity"`
	UI          FieldUI         `json:"ui"`
	Validation  FieldValidation `json:"validation"`
}

type FieldType string

const (
	FieldTypeString  FieldType = "string"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeInteger FieldType = "integer"
	FieldTypeNumber  FieldType = "number"
	FieldTypeObject  FieldType = "object"
	FieldTypeArray   FieldType = "array"
	// FieldTypeAny accepts every non-null JSON value. Domain validators retain
	// responsibility for narrowing the allowed value based on sibling fields.
	FieldTypeAny       FieldType = "any"
	FieldTypeReference FieldType = "reference"
)

type Sensitivity string

const (
	SensitivityPublic    Sensitivity = "public"
	SensitivitySensitive Sensitivity = "sensitive"
)

type FieldUI struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Control     string `json:"control"`
	Group       string `json:"group"`
	Order       int    `json:"order"`
	Placeholder string `json:"placeholder,omitempty"`
}

type FieldValidation struct {
	Enum      []json.RawMessage `json:"enum"`
	MinLength *int              `json:"min_length,omitempty"`
	MaxLength *int              `json:"max_length,omitempty"`
	Minimum   *float64          `json:"minimum,omitempty"`
	Maximum   *float64          `json:"maximum,omitempty"`
}

// Document is the Pack-neutral representation passed between future draft,
// validation, compilation, and diff services. Source and provider adapters do
// not appear in this contract.
type Document struct {
	SchemaVersion        uint64                       `json:"schema_version"`
	Entities             map[string][]json.RawMessage `json:"entities"`
	EnvironmentOverrides map[string]map[string]any    `json:"environment_overrides"`
}

type Compiler interface {
	Compile(context.Context, Document) (CompileResult, error)
}

type Validator interface {
	Validate(context.Context, Document) ([]Diagnostic, error)
}

type SemanticDiffer interface {
	Diff(context.Context, Document, Document) (SemanticDiff, error)
}

type SchemaMigrator interface {
	Migrate(context.Context, Document, uint64) (Document, error)
}

type CompileResult struct {
	Values map[string]json.RawMessage
}

type Diagnostic struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SemanticDiff struct {
	Changes []SemanticChange `json:"changes"`
}

type SemanticChange struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}
