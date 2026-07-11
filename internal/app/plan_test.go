package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/plan"
	"github.com/ConteMan/conflow/internal/project"
)

func TestPlanInvalidationInputsAndTTL(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, service *Service, workspace string, planID string)
	}{
		{"draft revision", func(t *testing.T, s *Service, _ string, _ string) {
			view, revision, err := s.GetDraft(context.Background(), "development")
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = s.MutateDraft(context.Background(), "development", draft.Mutation{ExpectedRevision: revision, ExpectedSourceRevision: view.SourceRevision, Scope: draft.ScopeBaseline, Action: "reset"})
			if err != nil {
				t.Fatal(err)
			}
		}},
		{"source digest", func(t *testing.T, _ *Service, workspace, _ string) {
			if err := os.WriteFile(filepath.Join(workspace, ".conflow", "data", "base.yaml"), []byte("feature_switches: []\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"remote etag", func(t *testing.T, _ *Service, workspace, _ string) { writeRemote(t, workspace, "etag-2") }},
		{"ttl", func(t *testing.T, s *Service, _ string, id string) {
			p, err := s.plans.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			p.ExpiresAt = time.Now().UTC().Add(-time.Second)
			if err := s.plans.Update(p); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, workspace := planService(t)
			p := buildReadyPlan(t, s)
			test.change(t, s, workspace, p.PlanID)
			got, err := s.GetPlan(context.Background(), p.PlanID)
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "ttl" {
				if got.Status != "expired" || got.InvalidationReason != "ttl_expired" {
					t.Fatalf("ttl result %#v", got)
				}
				return
			}
			if got.Status != "invalidated" {
				t.Fatalf("status=%s, want invalidated (%#v)", got.Status, got)
			}
		})
	}
}

func TestPlanOperationPersistsAndCanBeReadAfterReopen(t *testing.T) {
	s, workspace := planService(t)
	p := buildReadyPlan(t, s)
	op, err := s.StartPlan(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Second); ; {
		current, err := s.Operation(context.Background(), op.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == "succeeded" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("operation timeout")
		}
		time.Sleep(time.Millisecond)
	}
	reopened, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.GetPlan(context.Background(), p.PlanID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Operation(context.Background(), op.OperationID); err != nil {
		t.Fatal(err)
	}
}

func TestPlanPublishPreflightSeparatesRemoteETag(t *testing.T) {
	s, workspace := planService(t)
	p := buildReadyPlan(t, s)
	writeRemote(t, workspace, "etag-2")
	_, err := s.CheckPlanForPublish(context.Background(), p.PlanID)
	var mismatch *RemoteETagMismatchError
	if !errors.As(err, &mismatch) || mismatch.Expected != "etag-1" {
		t.Fatalf("error = %#v, want remote etag mismatch", err)
	}
}

func TestPlanPublishPreflightRejectsBlockingRisk(t *testing.T) {
	s, _ := planService(t)
	p := buildReadyPlan(t, s)
	p.BlockingReasons = []plan.BlockingReason{{ReasonCode: "validation_not_ready", Summary: "完整校验尚未就绪"}}
	if err := s.plans.Update(p); err != nil {
		t.Fatal(err)
	}
	_, err := s.CheckPlanForPublish(context.Background(), p.PlanID)
	var invalidated *PlanInvalidatedError
	if !errors.As(err, &invalidated) || invalidated.Reason != "validation_not_ready" {
		t.Fatalf("error = %#v, want validation blocker", err)
	}
}

func planService(t *testing.T) (*Service, string) {
	t.Helper()
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	writeRemote(t, workspace, "etag-1")
	s, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	return s, workspace
}

func buildReadyPlan(t *testing.T, s *Service) plan.Plan {
	t.Helper()
	op, err := s.StartPlan(context.Background(), "development")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		current, err := s.Operation(context.Background(), op.OperationID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == "succeeded" {
			returnPlan, err := s.GetPlan(context.Background(), current.Result.ResourceID)
			if err != nil {
				t.Fatal(err)
			}
			return returnPlan
		}
		if current.Status == "failed" {
			t.Fatalf("operation failed: %#v", current.Failure)
		}
		if time.Now().After(deadline) {
			t.Fatal("operation timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func writeRemote(t *testing.T, workspace, etag string) {
	t.Helper()
	directory := filepath.Join(workspace, ".conflow", "remote-snapshots")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(map[string]any{"remote_etag": etag, "version": "1", "observed_at": "2026-07-11T10:00:00Z", "summary": map[string]any{"parameter_count": 0, "managed_parameter_count": 0, "condition_count": 0, "content_digest": "sha256:test"}, "parameters": map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "development.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
}
