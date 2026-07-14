package app

import (
	"context"
	"encoding/json"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/importer"
)

// ExportImportBundle exports the current effective configuration for the named
// environment as an ImportBundle. unit_binding entities and Provider credentials
// are never included.
func (s *Service) ExportImportBundle(ctx context.Context, environmentID string) (importer.ImportBundle, error) {
	view, _, definition, err := s.entityContext(environmentID)
	if err != nil {
		return importer.ImportBundle{}, err
	}

	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return importer.ImportBundle{}, err
	}

	draftEntities := make(map[string][]importer.BundleEntity)
	for _, metadata := range definition.Metadata.EntityTypes {
		if metadata.Collection == "" || metadata.Name == "unit_binding" {
			continue
		}
		entityRecords := records(view.Effective, metadata.Collection)
		if len(entityRecords) == 0 {
			continue
		}
		bundleEntities := make([]importer.BundleEntity, len(entityRecords))
		for i, rec := range entityRecords {
			bundleEntities[i] = importer.BundleEntity{
				ID:     rec.ID,
				Fields: cloneConfiguration(rec.Fields),
			}
		}
		draftEntities[metadata.Name] = bundleEntities
	}

	return importer.Export(snapshot.Manifest.Pack.ID, definition.Schema.Version, environmentID, draftEntities)
}

// PreviewImport returns a PreviewResult for the given bundle without modifying
// the draft. The PreviewToken in the result must be supplied to ApplyImport.
func (s *Service) PreviewImport(ctx context.Context, environmentID string, bundle importer.ImportBundle, conflictMode importer.ConflictMode) (importer.PreviewResult, error) {
	view, revision, definition, err := s.entityContext(environmentID)
	if err != nil {
		return importer.PreviewResult{}, err
	}

	// Extract entity IDs from the current effective draft configuration.
	draftEntityIDs := make(map[string][]string)
	for _, metadata := range definition.Metadata.EntityTypes {
		if metadata.Collection == "" || metadata.Name == "unit_binding" {
			continue
		}
		entityRecords := records(view.Effective, metadata.Collection)
		ids := make([]string, len(entityRecords))
		for i, rec := range entityRecords {
			ids[i] = rec.ID
		}
		draftEntityIDs[metadata.Name] = ids
	}

	return importer.Preview(bundle, draftEntityIDs, revision, conflictMode)
}

// ApplyImport validates the previewToken and atomically writes the imported
// entities to the draft. Returns the updated draft view, the new revision, and
// import counts.
//
// The bundle must be the same one that was passed to PreviewImport; the token
// encodes the draft revision to detect concurrent edits.
func (s *Service) ApplyImport(ctx context.Context, environmentID string, bundle importer.ImportBundle, previewToken string, decisions []importer.ImportDecision, conflictMode importer.ConflictMode) (draft.View, uint64, importer.ApplyResult, error) {
	// Step 1: Load current draft state and pack definition.
	view, revision, definition, err := s.entityContext(environmentID)
	if err != nil {
		return draft.View{}, 0, importer.ApplyResult{}, err
	}

	// Step 2: Validate token and merge decisions into bundle entities.
	applyResult, err := importer.Apply(importer.ApplyInput{
		Bundle:        bundle,
		PreviewToken:  previewToken,
		Decisions:     decisions,
		ConflictMode:  conflictMode,
		DraftRevision: revision,
	})
	if err != nil {
		return draft.View{}, 0, importer.ApplyResult{}, err
	}

	// Step 3: Determine which entities to write based on conflict mode and
	// the current draft contents. This mirrors the Preview logic so the
	// operator never writes more than they previewed.
	type writeItem struct {
		collectionName string
		entity         importer.BundleEntity
		isReplace      bool
	}
	var toWrite []writeItem
	skippedCount := 0

	for entityType, preparedEntities := range applyResult.PreparedEntities {
		metadata, ok := findEntityMetadata(definition, entityType)
		if !ok || metadata.Collection == "" {
			continue
		}

		draftEffective := records(view.Effective, metadata.Collection)
		draftIDSet := make(map[string]bool, len(draftEffective))
		for _, rec := range draftEffective {
			draftIDSet[rec.ID] = true
		}
		typeHasDraftEntities := len(draftEffective) > 0

		for _, entity := range preparedEntities {
			inDraft := draftIDSet[entity.ID]
			switch {
			case !inDraft:
				if conflictMode == importer.ConflictSkip && typeHasDraftEntities {
					skippedCount++
				} else {
					toWrite = append(toWrite, writeItem{
						collectionName: metadata.Collection,
						entity:         entity,
						isReplace:      false,
					})
				}
			default:
				switch conflictMode {
				case importer.ConflictReplace:
					toWrite = append(toWrite, writeItem{
						collectionName: metadata.Collection,
						entity:         entity,
						isReplace:      true,
					})
				case importer.ConflictMerge, importer.ConflictSkip:
					skippedCount++
				}
			}
		}
	}

	appliedCount := len(toWrite)

	// Nothing to write — return current view without touching the draft.
	if appliedCount == 0 {
		return view, revision, importer.ApplyResult{AppliedCount: 0, SkippedCount: skippedCount}, nil
	}

	// Step 4: Build a map of write items keyed by collection name.
	byCollection := make(map[string][]writeItem, len(toWrite))
	for _, item := range toWrite {
		byCollection[item.collectionName] = append(byCollection[item.collectionName], item)
	}

	// Step 5: Load schema/environments for the Mutate call.
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return draft.View{}, 0, importer.ApplyResult{}, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return draft.View{}, 0, importer.ApplyResult{}, err
	}

	// Step 6: Atomically write all entities to the baseline draft layer.
	newView, newRevision, err := s.drafts.Mutate(schema, environments, environmentID, draft.Mutation{
		ExpectedRevision:       revision,
		ExpectedSourceRevision: view.SourceRevision,
		Scope:                  draft.ScopeBaseline,
		Action:                 "put",
		Prepare: func(current draft.View) (json.RawMessage, error) {
			base := cloneConfiguration(current.Baseline.Resolved.Value)
			for collectionName, items := range byCollection {
				collection := records(base, collectionName)
				for _, item := range items {
					rec := EntityRecord{ID: item.entity.ID, Fields: item.entity.Fields}
					if item.isReplace {
						collection = replaceRecord(collection, rec)
					} else {
						collection = append(collection, rec)
					}
				}
				base[collectionName] = recordsValue(collection)
			}
			return json.Marshal(base)
		},
	})
	if err != nil {
		return draft.View{}, 0, importer.ApplyResult{}, err
	}

	return newView, newRevision, importer.ApplyResult{AppliedCount: appliedCount, SkippedCount: skippedCount}, nil
}
