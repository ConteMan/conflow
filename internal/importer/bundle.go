// Package importer implements the configuration import flow defined in Spec 021.
// An ImportBundle is the versioned interchange format between source workspaces
// (or external bootstrap scripts) and target Conflow workspaces.
package importer

import "time"

// FormatVersion is the only format version currently supported.
const FormatVersion = 1

// ConflictMode controls how entity ID collisions are handled during import.
type ConflictMode string

const (
	// ConflictReplace overwrites existing entities with the same ID (default).
	ConflictReplace ConflictMode = "replace"
	// ConflictMerge only imports entities whose ID does not already exist in the draft.
	ConflictMerge ConflictMode = "merge"
	// ConflictSkip skips all conflicting entities; only adds entities of types
	// that are completely absent from the draft.
	ConflictSkip ConflictMode = "skip"
)

// ImportBundle is the versioned interchange format produced by export and consumed by preview/apply.
// It must not contain Firebase credentials, access tokens, private keys, or unit_binding entities.
type ImportBundle struct {
	FormatVersion  int                        `json:"format_version"`
	PackRef        string                     `json:"pack_ref"`
	SchemaVersion  uint64                     `json:"schema_version"`
	CreatedAt      time.Time                  `json:"created_at"`
	SourceID       string                     `json:"source_id,omitempty"`
	SourceRevision string                     `json:"source_revision,omitempty"`
	SourceDigest   string                     `json:"source_digest,omitempty"`
	Entities       map[string][]BundleEntity  `json:"entities"`
	// DecisionsRequired lists fields that cannot be inferred from the source and must
	// be supplied by the operator in the apply call.
	DecisionsRequired []DecisionRequired `json:"decisions_required,omitempty"`
}

// BundleEntity is a single entity record inside an ImportBundle.
type BundleEntity struct {
	ID     string                 `json:"id"`
	Fields map[string]interface{} `json:"fields"`
}

// DecisionRequired describes one field that the operator must supply before apply.
type DecisionRequired struct {
	// Key uses the format "<entity_type>.<entity_id>.<field_name>".
	Key    string `json:"key"`
	Reason string `json:"reason"`
	Hint   string `json:"hint,omitempty"`
}

// ImportDecision is the operator-supplied value for one DecisionRequired item.
type ImportDecision struct {
	// Key matches DecisionRequired.Key.
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// EntityAction describes what will happen to one entity during apply.
type EntityAction struct {
	EntityType string      `json:"entity_type"`
	ID         string      `json:"id"`
	Diff       interface{} `json:"diff,omitempty"`
}

// EntityPlan breaks down the full set of changes that apply would make.
type EntityPlan struct {
	ToAdd     []EntityAction `json:"to_add"`
	ToReplace []EntityAction `json:"to_replace"`
	ToSkip    []EntityAction `json:"to_skip"`
	ToKeep    []EntityAction `json:"to_keep"`
}

// PreviewResult is returned by PreviewImport. The PreviewToken must be supplied
// to ApplyImport via If-Match; it expires after 15 minutes or when the draft
// revision changes.
type PreviewResult struct {
	PreviewToken      string             `json:"preview_token"`
	ExpiresAt         time.Time          `json:"expires_at"`
	PackRef           string             `json:"pack_ref"`
	ConflictMode      ConflictMode       `json:"conflict_mode"`
	EntityPlan        EntityPlan         `json:"entity_plan"`
	DecisionsRequired []DecisionRequired `json:"decisions_required"`
	Risks             []string           `json:"risks,omitempty"`
}
