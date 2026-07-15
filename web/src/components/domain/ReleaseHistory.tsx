import { AlertTriangle, ChevronRight, Download, LoaderCircle, RefreshCw, RotateCcw } from "lucide-react";
import { useEffect, useState } from "react";
import { ConflowAPIError, defaultsURL, getRelease, getRemoteProjection, listReleases, type Environment, type Release, type ReleaseSummary, type RemoteProjection } from "../../api/client";
import { Button } from "../ui/Button";

export function ReleaseHistory({ environment, releaseID, onOpenRollback }: { environment: Environment; releaseID?: string; onOpenRollback: (releaseID: string) => void }) {
  const [records, setRecords] = useState<ReleaseSummary[] | null>(null);
  const [detail, setDetail] = useState<Release | null>(null);
  const [snapshot, setSnapshot] = useState<RemoteProjection | null | undefined>(undefined);
  const [snapshotError, setSnapshotError] = useState(false);
  const [snapshotRefresh, setSnapshotRefresh] = useState(0);
  const [error, setError] = useState(false);
  const loadRecords = () => { setError(false); void listReleases(environment.id).then((response) => setRecords(response.data)).catch(() => setError(true)); };
  const refresh = () => { loadRecords(); setSnapshotRefresh((current) => current + 1); };
  useEffect(() => { loadRecords(); }, [environment.id]);
  useEffect(() => {
    const controller = new AbortController();
    setSnapshot(undefined);
    setSnapshotError(false);
    void getRemoteProjection(environment.id, controller.signal)
      .then((response) => setSnapshot(response.data || null))
      .catch((cause) => {
        if (cause instanceof DOMException && cause.name === "AbortError") return;
        if (cause instanceof ConflowAPIError && cause.status === 404) setSnapshot(null);
        else setSnapshotError(true);
      });
    return () => controller.abort();
  }, [environment.id, snapshotRefresh]);
  useEffect(() => {
    if (!releaseID) { setDetail(null); return; }
    const controller = new AbortController();
    void getRelease(environment.id, releaseID, controller.signal).then((response) => setDetail(response.data)).catch(() => setError(true));
    return () => controller.abort();
  }, [environment.id, releaseID]);
  const newestFirst = records?.slice().sort((left, right) => Date.parse(right.created_at) - Date.parse(left.created_at));
  const snapshotUnavailable = snapshot === null;
  const downloadsDisabled = snapshot === undefined || snapshotUnavailable || snapshotError;
  return <main className="page-container releases-page"><header className="page-heading"><div><h1>发布记录</h1><p>{environment.name} · 版本、审计与回滚入口</p></div><Button onClick={refresh} icon={<RefreshCw size={16} />}>刷新</Button></header><section className="defaults-bar"><div><strong>下载默认值</strong><span>从最近一次拉取的线上快照导出客户端默认值。</span>{snapshot ? <span className="defaults-snapshot">快照：<time dateTime={snapshot.observed_at} title={dateTime(snapshot.observed_at)}>{relativeTime(snapshot.observed_at)}</time> · 线上版本 {snapshot.version || "-"}</span> : null}{isSnapshotOld(snapshot) ? <span className="defaults-warning"><AlertTriangle size={14} />快照较旧，建议先拉取最新线上配置</span> : null}{snapshotUnavailable ? <span className="defaults-unavailable">尚无线上快照，请先在概览页拉取远端快照</span> : null}{snapshotError ? <span className="defaults-unavailable">无法读取线上快照信息，请刷新页面重试</span> : null}</div><div>{(["xml", "json", "plist"] as const).map((format) => downloadsDisabled ? <button type="button" disabled key={format}><Download size={15} />{format.toUpperCase()}</button> : <a href={defaultsURL(environment.id, format)} download key={format}><Download size={15} />{format.toUpperCase()}</a>)}</div></section>{error ? <section className="history-empty panel"><h2>无法读取发布记录</h2><Button onClick={refresh}>重试</Button></section> : null}{records === null && !error ? <section className="history-empty panel"><LoaderCircle className="spin" /><p>正在读取发布记录。</p></section> : null}{records?.length === 0 ? <section className="history-empty panel"><h2>此环境还没有发布记录</h2><p>完成发布后，版本、远端状态和审计摘要会显示在这里。</p></section> : null}{newestFirst?.length ? <section className="table-panel release-table"><table><thead><tr><th>时间</th><th>类型</th><th>变更摘要</th><th>Firebase</th><th>结果</th><th><span className="sr-only">查看详情</span></th></tr></thead><tbody>{newestFirst.map((record) => <tr key={record.release_id}><td><button className="row-select-button" onClick={() => { window.location.hash = `releases/${encodeURIComponent(record.release_id)}`; }}>{dateTime(record.created_at)}</button></td><td><ReleaseKind record={record} /></td><td>{record.semantic_summary}</td><td>{record.remote_state === "unknown" ? "待核验" : record.operation_id}</td><td><Outcome record={record} /></td><td><button className="icon-button" aria-label={`查看 ${record.release_id}`} onClick={() => { window.location.hash = `releases/${encodeURIComponent(record.release_id)}`; }}><ChevronRight size={17} /></button></td></tr>)}</tbody></table></section> : null}{releaseID ? <ReleaseDetail detail={detail} onOpenRollback={onOpenRollback} /> : null}</main>;
}

function ReleaseDetail({ detail, onOpenRollback }: { detail: Release | null; onOpenRollback: (releaseID: string) => void }) {
  if (!detail) return <section className="history-detail panel"><LoaderCircle className="spin" /><p>正在读取审计详情。</p></section>;
  return <section className="history-detail panel"><header><div><h2>发布详情</h2><p><code>{detail.release_id}</code> · Operation <code>{detail.operation_id}</code></p></div><Outcome record={detail} /></header><div className="release-detail-grid"><AuditList detail={detail} /><div className="remote-audit-grid"><RemoteAudit title="发布前" remote={detail.remote_before} /><RemoteAudit title="发布后" remote={detail.remote_after} /></div></div>{detail.failure ? <section className="release-failure-detail"><strong>失败信息</strong><p>{detail.failure.message}</p><code>{detail.failure.code} · {detail.failure.stage}</code><p>线上状态：{remoteState(detail.remote_state)}</p></section> : null}<div className="history-detail-actions">{detail.outcome === "succeeded" ? <Button variant="danger" onClick={() => onOpenRollback(detail.release_id)} icon={<RotateCcw size={16} />}>预览回滚</Button> : null}</div></section>;
}

function AuditList({ detail }: { detail: Release }) { return <dl className="audit-list"><div><dt>类型</dt><dd>{detail.kind === "rollback" ? "回滚" : "发布"}</dd></div><div><dt>结果</dt><dd>{detail.outcome === "succeeded" ? "成功" : "失败"}</dd></div><div><dt>线上状态</dt><dd>{remoteState(detail.remote_state)}</dd></div><div><dt>计划</dt><dd><code>{detail.plan_id ?? "-"}</code></dd></div>{detail.rollback_of_release_id ? <div><dt>回滚目标</dt><dd><a href={`#releases/${encodeURIComponent(detail.rollback_of_release_id)}`}><code>{detail.rollback_of_release_id}</code></a></dd></div> : null}<div><dt>来源摘要</dt><dd><code>{detail.source_digest ?? "-"}</code></dd></div><div><dt>计划摘要</dt><dd><code>{detail.plan_digest ?? "-"}</code></dd></div></dl>; }
function RemoteAudit({ title, remote }: { title: string; remote?: Release["remote_before"] }) { return <section><h3>{title}</h3>{remote ? <dl><div><dt>版本</dt><dd>{remote.version}</dd></div><div><dt>参数</dt><dd>{remote.summary.parameter_count}</dd></div><div><dt>受管参数</dt><dd>{remote.summary.managed_parameter_count}</dd></div><div><dt>条件</dt><dd>{remote.summary.condition_count}</dd></div><div><dt>观察时间</dt><dd>{dateTime(remote.observed_at)}</dd></div></dl> : <p>没有可公开的远端快照。</p>}</section>; }
function ReleaseKind({ record }: { record: ReleaseSummary }) { return <span className={record.kind === "rollback" ? "release-kind release-kind--rollback" : "release-kind"}>{record.kind === "rollback" ? "回滚" : "发布"}</span>; }
function Outcome({ record }: { record: Pick<ReleaseSummary, "outcome"> }) { return <span className={record.outcome === "succeeded" ? "release-outcome release-outcome--success" : "release-outcome release-outcome--failure"}><i />{record.outcome === "succeeded" ? "成功" : "失败"}</span>; }
function remoteState(state: Release["remote_state"]) { return ({ unchanged: "线上未变化", changed: "线上已变化", unknown: "线上状态未知，必须核验" } as Record<string, string>)[state]; }
function dateTime(value: string) { try { return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)); } catch { return value; } }
function relativeTime(value: string) { const elapsed = Date.now() - Date.parse(value); if (!Number.isFinite(elapsed) || elapsed < 60_000) return "刚刚"; const minutes = Math.floor(elapsed / 60_000); if (minutes < 60) return `${minutes} 分钟前`; const hours = Math.floor(minutes / 60); if (hours < 24) return `${hours} 小时前`; return `${Math.floor(hours / 24)} 天前`; }
function isSnapshotOld(snapshot: RemoteProjection | null | undefined) { if (!snapshot) return false; const elapsed = Date.now() - Date.parse(snapshot.observed_at); return Number.isFinite(elapsed) && elapsed > 24 * 60 * 60 * 1000; }
