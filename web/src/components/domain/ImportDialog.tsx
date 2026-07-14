import { AlertTriangle, CheckCircle, LoaderCircle, Upload } from "lucide-react";
import { useRef, useState } from "react";
import {
  applyImport,
  previewImport,
  type ApplyResult,
  type DecisionRequired,
  type EntityAction,
  type ImportBundle,
  type ImportDecision,
  type PreviewResult,
} from "../../api/client";
import { Button } from "../ui/Button";
import { Modal } from "../ui/Dialog";

type Step = "upload" | "preview" | "decisions" | "result";

export function ImportDialog({
  environmentId,
  open,
  onClose,
  onSuccess,
}: {
  environmentId: string;
  open: boolean;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [step, setStep] = useState<Step>("upload");
  const [bundle, setBundle] = useState<ImportBundle | null>(null);
  const [parseError, setParseError] = useState<string | null>(null);
  const [conflictMode, setConflictMode] = useState<"replace" | "merge" | "skip">("replace");
  const [previewing, setPreviewing] = useState(false);
  const [previewResult, setPreviewResult] = useState<PreviewResult | null>(null);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [decisions, setDecisions] = useState<Record<string, string>>({});
  const [applying, setApplying] = useState(false);
  const [applyResult, setApplyResult] = useState<ApplyResult | null>(null);
  const [applyError, setApplyError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const reset = () => {
    setStep("upload");
    setBundle(null);
    setParseError(null);
    setConflictMode("replace");
    setPreviewing(false);
    setPreviewResult(null);
    setPreviewError(null);
    setDecisions({});
    setApplying(false);
    setApplyResult(null);
    setApplyError(null);
    if (fileInputRef.current) fileInputRef.current.value = "";
  };

  const handleClose = () => { reset(); onClose(); };

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    setParseError(null);
    setBundle(null);
    const reader = new FileReader();
    reader.onload = (e) => {
      try {
        const parsed = JSON.parse(e.target?.result as string) as ImportBundle;
        if (typeof parsed.format_version !== "number" || !parsed.entities) {
          setParseError("文件格式不合法：缺少 format_version 或 entities 字段。");
          return;
        }
        setBundle(parsed);
      } catch {
        setParseError("无法解析 JSON 文件，请检查文件格式。");
      }
    };
    reader.readAsText(file);
  };

  const handlePreview = async () => {
    if (!bundle) return;
    setPreviewing(true);
    setPreviewError(null);
    try {
      const response = await previewImport(environmentId, bundle, conflictMode);
      setPreviewResult(response.data);
      setStep("preview");
    } catch (cause) {
      setPreviewError(cause instanceof Error ? cause.message : "预览失败，请重试。");
    } finally {
      setPreviewing(false);
    }
  };

  const handleApply = async () => {
    if (!bundle || !previewResult) return;
    setApplying(true);
    setApplyError(null);
    const decisionList: ImportDecision[] = Object.entries(decisions).map(([key, value]) => ({ key, value }));
    try {
      const result = await applyImport(environmentId, bundle, previewResult.preview_token, decisionList, conflictMode);
      setApplyResult(result);
      setStep("result");
    } catch (cause) {
      setApplyError(cause instanceof Error ? cause.message : "导入失败，请重试。");
      setStep("result");
    } finally {
      setApplying(false);
    }
  };

  const entityCount = bundle
    ? Object.values(bundle.entities).reduce((sum, arr) => sum + arr.length, 0)
    : 0;
  const entityTypeCount = bundle ? Object.keys(bundle.entities).length : 0;

  const stepTitle: Record<Step, string> = {
    upload: "导入配置",
    preview: "预览导入效果",
    decisions: "填写必填决策",
    result: applyResult ? "导入成功" : "导入失败",
  };

  return (
    <Modal open={open} onOpenChange={(o) => { if (!o) handleClose(); }} title={stepTitle[step]}>
      {step === "upload" && (
        <div className="import-dialog-body">
          <label className="import-upload-area" aria-label="选择 Bundle 文件">
            <Upload size={24} aria-hidden="true" />
            <p>点击选择或拖拽 Bundle JSON 文件</p>
            <input
              ref={fileInputRef}
              type="file"
              accept=".json"
              className="sr-only"
              onChange={handleFileChange}
            />
          </label>
          {parseError && <p className="binding-error" role="alert">{parseError}</p>}
          {bundle && (
            <dl className="import-bundle-info">
              <div><dt>Pack</dt><dd><code>{bundle.pack_ref}</code></dd></div>
              <div><dt>Schema 版本</dt><dd>{bundle.schema_version}</dd></div>
              <div><dt>实体</dt><dd>{entityTypeCount} 种，共 {entityCount} 条</dd></div>
              {(bundle.decisions_required?.length ?? 0) > 0 && (
                <div><dt>必填决策</dt><dd>{bundle.decisions_required!.length} 项</dd></div>
              )}
            </dl>
          )}
          <label className="form-field">
            <span>冲突模式</span>
            <select
              aria-label="冲突模式"
              value={conflictMode}
              onChange={(e) => setConflictMode(e.target.value as "replace" | "merge" | "skip")}
            >
              <option value="replace">Replace — 覆盖已有同 ID 实体</option>
              <option value="merge">Merge — 仅写入不存在的实体</option>
              <option value="skip">Skip — 跳过所有冲突</option>
            </select>
          </label>
          {previewError && <p className="binding-error" role="alert">{previewError}</p>}
          <footer className="dialog-actions">
            <Button onClick={handleClose}>取消</Button>
            <Button
              variant="primary"
              disabled={!bundle || previewing}
              icon={previewing ? <LoaderCircle className="spin" size={16} /> : undefined}
              onClick={() => void handlePreview()}
            >
              {previewing ? "正在预览" : "预览"}
            </Button>
          </footer>
        </div>
      )}

      {step === "preview" && previewResult && (
        <div className="import-dialog-body">
          <div className="import-plan">
            <EntityPlanSection label="新增" items={previewResult.entity_plan.to_add} variant="add" />
            <EntityPlanSection label="替换" items={previewResult.entity_plan.to_replace} variant="replace" />
            <EntityPlanSection label="跳过" items={previewResult.entity_plan.to_skip} variant="skip" />
            <EntityPlanSection label="保留" items={previewResult.entity_plan.to_keep} variant="skip" />
          </div>
          {(previewResult.risks?.length ?? 0) > 0 && (
            <div className="import-risks">
              <div className="danger-callout">
                <AlertTriangle size={18} aria-hidden="true" />
                <strong>注意风险</strong>
              </div>
              <ul>{previewResult.risks!.map((risk, i) => <li key={i}>{risk}</li>)}</ul>
            </div>
          )}
          {(previewResult.decisions_required?.length ?? 0) > 0 && (
            <p className="import-decisions-hint">
              需填写 {previewResult.decisions_required.length} 项决策才能应用。
            </p>
          )}
          {applyError && <p className="binding-error" role="alert">{applyError}</p>}
          <footer className="dialog-actions">
            <Button onClick={() => setStep("upload")}>返回</Button>
            {(previewResult.decisions_required?.length ?? 0) > 0 ? (
              <Button variant="primary" onClick={() => { setApplyError(null); setStep("decisions"); }}>
                填写决策
              </Button>
            ) : (
              <Button
                variant="primary"
                disabled={applying}
                icon={applying ? <LoaderCircle className="spin" size={16} /> : undefined}
                onClick={() => void handleApply()}
              >
                {applying ? "正在应用" : "应用"}
              </Button>
            )}
          </footer>
        </div>
      )}

      {step === "decisions" && previewResult && (
        <div className="import-dialog-body">
          <div className="import-decisions">
            {previewResult.decisions_required.map((d) => (
              <DecisionField
                key={d.key}
                decision={d}
                value={decisions[d.key] ?? ""}
                onChange={(value) => setDecisions((prev) => ({ ...prev, [d.key]: value }))}
              />
            ))}
          </div>
          {applyError && <p className="binding-error" role="alert">{applyError}</p>}
          <footer className="dialog-actions">
            <Button onClick={() => { setApplyError(null); setStep("preview"); }}>返回</Button>
            <Button
              variant="primary"
              disabled={applying}
              icon={applying ? <LoaderCircle className="spin" size={16} /> : undefined}
              onClick={() => void handleApply()}
            >
              {applying ? "正在应用" : "应用"}
            </Button>
          </footer>
        </div>
      )}

      {step === "result" && (
        <div className="import-dialog-body">
          {applyResult ? (
            <div className="import-result import-result--success">
              <CheckCircle size={32} aria-hidden="true" />
              <p>
                {applyResult.applied_count} 条实体已导入
                {applyResult.skipped_count > 0 ? `，${applyResult.skipped_count} 条已跳过` : ""}。
              </p>
              <p className="import-result-revision">修订版本 {applyResult.revision}</p>
            </div>
          ) : (
            <div className="import-result import-result--error">
              <AlertTriangle size={32} aria-hidden="true" />
              <p>{applyError ?? "导入失败，请重试。"}</p>
            </div>
          )}
          <footer className="dialog-actions">
            <Button
              variant={applyResult ? "primary" : "secondary"}
              onClick={applyResult ? () => { reset(); onSuccess(); } : handleClose}
            >
              关闭
            </Button>
          </footer>
        </div>
      )}
    </Modal>
  );
}

function EntityPlanSection({
  label,
  items,
  variant,
}: {
  label: string;
  items: EntityAction[];
  variant: "add" | "replace" | "skip";
}) {
  if (items.length === 0) return null;
  return (
    <section className={`import-plan-section import-plan-section--${variant}`}>
      <header>
        <strong>{label}</strong>
        <span>{items.length} 条</span>
      </header>
      <ul>
        {items.map((item) => (
          <li key={`${item.entity_type}:${item.id}`}>
            <code>{item.entity_type}</code>
            <span>{item.id}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function DecisionField({
  decision,
  value,
  onChange,
}: {
  decision: DecisionRequired;
  value: string;
  onChange: (value: string) => void;
}) {
  const enumValues =
    decision.hint?.includes(" / ")
      ? decision.hint.split(" / ").map((v) => v.trim())
      : null;

  return (
    <div className="import-decision-item">
      <label className="form-field">
        <span><code>{decision.key}</code></span>
        <small>{decision.reason}</small>
        {enumValues ? (
          <select
            aria-label={decision.key}
            value={value}
            onChange={(e) => onChange(e.target.value)}
          >
            <option value="">请选择</option>
            {enumValues.map((v) => (
              <option key={v} value={v}>{v}</option>
            ))}
          </select>
        ) : (
          <input
            aria-label={decision.key}
            placeholder={decision.hint ?? ""}
            value={value}
            onChange={(e) => onChange(e.target.value)}
          />
        )}
      </label>
    </div>
  );
}
