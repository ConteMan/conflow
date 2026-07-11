package app

import (
	"bytes"
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
	kind        string
	rollbackOf  string
}

type RollbackRequest struct {
	RollbackPreviewID  string
	ExpectedRemoteETag string
	Confirmation       ReleaseConfirmation
}

var (
	ErrRollbackPreviewInvalid = errors.New("rollback preview is invalid")
	ErrReleaseNotFound        = errors.New("release not found")
	ErrDefaultsFormat         = errors.New("defaults format is invalid")
)

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
	op, err := s.operations.Create("publish")
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
	return releasePreflight{plan: p, environment: environment, remote: current, template: merged, kind: "publish"}, nil
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
		s.failRelease(operationID, preflight, "validating_remote", err, "unchanged")
		return
	}
	publisher, ok := adapter.(provider.Publisher)
	if !ok {
		s.failRelease(operationID, preflight, "validating_remote", provider.ErrNotConfigured, "unchanged")
		return
	}
	_, _ = s.operations.Update(operationID, "running", "validating_remote", nil, nil, "unchanged")
	if err := adapter.Validate(context.Background(), preflight.template); err != nil {
		s.failRelease(operationID, preflight, "validating_remote", err, "unchanged")
		return
	}
	_, _ = s.operations.Update(operationID, "running", "submitting", nil, nil, "unchanged")
	published, err := publisher.Publish(context.Background(), preflight.template, preflight.remote.RemoteETag)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.failRelease(operationID, preflight, "submitting", context.DeadlineExceeded, "unknown")
			return
		}
		if errors.Is(err, provider.ErrETagMismatch) {
			s.failRelease(operationID, preflight, "submitting", provider.ErrETagMismatch, "unchanged")
			return
		}
		s.failRelease(operationID, preflight, "submitting", err, "unchanged")
		return
	}
	_, _ = s.operations.Update(operationID, "running", "verifying", nil, nil, "changed")
	verified, err := adapter.Pull(context.Background())
	if err != nil {
		s.failRelease(operationID, preflight, "verifying", err, "changed")
		return
	}
	after, err := remote.SnapshotFromTemplate(verified.Raw, verified.ETag, verified.Version, verified.ObservedAt)
	if err != nil {
		s.failRelease(operationID, preflight, "verifying", err, "changed")
		return
	}
	if err := s.remote.(*remote.FileStore).Save(preflight.environment.ID, after); err != nil {
		s.failRelease(operationID, preflight, "recording_audit", err, "changed")
		return
	}
	_, _ = s.operations.Update(operationID, "running", "recording_audit", nil, nil, "changed")
	result := release.Release{ReleaseID: release.NewID(), EnvironmentID: preflight.environment.ID, Kind: preflight.kind, Outcome: "succeeded", CreatedAt: time.Now().UTC(), CompletedAt: time.Now().UTC(), OperationID: operationID, RemoteState: "changed", SemanticSummary: fmt.Sprintf("%d direct changes, %d affected entities", len(preflight.plan.SemanticChanges), len(preflight.plan.AffectedEntities)), RiskSummary: preflight.plan.Severity, PlanID: preflight.plan.PlanID, SourceDigest: preflight.plan.SourceDigest, PlanDigest: preflight.plan.ContentDigest, RollbackOfReleaseID: preflight.rollbackOf, RemoteBefore: auditState(preflight.remote), RemoteAfter: ptrAuditState(after)}
	if err := s.releases.SaveWithTemplate(result, verified.Raw); err != nil {
		s.failRelease(operationID, preflight, "recording_audit", err, "changed")
		return
	}
	_, _ = s.operations.Update(operationID, "succeeded", "completed", nil, &operation.Result{ResourceType: "release", ResourceID: result.ReleaseID, Href: "/api/v1/environments/" + preflight.environment.ID + "/releases/" + result.ReleaseID}, "changed")
	_ = published
}

func auditState(snapshot remote.Snapshot) release.AuditState {
	return release.AuditState{RemoteETag: snapshot.RemoteETag, Version: snapshot.Version, ObservedAt: snapshot.ObservedAt, Summary: snapshot.Summary}
}
func ptrAuditState(snapshot remote.Snapshot) *release.AuditState {
	value := auditState(snapshot)
	return &value
}
func (s *Service) failRelease(operationID string, preflight releasePreflight, stage string, err error, remoteState string) {
	failure := releaseFailure(stage, err)
	value := release.Release{ReleaseID: release.NewID(), EnvironmentID: preflight.environment.ID, Kind: preflight.kind, Outcome: "failed", CreatedAt: time.Now().UTC(), CompletedAt: time.Now().UTC(), OperationID: operationID, RemoteState: remoteState, SemanticSummary: fmt.Sprintf("%d direct changes, %d affected entities", len(preflight.plan.SemanticChanges), len(preflight.plan.AffectedEntities)), RiskSummary: preflight.plan.Severity, PlanID: preflight.plan.PlanID, SourceDigest: preflight.plan.SourceDigest, PlanDigest: preflight.plan.ContentDigest, RollbackOfReleaseID: preflight.rollbackOf, RemoteBefore: auditState(preflight.remote), Failure: &failure}
	_ = s.releases.Save(value)
	_, _ = s.operations.Update(operationID, "failed", stage, &failure, nil, remoteState)
}
func releaseFailure(stage string, err error) operation.Failure {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return operation.Failure{Code: "provider_response_unknown", Message: "provider response was not received; verify remote before any new release", Retryable: false, Stage: stage}
	case errors.Is(err, provider.ErrETagMismatch):
		return operation.Failure{Code: "remote_etag_mismatch", Message: "remote configuration changed; rebuild the rollback preview before retrying", Retryable: false, Stage: stage}
	case errors.Is(err, provider.ErrValidation):
		return operation.Failure{Code: "provider_validation_failed", Message: "provider rejected the release template", Retryable: false, Stage: stage}
	default:
		return operation.Failure{Code: "provider_unavailable", Message: "provider operation did not complete", Retryable: false, Stage: stage}
	}
}

func (s *Service) ReleasesPage(_ context.Context, environmentID string, limit int, cursor string) ([]release.Release, error) {
	return s.releases.List(environmentID, limit, cursor)
}
func (s *Service) Releases(ctx context.Context, environmentID string) []release.Release {
	items, _ := s.ReleasesPage(ctx, environmentID, 0, "")
	return items
}
func (s *Service) ReleaseWithError(_ context.Context, id string) (release.Release, bool, error) {
	return s.releases.Get(id)
}
func (s *Service) Release(ctx context.Context, id string) (release.Release, bool) {
	item, ok, _ := s.ReleaseWithError(ctx, id)
	return item, ok
}

// CreateRollbackPreview reads the live provider state before constructing an
// immutable preview. The protected target template is taken from the release
// audit store, never from an API caller.
func (s *Service) CreateRollbackPreview(ctx context.Context, environmentID, releaseID string) (operation.Operation, error) {
	target, ok, err := s.releases.Get(releaseID)
	if err != nil {
		return operation.Operation{}, err
	}
	if !ok || target.EnvironmentID != environmentID || target.Outcome != "succeeded" || target.RemoteAfter == nil {
		return operation.Operation{}, ErrReleaseNotFound
	}
	if _, err := s.releases.Template(releaseID); err != nil {
		return operation.Operation{}, ErrRollbackPreviewInvalid
	}
	if _, _, err := s.GetEnvironment(ctx, environmentID); err != nil {
		return operation.Operation{}, err
	}
	op, err := s.operations.Create("rollback_preview")
	if err != nil {
		return operation.Operation{}, err
	}
	go s.buildRollbackPreview(op.OperationID, environmentID, target)
	return op, nil
}

func (s *Service) buildRollbackPreview(operationID, environmentID string, target release.Release) {
	_, environment, err := s.GetEnvironment(context.Background(), environmentID)
	if err != nil {
		s.failProviderOperation(operationID, "reading_remote", err)
		return
	}
	adapter, err := s.providerFor(environment)
	if err != nil {
		s.failProviderOperation(operationID, "reading_remote", err)
		return
	}
	_, _ = s.operations.Update(operationID, "running", "reading_remote", nil, nil, "unchanged")
	pulled, err := adapter.Pull(context.Background())
	if err != nil {
		s.failProviderOperation(operationID, "reading_remote", err)
		return
	}
	current, err := remote.SnapshotFromTemplate(pulled.Raw, pulled.ETag, pulled.Version, pulled.ObservedAt)
	if err != nil {
		s.failProviderOperation(operationID, "reading_remote", err)
		return
	}
	targetRaw, err := s.releases.Template(target.ReleaseID)
	if err != nil {
		s.failProviderOperation(operationID, "reading_remote", err)
		return
	}
	targetSnapshot, err := remote.SnapshotFromTemplate(targetRaw, target.RemoteAfter.RemoteETag, target.RemoteAfter.Version, target.RemoteAfter.ObservedAt)
	if err != nil {
		s.failProviderOperation(operationID, "reading_remote", err)
		return
	}
	_, _ = s.operations.Update(operationID, "running", "analyzing", nil, nil, "unchanged")
	changes, semantic := rollbackDifferences(current, targetSnapshot)
	manifest, err := s.projects.Snapshot()
	if err != nil {
		s.failProviderOperation(operationID, "analyzing", err)
		return
	}
	input := plan.Input{EnvironmentID: environmentID, EnvironmentKind: environment.Kind, ProductionLowRiskMode: manifest.Manifest.Project.ReleaseConfirmationPolicy.ProductionLowRiskMode}
	requirements := rollbackConfirmationRequirements(input)
	encodedSemantic, _ := json.Marshal(semantic)
	encodedChanges, _ := json.Marshal(changes)
	emptyRisks, _ := json.Marshal([]plan.RiskItem{})
	emptyBlocking, _ := json.Marshal([]plan.BlockingReason{})
	encodedRequirements, _ := json.Marshal(requirements)
	now := time.Now().UTC()
	preview := release.RollbackPreview{RollbackPreviewID: rollbackPreviewID(environmentID, target.ReleaseID, now), EnvironmentID: environmentID, TargetReleaseID: target.ReleaseID, TargetRemoteVersion: target.RemoteAfter.Version, Status: "ready", ExpectedRemoteETag: current.RemoteETag, CreatedAt: now, ExpiresAt: now.Add(plan.DefaultTTL), CurrentRemote: auditState(current), SemanticChanges: encodedSemantic, RemoteParameterChanges: encodedChanges, Severity: "low", RiskItems: emptyRisks, BlockingReasons: emptyBlocking, ConfirmationRequirements: encodedRequirements}
	if err := s.releases.SavePreview(preview); err != nil {
		s.failProviderOperation(operationID, "analyzing", err)
		return
	}
	_, _ = s.operations.Update(operationID, "succeeded", "completed", nil, &operation.Result{ResourceType: "rollback_preview", ResourceID: preview.RollbackPreviewID, Href: "/api/v1/environments/" + environmentID + "/releases/" + target.ReleaseID + "/rollback-preview"}, "unchanged")
}

func rollbackPreviewID(environmentID, releaseID string, now time.Time) string {
	sum := sha256.Sum256([]byte(environmentID + "|" + releaseID + "|" + now.Format(time.RFC3339Nano)))
	return "rbp_" + hex.EncodeToString(sum[:12])
}

func rollbackDifferences(current, target remote.Snapshot) ([]plan.RemoteParameterChange, []plan.SemanticChange) {
	keys := map[string]struct{}{}
	for key := range current.Parameters {
		keys[key] = struct{}{}
	}
	for key := range target.Parameters {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	remoteChanges := make([]plan.RemoteParameterChange, 0, len(ordered))
	semantic := make([]plan.SemanticChange, 0, len(ordered))
	for _, key := range ordered {
		before, beforeOK := current.Parameters[key]
		after, afterOK := target.Parameters[key]
		if beforeOK == afterOK && fmt.Sprint(before) == fmt.Sprint(after) {
			continue
		}
		kind := "updated"
		if !afterOK {
			kind = "deleted"
		} else if !beforeOK {
			kind = "created"
		}
		node := "node_rollback_" + shortDigest(key)
		remoteChanges = append(remoteChanges, plan.RemoteParameterChange{NodeID: node, ProjectionID: "rvp_rollback_" + shortDigest(key), ParameterKey: key, ChangeKind: kind, BeforeSummary: fmt.Sprint(before), AfterSummary: fmt.Sprint(after), Managed: true, CausedBySemanticChangeIDs: []string{node}, AffectedEntityNodeIDs: []string{}})
		semantic = append(semantic, plan.SemanticChange{NodeID: node, ChangeKind: kind, Summary: "remote parameter rollback", BeforeSummary: fmt.Sprint(before), AfterSummary: fmt.Sprint(after), AffectedEntityIDs: []string{}, AffectedEntityNodeIDs: []string{}, RemoteParameterNodeIDs: []string{node}})
	}
	return remoteChanges, semantic
}
func shortDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
func rollbackConfirmationRequirements(in plan.Input) plan.ConfirmationRequirements {
	environmentRequirement := "not_required"
	if in.EnvironmentKind == "production" && in.ProductionLowRiskMode == "environment_id" {
		environmentRequirement = "required"
	}
	return plan.ConfirmationRequirements{RequiresAcknowledgement: true, EnvironmentIDRequirement: environmentRequirement, RequiredRiskItemIDs: []string{}, PolicySource: "project.release_confirmation_policy"}
}

func (s *Service) RollbackPreview(_ context.Context, environmentID, releaseID string) (release.RollbackPreview, error) {
	preview, err := s.releases.Preview(environmentID, releaseID)
	if err != nil {
		return release.RollbackPreview{}, err
	}
	return preview, nil
}

func (s *Service) StartRollback(ctx context.Context, environmentID, releaseID, idempotencyKey string, request RollbackRequest) (operation.Operation, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return operation.Operation{}, ErrIdempotencyRequired
	}
	digest, err := rollbackRequestDigest(request)
	if err != nil {
		return operation.Operation{}, err
	}
	if existing, found, err := s.releases.LookupAction(environmentID, "rollback", idempotencyKey, digest); err != nil {
		return operation.Operation{}, err
	} else if found {
		return s.operations.Get(existing)
	}
	preview, err := s.releases.PreviewByID(request.RollbackPreviewID)
	if err != nil {
		return operation.Operation{}, err
	}
	if preview.EnvironmentID != environmentID || preview.TargetReleaseID != releaseID || preview.Status != "ready" || preview.ExpectedRemoteETag != request.ExpectedRemoteETag {
		return operation.Operation{}, ErrRollbackPreviewInvalid
	}
	target, ok, err := s.releases.Get(releaseID)
	if err != nil {
		return operation.Operation{}, err
	}
	if !ok || target.Outcome != "succeeded" || target.RemoteAfter == nil {
		return operation.Operation{}, ErrReleaseNotFound
	}
	_, environment, err := s.GetEnvironment(ctx, environmentID)
	if err != nil {
		return operation.Operation{}, err
	}
	manifest, err := s.projects.Snapshot()
	if err != nil {
		return operation.Operation{}, err
	}
	var requirements plan.ConfirmationRequirements
	if err := json.Unmarshal(preview.ConfirmationRequirements, &requirements); err != nil {
		return operation.Operation{}, ErrRollbackPreviewInvalid
	}
	if err := validateReleaseConfirmation(plan.Plan{ConfirmationRequirements: requirements}, environment, manifest.Manifest.Project.ReleaseConfirmationPolicy, request.Confirmation); err != nil {
		return operation.Operation{}, err
	}
	adapter, err := s.providerFor(environment)
	if err != nil {
		return operation.Operation{}, err
	}
	if _, ok := adapter.(provider.Publisher); !ok || !adapter.Capabilities().Rollback {
		return operation.Operation{}, provider.ErrNotConfigured
	}
	pulled, err := adapter.Pull(ctx)
	if err != nil {
		return operation.Operation{}, err
	}
	current, err := remote.SnapshotFromTemplate(pulled.Raw, pulled.ETag, pulled.Version, pulled.ObservedAt)
	if err != nil {
		return operation.Operation{}, err
	}
	if current.RemoteETag != preview.ExpectedRemoteETag {
		_ = s.releases.InvalidatePreview(preview.RollbackPreviewID, "remote_etag_changed")
		return operation.Operation{}, &RemoteETagMismatchError{PlanID: preview.RollbackPreviewID, EnvironmentID: environmentID, Expected: preview.ExpectedRemoteETag, Current: current}
	}
	template, err := s.releases.Template(releaseID)
	if err != nil {
		return operation.Operation{}, ErrRollbackPreviewInvalid
	}
	rollbackPlan := plan.Plan{PlanID: preview.RollbackPreviewID, SourceDigest: target.SourceDigest, ContentDigest: target.PlanDigest, Severity: preview.Severity, ConfirmationRequirements: requirements}
	if err := json.Unmarshal(preview.SemanticChanges, &rollbackPlan.SemanticChanges); err != nil {
		return operation.Operation{}, ErrRollbackPreviewInvalid
	}
	if err := json.Unmarshal(preview.RemoteParameterChanges, &rollbackPlan.RemoteParameterChanges); err != nil {
		return operation.Operation{}, ErrRollbackPreviewInvalid
	}
	s.releaseMu.Lock()
	defer s.releaseMu.Unlock()
	if existing, found, err := s.releases.LookupAction(environmentID, "rollback", idempotencyKey, digest); err != nil {
		return operation.Operation{}, err
	} else if found {
		return s.operations.Get(existing)
	}
	op, err := s.operations.Create("rollback")
	if err != nil {
		return operation.Operation{}, err
	}
	if _, err := s.releases.ReserveAction(environmentID, "rollback", idempotencyKey, digest, op.OperationID); err != nil {
		return operation.Operation{}, err
	}
	preflight := releasePreflight{plan: rollbackPlan, environment: environment, remote: current, template: template, kind: "rollback", rollbackOf: releaseID}
	go s.publishRelease(op.OperationID, preflight)
	return op, nil
}

func rollbackRequestDigest(request RollbackRequest) (string, error) {
	return releaseRequestDigest(ReleaseRequest{PlanID: request.RollbackPreviewID, ExpectedRemoteETag: request.ExpectedRemoteETag, Confirmation: request.Confirmation})
}

// Defaults exports values from the protected current remote snapshot. These
// values are client defaults already intended for distribution; credentials
// and provider tokens are not part of the snapshot or this format.
func (s *Service) Defaults(_ context.Context, environmentID, format string) ([]byte, string, string, error) {
	if _, _, err := s.GetEnvironment(context.Background(), environmentID); err != nil {
		return nil, "", "", err
	}
	snapshot, err := s.remote.Current(environmentID)
	if err != nil {
		return nil, "", "", err
	}
	if snapshot.Status != "available" {
		return nil, "", "", ErrRollbackPreviewInvalid
	}
	digest := defaultsDigest(snapshot.Parameters)
	metadata := map[string]string{"source_version": snapshot.Version, "source_etag": snapshot.RemoteETag, "digest": digest}
	switch format {
	case "json":
		content, err := json.MarshalIndent(map[string]any{"metadata": metadata, "defaults": snapshot.Parameters}, "", "  ")
		if err != nil {
			return nil, "", "", err
		}
		return append(content, '\n'), "defaults.json", "application/json", nil
	case "xml":
		return defaultsXML(metadata, snapshot.Parameters), "defaults.xml", "application/xml", nil
	case "plist":
		return defaultsPlist(metadata, snapshot.Parameters), "defaults.plist", "application/x-plist", nil
	default:
		return nil, "", "", ErrDefaultsFormat
	}
}
func defaultsDigest(values map[string]any) string {
	content, _ := json.Marshal(values)
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func escapeXML(value string) string { var b bytes.Buffer; _ = xmlEscape(&b, value); return b.String() }
func xmlEscape(b *bytes.Buffer, value string) error {
	for _, r := range value {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case 39:
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return nil
}
func defaultsXML(metadata map[string]string, values map[string]any) []byte {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<defaults")
	for _, key := range []string{"source_version", "source_etag", "digest"} {
		b.WriteString(" ")
		b.WriteString(key)
		b.WriteString("=\"")
		b.WriteString(escapeXML(metadata[key]))
		b.WriteString("\"")
	}
	b.WriteString(">\n")
	for _, key := range keys {
		b.WriteString("  <entry key=\"")
		b.WriteString(escapeXML(key))
		b.WriteString("\" value=\"")
		b.WriteString(escapeXML(fmt.Sprint(values[key])))
		b.WriteString("\"/>\n")
	}
	b.WriteString("</defaults>\n")
	return []byte(b.String())
}
func defaultsPlist(metadata map[string]string, values map[string]any) []byte {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n<key>_conflow_metadata</key><dict>")
	for _, key := range []string{"source_version", "source_etag", "digest"} {
		b.WriteString("<key>")
		b.WriteString(key)
		b.WriteString("</key><string>")
		b.WriteString(escapeXML(metadata[key]))
		b.WriteString("</string>")
	}
	b.WriteString("</dict>\n")
	for _, key := range keys {
		b.WriteString("<key>")
		b.WriteString(escapeXML(key))
		b.WriteString("</key><string>")
		b.WriteString(escapeXML(fmt.Sprint(values[key])))
		b.WriteString("</string>\n")
	}
	b.WriteString("</dict></plist>\n")
	return []byte(b.String())
}
