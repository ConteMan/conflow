package gitreview

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConteMan/conflow/internal/source"
)

func TestPrepareBranchCommitPreservesUnrelatedChanges(t *testing.T) {
	root := reviewRepository(t)
	managed := filepath.Join(root, "config", "managed.json")
	if err := os.WriteFile(managed, []byte("{\"enabled\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager, err := Open(root, []string{"config/managed.json"}, source.DefaultGitExecutor)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := manager.Prepare(context.Background(), PrepareInput{EnvironmentID: "development", Slug: "enable ads"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prepared.ReviewMarkdown, "No Plan is available") || !strings.Contains(prepared.ReviewMarkdown, "notes.txt") {
		t.Fatalf("review = %s", prepared.ReviewMarkdown)
	}
	if !strings.HasPrefix(prepared.SuggestedBranch, "conflow/development-") || !strings.HasSuffix(prepared.SuggestedBranch, "-enable-ads") {
		t.Fatalf("branch = %q", prepared.SuggestedBranch)
	}
	branch, err := manager.CreateBranch(context.Background(), CreateBranchInput{Branch: prepared.SuggestedBranch, IdempotencyKey: "branch-key-0000001"})
	if err != nil || branch.Replayed {
		t.Fatalf("branch = %#v, err = %v", branch, err)
	}
	commit, err := manager.Commit(context.Background(), CommitInput{Files: []string{"config/managed.json"}, Message: prepared.CommitMessage, IdempotencyKey: "commit-key-0000001"})
	if err != nil || commit.Commit == "" {
		t.Fatalf("commit = %#v, err = %v", commit, err)
	}
	replayed, err := manager.Commit(context.Background(), CommitInput{Files: []string{"config/managed.json"}, Message: prepared.CommitMessage, IdempotencyKey: "commit-key-0000001"})
	if err != nil || !replayed.Replayed || replayed.Commit != commit.Commit {
		t.Fatalf("replayed = %#v, err = %v", replayed, err)
	}
	show := gitOutputForReview(t, root, "show", "--format=", "--name-only", "HEAD")
	if strings.TrimSpace(show) != "config/managed.json" {
		t.Fatalf("committed files = %q", show)
	}
	if got := gitOutputForReview(t, root, "status", "--porcelain"); !strings.Contains(got, "notes.txt") || strings.Contains(got, "managed.json") {
		t.Fatalf("status = %q", got)
	}
	if content, err := os.ReadFile(filepath.Join(root, "notes.txt")); err != nil || string(content) != "keep me\n" {
		t.Fatalf("unrelated content = %q, err = %v", content, err)
	}
}

func TestBranchExistsAndCommitFailureLeaveVisibleState(t *testing.T) {
	root := reviewRepository(t)
	manager, err := Open(root, []string{"config/managed.json"}, source.DefaultGitExecutor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateBranch(context.Background(), CreateBranchInput{Branch: "conflow/existing", IdempotencyKey: "branch-key-0000002"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.CreateBranch(context.Background(), CreateBranchInput{Branch: "conflow/existing", IdempotencyKey: "branch-key-0000003"}); !errors.Is(err, ErrBranchExists) {
		t.Fatalf("error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "managed.json"), []byte("{\"enabled\":true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Commit(context.Background(), CommitInput{Files: []string{"config/managed.json"}, Message: "feat(dev): test", IdempotencyKey: "commit-key-0000002"}); err == nil {
		t.Fatal("expected commit hook to fail")
	}
	status := gitOutputForReview(t, root, "status", "--porcelain")
	if !strings.Contains(status, "M  config/managed.json") || !strings.Contains(status, "?? other.txt") {
		t.Fatalf("status after failed commit = %q", status)
	}
}

func TestReviewMarkdownNoPlanGolden(t *testing.T) {
	got := reviewMarkdown(Status{Branch: "main", ManagedFiles: []string{"config/managed.json"}}, " config/managed.json | 2 +-\n", PrepareInput{EnvironmentID: "development"})
	want, err := os.ReadFile(filepath.Join("testdata", "review_no_plan.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("review markdown differs\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestCommitRejectsPathOutsideManagedWorkspace(t *testing.T) {
	manager, err := Open(reviewRepository(t), []string{"config/managed.json"}, source.DefaultGitExecutor)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Commit(context.Background(), CommitInput{Files: []string{"../outside.json"}, Message: "feat(dev): test", IdempotencyKey: "commit-key-0000003"})
	if !errors.Is(err, ErrInvalidManagedFile) {
		t.Fatalf("error = %v", err)
	}
}

func reviewRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "managed.json"), []byte("{\"enabled\":false}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitForReview(t, root, "init", "--quiet")
	runGitForReview(t, root, "config", "user.name", "Conflow Test")
	runGitForReview(t, root, "config", "user.email", "conflow@example.invalid")
	runGitForReview(t, root, "add", ".")
	runGitForReview(t, root, "commit", "--quiet", "-m", "fixture")
	return root
}
func runGitForReview(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}
func gitOutputForReview(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
