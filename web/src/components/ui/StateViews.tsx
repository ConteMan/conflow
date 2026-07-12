import { AlertTriangle, Clipboard, LoaderCircle, PlugZap, RefreshCw } from "lucide-react";
import { Button } from "./Button";

export const errorCopy: Record<string, { title: string; description: string }> = {
  validation_failed: { title: "内容未通过校验", description: "请检查表单中的必填项与格式后重试。" },
  environment_already_exists: { title: "环境 ID 已存在", description: "请使用另一个稳定 ID。" },
  last_environment: { title: "无法删除最后一个环境", description: "项目必须至少保留一个环境。" },
  environment_not_found: { title: "环境不存在", description: "它可能已被其他操作删除，请重新加载。" },
  precondition_required: { title: "页面状态已过期", description: "重新加载项目后再尝试保存。" },
  network_unavailable: { title: "无法连接本地服务", description: "确认 Conflow 服务仍在运行，然后重试。" },
  internal_error: { title: "操作失败", description: "本地服务遇到问题，请重试。" },
};

export function LoadingState() {
  return <main className="center-state" aria-live="polite"><LoaderCircle className="spin" /><h1>正在载入项目</h1><p>正在读取本地清单和环境。</p></main>;
}

export function EmptyState({ onRetry }: { onRetry: () => void }) {
  return <main className="center-state"><AlertTriangle /><h1>还没有可用项目</h1><p>在此工作目录运行 <code>conflow init</code> 启动创建向导；自动化场景请使用 <code>--non-interactive</code> 与项目、环境 flags。</p><Button variant="primary" onClick={onRetry} icon={<RefreshCw size={16} />}>重新检查</Button></main>;
}

export function ServiceUnavailable({ onRetry, requestId }: { onRetry: () => void; requestId?: string }) {
  return (
    <main className="center-state" data-testid="service-unavailable">
      <span className="state-icon state-icon--danger"><PlugZap /></span>
      <h1>无法连接到 Conflow 本地服务</h1>
      <p>确认 <code>conflow serve</code> 仍在运行，然后重试。</p>
      <Button variant="primary" onClick={onRetry} icon={<RefreshCw size={16} />}>重新连接</Button>
      {requestId ? <div className="request-id"><code>request_id: {requestId}</code><button className="icon-button" aria-label="复制请求 ID" onClick={() => void navigator.clipboard.writeText(requestId)}><Clipboard size={16} /></button></div> : null}
    </main>
  );
}

export function RequestError({ code, requestId, onDismiss }: { code: string; requestId?: string; onDismiss: () => void }) {
  const copy = errorCopy[code] ?? { title: "请求未完成", description: "请重试；若问题持续，使用请求 ID 排查。" };
  return (
    <div className="request-error" role="alert">
      <AlertTriangle size={18} />
      <div><strong>{copy.title}</strong><p>{copy.description}</p>{requestId ? <code>request_id: {requestId}</code> : null}</div>
      {requestId ? <button className="icon-button" aria-label="复制请求 ID" onClick={() => void navigator.clipboard.writeText(requestId)}><Clipboard size={16} /></button> : null}
      <button className="link-button" onClick={onDismiss}>关闭</button>
    </div>
  );
}
