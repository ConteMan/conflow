package packs

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
)

var (
	ErrInvalidReference    = errors.New("invalid pack reference")
	ErrUnknownPack         = errors.New("unknown pack")
	ErrUnknownVersion      = errors.New("unknown pack version")
	ErrSchemaIncompatible  = errors.New("pack schema incompatible")
	ErrInvalidDefinition   = errors.New("invalid pack definition")
	ErrDuplicateDefinition = errors.New("duplicate pack definition")
)

var (
	packNamePattern    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	packVersionPattern = regexp.MustCompile(`^v[1-9][0-9]*$`)
	identifierPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

// Reference preserves the name/version identity of a Pack without changing
// the manifest's opaque pack.id string.
type Reference struct {
	Name    string
	Version string
}

func ParseReference(value string) (Reference, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 || !packNamePattern.MatchString(parts[0]) || !packVersionPattern.MatchString(parts[1]) {
		return Reference{}, fmt.Errorf("%w: %q", ErrInvalidReference, value)
	}
	return Reference{Name: parts[0], Version: parts[1]}, nil
}

func (r Reference) String() string {
	return r.Name + "/" + r.Version
}

type SchemaIncompatibleError struct {
	Requested uint64
	Current   uint64
}

func (e *SchemaIncompatibleError) Error() string {
	return fmt.Sprintf("%v: requested %d, current %d", ErrSchemaIncompatible, e.Requested, e.Current)
}

func (e *SchemaIncompatibleError) Unwrap() error {
	return ErrSchemaIncompatible
}

type Snapshot struct {
	Definitions []Definition
	Revision    uint64
}

// Registry is safe for concurrent readers and registration during startup.
// It only accepts compiled-in declarations; it never loads or executes user
// supplied Pack code.
type Registry struct {
	mu          sync.RWMutex
	definitions map[Reference]Definition
	revision    uint64
}

func NewRegistry(definitions ...Definition) (*Registry, error) {
	registry := &Registry{
		definitions: make(map[Reference]Definition),
		revision:    1,
	}
	for _, definition := range definitions {
		copy, err := validatedCopy(definition)
		if err != nil {
			return nil, err
		}
		reference := Reference{Name: copy.Metadata.Name, Version: copy.Metadata.Version}
		if _, exists := registry.definitions[reference]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateDefinition, reference)
		}
		registry.definitions[reference] = copy
	}
	return registry, nil
}

func MustNewRegistry(definitions ...Definition) *Registry {
	registry, err := NewRegistry(definitions...)
	if err != nil {
		panic(err)
	}
	return registry
}

func (r *Registry) Register(definition Definition) error {
	copy, err := validatedCopy(definition)
	if err != nil {
		return err
	}
	reference := Reference{Name: copy.Metadata.Name, Version: copy.Metadata.Version}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.definitions[reference]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateDefinition, reference)
	}
	r.definitions[reference] = copy
	r.revision++
	return nil
}

func (r *Registry) List() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	definitions := make([]Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		definitions = append(definitions, cloneDefinition(definition))
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].Metadata.Name == definitions[j].Metadata.Name {
			return definitions[i].Metadata.Version < definitions[j].Metadata.Version
		}
		return definitions[i].Metadata.Name < definitions[j].Metadata.Name
	})
	return Snapshot{Definitions: definitions, Revision: r.revision}
}

func (r *Registry) Get(name, version string) (Definition, uint64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reference := Reference{Name: name, Version: version}
	definition, exists := r.definitions[reference]
	if exists {
		return cloneDefinition(definition), r.revision, nil
	}
	for registered := range r.definitions {
		if registered.Name == name {
			return Definition{}, r.revision, fmt.Errorf("%w: %s", ErrUnknownVersion, reference)
		}
	}
	return Definition{}, r.revision, fmt.Errorf("%w: %s", ErrUnknownPack, name)
}

func (r *Registry) Resolve(reference string) (Definition, uint64, error) {
	parsed, err := ParseReference(reference)
	if err != nil {
		return Definition{}, 0, err
	}
	return r.Get(parsed.Name, parsed.Version)
}

func (r *Registry) Schema(name, version string, requestedVersion *uint64) (Schema, uint64, error) {
	definition, revision, err := r.Get(name, version)
	if err != nil {
		return Schema{}, revision, err
	}
	if requestedVersion != nil && *requestedVersion != definition.Schema.Version {
		return Schema{}, revision, &SchemaIncompatibleError{Requested: *requestedVersion, Current: definition.Schema.Version}
	}
	return cloneSchema(definition.Schema), revision, nil
}

func validatedCopy(definition Definition) (Definition, error) {
	if !packNamePattern.MatchString(definition.Metadata.Name) {
		return Definition{}, fmt.Errorf("%w: metadata.name", ErrInvalidDefinition)
	}
	if !packVersionPattern.MatchString(definition.Metadata.Version) {
		return Definition{}, fmt.Errorf("%w: metadata.version", ErrInvalidDefinition)
	}
	if strings.TrimSpace(definition.Metadata.Description) == "" || definition.Schema.Version == 0 {
		return Definition{}, fmt.Errorf("%w: description and schema version are required", ErrInvalidDefinition)
	}

	schemas := make(map[string]EntitySchema, len(definition.Schema.Entities))
	for _, entity := range definition.Schema.Entities {
		if !identifierPattern.MatchString(entity.Name) {
			return Definition{}, fmt.Errorf("%w: schema entity %q", ErrInvalidDefinition, entity.Name)
		}
		if _, exists := schemas[entity.Name]; exists {
			return Definition{}, fmt.Errorf("%w: duplicated schema entity %q", ErrInvalidDefinition, entity.Name)
		}
		fields := make(map[string]struct{}, len(entity.Fields))
		for _, field := range entity.Fields {
			if !identifierPattern.MatchString(field.Name) || !validFieldType(field.Type) || field.Default == nil || !json.Valid(field.Default) {
				return Definition{}, fmt.Errorf("%w: field %q", ErrInvalidDefinition, field.Name)
			}
			if !matchesFieldType(field.Default, field.Type) {
				return Definition{}, fmt.Errorf("%w: field default type %q", ErrInvalidDefinition, field.Name)
			}
			if field.Sensitivity != SensitivityPublic && field.Sensitivity != SensitivitySensitive {
				return Definition{}, fmt.Errorf("%w: field sensitivity %q", ErrInvalidDefinition, field.Name)
			}
			if strings.TrimSpace(field.UI.Label) == "" || strings.TrimSpace(field.UI.Control) == "" || field.UI.Order < 0 {
				return Definition{}, fmt.Errorf("%w: field UI %q", ErrInvalidDefinition, field.Name)
			}
			if _, exists := fields[field.Name]; exists {
				return Definition{}, fmt.Errorf("%w: duplicated field %q", ErrInvalidDefinition, field.Name)
			}
			fields[field.Name] = struct{}{}
			for _, value := range field.Validation.Enum {
				if !json.Valid(value) || !matchesFieldType(value, field.Type) {
					return Definition{}, fmt.Errorf("%w: field enum %q", ErrInvalidDefinition, field.Name)
				}
			}
			if (field.Validation.MinLength != nil || field.Validation.MaxLength != nil) && field.Type != FieldTypeString && field.Type != FieldTypeReference {
				return Definition{}, fmt.Errorf("%w: field length constraints %q", ErrInvalidDefinition, field.Name)
			}
			if field.Validation.MinLength != nil && *field.Validation.MinLength < 0 || field.Validation.MaxLength != nil && *field.Validation.MaxLength < 0 {
				return Definition{}, fmt.Errorf("%w: field length range %q", ErrInvalidDefinition, field.Name)
			}
			if field.Validation.MinLength != nil && field.Validation.MaxLength != nil && *field.Validation.MinLength > *field.Validation.MaxLength {
				return Definition{}, fmt.Errorf("%w: field length range %q", ErrInvalidDefinition, field.Name)
			}
			if (field.Validation.Minimum != nil || field.Validation.Maximum != nil) && field.Type != FieldTypeInteger && field.Type != FieldTypeNumber {
				return Definition{}, fmt.Errorf("%w: field numeric constraints %q", ErrInvalidDefinition, field.Name)
			}
			if field.Validation.Minimum != nil && (math.IsNaN(*field.Validation.Minimum) || math.IsInf(*field.Validation.Minimum, 0)) || field.Validation.Maximum != nil && (math.IsNaN(*field.Validation.Maximum) || math.IsInf(*field.Validation.Maximum, 0)) {
				return Definition{}, fmt.Errorf("%w: field numeric range %q", ErrInvalidDefinition, field.Name)
			}
			if field.Validation.Minimum != nil && field.Validation.Maximum != nil && *field.Validation.Minimum > *field.Validation.Maximum {
				return Definition{}, fmt.Errorf("%w: field numeric range %q", ErrInvalidDefinition, field.Name)
			}
		}
		schemas[entity.Name] = entity
	}

	entities := make(map[string]struct{}, len(definition.Metadata.EntityTypes))
	for _, entity := range definition.Metadata.EntityTypes {
		schema, exists := schemas[entity.Name]
		if !exists || !identifierPattern.MatchString(entity.Name) || strings.TrimSpace(entity.Label) == "" || strings.TrimSpace(entity.Description) == "" {
			return Definition{}, fmt.Errorf("%w: entity metadata %q", ErrInvalidDefinition, entity.Name)
		}
		if strings.TrimSpace(entity.IDRule.Pattern) == "" || entity.IDRule.MinLength < 1 || entity.IDRule.MaxLength < entity.IDRule.MinLength {
			return Definition{}, fmt.Errorf("%w: ID rule %q", ErrInvalidDefinition, entity.Name)
		}
		if _, err := regexp.Compile(entity.IDRule.Pattern); err != nil {
			return Definition{}, fmt.Errorf("%w: ID pattern %q", ErrInvalidDefinition, entity.Name)
		}
		if entity.DeletionPolicy != DeletionPolicyRestrict && entity.DeletionPolicy != DeletionPolicyCascade && entity.DeletionPolicy != DeletionPolicyAllow {
			return Definition{}, fmt.Errorf("%w: deletion policy %q", ErrInvalidDefinition, entity.Name)
		}
		availableFields := make(map[string]struct{}, len(schema.Fields))
		for _, field := range schema.Fields {
			availableFields[field.Name] = struct{}{}
		}
		for _, field := range entity.EnvironmentOverrideFields {
			if _, exists := availableFields[field]; !exists {
				return Definition{}, fmt.Errorf("%w: environment override field %q", ErrInvalidDefinition, field)
			}
		}
		if _, exists := entities[entity.Name]; exists {
			return Definition{}, fmt.Errorf("%w: duplicated entity metadata %q", ErrInvalidDefinition, entity.Name)
		}
		entities[entity.Name] = struct{}{}
	}
	if len(entities) != len(schemas) {
		return Definition{}, fmt.Errorf("%w: metadata and schema entities differ", ErrInvalidDefinition)
	}

	for _, migration := range definition.Schema.Migrations {
		if migration.FromVersion == 0 || migration.ToVersion == 0 || migration.FromVersion >= migration.ToVersion || migration.ToVersion > definition.Schema.Version || strings.TrimSpace(migration.Description) == "" {
			return Definition{}, fmt.Errorf("%w: schema migration", ErrInvalidDefinition)
		}
	}
	return cloneDefinition(definition), nil
}

func matchesFieldType(raw json.RawMessage, fieldType FieldType) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return false
	}
	switch fieldType {
	case FieldTypeString, FieldTypeReference:
		_, ok := value.(string)
		return ok
	case FieldTypeBoolean:
		_, ok := value.(bool)
		return ok
	case FieldTypeInteger:
		number, ok := value.(float64)
		return ok && math.Trunc(number) == number
	case FieldTypeNumber:
		_, ok := value.(float64)
		return ok
	case FieldTypeObject:
		_, ok := value.(map[string]any)
		return ok
	case FieldTypeArray:
		_, ok := value.([]any)
		return ok
	default:
		return false
	}
}

func validFieldType(fieldType FieldType) bool {
	switch fieldType {
	case FieldTypeString, FieldTypeBoolean, FieldTypeInteger, FieldTypeNumber, FieldTypeObject, FieldTypeArray, FieldTypeReference:
		return true
	default:
		return false
	}
}

func cloneDefinition(definition Definition) Definition {
	clone := definition
	clone.Metadata.Capabilities = append([]string{}, definition.Metadata.Capabilities...)
	clone.Metadata.EntityTypes = make([]EntityMetadata, len(definition.Metadata.EntityTypes))
	copy(clone.Metadata.EntityTypes, definition.Metadata.EntityTypes)
	for index := range clone.Metadata.EntityTypes {
		clone.Metadata.EntityTypes[index].EnvironmentOverrideFields = append([]string{}, definition.Metadata.EntityTypes[index].EnvironmentOverrideFields...)
	}
	clone.Schema = cloneSchema(definition.Schema)
	return clone
}

func cloneSchema(schema Schema) Schema {
	clone := schema
	clone.Entities = make([]EntitySchema, len(schema.Entities))
	copy(clone.Entities, schema.Entities)
	for entityIndex := range clone.Entities {
		clone.Entities[entityIndex].Fields = make([]FieldSchema, len(schema.Entities[entityIndex].Fields))
		copy(clone.Entities[entityIndex].Fields, schema.Entities[entityIndex].Fields)
		for fieldIndex := range clone.Entities[entityIndex].Fields {
			source := schema.Entities[entityIndex].Fields[fieldIndex]
			clone.Entities[entityIndex].Fields[fieldIndex].Default = append(json.RawMessage(nil), source.Default...)
			clone.Entities[entityIndex].Fields[fieldIndex].Validation.Enum = cloneRawMessages(source.Validation.Enum)
			clone.Entities[entityIndex].Fields[fieldIndex].Validation.MinLength = cloneInt(source.Validation.MinLength)
			clone.Entities[entityIndex].Fields[fieldIndex].Validation.MaxLength = cloneInt(source.Validation.MaxLength)
			clone.Entities[entityIndex].Fields[fieldIndex].Validation.Minimum = cloneFloat(source.Validation.Minimum)
			clone.Entities[entityIndex].Fields[fieldIndex].Validation.Maximum = cloneFloat(source.Validation.Maximum)
		}
	}
	clone.Migrations = make([]SchemaMigration, len(schema.Migrations))
	copy(clone.Migrations, schema.Migrations)
	return clone
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	clone := make([]json.RawMessage, len(values))
	for index, value := range values {
		clone[index] = append(json.RawMessage(nil), value...)
	}
	return clone
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
