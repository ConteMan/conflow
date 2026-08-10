import { ArrowLeft, ChevronRight, Plus, Save, Search, ShieldAlert, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import {
  ConflowAPIError,
  createDraftEntity,
  replaceDraftEntity,
  type Diagnostic,
  type DraftView,
  type EntityRecord,
  type EntityView,
  type Environment,
} from "../../api/client";
import { Button } from "../ui/Button";
import { DataTable } from "../ui/DataTable";

type StrategyFields = {
  description: string | null;
  placement_rule_mode: "allowlist";
  allowlist_placement_ids: string[];
  frequency_policy_overrides: Record<string, Record<string, unknown>>;
};

export function AdStrategyConfiguration({ environment, revision, draft, settings, strategies, placements, loading, diagnostics, onSaved, onOpen, onDelete }: {
  environment: Environment;
  revision: number;
  draft: DraftView | null;
  settings: EntityView | null;
  strategies: EntityView[];
  placements: EntityView[];
  loading: boolean;
  diagnostics: Diagnostic[];
  onSaved: () => void;
  onOpen: (id?: string) => void;
  onDelete: (entity: EntityView) => void;
}) {
  return <div className="strategy-configuration">
    <StrategySettingsCard environment={environment} revision={revision} draft={draft} settings={settings} strategies={strategies} onSaved={onSaved} />
    <StrategyTable settings={settings} strategies={strategies} placements={placements} loading={loading} diagnostics={diagnostics} onOpen={onOpen} onDelete={onDelete} />
  </div>;
}

function StrategySettingsCard({ environment, revision, draft, settings, strategies, onSaved }: { environment: Environment; revision: number; draft: DraftView | null; settings: EntityView | null; strategies: EntityView[]; onSaved: () => void }) {
  const source = settings?.effective.value.fields;
  const [parameterKey, setParameterKey] = useState(String(source?.parameter_key ?? "ad_strategies_config"));
  const [defaultStrategyID, setDefaultStrategyID] = useState(String(source?.default_strategy_id ?? ""));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    setParameterKey(String(source?.parameter_key ?? "ad_strategies_config"));
    setDefaultStrategyID(String(source?.default_strategy_id ?? ""));
  }, [source?.default_strategy_id, source?.parameter_key]);
  const save = async () => {
    const entity: EntityRecord = { id: "default", fields: { parameter_key: parameterKey, payload_version: 1, default_strategy_id: defaultStrategyID || null } };
    setSaving(true); setError(null);
    try {
      if (settings) await replaceDraftEntity(environment.id, "ad_strategy_settings", "default", revision, { expected_source_revision: draft?.source_revision ?? settings.source_revision, write_scope: "baseline", entity });
      else await createDraftEntity(environment.id, revision, { expected_source_revision: draft?.source_revision ?? "", write_scope: "baseline", entity_type: "ad_strategy_settings", entity });
      onSaved();
    } catch (cause) {
      setError(strategyRequestError(cause, "保存广告策略设置失败。"));
    } finally { setSaving(false); }
  };
  return <section className="strategy-settings-card">
    <header><div><h2>策略发布设置</h2><p>{settings ? <>统一编译到 <code>{String(source?.parameter_key)}</code></> : "先初始化设置，再创建第一条策略。"}</p></div><span className={settings ? "status-chip status-chip--enabled" : "status-chip status-chip--disabled"}><i />{settings ? "已启用" : "未启用"}</span></header>
    <div className="strategy-settings-fields">
      <label className="form-field"><span>Remote Config 参数键</span><input value={parameterKey} onChange={(event) => setParameterKey(event.target.value)} /><small>通用值 · 必须与其他受管参数键唯一</small></label>
      <label className="form-field"><span>默认策略</span><select value={defaultStrategyID} onChange={(event) => setDefaultStrategyID(event.target.value)}><option value="">不启用默认策略</option>{strategies.map((strategy) => <option key={strategy.entity_id} value={strategy.entity_id}>{strategy.entity_id}</option>)}</select><small>默认策略被引用后不可删除</small></label>
      <label className="form-field"><span>负载版本</span><input value="1" disabled /><small>由 schema 3 固定管理</small></label>
    </div>
    {error ? <p className="binding-error" role="alert">{error}</p> : null}
    <footer><Button variant="primary" icon={<Save size={16} />} disabled={saving || !parameterKey.trim()} onClick={() => void save()}>{saving ? "正在保存" : settings ? "保存设置" : "初始化广告策略"}</Button></footer>
  </section>;
}

function StrategyTable({ settings, strategies, placements, loading, diagnostics, onOpen, onDelete }: { settings: EntityView | null; strategies: EntityView[]; placements: EntityView[]; loading: boolean; diagnostics: Diagnostic[]; onOpen: (id?: string) => void; onDelete: (entity: EntityView) => void }) {
  const [query, setQuery] = useState("");
  const [dirtyOnly, setDirtyOnly] = useState(false);
  const defaultID = String(settings?.effective.value.fields.default_strategy_id ?? "");
  const rows = useMemo(() => strategies.filter((strategy) => `${strategy.entity_id} ${String(strategy.effective.value.fields.description ?? "")}`.toLowerCase().includes(query.toLowerCase()) && (!dirtyOnly || strategy.change_status !== "unchanged")), [dirtyOnly, query, strategies]);
  const columns = useMemo<ColumnDef<EntityView, unknown>[]>(() => [
    { id: "id", header: "策略", accessorFn: (strategy) => strategy.entity_id, cell: (info) => { const strategy = info.row.original; const count = diagnostics.filter((item) => item.entity_ref === strategy.entity_ref).length; return <div className="entity-label"><strong>{strategy.entity_id}{strategy.entity_id === defaultID ? <span className="strategy-default-badge">默认</span> : null}</strong><span className="muted-cell">{String(strategy.effective.value.fields.description ?? "未填写描述")}</span>{count ? <span className="strategy-diagnostic">{count} 项校验问题</span> : null}</div>; } },
    { id: "placements", header: "适用广告位", accessorFn: (strategy) => stringArray(strategy.effective.value.fields.allowlist_placement_ids).length, cell: (info) => `${info.getValue()} / ${placements.length}` },
    { id: "overrides", header: "频控覆盖", accessorFn: (strategy) => Object.keys(objectValue(strategy.effective.value.fields.frequency_policy_overrides)).length, cell: (info) => `${info.getValue()} 个广告位` },
    { id: "status", header: "未发布修改", accessorFn: (strategy) => strategy.change_status, cell: (info) => <StrategyChangeStatus status={String(info.getValue())} /> },
    { id: "actions", header: () => <span className="sr-only">操作</span>, enableSorting: false, size: 96, minSize: 96, maxSize: 96, cell: (info) => <div className="row-actions strategy-editor-only"><button className="icon-button row-open" aria-label={`编辑策略 ${info.row.original.entity_id}`} onClick={(event) => { event.stopPropagation(); onOpen(info.row.original.entity_id); }}><ChevronRight size={18} /></button><button className="icon-button row-delete" aria-label={`删除策略 ${info.row.original.entity_id}`} onClick={(event) => { event.stopPropagation(); onDelete(info.row.original); }}><Trash2 size={16} /></button></div> },
  ], [defaultID, diagnostics, onDelete, onOpen, placements.length]);
  return <section className="strategy-list-section">
    <header><div><h2>广告策略</h2><p>按广告位控制策略适用范围，并只覆盖需要变化的频控。</p></div><Button className="strategy-editor-only" variant="primary" icon={<Plus size={16} />} disabled={!settings} onClick={() => onOpen()}>新建策略</Button></header>
    {!settings ? <div className="strategy-guidance"><ShieldAlert size={18} /><span>初始化上方发布设置后即可创建策略。</span></div> : null}
    <div className="strategy-mobile-readonly">窄屏仅提供策略摘要。请使用桌面端创建、编辑或删除策略。</div>
    <div className="entity-toolbar"><p className="table-summary">总计 {strategies.length} 个策略 · 筛选命中 {rows.length} 个</p><label className="toolbar-search"><Search size={16} /><span className="sr-only">搜索策略</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索策略 ID 或描述" /></label><label className="dirty-filter"><input type="checkbox" checked={dirtyOnly} onChange={(event) => setDirtyOnly(event.target.checked)} />仅看未发布修改</label></div>
    {loading ? <div className="table-skeleton">正在载入广告策略</div> : <div className="table-panel"><DataTable ariaLabel="广告策略列表" columns={columns} data={rows} defaultSorting={[{ id: "id", desc: false }]} emptyState={settings ? "还没有广告策略。" : "广告策略功能尚未启用。"} getRowId={(strategy) => strategy.entity_id} minTableWidth={720} onRowClick={(strategy) => onOpen(strategy.entity_id)} /></div>}
  </section>;
}

export function AdStrategyDetail({ environment, revision, draft, strategy, strategyID, placements, policies, diagnostics, onBack, onSaved }: { environment: Environment; revision: number; draft: DraftView | null; strategy?: EntityView; strategyID?: string; placements: EntityView[]; policies: EntityView[]; diagnostics: Diagnostic[]; onBack: () => void; onSaved: () => void }) {
  const creating = !strategyID;
  const [newID, setNewID] = useState("");
  const [fields, setFields] = useState<StrategyFields>(() => strategy ? strategyFields(strategy.effective.value.fields) : { description: null, placement_rule_mode: "allowlist", allowlist_placement_ids: [], frequency_policy_overrides: {} });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const entityID = strategyID ?? newID;
  useEffect(() => { if (strategy) setFields(strategyFields(strategy.effective.value.fields)); }, [strategy]);
  const selected = new Set(fields.allowlist_placement_ids);
  const togglePlacement = (placementID: string, enabled: boolean) => setFields((current) => {
    const ids = enabled ? [...new Set([...current.allowlist_placement_ids, placementID])].sort() : current.allowlist_placement_ids.filter((id) => id !== placementID);
    const overrides = { ...current.frequency_policy_overrides };
    if (!enabled) delete overrides[placementID];
    return { ...current, allowlist_placement_ids: ids, frequency_policy_overrides: overrides };
  });
  const updateOverride = (placementID: string, field: string, state: "inherit" | "disabled" | "custom", value?: unknown) => setFields((current) => {
    const overrides = { ...current.frequency_policy_overrides };
    const placementOverride = { ...(overrides[placementID] ?? {}) };
    if (state === "inherit") delete placementOverride[field];
    else placementOverride[field] = state === "disabled" ? null : value;
    if (Object.keys(placementOverride).length) overrides[placementID] = placementOverride;
    else delete overrides[placementID];
    return { ...current, frequency_policy_overrides: overrides };
  });
  const save = async () => {
    const entity: EntityRecord = { id: entityID, fields };
    setSaving(true); setError(null);
    try {
      if (creating) await createDraftEntity(environment.id, revision, { expected_source_revision: draft?.source_revision ?? "", write_scope: "baseline", entity_type: "ad_strategy", entity });
      else await replaceDraftEntity(environment.id, "ad_strategy", entityID, revision, { expected_source_revision: draft?.source_revision ?? strategy?.source_revision ?? "", write_scope: "baseline", entity });
      onSaved(); onBack();
    } catch (cause) {
      setError(strategyRequestError(cause, "保存广告策略失败。"));
    } finally { setSaving(false); }
  };
  const ownDiagnostics = diagnostics.filter((item) => !strategy || item.entity_ref === strategy.entity_ref);
  return <main className="page-container placement-detail strategy-detail-page">
    <header className="detail-heading"><div className="detail-heading-title"><button className="icon-button detail-back" aria-label="返回广告策略列表" onClick={onBack}><ArrowLeft size={19} /></button><div><h1>{creating ? "新建广告策略" : entityID}</h1><p><code>{creating ? "创建后策略 ID 不可修改" : entityID}</code> · allowlist</p></div></div><Button className="strategy-editor-only" variant="primary" icon={<Save size={16} />} disabled={saving || !entityID || fields.allowlist_placement_ids.length === 0} onClick={() => void save()}>{saving ? "正在保存" : "保存策略"}</Button></header>
    <div className="strategy-mobile-readonly">窄屏只读。请使用桌面端编辑广告策略。</div>
    <div className="detail-layout"><div className="detail-main">
      {ownDiagnostics.length ? <section className="strategy-diagnostics"><strong>{ownDiagnostics.length} 项校验问题</strong>{ownDiagnostics.map((item) => <p key={`${item.code}:${item.path}`}>{item.message}</p>)}</section> : null}
      <section className="editor-section"><header><div><h2>基础信息</h2><p>策略 ID 是客户端与审计使用的稳定标识。</p></div></header><div className="field-grid">{creating ? <label className="form-field"><span>策略 ID</span><input value={newID} onChange={(event) => setNewID(event.target.value)} placeholder="例如 balanced" /><small>小写字母、数字和下划线</small></label> : <div className="immutable-field"><span>策略 ID</span><code>{entityID}</code><small>创建后不可修改</small></div>}<label className="form-field"><span>描述</span><input value={fields.description ?? ""} onChange={(event) => setFields((current) => ({ ...current, description: event.target.value || null }))} placeholder="说明策略用途" /><small>仅用于 Conflow，不参与编译</small></label></div></section>
      <section className="editor-section strategy-placement-section"><header><div><h2>适用广告位与频控覆盖</h2><p>先选择 allowlist；未覆盖字段继承广告位基础频控，显式关闭会编译为 null。</p></div><span>{selected.size} 个广告位</span></header><div className="strategy-placement-list">{placements.map((placement) => { const checked = selected.has(placement.entity_id); const override = fields.frequency_policy_overrides[placement.entity_id] ?? {}; return <article className={checked ? "strategy-placement strategy-placement--selected" : "strategy-placement"} key={placement.entity_id}><header><label><input type="checkbox" checked={checked} onChange={(event) => togglePlacement(placement.entity_id, event.target.checked)} /><span><strong>{String(placement.effective.value.fields.key ?? placement.entity_id)}</strong><code>{String(placement.effective.value.fields.client_id ?? placement.entity_id)}</code></span></label><span>{basePolicyLabel(placement, policies)}</span></header>{checked ? <div className="strategy-override-grid">{(["cooldown", "interval", "max_count", "shift_count", "positions"] as const).map((field) => <OverrideField key={field} field={field} baseValue={basePolicyValue(placement, policies, field)} override={override} onChange={(state, value) => updateOverride(placement.entity_id, field, state, value)} />)}</div> : null}</article>; })}</div></section>
      {error ? <p className="binding-error" role="alert">{error}</p> : null}
    </div><aside className="detail-sidebar"><section className="change-summary"><h2>影响摘要</h2><dl><div><dt>当前环境</dt><dd>{environment.name}</dd></div><div><dt>适用广告位</dt><dd>{selected.size}</dd></div><div><dt>含覆盖广告位</dt><dd>{Object.keys(fields.frequency_policy_overrides).length}</dd></div><div><dt>风险</dt><dd>由 Plan 判定</dd></div></dl></section><details className="advanced-info"><summary>继承语义</summary><p>缺失 = 继承；null = 显式关闭；自定义值 = 覆盖。</p></details></aside></div>
  </main>;
}

function OverrideField({ field, baseValue, override, onChange }: { field: string; baseValue: unknown; override: Record<string, unknown>; onChange: (state: "inherit" | "disabled" | "custom", value?: unknown) => void }) {
  const hasOverride = Object.prototype.hasOwnProperty.call(override, field);
  const value = override[field];
  const state = !hasOverride ? "inherit" : value === null ? "disabled" : "custom";
  const setState = (next: "inherit" | "disabled" | "custom") => onChange(next, next === "custom" ? defaultOverrideValue(field, baseValue) : undefined);
  return <div className="strategy-override-field"><div><strong>{frequencyFieldLabel(field)}</strong><small>基础：{frequencyValueSummary(baseValue)}</small></div><select value={state} onChange={(event) => setState(event.target.value as "inherit" | "disabled" | "custom")}><option value="inherit">继承</option><option value="disabled">显式关闭</option><option value="custom">自定义</option></select>{state === "custom" ? <OverrideValueEditor field={field} value={value} onChange={(next) => onChange("custom", next)} /> : <span className={`strategy-override-state strategy-override-state--${state}`}>{state === "inherit" ? "使用基础频控" : "客户端不应用此约束"}</span>}</div>;
}

function OverrideValueEditor({ field, value, onChange }: { field: string; value: unknown; onChange: (value: unknown) => void }) {
  if (field === "shift_count") { const object = objectValue(value); return <div className="strategy-value-pair"><label>AM<input type="number" min="0" value={Number(object.am ?? 0)} onChange={(event) => onChange({ ...object, am: Number(event.target.value) })} /></label><label>PM<input type="number" min="0" value={Number(object.pm ?? 0)} onChange={(event) => onChange({ ...object, pm: Number(event.target.value) })} /></label></div>; }
  if (field === "positions") return <input value={stringArray(value).join(", ")} onChange={(event) => onChange(event.target.value.split(",").map((item) => item.trim()).filter(Boolean))} placeholder="位置，以逗号分隔" />;
  const object = objectValue(value);
  const units = field === "max_count" ? ["session", "day"] : field === "interval" ? ["seconds", "minutes", "hours", "days", "items"] : ["seconds", "minutes", "hours", "days"];
  return <div className="strategy-value-pair"><input type="number" min={field === "max_count" ? 0 : 1} value={Number(object.value ?? 1)} onChange={(event) => onChange({ ...object, value: Number(event.target.value) })} /><select value={String(object.unit ?? units[0])} onChange={(event) => onChange({ ...object, unit: event.target.value })}>{units.map((unit) => <option key={unit} value={unit}>{unit}</option>)}</select></div>;
}

function strategyFields(value: Record<string, unknown>): StrategyFields { return { description: typeof value.description === "string" ? value.description : null, placement_rule_mode: "allowlist", allowlist_placement_ids: stringArray(value.allowlist_placement_ids), frequency_policy_overrides: objectValue(value.frequency_policy_overrides) as Record<string, Record<string, unknown>> }; }
function objectValue(value: unknown): Record<string, any> { return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, any> : {}; }
function stringArray(value: unknown): string[] { return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : []; }
function basePolicyLabel(placement: EntityView, policies: EntityView[]) { const fields = placement.effective.value.fields; return fields.frequency_policy_type === "custom" ? "自定义基础频控" : `基础策略：${policies.find((item) => item.entity_id === fields.frequency_policy_id)?.entity_id ?? "未找到"}`; }
function basePolicyValue(placement: EntityView, policies: EntityView[], field: string) { const fields = placement.effective.value.fields; if (fields.frequency_policy_type === "custom") return objectValue(fields.custom_frequency_policy)[field]; return policies.find((item) => item.entity_id === fields.frequency_policy_id)?.effective.value.fields[field]; }
function defaultOverrideValue(field: string, base: unknown) { if (base !== undefined && base !== null) return base; if (field === "shift_count") return { am: 1, pm: 1 }; if (field === "positions") return []; if (field === "max_count") return { unit: "day", value: 1 }; return { unit: "minutes", value: 1 }; }
function frequencyFieldLabel(field: string) { return ({ cooldown: "冷却时间", interval: "展示间隔", max_count: "次数上限", shift_count: "分时上限", positions: "适用位置" } as Record<string, string>)[field] ?? field; }
function frequencyValueSummary(value: unknown) { if (value === null || value === undefined) return "未启用"; if (Array.isArray(value)) return value.length ? value.join("、") : "空集合"; const object = objectValue(value); if ("am" in object || "pm" in object) return `AM ${object.am ?? 0} / PM ${object.pm ?? 0}`; if ("unit" in object && "value" in object) return `${object.value} ${object.unit}`; return "已配置"; }
function strategyRequestError(cause: unknown, fallback: string) { if (cause instanceof ConflowAPIError) { if (cause.code === "revision_mismatch" || cause.code === "source_revision_mismatch") return "配置已在其他位置变化，请返回列表刷新后重试。"; if (cause.code === "validation_failed") return cause.details?.[0]?.message ?? "字段值不符合配置包规则。"; return cause.message; } return fallback; }
function StrategyChangeStatus({ status }: { status: string }) { if (status === "unchanged") return <span className="muted-cell">无</span>; return <span className={`dirty-chip${status === "created" ? " dirty-chip--created" : ""}`}>{({ created: "新增", modified: "已修改", deleted: "待删除" } as Record<string, string>)[status] ?? status}</span>; }
