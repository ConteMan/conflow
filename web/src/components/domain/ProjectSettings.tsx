import { LockKeyhole, Save } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import type { Project, UpdateProjectInput } from "../../api/client";
import { Button } from "../ui/Button";

export function ProjectSettings({ project, busy, readOnly, onSave }: { project: Project; busy: boolean; readOnly: boolean; onSave: (value: UpdateProjectInput) => Promise<boolean> }) {
  const [id, setId] = useState(project.id);
  const [name, setName] = useState(project.name);
  const nameRef = useRef<HTMLInputElement>(null);
  useEffect(() => { setId(project.id); setName(project.name); }, [project]);
  return (
    <main className="page-container narrow-page">
      <header className="page-heading"><div><h1>项目资料</h1><p>项目名称用于界面识别；稳定 ID 用于清单和 API。</p></div></header>
      {readOnly ? <div className="readonly-banner"><LockKeyhole size={18} /><div><strong>当前为只读模式</strong><p>服务端未授予项目编辑能力。</p></div></div> : null}
      <section className="panel settings-panel"><form onSubmit={(event) => { event.preventDefault(); void onSave({ id: id.trim(), name: name.trim() }); }}>
        <label>项目名称<input ref={nameRef} value={name} maxLength={120} disabled={readOnly} onChange={(event) => setName(event.target.value)} required /></label>
        <label>项目 ID<input value={id} pattern="[a-z][a-z0-9-]{1,62}" disabled={readOnly} onChange={(event) => setId(event.target.value)} required /><small>修改 ID 会改变项目的稳定标识，请谨慎操作。</small></label>
        <dl className="readonly-details"><div><dt>配置包</dt><dd><code>{project.pack_ref}</code></dd></div><div><dt>源适配器</dt><dd><code>{project.source_type}</code></dd></div></dl>
        <div className="form-actions"><Button variant="primary" type="submit" disabled={busy || readOnly || !id.trim() || !name.trim()} icon={<Save size={16} />}>{busy ? "保存中…" : "保存项目资料"}</Button></div>
      </form></section>
    </main>
  );
}
