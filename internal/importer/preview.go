package importer

import (
	"fmt"
	"time"
)

// Preview generates an EntityPlan for importing bundle into the current draft
// without modifying it. conflictMode controls how entity ID collisions are handled.
//
// draftEntities is a map of entity type name → slice of entity IDs currently
// present in the draft (excluding unit_binding).
func Preview(bundle ImportBundle, draftEntities map[string][]string, draftRevision uint64, conflictMode ConflictMode) (PreviewResult, error) {
	if bundle.FormatVersion != FormatVersion {
		return PreviewResult{}, fmt.Errorf("unsupported bundle format version: %d", bundle.FormatVersion)
	}

	plan := EntityPlan{
		ToAdd:     []EntityAction{},
		ToReplace: []EntityAction{},
		ToSkip:    []EntityAction{},
		ToKeep:    []EntityAction{},
	}

	// Track which bundle entity IDs we've seen per type, to compute ToKeep.
	bundleSeenIDs := make(map[string]map[string]bool)

	for entityType, bundleEntities := range bundle.Entities {
		if entityType == "unit_binding" {
			continue
		}

		draftIDs := draftEntities[entityType]
		draftIDSet := make(map[string]bool, len(draftIDs))
		for _, id := range draftIDs {
			draftIDSet[id] = true
		}

		bundleSeenIDs[entityType] = make(map[string]bool, len(bundleEntities))
		typeHasDraftEntities := len(draftIDs) > 0

		for _, entity := range bundleEntities {
			bundleSeenIDs[entityType][entity.ID] = true

			inDraft := draftIDSet[entity.ID]
			switch {
			case !inDraft:
				// Entity not in draft.
				// ConflictSkip: if the entity type has any draft entries, skip all
				// entities of that type (even ones with new IDs).
				if conflictMode == ConflictSkip && typeHasDraftEntities {
					plan.ToSkip = append(plan.ToSkip, EntityAction{EntityType: entityType, ID: entity.ID})
				} else {
					plan.ToAdd = append(plan.ToAdd, EntityAction{EntityType: entityType, ID: entity.ID})
				}
			default:
				// Entity already exists in draft.
				switch conflictMode {
				case ConflictReplace:
					plan.ToReplace = append(plan.ToReplace, EntityAction{EntityType: entityType, ID: entity.ID})
				case ConflictMerge, ConflictSkip:
					plan.ToSkip = append(plan.ToSkip, EntityAction{EntityType: entityType, ID: entity.ID})
				}
			}
		}
	}

	// ToKeep: draft entities that are not present in the bundle at all.
	for entityType, draftIDs := range draftEntities {
		seenIDs := bundleSeenIDs[entityType]
		for _, id := range draftIDs {
			if !seenIDs[id] {
				plan.ToKeep = append(plan.ToKeep, EntityAction{EntityType: entityType, ID: id})
			}
		}
	}

	token := GenerateToken(draftRevision)
	expiresAt := time.Now().UTC().Add(tokenTTL)

	var risks []string
	if len(plan.ToReplace) > 0 {
		risks = append(risks, fmt.Sprintf("将替换 %d 条已有实体", len(plan.ToReplace)))
	}

	return PreviewResult{
		PreviewToken:      token,
		ExpiresAt:         expiresAt,
		PackRef:           bundle.PackRef,
		ConflictMode:      conflictMode,
		EntityPlan:        plan,
		DecisionsRequired: bundle.DecisionsRequired,
		Risks:             risks,
	}, nil
}
