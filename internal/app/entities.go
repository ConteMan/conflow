package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/entities"
	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/releasedbaseline"
)

var (
	ErrEntityNotFound    = errors.New("entity not found")
	ErrEntityTypeInvalid = errors.New("entity type is not declared by the Pack")
	ErrEntityIDInvalid   = errors.New("entity ID is invalid")
	ErrEntityIDImmutable = errors.New("entity ID is immutable")
	ErrEntityExists      = errors.New("entity already exists")
)

// EntityRecord is the Pack-neutral record representation exposed by the
// entity resource. Its runtime collection shape is owned by entities.Record.
type EntityRecord = entities.Record

type EntityPresence struct {
	Present bool          `json:"present"`
	Value   *EntityRecord `json:"value,omitempty"`
}

func (p EntityPresence) MarshalJSON() ([]byte, error) {
	if !p.Present {
		return []byte(`{"present":false}`), nil
	}
	return json.Marshal(struct {
		Present bool          `json:"present"`
		Value   *EntityRecord `json:"value"`
	}{Present: true, Value: p.Value})
}

type EntityView struct {
	EntityRef      string         `json:"entity_ref"`
	EntityType     string         `json:"entity_type"`
	EntityID       string         `json:"entity_id"`
	Source         EntityPresence `json:"source"`
	Draft          EntityPresence `json:"draft"`
	Resolved       EntityPresence `json:"resolved"`
	Effective      EntityPresence `json:"effective"`
	Origin         string         `json:"origin"`
	SourceRevision string         `json:"source_revision"`
	ChangeStatus   string         `json:"change_status"`
}

type EntityReference struct {
	EntityRef  string `json:"entity_ref"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Path       string `json:"path"`
}

type EntityReferences struct {
	EntityRef    string            `json:"entity_ref"`
	ReferencedBy []EntityReference `json:"referenced_by"`
}

type EntityReferencedError struct {
	Revision   uint64
	References []EntityReference
}

func (e *EntityReferencedError) Error() string { return "entity is still referenced" }

type EntityMutation struct {
	ExpectedRevision       uint64
	ExpectedSourceRevision string
	Scope                  string
	EntityType             string
	EntityID               string
	Entity                 *EntityRecord
	Action                 string
}

func (s *Service) ListEntities(_ context.Context, environmentID, entityType string) ([]EntityView, uint64, error) {
	view, revision, definition, err := s.entityContext(environmentID)
	if err != nil {
		return nil, 0, err
	}
	if entityType != "" {
		if _, ok := findEntityMetadata(definition, entityType); !ok {
			return nil, 0, ErrEntityTypeInvalid
		}
	}
	baseline, baselineFound := s.releasedBaseline(environmentID)
	entities := make([]EntityView, 0)
	for _, metadata := range definition.Metadata.EntityTypes {
		if metadata.Collection == "" || (entityType != "" && metadata.Name != entityType) {
			continue
		}
		for _, record := range records(view.Effective, metadata.Collection) {
			entity, err := entityView(definition, view, metadata, record.ID, baseline, baselineFound)
			if err != nil {
				return nil, 0, err
			}
			entities = append(entities, entity)
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].EntityType == entities[j].EntityType {
			return entities[i].EntityID < entities[j].EntityID
		}
		return entities[i].EntityType < entities[j].EntityType
	})
	return entities, revision, nil
}

func (s *Service) GetEntity(_ context.Context, environmentID, entityType, entityID string) (EntityView, uint64, error) {
	view, revision, definition, err := s.entityContext(environmentID)
	if err != nil {
		return EntityView{}, 0, err
	}
	metadata, ok := findEntityMetadata(definition, entityType)
	if !ok || metadata.Collection == "" {
		return EntityView{}, 0, ErrEntityTypeInvalid
	}
	baseline, baselineFound := s.releasedBaseline(environmentID)
	entity, err := entityView(definition, view, metadata, entityID, baseline, baselineFound)
	return entity, revision, err
}

func (s *Service) GetEntityReferences(_ context.Context, environmentID, entityType, entityID string) (EntityReferences, uint64, error) {
	view, revision, definition, err := s.entityContext(environmentID)
	if err != nil {
		return EntityReferences{}, 0, err
	}
	metadata, ok := findEntityMetadata(definition, entityType)
	if !ok || metadata.Collection == "" {
		return EntityReferences{}, 0, ErrEntityTypeInvalid
	}
	if _, ok := findRecord(records(view.Effective, metadata.Collection), entityID); !ok {
		return EntityReferences{}, 0, ErrEntityNotFound
	}
	return EntityReferences{EntityRef: entityRef(view.PackRef, entityType, entityID), ReferencedBy: references(definition, view.PackRef, view.Effective, entityType, entityID)}, revision, nil
}

func (s *Service) MutateEntity(_ context.Context, environmentID string, mutation EntityMutation) (EntityView, uint64, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return EntityView{}, 0, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return EntityView{}, 0, err
	}
	definition, _, err := s.packRegistry.Resolve(snapshot.Manifest.Pack.ID)
	if err != nil {
		return EntityView{}, 0, err
	}
	var deleted EntityView
	currentEffective := map[string]any(nil)
	var metadata packs.EntityMetadata
	draftMutation := draft.Mutation{
		ExpectedRevision: mutation.ExpectedRevision, ExpectedSourceRevision: mutation.ExpectedSourceRevision,
		Scope: mutation.Scope, Action: "put",
		Prepare: func(current draft.View) (json.RawMessage, error) {
			currentEffective = cloneConfiguration(current.Effective)
			var found bool
			metadata, found = findEntityMetadata(definition, mutation.EntityType)
			if !found || metadata.Collection == "" {
				return nil, ErrEntityTypeInvalid
			}
			if err := validateEntityID(metadata, mutation.EntityID); err != nil {
				return nil, err
			}
			if mutation.Action != "delete" {
				if mutation.Entity == nil {
					return nil, ErrEntityIDInvalid
				}
				if mutation.Entity.ID != mutation.EntityID {
					return nil, ErrEntityIDImmutable
				}
			}
			if metadata.Name == "unit_binding" && mutation.Scope != draft.ScopeEnvironmentOverride {
				return nil, fmt.Errorf("%w: %s", draft.ErrInvalidScope, mutation.Scope)
			}
			if mutation.Scope == draft.ScopeEnvironmentOverride && len(metadata.EnvironmentOverrideFields) == 0 {
				return nil, fmt.Errorf("%w: %s", draft.ErrInvalidScope, mutation.Scope)
			}
			base := targetReplacement(current, mutation.Scope)
			collection := records(base, metadata.Collection)
			currentRecord, exists := findRecord(records(current.Effective, metadata.Collection), mutation.EntityID)
			switch mutation.Action {
			case "create":
				if exists {
					return nil, ErrEntityExists
				}
				collection = append(collection, cloneRecord(*mutation.Entity))
			case "replace":
				if !exists {
					return nil, ErrEntityNotFound
				}
				if _, found := findRecord(collection, mutation.EntityID); !found {
					return nil, ErrEntityNotFound
				}
				collection = replaceRecord(collection, cloneRecord(*mutation.Entity))
			case "delete":
				if !exists {
					return nil, ErrEntityNotFound
				}
				if _, found := findRecord(collection, mutation.EntityID); !found {
					return nil, ErrEntityNotFound
				}
				deleted, _ = s.entityView(definition, current, metadata, currentRecord.ID)
				collection = removeRecord(collection, mutation.EntityID)
			default:
				return nil, fmt.Errorf("invalid entity mutation action %q", mutation.Action)
			}
			base[metadata.Collection] = recordsValue(collection)
			return json.Marshal(base)
		},
		Validate: func(replacement map[string]any) error {
			if err := validateRecords(definition, metadata, replacement, mutation.Scope); err != nil {
				return err
			}
			effective := cloneConfiguration(currentEffective)
			effective[metadata.Collection] = replacement[metadata.Collection]
			if mutation.Action == "delete" && metadata.DeletionPolicy == packs.DeletionPolicyRestrict {
				refs := references(definition, schema.PackRef, effective, mutation.EntityType, mutation.EntityID)
				if len(refs) > 0 {
					return &EntityReferencedError{Revision: mutation.ExpectedRevision, References: refs}
				}
			}
			if err := validateReferences(effective); err != nil {
				return err
			}
			return nil
		},
	}
	view, revision, err := s.drafts.Mutate(schema, environments, environmentID, draftMutation)
	if err != nil {
		return EntityView{}, 0, err
	}
	if mutation.Action == "delete" {
		return deleted, revision, nil
	}
	entity, err := s.entityView(definition, view, metadata, mutation.EntityID)
	return entity, revision, err
}

func (s *Service) entityContext(environmentID string) (draft.View, uint64, packs.Definition, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return draft.View{}, 0, packs.Definition{}, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return draft.View{}, 0, packs.Definition{}, err
	}
	definition, _, err := s.packRegistry.Resolve(snapshot.Manifest.Pack.ID)
	if err != nil {
		return draft.View{}, 0, packs.Definition{}, err
	}
	view, revision, err := s.drafts.View(schema, environments, environmentID)
	return view, revision, definition, err
}

func findEntityMetadata(definition packs.Definition, entityType string) (packs.EntityMetadata, bool) {
	for _, metadata := range definition.Metadata.EntityTypes {
		if metadata.Name == entityType {
			return metadata, true
		}
	}
	return packs.EntityMetadata{}, false
}

func (s *Service) entityView(definition packs.Definition, view draft.View, metadata packs.EntityMetadata, entityID string) (EntityView, error) {
	baseline, baselineFound := s.releasedBaseline(view.EnvironmentID)
	return entityView(definition, view, metadata, entityID, baseline, baselineFound)
}

func entityView(definition packs.Definition, view draft.View, metadata packs.EntityMetadata, entityID string, baseline releasedbaseline.Document, baselineFound bool) (EntityView, error) {
	effective, ok := findRecord(records(view.Effective, metadata.Collection), entityID)
	if !ok {
		return EntityView{}, ErrEntityNotFound
	}
	source, sourcePresent := layeredRecord(view.EnvironmentOverride.Source.Value, view.Baseline.Source.Value, metadata.Collection, entityID)
	draftRecord, draftPresent := layeredRecord(view.EnvironmentOverride.Draft.Value, view.Baseline.Draft.Value, metadata.Collection, entityID)
	resolved, resolvedPresent := layeredRecord(view.EnvironmentOverride.Resolved.Value, view.Baseline.Resolved.Value, metadata.Collection, entityID)
	origin := "baseline"
	if _, ok := findRecord(records(view.EnvironmentOverride.Resolved.Value, metadata.Collection), entityID); ok {
		origin = "environment_override"
		if _, ok := findRecord(records(view.EnvironmentOverride.Draft.Value, metadata.Collection), entityID); ok {
			origin = "draft_environment_override"
		}
	} else if _, ok := findRecord(records(view.Baseline.Draft.Value, metadata.Collection), entityID); ok {
		origin = "draft_baseline"
	}
	fieldsHash, err := releasedbaseline.HashFields(effective.Fields)
	if err != nil {
		return EntityView{}, err
	}
	changeStatus := "created"
	if baselineFound {
		if baseline.Entities[entityRef(view.PackRef, metadata.Name, entityID)] == fieldsHash {
			changeStatus = "unchanged"
		} else if _, exists := baseline.Entities[entityRef(view.PackRef, metadata.Name, entityID)]; exists {
			changeStatus = "modified"
		}
	}
	return EntityView{EntityRef: entityRef(view.PackRef, metadata.Name, entityID), EntityType: metadata.Name, EntityID: entityID, Source: recordPresence(source, sourcePresent), Draft: recordPresence(draftRecord, draftPresent), Resolved: recordPresence(resolved, resolvedPresent), Effective: recordPresence(effective, true), Origin: origin, SourceRevision: view.SourceRevision, ChangeStatus: changeStatus}, nil
}

func (s *Service) entityHashes(view draft.View) (map[string]string, error) {
	return s.entityHashesForConfiguration(view.PackRef, view.Effective)
}

func (s *Service) entityHashesForConfiguration(packRef string, configuration map[string]any) (map[string]string, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return nil, err
	}
	definition, _, err := s.packRegistry.Resolve(snapshot.Manifest.Pack.ID)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, metadata := range definition.Metadata.EntityTypes {
		if metadata.Collection == "" {
			continue
		}
		for _, record := range records(configuration, metadata.Collection) {
			hash, err := releasedbaseline.HashFields(record.Fields)
			if err != nil {
				return nil, err
			}
			result[entityRef(packRef, metadata.Name, record.ID)] = hash
		}
	}
	return result, nil
}

func (s *Service) populateChangedEntityCount(view *draft.View) error {
	current, err := s.entityHashes(*view)
	if err != nil {
		return err
	}
	baseline, found := s.releasedBaseline(view.EnvironmentID)
	if !found {
		view.ChangedEntityCount = len(current)
		return nil
	}
	changed := 0
	for reference, hash := range current {
		if baseline.Entities[reference] != hash {
			changed++
		}
	}
	for reference := range baseline.Entities {
		if _, exists := current[reference]; !exists {
			changed++
		}
	}
	view.ChangedEntityCount = changed
	return nil
}

// releasedBaseline restores the entity baseline for workspaces created before
// released-baseline persistence was introduced. Read models remain available
// when historical state is absent or cannot be recovered.
func (s *Service) releasedBaseline(environmentID string) (releasedbaseline.Document, bool) {
	baseline, found, err := s.baselines.Load(environmentID)
	if err != nil || found {
		return baseline, found
	}
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return releasedbaseline.Document{}, false
	}
	releases, err := s.releases.List(environmentID, 0, "")
	if err != nil {
		return releasedbaseline.Document{}, false
	}
	for _, item := range releases {
		if item.Outcome != "succeeded" {
			continue
		}
		raw, err := s.releases.ConflowState(item.ReleaseID)
		if err != nil {
			continue
		}
		var configuration map[string]any
		if err := json.Unmarshal(raw, &configuration); err != nil {
			continue
		}
		hashes, err := s.entityHashesForConfiguration(snapshot.Manifest.Pack.ID, configuration)
		if err != nil {
			continue
		}
		baseline = releasedbaseline.Document{
			EnvironmentID:  environmentID,
			ReleaseID:      item.ReleaseID,
			ReleasedAt:     item.CompletedAt,
			SourceRevision: item.SourceDigest,
			Entities:       hashes,
		}
		if err := s.baselines.Save(baseline); err != nil {
			return releasedbaseline.Document{}, false
		}
		return baseline, true
	}
	return releasedbaseline.Document{}, false
}

func layeredRecord(high, low map[string]any, collection, id string) (EntityRecord, bool) {
	if record, ok := findRecord(records(high, collection), id); ok {
		return record, true
	}
	return findRecord(records(low, collection), id)
}

func recordPresence(record EntityRecord, ok bool) EntityPresence {
	if !ok {
		return EntityPresence{}
	}
	copy := cloneRecord(record)
	return EntityPresence{Present: true, Value: &copy}
}

func records(configuration map[string]any, collection string) []EntityRecord {
	return entities.Records(configuration, collection)
}

func recordsValue(records []EntityRecord) []any {
	return entities.Values(records)
}

func findRecord(records []EntityRecord, id string) (EntityRecord, bool) {
	for _, record := range records {
		if record.ID == id {
			return record, true
		}
	}
	return EntityRecord{}, false
}

func replaceRecord(records []EntityRecord, replacement EntityRecord) []EntityRecord {
	for index := range records {
		if records[index].ID == replacement.ID {
			records[index] = replacement
		}
	}
	return records
}

func removeRecord(records []EntityRecord, id string) []EntityRecord {
	result := records[:0]
	for _, record := range records {
		if record.ID != id {
			result = append(result, record)
		}
	}
	return result
}

func targetReplacement(view draft.View, scope string) map[string]any {
	if scope == draft.ScopeEnvironmentOverride {
		return cloneConfiguration(view.EnvironmentOverride.Resolved.Value)
	}
	return cloneConfiguration(view.Baseline.Resolved.Value)
}

func cloneConfiguration(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, _ := json.Marshal(value)
	result := map[string]any{}
	_ = json.Unmarshal(raw, &result)
	return result
}

func cloneRecord(record EntityRecord) EntityRecord {
	return EntityRecord{ID: record.ID, Fields: cloneConfiguration(record.Fields)}
}

func entityRef(packRef, entityType, entityID string) string {
	return "entity:" + packRef + ":" + entityType + ":" + entityID
}

func validateEntityID(metadata packs.EntityMetadata, id string) error {
	if len(id) < metadata.IDRule.MinLength || len(id) > metadata.IDRule.MaxLength {
		return ErrEntityIDInvalid
	}
	pattern, err := regexp.Compile(metadata.IDRule.Pattern)
	if err != nil || !pattern.MatchString(id) {
		return ErrEntityIDInvalid
	}
	return nil
}

func validateRecords(definition packs.Definition, metadata packs.EntityMetadata, replacement map[string]any, scope string) error {
	schema, ok := findEntitySchema(definition, metadata.Name)
	if !ok {
		return ErrEntityTypeInvalid
	}
	for _, record := range records(replacement, metadata.Collection) {
		if err := validateEntityID(metadata, record.ID); err != nil {
			return err
		}
		known := make(map[string]packs.FieldSchema, len(schema.Fields))
		for _, field := range schema.Fields {
			known[field.Name] = field
			value, exists := record.Fields[field.Name]
			if field.Required && !exists {
				return validationError(scope, metadata.Collection, record.ID, field.Name, "required_field_missing")
			}
			if !exists {
				continue
			}
			if value == nil {
				if !field.Nullable {
					return validationError(scope, metadata.Collection, record.ID, field.Name, "explicit_null_forbidden")
				}
				continue
			}
			if !matchesEntityType(value, field.Type) {
				return validationError(scope, metadata.Collection, record.ID, field.Name, "field_type_mismatch")
			}
			if !allowsEntityValue(value, field) {
				return validationError(scope, metadata.Collection, record.ID, field.Name, "value_not_allowed")
			}
		}
		for name := range record.Fields {
			if _, exists := known[name]; !exists {
				return validationError(scope, metadata.Collection, record.ID, name, "invalid_config_shape")
			}
		}
	}
	return nil
}

func validationError(scope, collection, id, field, code string) error {
	return &draft.ValidationError{Details: []draft.StructuralError{{Code: code, Path: "/" + collection + "/" + id + "/fields/" + field, Scope: scope, Message: "字段值不符合配置包规则"}}}
}

func findEntitySchema(definition packs.Definition, name string) (packs.EntitySchema, bool) {
	for _, schema := range definition.Schema.Entities {
		if schema.Name == name {
			return schema, true
		}
	}
	return packs.EntitySchema{}, false
}

func matchesEntityType(value any, fieldType packs.FieldType) bool {
	switch fieldType {
	case packs.FieldTypeString, packs.FieldTypeReference:
		_, ok := value.(string)
		return ok
	case packs.FieldTypeBoolean:
		_, ok := value.(bool)
		return ok
	case packs.FieldTypeInteger:
		number, ok := value.(float64)
		return ok && number == float64(int64(number))
	case packs.FieldTypeNumber:
		_, ok := value.(float64)
		return ok
	case packs.FieldTypeArray:
		_, ok := value.([]any)
		return ok
	case packs.FieldTypeObject:
		_, ok := value.(map[string]any)
		return ok
	default:
		return false
	}
}

func allowsEntityValue(value any, field packs.FieldSchema) bool {
	if len(field.Validation.Enum) > 0 {
		raw, _ := json.Marshal(value)
		found := false
		for _, allowed := range field.Validation.Enum {
			if string(raw) == string(allowed) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if text, ok := value.(string); ok {
		length := len([]rune(text))
		if field.Validation.MinLength != nil && length < *field.Validation.MinLength {
			return false
		}
		if field.Validation.MaxLength != nil && length > *field.Validation.MaxLength {
			return false
		}
	}
	if number, ok := value.(float64); ok {
		if field.Validation.Minimum != nil && number < *field.Validation.Minimum {
			return false
		}
		if field.Validation.Maximum != nil && number > *field.Validation.Maximum {
			return false
		}
	}
	return true
}

func validateReferences(effective map[string]any) error {
	for _, placement := range records(effective, "placements") {
		policyID, _ := placement.Fields["frequency_policy_id"].(string)
		if _, found := findRecord(records(effective, "frequency_policies"), policyID); !found {
			return validationError(draft.ScopeBaseline, "placements", placement.ID, "frequency_policy_id", "value_not_allowed")
		}
	}
	for _, binding := range records(effective, "unit_bindings") {
		placementID, _ := binding.Fields["placement_id"].(string)
		if _, found := findRecord(records(effective, "placements"), placementID); !found {
			return validationError(draft.ScopeEnvironmentOverride, "unit_bindings", binding.ID, "placement_id", "value_not_allowed")
		}
	}
	return nil
}

func references(definition packs.Definition, packRef string, effective map[string]any, targetType, targetID string) []EntityReference {
	var result []EntityReference
	if targetType == "frequency_policy" {
		for _, placement := range records(effective, "placements") {
			if placement.Fields["frequency_policy_id"] == targetID {
				result = append(result, EntityReference{EntityRef: entityRef(packRef, "placement", placement.ID), EntityType: "placement", EntityID: placement.ID, Path: "/frequency_policy_id"})
			}
		}
	}
	if targetType == "placement" {
		for _, binding := range records(effective, "unit_bindings") {
			if binding.Fields["placement_id"] == targetID {
				result = append(result, EntityReference{EntityRef: entityRef(packRef, "unit_binding", binding.ID), EntityType: "unit_binding", EntityID: binding.ID, Path: "/placement_id"})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].EntityRef < result[j].EntityRef })
	return result
}
