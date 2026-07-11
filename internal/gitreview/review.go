// Package gitreview implements the explicit local Git review workflow.
package gitreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ConteMan/conflow/internal/plan"
	"github.com/ConteMan/conflow/internal/source"
	"github.com/ConteMan/conflow/internal/validation"
)

var (
	ErrNotGitRepository       = errors.New("not a git repository")
	ErrBranchExists           = errors.New("branch already exists")
	ErrInvalidManagedFile     = errors.New("file is not a declared managed file")
	ErrNoManagedFiles         = errors.New("managed file list is required")
	ErrIdempotencyConflict    = errors.New("idempotency key conflicts with a different request")
	ErrIdempotencyKeyRequired = errors.New("idempotency key is required")
)

type Status struct {
	Root           string   `json:"root"`
	Branch         string   `json:"branch"`
	Head           string   `json:"head"`
	Dirty          bool     `json:"dirty"`
	ManagedFiles   []string `json:"managed_files"`
	ChangedFiles   []string `json:"changed_files"`
	UnrelatedFiles []string `json:"unrelated_files"`
}

type PrepareInput struct {
	EnvironmentID string
	Slug          string
	Plan          *plan.Plan
	Validation    *validation.Result
}

type PrepareResult struct {
	Status          Status `json:"status"`
	SuggestedBranch string `json:"suggested_branch"`
	CommitMessage   string `json:"commit_message"`
	ReviewMarkdown  string `json:"review_markdown"`
}

type CreateBranchInput struct{ Branch, IdempotencyKey string }
type CommitInput struct {
	Files                   []string
	Message, IdempotencyKey string
}
type CommitResult struct {
	Commit   string   `json:"commit"`
	Files    []string `json:"files"`
	Replayed bool     `json:"replayed,omitempty"`
}
type BranchResult struct {
	Branch   string `json:"branch"`
	Replayed bool   `json:"replayed,omitempty"`
}

type idempotencyRecord struct {
	Digest string          `json:"digest"`
	Result json.RawMessage `json:"result"`
}
type disk struct {
	Idempotency map[string]idempotencyRecord `json:"idempotency"`
}

type Manager struct {
	mu        sync.Mutex
	root      string
	managed   []string
	executor  source.GitExecutor
	storePath string
	disk      disk
}

func Open(root string, managed []string, executor source.GitExecutor) (*Manager, error) {
	clean := uniquePaths(managed)
	m := &Manager{root: root, managed: clean, executor: executor, storePath: filepath.Join(root, ".conflow", "git-review.json"), disk: disk{Idempotency: map[string]idempotencyRecord{}}}
	b, err := os.ReadFile(m.storePath)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read git review store: %w", err)
	}
	if err := json.Unmarshal(b, &m.disk); err != nil {
		return nil, fmt.Errorf("parse git review store: %w", err)
	}
	if m.disk.Idempotency == nil {
		m.disk.Idempotency = map[string]idempotencyRecord{}
	}
	return m, nil
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	root, err := m.output(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return Status{}, fmt.Errorf("%w: %v", ErrNotGitRepository, err)
	}
	branch, err := m.output(ctx, "branch", "--show-current")
	if err != nil {
		return Status{}, err
	}
	head, err := m.output(ctx, "rev-parse", "HEAD")
	if err != nil {
		return Status{}, err
	}
	porcelain, err := m.output(ctx, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	changed := porcelainPaths(porcelain)
	managedSet := make(map[string]bool, len(m.managed))
	for _, path := range m.managed {
		managedSet[path] = true
	}
	unrelated := make([]string, 0)
	for _, path := range changed {
		if !managedSet[path] {
			unrelated = append(unrelated, path)
		}
	}
	return Status{Root: strings.TrimSpace(root), Branch: strings.TrimSpace(branch), Head: strings.TrimSpace(head), Dirty: len(changed) != 0, ManagedFiles: append([]string{}, m.managed...), ChangedFiles: changed, UnrelatedFiles: unrelated}, nil
}

func (m *Manager) Prepare(ctx context.Context, input PrepareInput) (PrepareResult, error) {
	status, err := m.Status(ctx)
	if err != nil {
		return PrepareResult{}, err
	}
	diff, err := m.output(ctx, append([]string{"diff", "HEAD", "--stat", "--"}, m.managed...)...)
	if err != nil {
		return PrepareResult{}, err
	}
	return PrepareResult{Status: status, SuggestedBranch: suggestedBranch(input.EnvironmentID, input.Slug), CommitMessage: suggestedCommit(input.EnvironmentID, input.Plan), ReviewMarkdown: reviewMarkdown(status, string(diff), input)}, nil
}

func (m *Manager) CreateBranch(ctx context.Context, input CreateBranchInput) (BranchResult, error) {
	if input.IdempotencyKey == "" {
		return BranchResult{}, ErrIdempotencyKeyRequired
	}
	if !validBranch(input.Branch) {
		return BranchResult{}, fmt.Errorf("invalid branch name")
	}
	digest := digest(input.Branch)
	m.mu.Lock()
	defer m.mu.Unlock()
	if replay, ok, err := m.replayBranch("create-branch", input.IdempotencyKey, digest); ok || err != nil {
		return replay, err
	}
	if _, err := m.output(ctx, "show-ref", "--verify", "--quiet", "refs/heads/"+input.Branch); err == nil {
		return BranchResult{}, ErrBranchExists
	}
	if _, err := m.output(ctx, "switch", "-c", input.Branch); err != nil {
		return BranchResult{}, fmt.Errorf("create branch: %w", err)
	}
	result := BranchResult{Branch: input.Branch}
	return result, m.save("create-branch", input.IdempotencyKey, digest, result)
}

func (m *Manager) Commit(ctx context.Context, input CommitInput) (CommitResult, error) {
	if input.IdempotencyKey == "" {
		return CommitResult{}, ErrIdempotencyKeyRequired
	}
	for _, path := range input.Files {
		if !safeRelativePath(path) {
			return CommitResult{}, fmt.Errorf("%w: %s", ErrInvalidManagedFile, path)
		}
	}
	files := uniquePaths(input.Files)
	if len(files) == 0 {
		return CommitResult{}, ErrNoManagedFiles
	}
	for _, path := range files {
		if !contains(m.managed, path) {
			return CommitResult{}, fmt.Errorf("%w: %s", ErrInvalidManagedFile, path)
		}
	}
	if strings.TrimSpace(input.Message) == "" {
		return CommitResult{}, errors.New("commit message is required")
	}
	digest := digest(input.Message, strings.Join(files, "\n"))
	m.mu.Lock()
	defer m.mu.Unlock()
	if replay, ok, err := m.replayCommit("commit", input.IdempotencyKey, digest); ok || err != nil {
		return replay, err
	}
	for _, path := range files {
		if _, err := m.output(ctx, "add", "--", path); err != nil {
			return CommitResult{}, fmt.Errorf("stage %s: %w", path, err)
		}
	}
	args := []string{"commit", "--only", "-m", input.Message, "--"}
	args = append(args, files...)
	if _, err := m.output(ctx, args...); err != nil {
		return CommitResult{}, fmt.Errorf("commit managed files: %w", err)
	}
	commit, err := m.output(ctx, "rev-parse", "HEAD")
	if err != nil {
		return CommitResult{}, fmt.Errorf("read commit metadata: %w", err)
	}
	result := CommitResult{Commit: strings.TrimSpace(commit), Files: files}
	return result, m.save("commit", input.IdempotencyKey, digest, result)
}

func (m *Manager) output(ctx context.Context, args ...string) (string, error) {
	b, err := m.executor.Output(ctx, m.root, args...)
	return strings.TrimSpace(string(b)), err
}
func (m *Manager) replayBranch(action, key, want string) (BranchResult, bool, error) {
	var r BranchResult
	ok, err := m.replay(action, key, want, &r)
	r.Replayed = ok && err == nil
	return r, ok, err
}
func (m *Manager) replayCommit(action, key, want string) (CommitResult, bool, error) {
	var r CommitResult
	ok, err := m.replay(action, key, want, &r)
	r.Replayed = ok && err == nil
	return r, ok, err
}
func (m *Manager) replay(action, key, want string, target any) (bool, error) {
	r, ok := m.disk.Idempotency[action+"|"+key]
	if !ok {
		return false, nil
	}
	if r.Digest != want {
		return true, ErrIdempotencyConflict
	}
	return true, json.Unmarshal(r.Result, target)
}
func (m *Manager) save(action, key, want string, result any) error {
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	m.disk.Idempotency[action+"|"+key] = idempotencyRecord{Digest: want, Result: b}
	b, err = json.MarshalIndent(m.disk, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(m.storePath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(m.storePath, append(b, '\n'), 0o600)
}
func porcelainPaths(raw string) []string {
	set := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if len(line) < 4 {
			continue
		}
		p := strings.TrimSpace(line[3:])
		if i := strings.Index(p, " -> "); i >= 0 {
			p = p[i+4:]
		}
		if p != "" {
			set[filepath.ToSlash(p)] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
func uniquePaths(paths []string) []string {
	set := map[string]bool{}
	for _, p := range paths {
		p = filepath.ToSlash(filepath.Clean(p))
		if p != "." && !strings.HasPrefix(p, "../") && !filepath.IsAbs(p) {
			set[p] = true
		}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func safeRelativePath(path string) bool {
	normalized := filepath.ToSlash(filepath.Clean(path))
	return normalized != "." && !strings.HasPrefix(normalized, "../") && !filepath.IsAbs(path) && !strings.Contains(path, "\\")
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func digest(values ...string) string {
	h := sha256.New()
	for _, v := range values {
		_, _ = h.Write([]byte(v))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
func suggestedBranch(environment, slug string) string {
	if environment == "" {
		environment = "config"
	}
	slug = slugify(slug)
	if slug == "" {
		slug = "review"
	}
	return "conflow/" + slugify(environment) + "-" + time.Now().UTC().Format("20060102") + "-" + slug
}
func suggestedCommit(environment string, p *plan.Plan) string {
	count := 0
	if p != nil {
		count = len(p.SemanticChanges)
	}
	if environment == "" {
		environment = "config"
	}
	if count == 0 {
		return "chore(" + slugify(environment) + "): review config changes"
	}
	return fmt.Sprintf("feat(%s): update %d config changes", slugify(environment), count)
}
func slugify(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	dash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
func validBranch(branch string) bool {
	return branch != "" && !strings.HasPrefix(branch, "-") && !strings.Contains(branch, "..") && !strings.ContainsAny(branch, " ~^:?*[\\") && !strings.HasSuffix(branch, "/") && !strings.HasSuffix(branch, ".")
}
func reviewMarkdown(status Status, stat string, input PrepareInput) string {
	var b strings.Builder
	b.WriteString("# Git Review\n\n")
	fmt.Fprintf(&b, "Branch: `%s`\n\n", status.Branch)
	b.WriteString("## Managed files\n\n")
	for _, p := range status.ManagedFiles {
		fmt.Fprintf(&b, "- `%s`\n", p)
	}
	b.WriteString("\n## File diff summary\n\n```text\n")
	b.WriteString(strings.TrimSpace(stat))
	b.WriteString("\n```\n\n## Semantic diff\n\n")
	if input.Plan == nil {
		b.WriteString("No Plan is available; semantic diff and risk assessment are unavailable.\n")
	} else if len(input.Plan.SemanticChanges) == 0 {
		b.WriteString("No semantic changes.\n")
	} else {
		for _, c := range input.Plan.SemanticChanges {
			fmt.Fprintf(&b, "- %s: %s (%s -> %s)\n", c.DirectEntityRef, c.ChangeKind, c.BeforeSummary, c.AfterSummary)
		}
	}
	b.WriteString("\n## Validation\n\n")
	if input.Validation == nil {
		b.WriteString("No validation result is available.\n")
	} else {
		fmt.Fprintf(&b, "Readiness: **%s** (%s, %d diagnostics)\n", input.Validation.Readiness, input.Validation.Status, len(input.Validation.Diagnostics))
	}
	b.WriteString("\n## Plan digest\n\n")
	if input.Plan == nil {
		b.WriteString("No Plan is available.\n")
	} else {
		fmt.Fprintf(&b, "Plan `%s`, digest `%s`, severity `%s`.\n", input.Plan.PlanID, input.Plan.ContentDigest, input.Plan.Severity)
	}
	b.WriteString("\n## Risks\n\n")
	if input.Plan == nil {
		b.WriteString("Risk list unavailable because no Plan is available.\n")
	} else if len(input.Plan.RiskItems) == 0 {
		b.WriteString("No risks reported.\n")
	} else {
		for _, r := range input.Plan.RiskItems {
			fmt.Fprintf(&b, "- **%s** `%s`: %s\n", r.Severity, r.ReasonCode, r.Summary)
		}
	}
	if len(status.UnrelatedFiles) > 0 {
		b.WriteString("\n## Protected unrelated changes\n\n")
		for _, p := range status.UnrelatedFiles {
			fmt.Fprintf(&b, "- `%s`\n", p)
		}
	}
	return b.String()
}
