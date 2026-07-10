import { ChevronDown, Layers3, Menu, X } from "lucide-react";
import { useLayoutEffect, useRef, useState, type RefObject } from "react";
import type { Environment, Project } from "../../api/client";

export type Page = "overview" | "environments" | "project";

const disabledNavigation = ["配置", "校验", "发布计划", "发布记录"];

export function AppTopBar({ project, environments, selectedEnvironment, page, environmentSelectRef, onEnvironmentChange, onPageChange }: {
  project: Project;
  environments: Environment[];
  selectedEnvironment: Environment;
  page: Page;
  environmentSelectRef: RefObject<HTMLSelectElement | null>;
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
        <OverflowProjectName name={project.name} />
        <span className="context-separator">/</span>
        <select ref={environmentSelectRef} className={production ? "context-environment context-environment--production" : "context-environment"} aria-label="切换环境" value={selectedEnvironment.id} onChange={(event) => onEnvironmentChange(event.target.value)}>
          {environments.map((environment) => <option value={environment.id} key={environment.id}>{environment.name}</option>)}
        </select>
        {production ? <span className="sr-only" data-testid="production-marker">Production 环境</span> : null}
        <ChevronDown size={14} aria-hidden="true" />
      </label>
      <nav className="desktop-nav" aria-label="主导航">
        <NavButton active={page === "overview"} onClick={onPageChange} />
        {disabledNavigation.map((label) => <span key={label} className="nav-disabled" aria-disabled="true" title="后续 Spec 接入">{label}</span>)}
      </nav>
      <div className="draft-slot" aria-label="未发布修改状态">未发布修改：尚未接入</div>
      <button className="mobile-menu-button" aria-label={menuOpen ? "关闭导航" : "打开导航"} aria-expanded={menuOpen} onClick={() => setMenuOpen((value) => !value)}>
        {menuOpen ? <X /> : <Menu />}
      </button>
      {menuOpen ? <nav className="mobile-nav" aria-label="窄屏导航">
        <NavButton active={page === "overview"} onClick={(next) => { onPageChange(next); setMenuOpen(false); }} />
        {disabledNavigation.map((label) => <span key={label} className="nav-disabled" aria-disabled="true" title="后续 Spec 接入">{label}</span>)}
      </nav> : null}
    </header>
  );
}

function OverflowProjectName({ name }: { name: string }) {
  const elementRef = useRef<HTMLSpanElement>(null);
  const [truncated, setTruncated] = useState(false);

  useLayoutEffect(() => {
    const element = elementRef.current;
    if (!element) return;
    const update = () => setTruncated(element.scrollWidth > element.clientWidth);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [name]);

  return <span ref={elementRef} className="context-project" title={truncated ? name : undefined}>{name}</span>;
}

function NavButton({ active, onClick }: { active: boolean; onClick: (page: Page) => void }) {
  return <button className={active ? "nav-button nav-button--active" : "nav-button"} aria-current={active ? "page" : undefined} onClick={() => onClick("overview")}>概览</button>;
}
