import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ConflowAPIError,
  ConflowNetworkError,
  createEnvironment,
  deleteEnvironment,
  getBootstrap,
  getPackMetadata,
  updateEnvironment,
  updateProject,
  type BootstrapData,
  type CreateEnvironmentInput,
  type Environment,
  type ManifestState,
  type PackMetadata,
  type UpdateEnvironmentInput,
  type UpdateProjectInput,
} from "./api/client";
import { AppTopBar, type Page } from "./components/domain/AppTopBar";
import { ConflictDialog, type LocalConflictValue } from "./components/domain/ConflictDialog";
import { EnvironmentManager } from "./components/domain/EnvironmentManager";
import { Overview } from "./components/domain/Overview";
import { ProjectSettings } from "./components/domain/ProjectSettings";
import { EmptyState, LoadingState, RequestError, ServiceUnavailable } from "./components/ui/StateViews";

type Conflict = { state?: ManifestState; revision?: number; local: LocalConflictValue | null };

export default function App() {
  const [data, setData] = useState<BootstrapData | null>(null);
  const [revision, setRevision] = useState(0);
  const [pack, setPack] = useState<PackMetadata | null>(null);
  const [selectedEnvironmentId, setSelectedEnvironmentId] = useState("");
  const [page, setPage] = useState<Page>(pageFromHash());
  const [loading, setLoading] = useState(true);
  const [networkDown, setNetworkDown] = useState(false);
  const [busy, setBusy] = useState(false);
  const [requestError, setRequestError] = useState<{ code: string; requestId?: string } | null>(null);
  const [conflict, setConflict] = useState<Conflict | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setNetworkDown(false);
    setRequestError(null);
    try {
      const response = await getBootstrap(signal);
      setData(response.data);
      setRevision(response.meta.revision);
      setSelectedEnvironmentId((current) => response.data.environments.some((item) => item.id === current) ? current : response.data.environments[0]?.id ?? "");
      try {
        const metadata = await getPackMetadata(response.data.project.pack_ref, signal);
        setPack(metadata.data);
      } catch (error) {
        if (isAbortError(error)) return;
        setPack(null);
      }
    } catch (error) {
      if (isAbortError(error)) return;
      if (error instanceof ConflowNetworkError) setNetworkDown(true);
      else handleError(error, setRequestError, setConflict, null);
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);
  useEffect(() => {
    const onHashChange = () => setPage(pageFromHash());
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  const selectPage = (next: Page) => { window.location.hash = next; setPage(next); };
  const selectedEnvironment = useMemo(() => data?.environments.find((environment) => environment.id === selectedEnvironmentId) ?? data?.environments[0], [data, selectedEnvironmentId]);

  const runMutation = async (operation: () => Promise<{ data: unknown; meta: { revision: number } }>, local: LocalConflictValue | null, apply: (result: unknown) => void) => {
    setBusy(true); setRequestError(null);
    try {
      const response = await operation();
      setRevision(response.meta.revision);
      apply(response.data);
      return true;
    } catch (error) {
      handleError(error, setRequestError, setConflict, local);
      return false;
    } finally { setBusy(false); }
  };

  if (loading && !data) return <LoadingState />;
  if (networkDown || (!data && !loading)) return <ServiceUnavailable requestId={requestError?.requestId} onRetry={() => void load()} />;
  if (!data || data.environments.length === 0 || !selectedEnvironment) return <EmptyState onRetry={() => void load()} />;

  const saveProject = (input: UpdateProjectInput) => runMutation(
    () => updateProject(revision, input),
    { resource: "project", title: data.project.id, name: input.name },
    (result) => setData((current) => current ? { ...current, project: result as BootstrapData["project"] } : current),
  );
  const saveEnvironment = (payload: { mode: "create"; value: CreateEnvironmentInput } | { mode: "edit"; id: string; value: UpdateEnvironmentInput }) => {
    if (payload.mode === "create") return runMutation(
      () => createEnvironment(revision, payload.value),
      { resource: "environment", title: payload.value.id, name: payload.value.name, providerProjectId: payload.value.provider.project_id },
      (result) => { const environment = result as Environment; setData((current) => current ? { ...current, environments: [...current.environments, environment] } : current); setSelectedEnvironmentId(environment.id); },
    );
    return runMutation(
      () => updateEnvironment(payload.id, revision, payload.value),
      { resource: "environment", title: payload.id, name: payload.value.name, providerProjectId: payload.value.provider.project_id },
      (result) => { const environment = result as Environment; setData((current) => current ? { ...current, environments: current.environments.map((item) => item.id === environment.id ? environment : item) } : current); },
    );
  };
  const removeEnvironment = (environment: Environment) => runMutation(
    () => deleteEnvironment(environment.id, revision),
    { resource: "environment", title: environment.id, name: environment.name, providerProjectId: environment.provider.project_id },
    () => { setData((current) => current ? { ...current, environments: current.environments.filter((item) => item.id !== environment.id) } : current); if (selectedEnvironmentId === environment.id) setSelectedEnvironmentId(data.environments.find((item) => item.id !== environment.id)?.id ?? ""); },
  );

  return (
    <div className="app-shell">
      <AppTopBar project={data.project} environments={data.environments} selectedEnvironment={selectedEnvironment} page={page} onEnvironmentChange={setSelectedEnvironmentId} onPageChange={selectPage} />
      {requestError ? <div className="error-container"><RequestError {...requestError} onDismiss={() => setRequestError(null)} /></div> : null}
      {page === "overview" ? <Overview project={data.project} selectedEnvironment={selectedEnvironment} environments={data.environments} pack={pack} onManageEnvironments={(environmentId) => { if (environmentId) setSelectedEnvironmentId(environmentId); selectPage("environments"); }} /> : null}
      {page === "environments" ? <EnvironmentManager environments={data.environments} selectedEnvironmentId={selectedEnvironment.id} busy={busy} readOnly={!data.capabilities.environment_manage} onSelect={setSelectedEnvironmentId} onSubmit={saveEnvironment} onDelete={removeEnvironment} /> : null}
      {page === "project" ? <ProjectSettings project={data.project} busy={busy} readOnly={!data.capabilities.project_edit} onSave={saveProject} /> : null}
      <ConflictDialog open={conflict !== null} state={conflict?.state} revision={conflict?.revision} local={conflict?.local ?? null} onClose={() => setConflict(null)} onReload={() => { if (conflict?.state && conflict.revision) { setData((current) => current ? { ...current, ...conflict.state } : current); setRevision(conflict.revision); } setConflict(null); }} />
    </div>
  );
}

function pageFromHash(): Page {
  const value = window.location.hash.replace(/^#\/?/, "");
  return value === "environments" || value === "project" ? value : "overview";
}

function isAbortError(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

function handleError(error: unknown, setRequestError: (value: { code: string; requestId?: string } | null) => void, setConflict: (value: Conflict | null) => void, local: LocalConflictValue | null) {
  if (error instanceof ConflowAPIError) {
    if (error.code === "revision_mismatch") setConflict({ state: error.currentState, revision: error.currentRevision, local });
    else setRequestError({ code: error.code, requestId: error.requestId });
  } else if (error instanceof ConflowNetworkError) {
    setRequestError({ code: "network_unavailable" });
  } else {
    setRequestError({ code: "internal_error" });
  }
}
