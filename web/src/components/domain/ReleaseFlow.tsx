import { AlertTriangle, Check, CheckCircle2, CircleAlert, LoaderCircle, RefreshCw, RotateCcw, ShieldAlert } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { ConflowAPIError, ConflowNetworkError, createRelease, createRollbackPreview, getOperation, getPlan, getRelease, getRollbackPreview, rollbackRelease, type Environment, type Operation, type Plan, type Release, type RollbackPreview } from "../../api/client";
import { RiskPanel, SemanticTree, riskLabel } from "./ReleasePlan";
import { Button } from "../ui/Button";

type ConfirmationRequirements = Plan["confirmation_requirements"];
type Reviewable = Pick<Plan, "risk_items" | "confirmation_requirements" | "severity">;
type FlowError = { code: string; message?: string; currentRemote?: Release["remote_before"] };
type StoredOperation = { operationID: string; resourceID: string; action: "publish" | "rollback_preview" | "rollback"; idempotencyKey: string };
type ConfirmationInput = { acknowledged: boolean; environmentID: string; riskIDs: string[] };

const releaseStages = ["queued", "validating_remote", "submitting", "verifying", "recording_audit", "completed"];
const operationKey = (environmentID: string) => `conflow.release.operation.${environmentID}`;

export function ReleaseFlow({ environment, planID, onOpenPlan, onOpenHistory, onOpenRollback }: { environment: Environment; planID: string; onOpenPlan: () => void; onOpenHistory: (releaseID?: string) => void; onOpenRollback: (releaseID: string) => void }) {
  const [plan, setPlan] = useState<Plan | null>(null);
  const [step, setStep] = useState<"review" | "confirm" | "executing" | "success" | "failure" | "conflict">("review");
  const [operation, setOperation] = useState<Operation | null>(null);
  const [release, setRelease] = useState<Release | null>(null);
  const [error, setError] = useState<FlowError | null>(null);
  const [confirmation, setConfirmation] = useState<ConfirmationInput>(emptyConfirmation);
  const idempotencyKey = useRef(newIdempotencyKey());
  const operationID = useRef<string | null>(null);

  const poll = useCallback(async (id: string, signal?: AbortSignal) => {
    try {
      const response = await getOperation(id, signal);
      setOperation(response.data);
      if (response.data.status === "succeeded") {
        if (response.data.result?.resource_type === "release") {
          const result = await getRelease(environment.id, response.data.result.resource_id, signal);
          setRelease(result.data);
        }
        setStep("success");
      } else if (response.data.status === "failed" || response.data.status === "cancelled") {
        setError({ code: response.data.failure?.code ?? "operation_failed", message: response.data.failure?.message });
        setStep("failure");
      }
    } catch (cause) { if (!isAbort(cause)) { setError(errorFrom(cause)); setStep("failure"); } }
  }, [environment.id]);

  useEffect(() => {
    const controller = new AbortController();
    void getPlan(planID, controller.signal).then((response) => setPlan(response.data)).catch((cause) => setError(errorFrom(cause)));
    const saved = readStoredOperation(environment.id, planID, "publish");
    if (saved) {
      idempotencyKey.current = saved.idempotencyKey;
      operationID.current = saved.operationID;
      setStep("executing");
      void poll(saved.operationID, controller.signal);
    }
    return () => controller.abort();
  }, [environment.id, planID, poll]);

  useEffect(() => {
    if (step !== "executing" || !operationID.current) return;
    const timer = window.setInterval(() => void poll(operationID.current!), 1500);
    return () => window.clearInterval(timer);
  }, [operation, poll, step]);

  const submit = async () => {
    if (!plan) return;
    setError(null);
    try {
      const response = await createRelease(environment.id, {
        plan_id: plan.plan_id,
        expected_draft_revision: plan.draft_revision,
        expected_remote_etag: plan.remote_etag ?? "",
        confirmation: confirmationFor(plan.confirmation_requirements, confirmation),
      }, idempotencyKey.current);
      operationID.current = response.data.operation_id;
      saveStoredOperation(environment.id, { operationID: response.data.operation_id, resourceID: plan.plan_id, action: "publish", idempotencyKey: idempotencyKey.current });
      setOperation(response.data);
      setStep("executing");
    } catch (cause) {
      const next = errorFrom(cause);
      setError(next);
      setStep(next.code === "remote_etag_mismatch" ? "conflict" : "failure");
    }
  };

  if (error && !plan) return <main className="page-container"><FlowErrorView error={error} onBack={onOpenPlan} /></main>;
  if (!plan) return <main className="page-container"><LoadingFlow label="正在读取发布计划" /></main>;
  if (plan.status !== "ready") return <main className="page-container"><PlanUnavailable onBack={onOpenPlan} /></main>;
  if (step === "conflict") return <main className="page-container"><RemoteConflict currentRemote={error?.currentRemote} onRebuild={() => { sessionStorage.removeItem(operationKey(environment.id)); onOpenPlan(); }} /></main>;
  if (step === "success") return <main className="page-container"><SuccessView environment={environment} release={release} operation={operation} onOpenHistory={onOpenHistory} onOpenRollback={onOpenRollback} /></main>;
  if (step === "failure") return <main className="page-container"><FailureView operation={operation} error={error} onRetry={operation?.remote_state === "unknown" ? undefined : () => void submit()} onBack={onOpenPlan} /></main>;

  return <main className="page-container release-flow-page">
    <ReleaseHeading eyebrow="PRODUCTION RELEASE" title={step === "executing" ? "正在发布" : `发布到 ${environment.name}`} description={step === "review" ? "独立步骤页，提交前会由服务端再次校验保存版本与线上配置。" : "确认要求和风险清单由服务端当前计划给出。"} />
    <StepIndicator step={step} />
    {step === "review" ? <ReviewStep plan={plan} environment={environment} onNext={() => setStep("confirm")} /> : null}
    {step === "confirm" ? <ConfirmationStep reviewable={plan} environment={environment} value={confirmation} onChange={setConfirmation} actionLabel={`发布到 ${environment.name}`} onBack={() => setStep("review")} onSubmit={() => void submit()} /> : null}
    {step === "executing" ? <ExecutionView environment={environment} operation={operation} /> : null}
  </main>;
}

export function RollbackFlow({ environment, releaseID, onOpenHistory, onOpenRecord }: { environment: Environment; releaseID: string; onOpenHistory: () => void; onOpenRecord: (releaseID: string) => void }) {
  const [preview, setPreview] = useState<RollbackPreview | null>(null);
  const [operation, setOperation] = useState<Operation | null>(null);
  const [step, setStep] = useState<"previewing" | "review" | "confirm" | "executing" | "success" | "failure" | "conflict">("previewing");
  const [release, setRelease] = useState<Release | null>(null);
  const [error, setError] = useState<FlowError | null>(null);
  const [confirmation, setConfirmation] = useState<ConfirmationInput>(emptyConfirmation);
  const operationID = useRef<string | null>(null);
  const idempotencyKey = useRef(newIdempotencyKey());

  const poll = useCallback(async (id: string, action: StoredOperation["action"], signal?: AbortSignal) => {
    try {
      const response = await getOperation(id, signal);
      setOperation(response.data);
      if (response.data.status === "succeeded") {
        if (action === "rollback_preview") {
          const result = await getRollbackPreview(environment.id, releaseID, signal);
          setPreview(result.data); setStep("review");
        } else if (response.data.result?.resource_type === "release") {
          const result = await getRelease(environment.id, response.data.result.resource_id, signal);
          setRelease(result.data); setStep("success");
        } else setStep("success");
      } else if (response.data.status === "failed" || response.data.status === "cancelled") {
        setError({ code: response.data.failure?.code ?? "operation_failed", message: response.data.failure?.message }); setStep("failure");
      }
    } catch (cause) { if (!isAbort(cause)) { setError(errorFrom(cause)); setStep("failure"); } }
  }, [environment.id, releaseID]);

  const beginPreview = useCallback(async () => {
    setError(null); setPreview(null); setStep("previewing");
    try {
      const response = await createRollbackPreview(environment.id, releaseID);
      operationID.current = response.data.operation_id;
      saveStoredOperation(environment.id, { operationID: response.data.operation_id, resourceID: releaseID, action: "rollback_preview", idempotencyKey: idempotencyKey.current });
      setOperation(response.data);
    } catch (cause) { setError(errorFrom(cause)); setStep("failure"); }
  }, [environment.id, releaseID]);

  useEffect(() => {
    const controller = new AbortController();
    const saved = readStoredOperation(environment.id, releaseID);
    if (saved && (saved.action === "rollback_preview" || saved.action === "rollback")) {
      idempotencyKey.current = saved.idempotencyKey; operationID.current = saved.operationID;
      setStep(saved.action === "rollback" ? "executing" : "previewing");
      void poll(saved.operationID, saved.action, controller.signal);
    } else void beginPreview();
    return () => controller.abort();
  }, [beginPreview, environment.id, poll, releaseID]);

  useEffect(() => {
    if ((step !== "previewing" && step !== "executing") || !operationID.current) return;
    const action = step === "previewing" ? "rollback_preview" : "rollback";
    const timer = window.setInterval(() => void poll(operationID.current!, action), 1500);
    return () => window.clearInterval(timer);
  }, [operation, poll, step]);

  const submit = async () => {
    if (!preview) return;
    setError(null);
    try {
      const response = await rollbackRelease(environment.id, releaseID, {
        rollback_preview_id: preview.rollback_preview_id,
        expected_remote_etag: preview.expected_remote_etag,
        confirmation: confirmationFor(preview.confirmation_requirements, confirmation),
      }, idempotencyKey.current);
      operationID.current = response.data.operation_id;
      saveStoredOperation(environment.id, { operationID: response.data.operation_id, resourceID: releaseID, action: "rollback", idempotencyKey: idempotencyKey.current });
      setOperation(response.data); setStep("executing");
    } catch (cause) { const next = errorFrom(cause); setError(next); setStep(next.code === "remote_etag_mismatch" ? "conflict" : "failure"); }
  };

  if (step === "conflict") return <main className="page-container"><RemoteConflict currentRemote={error?.currentRemote} onRebuild={() => void beginPreview()} rebuildLabel="重新生成预览" /></main>;
  if (step === "success") return <main className="page-container"><SuccessView environment={environment} release={release} operation={operation} rollback onOpenHistory={(completedReleaseID) => onOpenRecord(completedReleaseID ?? releaseID)} /></main>;
  if (step === "failure") return <main className="page-container"><FailureView operation={operation} error={error} onRetry={operation?.remote_state === "unknown" ? undefined : () => void beginPreview()} onBack={onOpenHistory} /></main>;
  if (step === "previewing") return <main className="page-container"><ReleaseHeading eyebrow="PRODUCTION ROLLBACK" title="正在生成回滚预览" description="系统会基于当前线上模板重新生成反向计划。" /><ExecutionView environment={environment} operation={operation} rollbackPreview /></main>;
  if (!preview) return <main className="page-container"><LoadingFlow label="正在读取回滚预览" /></main>;
  if (preview.status !== "ready") return <main className="page-container"><PreviewInvalid preview={preview} onRebuild={() => void beginPreview()} onBack={onOpenHistory} /></main>;
  return <main className="page-container release-flow-page">
    <ReleaseHeading eyebrow="PRODUCTION ROLLBACK" title={step === "confirm" ? "确认回滚" : "回滚预览"} description="回滚会生成一条新的发布记录；确认要求由服务端的预览结果决定。" />
    {step === "review" ? <RollbackReview preview={preview} environment={environment} onNext={() => setStep("confirm")} onBack={onOpenHistory} /> : null}
    {step === "confirm" ? <ConfirmationStep reviewable={preview} environment={environment} value={confirmation} onChange={setConfirmation} actionLabel={`回滚到版本 ${preview.target_remote_version}`} onBack={() => setStep("review")} onSubmit={() => void submit()} danger /> : null}
    {step === "executing" ? <ExecutionView environment={environment} operation={operation} rollback /> : null}
  </main>;
}

function ReviewStep({ plan, environment, onNext }: { plan: Plan; environment: Environment; onNext: () => void }) {
  return <div className="release-flow-layout"><section className="panel semantic-tree"><header className="tree-heading"><div><h2>审阅发布计划</h2><p>{plan.semantic_changes.length} 项直接修改 · {plan.affected_entities.length} 个受影响实体 · {new Set(plan.remote_parameter_changes.map((c) => c.parameter_key)).size} 个远端参数</p></div><span className={`risk-tag risk-tag--${plan.severity === "blocking" ? "high" : plan.severity}`}>{riskLabel(plan.severity)}风险</span></header><SemanticTree plan={plan} /></section><aside className="release-flow-sidebar"><Button variant="primary" onClick={onNext}>继续确认风险</Button><TargetCard environment={environment} planID={plan.plan_id} remoteVersion={plan.remote_snapshot.version} /><RiskPanel plan={plan} /></aside></div>;
}

function RollbackReview({ preview, environment, onNext, onBack }: { preview: RollbackPreview; environment: Environment; onNext: () => void; onBack: () => void }) {
  const reviewPlan = preview as unknown as Plan;
  return <><section className="rollback-warning"><AlertTriangle size={18} /><div><strong>目标版本早于当前线上版本</strong><p>将基于当前线上配置回滚到 Firebase 版本 {preview.target_remote_version}，不会复用旧 ETag。</p></div></section><div className="release-flow-layout"><section className="panel semantic-tree"><header className="tree-heading"><div><h2>反向业务变更</h2><p>{preview.semantic_changes.length} 项变更 · 当前线上版本 {preview.current_remote.version}</p></div><span className={`risk-tag risk-tag--${preview.severity}`}>{riskLabel(preview.severity)}风险</span></header><SemanticTree plan={reviewPlan} /></section><aside className="release-flow-sidebar"><Button variant="danger" onClick={onNext} icon={<RotateCcw size={16} />}>继续确认回滚</Button><Button onClick={onBack}>取消</Button><TargetCard environment={environment} remoteVersion={preview.target_remote_version} releaseID={preview.target_release_id} /><RiskPanel plan={reviewPlan} /></aside></div></>;
}

function ConfirmationStep({ reviewable, environment, value, onChange, actionLabel, onBack, onSubmit, danger = false }: { reviewable: Reviewable; environment: Environment; value: ConfirmationInput; onChange: (value: ConfirmationInput) => void; actionLabel: string; onBack: () => void; onSubmit: () => void; danger?: boolean }) {
  const requirements = reviewable.confirmation_requirements;
  const requiredRisks = reviewable.risk_items.filter((risk) => requirements.required_risk_item_ids.includes(risk.risk_item_id));
  const ready = (!requirements.requires_acknowledgement || value.acknowledged) && (requirements.environment_id_requirement !== "required" || value.environmentID === environment.id) && requirements.required_risk_item_ids.every((id) => value.riskIDs.includes(id));
  return <div className="release-flow-layout"><section className="panel confirmation-panel"><header><h2>确认风险</h2><p>所有必填项均由服务端发布计划声明，前端只收集确认结果。</p></header>{requiredRisks.length ? <div className="confirmation-risks"><h3>逐项确认高风险修改</h3>{requiredRisks.map((risk) => <label className="confirmation-risk" key={risk.risk_item_id}><input type="checkbox" checked={value.riskIDs.includes(risk.risk_item_id)} onChange={(event) => onChange({ ...value, riskIDs: event.target.checked ? [...value.riskIDs, risk.risk_item_id] : value.riskIDs.filter((id) => id !== risk.risk_item_id) })} /><span><strong>{risk.summary}</strong>{risk.entity_ref ? <code className="risk-entity-ref">{risk.entity_ref}</code> : null}<small>{riskLabel(risk.severity)}风险 · 必须逐项确认</small></span></label>)}</div> : null}{requirements.requires_acknowledgement ? <label className="confirmation-check"><input type="checkbox" checked={value.acknowledged} onChange={(event) => onChange({ ...value, acknowledged: event.target.checked })} /><span><strong>我已审阅影响范围和风险</strong><small>线上写入由服务端操作执行，不能由本页跳过。</small></span></label> : null}{requirements.environment_id_requirement === "required" ? <label className="confirmation-input">输入环境 ID 以确认<input value={value.environmentID} onChange={(event) => onChange({ ...value, environmentID: event.target.value })} placeholder={environment.id} aria-label="输入环境 ID 以确认" /><small>环境显示名称为 {environment.name}，稳定 ID 为 <code>{environment.id}</code>。</small></label> : null}</section><aside className="release-flow-sidebar"><Button variant={danger ? "danger" : "primary"} disabled={!ready} onClick={onSubmit}>{actionLabel}</Button><Button onClick={onBack}>返回审阅</Button><section className="panel confirmation-summary"><ShieldAlert size={18} /><h2>最终确认</h2><p>最高风险为 {riskLabel(reviewable.severity)}。确认后会由服务端校验要求、计划版本和线上 ETag。</p></section></aside></div>;
}

function confirmationFor(requirements: ConfirmationRequirements, value: ConfirmationInput) {
  return {
    acknowledged: !requirements.requires_acknowledgement || value.acknowledged,
    environment_id: requirements.environment_id_requirement === "required" ? value.environmentID : undefined,
    acknowledged_risk_item_ids: value.riskIDs,
  };
}

function ExecutionView({ environment, operation, rollback = false, rollbackPreview = false }: { environment: Environment; operation: Operation | null; rollback?: boolean; rollbackPreview?: boolean }) {
  const active = Math.max(0, releaseStages.indexOf(operation?.stage ?? "queued"));
  const title = rollbackPreview ? "正在生成回滚预览" : rollback ? `正在回滚 ${environment.name}` : `正在发布到 ${environment.name}`;
  return <section className="execution-panel panel" aria-live="polite"><header><div><LoaderCircle className="spin" size={20} /><div><h2>{title}</h2><p><code>{operation?.operation_id ?? "正在创建操作"}</code> · 刷新页面后会恢复当前 Operation。</p></div></div><span>{active + 1} / {releaseStages.length}</span></header><div className="operation-progress-bar"><span style={{ width: `${((active + 1) / releaseStages.length) * 100}%` }} /></div><ol className="execution-stages">{releaseStages.map((stage, index) => <li key={stage} className={index < active ? "done" : index === active ? "active" : ""}><i>{index < active ? <Check size={14} /> : index + 1}</i><span>{releaseStageLabel(stage)}</span><small>{index < active ? "已完成" : index === active ? "进行中" : "等待"}</small></li>)}</ol><p className="operation-recovery"><CircleAlert size={16} />若连接中断，重新打开页面会按 operation ID 恢复状态，不会重复发起同一个操作。</p></section>;
}

function SuccessView({ environment, release, operation, rollback = false, onOpenHistory, onOpenRollback }: { environment: Environment; release: Release | null; operation: Operation | null; rollback?: boolean; onOpenHistory: (releaseID?: string) => void; onOpenRollback?: (releaseID: string) => void }) {
  const version = release?.remote_after?.version ?? "已验证";
  return <section className="release-success"><span className="success-mark"><CheckCircle2 size={32} /></span><h1>{environment.name} {rollback ? "回滚成功" : "发布成功"}</h1><p>Firebase 版本 {version} 已验证，审计记录已写入。</p><dl className="panel release-success-summary"><div><dt>发布记录</dt><dd><code>{release?.release_id ?? "正在写入"}</code></dd></div><div><dt>Operation</dt><dd><code>{operation?.operation_id ?? "-"}</code></dd></div><div><dt>Firebase 版本</dt><dd>{version}</dd></div><div><dt>线上状态</dt><dd>{remoteStateLabel(operation?.remote_state ?? release?.remote_state)}</dd></div></dl><div className="release-success-actions"><Button variant="primary" onClick={() => onOpenHistory(release?.release_id)}>查看发布记录</Button>{!rollback && release && onOpenRollback ? <Button onClick={() => onOpenRollback(release.release_id)} icon={<RotateCcw size={16} />}>预览回滚</Button> : null}</div></section>;
}

function FailureView({ operation, error, onRetry, onBack }: { operation: Operation | null; error: FlowError | null; onRetry?: () => void; onBack: () => void }) {
  const state = operation?.remote_state ?? "unchanged";
  const next = state === "unknown" ? "先在 Firebase 核验线上配置；结果不确定时不能盲目重试。" : state === "changed" ? "线上可能已变化。查看发布记录并核验版本后再决定下一步。" : "线上未发生变化。修正问题后可使用相同幂等键重试。";
  return <section className="failure-view"><span className="failure-mark"><AlertTriangle size={30} /></span><h1>发布未完成</h1><p>{error?.message ?? "服务端 Operation 未能完成。"}</p><div className="failure-answers panel"><section><strong>线上现在是什么状态？</strong><p>{remoteStateLabel(state)}</p></section><section><strong>下一步做什么？</strong><p>{next}</p></section></div><div className="release-success-actions">{onRetry ? <Button variant="primary" onClick={onRetry} icon={<RefreshCw size={16} />}>重试</Button> : null}<Button onClick={onBack}>返回</Button></div></section>;
}

function RemoteConflict({ currentRemote, onRebuild, rebuildLabel = "重新构建计划" }: { currentRemote?: Release["remote_before"]; onRebuild: () => void; rebuildLabel?: string }) {
  return <section className="conflict-view"><span className="failure-mark"><AlertTriangle size={30} /></span><h1>线上配置已变化</h1><p>提交已中止，旧计划或旧回滚预览不可复用。请重新读取线上配置后再次审阅。</p>{currentRemote ? <RemoteSummary title="当前线上摘要" remote={currentRemote} /> : null}<Button variant="primary" onClick={onRebuild} icon={<RefreshCw size={16} />}>{rebuildLabel}</Button></section>;
}

function PreviewInvalid({ preview, onRebuild, onBack }: { preview: RollbackPreview; onRebuild: () => void; onBack: () => void }) { return <section className="conflict-view"><span className="failure-mark"><AlertTriangle size={30} /></span><h1>回滚预览已失效</h1><p>{preview.status === "expired" ? "预览已过期，必须重新读取线上配置。" : "线上配置已变化，旧回滚预览不能继续执行。"}</p><div className="release-success-actions"><Button variant="primary" onClick={onRebuild} icon={<RefreshCw size={16} />}>重新生成预览</Button><Button onClick={onBack}>返回发布记录</Button></div></section>; }
function TargetCard({ environment, planID, remoteVersion, releaseID }: { environment: Environment; planID?: string; remoteVersion?: string; releaseID?: string }) { return <section className="panel target-card"><h2>发布目标</h2><dl><div><dt>环境</dt><dd>{environment.name}</dd></div><div><dt>Firebase</dt><dd><code>{environment.provider.project_id}</code></dd></div>{planID ? <div><dt>计划</dt><dd><code>{planID}</code></dd></div> : null}{releaseID ? <div><dt>目标记录</dt><dd><code>{releaseID}</code></dd></div> : null}<div><dt>目标版本</dt><dd>{remoteVersion ?? "当前版本"}</dd></div></dl></section>; }
function RemoteSummary({ title, remote }: { title: string; remote: NonNullable<Release["remote_before"]> }) { return <section className="panel remote-summary"><h2>{title}</h2><dl><div><dt>Firebase 版本</dt><dd>{remote.version}</dd></div><div><dt>参数</dt><dd>{remote.summary.parameter_count}</dd></div><div><dt>受管参数</dt><dd>{remote.summary.managed_parameter_count}</dd></div><div><dt>条件</dt><dd>{remote.summary.condition_count}</dd></div></dl></section>; }
function StepIndicator({ step }: { step: "review" | "confirm" | "executing" }) { const active = step === "review" ? 0 : step === "confirm" ? 1 : 2; return <ol className="release-steps">{["审阅", "确认风险", "执行"].map((label, index) => <li className={index < active ? "done" : index === active ? "active" : ""} key={label}><i>{index < active ? <Check size={14} /> : index + 1}</i><span>{label}</span></li>)}</ol>; }
function ReleaseHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) { return <header className="page-heading release-heading"><div><span className="release-eyebrow">{eyebrow}</span><h1>{title}</h1><p>{description}</p></div></header>; }
function LoadingFlow({ label }: { label: string }) { return <section className="operation-progress panel"><LoaderCircle className="spin" size={20} /><h2>{label}</h2></section>; }
function PlanUnavailable({ onBack }: { onBack: () => void }) { return <section className="conflict-view"><span className="failure-mark"><AlertTriangle size={30} /></span><h1>发布计划不可用</h1><p>计划已失效或仅可预览，必须返回发布计划重新构建后才能提交。</p><Button variant="primary" onClick={onBack} icon={<RefreshCw size={16} />}>重新构建计划</Button></section>; }
function FlowErrorView({ error, onBack }: { error: FlowError; onBack: () => void }) { return <section className="conflict-view"><span className="failure-mark"><AlertTriangle size={30} /></span><h1>无法继续发布流程</h1><p>{error.message ?? "本地服务未返回可用数据。"}</p><Button onClick={onBack}>返回发布计划</Button></section>; }
function releaseStageLabel(stage: string) { return ({ queued: "等待开始", validating_remote: "发布前检查", submitting: "写入 Firebase Remote Config", verifying: "验证远端版本", recording_audit: "写入审计记录", completed: "已完成" } as Record<string, string>)[stage] ?? "处理中"; }
function remoteStateLabel(state?: Operation["remote_state"]) { return ({ unchanged: "线上未变化", changed: "线上已变化", unknown: "线上状态未知，必须核验" } as Record<string, string>)[state ?? "unchanged"]; }
function newIdempotencyKey() { return typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : `release-${Date.now()}-${Math.random().toString(16).slice(2)}`; }
function emptyConfirmation(): ConfirmationInput { return { acknowledged: false, environmentID: "", riskIDs: [] }; }
function isAbort(error: unknown) { return error instanceof DOMException && error.name === "AbortError"; }
function errorFrom(cause: unknown): FlowError { if (cause instanceof ConflowAPIError) return { code: cause.code, message: cause.message, currentRemote: cause.currentRemote }; if (cause instanceof ConflowNetworkError) return { code: "network_unavailable", message: "无法连接本地服务；发布结果尚未确认。" }; return { code: "internal_error", message: "本地服务未完成请求。" }; }
function saveStoredOperation(environmentID: string, value: StoredOperation) { sessionStorage.setItem(operationKey(environmentID), JSON.stringify(value)); }
function readStoredOperation(environmentID: string, resourceID: string, action?: StoredOperation["action"]) { try { const value = JSON.parse(sessionStorage.getItem(operationKey(environmentID)) ?? "null") as StoredOperation | null; return value && value.resourceID === resourceID && (!action || value.action === action) ? value : null; } catch { return null; } }
