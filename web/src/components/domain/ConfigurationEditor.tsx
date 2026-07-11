import { ArrowLeft, ChevronRight, CircleAlert, LoaderCircle, Plus, Save, Search, SlidersHorizontal } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ConflowAPIError,
  ConflowNetworkError,
  createDraftEntity,
  getDraft,
  getDraftEntity,
  getPackSchema,
  listDraftEntities,
  replaceDraftEntity,
  type DraftStructuralErrorDetail,
  type DraftView,
  type EntityRecord,
  type EntityView,
  type Environment,
  type FieldSchema,
  type PackSchema,
} from "../../api/client";
import { Button } from "../ui/Button";
import { Modal } from "../ui/Dialog";
import { RequestError } from "../ui/StateViews";

type EditorRoute = { mode: "list" } | { mode: "detail"; id?: string };
type EntityConflict = { local: EntityRecord; state: DraftView; revision: number };
type BindingLoad = Record<string, EntityView[]>;

export function ConfigurationEditor({ environment, environments, revision, packRef, onRevision }: {
  environment: Environment;
  environments: Environment[];
  revision: number;
  packRef: string;
  onRevision: (revision: number, dirty: boolean) => void;
}) {
  const [route, setRoute] = useState<EditorRoute>({ mode: "list" });
  const [schema, setSchema] = useState<PackSchema | null>(null);
  const [placements, setPlacements] = useState<EntityView[]>([]);
  const [draft, setDraft] = useState<DraftView | null>(null);
  const [bindings, setBindings] = useState<BindingLoad>({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<{ code: string; requestId?: string } | null>(null);

  const loadList = useCallback(async (signal?: AbortSignal) => {
    setLoading(true); setError(null);
    try {
      const [nextSchema, nextPlacements, nextDraft, nextBindings] = await Promise.all([
        getPackSchema(packRef, signal),
        listDraftEntities(environment.id, "placement", signal),
        getDraft(environment.id, signal),
        Promise.all(environments.map(async (item) => [item.id, (await listDraftEntities(item.id, "unit_binding", signal)).data] as const)),
      ]);
      setSchema(nextSchema.data); setPlacements(nextPlacements.data); setDraft(nextDraft.data); setBindings(Object.fromEntries(nextBindings));
      onRevision(nextDraft.meta.revision, nextDraft.data.dirty);
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === "AbortError") return;
      setError(toRequestError(cause));
    } finally { if (!signal?.aborted) setLoading(false); }
  }, [environment.id, environments, onRevision, packRef]);

  useEffect(() => { const controller = new AbortController(); void loadList(controller.signal); return () => controller.abort(); }, [loadList]);
  useEffect(() => { setRoute({ mode: "list" }); }, [environment.id]);

  if (route.mode === "detail") {
    return <PlacementDetail key={`${environment.id}:${route.id ?? "new"}`} environment={environment} environments={environments} revision={revision} schema={schema} placementID={route.id} onBack={() => { setRoute({ mode: "list" }); void loadList(); }} onSaved={(nextRevision, dirty) => { onRevision(nextRevision, dirty); void loadList(); }} />;
  }

  return <PlacementTable placements={placements} draft={draft} bindings={bindings} environmentCount={environments.length} loading={loading} error={error} onRetry={() => void loadList()} onOpen={(id) => setRoute({ mode: "detail", id })} onCreate={() => setRoute({ mode: "detail" })} />;
}

function PlacementTable({ placements, draft, bindings, environmentCount, loading, error, onRetry, onOpen, onCreate }: {
  placements: EntityView[];
  draft: DraftView | null;
  bindings: BindingLoad;
  environmentCount: number;
  loading: boolean;
  error: { code: string; requestId?: string } | null;
  onRetry: () => void;
  onOpen: (id: string) => void;
  onCreate: () => void;
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

  return <main className="page-container configuration-page">
    <header className="page-heading configuration-heading">
      <div><h1>配置</h1><p>按业务对象维护广告位配置。</p></div>
      <Button variant="primary" icon={<Plus size={17} />} onClick={onCreate}>新建广告位</Button>
    </header>
    <div className="entity-tabs" role="tablist" aria-label="配置对象">
      <button className="entity-tab entity-tab--active" role="tab" aria-selected="true">广告位</button>
      <button className="entity-tab" role="tab" disabled>频控策略</button>
      <button className="entity-tab" role="tab" disabled>功能开关</button>
      <button className="entity-tab" role="tab" disabled>环境绑定</button>
    </div>
    <div className="entity-toolbar">
      <label className="toolbar-select"><span>类型</span><select aria-label="按类型筛选" value={type} onChange={(event) => setType(event.target.value)}><option value="all">全部类型</option><option value="app_open">App Open</option><option value="interstitial">插屏</option><option value="native">原生</option></select></label>
      <label className="toolbar-search"><Search size={16} aria-hidden="true" /><span className="sr-only">搜索名称或 key</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索名称或 key" /></label>
      <label className="dirty-filter"><input type="checkbox" checked={dirtyOnly} onChange={(event) => setDirtyOnly(event.target.checked)} />仅看未发布修改</label>
    </div>
    {error ? <RequestError {...error} onDismiss={onRetry} /> : null}
    {loading ? <TableSkeleton /> : placements.length === 0 ? <section className="inline-empty"><SlidersHorizontal size={24} /><h2>还没有广告位</h2><p>广告位定义应用内稳定的广告展示位置。</p><Button variant="primary" icon={<Plus size={16} />} onClick={onCreate}>新建广告位</Button></section> : <section className="table-panel entity-table-panel">
      <table className="entity-table"><thead><tr><th>广告位 key</th><th>类型</th><th>启用状态</th><th>频控策略</th><th>加载超时</th><th>绑定完整度</th><th>未发布修改</th><th><span className="sr-only">打开详情</span></th></tr></thead>
        <tbody>{rows.map((placement) => <PlacementRow key={placement.entity_id} placement={placement} bindings={bindings} environmentCount={environmentCount} onOpen={onOpen} />)}</tbody>
      </table>
      {rows.length === 0 ? <div className="table-no-results">没有符合当前筛选条件的广告位。</div> : <footer className="table-footer">显示 {rows.length} / {placements.length} 个广告位{draft?.dirty ? <span>当前环境有未发布修改</span> : null}</footer>}
    </section>}
  </main>;
}

function PlacementRow({ placement, bindings, environmentCount, onOpen }: { placement: EntityView; bindings: BindingLoad; environmentCount: number; onOpen: (id: string) => void }) {
  const fields = placement.effective.value.fields;
  const key = String(fields.key ?? placement.entity_id);
  const configured = Object.values(bindings).flat().filter((binding) => binding.effective.value.fields.placement_id === placement.entity_id && binding.effective.value.fields.status === "configured" && binding.effective.value.fields.unit_id_ref).length;
  return <tr onClick={() => onOpen(placement.entity_id)}>
    <td><code>{key}</code></td><td>{adTypeLabel(fields.ad_type)}</td><td><StatusChip enabled={Boolean(fields.enabled)} /></td><td><code>{String(fields.frequency_policy_id ?? "-")}</code></td><td>{Number(fields.load_timeout_ms ?? 0)} ms</td><td>{configured}/{environmentCount * 2}</td><td>{isEntityDirty(placement) ? <span className="dirty-chip">未发布修改</span> : <span className="muted-cell">-</span>}</td><td><button className="icon-button row-open" aria-label={`编辑 ${key}`} onClick={(event) => { event.stopPropagation(); onOpen(placement.entity_id); }}><ChevronRight size={18} /></button></td>
  </tr>;
}

function PlacementDetail({ environment, environments, revision, schema, placementID, onBack, onSaved }: {
  environment: Environment;
  environments: Environment[];
  revision: number;
  schema: PackSchema | null;
  placementID?: string;
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

  return <main className="page-container placement-detail">
    <header className="detail-heading"><div className="detail-heading-title"><button className="icon-button detail-back" aria-label="返回配置列表" onClick={onBack}><ArrowLeft size={19} /></button><div><h1>{title}</h1><p><code>{placementID || "创建后生成稳定 ID"}</code>{placementID ? ` · ${adTypeLabel(fields.ad_type)}` : ""}</p></div></div><Button variant="primary" icon={<Save size={16} />} disabled={loading || saving} onClick={() => void save()}>{saving ? "正在保存" : fieldErrorCount(fieldErrors) ? `保存修改（${fieldErrorCount(fieldErrors)} 项错误）` : "保存修改"}</Button></header>
    {systemError ? <RequestError {...systemError} onDismiss={() => setSystemError(null)} /> : null}
    {loading ? <DetailSkeleton /> : <div className="detail-layout"><div className="detail-main">
      {groups.map(([group, names]) => { const groupFields = allFields.filter((field) => (names as readonly string[]).includes(field.name)); return groupFields.length ? <section className="editor-section" key={group}><h2>{group}</h2><div className="field-grid">{group === "基础信息" && !placementID ? <label className={fieldErrors.id ? "form-field form-field--error" : "form-field"} htmlFor="placement-id"><span>稳定 ID</span><input id="placement-id" aria-label="稳定 ID" value={newID} onChange={(event) => setNewID(event.target.value)} placeholder="例如 ad_interstitial_011" /><small>创建后不可修改</small>{fieldErrors.id ? <span className="field-error" role="alert">{fieldErrors.id}</span> : null}</label> : null}{groupFields.map((field) => <PlacementField key={field.name} field={field} value={fields[field.name]} readOnly={Boolean(placementID && (field.name === "key" || field.name === "ad_type"))} policies={policies} caption={fieldCaption(draft, placementID, field.name)} error={fieldErrors[field.name]} onChange={updateField} />)}</div></section> : null; })}
      <BindingMatrix environments={environments} bindings={bindings} placementID={placementID} revision={revision} sourceRevision={draft?.source_revision ?? ""} onSaved={(nextRevision) => { onSaved(nextRevision, true); void load(); }} />
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

function BindingMatrix({ environments, bindings, placementID, revision, sourceRevision, onSaved }: { environments: Environment[]; bindings: BindingLoad; placementID?: string; revision: number; sourceRevision: string; onSaved: (revision: number) => void }) {
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
  return <section className="editor-section binding-section"><header><div><h2>环境绑定</h2><p>按环境维护 iOS 和 Android 的广告单元。</p></div></header>{!placementID ? <p className="muted-copy">保存广告位后即可设置环境绑定。</p> : <div className="binding-matrix"><div className="binding-head"><span>环境</span><span>iOS</span><span>Android</span></div>{environments.map((environment) => <div className="binding-row" key={environment.id}><strong>{environment.name}</strong>{(["ios", "android"] as const).map((platform) => { const binding = bindingFor(environment.id, platform); const missing = environment.kind === "production" && !binding?.effective.value.fields.unit_id_ref; const active = editing === `${environment.id}:${platform}`; return <div className={missing ? "binding-cell binding-cell--warning" : "binding-cell"} key={platform}>{active ? <div className="binding-edit"><input aria-label={`${environment.name} ${platform} 单元 ID`} value={value} onChange={(event) => setValue(event.target.value)} /><Button variant="primary" disabled={saving} onClick={() => void save()}>保存</Button><button className="link-button" onClick={() => setEditing(null)}>取消</button></div> : <button className="binding-value" aria-label={`编辑 ${environment.name} ${platform} 绑定`} onClick={() => openEdit(environment.id, platform)}><code>{String(binding?.effective.value.fields.unit_id_ref ?? "未绑定")}</code>{missing ? <span>Production 缺少绑定</span> : null}</button>}</div>; })}</div>)}</div>}{error ? <p className="binding-error" role="alert">{error}</p> : null}</section>;
}

function EntityConflictDialog({ conflict, onClose, onReload }: { conflict: EntityConflict | null; onClose: () => void; onReload: () => void }) {
  const current = conflict ? findPlacement(conflict.state, conflict.local.id) : undefined;
  return <Modal open={conflict !== null} onOpenChange={(open) => { if (!open) onClose(); }} title="广告位已被其他操作修改" description={`服务端当前版本 ${conflict?.revision ?? "未知"}。重新加载前不会覆盖服务端当前值。`}><div className="conflict-icon"><CircleAlert size={18} /></div><div className="conflict-grid"><section><span>我的修改</span><code>{JSON.stringify(conflict?.local.fields ?? {}, null, 2)}</code></section><section><span>服务端当前值</span><code>{JSON.stringify(current?.fields ?? "广告位已删除", null, 2)}</code></section></div><footer className="dialog-actions"><Button onClick={onClose}>保留我的输入</Button><Button variant="primary" onClick={onReload}>重新加载当前值</Button></footer></Modal>;
}

function TableSkeleton() { return <section className="table-panel entity-table-panel"><div className="table-skeleton"><LoaderCircle className="spin" /><span>正在载入广告位</span></div></section>; }
function DetailSkeleton() { return <div className="detail-skeleton"><LoaderCircle className="spin" /><span>正在载入广告位详情</span></div>; }
function StatusChip({ enabled }: { enabled: boolean }) { return <span className={enabled ? "status-chip status-chip--enabled" : "status-chip status-chip--disabled"}><i />{enabled ? "已启用" : "已停用"}</span>; }
function isEntityDirty(entity: EntityView) { return entity.origin === "draft_baseline" || entity.origin === "draft_environment_override" || entity.draft.present; }
function adTypeLabel(value: unknown) { return ({ app_open: "App Open", interstitial: "插屏", native: "原生" } as Record<string, string>)[String(value)] ?? String(value ?? "-"); }
function enumLabel(name: string, value: string) { if (name === "ad_type") return adTypeLabel(value); return value; }
function fieldErrorCount(errors: Record<string, string>) { return Object.keys(errors).length; }
function defaultsFor(fields: FieldSchema[]) { return Object.fromEntries(fields.map((field) => [field.name, field.default])); }
function findPlacement(state: DraftView, id: string): EntityRecord | undefined { const placements = state.effective.placements; return Array.isArray(placements) ? placements.find((item): item is EntityRecord => Boolean(item && typeof item === "object" && "id" in item && (item as EntityRecord).id === id)) : undefined; }
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
