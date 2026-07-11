package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ConteMan/conflow/internal/operation"
	"github.com/ConteMan/conflow/internal/plan"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/provider"
	"github.com/ConteMan/conflow/internal/release"
	"github.com/ConteMan/conflow/internal/remote"
)

var (
	ErrConfirmationInvalid = errors.New("release confirmation is invalid")
	ErrIdempotencyRequired = errors.New("idempotency key is required")
)

// ReleaseRequest is the frozen POST /releases body expressed independently of
// HTTP. Risk severity and environment kind are intentionally absent.
type ReleaseRequest struct {
	PlanID                string
	ExpectedDraftRevision uint64
	ExpectedRemoteETag    string
	Confirmation          ReleaseConfirmation
}
type ReleaseConfirmation struct {
	Acknowledged            bool
	EnvironmentID           string
	AcknowledgedRiskItemIDs []string
}

type releasePreflight struct {
	plan        plan.Plan
	environment project.Environment
	remote      remote.Snapshot
	template    []byte
}

func (s *Service) StartRelease(ctx context.Context, environmentID, idempotencyKey string, request ReleaseRequest) (operation.Operation, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return operation.Operation{}, ErrIdempotencyRequired
	}
	digest, err := releaseRequestDigest(request)
	if err != nil {
		return operation.Operation{}, err
	}
	// A completed replay must keep returning its original terminal Operation,
	// even though that successful publish intentionally made its old Plan stale.
	// New requests still run the contractual preflight sequence below.
	s.releaseMu.Lock()
	existing, found, err := s.releases.Lookup(environmentID, idempotencyKey, digest)
	s.releaseMu.Unlock()
	if err != nil {
		return operation.Operation{}, err
	}
	if found {
		return s.operations.Get(existing)
	}
	preflight, err := s.preflightRelease(ctx, environmentID, request)
	if err != nil {
		return operation.Operation{}, err
	}
	s.releaseMu.Lock()
	defer s.releaseMu.Unlock()
	if existing, found, err := s.releases.Lookup(environmentID, idempotencyKey, digest); err != nil {
		return operation.Operation{}, err
	} else if found {
		return s.operations.Get(existing)
	}
	op, err := s.operations.Create("release")
	if err != nil {
		return operation.Operation{}, err
	}
	operationID, err := s.releases.Reserve(environmentID, idempotencyKey, digest, op.OperationID)
	if err != nil {
		return operation.Operation{}, err
	}
	if operationID != op.OperationID {
		return s.operations.Get(operationID)
	}
	go s.publishRelease(op.OperationID, preflight)
	return op, nil
}

func (s *Service) preflightRelease(ctx context.Context, environmentID string, request ReleaseRequest) (releasePreflight, error) {
	p, err := s.plans.Get(request.PlanID)
	if err != nil {
		return releasePreflight{}, err
	}
	// 1. path environment <-> Plan environment.
	if p.EnvironmentID != environmentID {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: "environment_changed"}
	}
	_, environment, err := s.GetEnvironment(ctx, environmentID)
	if err != nil {
		return releasePreflight{}, err
	}
	// 2. ready and not expired.
	if p.Status != "ready" || !time.Now().UTC().Before(p.ExpiresAt) {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: "plan_not_ready"}
	}
	// 3. immutable snapshot identity, draft revision, source digest.
	if p.SnapshotToken == "" {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: "snapshot_token_missing"}
	}
	if request.ExpectedDraftRevision != p.DraftRevision {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: "draft_revision_changed"}
	}
	_, revision, err := s.GetDraft(ctx, environmentID)
	if err != nil {
		return releasePreflight{}, err
	}
	if revision != p.DraftRevision {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: "draft_revision_changed"}
	}
	info, err := s.SourceInfo(ctx)
	if err != nil {
		return releasePreflight{}, err
	}
	if info.Status.Digest != p.SourceDigest {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: "source_digest_changed"}
	}
	// 4. Current provider ETag, never a stale local cache ETag.
	if p.RemoteETag == nil || request.ExpectedRemoteETag != *p.RemoteETag {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: "remote_etag_changed"}
	}
	adapter, err := s.providerFor(environment)
	if err != nil {
		return releasePreflight{}, err
	}
	publisher, ok := adapter.(provider.Publisher)
	if !ok || !adapter.Capabilities().Publish {
		return releasePreflight{}, provider.ErrNotConfigured
	}
	pulled, err := adapter.Pull(ctx)
	if err != nil {
		return releasePreflight{}, err
	}
	current, err := remote.SnapshotFromTemplate(pulled.Raw, pulled.ETag, pulled.Version, pulled.ObservedAt)
	if err != nil {
		return releasePreflight{}, err
	}
	if current.RemoteETag != *p.RemoteETag {
		return releasePreflight{}, &RemoteETagMismatchError{PlanID: p.PlanID, EnvironmentID: environmentID, Expected: *p.RemoteETag, Current: current}
	}
	// 5. Plan readiness and defensive current-template condition check.
	if len(p.BlockingReasons) > 0 {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: p.BlockingReasons[0].ReasonCode}
	}
	if current.Summary == nil || current.Summary.HasUnmodeledConditions {
		return releasePreflight{}, &PlanInvalidatedError{PlanID: p.PlanID, Reason: "unmodeled_remote_condition"}
	}
	// 6. only server-produced requirements and the Project policy are trusted.
	manifest, err := s.projects.Snapshot()
	if err != nil {
		return releasePreflight{}, err
	}
	if err := validateReleaseConfirmation(p, environment, manifest.Manifest.Project.ReleaseConfirmationPolicy, request.Confirmation); err != nil {
		return releasePreflight{}, err
	}
	input, _, err := s.plans.Artifact(p.PlanID, "provider-input.json")
	if err != nil {
		return releasePreflight{}, err
	}
	merged, err := plan.MergeFirebaseTemplate(pulled.Raw, input, p.RemoteParameterChanges)
	if err != nil {
		return releasePreflight{}, err
	}
	_ = publisher
	return releasePreflight{plan: p, environment: environment, remote: current, template: merged}, nil
}

func validateReleaseConfirmation(p plan.Plan, environment project.Environment, policy project.ReleaseConfirmationPolicy, confirmation ReleaseConfirmation) error {
	requirements := p.ConfirmationRequirements
	if requirements.RequiresAcknowledgement && !confirmation.Acknowledged {
		return ErrConfirmationInvalid
	}
	required := setOf(requirements.RequiredRiskItemIDs)
	provided := setOf(confirmation.AcknowledgedRiskItemIDs)
	if len(provided) != len(confirmation.AcknowledgedRiskItemIDs) || len(required) != len(provided) {
		return ErrConfirmationInvalid
	}
	for id := range provided {
		if _, ok := required[id]; !ok {
			return ErrConfirmationInvalid
		}
	}
	// Current policy can only affect the low-risk production ID field. Any Plan
	// required risk item remains authoritative and can never be relaxed.
	requiresEnvironmentID := requirements.EnvironmentIDRequirement == "required"
	if environment.Kind == "production" && len(required) == 0 && policy.ProductionLowRiskMode == "environment_id" {
		requiresEnvironmentID = true
	}
	if requiresEnvironmentID && confirmation.EnvironmentID != environment.ID {
		return ErrConfirmationInvalid
	}
	return nil
}

func setOf(values []string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func releaseRequestDigest(request ReleaseRequest) (string, error) {
	riskIDs := append([]string(nil), request.Confirmation.AcknowledgedRiskItemIDs...)
	sort.Strings(riskIDs)
	payload := struct {
		PlanID       string `json:"plan_id"`
		Draft        uint64 `json:"expected_draft_revision"`
		ETag         string `json:"expected_remote_etag"`
		Confirmation struct {
			Acknowledged  bool     `json:"acknowledged"`
			EnvironmentID string   `json:"environment_id"`
			RiskIDs       []string `json:"acknowledged_risk_item_ids"`
		} `json:"confirmation"`
	}{PlanID: request.PlanID, Draft: request.ExpectedDraftRevision, ETag: request.ExpectedRemoteETag}
	payload.Confirmation.Acknowledged, payload.Confirmation.EnvironmentID, payload.Confirmation.RiskIDs = request.Confirmation.Acknowledged, request.Confirmation.EnvironmentID, riskIDs
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) publishRelease(operationID string, preflight releasePreflight) {
	adapter, err := s.providerFor(preflight.environment)
	if err != nil {
		s.failProviderOperation(operationID, "validating_remote", err)
		return
	}
	publisher, ok := adapter.(provider.Publisher)
	if !ok {
		s.failProviderOperation(operationID, "validating_remote", provider.ErrNotConfigured)
		return
	}
	_, _ = s.operations.Update(operationID, "running", "validating_remote", nil, nil, "unchanged")
	if err := adapter.Validate(context.Background(), preflight.template); err != nil {
		s.failProviderOperation(operationID, "validating_remote", err)
		return
	}
	_, _ = s.operations.Update(operationID, "running", "submitting", nil, nil, "unchanged")
	published, err := publisher.Publish(context.Background(), preflight.template, preflight.remote.RemoteETag)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_, _ = s.operations.Update(operationID, "failed", "submitting", &operation.Failure{Code: "provider_response_unknown", Message: "provider response was not received; verify remote before any new publish", Retryable: false, Stage: "submitting"}, nil, "unknown")
			return
		}
		if errors.Is(err, provider.ErrETagMismatch) {
			_, _ = s.operations.Update(operationID, "failed", "submitting", &operation.Failure{Code: "remote_etag_mismatch", Message: "remote configuration changed; rebuild the Plan before publishing", Retryable: false, Stage: "submitting"}, nil, "unchanged")
			return
		}
		s.failProviderOperation(operationID, "submitting", err)
		return
	}
	_, _ = s.operations.Update(operationID, "running", "verifying", nil, nil, "changed")
	verified, err := adapter.Pull(context.Background())
	if err != nil {
		s.failReleaseAfterWrite(operationID, "verifying", err)
		return
	}
	after, err := remote.SnapshotFromTemplate(verified.Raw, verified.ETag, verified.Version, verified.ObservedAt)
	if err != nil {
		s.failReleaseAfterWrite(operationID, "verifying", err)
		return
	}
	if err := s.remote.(*remote.FileStore).Save(preflight.environment.ID, after); err != nil {
		s.failReleaseAfterWrite(operationID, "recording_audit", err)
		return
	}
	_, _ = s.operations.Update(operationID, "running", "recording_audit", nil, nil, "changed")
	result := release.Release{ReleaseID: release.NewID(), EnvironmentID: preflight.environment.ID, Kind: "publish", Outcome: "succeeded", CreatedAt: time.Now().UTC(), CompletedAt: time.Now().UTC(), OperationID: operationID, RemoteState: "changed", SemanticSummary: fmt.Sprintf("%d direct changes, %d affected entities", len(preflight.plan.SemanticChanges), len(preflight.plan.AffectedEntities)), RiskSummary: preflight.plan.Severity, PlanID: preflight.plan.PlanID, SourceDigest: preflight.plan.SourceDigest, PlanDigest: preflight.plan.ContentDigest, RemoteBefore: auditState(preflight.remote), RemoteAfter: auditState(after)}
	if err := s.releases.Save(result); err != nil {
		s.failReleaseAfterWrite(operationID, "recording_audit", err)
		return
	}
	_, _ = s.operations.Update(operationID, "succeeded", "completed", nil, &operation.Result{ResourceType: "release", ResourceID: result.ReleaseID, Href: "/api/v1/environments/" + preflight.environment.ID + "/releases/" + result.ReleaseID}, "changed")
	_ = published
}

func auditState(snapshot remote.Snapshot) release.AuditState {
	return release.AuditState{RemoteETag: snapshot.RemoteETag, Version: snapshot.Version, ObservedAt: snapshot.ObservedAt, Summary: snapshot.Summary}
}
func (s *Service) failReleaseAfterWrite(operationID, stage string, err error) {
	_, _ = s.operations.Update(operationID, "failed", stage, &operation.Failure{Code: "provider_unavailable", Message: "remote was updated but confirmation or audit did not complete", Retryable: false, Stage: stage}, nil, "changed")
}

func (s *Service) Releases(_ context.Context, environmentID string) []release.Release {
	return s.releases.List(environmentID)
}
func (s *Service) Release(_ context.Context, id string) (release.Release, bool) {
	return s.releases.Get(id)
}
