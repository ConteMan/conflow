package importer

import (
	"fmt"
	"strings"
)

// ApplyInput is the input to Apply.
type ApplyInput struct {
	Bundle        ImportBundle
	PreviewToken  string
	Decisions     []ImportDecision
	ConflictMode  ConflictMode
	DraftRevision uint64
}

// ApplyResult is returned by Apply. The service layer uses PreparedEntities to
// perform the actual draft write.
type ApplyResult struct {
	AppliedCount int
	SkippedCount int
	// PreparedEntities holds the bundle entities after decisions have been merged
	// in, keyed by entity type. unit_binding is always excluded.
	PreparedEntities map[string][]BundleEntity
}

// Apply validates the previewToken against the current draft revision, verifies
// that every DecisionRequired item has a corresponding operator-supplied
// decision, and merges decisions into the bundle entities.
//
// Apply does not modify the draft; the service layer applies PreparedEntities
// using draft.Mutation.
func Apply(input ApplyInput) (ApplyResult, error) {
	if err := ValidateToken(input.PreviewToken, input.DraftRevision); err != nil {
		return ApplyResult{}, err
	}

	// Build a decision lookup map keyed by DecisionRequired.Key.
	decisionMap := make(map[string]interface{}, len(input.Decisions))
	for _, d := range input.Decisions {
		decisionMap[d.Key] = d.Value
	}

	// Every DecisionRequired item must have a corresponding decision.
	var missing []string
	for _, required := range input.Bundle.DecisionsRequired {
		if _, ok := decisionMap[required.Key]; !ok {
			missing = append(missing, required.Key)
		}
	}
	if len(missing) > 0 {
		return ApplyResult{}, fmt.Errorf("missing required decisions: %s", strings.Join(missing, ", "))
	}

	// Merge decisions into copies of bundle entities (never mutate the input).
	prepared := make(map[string][]BundleEntity, len(input.Bundle.Entities))
	for entityType, bundleEntities := range input.Bundle.Entities {
		if entityType == "unit_binding" {
			continue
		}
		merged := make([]BundleEntity, len(bundleEntities))
		for i, entity := range bundleEntities {
			newEntity := BundleEntity{
				ID:     entity.ID,
				Fields: cloneFields(entity.Fields),
			}
			// Decision key format: "<entity_type>.<entity_id>.<field_name>"
			prefix := entityType + "." + entity.ID + "."
			for key, value := range decisionMap {
				if strings.HasPrefix(key, prefix) {
					fieldName := key[len(prefix):]
					if fieldName != "" {
						newEntity.Fields[fieldName] = value
					}
				}
			}
			merged[i] = newEntity
		}
		prepared[entityType] = merged
	}

	return ApplyResult{PreparedEntities: prepared}, nil
}

func cloneFields(fields map[string]interface{}) map[string]interface{} {
	if fields == nil {
		return make(map[string]interface{})
	}
	result := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		result[k] = v
	}
	return result
}
