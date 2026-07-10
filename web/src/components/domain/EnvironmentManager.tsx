import { LockKeyhole, Plus, Save, ShieldCheck, Trash2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { CreateEnvironmentInput, Environment, EnvironmentKind, UpdateEnvironmentInput } from "../../api/client";
import { Button } from "../ui/Button";
import { Modal } from "../ui/Dialog";
import { kindLabel } from "./Overview";

type SubmitPayload = { mode: "create"; value: CreateEnvironmentInput } | { mode: "edit"; id: string; value: UpdateEnvironmentInput };

export function EnvironmentManager({ environments, selectedEnvironmentId, busy, readOnly, onSelect, onSubmit, onDelete }: {
  environments: Environment[];
  selectedEnvironmentId: string;
  busy: boolean;
  readOnly: boolean;
  onSelect: (id: string) => void;
  onSubmit: (payload: SubmitPayload) => Promise<boolean>;
  onDelete: (environment: Environment) => Promise<boolean>;
}) {
  const [mode, setMode] = useState<"edit" | "create">("edit");
  const selected = environments.find((environment) => environment.id === selectedEnvironmentId) ?? environments[0];
  const [deleteTarget, setDeleteTarget] = useState<Environment | null>(null);
  const createButtonRef = useRef<HTMLButtonElement>(null);

  return (
    <main className="page-container">
      <header className="page-heading"><div><h1>环境管理</h1><p>环境 ID 与类型创建后不可修改；显示名称可随时调整。</p></div><Button ref={createButtonRef} variant="primary" icon={<Plus size={16} />} disabled={readOnly} onClick={() => setMode("create")}>新建环境</Button></header>
      {readOnly ? <div className="readonly-banner"><LockKeyhole size={18} /><div><strong>当前为只读模式</strong><p>服务端未授予环境管理能力。</p></div></div> : null}
      <div className="management-layout">
        <section className="table-panel">
          <table>
            <thead><tr><th>名称</th><th>环境 ID</th><th>类型</th><th>Firebase 项目</th><th>发布确认</th></tr></thead>
            <tbody>{environments.map((environment) => <tr key={environment.id} className={selected?.id === environment.id && mode === "edit" ? "selected-row" : ""} onClick={() => { onSelect(environment.id); setMode("edit"); }}><td><button type="button" className="row-select-button" aria-label={`编辑环境 ${environment.name}`} onClick={() => { onSelect(environment.id); setMode("edit"); }}><strong>{environment.name}</strong></button></td><td><code>{environment.id}</code></td><td><span className={`kind-badge kind-badge--${environment.kind}`}>{kindLabel(environment.kind)}</span></td><td><code>{environment.provider.project_id}</code></td><td>{environment.publish.requires_confirmation ? "需要" : "不需要"}</td></tr>)}</tbody>
          </table>
        </section>
        <EnvironmentForm key={mode === "create" ? "create" : selected?.id} mode={mode} environment={mode === "edit" ? selected : undefined} busy={busy} readOnly={readOnly} onCancel={() => { setMode("edit"); createButtonRef.current?.focus(); }} onSubmit={async (payload) => { const saved = await onSubmit(payload); if (saved) setMode("edit"); }} onRequestDelete={(environment) => setDeleteTarget(environment)} canDelete={environments.length > 1} />
      </div>
      <Modal open={deleteTarget !== null} onOpenChange={(open) => { if (!open) setDeleteTarget(null); }} title={`删除 ${deleteTarget?.name ?? "环境"}`} description="此操作会从项目清单中删除环境。最后一个环境受到保护，无法删除。">
        <div className="danger-callout"><Trash2 size={18} /><p>环境 ID <code>{deleteTarget?.id}</code> 将不再可用。Provider 远端项目不会被删除。</p></div>
        <footer className="dialog-actions"><Button onClick={() => setDeleteTarget(null)}>取消</Button><Button variant="danger" disabled={busy} onClick={async () => { if (deleteTarget && await onDelete(deleteTarget)) setDeleteTarget(null); }}>确认删除</Button></footer>
      </Modal>
    </main>
  );
}

function EnvironmentForm({ mode, environment, busy, readOnly, onCancel, onSubmit, onRequestDelete, canDelete }: {
  mode: "edit" | "create";
  environment?: Environment;
  busy: boolean;
  readOnly: boolean;
  onCancel: () => void;
  onSubmit: (payload: SubmitPayload) => void;
  onRequestDelete: (environment: Environment) => void;
  canDelete: boolean;
}) {
  const [id, setId] = useState(environment?.id ?? "");
  const [name, setName] = useState(environment?.name ?? "");
  const [kind, setKind] = useState<EnvironmentKind>(environment?.kind ?? "staging");
  const [projectId, setProjectId] = useState(environment?.provider.project_id ?? "");
  const [requiresConfirmation, setRequiresConfirmation] = useState(environment?.publish.requires_confirmation ?? true);
  const nameRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!environment) return;
    setId(environment.id);
    setName(environment.name);
    setKind(environment.kind);
    setProjectId(environment.provider.project_id);
    setRequiresConfirmation(environment.publish.requires_confirmation);
  }, [environment]);
  useEffect(() => { nameRef.current?.focus(); }, [mode, environment?.id]);
  const valid = id.trim().length >= 2 && name.trim().length > 0 && projectId.trim().length > 0;
  return (
    <aside className="editor-panel" aria-label={mode === "create" ? "新建环境" : `编辑 ${environment?.name}`}>
      <header><div><h2>{mode === "create" ? "新建环境" : `编辑 ${environment?.name}`}</h2><p>连接和发布策略仅影响此环境。</p></div>{mode === "edit" && environment ? <span className={`kind-badge kind-badge--${environment.kind}`}>{kindLabel(environment.kind)}</span> : null}</header>
      <form onSubmit={(event) => { event.preventDefault(); if (!valid) return; if (mode === "create") onSubmit({ mode, value: { id: id.trim(), name: name.trim(), kind, provider: { type: "firebase-remote-config", project_id: projectId.trim() }, publish: { requires_confirmation: requiresConfirmation } } }); else if (environment) onSubmit({ mode, id: environment.id, value: { name: name.trim(), provider: { type: "firebase-remote-config", project_id: projectId.trim() }, publish: { requires_confirmation: requiresConfirmation } } }); }}>
        <label>显示名称<input ref={nameRef} value={name} maxLength={120} disabled={readOnly} onChange={(event) => setName(event.target.value)} required /></label>
        <label>环境 ID<div className="locked-input"><input value={id} disabled={mode === "edit" || readOnly} pattern="[a-z][a-z0-9-]{1,62}" onChange={(event) => setId(event.target.value)} required />{mode === "edit" ? <LockKeyhole size={15} /> : null}</div><small>{mode === "edit" ? "创建后不可修改" : "2–63 位小写字母、数字或连字符"}</small></label>
        <label>环境类型<select value={kind} disabled={mode === "edit" || readOnly} onChange={(event) => setKind(event.target.value as EnvironmentKind)}>{(["development", "staging", "production", "custom"] as EnvironmentKind[]).map((value) => <option value={value} key={value}>{kindLabel(value)}</option>)}</select><small>{mode === "edit" ? "类型由服务端保留，无法修改" : "只有 Production 会触发持续风险标识"}</small></label>
        <label>Firebase 项目<input value={projectId} maxLength={128} disabled={readOnly} onChange={(event) => setProjectId(event.target.value)} required /></label>
        <label className="checkbox-field"><input type="checkbox" checked={requiresConfirmation} disabled={readOnly} onChange={(event) => setRequiresConfirmation(event.target.checked)} /><span><strong>发布前需要确认</strong><small>Provider 发布能力尚未接入；此处保存清单策略。</small></span><ShieldCheck size={18} /></label>
        <div className="form-actions">{mode === "create" ? <Button type="button" onClick={onCancel}>取消</Button> : environment ? <Button type="button" variant="ghost" disabled={!canDelete || readOnly} icon={<Trash2 size={16} />} onClick={() => onRequestDelete(environment)}>删除</Button> : null}<Button type="submit" variant="primary" disabled={!valid || busy || readOnly} icon={<Save size={16} />}>{busy ? "保存中…" : "保存环境"}</Button></div>
      </form>
    </aside>
  );
}
