package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/plan"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/provider"
	"github.com/ConteMan/conflow/internal/release"
)

func TestReleaseSuccessReplayConflictAndAudit(t *testing.T) {
	s, fake := releaseService(t)
	p := buildReadyPlan(t, s)
	request := releaseRequest(p)
	started, err := s.StartRelease(context.Background(), "development", "publish-key-0000001", request)
	if err != nil {
		t.Fatal(err)
	}
	completed := waitAppOperation(t, s, started.OperationID)
	if completed.Status != "succeeded" || completed.RemoteState != "changed" || completed.Result == nil {
		t.Fatalf("operation=%#v", completed)
	}
	if len(fake.published) == 0 {
		t.Fatal("provider did not receive publish")
	}
	if records := s.Releases(context.Background(), "development"); len(records) != 1 || records[0].OperationID != started.OperationID || records[0].Outcome != "succeeded" {
		t.Fatalf("audit=%#v", records)
	}
	replayed, err := s.StartRelease(context.Background(), "development", "publish-key-0000001", request)
	if err != nil || replayed.OperationID != started.OperationID || replayed.Status != "succeeded" {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	request.ExpectedDraftRevision++
	_, err = s.StartRelease(context.Background(), "development", "publish-key-0000001", request)
	if !errors.Is(err, release.ErrIdempotencyConflict) {
		t.Fatalf("conflict=%v", err)
	}
}

func TestReleaseETagMismatchDoesNotPublish(t *testing.T) {
	s, fake := releaseService(t)
	p := buildReadyPlan(t, s)
	fake.template.ETag = "etag-current"
	_, err := s.StartRelease(context.Background(), "development", "publish-key-0000002", releaseRequest(p))
	var mismatch *RemoteETagMismatchError
	if !errors.As(err, &mismatch) || mismatch.Current.RemoteETag != "etag-current" || len(fake.published) != 0 {
		t.Fatalf("err=%#v published=%s", err, fake.published)
	}
}

func TestReleaseTimeoutMakesRemoteStateUnknown(t *testing.T) {
	s, fake := releaseService(t)
	fake.publishErr = context.DeadlineExceeded
	p := buildReadyPlan(t, s)
	started, err := s.StartRelease(context.Background(), "development", "publish-key-0000003", releaseRequest(p))
	if err != nil {
		t.Fatal(err)
	}
	completed := waitAppOperation(t, s, started.OperationID)
	if completed.Status != "failed" || completed.RemoteState != "unknown" || completed.Failure == nil || completed.Failure.Code != "provider_response_unknown" {
		t.Fatalf("operation=%#v", completed)
	}
	if len(s.Releases(context.Background(), "development")) != 0 {
		t.Fatal("failed release wrote successful audit")
	}
}

func TestReleaseBlockingPlanCannotBeConfirmedAway(t *testing.T) {
	s, fake := releaseService(t)
	p := buildReadyPlan(t, s)
	p.BlockingReasons = []plan.BlockingReason{{ReasonCode: "unmodeled_remote_condition", Summary: "blocked"}}
	if err := s.plans.Update(p); err != nil {
		t.Fatal(err)
	}
	_, err := s.StartRelease(context.Background(), "development", "publish-key-0000004", releaseRequest(p))
	var invalidated *PlanInvalidatedError
	if !errors.As(err, &invalidated) || len(fake.published) != 0 {
		t.Fatalf("err=%#v published=%s", err, fake.published)
	}
}

func TestReleaseConfirmationPolicyAndRiskSets(t *testing.T) {
	base := plan.Plan{ConfirmationRequirements: plan.ConfirmationRequirements{RequiresAcknowledgement: true, RequiredRiskItemIDs: []string{}}}
	production := project.Environment{ID: "production", Kind: "production"}
	lowAcknowledged := ReleaseConfirmation{Acknowledged: true}
	if err := validateReleaseConfirmation(base, production, project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "acknowledgement"}, lowAcknowledged); err != nil {
		t.Fatalf("low acknowledgement=%v", err)
	}
	if err := validateReleaseConfirmation(base, production, project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "environment_id"}, lowAcknowledged); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("low environment id=%v", err)
	}
	base.ConfirmationRequirements.RequiredRiskItemIDs = []string{"risk_high"}
	if err := validateReleaseConfirmation(base, production, project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "acknowledgement"}, ReleaseConfirmation{Acknowledged: true, AcknowledgedRiskItemIDs: []string{"risk_high"}}); err != nil {
		t.Fatalf("high risk=%v", err)
	}
	if err := validateReleaseConfirmation(base, production, project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "acknowledgement"}, ReleaseConfirmation{Acknowledged: true, AcknowledgedRiskItemIDs: []string{"unknown"}}); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("unknown risk=%v", err)
	}
}

func TestReleaseContractFixtureScenariosAreConsumed(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "mobile-ad-monetization", "v1", "plan-risk-operation-rollback.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Scenarios []struct {
			ID                     string `json:"id"`
			ExpectedPublishRequest struct {
				IdempotencyKey string `json:"idempotency_key"`
				Confirmation   struct {
					AcknowledgedRiskItemIDs []string `json:"acknowledged_risk_item_ids"`
				} `json:"confirmation"`
			} `json:"expected_publish_request"`
			Expected struct {
				ErrorCode        string `json:"error_code"`
				RemoteUpdateSent bool   `json:"remote_update_sent"`
			} `json:"expected"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(content, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Scenarios) < 3 || fixture.Scenarios[0].ExpectedPublishRequest.IdempotencyKey == "" || len(fixture.Scenarios[0].ExpectedPublishRequest.Confirmation.AcknowledgedRiskItemIDs) == 0 {
		t.Fatalf("publish fixture=%#v", fixture.Scenarios)
	}
	if fixture.Scenarios[1].Expected.ErrorCode != "remote_etag_mismatch" || fixture.Scenarios[1].Expected.RemoteUpdateSent {
		t.Fatalf("etag fixture=%#v", fixture.Scenarios[1])
	}
}

func releaseService(t *testing.T) (*Service, *fakeFirebase) {
	t.Helper()
	workspace := t.TempDir()
	if _, err := project.CreateExample(workspace); err != nil {
		t.Fatal(err)
	}
	fake := &fakeFirebase{template: providerTemplate("etag-before")}
	s, err := OpenWithPacksAndProviderFactory(workspace, packs.BuiltinRegistry(), func(project.Environment) (provider.Adapter, error) { return fake, nil })
	if err != nil {
		t.Fatal(err)
	}
	return s, fake
}

func providerTemplate(etag string) provider.Template {
	return provider.Template{ETag: etag, Version: "1", ObservedAt: time.Now().UTC(), Raw: []byte(`{"parameters":{"unmanaged":{"defaultValue":{"value":"keep"}}}}`)}
}
func releaseRequest(p plan.Plan) ReleaseRequest {
	return ReleaseRequest{PlanID: p.PlanID, ExpectedDraftRevision: p.DraftRevision, ExpectedRemoteETag: *p.RemoteETag, Confirmation: ReleaseConfirmation{Acknowledged: true, AcknowledgedRiskItemIDs: append([]string(nil), p.ConfirmationRequirements.RequiredRiskItemIDs...)}}
}
