import { ArrowLeft, ChevronRight, CircleAlert, Link2, LoaderCircle, Plus, Save, Search, ShieldAlert, ShieldCheck, SlidersHorizontal, Trash2, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ConflowAPIError,
  ConflowNetworkError,
  createDraftEntity,
  deleteDraftEntity,
  getDraft,
  getDraftDiagnostics,
  getDraftEntity,
  getDraftEntityReferences,
  getPackSchema,
  listDraftEntities,
  replaceDraftEntity,
  validateDraft,
  type Diagnostic,
  type DraftStructuralErrorDetail,
  type DraftView,
  type EntityRecord,
  type EntityReference,
  type EntityView,
  type Environment,
  type FieldSchema,
  type PackSchema,
  type ValidationResult,
} from "../../api/client";
import { Button } from "../ui/Button";
import { Modal } from "../ui/Dialog";
import { RequestError } from "../ui/StateViews";

type EditorRoute = { mode: "list" } | { mode: "detail"; id?: string; section?: "bindings" };
type EditorTab = "placement" | "frequency_policy" | "feature_switch" | "unit_binding";
type EntityConflict = { local: EntityRecord; state: DraftView; revision: number; entityType?: string };
type BindingLoad = Record<string, EntityView[]>;
type DeleteTarget = { entity: EntityView; entityType: "placement" | "frequency_policy" };
type DiagnosticCategory = "blocking" | "warning" | "info";

export function ConfigurationEditor({ environment, environments, revision, packRef, focusEntityRef, onRevision, onValidation }: {
  environment: Environment;
  environments: Environment[];
  revision: number;
  packRef: string;
  focusEntityRef?: string;
  onRevision: (revision: number, dirty: boolean) => void;
  onValidation?: (result: ValidationResult | null) => void;
}) {
  const [route, setRoute] = useState<EditorRoute>({ mode: "list" });
  const [schema, setSchema] = useState<PackSchema | null>(null);
  const [placements, setPlacements] = useState<EntityView[]>([]);
  const [policies, setPolicies] = useState<EntityView[]>([]);
  const [switches, setSwitches] = useState<EntityView[]>([]);
  const [draft, setDraft] = useState<DraftView | null>(null);
  const [bindings, setBindings] = useState<BindingLoad>({});
  const [tab, setTab] = useState<EditorTab>("placement");
  const [editingPolicy, setEditingPolicy] = useState<EntityView | null>(null);
  const [deleting, setDeleting] = useState<DeleteTarget | null>(null);
  const [blockedReferences, setBlockedReferences] = useState<{ target: DeleteTarget; references: EntityReference[] } | null>(null);
  const [validation, setValidation] = useState<ValidationResult | null>(null);
  const [validating, setValidating] = useState(false);
  const [validationError, setValidationError] = useState<{ code: string; requestId?: string } | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<{ code: string; requestId?: string } | null>(null);

  const loadList = useCallback(async (signal?: AbortSignal) => {
    setLoading(true); setError(null);
    try {
      const diagnostics = getDraftDiagnostics(environment.id, signal).catch((cause) => {
        if (cause instanceof ConflowAPIError && cause.code === "validation_not_found") return null;
        throw cause;
      });
      const [nextSchema, nextPlacements, nextPolicies, nextSwitches, nextDraft, nextBindings, nextDiagnostics] = await Promise.all([
        getPackSchema(packRef, signal),
        listDraftEntities(environment.id, "placement", signal),
        listDraftEntities(environment.id, "frequency_policy", signal),
        listDraftEntities(environment.id, "feature_switch", signal),
        getDraft(environment.id, signal),
        Promise.all(environments.map(async (item) => [item.id, (await listDraftEntities(item.id, "unit_binding", signal)).data] as const)),
        diagnostics,
      ]);
      setSchema(nextSchema.data); setPlacements(nextPlacements.data); setPolicies(nextPolicies.data); setSwitches(nextSwitches.data); setDraft(nextDraft.data); setBindings(Object.fromEntries(nextBindings)); setValidation(nextDiagnostics?.data ?? null); onValidation?.(nextDiagnostics?.data ?? null);
      onRevision(nextDraft.meta.revision, nextDraft.data.dirty);
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === "AbortError") return;
      setError(toRequestError(cause));
    } finally { if (!signal?.aborted) setLoading(false); }
  }, [environment.id, environments, onRevision, onValidation, packRef]);

  useEffect(() => { const controller = new AbortController(); void loadList(controller.signal); return () => controller.abort(); }, [loadList]);
  useEffect(() => { setRoute({ mode: "list" }); }, [environment.id]);
  useEffect(() => {
    if (!focusEntityRef) return;
    const [, , entityType, entityID] = focusEntityRef.split(":");
    if (entityType === "placement") setRoute({ mode: "detail", id: entityID });
    else if (entityType === "frequency_policy") { setTab("frequency_policy"); setEditingPolicy(policies.find((item) => item.entity_id === entityID) ?? null); }
    else if (entityType === "feature_switch") setTab("feature_switch");
    else if (entityType === "unit_binding") setTab("unit_binding");
  }, [focusEntityRef, policies]);

  const runValidation = async () => {
    setValidating(true); setValidationError(null);
    try { const result = (await validateDraft(environment.id)).data; setValidation(result); onValidation?.(result); }
    catch (cause) { setValidationError(toRequestError(cause)); }
    finally { setValidating(false); }
  };

  if (route.mode === "detail") {
    return <PlacementDetail key={`${environment.id}:${route.id ?? "new"}`} environment={environment} environments={environments} revision={revision} schema={schema} validation={validation} placementID={route.id} focusBindings={route.section === "bindings"} onBack={() => { setRoute({ mode: "list" }); void loadList(); }} onSaved={(nextRevision, dirty) => { onRevision(nextRevision, dirty); void loadList(); }} />;
  }

  const openReference = (reference: EntityReference) => {
    setBlockedReferences(null); setEditingPolicy(null);
    if (reference.entity_type === "placement") setRoute({ mode: "detail", id: reference.entity_id });
  };
  return <>
    <ConfigurationList tab={tab} onTabChange={setTab} placements={placements} policies={policies} switches={switches} draft={draft} bindings={bindings} environments={environments} revision={revision} loading={loading} error={error} validation={validation} validating={validating} validationError={validationError} onRetry={() => void loadList()} onValidate={() => void runValidation()} onDismissValidationError={() => setValidationError(null)} onOpenPlacement={(id) => setRoute({ mode: "detail", id })} onOpenBinding={(id) => setRoute({ mode: "detail", id, section: "bindings" })} onCreate={() => setRoute({ mode: "detail" })} onOpenPolicy={setEditingPolicy} onDelete={(entity, entityType) => setDeleting({ entity, entityType })} onSwitchSaved={() => void loadList()} />
    <FrequencyDrawer policy={editingPolicy} environment={environment} revision={revision} draft={draft} diagnostics={validation?.diagnostics ?? []} onClose={() => setEditingPolicy(null)} onSaved={() => { setEditingPolicy(null); void loadList(); }} onDelete={(entity) => { setEditingPolicy(null); setDeleting({ entity, entityType: "frequency_policy" }); }} onOpenReference={openReference} />
    <DeleteEntityDialog target={deleting} environment={environment} revision={revision} draft={draft} onClose={() => setDeleting(null)} onDeleted={() => { setDeleting(null); void loadList(); }} onBlocked={(target, references) => { setDeleting(null); setBlockedReferences({ target, references }); }} />
    <ReferencedDeleteDialog blocked={blockedReferences} onClose={() => setBlockedReferences(null)} onOpenReference={openReference} />
  </>;
}

function ConfigurationList({ tab, onTabChange, placements, policies, switches, draft, bindings, environments, revision, loading, error, validation, validating, validationError, onRetry, onValidate, onDismissValidationError, onOpenPlacement, onOpenBinding, onCreate, onOpenPolicy, onDelete, onSwitchSaved }: {
  tab: EditorTab;
  onTabChange: (tab: EditorTab) => void;
  placements: EntityView[];
  policies: EntityView[];
  switches: EntityView[];
  draft: DraftView | null;
  bindings: BindingLoad;
  environments: Environment[];
  revision: number;
  loading: boolean;
  error: { code: string; requestId?: string } | null;
  validation: ValidationResult | null;
  validating: boolean;
  validationError: { code: string; requestId?: string } | null;
  onRetry: () => void;
  onValidate: () => void;
  onDismissValidationError: () => void;
  onOpenPlacement: (id: string) => void;
  onOpenBinding: (id: string) => void;
  onCreate: () => void;
  onOpenPolicy: (policy: EntityView) => void;
  onDelete: (entity: EntityView, entityType: "placement" | "frequency_policy") => void;
  onSwitchSaved: () => void;
}) {
  const title = ({ placement: "配置", frequency_policy: "频控策略", feature_switch: "功能开关", unit_binding: "环境绑定" } as Record<EditorTab, string>)[tab];
  const description = ({ placement: "按业务对象维护广告位配置。", frequency_policy: "通用频控值会影响引用它的广告位。", feature_switch: "开关默认值的风险与回滚方式由配置包定义。", unit_binding: "跨广告位查看各环境的广告单元绑定。" } as Record<EditorTab, string>)[tab];
  return <main className="page-container configuration-page">
    <header className="page-heading configuration-heading"><div><h1>{title}</h1><p>{description}</p></div><div className="configuration-actions"><Button icon={validating ? <LoaderCircle className="spin" size={16} /> : <ShieldCheck size={16} />} disabled={validating || loading} onClick={onValidate}>{validating ? "正在校验" : validation?.status === "stale" ? "重新运行校验" : "运行校验"}</Button>{tab === "placement" ? <Button variant="primary" icon={<Plus size={17} />} onClick={onCreate}>新建广告位</Button> : null}</div></header>
    <div className="entity-tabs" role="tablist" aria-label="配置对象">
      <TabButton active={tab === "placement"} onClick={() => onTabChange("placement")}>广告位</TabButton>
      <TabButton active={tab === "frequency_policy"} onClick={() => onTabChange("frequency_policy")}>频控策略</TabButton>
      <TabButton active={tab === "feature_switch"} onClick={() => onTabChange("feature_switch")}>功能开关</TabButton>
      <TabButton active={tab === "unit_binding"} onClick={() => onTabChange("unit_binding")}>环境绑定</TabButton>
    </div>
    {validation ? <ValidationSummary result={validation} validating={validating} onValidate={onValidate} /> : null}
    {error ? <RequestError {...error} onDismiss={onRetry} /> : null}
    {validationError ? <RequestError {...validationError} onDismiss={onDismissValidationError} /> : null}
    {tab === "placement" ? <PlacementTable placements={placements} draft={draft} bindings={bindings} environmentCount={environments.length} diagnostics={validation?.diagnostics ?? []} loading={loading} onOpen={onOpenPlacement} onCreate={onCreate} onDelete={onDelete} /> : null}
    {tab === "frequency_policy" ? <FrequencyTable policies={policies} diagnostics={validation?.diagnostics ?? []} loading={loading} onOpen={onOpenPolicy} onDelete={onDelete} /> : null}
    {tab === "feature_switch" ? <FeatureSwitchTable switches={switches} diagnostics={validation?.diagnostics ?? []} environment={environments.find((item) => item.id === draft?.environment_id) ?? environments[0]} revision={revision} draft={draft} loading={loading} onSaved={onSwitchSaved} /> : null}
    {tab === "unit_binding" ? <BindingOverview placements={placements} bindings={bindings} diagnostics={validation?.diagnostics ?? []} environments={environments} loading={loading} onOpen={onOpenBinding} /> : null}
  </main>;
}

function TabButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: string }) {
  return <button className={active ? "entity-tab entity-tab--active" : "entity-tab"} role="tab" aria-selected={active} onClick={onClick}>{children}</button>;
}

function ValidationSummary({ result, validating, onValidate }: { result: ValidationResult; validating: boolean; onValidate: () => void }) {
  const counts = diagnosticCounts(result.diagnostics);
  return <section className={`validation-summary validation-summary--${result.readiness}`} aria-live="polite"><div><strong>{result.readiness === "ready" ? "可发布" : "存在阻断"}</strong><span>阻断 {counts.blocking} · 警告 {counts.warning} · 建议 {counts.info}</span><span>校验时间 {formatValidatedAt(result.validated_at)}</span></div>{result.status === "stale" ? <div className="validation-stale"><span>结果可能已过期</span><Button variant="secondary" disabled={validating} onClick={onValidate}>{validating ? "正在校验" : "重新运行校验"}</Button></div> : null}</section>;
}

function DiagnosticAnchor({ diagnostics }: { diagnostics: Diagnostic[] }) {
  if (diagnostics.length === 0) return null;
  const category = highestDiagnosticCategory(diagnostics);
  return <span className={`diagnostic-anchor diagnostic-anchor--${category}`} aria-label={`${diagnosticCategoryLabel(category)} ${diagnostics.length} 项`} title={`${diagnosticCategoryLabel(category)} ${diagnostics.length} 项`}><i />{diagnostics.length}</span>;
}

function EntityDiagnostics({ diagnostics, title }: { diagnostics: Diagnostic[]; title: string }) {
  if (diagnostics.length === 0) return null;
  return <section className="entity-diagnostics" aria-label={title}><header><h2>{title}</h2><DiagnosticAnchor diagnostics={diagnostics} /></header><ul>{diagnostics.map((diagnostic) => { const category = diagnosticCategory(diagnostic); return <li key={`${diagnostic.code}:${diagnostic.path}`}><span className={`diagnostic-category diagnostic-category--${category}`}>{diagnosticCategoryLabel(category)}</span><div><strong>{diagnostic.message}</strong><p>建议：{diagnostic.fix_suggestion}</p>{diagnostic.documentation_url ? <a href={diagnostic.documentation_url} target="_blank" rel="noreferrer">查看说明</a> : null}</div></li>; })}</ul></section>;
}

function PlacementTable({ placements, draft, bindings, environmentCount, diagnostics, loading, onOpen, onCreate, onDelete }: {
  placements: EntityView[];
  draft: DraftView | null;
  bindings: BindingLoad;
  environmentCount: number;
  diagnostics: Diagnostic[];
  loading: boolean;
  onOpen: (id: string) => void;
  onCreate: () => void;
  onDelete: (entity: EntityView, entityType: "placement") => void;
}) {
  const [type, setType] = useState("all");
  const [query, setQuery] = useState("");
  const [dirtyOnly, setDirtyOnly] = useState(false);
  const rows = useMemo(() => placements.filter((placement) => {
    const fields = placement.effective.value.fields;
    const matchesType = type === "all" || fields.ad_type === type;
    const text = `${placement.entity_id} ${String(fields.key ?? "")}`.toLowerCase();
    return matchesType && text.includes(query.toLowerCase()) && (!dirtyOnly || isEntityDirty(placement));
  }), [dirtyOnly, placements, query, type]);

  return <>
    <div className="entity-toolbar">
      <label className="toolbar-select"><span>类型</span><select aria-label="按类型筛选" value={type} onChange={(event) => setType(event.target.value)}><option value="all">全部类型</option><option value="app_open">App Open</option><option value="interstitial">插屏</option><option value="native">原生</option></select></label>
      <label className="toolbar-search"><Search size={16} aria-hidden="true" /><span className="sr-only">搜索名称或 key</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索名称或 key" /></label>
      <label className="dirty-filter"><input type="checkbox" checked={dirtyOnly} onChange={(event) => setDirtyOnly(event.target.checked)} />仅看未发布修改</label>
    </div>
    {loading ? <TableSkeleton /> : placements.length === 0 ? <section className="inline-empty"><SlidersHorizontal size={24} /><h2>还没有广告位</h2><p>广告位定义应用内稳定的广告展示位置。</p><Button variant="primary" icon={<Plus size={16} />} onClick={onCreate}>新建广告位</Button></section> : <section className="table-panel entity-table-panel">
      <table className="entity-table"><thead><tr><th>广告位 key</th><th>类型</th><th>启用状态</th><th>频控策略</th><th>加载超时</th><th>绑定完整度</th><th>未发布修改</th><th><span className="sr-only">打开详情</span></th></tr></thead>
        <tbody>{rows.map((placement) => <PlacementRow key={placement.entity_id} placement={placement} bindings={bindings} environmentCount={environmentCount} diagnostics={diagnosticsForEntity(diagnostics, placement)} onOpen={onOpen} onDelete={onDelete} />)}</tbody>
      </table>
      {rows.length === 0 ? <div className="table-no-results">没有符合当前筛选条件的广告位。</div> : <footer className="table-footer">显示 {rows.length} / {placements.length} 个广告位{draft?.dirty ? <span>当前环境有未发布修改</span> : null}</footer>}
    </section>}
  </>;
}

function PlacementRow({ placement, bindings, environmentCount, diagnostics, onOpen, onDelete }: { placement: EntityView; bindings: BindingLoad; environmentCount: number; diagnostics: Diagnostic[]; onOpen: (id: string) => void; onDelete: (entity: EntityView, entityType: "placement") => void }) {
  const fields = placement.effective.value.fields;
  const key = String(fields.key ?? placement.entity_id);
  const configured = Object.values(bindings).flat().filter((binding) => binding.effective.value.fields.placement_id === placement.entity_id && binding.effective.value.fields.status === "configured" && binding.effective.value.fields.unit_id_ref).length;
  return <tr onClick={() => onOpen(placement.entity_id)}>
    <td><code>{key}</code><DiagnosticAnchor diagnostics={diagnostics} /></td><td>{adTypeLabel(fields.ad_type)}</td><td><StatusChip enabled={Boolean(fields.enabled)} /></td><td><code>{String(fields.frequency_policy_id ?? "-")}</code></td><td>{Number(fields.load_timeout_ms ?? 0)} ms</td><td>{configured}/{environmentCount * 2}</td><td>{isEntityDirty(placement) ? <span className="dirty-chip">未发布修改</span> : <span className="muted-cell">-</span>}</td><td className="row-actions"><button className="icon-button row-open" aria-label={`编辑 ${key}`} onClick={(event) => { event.stopPropagation(); onOpen(placement.entity_id); }}><ChevronRight size={18} /></button><button className="icon-button row-delete" aria-label={`删除 ${key}`} onClick={(event) => { event.stopPropagation(); onDelete(placement, "placement"); }}><Trash2 size={16} /></button></td>
  </tr>;
}

function FrequencyTable({ policies, diagnostics, loading, onOpen, onDelete }: { policies: EntityView[]; diagnostics: Diagnostic[]; loading: boolean; onOpen: (policy: EntityView) => void; onDelete: (entity: EntityView, entityType: "frequency_policy") => void }) {
  return loading ? <TableSkeleton /> : <section className="table-panel entity-table-panel"><table className="entity-table frequency-table"><thead><tr><th>频控策略</th><th>冷却时间</th><th>统计周期</th><th>周期内上限</th><th>覆盖位置</th><th><span className="sr-only">操作</span></th></tr></thead><tbody>{policies.map((policy) => { const fields = policy.effective.value.fields; return <tr key={policy.entity_id} onClick={() => onOpen(policy)}><td><strong><code>{policy.entity_id}</code></strong><DiagnosticAnchor diagnostics={diagnosticsForEntity(diagnostics, policy)} /></td><td>{formatMilliseconds(fields.cooldown_ms)}</td><td>{formatMilliseconds(fields.interval_ms)}</td><td>{String(fields.max_count ?? "-")} 次</td><td>{arrayValue(fields.positions).join("、") || "-"}</td><td className="row-actions"><button className="icon-button row-open" aria-label={`编辑频控策略 ${policy.entity_id}`} onClick={(event) => { event.stopPropagation(); onOpen(policy); }}><ChevronRight size={18} /></button><button className="icon-button row-delete" aria-label={`删除频控策略 ${policy.entity_id}`} onClick={(event) => { event.stopPropagation(); onDelete(policy, "frequency_policy"); }}><Trash2 size={16} /></button></td></tr>; })}</tbody></table>{policies.length === 0 ? <div className="table-no-results">还没有频控策略。</div> : <footer className="table-footer">共 {policies.length} 个频控策略</footer>}</section>;
}

function FeatureSwitchTable({ switches, diagnostics, environment, revision, draft, loading, onSaved }: { switches: EntityView[]; diagnostics: Diagnostic[]; environment: Environment; revision: number; draft: DraftView | null; loading: boolean; onSaved: () => void }) {
  const [saving, setSaving] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const toggle = async (item: EntityView) => {
    setSaving(item.entity_id); setError(null);
    const fields = item.effective.value.fields;
    try {
      await replaceDraftEntity(environment.id, "feature_switch", item.entity_id, revision, { expected_source_revision: draft?.source_revision ?? item.source_revision, write_scope: "baseline", entity: { id: item.entity_id, fields: { ...fields, default_value: !Boolean(fields.default_value) } } });
      onSaved();
    } catch (cause) { setError(cause instanceof ConflowAPIError ? cause.message : "保存开关失败，请重试。"); } finally { setSaving(null); }
  };
  return loading ? <TableSkeleton /> : <section className="switch-list table-panel"><header><strong>{switches.length} 个功能开关</strong><span>默认值为通用值，保存后才会进入未发布修改。</span></header>{switches.map((item) => { const fields = item.effective.value.fields; const key = String(fields.key ?? item.entity_id); return <div className={`switch-row switch-row--${String(fields.risk_level ?? "low")}`} key={item.entity_id}><div><strong>{switchName(key)}</strong><code>{key}</code><DiagnosticAnchor diagnostics={diagnosticsForEntity(diagnostics, item)} /></div><div className="switch-row-meta"><RiskTag level={String(fields.risk_level ?? "low")} /><span>回滚：{rollbackLabel(String(fields.rollback_method ?? ""))}</span><button className={fields.default_value ? "switch-control switch-control--on" : "switch-control"} type="button" role="switch" aria-label={`切换 ${key}`} aria-checked={Boolean(fields.default_value)} disabled={saving === item.entity_id} onClick={() => void toggle(item)}><span /></button></div></div>; })}{error ? <p className="binding-error switch-error" role="alert">{error}</p> : null}</section>;
}

function BindingOverview({ placements, bindings, diagnostics, environments, loading, onOpen }: { placements: EntityView[]; bindings: BindingLoad; diagnostics: Diagnostic[]; environments: Environment[]; loading: boolean; onOpen: (id: string) => void }) {
  return loading ? <TableSkeleton /> : <section className="table-panel binding-overview"><table><thead><tr><th>广告位</th>{environments.flatMap((environment) => [<th key={`${environment.id}-ios`}>{environment.name} iOS</th>, <th key={`${environment.id}-android`}>{environment.name} Android</th>])}</tr></thead><tbody>{placements.map((placement) => <tr key={placement.entity_id} onClick={() => onOpen(placement.entity_id)}><td><strong>{String(placement.effective.value.fields.key ?? placement.entity_id)}</strong><code>{placement.entity_id}</code><DiagnosticAnchor diagnostics={diagnosticsForEntity(diagnostics, placement)} /></td>{environments.flatMap((environment) => (["ios", "android"] as const).map((platform) => { const binding = bindings[environment.id]?.find((item) => item.effective.value.fields.placement_id === placement.entity_id && item.effective.value.fields.platform === platform); const value = binding?.effective.value.fields.unit_id_ref; return <td key={`${environment.id}-${platform}`} className={value ? "binding-overview-cell" : "binding-overview-cell binding-overview-cell--missing"}><code>{String(value ?? "未绑定")}</code>{binding ? <DiagnosticAnchor diagnostics={diagnosticsForEntity(diagnostics, binding)} /> : null}</td>; }))}</tr>)}</tbody></table><footer className="table-footer">点击广告位进入详情中的环境绑定区。</footer></section>;
}

function FrequencyDrawer({ policy, environment, revision, draft, diagnostics, onClose, onSaved, onDelete, onOpenReference }: { policy: EntityView | null; environment: Environment; revision: number; draft: DraftView | null; diagnostics: Diagnostic[]; onClose: () => void; onSaved: () => void; onDelete: (entity: EntityView) => void; onOpenReference: (reference: EntityReference) => void }) {
  const [fields, setFields] = useState<Record<string, unknown>>({});
  const [references, setReferences] = useState<EntityReference[]>([]);
  const [loadingReferences, setLoadingReferences] = useState(false);
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [systemError, setSystemError] = useState<string | null>(null);
  const [conflict, setConflict] = useState<EntityConflict | null>(null);
  useEffect(() => {
    if (!policy) return;
    setFields(policy.effective.value.fields); setReferences([]); setErrors({}); setSystemError(null); setLoadingReferences(true);
    const controller = new AbortController();
    void getDraftEntityReferences(environment.id, "frequency_policy", policy.entity_id, controller.signal).then((response) => setReferences(response.data.referenced_by)).catch((cause) => { if (!(cause instanceof DOMException && cause.name === "AbortError")) setSystemError(cause instanceof ConflowAPIError ? cause.message : "无法载入引用广告位。"); }).finally(() => { if (!controller.signal.aborted) setLoadingReferences(false); });
    return () => controller.abort();
  }, [environment.id, policy]);
  if (!policy) return null;
  const update = (name: string, value: unknown) => setFields((current) => ({ ...current, [name]: value }));
  const save = async () => {
    setSaving(true); setErrors({}); setSystemError(null);
    const entity = { id: policy.entity_id, fields };
    try { await replaceDraftEntity(environment.id, "frequency_policy", policy.entity_id, revision, { expected_source_revision: draft?.source_revision ?? policy.source_revision, write_scope: "baseline", entity }); onSaved(); }
    catch (cause) {
      if (cause instanceof ConflowAPIError && (cause.code === "revision_mismatch" || cause.code === "source_revision_mismatch") && cause.currentState && isDraftView(cause.currentState)) setConflict({ local: entity, state: cause.currentState, revision: cause.currentRevision ?? revision, entityType: "frequency_policy" });
      else if (cause instanceof ConflowAPIError && cause.code === "validation_failed") setErrors(errorsForEntity(cause.details ?? [], policy.entity_ref, policy.entity_id));
      else setSystemError(cause instanceof ConflowAPIError ? cause.message : "保存频控策略失败，请重试。");
    } finally { setSaving(false); }
  };
  return <aside className="frequency-drawer" role="dialog" aria-modal="true" aria-label={`编辑频控策略 ${policy.entity_id}`}><header><div><h2>编辑频控策略</h2><code>{policy.entity_id}</code></div><button className="icon-button" aria-label="关闭频控策略编辑" onClick={onClose}><X size={18} /></button></header><div className="frequency-drawer-body"><p className="drawer-scope"><Link2 size={15} />通用值，改动会影响引用此策略的广告位。</p><EntityDiagnostics diagnostics={diagnosticsForEntity(diagnostics, policy)} title="此频控策略的校验问题" /><div className="frequency-fields"><NumberField label="冷却时间（毫秒）" name="cooldown_ms" value={fields.cooldown_ms} error={errors.cooldown_ms} onChange={update} /><NumberField label="统计周期（毫秒）" name="interval_ms" value={fields.interval_ms} error={errors.interval_ms} onChange={update} /><NumberField label="周期内最大次数" name="max_count" value={fields.max_count} error={errors.max_count} onChange={update} /><NumberField label="起始偏移次数" name="shift_count" value={fields.shift_count} error={errors.shift_count} onChange={update} /><label className={errors.positions ? "form-field form-field--error" : "form-field"}><span>适用位置</span><input aria-label="适用位置" value={arrayValue(fields.positions).join(", ")} onChange={(event) => update("positions", event.target.value.split(",").map((item) => item.trim()).filter(Boolean))} /><small>通用值 · 以逗号分隔</small>{errors.positions ? <span className="field-error">{errors.positions}</span> : null}</label></div>{systemError ? <p className="binding-error" role="alert">{systemError}</p> : null}<section className="affected-entities"><header><h3>引用此策略的广告位</h3><span>{loadingReferences ? "载入中" : `${references.length} 个`}</span></header>{!loadingReferences && references.length === 0 ? <p>未被引用</p> : references.slice(0, 6).map((reference) => <button key={reference.entity_ref} onClick={() => onOpenReference(reference)}><span>{reference.entity_id}</span><ChevronRight size={16} /></button>)}{references.length > 6 ? <p className="more-references">还有 {references.length - 6} 个广告位</p> : null}</section></div><footer><Button icon={<Trash2 size={16} />} onClick={() => onDelete(policy)}>删除策略</Button><Button onClick={onClose}>取消</Button><Button variant="primary" icon={<Save size={16} />} disabled={saving} onClick={() => void save()}>{saving ? "正在保存" : "保存策略"}</Button></footer><EntityConflictDialog conflict={conflict} onClose={() => setConflict(null)} onReload={() => { setConflict(null); onSaved(); }} /></aside>;
}

function NumberField({ label, name, value, error, onChange }: { label: string; name: string; value: unknown; error?: string; onChange: (name: string, value: number) => void }) { return <label className={error ? "form-field form-field--error" : "form-field"}><span>{label}</span><input aria-label={label} type="number" value={String(value ?? "")} onChange={(event) => onChange(name, Number(event.target.value))} /><small>通用值</small>{error ? <span className="field-error">{error}</span> : null}</label>; }

function DeleteEntityDialog({ target, environment, revision, draft, onClose, onDeleted, onBlocked }: { target: DeleteTarget | null; environment: Environment; revision: number; draft: DraftView | null; onClose: () => void; onDeleted: () => void; onBlocked: (target: DeleteTarget, references: EntityReference[]) => void }) {
  const [deleting, setDeleting] = useState(false); const [error, setError] = useState<string | null>(null);
  useEffect(() => { setDeleting(false); setError(null); }, [target]);
  const remove = async () => { if (!target) return; setDeleting(true); setError(null); try { await deleteDraftEntity(environment.id, target.entityType, target.entity.entity_id, revision, { expected_source_revision: draft?.source_revision ?? target.entity.source_revision, write_scope: "baseline" }); onDeleted(); } catch (cause) { if (cause instanceof ConflowAPIError && cause.code === "entity_referenced") { const references = (cause as ConflowAPIError & { references?: EntityReference[] }).references ?? []; onBlocked(target, references); } else setError(cause instanceof ConflowAPIError ? cause.message : "删除失败，请重试。"); } finally { setDeleting(false); } };
  return <Modal open={target !== null} onOpenChange={(open) => { if (!open) onClose(); }} title={`删除${target?.entityType === "frequency_policy" ? "频控策略" : "广告位"}`} description="删除后会作为未发布修改保存，仍需校验与发布。"><div className="delete-dialog-content"><p>确定删除 <code>{target?.entity.entity_id}</code> 吗？</p>{error ? <p className="binding-error" role="alert">{error}</p> : null}</div><footer className="dialog-actions"><Button onClick={onClose}>取消</Button><Button variant="danger" icon={<Trash2 size={16} />} disabled={deleting} onClick={() => void remove()}>{deleting ? "正在删除" : "确认删除"}</Button></footer></Modal>;
}

function ReferencedDeleteDialog({ blocked, onClose, onOpenReference }: { blocked: { target: DeleteTarget; references: EntityReference[] } | null; onClose: () => void; onOpenReference: (reference: EntityReference) => void }) { return <Modal open={blocked !== null} onOpenChange={(open) => { if (!open) onClose(); }} title={`无法删除 ${blocked?.target.entity.entity_id ?? ""}`} description={`此频控策略仍被 ${blocked?.references.length ?? 0} 个广告位引用。先迁移这些引用，才能删除策略。`}><div className="referenced-delete"><div className="danger-callout"><ShieldAlert size={20} /><p>存在引用时不允许继续删除。</p></div>{blocked?.references.slice(0, 5).map((reference) => <button key={reference.entity_ref} onClick={() => onOpenReference(reference)}><Link2 size={15} /><span>{reference.entity_id}</span><ChevronRight size={16} /></button>)}{(blocked?.references.length ?? 0) > 5 ? <p>还有 {(blocked?.references.length ?? 0) - 5} 个广告位</p> : null}</div><footer className="dialog-actions"><Button onClick={onClose}>返回</Button></footer></Modal>; }

function RiskTag({ level }: { level: string }) { const labels: Record<string, string> = { low: "低风险", medium: "中风险", high: "高风险" }; return <span className={`risk-tag risk-tag--${level}`}>{labels[level] ?? level}</span>; }
function rollbackLabel(value: string) { return ({ disable: "关闭开关", disable_and_publish: "关闭后发布", disable_and_regenerate_plan: "关闭后重新生成发布计划", disable_and_clear_memory_cache: "关闭并清理内存缓存", remove_legacy_override_and_confirm_production: "移除旧覆盖并确认 Production", enable_and_publish: "启用后发布" } as Record<string, string>)[value] ?? (value || "按运行手册"); }
function switchName(key: string) { return ({ use_amazon_bidding: "启用 Amazon Bidding", enable_native_preload: "启用原生广告预加载", show_subscription_offer: "展示订阅推荐", enable_ad_debug_overlay: "启用广告调试浮层", defer_app_open_until_consent: "同意隐私后展示开屏广告", ads_enabled_legacy: "启用旧版广告开关" } as Record<string, string>)[key] ?? key; }
function arrayValue(value: unknown) { return Array.isArray(value) ? value.map(String) : []; }
function formatMilliseconds(value: unknown) { const number = Number(value ?? 0); return number >= 60000 && number % 60000 === 0 ? `${number / 60000} 分钟` : `${number / 1000} 秒`; }

function PlacementDetail({ environment, environments, revision, schema, validation, placementID, focusBindings, onBack, onSaved }: {
  environment: Environment;
  environments: Environment[];
  revision: number;
  schema: PackSchema | null;
  validation: ValidationResult | null;
  placementID?: string;
  focusBindings: boolean;
  onBack: () => void;
  onSaved: (revision: number, dirty: boolean) => void;
}) {
  const [placement, setPlacement] = useState<EntityView | null>(null);
  const [draft, setDraft] = useState<DraftView | null>(null);
  const [policies, setPolicies] = useState<EntityView[]>([]);
  const [bindings, setBindings] = useState<BindingLoad>({});
  const [fields, setFields] = useState<Record<string, unknown>>({});
  const [newID, setNewID] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [systemError, setSystemError] = useState<{ code: string; requestId?: string } | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [conflict, setConflict] = useState<EntityConflict | null>(null);
  const placementSchema = schema?.entities.find((item) => item.name === "placement");
  const id = placementID ?? newID;

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true); setSystemError(null);
    try {
      const [nextDraft, nextPolicies, nextBindings, nextPlacement] = await Promise.all([
        getDraft(environment.id, signal),
        listDraftEntities(environment.id, "frequency_policy", signal),
        Promise.all(environments.map(async (item) => [item.id, (await listDraftEntities(item.id, "unit_binding", signal)).data] as const)),
        placementID ? getDraftEntity(environment.id, "placement", placementID, signal) : Promise.resolve(null),
      ]);
      const initial = nextPlacement?.data.effective.value.fields ?? defaultsFor(placementSchema?.fields ?? []);
      setDraft(nextDraft.data); setPolicies(nextPolicies.data); setBindings(Object.fromEntries(nextBindings)); setPlacement(nextPlacement?.data ?? null); setFields(initial);
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === "AbortError") return;
      setSystemError(toRequestError(cause));
    } finally { if (!signal?.aborted) setLoading(false); }
  }, [environment.id, environments, placementID, placementSchema?.fields]);

  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort(); }, [load]);
  useEffect(() => { if (!loading && focusBindings) document.getElementById("environment-bindings")?.scrollIntoView({ block: "start" }); }, [focusBindings, loading]);

  const save = async () => {
    if (!draft || !placementSchema) return;
    setSaving(true); setSystemError(null); setFieldErrors({});
    const record: EntityRecord = { id, fields };
    try {
      const response = placementID
        ? await replaceDraftEntity(environment.id, "placement", placementID, revision, { expected_source_revision: draft.source_revision, write_scope: "baseline", entity: record })
        : await createDraftEntity(environment.id, revision, { expected_source_revision: draft.source_revision, write_scope: "baseline", entity_type: "placement", entity: record });
      setPlacement(response.data); setFields(response.data.effective.value.fields); onSaved(response.meta.revision, true);
      if (!placementID) onBack();
    } catch (cause) {
      if (cause instanceof ConflowAPIError) {
        if ((cause.code === "revision_mismatch" || cause.code === "source_revision_mismatch") && cause.currentState && isDraftView(cause.currentState)) {
          setConflict({ local: record, state: cause.currentState, revision: cause.currentRevision ?? revision });
        } else if (cause.code === "validation_failed") setFieldErrors(errorsForEntity(cause.details ?? [], placement?.entity_ref, placementID));
        else setSystemError(toRequestError(cause));
      } else setSystemError(toRequestError(cause));
    } finally { setSaving(false); }
  };

  const updateField = (name: string, value: unknown) => setFields((current) => ({ ...current, [name]: value }));
  const title = placementID ? String(fields.key ?? placementID) : "新建广告位";
  const allFields = (placementSchema?.fields ?? []).slice().sort((a, b) => a.ui.order - b.ui.order);
  const groups = [["基础信息", ["key", "ad_type"]], ["加载行为", ["enabled", "network_mode", "load_timeout_ms", "cache_policy", "fallback_behavior"]], ["频控", ["frequency_policy_id"]]] as const;
  const diagnostics = placement ? diagnosticsForEntity(validation?.diagnostics ?? [], placement) : [];

  return <main className="page-container placement-detail">
    <header className="detail-heading"><div className="detail-heading-title"><button className="icon-button detail-back" aria-label="返回配置列表" onClick={onBack}><ArrowLeft size={19} /></button><div><h1>{title}</h1><p><code>{placementID || "创建后生成稳定 ID"}</code>{placementID ? ` · ${adTypeLabel(fields.ad_type)}` : ""}</p></div></div><Button variant="primary" icon={<Save size={16} />} disabled={loading || saving} onClick={() => void save()}>{saving ? "正在保存" : fieldErrorCount(fieldErrors) ? `保存修改（${fieldErrorCount(fieldErrors)} 项错误）` : "保存修改"}</Button></header>
    {systemError ? <RequestError {...systemError} onDismiss={() => setSystemError(null)} /> : null}
    {loading ? <DetailSkeleton /> : <div className="detail-layout"><div className="detail-main">
      <EntityDiagnostics diagnostics={diagnostics} title="此广告位的校验问题" />
      {groups.map(([group, names]) => { const groupFields = allFields.filter((field) => (names as readonly string[]).includes(field.name)); return groupFields.length ? <section className="editor-section" key={group}><h2>{group}</h2><div className="field-grid">{group === "基础信息" && !placementID ? <label className={fieldErrors.id ? "form-field form-field--error" : "form-field"} htmlFor="placement-id"><span>稳定 ID</span><input id="placement-id" aria-label="稳定 ID" value={newID} onChange={(event) => setNewID(event.target.value)} placeholder="例如 ad_interstitial_011" /><small>创建后不可修改</small>{fieldErrors.id ? <span className="field-error" role="alert">{fieldErrors.id}</span> : null}</label> : null}{group === "基础信息" && placementID ? <div className="form-field immutable-field"><span>稳定 ID</span><code>{placementID}</code><small>所有环境一致，不可修改</small></div> : null}{groupFields.map((field) => <PlacementField key={field.name} field={field} value={fields[field.name]} readOnly={Boolean(placementID && (field.name === "key" || field.name === "ad_type"))} policies={policies} caption={field.name === "key" && placementID ? "所有环境一致，不可修改" : fieldCaption(draft, placementID, field.name)} error={fieldErrors[field.name]} onChange={updateField} />)}</div></section> : null; })}
      <BindingMatrix environments={environments} bindings={bindings} diagnostics={validation?.diagnostics ?? []} placementID={placementID} revision={revision} sourceRevision={draft?.source_revision ?? ""} onSaved={(nextRevision) => { onSaved(nextRevision, true); void load(); }} />
    </div><aside className="detail-sidebar"><section className="change-summary"><h2>修改摘要</h2><dl><div><dt>当前环境</dt><dd>{environment.name}</dd></div><div><dt>字段错误</dt><dd>{fieldErrorCount(fieldErrors) ? `${fieldErrorCount(fieldErrors)} 项` : "无"}</dd></div></dl></section><details className="advanced-info"><summary>高级信息与源映射</summary><code>{placement?.entity_ref ?? "将在创建后生成"}</code></details></aside></div>}
    <EntityConflictDialog conflict={conflict} onClose={() => setConflict(null)} onReload={() => { setConflict(null); void load(); }} />
  </main>;
}

function PlacementField({ field, value, readOnly, policies, caption, error, onChange }: { field: FieldSchema; value: unknown; readOnly: boolean; policies: EntityView[]; caption: string; error?: string; onChange: (name: string, value: unknown) => void }) {
  const id = `placement-${field.name}`;
  const options = field.type === "reference" ? policies.map((policy) => policy.entity_id) : field.validation.enum.map(String);
  return <label className={error ? "form-field form-field--error" : "form-field"} htmlFor={id}><span>{field.ui.label}</span>
    {field.type === "boolean" ? <button id={id} type="button" className={value ? "switch-control switch-control--on" : "switch-control"} role="switch" aria-checked={Boolean(value)} disabled={readOnly} onClick={() => onChange(field.name, !value)}><span aria-hidden="true" /></button>
      : field.type === "reference" || field.validation.enum.length > 0 ? <select id={id} aria-label={field.ui.label} value={String(value ?? "")} disabled={readOnly} onChange={(event) => onChange(field.name, event.target.value)}>{options.map((option) => <option key={option} value={option}>{field.type === "reference" ? option : enumLabel(field.name, option)}</option>)}</select>
        : <input id={id} aria-label={field.ui.label} type={field.type === "integer" || field.type === "number" ? "number" : "text"} value={String(value ?? "")} readOnly={readOnly} onChange={(event) => onChange(field.name, field.type === "integer" || field.type === "number" ? Number(event.target.value) : event.target.value)} />}
    <small>{caption}{field.ui.description ? ` · ${field.ui.description}` : ""}</small>{error ? <span className="field-error" role="alert">{error}</span> : null}
  </label>;
}

function BindingMatrix({ environments, bindings, diagnostics, placementID, revision, sourceRevision, onSaved }: { environments: Environment[]; bindings: BindingLoad; diagnostics: Diagnostic[]; placementID?: string; revision: number; sourceRevision: string; onSaved: (revision: number) => void }) {
  const [editing, setEditing] = useState<string | null>(null);
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const rowID = (environmentID: string, platform: "ios" | "android") => `ub_${environmentID}_${platform}_${placementID}`;
  const bindingFor = (environmentID: string, platform: "ios" | "android") => bindings[environmentID]?.find((item) => item.entity_id === rowID(environmentID, platform));
  const openEdit = (environmentID: string, platform: "ios" | "android") => { const binding = bindingFor(environmentID, platform); setEditing(`${environmentID}:${platform}`); setValue(String(binding?.effective.value.fields.unit_id_ref ?? "")); setError(null); };
  const save = async () => {
    if (!editing || !placementID) return;
    const [environmentID, platform] = editing.split(":") as [string, "ios" | "android"];
    setSaving(true); setError(null);
    const entity: EntityRecord = { id: rowID(environmentID, platform), fields: { placement_id: placementID, environment_id: environmentID, platform, unit_id_ref: value, status: value ? "configured" : "missing" } };
    try {
      const current = bindingFor(environmentID, platform);
      const response = current
        ? await replaceDraftEntity(environmentID, "unit_binding", entity.id, revision, { expected_source_revision: sourceRevision, write_scope: "environment_override", entity })
        : await createDraftEntity(environmentID, revision, { expected_source_revision: sourceRevision, write_scope: "environment_override", entity_type: "unit_binding", entity });
      setEditing(null); onSaved(response.meta.revision);
    } catch (cause) { setError(cause instanceof ConflowAPIError ? cause.message : "保存绑定失败，请重试。"); } finally { setSaving(false); }
  };
  const bindingDiagnostics = Object.values(bindings).flat().flatMap((binding) => diagnosticsForEntity(diagnostics, binding));
  return <section id="environment-bindings" className="editor-section binding-section"><header><div><h2>环境绑定</h2><p>按环境维护 iOS 和 Android 的广告单元。</p></div></header><EntityDiagnostics diagnostics={bindingDiagnostics} title="此广告位环境绑定的校验问题" />{!placementID ? <p className="muted-copy">保存广告位后即可设置环境绑定。</p> : <div className="binding-matrix"><div className="binding-head"><span>环境</span><span>iOS</span><span>Android</span></div>{environments.map((environment) => <div className="binding-row" key={environment.id}><strong>{environment.name}</strong>{(["ios", "android"] as const).map((platform) => { const binding = bindingFor(environment.id, platform); const missing = environment.kind === "production" && !binding?.effective.value.fields.unit_id_ref; const active = editing === `${environment.id}:${platform}`; const bindingIssues = binding ? diagnosticsForEntity(diagnostics, binding) : []; return <div className={missing ? "binding-cell binding-cell--warning" : "binding-cell"} key={platform}>{active ? <div className="binding-edit"><input aria-label={`${environment.name} ${platform} 单元 ID`} value={value} onChange={(event) => setValue(event.target.value)} /><Button variant="primary" disabled={saving} onClick={() => void save()}>保存</Button><button className="link-button" onClick={() => setEditing(null)}>取消</button></div> : <button className="binding-value" aria-label={`编辑 ${environment.name} ${platform} 绑定`} onClick={() => openEdit(environment.id, platform)}><code>{String(binding?.effective.value.fields.unit_id_ref ?? "未绑定")}</code><DiagnosticAnchor diagnostics={bindingIssues} />{missing ? <span>Production 缺少绑定</span> : null}</button>}</div>; })}</div>)}</div>}{error ? <p className="binding-error" role="alert">{error}</p> : null}</section>;
}

function EntityConflictDialog({ conflict, onClose, onReload }: { conflict: EntityConflict | null; onClose: () => void; onReload: () => void }) {
  const current = conflict ? findEntity(conflict.state, conflict.local.id, conflict.entityType ?? "placement") : undefined;
  const label = conflict?.entityType === "frequency_policy" ? "频控策略" : "广告位";
  return <Modal open={conflict !== null} onOpenChange={(open) => { if (!open) onClose(); }} title={`${label}已被其他操作修改`} description={`服务端当前版本 ${conflict?.revision ?? "未知"}。重新加载前不会覆盖服务端当前值。`}><div className="conflict-icon"><CircleAlert size={18} /></div><div className="conflict-grid"><section><span>我的修改</span><code>{JSON.stringify(conflict?.local.fields ?? {}, null, 2)}</code></section><section><span>服务端当前值</span><code>{JSON.stringify(current?.fields ?? `${label}已删除`, null, 2)}</code></section></div><footer className="dialog-actions"><Button onClick={onClose}>保留我的输入</Button><Button variant="primary" onClick={onReload}>重新加载当前值</Button></footer></Modal>;
}

function TableSkeleton() { return <section className="table-panel entity-table-panel"><div className="table-skeleton"><LoaderCircle className="spin" /><span>正在载入广告位</span></div></section>; }
function DetailSkeleton() { return <div className="detail-skeleton"><LoaderCircle className="spin" /><span>正在载入广告位详情</span></div>; }
function StatusChip({ enabled }: { enabled: boolean }) { return <span className={enabled ? "status-chip status-chip--enabled" : "status-chip status-chip--disabled"}><i />{enabled ? "已启用" : "已停用"}</span>; }
function isEntityDirty(entity: EntityView) { return entity.origin === "draft_baseline" || entity.origin === "draft_environment_override" || entity.draft.present; }
function adTypeLabel(value: unknown) { return ({ app_open: "App Open", interstitial: "插屏", native: "原生" } as Record<string, string>)[String(value)] ?? String(value ?? "-"); }
function enumLabel(name: string, value: string) { if (name === "ad_type") return adTypeLabel(value); return value; }
function fieldErrorCount(errors: Record<string, string>) { return Object.keys(errors).length; }
function defaultsFor(fields: FieldSchema[]) { return Object.fromEntries(fields.map((field) => [field.name, field.default])); }
function diagnosticsForEntity(diagnostics: Diagnostic[], entity: EntityView) { return diagnostics.filter((diagnostic) => diagnostic.entity_ref === entity.entity_ref); }
function diagnosticCategory(diagnostic: Diagnostic): DiagnosticCategory { return diagnostic.severity === "blocking" || diagnostic.severity === "error" ? "blocking" : diagnostic.severity; }
function highestDiagnosticCategory(diagnostics: Diagnostic[]): DiagnosticCategory { return diagnostics.some((diagnostic) => diagnosticCategory(diagnostic) === "blocking") ? "blocking" : diagnostics.some((diagnostic) => diagnosticCategory(diagnostic) === "warning") ? "warning" : "info"; }
function diagnosticCategoryLabel(category: DiagnosticCategory) { return ({ blocking: "阻断", warning: "警告", info: "建议" } as const)[category]; }
function diagnosticCounts(diagnostics: Diagnostic[]): Record<DiagnosticCategory, number> { return diagnostics.reduce<Record<DiagnosticCategory, number>>((counts, diagnostic) => { counts[diagnosticCategory(diagnostic)] += 1; return counts; }, { blocking: 0, warning: 0, info: 0 }); }
function formatValidatedAt(value: string) { return value.replace("T", " ").replace(/\.\d+Z$/, " UTC").replace("Z", " UTC"); }
function findEntity(state: DraftView, id: string, entityType: string): EntityRecord | undefined { const key = ({ placement: "placements", frequency_policy: "frequency_policies", feature_switch: "feature_switches" } as Record<string, string>)[entityType] ?? `${entityType}s`; const collection = state.effective[key]; return Array.isArray(collection) ? collection.find((item): item is EntityRecord => Boolean(item && typeof item === "object" && "id" in item && (item as EntityRecord).id === id)) : undefined; }
function isDraftView(value: unknown): value is DraftView { return Boolean(value && typeof value === "object" && "environment_id" in value && "effective" in value); }
function fieldCaption(draft: DraftView | null, entityID: string | undefined, field: string) {
  const states = draft?.field_states ?? [];
  const state = states.find((item) => item.path.endsWith(`/${entityID}/${field}`) || item.path.endsWith(`/${field}`)) ?? states.find((item) => item.path === "/placements");
  if (!state) return "通用值";
  const environment = state.origin === "environment_override" || state.origin === "draft_environment_override";
  const dirty = state.origin.startsWith("draft_");
  return `${environment ? "本环境专属值" : "通用值"}${dirty ? " · 未发布修改" : ""}`;
}
function errorsForEntity(details: DraftStructuralErrorDetail[], entityRef?: string, entityID?: string) { return Object.fromEntries(details.filter((detail) => !detail.entity_ref || detail.entity_ref === entityRef || detail.entity_ref.endsWith(`:${entityID}`)).map((detail) => [detail.path.split("/").filter(Boolean).pop() ?? "form", detail.message])); }
function toRequestError(cause: unknown) { if (cause instanceof ConflowAPIError) return { code: cause.code, requestId: cause.requestId }; if (cause instanceof ConflowNetworkError) return { code: "network_unavailable" }; return { code: "internal_error" }; }
