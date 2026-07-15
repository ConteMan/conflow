import { ChevronDown, Layers3, Menu, X } from "lucide-react";
import { useLayoutEffect, useRef, useState, type RefObject } from "react";
import type { Environment, Project, ValidationResult } from "../../api/client";

export type Page = "overview" | "configuration" | "environments" | "project" | "validation" | "plan" | "release" | "releases" | "rollback";

export function AppTopBar({ project, environments, selectedEnvironment, page, draftDirty, validation, environmentSelectRef, onEnvironmentChange, onPageChange }: {
  project: Project;
  environments: Environment[];
  selectedEnvironment: Environment;
  page: Page;
  draftDirty: boolean | null;
  validation: ValidationResult | null;
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
        <NavButton label="概览" page="overview" active={page === "overview"} onClick={onPageChange} />
        <NavButton label="配置" page="configuration" active={page === "configuration"} onClick={onPageChange} />
        <NavButton label="校验" page="validation" active={page === "validation"} onClick={onPageChange} />
        <NavButton label="发布计划" page="plan" active={page === "plan"} onClick={onPageChange} />
        <NavButton label="发布记录" page="releases" active={page === "releases" || page === "rollback"} onClick={onPageChange} />
      </nav>
      <div className="topbar-statuses">
        {validation ? <button className={`validation-global-badge validation-global-badge--${validation.readiness} validation-global-badge--${validation.status}`} onClick={() => onPageChange("validation")} aria-label="查看校验结果">{validation.status === "stale" ? "校验结果可能过期" : validation.readiness === "ready" ? "校验通过" : `${validation.diagnostics.length} 项校验问题`}</button> : null}
        <div className={draftDirty ? "draft-slot draft-slot--dirty" : "draft-slot"} aria-label="未发布修改状态">{draftDirty === null ? "正在获取草稿状态" : draftDirty ? "有未发布修改" : "尚未有修改"}</div>
      </div>
      <button className="mobile-menu-button" aria-label={menuOpen ? "关闭导航" : "打开导航"} aria-expanded={menuOpen} onClick={() => setMenuOpen((value) => !value)}>
        {menuOpen ? <X /> : <Menu />}
      </button>
      {menuOpen ? <nav className="mobile-nav" aria-label="窄屏导航">
        <NavButton label="概览" page="overview" active={page === "overview"} onClick={(next) => { onPageChange(next); setMenuOpen(false); }} />
        <NavButton label="配置" page="configuration" active={page === "configuration"} onClick={(next) => { onPageChange(next); setMenuOpen(false); }} />
        <NavButton label="校验" page="validation" active={page === "validation"} onClick={(next) => { onPageChange(next); setMenuOpen(false); }} />
        <NavButton label="发布计划" page="plan" active={page === "plan"} onClick={(next) => { onPageChange(next); setMenuOpen(false); }} />
        <NavButton label="发布记录" page="releases" active={page === "releases" || page === "rollback"} onClick={(next) => { onPageChange(next); setMenuOpen(false); }} />
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

function NavButton({ label, page, active, onClick }: { label: string; page: Page; active: boolean; onClick: (page: Page) => void }) {
  return <button className={active ? "nav-button nav-button--active" : "nav-button"} aria-current={active ? "page" : undefined} onClick={() => onClick(page)}>{label}</button>;
}
