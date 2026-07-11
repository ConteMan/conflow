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

type APIErrorResponse = components["schemas"]["ErrorResponse"] | components["schemas"]["DraftValidationErrorResponse"];
type ConflictResponse = components["schemas"]["ManifestRevisionMismatchResponse"] | components["schemas"]["DraftRevisionMismatchResponse"];
type ProjectResponse = components["schemas"]["ProjectResponse"];
type EnvironmentResponse = components["schemas"]["EnvironmentResponse"];
type DeleteEnvironmentResponse = components["schemas"]["DeleteEnvironmentResponse"];
type PackMetadataResponse = components["schemas"]["PackMetadataResponse"];

export class ConflowAPIError extends Error {
  readonly code: string;
  readonly requestId: string;
  readonly status: number;
  readonly currentRevision?: number;
  readonly currentState?: ManifestState | DraftView;
  readonly details?: DraftStructuralErrorDetail[];

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
