import { AlertTriangle, CheckCircle2, ChevronDown, ChevronRight, CircleAlert, Download, FileClock, LoaderCircle, RefreshCw, ShieldAlert, TriangleAlert } from "lucide-react";
import { useCallback, useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { ConflowAPIError, ConflowNetworkError, createPlan, getOperation, getPlan, planArtifactURL, type AffectedEntity, type Environment, type Operation, type Plan, type RemoteParameterChange } from "../../api/client";
import { Button } from "../ui/Button";
import { Modal } from "../ui/Dialog";
import { RequestError } from "../ui/StateViews";

const operationStorageKey = (environmentID: string) => `conflow.plan.operation.${environmentID}`;
const stageOrder = ["queued", "reading_remote", "compiling", "analyzing"];

export function ReleasePlan({ environment, onOpenConfiguration, onOpenRelease }: { environment: Environment; onOpenConfiguration: () => void; onOpenRelease: (planID: string) => void }) {
  const [operation, setOperation] = useState<Operation | null>(null);
  const [plan, setPlan] = useState<Plan | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<{ code: string; requestId?: string } | null>(null);
  const operationRef = useRef<string | null>(null);
  const startingEnvironmentRef = useRef<string | null>(null);

  const poll = useCallback(async (operationID: string, signal?: AbortSignal) => {
    try {
      const response = await getOperation(operationID, signal);
      setOperation(response.data);
      if (response.data.status === "succeeded" && response.data.result?.resource_type === "plan") {
        const nextPlan = await getPlan(response.data.result.resource_id, signal);
        setPlan(nextPlan.data); setLoading(false);
      } else if (["failed", "cancelled"].includes(response.data.status)) { setLoading(false); setError({ code: response.data.failure?.code ?? "operation_failed" }); }
    } catch (cause) { if (!(cause instanceof DOMException && cause.name === "AbortError")) { setLoading(false); setError(toRequestError(cause)); } }
  }, []);

  const start = useCallback(async (replace = false) => {
    const environmentID = environment.id;
    if (!replace && startingEnvironmentRef.current === environmentID) return;
    startingEnvironmentRef.current = environmentID;
    setLoading(true); setError(null); setPlan(null); setOperation(null);
    if (replace) sessionStorage.removeItem(operationStorageKey(environmentID));
    try {
      const response = await createPlan(environmentID);
      operationRef.current = response.data.operation_id;
      sessionStorage.setItem(operationStorageKey(environmentID), response.data.operation_id);
      setOperation(response.data);
    } catch (cause) { setLoading(false); setError(toRequestError(cause)); }
    finally {
      if (startingEnvironmentRef.current === environmentID) startingEnvironmentRef.current = null;
    }
  }, [environment.id]);

  useEffect(() => {
    const savedOperationID = sessionStorage.getItem(operationStorageKey(environment.id));
    setPlan(null); setOperation(null); setError(null);
    operationRef.current = savedOperationID;
    if (new URLSearchParams(window.location.hash.split("?")[1]).get("rebuild") === "1") void start(true);
    else if (savedOperationID) { setLoading(true); void poll(savedOperationID); }
    else if (startingEnvironmentRef.current !== environment.id) void start();
    return () => { operationRef.current = null; };
  }, [environment.id, poll, start]);

  useEffect(() => {
    const operationID = operationRef.current;
    if (!operationID || plan || error) return;
    const timer = window.setInterval(() => void poll(operationID), 1500);
    return () => window.clearInterval(timer);
  }, [error, plan, poll, operation]);

  useEffect(() => {
    const operationID = operationRef.current;
    if (!operationID || plan || error || typeof EventSource === "undefined") return;
    const stream = new EventSource(`/api/v1/events?operation_id=${encodeURIComponent(operationID)}`);
    stream.addEventListener("progress", () => void poll(operationID));
    stream.onerror = () => stream.close();
    return () => stream.close();
  }, [error, plan, poll, operation]);

  return <main className="page-container plan-page">
    <header className="page-heading plan-heading"><div><h1>发布计划</h1><p>{plan ? `计划 ${plan.plan_id} · 当前保存版本 ${plan.draft_revision}` : "正在为当前环境构建发布前审阅"}</p></div><Button onClick={() => void start(true)} disabled={loading} icon={<RefreshCw size={16} />}>重新构建计划</Button></header>
    {error ? <RequestError {...error} onDismiss={() => setError(null)} /> : null}
    {loading && !plan ? <OperationProgress operation={operation} /> : null}
    {plan ? <PlanReview plan={plan} environment={environment} onRebuild={() => void start(true)} onOpenConfiguration={onOpenConfiguration} onOpenRelease={onOpenRelease} /> : null}
  </main>;
}

function OperationProgress({ operation }: { operation: Operation | null }) {
  const activeIndex = operation ? Math.max(0, stageOrder.indexOf(operation.stage)) : 0;
  return <section className="operation-progress panel" aria-live="polite"><header><div><LoaderCircle className="spin" size={20} /><div><h2>正在构建发布计划</h2><p>读取线上配置、计算业务影响并分析风险。</p></div></div><code>{operation?.operation_id ?? "正在创建操作"}</code></header><ol>{stageOrder.map((stage, index) => <li className={index <= activeIndex ? "done" : ""} key={stage}><i>{index + 1}</i><span>{stageLabel(stage)}</span></li>)}</ol><div className="operation-progress-bar"><span style={{ width: `${((activeIndex + 1) / stageOrder.length) * 100}%` }} /></div><p className="muted-copy">页面刷新后会恢复此操作的状态；进度流中断时将自动继续读取当前状态。</p></section>;
}

function PlanReview({ plan, environment, onRebuild, onOpenConfiguration, onOpenRelease }: { plan: Plan; environment: Environment; onRebuild: () => void; onOpenConfiguration: () => void; onOpenRelease: (planID: string) => void }) {
  const invalid = plan.status === "invalidated" || plan.status === "expired";
  const [invalidDialogOpen, setInvalidDialogOpen] = useState(invalid);
  const [treeClosed, setTreeClosed] = useState(false);
  useEffect(() => setInvalidDialogOpen(invalid), [invalid, plan.plan_id]);
  const directChanges = plan.semantic_changes.length;
  return <>
    <section className={`plan-status ${invalid ? "plan-status--invalid" : plan.status === "preview_only" ? "plan-status--preview" : "plan-status--ready"}`}>{invalid ? <><FileClock size={18} /><strong>这份发布计划已失效</strong><span>{invalidationText(plan.invalidation_reason)}</span></> : plan.status === "preview_only" ? <><CircleAlert size={18} /><strong>不可发布</strong><span>{plan.blocking_reasons.map((item) => item.summary).join("；") || "服务端仅允许预览此计划。"}</span></> : <><CheckCircle2 size={18} /><strong>计划可审阅</strong><span>风险与发布条件均以服务端结果为准。</span></>}</section>
    <section className={invalid ? "plan-review plan-review--stale" : "plan-review"}>
      <div className="metric-grid plan-metrics"><Metric label="直接修改" value={`${directChanges} 项`} copy="用户明确修改" /><Metric label="受影响实体" value={`${plan.affected_entities.length} 个`} copy="由业务影响展开" /><Metric label="远端参数" value={`${new Set(plan.remote_parameter_changes.map((c) => c.parameter_key)).size} 项`} copy="最终写入目标" /><Metric label="最高风险" value={riskLabel(plan.severity)} copy={`${plan.risk_items.filter((item) => item.acknowledgement_required).length} 项需要确认`} risk={plan.severity} /></div>
      <div className="plan-layout">
        <div className="plan-main">
          <section className="semantic-tree panel">
            <button className="panel-section-toggle tree-heading" onClick={() => setTreeClosed((c) => !c)} aria-expanded={!treeClosed}>
              <div><h2>业务变更与影响</h2><p>{`${directChanges} 项直接修改 · ${plan.affected_entities.length} 个受影响实体 · ${new Set(plan.remote_parameter_changes.map((c) => c.parameter_key)).size} 个远端参数`}</p></div>
              <ChevronDown size={16} className={treeClosed ? "section-chevron section-chevron--closed" : "section-chevron"} />
            </button>
            {!treeClosed ? <SemanticTree plan={plan} /> : null}
          </section>
          <RemoteParamPanel plan={plan} />
        </div>
        <aside className="plan-sidebar">
          <section className="panel release-placeholder"><h2>下一步</h2><p>{invalid ? "旧计划仅供参考，重新构建后才能继续。" : plan.status === "preview_only" ? "此计划不可发布，请先处理服务端给出的原因。" : "继续进入独立发布步骤页，服务端会在提交时再次确认计划、版本和线上 ETag。"}</p><Button variant="primary" disabled={invalid || plan.status !== "ready"} onClick={() => onOpenRelease(plan.plan_id)} icon={<ChevronRight size={16} />}>发布到 {environment.name}</Button></section>
          <RemoteBaseline plan={plan} />
          <RiskPanel plan={plan} />
          <ArtifactPanel plan={plan} />
        </aside>
      </div>
      {invalid ? <section className="plan-invalid-actions"><AlertTriangle size={18} /><div><strong>旧计划保留为参考</strong><p>{invalidationText(plan.invalidation_reason)}</p></div><Button variant="primary" onClick={onRebuild} icon={<RefreshCw size={16} />}>重新构建计划</Button></section> : null}
      {!invalid && plan.status === "ready" && !plan.semantic_changes.length ? <section className="validation-empty"><CheckCircle2 size={28} /><div><h2>当前环境没有待发布的修改</h2><p>修改配置后可重新构建发布计划。</p><Button onClick={onOpenConfiguration}>返回配置</Button></div></section> : null}
    </section>
    <Modal open={invalid && invalidDialogOpen} onOpenChange={setInvalidDialogOpen} title="这份发布计划已失效" description="配置或线上配置已变化。旧计划只能查看，不能继续发布。">
      <div className="plan-invalid-dialog"><div><strong>失效原因</strong><p>{invalidationText(plan.invalidation_reason)}</p></div><Button variant="primary" onClick={onRebuild} icon={<RefreshCw size={16} />}>重新构建计划</Button></div>
    </Modal>
  </>;
}

export function SemanticTree({ plan }: { plan: Plan }) {
  const [openSemantic, setOpenSemantic] = useState<Set<string>>(new Set());
  const [openEntities, setOpenEntities] = useState<Set<string>>(new Set());
  const entities = new Map(plan.affected_entities.map((item) => [item.node_id, item]));
  const parameters = new Map(plan.remote_parameter_changes.map((item) => [item.node_id, item]));
  const toggle = (id: string, setter: Dispatch<SetStateAction<Set<string>>>) => setter((current) => { const next = new Set(current); next.has(id) ? next.delete(id) : next.add(id); return next; });
  return <div className="tree-list">{plan.semantic_changes.map((change) => { const open = openSemantic.has(change.node_id); const relatedEntities = change.affected_entity_node_ids.map((id) => entities.get(id)).filter(Boolean) as AffectedEntity[]; const relatedParameters = change.remote_parameter_node_ids.map((id) => parameters.get(id)).filter(Boolean) as RemoteParameterChange[]; return <div className="semantic-node" key={change.node_id}><button className="tree-row tree-row--semantic" onClick={() => toggle(change.node_id, setOpenSemantic)} aria-expanded={open}>{open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}<span><small>{changeKind(change.change_kind)}</small><strong>{change.summary}</strong>{change.before_summary || change.after_summary ? <em>{change.before_summary ?? "无"} → {change.after_summary ?? "无"}</em> : null}</span><b>{relatedEntities.length} 个实体</b></button>{open ? <div className="tree-children">{relatedEntities.map((entity) => { const entityOpen = openEntities.has(entity.node_id); const entityParameters = relatedParameters.filter((parameter) => parameter.affected_entity_node_ids.includes(entity.node_id)); return <div className="entity-node" key={entity.node_id}><button className="tree-row tree-row--entity" onClick={() => toggle(entity.node_id, setOpenEntities)} aria-expanded={entityOpen}>{entityOpen ? <ChevronDown size={16} /> : <ChevronRight size={16} />}<span><strong>{entityType(entity.entity_type)} · {entity.entity_id}</strong><em>{impactKind(entity.impact_kind)}</em></span><b>{entityParameters.length} 个远端参数</b></button>{entityOpen ? <div className="tree-children tree-children--remote">{entityParameters.map((parameter) => <RemoteParameter key={parameter.node_id} parameter={parameter} />)}{!entityParameters.length ? <p className="muted-copy">没有直接关联的远端参数。</p> : null}</div> : null}</div>; })}{!relatedEntities.length ? <div className="tree-children tree-children--remote">{relatedParameters.map((parameter) => <RemoteParameter key={parameter.node_id} parameter={parameter} />)}</div> : null}</div> : null}</div>; })}</div>;
}

function RemoteParameter({ parameter }: { parameter: RemoteParameterChange }) { return <div className="remote-parameter"><code>{parameter.parameter_key}</code><span>{changeKind(parameter.change_kind)}</span><strong>{parameter.before_summary ?? "无"} → {parameter.after_summary ?? "无"}</strong></div>; }
function formatParamValue(value: string | undefined): string {
  if (!value || value === "null") return "无";
  try {
    const parsed = JSON.parse(value);
    if (typeof parsed === "string") {
      try { return JSON.stringify(JSON.parse(parsed), null, 2); } catch { return parsed; }
    }
    return JSON.stringify(parsed, null, 2);
  } catch { return value; }
}
export function RemoteParamPanel({ plan }: { plan: Plan }) {
  const [collapsed, setCollapsed] = useState(false);
  const changes = plan.remote_parameter_changes;
  const seen = new Set<string>();
  const unique = changes.filter((c) => { if (seen.has(c.parameter_key)) return false; seen.add(c.parameter_key); return true; });
  if (!unique.length) return null;
  return <section className="panel remote-param-panel">
    <button className="panel-section-toggle" onClick={() => setCollapsed((c) => !c)} aria-expanded={!collapsed}>
      <h2>远端参数预览<small>{unique.length} 个参数</small></h2>
      <ChevronDown size={16} className={collapsed ? "section-chevron section-chevron--closed" : "section-chevron"} />
    </button>
    {!collapsed ? <table className="remote-param-table"><thead><tr><th>参数键</th><th>变更</th><th>变更前</th><th>变更后</th></tr></thead><tbody>{unique.map((c) => <tr key={c.parameter_key}><td><code>{c.parameter_key}</code></td><td><span className={`change-tag change-tag--${c.change_kind}`}>{changeKind(c.change_kind)}</span></td><td><pre className="param-value">{formatParamValue(c.before_summary)}</pre></td><td><pre className="param-value param-value--after">{formatParamValue(c.after_summary)}</pre></td></tr>)}</tbody></table> : null}
  </section>;
}
function RemoteBaseline({ plan }: { plan: Plan }) { const snapshot = plan.remote_snapshot; return <section className="panel remote-baseline"><h2>线上配置基线</h2>{snapshot.status === "available" ? <dl><div><dt>版本</dt><dd>{snapshot.version ?? "已读取"}</dd></div><div><dt>参数数量</dt><dd>{snapshot.summary?.parameter_count ?? "-"}</dd></div><div><dt>受管参数</dt><dd>{snapshot.summary?.managed_parameter_count ?? "-"}</dd></div><div><dt>条件值</dt><dd>{snapshot.summary?.condition_count ?? "-"}</dd></div></dl> : <p>当前无法读取线上配置，发布将保持不可用。</p>}</section>; }
export function RiskPanel({ plan }: { plan: Plan }) { const groups = ["blocking", "high", "medium", "low"] as const; return <section className="panel risk-panel"><h2>风险清单</h2>{plan.blocking_reasons.length ? <div className="blocking-reasons"><strong>阻断原因</strong>{plan.blocking_reasons.map((item) => <p key={item.reason_code}>{item.summary}</p>)}</div> : null}{groups.map((severity) => { const items = plan.risk_items.filter((item) => item.severity === severity); return items.length ? <div className="risk-group" key={severity}><h3 className={`risk-tag risk-tag--${severity === "blocking" ? "high" : severity}`}>{riskLabel(severity)}</h3>{items.map((item) => <p key={item.risk_item_id}>{item.summary}{item.entity_ref ? <> · <code>{item.entity_ref}</code></> : null}</p>)}</div> : null; })}{!plan.risk_items.length && !plan.blocking_reasons.length ? <p className="muted-copy">服务端未报告额外风险。</p> : null}</section>; }
function ArtifactPanel({ plan }: { plan: Plan }) { const artifacts = plan.artifact_metadata.filter((item) => item.available && (item.artifact_name === "review.json" || item.artifact_name === "review.md")); return <section className="panel artifact-panel"><h2>审阅文件</h2>{artifacts.length ? artifacts.map((artifact) => <a href={planArtifactURL(plan.plan_id, artifact.artifact_name)} key={artifact.artifact_name} download><Download size={15} />{artifact.artifact_name}</a>) : <p className="muted-copy">当前计划没有可下载的审阅文件。</p>}</section>; }
function Metric({ label, value, copy, risk }: { label: string; value: string; copy: string; risk?: string }) { return <div className="metric"><span className="metric-label">{label}</span><strong className={risk ? `risk-value risk-value--${risk}` : undefined}>{value}</strong><p>{copy}</p></div>; }
function stageLabel(value: string) { return ({ queued: "等待开始", reading_remote: "读取线上配置", compiling: "计算业务变更", analyzing: "分析影响与风险" } as Record<string, string>)[value] ?? "处理中"; }
function invalidationText(reason?: string) { return ({ draft_revision_changed: "配置已变化，旧计划不能继续发布。", source_digest_changed: "配置来源已变化，旧计划不能继续发布。", remote_etag_changed: "线上配置已变化，旧计划不能继续发布。", remote_snapshot_unavailable: "无法读取线上配置，旧计划不能继续发布。", ttl_expired: "计划已过期，请重新构建。", provider_capability_changed: "发布目标能力已变化，请重新构建。" } as Record<string, string>)[reason ?? ""] ?? "计划已失效，请重新构建。"; }
export function riskLabel(value: string) { return ({ low: "低", medium: "中", high: "高", blocking: "阻断" } as Record<string, string>)[value] ?? value; }
function changeKind(value: string) { return ({ created: "新增", updated: "修改", deleted: "删除", overridden: "环境专属修改" } as Record<string, string>)[value] ?? value; }
function entityType(value: string) { return ({ placement: "广告位", frequency_policy: "频控策略", feature_switch: "功能开关", unit_binding: "广告单元绑定" } as Record<string, string>)[value] ?? value; }
function impactKind(value: string) { return ({ direct: "直接修改", inherited: "继承影响", referenced: "引用影响", compiled: "编译结果" } as Record<string, string>)[value] ?? value; }
function toRequestError(cause: unknown) { if (cause instanceof ConflowAPIError) return { code: cause.code, requestId: cause.requestId }; if (cause instanceof ConflowNetworkError) return { code: "network_unavailable" }; return { code: "internal_error" }; }
