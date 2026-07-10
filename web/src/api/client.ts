import type { components } from "./schema";

export type BootstrapResponse = components["schemas"]["BootstrapResponse"];
export type APIErrorResponse = components["schemas"]["ErrorResponse"];

export class ConflowAPIError extends Error {
  readonly code: string;
  readonly requestId: string;

  constructor(error: components["schemas"]["Error"]) {
    super(error.message);
    this.name = "ConflowAPIError";
    this.code = error.code;
    this.requestId = error.request_id;
  }
}

export async function getBootstrap(signal?: AbortSignal): Promise<BootstrapResponse> {
  const response = await fetch("/api/v1/bootstrap", {
    headers: { Accept: "application/json" },
    signal,
  });
  const payload = (await response.json()) as BootstrapResponse | APIErrorResponse;
  if (!response.ok) {
    throw new ConflowAPIError((payload as APIErrorResponse).error);
  }
  return payload as BootstrapResponse;
}
