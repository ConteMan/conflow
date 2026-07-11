package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ConteMan/conflow/internal/draft"
	"github.com/ConteMan/conflow/internal/operation"
	"github.com/ConteMan/conflow/internal/packs"
	"github.com/ConteMan/conflow/internal/plan"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/ConteMan/conflow/internal/provider"
	"github.com/ConteMan/conflow/internal/remote"
	"github.com/ConteMan/conflow/internal/source"
	"github.com/ConteMan/conflow/internal/validation"
)

var (
	ErrLastEnvironment         = errors.New("cannot delete the last environment")
	ErrPackRegistryUnavailable = errors.New("pack registry is unavailable")
)

// PlanInvalidatedError is the 409 precondition domain for local inputs.
// Remote ETag conflicts intentionally use RemoteETagMismatchError instead.
type PlanInvalidatedError struct{ PlanID, Reason string }

func (e *PlanInvalidatedError) Error() string { return "plan invalidated: " + e.Reason }

// RemoteETagMismatchError is the 412 domain used by a publish preflight.
// It contains only the protected remote summary, never provider credentials.
type RemoteETagMismatchError struct {
	PlanID, Expected string
	Current          remote.Snapshot
}

func (e *RemoteETagMismatchError) Error() string { return "remote etag mismatch" }

type Service struct {
	projects     *project.Store
	packRegistry *packs.Registry
	drafts       *draft.Store
	source       source.Adapter
	validations  *validation.Store
	operations   *operation.Store
	plans        *plan.Store
	remote       remote.Store
	workspace    string
	providerFor  func(project.Environment) (provider.Adapter, error)
}

func Initialize(workspace string) (string, error) {
	return project.CreateExample(workspace)
}

func Open(workspace string) (*Service, error) {
	return OpenWithPacks(workspace, packs.BuiltinRegistry())
}

func OpenWithPacks(workspace string, registry *packs.Registry) (*Service, error) {
	return OpenWithPacksAndProviderFactory(workspace, registry, nil)
}

// OpenWithPacksAndProviderFactory keeps the application orchestration
// testable without allowing provider protocol details into the app package.
func OpenWithPacksAndProviderFactory(workspace string, registry *packs.Registry, factory func(project.Environment) (provider.Adapter, error)) (*Service, error) {
	if registry == nil {
		return nil, ErrPackRegistryUnavailable
	}
	store, err := project.Open(workspace)
	if err != nil {
		return nil, err
	}
	manifest, err := store.Snapshot()
	if err != nil {
		return nil, err
	}
	if manifest.Manifest.Source.Type != "managed-file" {
		return nil, fmt.Errorf("unsupported source adapter %q", manifest.Manifest.Source.Type)
	}
	managed := source.OpenManagedFile(workspace)
	draftStore, err := draft.Open(filepath.Join(workspace, ".conflow", "draft.json"), func() (draft.SourceSnapshot, error) {
		snapshot, snapshotErr := managed.Load()
		if snapshotErr != nil {
			return draft.SourceSnapshot{}, snapshotErr
		}
		return draftSourceSnapshot(snapshot), nil
	})
	if err != nil {
		return nil, err
	}
	validationStore, err := validation.Open(filepath.Join(workspace, ".conflow", "validation-results.json"))
	if err != nil {
		return nil, err
	}
	operationStore, err := operation.Open(filepath.Join(workspace, ".conflow", "operations.json"))
	if err != nil {
		return nil, err
	}
	planStore, err := plan.Open(filepath.Join(workspace, ".conflow", "plans"))
	if err != nil {
		return nil, err
	}
	if factory == nil {
		factory = func(environment project.Environment) (provider.Adapter, error) {
			credentialPath, configErr := provider.LoadCredentialReference(workspace, environment.ID)
			if configErr != nil {
				return nil, configErr
			}
			return provider.NewFirebase(provider.FirebaseConfig{ProjectID: environment.Provider.ProjectID, CredentialsPath: credentialPath}), nil
		}
	}
	return &Service{projects: store, packRegistry: registry, drafts: draftStore, source: managed, validations: validationStore, operations: operationStore, plans: planStore, remote: remote.OpenFileStore(workspace), workspace: workspace, providerFor: factory}, nil
}

func (s *Service) Snapshot(_ context.Context) (project.Snapshot, error) {
	return s.projects.Snapshot()
}

func (s *Service) UpdateProject(_ context.Context, expectedRevision uint64, metadata project.Project) (project.Snapshot, error) {
	return s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		manifest.Project = metadata
		return nil
	})
}

func (s *Service) CreateEnvironment(_ context.Context, expectedRevision uint64, environment project.Environment) (project.Snapshot, project.Environment, error) {
	snapshot, err := s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		if slices.ContainsFunc(manifest.Environments, func(current project.Environment) bool {
			return current.ID == environment.ID
		}) {
			return project.ErrAlreadyExists
		}
		manifest.Environments = append(manifest.Environments, environment)
		return nil
	})
	return snapshot, environment, err
}

func (s *Service) GetEnvironment(ctx context.Context, environmentID string) (project.Snapshot, project.Environment, error) {
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return project.Snapshot{}, project.Environment{}, err
	}
	for _, environment := range snapshot.Manifest.Environments {
		if environment.ID == environmentID {
			return snapshot, environment, nil
		}
	}
	return snapshot, project.Environment{}, project.ErrNotFound
}

func (s *Service) UpdateEnvironment(_ context.Context, expectedRevision uint64, environmentID string, replacement project.Environment) (project.Snapshot, project.Environment, error) {
	snapshot, err := s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		for index := range manifest.Environments {
			if manifest.Environments[index].ID == environmentID {
				replacement.ID = environmentID
				replacement.Kind = manifest.Environments[index].Kind
				manifest.Environments[index] = replacement
				return nil
			}
		}
		return project.ErrNotFound
	})
	if err != nil {
		return snapshot, project.Environment{}, err
	}
	return snapshot, replacement, nil
}

func (s *Service) DeleteEnvironment(_ context.Context, expectedRevision uint64, environmentID string) (project.Snapshot, error) {
	return s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		if len(manifest.Environments) == 1 && manifest.Environments[0].ID == environmentID {
			return ErrLastEnvironment
		}
		for index := range manifest.Environments {
			if manifest.Environments[index].ID == environmentID {
				manifest.Environments = slices.Delete(manifest.Environments, index, index+1)
				return nil
			}
		}
		return project.ErrNotFound
	})
}

func (s *Service) ListPacks(_ context.Context) packs.Snapshot {
	return s.packRegistry.List()
}

func (s *Service) GetPack(_ context.Context, name, version string) (packs.Definition, uint64, error) {
	return s.packRegistry.Get(name, version)
}

func (s *Service) GetPackSchema(_ context.Context, name, version string, requestedVersion *uint64) (packs.Schema, uint64, error) {
	return s.packRegistry.Schema(name, version, requestedVersion)
}

func (s *Service) GetDraft(_ context.Context, environmentID string) (draft.View, uint64, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return draft.View{}, 0, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return draft.View{}, 0, err
	}
	return s.drafts.View(schema, environments, environmentID)
}

func (s *Service) MutateDraft(_ context.Context, environmentID string, mutation draft.Mutation) (draft.View, uint64, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return draft.View{}, 0, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return draft.View{}, 0, err
	}
	return s.drafts.Mutate(schema, environments, environmentID, mutation)
}

type SourceInfo struct {
	Type         string
	Capabilities source.Capabilities
	Status       source.Status
}

func (s *Service) SourceInfo(_ context.Context) (SourceInfo, error) {
	status, err := s.source.Status()
	if err != nil {
		return SourceInfo{}, err
	}
	return SourceInfo{Type: status.Type, Capabilities: s.source.Capabilities(), Status: status}, nil
}

// SaveDraft writes the resolved source replacement for the selected view and
// then consumes that view's draft replacements. The draft Store retains the
// existing lock/snapshot boundary from Spec 004.
func (s *Service) SaveDraft(_ context.Context, environmentID string, expectedRevision uint64, expectedSourceRevision string) (draft.View, uint64, error) {
	snapshot, err := s.projects.Snapshot()
	if err != nil {
		return draft.View{}, 0, err
	}
	schema, environments, err := s.draftSchema(snapshot.Manifest)
	if err != nil {
		return draft.View{}, 0, err
	}
	return s.drafts.SaveToSource(schema, environments, environmentID, draft.Save{
		ExpectedRevision: expectedRevision, ExpectedSourceRevision: expectedSourceRevision,
		Commit: func(current draft.SourceSnapshot, state draft.State) (draft.SourceSnapshot, error) {
			baseline := current.Baseline
			if state.BaselinePresent {
				baseline = state.Baseline
			}
			override, present := current.EnvironmentOverrides[environmentID]
			if replacement, exists := state.EnvironmentOverrides[environmentID]; exists {
				override, present = replacement, true
			}
			if !present {
				override = nil
			}
			next, saveErr := s.source.Save(source.SaveInput{ExpectedRevision: current.Revision, EnvironmentID: environmentID, Baseline: baseline, EnvironmentOverride: override})
			if errors.Is(saveErr, source.ErrRevisionMismatch) {
				return draft.SourceSnapshot{}, draft.ErrSourceRevisionChanged
			}
			if saveErr != nil {
				return draft.SourceSnapshot{}, saveErr
			}
			return draftSourceSnapshot(next), nil
		},
	})
}

func draftSourceSnapshot(snapshot source.Snapshot) draft.SourceSnapshot {
	return draft.SourceSnapshot{Revision: snapshot.Revision, Baseline: snapshot.Baseline, EnvironmentOverrides: snapshot.EnvironmentOverrides}
}

// ValidateDraft runs the complete Spec 007 validation against one captured
// DraftView and stores the resulting project-level draft revision.
func (s *Service) ValidateDraft(_ context.Context, environmentID string) (validation.Result, uint64, error) {
	view, revision, environment, err := s.validationContext(environmentID)
	if err != nil {
		return validation.Result{}, 0, err
	}
	result := validation.Result{
		EnvironmentID:          environmentID,
		ValidatedDraftRevision: revision,
		ValidatedAt:            time.Now().UTC(),
		Status:                 validation.StatusFresh,
		Diagnostics: validation.Validate(validation.Input{
			PackRef:         view.PackRef,
			EnvironmentID:   environmentID,
			EnvironmentKind: environment.Kind,
			Effective:       view.Effective,
		}),
	}
	result.Readiness = validation.ReadinessFor(result.Diagnostics)
	if err := s.validations.Save(result); err != nil {
		return validation.Result{}, 0, err
	}
	// A mutation may have completed while validation was executing. Preserve
	// the captured revision and expose the result's actual freshness.
	_, currentRevision, err := s.GetDraft(context.Background(), environmentID)
	if err != nil {
		return validation.Result{}, 0, err
	}
	result, err = s.validations.Get(environmentID, currentRevision)
	return result, currentRevision, err
}

// Diagnostics returns the latest stored complete-validation result.
func (s *Service) Diagnostics(ctx context.Context, environmentID string) (validation.Result, uint64, error) {
	_, revision, err := s.GetDraft(ctx, environmentID)
	if err != nil {
		return validation.Result{}, 0, err
	}
	result, err := s.validations.Get(environmentID, revision)
	return result, revision, err
}

func (s *Service) validationContext(environmentID string) (draft.View, uint64, project.Environment, error) {
	view, revision, err := s.GetDraft(context.Background(), environmentID)
	if err != nil {
		return draft.View{}, 0, project.Environment{}, err
	}
	_, environment, err := s.GetEnvironment(context.Background(), environmentID)
	if err != nil {
		return draft.View{}, 0, project.Environment{}, err
	}
	return view, revision, environment, nil
}

// StartPlan always creates a durable Operation before starting work. The
// operation store is deliberately the recovery authority for callers that
// disconnect while this local workflow runs.
func (s *Service) StartPlan(ctx context.Context, environmentID string) (operation.Operation, error) {
	if _, _, err := s.GetEnvironment(ctx, environmentID); err != nil {
		return operation.Operation{}, err
	}
	op, err := s.operations.Create("plan")
	if err != nil {
		return operation.Operation{}, err
	}
	go s.buildPlan(op.OperationID, environmentID)
	return op, nil
}

func (s *Service) Operation(_ context.Context, id string) (operation.Operation, error) {
	return s.operations.Get(id)
}

type ProviderInfo struct {
	EnvironmentID string                `json:"environment_id"`
	ProviderType  string                `json:"provider_type"`
	Status        string                `json:"status"`
	Capabilities  provider.Capabilities `json:"capabilities"`
}

func (s *Service) ProviderStatus(ctx context.Context, environmentID string) (ProviderInfo, error) {
	_, environment, err := s.GetEnvironment(ctx, environmentID)
	if err != nil {
		return ProviderInfo{}, err
	}
	adapter, err := s.providerFor(environment)
	if err != nil || adapter == nil {
		return ProviderInfo{EnvironmentID: environmentID, ProviderType: environment.Provider.Type, Status: "not_configured"}, nil
	}
	status := adapter.Status(ctx)
	return ProviderInfo{EnvironmentID: environmentID, ProviderType: environment.Provider.Type, Status: status.Status, Capabilities: status.Capabilities}, nil
}

func (s *Service) StartProviderConnect(ctx context.Context, environmentID string) (operation.Operation, error) {
	if _, _, err := s.GetEnvironment(ctx, environmentID); err != nil {
		return operation.Operation{}, err
	}
	op, err := s.operations.Create("provider_connect")
	if err != nil {
		return operation.Operation{}, err
	}
	go func() {
		_, _ = s.operations.Update(op.OperationID, "running", "reading_remote", nil, nil, "unchanged")
		_, environment, getErr := s.GetEnvironment(context.Background(), environmentID)
		if getErr == nil {
			var adapter provider.Adapter
			adapter, getErr = s.providerFor(environment)
			if getErr == nil {
				getErr = adapter.Connect(context.Background())
			}
		}
		if getErr != nil {
			s.failProviderOperation(op.OperationID, "reading_remote", getErr)
		} else {
			_, _ = s.operations.Update(op.OperationID, "succeeded", "completed", nil, nil, "unchanged")
		}
	}()
	return op, nil
}

func (s *Service) StartPull(ctx context.Context, environmentID string) (operation.Operation, error) {
	if _, _, err := s.GetEnvironment(ctx, environmentID); err != nil {
		return operation.Operation{}, err
	}
	op, err := s.operations.Create("remote_pull")
	if err != nil {
		return operation.Operation{}, err
	}
	go s.pullRemote(op.OperationID, environmentID)
	return op, nil
}

func (s *Service) pullRemote(operationID, environmentID string) {
	_, _ = s.operations.Update(operationID, "running", "reading_remote", nil, nil, "unchanged")
	_, environment, err := s.GetEnvironment(context.Background(), environmentID)
	if err == nil {
		var adapter provider.Adapter
		adapter, err = s.providerFor(environment)
		if err == nil {
			var pulled provider.Template
			pulled, err = adapter.Pull(context.Background())
			if err == nil {
				var snapshot remote.Snapshot
				snapshot, err = remote.SnapshotFromTemplate(pulled.Raw, pulled.ETag, pulled.Version, pulled.ObservedAt)
				if err == nil {
					_, _ = s.operations.Update(operationID, "running", "snapshotting", nil, nil, "unchanged")
					err = s.remote.(*remote.FileStore).Save(environmentID, snapshot)
					if err == nil {
						_, _ = s.operations.Update(operationID, "succeeded", "completed", nil, &operation.Result{ResourceType: "remote_snapshot", ResourceID: environmentID, Href: "/api/v1/environments/" + environmentID + "/remote/projection"}, "unchanged")
						return
					}
				}
			}
		}
	}
	s.failProviderOperation(operationID, "reading_remote", err)
}

func (s *Service) StartRemoteValidate(ctx context.Context, environmentID, planID string) (operation.Operation, error) {
	_, environment, err := s.GetEnvironment(ctx, environmentID)
	if err != nil {
		return operation.Operation{}, err
	}
	p, err := s.plans.Get(planID)
	if err != nil {
		return operation.Operation{}, err
	}
	if p.EnvironmentID != environment.ID || p.Status != "ready" {
		return operation.Operation{}, &PlanInvalidatedError{PlanID: planID, Reason: "validation_not_ready"}
	}
	op, err := s.operations.Create("remote_validate")
	if err != nil {
		return operation.Operation{}, err
	}
	go func() {
		_, _ = s.operations.Update(op.OperationID, "running", "validating_remote", nil, nil, "unchanged")
		adapter, callErr := s.providerFor(environment)
		if callErr == nil {
			var input []byte
			input, _, callErr = s.plans.Artifact(planID, "provider-input.json")
			if callErr == nil {
				callErr = adapter.Validate(context.Background(), input)
			}
		}
		if callErr != nil {
			s.failProviderOperation(op.OperationID, "validating_remote", callErr)
			return
		}
		_, _ = s.operations.Update(op.OperationID, "succeeded", "completed", nil, &operation.Result{ResourceType: "plan", ResourceID: planID, Href: "/api/v1/plans/" + planID}, "unchanged")
	}()
	return op, nil
}

func (s *Service) failProviderOperation(operationID, stage string, err error) {
	safe := provider.SafeError(err)
	code, retryable := "provider_unavailable", true
	if errors.Is(safe, provider.ErrUnauthorized) {
		code, retryable = "provider_unauthorized", false
	} else if errors.Is(safe, provider.ErrNotConfigured) {
		code, retryable = "provider_not_configured", false
	} else if errors.Is(safe, provider.ErrValidation) {
		code, retryable = "provider_validation_failed", false
	} else if errors.Is(safe, context.Canceled) {
		code, retryable = "operation_cancelled", false
	}
	// Never retain upstream error text: it can contain Authorization material.
	_, _ = s.operations.Update(operationID, "failed", stage, &operation.Failure{Code: code, Message: "provider request failed", Retryable: retryable, Stage: stage}, nil, "unchanged")
}

func (s *Service) buildPlan(operationID, environmentID string) {
	fail := func(stage string, err error) {
		_, _ = s.operations.Update(operationID, "failed", stage, &operation.Failure{Code: "plan_build_failed", Message: err.Error(), Retryable: false, Stage: stage}, nil, "unchanged")
	}
	_, _ = s.operations.Update(operationID, "running", "reading_remote", nil, nil, "unchanged")
	remoteSnapshot := s.planRemoteSnapshot(environmentID)
	_, _ = s.operations.Update(operationID, "running", "compiling", nil, nil, "unchanged")
	view, revision, environment, err := s.validationContext(environmentID)
	if err != nil {
		fail("compiling", err)
		return
	}
	inspectRemote(&remoteSnapshot, view.Effective)
	validationResult, _, err := s.ValidateDraft(context.Background(), environmentID)
	if err != nil {
		fail("compiling", err)
		return
	}
	manifest, err := s.projects.Snapshot()
	if err != nil {
		fail("compiling", err)
		return
	}
	schema, environments, err := s.draftSchema(manifest.Manifest)
	if err != nil {
		fail("compiling", err)
		return
	}
	sourceSnapshot, err := s.source.Load()
	if err != nil {
		fail("compiling", err)
		return
	}
	clean, err := draft.BuildView(schema, environments, draftSourceSnapshot(sourceSnapshot), draft.State{Revision: 1, EnvironmentOverrides: map[string]map[string]any{}}, environmentID)
	if err != nil {
		fail("compiling", err)
		return
	}
	mode := manifest.Manifest.Project.ReleaseConfirmationPolicy.ProductionLowRiskMode
	if mode == "" {
		mode = "environment_id"
	}
	_, _ = s.operations.Update(operationID, "running", "analyzing", nil, nil, "unchanged")
	built, err := plan.Build(plan.Input{EnvironmentID: environmentID, EnvironmentKind: environment.Kind, PackRef: view.PackRef, SourceDigest: view.SourceRevision, DraftRevision: revision, Desired: view.Effective, Baseline: clean.Effective, BaseLayer: view.Baseline.Resolved.Value, RemoteSnapshot: remoteSnapshot, ValidationReady: validationResult.Readiness == validation.ReadinessReady && validationResult.Status == validation.StatusFresh, ProductionLowRiskMode: mode})
	if err != nil {
		fail("analyzing", err)
		return
	}
	if err := s.plans.Save(built); err != nil {
		fail("analyzing", err)
		return
	}
	_, _ = s.operations.Update(operationID, "succeeded", "completed", nil, &operation.Result{ResourceType: "plan", ResourceID: built.Plan.PlanID, Href: "/api/v1/plans/" + built.Plan.PlanID}, "unchanged")
}

// planRemoteSnapshot owns the Plan read boundary. A configured provider is
// read afresh; on any read failure it returns an unavailable fact rather than
// allowing an older ETag to masquerade as current. Workspaces without a
// credential reference may still use their protected local snapshot for the
// offline/fixture workflow established by Spec 008.
func (s *Service) planRemoteSnapshot(environmentID string) remote.Snapshot {
	_, environment, err := s.GetEnvironment(context.Background(), environmentID)
	if err != nil {
		return remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnavailable}
	}
	adapter, err := s.providerFor(environment)
	if err != nil {
		return remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnavailable}
	}
	if adapter.Status(context.Background()).Status == "not_configured" {
		snapshot, cacheErr := s.remote.Current(environmentID)
		if cacheErr != nil {
			return remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnavailable}
		}
		return snapshot
	}
	pulled, err := adapter.Pull(context.Background())
	if err != nil {
		if errors.Is(provider.SafeError(err), provider.ErrUnauthorized) {
			return remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnauthorized}
		}
		return remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnavailable}
	}
	snapshot, err := remote.SnapshotFromTemplate(pulled.Raw, pulled.ETag, pulled.Version, pulled.ObservedAt)
	if err != nil {
		return remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnavailable}
	}
	if err := s.remote.(*remote.FileStore).Save(environmentID, snapshot); err != nil {
		return remote.Snapshot{Status: "unavailable", UnavailableReason: remote.ProviderUnavailable}
	}
	return snapshot
}

func (s *Service) GetPlan(ctx context.Context, id string) (plan.Plan, error) {
	p, err := s.plans.Get(id)
	if err != nil {
		return plan.Plan{}, err
	}
	if p.Status == "invalidated" || p.Status == "expired" {
		return p, nil
	}
	if !time.Now().UTC().Before(p.ExpiresAt) {
		p.Status = "expired"
		p.InvalidationReason = "ttl_expired"
		_ = s.plans.Update(p)
		return p, nil
	}
	_, revision, err := s.GetDraft(ctx, p.EnvironmentID)
	if err != nil {
		return plan.Plan{}, err
	}
	if revision != p.DraftRevision {
		p.Status = "invalidated"
		p.InvalidationReason = "draft_revision_changed"
		_ = s.plans.Update(p)
		return p, nil
	}
	info, err := s.SourceInfo(ctx)
	if err != nil {
		return plan.Plan{}, err
	}
	if info.Status.Digest != p.SourceDigest {
		p.Status = "invalidated"
		p.InvalidationReason = "source_digest_changed"
		_ = s.plans.Update(p)
		return p, nil
	}
	current, err := s.remote.Current(p.EnvironmentID)
	if err != nil {
		return plan.Plan{}, err
	}
	if p.RemoteETag != nil {
		if current.Status != "available" {
			p.Status = "invalidated"
			p.InvalidationReason = "remote_snapshot_unavailable"
		} else if current.RemoteETag != *p.RemoteETag {
			p.Status = "invalidated"
			p.InvalidationReason = "remote_etag_changed"
		}
		if p.Status == "invalidated" {
			_ = s.plans.Update(p)
		}
	}
	return p, nil
}

func (s *Service) PlanArtifact(ctx context.Context, planID, name string) ([]byte, plan.ArtifactMetadata, plan.Plan, error) {
	p, err := s.GetPlan(ctx, planID)
	if err != nil {
		return nil, plan.ArtifactMetadata{}, plan.Plan{}, err
	}
	b, m, err := s.plans.Artifact(planID, name)
	return b, m, p, err
}

// CheckPlanForPublish is the reusable preflight boundary for Spec 009. It is
// deliberately not an HTTP endpoint: this Spec creates Plans but does not add
// publishing. Its typed errors preserve the frozen 409/412 error separation.
func (s *Service) CheckPlanForPublish(ctx context.Context, id string) (plan.Plan, error) {
	p, err := s.plans.Get(id)
	if err != nil {
		return plan.Plan{}, err
	}
	if !time.Now().UTC().Before(p.ExpiresAt) {
		return plan.Plan{}, &PlanInvalidatedError{PlanID: id, Reason: "ttl_expired"}
	}
	_, revision, err := s.GetDraft(ctx, p.EnvironmentID)
	if err != nil {
		return plan.Plan{}, err
	}
	if revision != p.DraftRevision {
		return plan.Plan{}, &PlanInvalidatedError{PlanID: id, Reason: "draft_revision_changed"}
	}
	info, err := s.SourceInfo(ctx)
	if err != nil {
		return plan.Plan{}, err
	}
	if info.Status.Digest != p.SourceDigest {
		return plan.Plan{}, &PlanInvalidatedError{PlanID: id, Reason: "source_digest_changed"}
	}
	if p.Status != "ready" || p.RemoteETag == nil {
		return plan.Plan{}, &PlanInvalidatedError{PlanID: id, Reason: "remote_snapshot_unavailable"}
	}
	if len(p.BlockingReasons) > 0 {
		return plan.Plan{}, &PlanInvalidatedError{PlanID: id, Reason: p.BlockingReasons[0].ReasonCode}
	}
	current, err := s.remote.Current(p.EnvironmentID)
	if err != nil {
		return plan.Plan{}, err
	}
	if current.Status != "available" {
		return plan.Plan{}, &PlanInvalidatedError{PlanID: id, Reason: "remote_snapshot_unavailable"}
	}
	if current.RemoteETag != *p.RemoteETag {
		return plan.Plan{}, &RemoteETagMismatchError{PlanID: id, Expected: *p.RemoteETag, Current: current}
	}
	return p, nil
}

func (s *Service) draftSchema(manifest project.Manifest) (draft.Schema, []draft.Environment, error) {
	definition, _, err := s.packRegistry.Resolve(manifest.Pack.ID)
	if err != nil {
		return draft.Schema{}, nil, err
	}
	allowed := make(map[string]map[string]bool, len(definition.Metadata.EntityTypes))
	for _, entity := range definition.Metadata.EntityTypes {
		fields := make(map[string]bool, len(entity.EnvironmentOverrideFields))
		for _, field := range entity.EnvironmentOverrideFields {
			fields[field] = true
		}
		allowed[entity.Name] = fields
	}
	schema := draft.Schema{PackRef: manifest.Pack.ID, Defaults: map[string]any{}, Fields: []draft.Field{}}
	for _, entity := range definition.Schema.Entities {
		metadata := entityMetadata(definition.Metadata.EntityTypes, entity.Name)
		if metadata.Collection != "" {
			schema.Defaults[metadata.Collection] = []any{}
			schema.Fields = append(schema.Fields, draft.Field{Path: "/" + pointerToken(metadata.Collection), Type: "array", EnvironmentOverrideAllowed: len(metadata.EnvironmentOverrideFields) > 0, Default: []any{}})
			continue
		}
		entityDefaults := map[string]any{}
		for _, field := range entity.Fields {
			var defaultValue any
			if err := json.Unmarshal(field.Default, &defaultValue); err != nil {
				return draft.Schema{}, nil, err
			}
			entityDefaults[field.Name] = defaultValue
			schema.Fields = append(schema.Fields, draft.Field{Path: "/" + pointerToken(entity.Name) + "/" + pointerToken(field.Name), Type: string(field.Type), Nullable: field.Nullable, EnvironmentOverrideAllowed: allowed[entity.Name][field.Name], Required: field.Required, Default: defaultValue, Enum: rawValues(field.Validation.Enum), MinLength: field.Validation.MinLength, MaxLength: field.Validation.MaxLength, Minimum: field.Validation.Minimum, Maximum: field.Validation.Maximum})
		}
		if len(entityDefaults) > 0 {
			schema.Defaults[entity.Name] = entityDefaults
		}
	}
	environments := make([]draft.Environment, len(manifest.Environments))
	for index, environment := range manifest.Environments {
		environments[index] = draft.Environment{ID: environment.ID, Name: environment.Name, Kind: environment.Kind}
	}
	return schema, environments, nil
}

func entityMetadata(entities []packs.EntityMetadata, name string) packs.EntityMetadata {
	for _, entity := range entities {
		if entity.Name == name {
			return entity
		}
	}
	return packs.EntityMetadata{}
}

func rawValues(values []json.RawMessage) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		var decoded any
		if json.Unmarshal(value, &decoded) == nil {
			result = append(result, decoded)
		}
	}
	return result
}

func pointerToken(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}
