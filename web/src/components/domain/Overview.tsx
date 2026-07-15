import { Boxes, Cloud, GitBranch, Plus, Settings, ShieldCheck, SwitchCamera } from "lucide-react";
import { useEffect, useState } from "react";
import { getProviderStatus, listDraftEntities, type Environment, type PackMetadata, type Project, type ProviderStatus, type ValidationResult } from "../../api/client";
import { Button } from "../ui/Button";
import { ProviderConnectionCard } from "./ProviderConnectionCard";

type EntityCounts = { placement: number; frequencyPolicy: number; featureSwitch: number };

export function Overview({ project, selectedEnvironment, environments, pack, validation, draftDirty, revision, onManageEnvironments, onManageProject, onSwitchEnvironment, onCreateEnvironment }: {
  project: Project;
  selectedEnvironment: Environment;
  environments: Environment[];
  pack: PackMetadata | null;
  validation: ValidationResult | null;
  draftDirty: boolean;
  revision: number;
  onManageEnvironments: (environmentId?: string) => void;
  onManageProject: () => void;
  onSwitchEnvironment: () => void;
  onCreateEnvironment: () => void;
}) {
  const [entityCounts, setEntityCounts] = useState<EntityCounts | null>(null);
  const [providerStatus, setProviderStatus] = useState<ProviderStatus | null>(null);
  const production = selectedEnvironment.kind === "production";

  useEffect(() => {
    const controller = new AbortController();
    setEntityCounts(null);
    setProviderStatus(null);

    void Promise.all([
      listDraftEntities(selectedEnvironment.id, "placement", controller.signal),
      listDraftEntities(selectedEnvironment.id, "frequency_policy", controller.signal),
      listDraftEntities(selectedEnvironment.id, "feature_switch", controller.signal),
    ]).then(([placements, frequencyPolicies, featureSwitches]) => {
      if (!controller.signal.aborted) setEntityCounts({ placement: placements.data.length, frequencyPolicy: frequencyPolicies.data.length, featureSwitch: featureSwitches.data.length });
    }).catch(() => {
      // Each metric falls back to an unavailable state when local data cannot be loaded.
    });

    void getProviderStatus(selectedEnvironment.id, controller.signal)
      .then((response) => {
        if (!controller.signal.aborted) setProviderStatus(response.data);
      })
      .catch(() => {
        // The overview remains usable when the provider cannot be reached.
      });

    return () => controller.abort();
  }, [selectedEnvironment.id]);

  const diagnosticCounts = countDiagnostics(validation);
  const validationReady = validation?.readiness === "ready" && validation.status === "fresh";
  const provider = providerMetric(providerStatus);
  return (
    <main className="page-container">
      <header className="page-heading">
        <div><span className="eyebrow">项目概览</span><div className="project-title"><h1>{project.name}</h1><Button className="project-settings-link" icon={<Settings size={15} />} onClick={onManageProject}>项目设置</Button></div><p>{uniqueSubtitle(project.name, selectedEnvironment.name)}</p></div>
        <div className="overview-heading-actions"><Button onClick={() => onManageEnvironments()}>管理环境</Button><Button variant="primary" icon={<Plus size={16} />} onClick={onCreateEnvironment}>新建环境</Button></div>
      </header>
      <section className="metric-grid" aria-label="项目状态">
        <Metric icon={<Boxes />} label="配置实体" value={entityCounts ? String(entityCounts.placement + entityCounts.frequencyPolicy + entityCounts.featureSwitch) : "—"} detail={entityCounts ? `${entityCounts.placement} 广告位 · ${entityCounts.frequencyPolicy} 频控 · ${entityCounts.featureSwitch} 开关` : "实体数据暂不可用"} muted={!entityCounts} />
        <Metric icon={<GitBranch />} label="未发布修改" value={draftDirty ? "有修改" : "已同步"} detail={draftDirty ? "等待校验与发布" : `revision ${revision}`} />
        <Metric icon={<ShieldCheck />} label="校验状态" value={validationReady ? "可发布" : validation ? "有阻断" : "未校验"} detail={validation ? `阻断 ${diagnosticCounts.blocking} · 提醒 ${diagnosticCounts.warning}` : "运行校验以检查发布条件"} muted={!validation} />
        <Metric icon={<Cloud />} label="远端连接" value={provider.value} detail={provider.detail} muted={providerStatus === null} />
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
          {pack ? <><p className="pack-description">{pack.description}</p><div className="chip-list">{pack.capabilities.map((capability) => <span className="chip" key={capability}>{capabilityLabel(capability)}</span>)}</div><dl className="detail-list"><div><dt>Schema 版本</dt><dd>{pack.schema_version}</dd></div><div><dt>实体类型</dt><dd>{pack.entity_types.length}</dd></div></dl></> : <p className="muted-copy">Pack 元数据暂时不可用；项目仍可管理。</p>}
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

function uniqueSubtitle(projectName: string, environmentName: string) {
  return [...new Set([projectName, environmentName])].join(" · ");
}

function capabilityLabel(capability: string) {
  return ({
    entities: "实体管理",
    environment_overrides: "环境覆盖",
  } as Record<string, string>)[capability] ?? capability;
}

function countDiagnostics(validation: ValidationResult | null) {
  return (validation?.diagnostics ?? []).reduce((counts, diagnostic) => {
    if (diagnostic.severity === "blocking" || diagnostic.severity === "error") counts.blocking += 1;
    else if (diagnostic.severity === "warning") counts.warning += 1;
    return counts;
  }, { blocking: 0, warning: 0 });
}

function providerMetric(status: ProviderStatus | null) {
  if (!status) return { value: "—", detail: "连接状态暂不可用" };
  if (status.status === "connected") return { value: "已连接", detail: "Firebase Remote Config" };
  if (status.status === "not_configured") return { value: "未配置", detail: "请配置 Firebase 服务账号" };
  return { value: "不可用", detail: status.status === "unauthorized" ? "服务账号未获授权" : "请检查 Provider 连接" };
}
