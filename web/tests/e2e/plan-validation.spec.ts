import { expect, test, type Page, type Route } from "@playwright/test";

type Mode = "ready" | "preview" | "invalid" | "routine" | "stale";

async function mockReleaseAPI(page: Page, mode: Mode = "ready") {
  let planRequests = 0;
  let operationReads = 0;
  const planID = mode === "invalid" ? "plan_invalid" : mode === "routine" ? "plan_routine" : mode === "preview" ? "plan_preview" : "plan_ready";
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request(); const url = new URL(request.url()); const path = url.pathname; const method = request.method();
    if (path === "/api/v1/bootstrap") return json(route, bootstrap());
    if (path === "/api/v1/packs/mobile-ad-monetization/versions/v1") return json(route, { data: { ref: "mobile-ad-monetization/v1", name: "mobile-ad-monetization", version: "v1", description: "广告配置", capabilities: [], schema_version: 1, entity_types: [] }, meta: meta() });
    if (path.match(/^\/api\/v1\/drafts\/[^/]+\/diagnostics$/) && method === "GET") {
      if (mode === "ready") return json(route, validation("fresh", "ready"));
      if (mode === "stale") return json(route, validation("stale", "blocked"));
      return json(route, validation("fresh", "blocked"));
    }
    if (path.match(/^\/api\/v1\/drafts\/[^/]+:validate$/) && method === "POST") return json(route, validation("fresh", "ready"));
    if (path.match(/^\/api\/v1\/drafts\/[^/]+:plan$/) && method === "POST") { planRequests += 1; return json(route, operation("running", "queued"), 202); }
    if (path === "/api/v1/operations/op_plan") {
      operationReads += 1;
      if (operationReads < 2) return json(route, operation("running", "reading_remote"));
      if (operationReads < 3) return json(route, operation("running", "compiling"));
      return json(route, operation("succeeded", "completed", planID));
    }
    if (path === `/api/v1/plans/${planID}`) return json(route, { data: plan(planID, mode), meta: meta() });
    if (path === "/api/v1/events") return route.fulfill({ status: 204 });
    // The diagnostic link opens the existing configuration view. These fixtures are only needed to
    // let that page finish its initial data reads after navigation.
    if (path.match(/^\/api\/v1\/packs\/mobile-ad-monetization\/versions\/v1\/schema$/)) return json(route, { data: { version: 1, entities: [], migrations: [] }, meta: meta() });
    if (path.match(/^\/api\/v1\/drafts\/[^/]+$/)) return json(route, { data: { environment_id: "staging", pack_ref: "mobile-ad-monetization/v1", source_revision: "source", dirty: true, dirty_scopes: ["baseline"], baseline: { source: { present: true, value: {} }, draft: { present: true, value: {} }, resolved: { present: true, value: {} }, dirty: true }, environment_override: { source: { present: false }, draft: { present: false }, resolved: { present: false }, dirty: false }, effective: {}, field_states: [], affected_environments: [] }, meta: meta() });
    if (path.includes("/entities") && method === "GET") return json(route, { data: [], meta: meta() });
    return json(route, { error: { code: "route_not_found", message: "missing test route", request_id: "req_missing" } }, 404);
  });
  return { planRequests: () => planRequests };
}

test("校验中心可加载、筛选、跳转并重新校验", async ({ page }) => {
  await mockReleaseAPI(page, "stale");
  await page.goto("/#validation");
  await expect(page.getByRole("heading", { name: "校验中心" })).toBeVisible();
  await expect(page.getByText("结果可能已过期", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "警告", exact: true }).click();
  await expect(page.getByText("频控策略接近风险阈值")).toBeVisible();
  await expect(page.getByText("广告位配置不完整")).toHaveCount(0);
  await page.getByRole("button", { name: "全部" }).click();
  await page.getByText("广告位配置不完整").click();
  await page.getByRole("button", { name: "前往字段修复" }).click();
  await expect(page).toHaveURL(/#configuration\?entity_ref=/);
  await page.goto("/#validation");
  await page.getByRole("button", { name: "重新校验" }).first().click();
  await expect(page.getByText("可查看 staging 的发布计划")).toBeVisible();
});

test("发布计划显示 Operation 进度和三级展开树", async ({ page }) => {
  await mockReleaseAPI(page);
  await page.goto("/#plan");
  await expect(page.getByText("正在构建发布计划")).toBeVisible();
  await expect(page.getByText("读取线上配置", { exact: true })).toBeVisible();
  await expect(page.getByText("频控策略冷却时间")).toBeVisible({ timeout: 7000 });
  await expect(page.getByText("2 项直接修改 · 2 个受影响实体 · 2 个远端参数")).toBeVisible();
  await page.getByRole("button", { name: /频控策略冷却时间/ }).click();
  await page.getByRole("button", { name: /广告位 · ad_interstitial_001/ }).click();
  // ReleasePlan.tsx:118-147 renders this parameter in the expanded semantic tree; the preview table also contains it.
  await expect(page.locator(".semantic-tree").getByText("ad_frequency_inter_global_cap", { exact: true })).toBeVisible();
});

test("预览计划明确不可发布", async ({ page }) => {
  await mockReleaseAPI(page, "preview");
  await page.goto("/#plan");
  await expect(page.getByText("不可发布", { exact: true })).toBeVisible({ timeout: 7000 });
  await expect(page.locator(".plan-status").getByText("线上配置包含未建模条件值", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "发布到 Staging" })).toBeDisabled();
});

test("失效计划保留旧树并可重新构建", async ({ page }) => {
  const mock = await mockReleaseAPI(page, "invalid");
  await page.goto("/#plan");
  await expect(page.locator(".plan-invalidation-banner").getByText("这份发布计划已失效", { exact: true })).toBeVisible({ timeout: 7000 });
  await expect(page.getByText("频控策略冷却时间")).toBeVisible();
  await page.locator(".plan-invalidation-banner").getByRole("button", { name: "重新构建计划" }).click();
  await expect.poll(mock.planRequests).toBe(2);
});

test("routine 失效仅自动重新构建一次，第二次失效转为横幅", async ({ page }) => {
  const mock = await mockReleaseAPI(page, "routine");
  await page.goto("/#plan");
  await expect(page.getByText("正在重新构建计划…", { exact: true })).toBeVisible({ timeout: 7000 });
  await expect.poll(mock.planRequests).toBe(2);
  await expect(page.locator(".plan-invalidation-banner")).toContainText("自动重新构建后的计划仍然失效，请手动重新构建计划。", { timeout: 7000 });
});

test("刷新后从 sessionStorage 恢复计划 Operation", async ({ page }) => {
  const mock = await mockReleaseAPI(page);
  await page.goto("/#plan");
  await expect(page.getByText("正在构建发布计划")).toBeVisible();
  await expect(page.getByText("op_plan", { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByText("频控策略冷却时间")).toBeVisible({ timeout: 7000 });
  expect(mock.planRequests()).toBe(1);
});

function bootstrap() { return { data: { project: { id: "photo-editor", name: "Photo Editor", pack_ref: "mobile-ad-monetization/v1", source_type: "managed-file" }, environments: [{ id: "staging", name: "Staging", kind: "staging", provider: { type: "firebase-remote-config", project_id: "photo-editor-staging" }, publish: { requires_confirmation: false } }], capabilities: { project_edit: true, environment_manage: true } }, meta: meta() }; }
function meta() { return { request_id: "req_spec_015", revision: 13 }; }
function operation(status: string, stage: string, planID?: string) { return { data: { operation_id: "op_plan", operation_type: "plan", status, stage, remote_state: "unchanged", created_at: "2026-07-11T10:00:00Z", updated_at: "2026-07-11T10:00:02Z", ...(planID ? { result: { resource_type: "plan", resource_id: planID, href: `/api/v1/plans/${planID}` } } : {}) }, meta: meta() }; }
function validation(status: "fresh" | "stale", readiness: "ready" | "blocked") { return { data: { environment_id: "staging", validated_draft_revision: 13, validated_at: "2026-07-11T09:30:00Z", status, readiness, diagnostics: [{ code: "placement_missing", path: "/placements/ad_interstitial_001/enabled", severity: "error", message: "广告位配置不完整", entity_ref: "entity:mobile-ad-monetization/v1:placement:ad_interstitial_001", fix_suggestion: "补齐广告位配置后重新校验。" }, { code: "policy_warning", path: "/frequency_policies/inter_global_cap", severity: "warning", message: "频控策略接近风险阈值", entity_ref: "entity:mobile-ad-monetization/v1:frequency_policy:inter_global_cap", fix_suggestion: "确认频控策略。" }] }, meta: meta() }; }
function plan(planID: string, mode: Mode) { const preview = mode === "preview"; const invalid = mode === "invalid" || mode === "routine"; return { plan_id: planID, environment_id: "staging", status: invalid ? "invalidated" : preview ? "preview_only" : "ready", snapshot_token: "snapshot", draft_revision: 13, source_digest: "sha256:source", remote_etag: "etag-57", created_at: "2026-07-11T10:00:00Z", expires_at: "2026-07-11T10:15:00Z", ...(mode === "invalid" ? { invalidation_reason: "remote_etag_changed" } : mode === "routine" ? { invalidation_reason: "ttl_expired", invalidation: { code: "plan_expired", tier: "routine", message: "计划已过期，请重新构建。" } } : {}), content_digest: "sha256:plan", remote_snapshot: { status: "available", remote_etag: "etag-57" }, semantic_changes: [{ node_id: "semantic_frequency", change_kind: "updated", summary: "频控策略冷却时间", direct_entity_ref: "entity:mobile-ad-monetization/v1:frequency_policy:inter_global_cap", before_summary: "30 秒", after_summary: "120 秒", affected_entity_node_ids: ["entity_one", "entity_two"], remote_parameter_node_ids: ["remote_one", "remote_two"] }, { node_id: "semantic_switch", change_kind: "updated", summary: "功能开关已启用", direct_entity_ref: "entity:mobile-ad-monetization/v1:feature_switch:use_amazon_bidding", before_summary: "关闭", after_summary: "开启", affected_entity_node_ids: [], remote_parameter_node_ids: [] }], affected_entities: [{ node_id: "entity_one", entity_ref: "entity:mobile-ad-monetization/v1:placement:ad_interstitial_001", entity_type: "placement", entity_id: "ad_interstitial_001", impact_kind: "referenced", caused_by_semantic_change_ids: ["semantic_frequency"] }, { node_id: "entity_two", entity_ref: "entity:mobile-ad-monetization/v1:placement:ad_interstitial_002", entity_type: "placement", entity_id: "ad_interstitial_002", impact_kind: "referenced", caused_by_semantic_change_ids: ["semantic_frequency"] }], remote_parameter_changes: [{ node_id: "remote_one", parameter_key: "ad_frequency_inter_global_cap", change_kind: "updated", before_summary: "30 秒", after_summary: "120 秒", managed: true, caused_by_semantic_change_ids: ["semantic_frequency"], affected_entity_node_ids: ["entity_one"] }, { node_id: "remote_two", parameter_key: "ad_frequency_inter_global_cap_backup", change_kind: "updated", before_summary: "30 秒", after_summary: "120 秒", managed: true, caused_by_semantic_change_ids: ["semantic_frequency"], affected_entity_node_ids: ["entity_two"] }], artifact_metadata: [{ artifact_name: "review.json", media_type: "application/json", content_digest: "sha256:review", size_bytes: 100, sensitive: false, available: true }, { artifact_name: "review.md", media_type: "text/markdown", content_digest: "sha256:review-md", size_bytes: 100, sensitive: false, available: true }], severity: preview ? "blocking" : "high", risk_items: preview ? [{ risk_item_id: "risk_conditions", severity: "blocking", reason_code: "unmodeled_remote_condition", summary: "线上配置包含未建模条件值", acknowledgement_required: true, semantic_change_ids: [], remote_parameter_node_ids: [] }] : [{ risk_item_id: "risk_frequency", severity: "high", reason_code: "shared_frequency_policy_relaxed", summary: "共享频控策略变更会影响多个广告位", acknowledgement_required: true, semantic_change_ids: ["semantic_frequency"], remote_parameter_node_ids: ["remote_one"] }], blocking_reasons: preview ? [{ reason_code: "unmodeled_remote_condition", summary: "线上配置包含未建模条件值", risk_item_id: "risk_conditions" }] : [], confirmation_requirements: { requires_acknowledgement: true, environment_id_requirement: "not_required", required_risk_item_ids: [], policy_source: "project.release_confirmation_policy" } }; }
async function json(route: Route, body: unknown, status = 200) { await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) }); }
