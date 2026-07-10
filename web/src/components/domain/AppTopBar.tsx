import { ChevronDown, Layers3, Menu, X } from "lucide-react";
import { useState } from "react";
import type { Environment, Project } from "../../api/client";

export type Page = "overview" | "environments" | "project";

const navigation: Array<{ id: Page; label: string }> = [
  { id: "overview", label: "概览" },
  { id: "environments", label: "环境" },
  { id: "project", label: "项目设置" },
];

export function AppTopBar({ project, environments, selectedEnvironment, page, onEnvironmentChange, onPageChange }: {
  project: Project;
  environments: Environment[];
  selectedEnvironment: Environment;
  page: Page;
  onEnvironmentChange: (id: string) => void;
  onPageChange: (page: Page) => void;
}) {
  const [menuOpen, setMenuOpen] = useState(false);
  const production = selectedEnvironment.kind === "production";
  return (
    <header className={`app-topbar${production ? " app-topbar--production" : ""}`} data-testid="app-topbar">
      <a className="brand" href="#overview" onClick={() => onPageChange("overview")}>Conflow</a>
      <label className="context-selector">
        <Layers3 size={17} aria-hidden="true" />
        <span className="sr-only">当前环境</span>
        <span className="context-project">{project.name}</span>
        <span className="context-separator">/</span>
        <select aria-label="切换环境" value={selectedEnvironment.id} onChange={(event) => onEnvironmentChange(event.target.value)}>
          {environments.map((environment) => <option value={environment.id} key={environment.id}>{environment.name}</option>)}
        </select>
        <ChevronDown size={14} aria-hidden="true" />
      </label>
      <nav className="desktop-nav" aria-label="主导航">
        {navigation.map((item) => <NavButton key={item.id} item={item} active={page === item.id} onClick={onPageChange} />)}
        <span className="nav-disabled" title="将在后续 Spec 中接入">配置</span>
        <span className="nav-disabled" title="将在后续 Spec 中接入">校验</span>
      </nav>
      <div className="draft-slot" aria-label="未发布修改状态">未发布修改：尚未接入</div>
      <button className="mobile-menu-button" aria-label={menuOpen ? "关闭导航" : "打开导航"} aria-expanded={menuOpen} onClick={() => setMenuOpen((value) => !value)}>
        {menuOpen ? <X /> : <Menu />}
      </button>
      {menuOpen ? <nav className="mobile-nav" aria-label="窄屏导航">{navigation.map((item) => <NavButton key={item.id} item={item} active={page === item.id} onClick={(next) => { onPageChange(next); setMenuOpen(false); }} />)}</nav> : null}
      {production ? <div className="production-marker" data-testid="production-marker">Production 环境</div> : null}
    </header>
  );
}

function NavButton({ item, active, onClick }: { item: { id: Page; label: string }; active: boolean; onClick: (page: Page) => void }) {
  return <button className={active ? "nav-button nav-button--active" : "nav-button"} aria-current={active ? "page" : undefined} onClick={() => onClick(item.id)}>{item.label}</button>;
}
