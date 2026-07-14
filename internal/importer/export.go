package importer

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Export reads all non-unit_binding entities from draftEntities and produces an
// ImportBundle. Credentials and Provider config are never included; they must
// not appear in draftEntities.
//
// packRef is the Pack reference string (e.g. "mobile-ad-monetization/v2").
// schemaVersion is the current Pack schema version.
// sourceID is an opaque identifier for the source workspace (used for tracing).
func Export(packRef string, schemaVersion uint64, sourceID string, draftEntities map[string][]BundleEntity) (ImportBundle, error) {
	// Remove unit_binding from entities (defensive; callers should not pass it).
	entities := make(map[string][]BundleEntity, len(draftEntities))
	for entityType, ents := range draftEntities {
		if entityType == "unit_binding" {
			continue
		}
		if len(ents) > 0 {
			entities[entityType] = ents
		}
	}

	digest, err := computeDigest(entities)
	if err != nil {
		return ImportBundle{}, fmt.Errorf("compute source digest: %w", err)
	}

	return ImportBundle{
		FormatVersion: FormatVersion,
		PackRef:       packRef,
		SchemaVersion: schemaVersion,
		CreatedAt:     time.Now().UTC(),
		SourceID:      sourceID,
		SourceDigest:  digest,
		Entities:      entities,
	}, nil
}

// computeDigest produces a deterministic SHA-256 digest over the entity set so
// that identical inputs always produce semantically equivalent bundles.
func computeDigest(entities map[string][]BundleEntity) (string, error) {
	// Sorted entity types for determinism.
	types := make([]string, 0, len(entities))
	for t := range entities {
		types = append(types, t)
	}
	sort.Strings(types)

	type digestEntry struct {
		EntityType string         `json:"entity_type"`
		Entities   []BundleEntity `json:"entities"`
	}

	sorted := make([]digestEntry, 0, len(types))
	for _, entityType := range types {
		ents := make([]BundleEntity, len(entities[entityType]))
		copy(ents, entities[entityType])
		// Sort by ID for determinism.
		sort.Slice(ents, func(i, j int) bool {
			return ents[i].ID < ents[j].ID
		})
		sorted = append(sorted, digestEntry{EntityType: entityType, Entities: ents})
	}

	data, err := json.Marshal(sorted)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", hash), nil
}
