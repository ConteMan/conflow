import { AlertCircle, CheckCircle2, ChevronRight, FileWarning, LoaderCircle, PlayCircle, RefreshCw, ShieldAlert, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ConflowAPIError, ConflowNetworkError, getDraftDiagnostics, validateDraft, type Diagnostic, type Environment, type ValidationResult } from "../../api/client";
import { Button } from "../ui/Button";
import { RequestError } from "../ui/StateViews";

type Filter = "all" | "blocking" | "warning" | "info";

export function ValidationCenter({ environment, draftDirty, onValidation, onOpenEntity, onOpenPlan }: {
  environment: Environment;
  draftDirty: boolean;
  onValidation: (result: ValidationResult | null) => void;
  onOpenEntity: (entityRef: string) => void;
  onOpenPlan: () => void;
}) {
  const [result, setResult] = useState<ValidationResult | null>(null);
  const [loading, setLoading] = useState(true);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<{ code: string; requestId?: string } | null>(null);
  const [filter, setFilter] = useState<Filter>("all");
  const [selected, setSelected] = useState<Diagnostic | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true); setError(null);
    try {
      const response = await getDraftDiagnostics(environment.id, signal);
      setResult(response.data); onValidation(response.data);
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === "AbortError") return;
      if (cause instanceof ConflowAPIError && cause.code === "validation_not_found") { setResult(null); onValidation(null); }
      else setError(toRequestError(cause));
    } finally { if (!signal?.aborted) setLoading(false); }
  }, [environment.id, onValidation]);

  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load]);
  useEffect(() => { setSelected(null); setFilter("all"); }, [environment.id]);

  const run = async () => {
    setRunning(true); setError(null);
    try { const response = await validateDraft(environment.id); setResult(response.data); onValidation(response.data); }
    catch (cause) { setError(toRequestError(cause)); }
    finally { setRunning(false); }
  };

  const diagnostics = useMemo(() => (result?.diagnostics ?? []).filter((item) => filter === "all" || categoryOf(item) === filter), [filter, result]);
  const groups = useMemo(() => groupDiagnostics(diagnostics), [diagnostics]);
  const counts = useMemo(() => countDiagnostics(result?.diagnostics ?? []), [result]);

  return <main className="page-container validation-page">
    <header className="page-heading validation-page-heading">
      <div><h1>校验中心</h1><p>{result ? `上次校验于 ${formatDate(result.validated_at)} · 基于当前保存版本 ${result.validated_draft_revision}` : "检查未发布修改是否满足发布要求"}</p></div>
      <Button variant="primary" onClick={() => void run()} disabled={running} icon={running ? <LoaderCircle className="spin" size={16} /> : <RefreshCw size={16} />}>{running ? "正在校验" : result ? "重新校验" : "运行校验"}</Button>
    </header>
    {error ? <RequestError {...error} onDismiss={() => setError(null)} /> : null}
    {loading ? <ValidationSkeleton /> : null}
    {!loading && !result && !draftDirty ? <section className="validation-empty"><CheckCircle2 size={28} /><div><h2>没有需要校验的修改</h2><p>当前环境没有未发布修改。修改配置后可在这里运行完整校验。</p></div></section> : null}
    {!loading && !result && draftDirty ? <section className="validation-empty"><FileWarning size={28} /><div><h2>尚未运行完整校验</h2><p>当前有未发布修改。运行校验后将显示可发布状态与可修复问题。</p></div></section> : null}
    {result ? <>
      <ValidationSummary result={result} counts={counts} onOpenPlan={onOpenPlan} />
      {result.status === "stale" ? <div className="stale-callout"><TriangleAlert size={18} /><div><strong>结果可能已过期</strong><span>配置在上次校验后已变化，请重新校验后再查看发布计划。</span></div><Button onClick={() => void run()} disabled={running}>重新校验</Button></div> : null}
      <div className="validation-content">
        <section className="validation-list panel">
          <header className="validation-list-heading"><h2>按实体分组</h2><div className="diagnostic-filters" role="group" aria-label="按严重级别筛选">{(["all", "blocking", "warning", "info"] as Filter[]).map((item) => <button key={item} className={filter === item ? "active" : ""} onClick={() => setFilter(item)}>{filterLabel(item)}</button>)}</div></header>
          {groups.length ? groups.map(([key, items]) => <div className="diagnostic-group" key={key}><h3>{groupTitle(key)}</h3>{items.map((diagnostic) => <button className={`diagnostic-row diagnostic-row--${categoryOf(diagnostic)}${selected === diagnostic ? " selected" : ""}`} key={`${diagnostic.code}:${diagnostic.path}`} onClick={() => setSelected(diagnostic)}><SeverityIcon diagnostic={diagnostic} /><span><strong>{diagnostic.message}</strong><code>{diagnostic.code}</code></span><span className="diagnostic-row-category">{categoryLabel(diagnostic)}</span><ChevronRight size={16} /></button>)}</div>) : <p className="muted-copy diagnostic-no-results">没有符合筛选条件的问题。</p>}
        </section>
        <DiagnosticDetail diagnostic={selected} onOpenEntity={onOpenEntity} />
      </div>
    </> : null}
  </main>;
}

function ValidationSummary({ result, counts, onOpenPlan }: { result: ValidationResult; counts: Counts; onOpenPlan: () => void }) {
  const ready = result.readiness === "ready" && result.status === "fresh";
  return <section className={`validation-readiness validation-readiness--${ready ? "ready" : "blocked"}`}>
    <div><strong>{ready ? `可查看 ${result.environment_id} 的发布计划` : result.status === "stale" ? "需要重新校验" : "存在阻断发布的问题"}</strong><span>{result.status === "stale" ? "结果基于旧的保存版本" : `阻断 ${counts.blocking} · 警告 ${counts.warning} · 建议 ${counts.info}`}</span></div>
    {ready ? <Button variant="primary" icon={<PlayCircle size={16} />} onClick={onOpenPlan}>查看发布计划</Button> : null}
  </section>;
}

function DiagnosticDetail({ diagnostic, onOpenEntity }: { diagnostic: Diagnostic | null; onOpenEntity: (entityRef: string) => void }) {
  if (!diagnostic) return <aside className="diagnostic-detail panel diagnostic-detail--empty"><AlertCircle size={24} /><h2>选择一个问题查看详情</h2><p>问题会按相关实体聚合。选择后可查看字段位置和修复建议。</p></aside>;
  return <aside className="diagnostic-detail panel">
    <span className={`severity-badge severity-badge--${categoryOf(diagnostic)}`}>{categoryLabel(diagnostic)}</span>
    <h2>{diagnostic.message}</h2><code>{diagnostic.code}</code>
    <p>{diagnostic.fix_suggestion}</p>
    <div className="diagnostic-location"><span>位置</span><code>{diagnostic.entity_ref ?? diagnostic.path}</code></div>
    {diagnostic.entity_ref ? <Button variant="primary" onClick={() => onOpenEntity(diagnostic.entity_ref!)} icon={<ChevronRight size={16} />}>前往字段修复</Button> : <p className="muted-copy">该问题没有实体定位，请按字段路径检查配置。</p>}
  </aside>;
}

function ValidationSkeleton() { return <div className="validation-skeleton" aria-label="正在加载校验结果"><div /><div /><div /><section /></div>; }
type Counts = { blocking: number; warning: number; info: number };
function countDiagnostics(items: Diagnostic[]): Counts { return items.reduce<Counts>((counts, item) => { counts[categoryOf(item)] += 1; return counts; }, { blocking: 0, warning: 0, info: 0 }); }
function categoryOf(item: Diagnostic): "blocking" | "warning" | "info" { return item.severity === "blocking" || item.severity === "error" ? "blocking" : item.severity === "warning" ? "warning" : "info"; }
function categoryLabel(item: Diagnostic) { return categoryOf(item) === "blocking" ? "阻断" : categoryOf(item) === "warning" ? "警告" : "建议"; }
function filterLabel(value: Filter) { return value === "all" ? "全部" : value === "blocking" ? "阻断" : value === "warning" ? "警告" : "建议"; }
function groupDiagnostics(items: Diagnostic[]) { const grouped = new Map<string, Diagnostic[]>(); items.forEach((item) => { const key = item.entity_ref ?? `path:${item.path.split("/").filter(Boolean)[0] ?? "配置"}`; grouped.set(key, [...(grouped.get(key) ?? []), item]); }); return [...grouped]; }
function groupTitle(key: string) { if (!key.startsWith("entity:")) return key.slice(5); const [, , entityType, entityID] = key.split(":"); return `${entityTypeLabel(entityType)} · ${entityID}`; }
function entityTypeLabel(type: string) { return ({ placement: "广告位", frequency_policy: "频控策略", feature_switch: "功能开关", unit_binding: "环境绑定" } as Record<string, string>)[type] ?? type; }
function SeverityIcon({ diagnostic }: { diagnostic: Diagnostic }) { const category = categoryOf(diagnostic); return category === "blocking" ? <ShieldAlert size={18} /> : category === "warning" ? <TriangleAlert size={18} /> : <AlertCircle size={18} />; }
function formatDate(value: string) { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)); }
function toRequestError(cause: unknown) { if (cause instanceof ConflowAPIError) return { code: cause.code, requestId: cause.requestId }; if (cause instanceof ConflowNetworkError) return { code: "network_unavailable" }; return { code: "internal_error" }; }
