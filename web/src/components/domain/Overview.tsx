import { Boxes, Cloud, ExternalLink, GitBranch, Plus, Settings, ShieldCheck, SwitchCamera } from "lucide-react";
import type { Environment, PackMetadata, Project } from "../../api/client";
import { Button } from "../ui/Button";
import { ProviderConnectionCard } from "./ProviderConnectionCard";

export function Overview({ project, selectedEnvironment, environments, pack, onManageEnvironments, onManageProject, onSwitchEnvironment, onCreateEnvironment }: {
  project: Project;
  selectedEnvironment: Environment;
  environments: Environment[];
  pack: PackMetadata | null;
  onManageEnvironments: (environmentId?: string) => void;
  onManageProject: () => void;
  onSwitchEnvironment: () => void;
  onCreateEnvironment: () => void;
}) {
  const production = selectedEnvironment.kind === "production";
  return (
    <main className="page-container">
      {production ? <section className="production-banner"><ShieldCheck /><div><strong>你正在查看 Production</strong><p>修改会影响真实用户；发布能力将在后续 Spec 接入。</p></div><Button className="production-switch" icon={<SwitchCamera size={16} />} onClick={onSwitchEnvironment}>切换环境</Button></section> : null}
      <header className="page-heading">
        <div><span className="eyebrow">项目概览</span><div className="project-title"><h1>{project.name}</h1><Button className="project-settings-link" icon={<Settings size={15} />} onClick={onManageProject}>项目设置</Button></div><p>{uniqueSubtitle(project.name, selectedEnvironment.name)}</p></div>
        <div className="overview-heading-actions"><Button onClick={() => onManageEnvironments()}>管理环境</Button><Button variant="primary" icon={<Plus size={16} />} onClick={onCreateEnvironment}>新建环境</Button></div>
      </header>
      <section className="metric-grid" aria-label="项目状态">
        <Metric icon={<Boxes />} label="环境" value={String(environments.length)} detail="项目清单中的可用环境" />
        <Metric icon={<GitBranch />} label="配置来源" value={sourceLabel(project.source_type)} detail={project.source_type} />
        <Metric icon={<Cloud />} label="Provider 状态" value="查看连接卡" detail="在下方验证本地服务账号并拉取线上配置" muted />
        <Metric icon={<ExternalLink />} label="最近发布" value="见发布记录" detail="发布与回滚历史在「发布记录」页查看" muted />
      </section>
      <div className="overview-grid">
        <section className="panel">
          <div className="panel-heading"><div><h2>环境</h2><p>服务端环境类别决定 Production 风险状态。</p></div></div>
          <div className="environment-summary-list">
            {environments.map((environment) => (
              <button key={environment.id} className={environment.id === selectedEnvironment.id ? "environment-summary environment-summary--active" : "environment-summary"} onClick={() => onManageEnvironments(environment.id)}>
                <span><strong>{environment.name}</strong><code>{environment.id}</code></span>
                <span className={`kind-badge kind-badge--${environment.kind}`}>{kindLabel(environment.kind)}</span>
              </button>
            ))}
          </div>
        </section>
        <section className="panel">
          <div className="panel-heading"><div><h2>配置包能力</h2><p><code>{project.pack_ref}</code></p></div></div>
          {pack ? <><p className="pack-description">{pack.description}</p><div className="chip-list">{pack.capabilities.map((capability) => <span className="chip" key={capability}>{capability.replaceAll("_", " ")}</span>)}</div><dl className="detail-list"><div><dt>Schema version</dt><dd>{pack.schema_version}</dd></div><div><dt>实体类型</dt><dd>{pack.entity_types.length}</dd></div></dl></> : <p className="muted-copy">Pack 元数据暂时不可用；项目仍可管理。</p>}
        </section>
      </div>
      <ProviderConnectionCard environment={selectedEnvironment} />
    </main>
  );
}

function Metric({ icon, label, value, detail, muted = false }: { icon: React.ReactNode; label: string; value: string; detail: string; muted?: boolean }) {
  return <article className={muted ? "metric metric--muted" : "metric"}><div className="metric-label"><span>{label}</span>{icon}</div><strong>{value}</strong><p>{detail}</p></article>;
}

export function kindLabel(kind: Environment["kind"]) {
  return ({ development: "Development", staging: "Staging", production: "Production", custom: "Custom" })[kind];
}

function sourceLabel(source: Project["source_type"]) { return source === "managed-file" ? "托管文件" : "Git JSON"; }

function uniqueSubtitle(projectName: string, environmentName: string) {
  return [...new Set([projectName, environmentName])].join(" · ");
}
