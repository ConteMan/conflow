import { GitCompareArrows, RefreshCw } from "lucide-react";
import type { Environment, ManifestState } from "../../api/client";
import { Button } from "../ui/Button";
import { Modal } from "../ui/Dialog";

export type LocalConflictValue = {
  resource: "project" | "environment";
  title: string;
  name: string;
  providerProjectId?: string;
};

export function ConflictDialog({ state, revision, local, open, onReload, onClose }: {
  state: ManifestState | undefined;
  revision: number | undefined;
  local: LocalConflictValue | null;
  open: boolean;
  onReload: () => void;
  onClose: () => void;
}) {
  const current = findCurrent(state, local);
  const currentName = local?.resource === "environment"
    ? current?.name ?? "资源已删除"
    : state?.project.name ?? "项目不可用";
  return (
    <Modal open={open} onOpenChange={(next) => { if (!next) onClose(); }} title="项目清单已被其他操作修改" description={`服务端当前 revision ${revision ?? "未知"}。请对照后重新加载，Conflow 不会自动覆盖。`}>
      <div className="conflict-icon"><GitCompareArrows size={18} /></div>
      <div className="conflict-grid">
        <section><span>我的修改</span><strong>{local?.name ?? "未保存修改"}</strong>{local?.providerProjectId ? <code>{local.providerProjectId}</code> : null}</section>
        <section><span>服务端当前值</span><strong>{currentName}</strong>{current ? <code>{current.provider.project_id}</code> : null}</section>
      </div>
      <footer className="dialog-actions"><Button onClick={onClose}>保留我的输入</Button><Button variant="primary" icon={<RefreshCw size={16} />} onClick={onReload}>重新加载当前值</Button></footer>
    </Modal>
  );
}

function findCurrent(state: ManifestState | undefined, local: LocalConflictValue | null): Environment | undefined {
  if (!state || local?.resource !== "environment") return undefined;
  return state.environments.find((environment) => environment.id === local.title);
}
