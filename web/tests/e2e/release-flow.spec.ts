import { expect, test, type Page, type Route } from "@playwright/test";

type Scenario = "success" | "conflict" | "unknown_failure" | "restore" | "rollback" | "no_snapshot";

test("正常发布经过审阅、风险确认和 Operation 成功页", async ({ page }) => {
  const api = await mockReleaseFlow(page, "success");
  await page.goto("/#release/plan_release");
  await expect(page.getByRole("heading", { name: "发布到 Production" })).toBeVisible();
  await page.getByRole("button", { name: "继续确认风险" }).click();
  const submit = page.getByRole("button", { name: "发布到 Production" });
  await expect(submit).toBeDisabled();
  await page.getByLabel("确认风险项目").check();
  await page.getByLabel("我已审阅影响范围和风险").check();
  await page.getByLabel("输入环境 ID 以确认").fill("production");
  await expect(submit).toBeEnabled();
  await submit.click();
  await expect(page.getByText("正在发布到 Production")).toBeVisible();
  await expect(page.getByRole("heading", { name: "Production 发布成功" })).toBeVisible({ timeout: 5000 });
  expect(api.releaseRequests()).toBe(1);
  expect(api.idempotencyKey()).toMatch(/.+/);
});

test("必填风险未逐项确认时不能发布", async ({ page }) => {
  await mockReleaseFlow(page, "success");
  await page.goto("/#release/plan_release");
  await page.getByRole("button", { name: "继续确认风险" }).click();
  await page.getByLabel("我已审阅影响范围和风险").check();
  await page.getByLabel("输入环境 ID 以确认").fill("production");
  await expect(page.getByRole("button", { name: "发布到 Production" })).toBeDisabled();
});

test("环境 ID 不匹配时不能发布", async ({ page }) => {
  await mockReleaseFlow(page, "success");
  await page.goto("/#release/plan_release");
  await page.getByRole("button", { name: "继续确认风险" }).click();
  await page.getByLabel("确认风险项目").check();
  await page.getByLabel("我已审阅影响范围和风险").check();
  await page.getByLabel("输入环境 ID 以确认").fill("Production");
  await expect(page.getByRole("button", { name: "发布到 Production" })).toBeDisabled();
});

test("412 remote_etag_mismatch 展示当前摘要并强制回到 Plan 重建", async ({ page }) => {
  await mockReleaseFlow(page, "conflict");
  await page.goto("/#release/plan_release");
  await page.getByRole("button", { name: "继续确认风险" }).click();
  await confirmRelease(page);
  await page.getByRole("button", { name: "发布到 Production" }).click();
  await expect(page.getByRole("heading", { name: "线上配置已变化" })).toBeVisible();
  await expect(page.getByText("当前线上摘要")).toBeVisible();
  await page.getByRole("button", { name: "重新构建计划" }).click();
  await expect(page).toHaveURL(/#plan\?rebuild=1/);
});

test("Operation 失败明确展示未知线上状态与核验指引", async ({ page }) => {
  await mockReleaseFlow(page, "unknown_failure");
  await page.goto("/#release/plan_release");
  await page.getByRole("button", { name: "继续确认风险" }).click();
  await confirmRelease(page);
  await page.getByRole("button", { name: "发布到 Production" }).click();
  await expect(page.getByRole("heading", { name: "发布未完成" })).toBeVisible({ timeout: 5000 });
  await expect(page.getByText("线上现在是什么状态？")).toBeVisible();
  await expect(page.getByText("下一步做什么？")).toBeVisible();
  await expect(page.getByText("线上状态未知，必须核验")).toBeVisible();
  await expect(page.getByText("先在 Firebase 核验线上配置；结果不确定时不能盲目重试。")).toBeVisible();
  await expect(page.getByRole("button", { name: "重试" })).toHaveCount(0);
});

test("刷新后按已保存 Operation 恢复，不会重复创建发布", async ({ page }) => {
  const api = await mockReleaseFlow(page, "restore");
  await page.addInitScript(() => sessionStorage.setItem("conflow.release.operation.production", JSON.stringify({ operationID: "op_release", resourceID: "plan_release", action: "publish", idempotencyKey: "retry-key" })));
  await page.goto("/#release/plan_release");
  await expect(page.getByRole("heading", { name: "Production 发布成功" })).toBeVisible({ timeout: 5000 });
  expect(api.releaseRequests()).toBe(0);
});

test("发布记录倒序、默认值下载、回滚目标链接和回滚全链", async ({ page }) => {
  const api = await mockReleaseFlow(page, "rollback");
  await page.goto("/#releases");
  await expect(page.getByRole("heading", { name: "发布记录" })).toBeVisible();
  await expect(page.locator("tbody tr").first()).toContainText("最新发布");
  await expect(page.getByText("线上版本 128")).toBeVisible();
  await expect(page.getByText("快照：刚刚")).toBeVisible();
  await expect(page.getByRole("link", { name: "XML" })).toHaveAttribute("href", /defaults\?format=xml/);
  await expect(page.getByRole("link", { name: "JSON" })).toHaveAttribute("href", /defaults\?format=json/);
  await expect(page.getByRole("link", { name: "PLIST" })).toHaveAttribute("href", /defaults\?format=plist/);
  await page.getByRole("button", { name: "查看 rel_rollback" }).click();
  await expect(page.getByRole("link", { name: "rel_publish" })).toHaveAttribute("href", "#releases/rel_publish");
  await page.getByRole("button", { name: "预览回滚" }).click();
  await expect(page.getByRole("heading", { name: "回滚预览", exact: true })).toBeVisible({ timeout: 5000 });
  await page.getByRole("button", { name: "继续确认回滚" }).click();
  await page.getByLabel("我已审阅影响范围和风险").check();
  await page.getByLabel("输入环境 ID 以确认").fill("production");
  await page.getByRole("button", { name: "回滚到版本 127" }).click();
  await expect(page.getByRole("heading", { name: "Production 回滚成功" })).toBeVisible({ timeout: 5000 });
  expect(api.rollbackRequests()).toBe(1);
});

test("没有线上快照时禁用默认值下载", async ({ page }) => {
  await mockReleaseFlow(page, "no_snapshot");
  await page.goto("/#releases");
  await expect(page.getByText("尚无线上快照，请先在概览页拉取远端快照")).toBeVisible();
  await expect(page.getByRole("button", { name: "XML" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "JSON" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "PLIST" })).toBeDisabled();
});

async function confirmRelease(page: Page) {
  await page.getByLabel("确认风险项目").check();
  await page.getByLabel("我已审阅影响范围和风险").check();
  await page.getByLabel("输入环境 ID 以确认").fill("production");
}

async function mockReleaseFlow(page: Page, scenario: Scenario) {
  let releaseRequests = 0;
  let rollbackRequests = 0;
  let lastIdempotencyKey = "";
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    if (path === "/api/v1/bootstrap") return json(route, bootstrap());
    if (path === "/api/v1/environments/production/remote/projection") return scenario === "no_snapshot" ? json(route, { error: { code: "remote_snapshot_not_found", message: "远端快照不可用", request_id: "req_snapshot_missing" } }, 404) : json(route, { data: projection(), meta: meta() });
    if (path === "/api/v1/packs/mobile-ad-monetization/versions/v1") return json(route, { data: { ref: "mobile-ad-monetization/v1", name: "mobile-ad-monetization", version: "v1", description: "广告配置", capabilities: [], schema_version: 1, entity_types: [] }, meta: meta() });
    if (path.endsWith("/diagnostics")) return json(route, { data: { environment_id: "production", validated_draft_revision: 21, validated_at: "2026-07-12T08:00:00Z", status: "fresh", readiness: "ready", diagnostics: [] }, meta: meta() });
    if (path === "/api/v1/plans/plan_release") return json(route, { data: releasePlan(), meta: meta() });
    if (path === "/api/v1/operations/op_release") {
      if (scenario === "unknown_failure") return json(route, operation("op_release", "failed", "submitting", "unknown"));
      return json(route, operation("op_release", "succeeded", "completed", "changed", "release", scenario === "rollback" ? "rel_rolled_back" : "rel_new"));
    }
    if (path === "/api/v1/environments/production/releases" && method === "POST") {
      releaseRequests += 1;
      lastIdempotencyKey = (await request.headerValue("idempotency-key")) ?? "";
      expect((await request.headerValue("content-type")) ?? "").toContain("application/json");
      if (scenario === "conflict") return json(route, remoteMismatch(), 412);
      return json(route, operation("op_release", "running", "queued", "unchanged"), 202);
    }
    if (path === "/api/v1/environments/production/releases/rel_new") return json(route, { data: release("rel_new", "publish", "succeeded", "changed"), meta: meta() });
    if (path === "/api/v1/environments/production/releases/rel_rolled_back") return json(route, { data: release("rel_rolled_back", "rollback", "succeeded", "changed", "rel_publish"), meta: meta() });
    if (path === "/api/v1/environments/production/releases" && method === "GET") return json(route, { data: [summary("rel_publish", "publish", "2026-07-11T08:00:00Z", "旧发布"), summary("rel_failed", "publish", "2026-07-11T09:00:00Z", "失败发布", "failed"), summary("rel_rollback", "rollback", "2026-07-12T10:00:00Z", "最新发布")], meta: meta() });
    if (path === "/api/v1/environments/production/releases/rel_rollback") return json(route, { data: release("rel_rollback", "rollback", "succeeded", "changed", "rel_publish"), meta: meta() });
    if (path === "/api/v1/environments/production/releases/rel_rollback:rollback-preview" && method === "POST") {
      expect(await request.headerValue("content-type")).toBe("application/json");
      return json(route, operation("op_preview", "running", "queued", "unchanged"), 202);
    }
    if (path === "/api/v1/operations/op_preview") return json(route, operation("op_preview", "succeeded", "completed", "unchanged", "rollback_preview", "rbp_ready"));
    if (path === "/api/v1/environments/production/releases/rel_rollback/rollback-preview") return json(route, { data: rollbackPreview(), meta: meta() });
    if (path === "/api/v1/environments/production/releases/rel_rollback:rollback" && method === "POST") {
      rollbackRequests += 1;
      lastIdempotencyKey = (await request.headerValue("idempotency-key")) ?? "";
      return json(route, operation("op_rollback", "succeeded", "completed", "changed", "release", "rel_rolled_back"), 202);
    }
    if (path === "/api/v1/operations/op_rollback") return json(route, operation("op_rollback", "succeeded", "completed", "changed", "release", "rel_rolled_back"));
    if (path === "/api/v1/events") return route.fulfill({ status: 204 });
    return json(route, { error: { code: "route_not_found", message: "missing test route", request_id: "req_missing" } }, 404);
  });
  return { releaseRequests: () => releaseRequests, rollbackRequests: () => rollbackRequests, idempotencyKey: () => lastIdempotencyKey };
}

function bootstrap() { return { data: { project: { id: "photo-editor", name: "Photo Editor", pack_ref: "mobile-ad-monetization/v1", source_type: "managed-file" }, environments: [{ id: "production", name: "Production", kind: "production", provider: { type: "firebase-remote-config", project_id: "photo-editor-prod" }, publish: { requires_confirmation: true } }], capabilities: { project_edit: true, environment_manage: true } }, meta: meta() }; }
function meta() { return { request_id: "req_release_ui", revision: 21 }; }
function remote() { return { remote_etag: "etag-current", version: "128", observed_at: "2026-07-12T10:00:00Z", summary: { parameter_count: 12, managed_parameter_count: 9, condition_count: 0, content_digest: "sha256:remote" } }; }
function projection() { return { environment_id: "production", snapshot_etag: "etag-current", version: "128", observed_at: new Date().toISOString(), projections: [] }; }
function releasePlan() { return { plan_id: "plan_release", environment_id: "production", status: "ready", snapshot_token: "snapshot", draft_revision: 21, source_digest: "sha256:source", remote_etag: "etag-plan", created_at: "2026-07-12T10:00:00Z", expires_at: "2026-07-12T10:15:00Z", content_digest: "sha256:plan", remote_snapshot: { status: "available", remote_etag: "etag-plan", version: "127", observed_at: "2026-07-12T10:00:00Z", summary: remote().summary }, semantic_changes: [], affected_entities: [], remote_parameter_changes: [], artifact_metadata: [], severity: "high", risk_items: [{ risk_item_id: "risk_release", severity: "high", reason_code: "global_feature_switch_changed", summary: "确认风险项目", acknowledgement_required: true, semantic_change_ids: [], remote_parameter_node_ids: [] }], blocking_reasons: [], confirmation_requirements: { requires_acknowledgement: true, environment_id_requirement: "required", required_risk_item_ids: ["risk_release"], policy_source: "project.release_confirmation_policy" } }; }
function rollbackPreview() { return { rollback_preview_id: "rbp_ready", environment_id: "production", target_release_id: "rel_rollback", target_remote_version: "127", status: "ready", expected_remote_etag: "etag-current", created_at: "2026-07-12T10:00:00Z", expires_at: "2026-07-12T10:15:00Z", current_remote: remote(), semantic_changes: [], affected_entities: [], remote_parameter_changes: [], artifact_metadata: [], severity: "low", risk_items: [], blocking_reasons: [], confirmation_requirements: { requires_acknowledgement: true, environment_id_requirement: "required", required_risk_item_ids: [], policy_source: "project.release_confirmation_policy" } }; }
function operation(id: string, status: string, stage: string, remoteState: string, resourceType?: string, resourceID?: string) { return { data: { operation_id: id, operation_type: id === "op_preview" ? "rollback_preview" : id === "op_rollback" ? "rollback" : "publish", status, stage, remote_state: remoteState, created_at: "2026-07-12T10:00:00Z", updated_at: "2026-07-12T10:00:01Z", ...(status === "failed" ? { failure: { code: "provider_response_unknown", message: "发布结果未确认", retryable: false, stage } } : {}), ...(resourceType && resourceID ? { result: { resource_type: resourceType, resource_id: resourceID, href: `/api/v1/resource/${resourceID}` } } : {}) }, meta: meta() }; }
function summary(id: string, kind: string, createdAt: string, semanticSummary: string, outcome = "succeeded") { return { release_id: id, environment_id: "production", kind, outcome, created_at: createdAt, operation_id: `op_${id}`, remote_state: outcome === "failed" ? "unknown" : "changed", semantic_summary: semanticSummary, risk_summary: "low" }; }
function release(id: string, kind: string, outcome: string, remoteState: string, rollbackOf?: string) { return { ...summary(id, kind, "2026-07-12T10:00:00Z", kind === "rollback" ? "已回滚" : "已发布", outcome), remote_state: remoteState, ...(rollbackOf ? { rollback_of_release_id: rollbackOf } : {}), plan_id: "plan_release", source_digest: "sha256:source", plan_digest: "sha256:plan", remote_before: remote(), remote_after: remote() }; }
function remoteMismatch() { return { error: { code: "remote_etag_mismatch", message: "remote changed", request_id: "req_conflict", plan_id: "plan_release", expected_remote_etag: "etag-plan", current_remote: remote(), rebuild: { required: true, plan_endpoint: "/api/v1/drafts/production:plan", reason_code: "remote_etag_changed" } } }; }
async function json(route: Route, body: unknown, status = 200) { await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) }); }
