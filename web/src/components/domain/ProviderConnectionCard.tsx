import { CheckCircle2, CloudDownload, LoaderCircle, PlugZap, RefreshCw } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { ConflowAPIError, connectProvider, getOperation, getProviderStatus, pullRemote, type Environment, type Operation, type ProviderStatus } from "../../api/client";
import { Button } from "../ui/Button";

export function ProviderConnectionCard({ environment }: { environment: Environment }) {
  const [status, setStatus] = useState<ProviderStatus | null>(null);
  const [credentialsPath, setCredentialsPath] = useState("");
  const [operation, setOperation] = useState<Operation | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadStatus = useCallback(async (signal?: AbortSignal) => {
    try {
      const response = await getProviderStatus(environment.id, signal);
      if (!signal?.aborted) setStatus(response.data);
    } catch (cause) {
      if (!signal?.aborted) setStatus(null);
    }
  }, [environment.id]);

  useEffect(() => {
    const controller = new AbortController();
    void loadStatus(controller.signal);
    return () => controller.abort();
  }, [loadStatus]);

  const waitForOperation = async (operationID: string) => {
    for (let attempt = 0; attempt < 60; attempt += 1) {
      await new Promise((resolve) => window.setTimeout(resolve, 300));
      const result = await getOperation(operationID);
      setOperation(result.data);
      if (["succeeded", "failed", "cancelled"].includes(result.data.status)) return result.data;
    }
    throw new Error("operation_timeout");
  };

  const runConnect = async () => {
    if (!credentialsPath.trim()) return;
    setBusy(true); setError(null); setOperation(null);
    try {
      const response = await connectProvider(environment.id, credentialsPath.trim());
      setCredentialsPath("");
      const result = await waitForOperation(response.data.operation_id);
      if (result.status !== "succeeded") throw new Error(result.failure?.code ?? "provider_failed");
      await loadStatus();
    } catch (cause) {
      setError(connectionError(cause));
    } finally { setBusy(false); }
  };

  const runPull = async () => {
    setBusy(true); setError(null); setOperation(null);
    try {
      const response = await pullRemote(environment.id);
      const result = await waitForOperation(response.data.operation_id);
      if (result.status !== "succeeded") throw new Error(result.failure?.code ?? "provider_failed");
    } catch (cause) {
      setError(connectionError(cause));
    } finally { setBusy(false); }
  };

  const projectReady = environment.provider.project_id.trim().length > 0;
  const connected = status?.status === "connected";
  return <section className="panel provider-card" aria-label="Firebase 连接">
    <header className="panel-heading"><div><h2>Firebase 连接</h2><p>{environment.name} 的本地服务账号验证与线上配置读取。</p></div><StatusBadge status={status?.status} /></header>
    {!projectReady ? <p className="provider-callout">先在环境管理中填写 Firebase 项目 ID。</p> : <>
      <label className="provider-path-field">服务账号 JSON 路径<input value={credentialsPath} disabled={busy} placeholder="/Users/me/secrets/firebase.json" onChange={(event) => setCredentialsPath(event.target.value)} /></label>
      {status?.credentials_path_display ? <p className="provider-path-display">已配置：<code>{status.credentials_path_display}</code></p> : <p className="provider-path-display">尚未配置本地服务账号路径。</p>}
      <div className="provider-actions"><Button variant="primary" disabled={busy || !credentialsPath.trim()} icon={busy ? <LoaderCircle className="spin" size={16} /> : <PlugZap size={16} />} onClick={() => void runConnect()}>{busy ? "正在验证" : "连接并验证"}</Button>{connected ? <Button disabled={busy} icon={<CloudDownload size={16} />} onClick={() => void runPull()}>拉取线上配置</Button> : null}<Button variant="ghost" disabled={busy} icon={<RefreshCw size={16} />} onClick={() => void loadStatus()}>刷新状态</Button></div>
    </>}
    {operation ? <p className="provider-operation">{operation.status === "succeeded" ? "操作已完成" : `正在${operation.stage === "reading_remote" ? "验证凭据" : "处理"}`}</p> : null}
    {error ? <p className="provider-error" role="alert">{error}</p> : null}
  </section>;
}

function StatusBadge({ status }: { status?: ProviderStatus["status"] }) {
  const copy = { connected: "已连接", unavailable: "不可用", unauthorized: "未授权", not_configured: "未配置" }[status ?? "not_configured"];
  return <span className={`provider-status provider-status--${status ?? "not_configured"}`}>{status === "connected" ? <CheckCircle2 size={15} /> : <PlugZap size={15} />}{copy}</span>;
}

function connectionError(cause: unknown) {
  if (cause instanceof ConflowAPIError && cause.code === "provider_project_id_required") return "先在环境管理中填写 Firebase 项目 ID。";
  if (cause instanceof ConflowAPIError && cause.code === "invalid_request") return "请选择本地服务账号 JSON 路径。";
  return "连接未完成。请检查本地服务账号文件和 Firebase 权限后重试。";
}
