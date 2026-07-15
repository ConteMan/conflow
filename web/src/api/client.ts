import type { components } from "./schema";

export type BootstrapResponse = components["schemas"]["BootstrapResponse"];
export type BootstrapData = components["schemas"]["BootstrapData"];
export type Project = components["schemas"]["Project"];
export type Environment = components["schemas"]["Environment"];
export type EnvironmentKind = components["schemas"]["EnvironmentKind"];
export type UpdateProjectInput = components["schemas"]["UpdateProjectInput"];
export type CreateEnvironmentInput = components["schemas"]["CreateEnvironmentInput"];
export type UpdateEnvironmentInput = components["schemas"]["UpdateEnvironmentInput"];
export type ManifestState = components["schemas"]["ManifestState"];
export type PackMetadata = components["schemas"]["PackMetadata"];
export type PackSchema = components["schemas"]["PackSchema"];
export type DraftView = components["schemas"]["DraftView"];
export type EntityView = components["schemas"]["EntityView"];
export type EntityRecord = components["schemas"]["EntityRecord"];
export type EntitiesResponse = components["schemas"]["EntitiesResponse"];
export type EntityResponse = components["schemas"]["EntityResponse"];
export type CreateEntityInput = components["schemas"]["CreateEntityInput"];
export type EntityMutationInput = components["schemas"]["EntityMutationInput"];
export type DraftStructuralErrorDetail = components["schemas"]["DraftStructuralErrorDetail"];
export type FieldSchema = components["schemas"]["FieldSchema"];
export type EntityReference = components["schemas"]["EntityReference"];
export type EntityReferencesResponse = components["schemas"]["EntityReferencesResponse"];
export type Diagnostic = components["schemas"]["Diagnostic"];
export type ValidationResult = components["schemas"]["ValidationResult"];
export type ValidationResponse = components["schemas"]["ValidationResponse"];
export type Operation = components["schemas"]["Operation"];
export type OperationResponse = components["schemas"]["OperationResponse"];
export type Plan = components["schemas"]["Plan"];
export type PlanResponse = components["schemas"]["PlanResponse"];
export type AffectedEntity = components["schemas"]["AffectedEntity"];
export type RemoteParameterChange = components["schemas"]["RemoteParameterChange"];
export type SemanticChange = components["schemas"]["SemanticChange"];
export type Release = components["schemas"]["Release"];
export type ReleaseSummary = components["schemas"]["ReleaseSummary"];
export type RollbackPreview = components["schemas"]["RollbackPreview"];
export type ReleaseConfirmation = components["schemas"]["ReleaseConfirmation"];
export type RemoteAuditState = components["schemas"]["RemoteAuditState"];
export type ProviderStatus = components["schemas"]["ProviderStatus"];
export type RemoteProjection = components["schemas"]["RemoteProjection"];

type APIErrorResponse = components["schemas"]["ErrorResponse"] | components["schemas"]["DraftValidationErrorResponse"] | components["schemas"]["EntityReferencedErrorResponse"] | components["schemas"]["RemoteETagMismatchResponse"];
type ConflictResponse = components["schemas"]["ManifestRevisionMismatchResponse"] | components["schemas"]["DraftRevisionMismatchResponse"];
type ProjectResponse = components["schemas"]["ProjectResponse"];
type EnvironmentResponse = components["schemas"]["EnvironmentResponse"];
type DeleteEnvironmentResponse = components["schemas"]["DeleteEnvironmentResponse"];
type PackMetadataResponse = components["schemas"]["PackMetadataResponse"];
type ProviderStatusResponse = components["schemas"]["ProviderStatusResponse"];

export class ConflowAPIError extends Error {
  readonly code: string;
  readonly requestId: string;
  readonly status: number;
  readonly currentRevision?: number;
  readonly currentState?: ManifestState | DraftView;
  readonly details?: DraftStructuralErrorDetail[];
  readonly references?: EntityReference[];
  readonly currentRemote?: RemoteAuditState;
  readonly rebuildRequired?: boolean;

  constructor(status: number, error: APIErrorResponse["error"] | ConflictResponse["error"]) {
    super(error.message);
    this.name = "ConflowAPIError";
    this.code = error.code;
    this.requestId = error.request_id;
    this.status = status;
    this.currentRevision = "current_revision" in error ? error.current_revision : undefined;
    if ((error.code === "revision_mismatch" || error.code === "source_revision_mismatch") && "current_state" in error) {
      this.currentState = error.current_state;
    }
    if (error.code === "validation_failed" && "details" in error) this.details = error.details as DraftStructuralErrorDetail[];
    if (error.code === "entity_referenced" && "references" in error) this.references = error.references as EntityReference[];
    if (error.code === "remote_etag_mismatch" && "current_remote" in error) {
      this.currentRemote = error.current_remote as RemoteAuditState;
      this.rebuildRequired = "rebuild" in error && error.rebuild.required;
    }
  }
}

export class ConflowNetworkError extends Error {
  constructor() {
    super("Conflow local API is unavailable");
    this.name = "ConflowNetworkError";
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`/api/v1${path}`, {
      ...init,
      headers: { Accept: "application/json", ...init.headers },
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") throw error;
    throw new ConflowNetworkError();
  }

  const payload = (await response.json()) as T | APIErrorResponse | ConflictResponse;
  if (!response.ok) {
    throw new ConflowAPIError(response.status, (payload as APIErrorResponse | ConflictResponse).error);
  }
  return payload as T;
}

function mutationInit(method: "POST" | "PUT" | "DELETE", revision: number, body?: unknown): RequestInit {
  return {
    method,
    headers: {
      "Content-Type": "application/json",
      "If-Match": `"${revision}"`,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  };
}

export function getBootstrap(signal?: AbortSignal): Promise<BootstrapResponse> {
  return request("/bootstrap", { signal });
}

export function updateProject(revision: number, input: UpdateProjectInput): Promise<ProjectResponse> {
  return request("/project", mutationInit("PUT", revision, input));
}

export function createEnvironment(revision: number, input: CreateEnvironmentInput): Promise<EnvironmentResponse> {
  return request("/environments", mutationInit("POST", revision, input));
}

export function updateEnvironment(id: string, revision: number, input: UpdateEnvironmentInput): Promise<EnvironmentResponse> {
  return request(`/environments/${encodeURIComponent(id)}`, mutationInit("PUT", revision, input));
}

export function deleteEnvironment(id: string, revision: number): Promise<DeleteEnvironmentResponse> {
  return request(`/environments/${encodeURIComponent(id)}`, mutationInit("DELETE", revision));
}

export function getProviderStatus(environmentID: string, signal?: AbortSignal): Promise<ProviderStatusResponse> {
  return request(`/environments/${encodeURIComponent(environmentID)}/provider`, { signal });
}

export function getRemoteProjection(environmentID: string, signal?: AbortSignal): Promise<{ data: RemoteProjection; meta: { revision: number } }> {
  return request(`/environments/${encodeURIComponent(environmentID)}/remote/projection`, { signal });
}

export function connectProvider(environmentID: string, credentialsPath: string): Promise<OperationResponse> {
  return request(`/environments/${encodeURIComponent(environmentID)}/provider:connect`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ credentials_path: credentialsPath }),
  });
}

export function pullRemote(environmentID: string): Promise<OperationResponse> {
  return request(`/environments/${encodeURIComponent(environmentID)}/remote:pull`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  });
}

export function getPackMetadata(packRef: string, signal?: AbortSignal): Promise<PackMetadataResponse> {
  const separator = packRef.lastIndexOf("/");
  const name = packRef.slice(0, separator);
  const version = packRef.slice(separator + 1);
  return request(`/packs/${encodeURIComponent(name)}/versions/${encodeURIComponent(version)}`, { signal });
}

function packPath(packRef: string) {
  const separator = packRef.lastIndexOf("/");
  return { name: packRef.slice(0, separator), version: packRef.slice(separator + 1) };
}

export function getPackSchema(packRef: string, signal?: AbortSignal): Promise<{ data: PackSchema; meta: { revision: number } }> {
  const { name, version } = packPath(packRef);
  return request(`/packs/${encodeURIComponent(name)}/versions/${encodeURIComponent(version)}/schema`, { signal });
}

export function getDraft(environmentID: string, signal?: AbortSignal): Promise<{ data: DraftView; meta: { revision: number } }> {
  return request(`/drafts/${encodeURIComponent(environmentID)}`, { signal });
}

export function listDraftEntities(environmentID: string, entityType?: string, signal?: AbortSignal): Promise<EntitiesResponse> {
  const query = entityType ? `?entity_type=${encodeURIComponent(entityType)}` : "";
  return request(`/drafts/${encodeURIComponent(environmentID)}/entities${query}`, { signal });
}

export function getDraftEntity(environmentID: string, entityType: string, entityID: string, signal?: AbortSignal): Promise<EntityResponse> {
  return request(`/drafts/${encodeURIComponent(environmentID)}/entities/${encodeURIComponent(entityType)}/${encodeURIComponent(entityID)}`, { signal });
}

export function createDraftEntity(environmentID: string, revision: number, input: CreateEntityInput): Promise<EntityResponse> {
  return request(`/drafts/${encodeURIComponent(environmentID)}/entities`, mutationInit("POST", revision, input));
}

export function replaceDraftEntity(environmentID: string, entityType: string, entityID: string, revision: number, input: EntityMutationInput): Promise<EntityResponse> {
  return request(`/drafts/${encodeURIComponent(environmentID)}/entities/${encodeURIComponent(entityType)}/${encodeURIComponent(entityID)}`, mutationInit("PUT", revision, input));
}

export function deleteDraftEntity(environmentID: string, entityType: string, entityID: string, revision: number, input: components["schemas"]["EntityDeleteInput"]): Promise<EntityResponse> {
  return request(`/drafts/${encodeURIComponent(environmentID)}/entities/${encodeURIComponent(entityType)}/${encodeURIComponent(entityID)}`, mutationInit("DELETE", revision, input));
}

export function getDraftEntityReferences(environmentID: string, entityType: string, entityID: string, signal?: AbortSignal): Promise<EntityReferencesResponse> {
  return request(`/drafts/${encodeURIComponent(environmentID)}/entities/${encodeURIComponent(entityType)}/${encodeURIComponent(entityID)}/referenced-by`, { signal });
}

export function validateDraft(environmentID: string): Promise<ValidationResponse> {
  return request(`/drafts/${encodeURIComponent(environmentID)}:validate`, { method: "POST", headers: { "Content-Type": "application/json" } });
}

export function getDraftDiagnostics(environmentID: string, signal?: AbortSignal): Promise<ValidationResponse> {
  return request(`/drafts/${encodeURIComponent(environmentID)}/diagnostics`, { signal });
}

export function createPlan(environmentID: string): Promise<OperationResponse> {
  return request(`/drafts/${encodeURIComponent(environmentID)}:plan`, { method: "POST", headers: { "Content-Type": "application/json" } });
}

export function getOperation(operationID: string, signal?: AbortSignal): Promise<OperationResponse> {
  return request(`/operations/${encodeURIComponent(operationID)}`, { signal });
}

export function getPlan(planID: string, signal?: AbortSignal): Promise<PlanResponse> {
  return request(`/plans/${encodeURIComponent(planID)}`, { signal });
}

export function planArtifactURL(planID: string, artifactName: string) {
  return `/api/v1/plans/${encodeURIComponent(planID)}/artifacts/${encodeURIComponent(artifactName)}`;
}

export function createRelease(environmentID: string, input: components["schemas"]["CreateReleaseInput"], idempotencyKey: string): Promise<OperationResponse> {
  return request(`/environments/${encodeURIComponent(environmentID)}/releases`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey },
    body: JSON.stringify(input),
  });
}

export function listReleases(environmentID: string, signal?: AbortSignal): Promise<components["schemas"]["ReleasesResponse"]> {
  return request(`/environments/${encodeURIComponent(environmentID)}/releases`, { signal });
}

export function getRelease(environmentID: string, releaseID: string, signal?: AbortSignal): Promise<components["schemas"]["ReleaseResponse"]> {
  return request(`/environments/${encodeURIComponent(environmentID)}/releases/${encodeURIComponent(releaseID)}`, { signal });
}

export function createRollbackPreview(environmentID: string, releaseID: string): Promise<OperationResponse> {
  return request(`/environments/${encodeURIComponent(environmentID)}/releases/${encodeURIComponent(releaseID)}:rollback-preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
  });
}

export function getRollbackPreview(environmentID: string, releaseID: string, signal?: AbortSignal): Promise<components["schemas"]["RollbackPreviewResponse"]> {
  return request(`/environments/${encodeURIComponent(environmentID)}/releases/${encodeURIComponent(releaseID)}/rollback-preview`, { signal });
}

export function rollbackRelease(environmentID: string, releaseID: string, input: components["schemas"]["RollbackInput"], idempotencyKey: string): Promise<OperationResponse> {
  return request(`/environments/${encodeURIComponent(environmentID)}/releases/${encodeURIComponent(releaseID)}:rollback`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Idempotency-Key": idempotencyKey },
    body: JSON.stringify(input),
  });
}

export function defaultsURL(environmentID: string, format: "xml" | "json" | "plist") {
  return `/api/v1/environments/${encodeURIComponent(environmentID)}/defaults?format=${format}`;
}

// ── Import types (Spec 021) ───────────────────────────────────────────────────

export interface ImportBundle {
  format_version: number;
  pack_ref: string;
  schema_version: number;
  entities: Record<string, Array<{ id: string; fields: Record<string, unknown> }>>;
  decisions_required?: DecisionRequired[];
}

export interface DecisionRequired {
  key: string;
  reason: string;
  hint?: string;
}

export interface ImportDecision {
  key: string;
  value: unknown;
}

export interface EntityAction {
  entity_type: string;
  id: string;
}

export interface PreviewResult {
  preview_token: string;
  expires_at: string;
  pack_ref: string;
  conflict_mode: string;
  entity_plan: {
    to_add: EntityAction[];
    to_replace: EntityAction[];
    to_skip: EntityAction[];
    to_keep: EntityAction[];
  };
  decisions_required: DecisionRequired[];
  risks?: string[];
}

export interface ApplyResult {
  applied_count: number;
  skipped_count: number;
  revision: number;
}

export function previewImport(
  environmentId: string,
  bundle: ImportBundle,
  conflictMode: string,
): Promise<{ data: PreviewResult; meta: { revision: number } }> {
  return request(`/drafts/${encodeURIComponent(environmentId)}:import-preview`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ bundle, conflict_mode: conflictMode }),
  });
}

export function applyImport(
  environmentId: string,
  bundle: ImportBundle,
  previewToken: string,
  decisions: ImportDecision[],
  conflictMode: string,
): Promise<ApplyResult> {
  return request(`/drafts/${encodeURIComponent(environmentId)}:import-apply`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "If-Match": previewToken },
    body: JSON.stringify({ bundle, decisions, conflict_mode: conflictMode }),
  });
}
