// Package validation implements complete DraftView validation from Spec 007.
package validation

import (
	"sort"
	"time"
)

const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityError    = "error"
	SeverityBlocking = "blocking"

	ReadinessReady   = "ready"
	ReadinessBlocked = "blocked"
	StatusFresh      = "fresh"
	StatusStale      = "stale"
)

// Diagnostic is the stable, display-independent diagnosis contract.
type Diagnostic struct {
	Code             string `json:"code"`
	Path             string `json:"path"`
	Severity         string `json:"severity"`
	Message          string `json:"message"`
	EntityRef        string `json:"entity_ref,omitempty"`
	FixSuggestion    string `json:"fix_suggestion"`
	DocumentationURL string `json:"documentation_url,omitempty"`
}

// Result is persisted after a complete validation run.
type Result struct {
	EnvironmentID          string       `json:"environment_id"`
	ValidatedDraftRevision uint64       `json:"validated_draft_revision"`
	ValidatedAt            time.Time    `json:"validated_at"`
	Status                 string       `json:"status"`
	Readiness              string       `json:"readiness"`
	Diagnostics            []Diagnostic `json:"diagnostics"`
}

// SortDiagnostics applies the frozen diagnostic ordering.
func SortDiagnostics(diagnostics []Diagnostic) {
	sort.SliceStable(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) < severityRank(right.Severity)
		}
		if left.EntityRef != right.EntityRef {
			return left.EntityRef < right.EntityRef
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.Code < right.Code
	})
}

// ReadinessFor returns the complete-validation readiness for diagnostics.
func ReadinessFor(diagnostics []Diagnostic) string {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == SeverityError || diagnostic.Severity == SeverityBlocking {
			return ReadinessBlocked
		}
	}
	return ReadinessReady
}

// ExitCodeFor returns the CLI process exit code defined by Spec 007.
func ExitCodeFor(diagnostics []Diagnostic) int {
	code := 0
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case SeverityBlocking:
			return 2
		case SeverityError:
			code = 1
		}
	}
	return code
}

func severityRank(severity string) int {
	switch severity {
	case SeverityBlocking:
		return 0
	case SeverityError:
		return 1
	case SeverityWarning:
		return 2
	default:
		return 3
	}
}
