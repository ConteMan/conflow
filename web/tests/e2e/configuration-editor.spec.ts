import { expect, test, type Page, type Route } from "@playwright/test";
import { readFileSync } from "node:fs";

type Placement = { id: string; key: string; ad_type: string; enabled: boolean; network_mode: string; frequency_policy_id: string; load_timeout_ms: number; cache_policy: string; fallback_behavior: string };
type FrequencyPolicy = { id: string; cooldown_ms: number; interval_ms: number; max_count: number; shift_count: number; positions: string[] };
type FeatureSwitch = { id: string; key: string; default_value: boolean; risk_level: string; rollback_method: string };
type Fixture = { entities: { placements: Placement[]; frequency_policies: FrequencyPolicy[]; feature_switches: FeatureSwitch[] } };
const fixture = JSON.parse(readFileSync(new URL("../../../testdata/contracts/mobile-ad-monetization/v1/entities.json", import.meta.url), "utf8")) as Fixture;

const environments = [
  { id: "development", name: "Development", kind: "development", provider: { type: "firebase-remote-config", project_id: "photo-editor-dev" }, publish: { requires_confirmation: false } },
  { id: "production", name: "Production", kind: "production", provider: { type: "firebase-remote-config", project_id: "photo-editor-prod" }, publish: { requires_confirmation: true } },
] as const;

const placementSchema = [
  field("key", "string", "广告位键", "基础", "", []), field("ad_type", "string", "广告类型", "基础", "interstitial", ["app_open", "interstitial", "native"]),
  field("enabled", "boolean", "启用", "基础", true, []), field("network_mode", "string", "广告网络模式", "投放", "hybrid", ["hybrid", "bidding", "waterfall"]),
  field("frequency_policy_id", "reference", "频控策略", "投放", "", []), field("load_timeout_ms", "integer", "加载超时（毫秒）", "投放", 4000, []),
  field("cache_policy", "string", "缓存策略", "投放", "memory", ["memory", "disk", "none"]), field("fallback_behavior", "string", "兜底行为", "投放", "continue", []),
];

function field(name: string, type: string, label: string, group: string, defaultValue: unknown, values: string[]) {
  return { name, type, required: true, nullable: false, default: defaultValue, sensitivity: "public", ui: { label, description: "测试字段", control: type === "boolean" ? "switch" : "input", group, order: placementSchemaOrder(name) }, validation: { enum: values } };
}
function placementSchemaOrder(name: string) { return ["key", "ad_type", "enabled", "network_mode", "frequency_policy_id", "load_timeout_ms", "cache_policy", "fallback_behavior"].indexOf(name); }

async function mockConfigurationAPI(page: Page, mode: "normal" | "conflict" | "validation" | "referenced" | "stale" | "empty" = "normal") {
  let revision = 1;
  let dirty = false;
  const placements = mode === "empty" ? [] : [...fixture.entities.placements.slice(0, 3), fixture.entities.placements.find((item) => item.id === "ad_interstitial_001")!].map((item) => ({ ...item }));
  const policies = mode === "empty" ? [] : fixture.entities.frequency_policies.map((item) => ({ ...item }));
  const switches = mode === "empty" ? [] : fixture.entities.feature_switches.map((item) => ({ ...item }));
  const bindings: Record<string, Array<{ id: string; fields: Record<string, unknown> }>> = {
    development: mode === "empty" ? [] : placements.flatMap((placement, index) => ["ios", "android"].map((platform) => ({ id: `ub_development_${platform}_${placement.id}`, fields: { placement_id: placement.id, environment_id: "development", platform, unit_id_ref: `${platform}_dev_${index + 1}`, status: "configured" } }))),
    production: mode === "empty" ? [] : placements.flatMap((placement, index) => ["ios", "android"].map((platform) => ({ id: `ub_production_${platform}_${placement.id}`, fields: { placement_id: placement.id, environment_id: "production", platform, unit_id_ref: placement.id === placements[2].id ? null : `${platform}_prod_${index + 1}`, status: placement.id === placements[2].id ? "missing" : "configured" } }))),
  };
  const state = { data: { project: { id: "photo-editor", name: "Photo Editor", pack_ref: "mobile-ad-monetization/v1", source_type: "managed-file" }, environments, capabilities: { project_edit: true, environment_manage: true } }, meta: { request_id: "req_bootstrap", revision } };
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request(); const path = new URL(request.url()).pathname; const method = request.method();
    if (path === "/api/v1/bootstrap") return json(route, { ...state, meta: { ...state.meta, revision } });
    if (path === "/api/v1/packs/mobile-ad-monetization/versions/v1") return json(route, { data: { ref: "mobile-ad-monetization/v1", name: "mobile-ad-monetization", version: "v1", description: "广告配置", capabilities: ["entities"], schema_version: 1, entity_types: [] }, meta: meta(revision) });
    if (path === "/api/v1/packs/mobile-ad-monetization/versions/v1/schema") return json(route, { data: { version: 1, entities: [{ name: "placement", fields: placementSchema }], migrations: [] }, meta: meta(revision) });
    const validateMatch = path.match(/^\/api\/v1\/drafts\/([^/]+):validate$/);
    if (validateMatch && method === "POST") return json(route, validationResult(validateMatch[1], "fresh"));
    const diagnosticsMatch = path.match(/^\/api\/v1\/drafts\/([^/]+)\/diagnostics$/);
    if (diagnosticsMatch && method === "GET") {
      if (mode === "normal" || mode === "conflict" || mode === "validation" || mode === "referenced") return json(route, { error: { code: "validation_not_found", message: "尚未运行完整校验", request_id: "req_validation_not_found" } }, 404);
      return json(route, validationResult(diagnosticsMatch[1], "stale"));
    }
    const draftMatch = path.match(/^\/api\/v1\/drafts\/([^/]+)$/);
    if (draftMatch && method === "GET") return json(route, { data: draft(draftMatch[1], placements, dirty), meta: meta(revision) });
    const listMatch = path.match(/^\/api\/v1\/drafts\/([^/]+)\/entities$/);
    if (listMatch && method === "GET") {
      const entityType = new URL(request.url()).searchParams.get("entity_type");
      const env = listMatch[1];
      if (entityType === "placement") return json(route, { data: placements.map((item) => view("placement", item.id, fieldsOf(item), dirty)), meta: meta(revision) });
      if (entityType === "frequency_policy") return json(route, { data: policies.map((item) => view("frequency_policy", item.id, fieldsOf(item), dirty)), meta: meta(revision) });
      if (entityType === "feature_switch") return json(route, { data: switches.map((item) => view("feature_switch", item.id, fieldsOf(item), dirty)), meta: meta(revision) });
      if (entityType === "unit_binding") return json(route, { data: (bindings[env] ?? []).map((item) => view("unit_binding", item.id, item.fields, dirty)), meta: meta(revision) });
    }
    const referencesMatch = path.match(/^\/api\/v1\/drafts\/([^/]+)\/entities\/([^/]+)\/([^/]+)\/referenced-by$/);
    if (referencesMatch && method === "GET") {
      const [, , type, id] = referencesMatch;
      const referencedBy = type === "frequency_policy" ? placements.filter((item) => item.frequency_policy_id === id).map((item) => ({ entity_ref: `entity:mobile-ad-monetization/v1:placement:${item.id}`, entity_type: "placement", entity_id: item.id, path: "/frequency_policy_id" })) : [];
      return json(route, { data: { entity_ref: `entity:mobile-ad-monetization/v1:${type}:${id}`, referenced_by: referencedBy }, meta: meta(revision) });
    }
    const entityMatch = path.match(/^\/api\/v1\/drafts\/([^/]+)\/entities\/([^/]+)\/([^/]+)$/);
    if (entityMatch && method === "GET") {
      const [, , type, id] = entityMatch; const placement = placements.find((item) => item.id === id);
      if (type === "placement" && placement) return json(route, { data: view("placement", id, fieldsOf(placement), dirty), meta: meta(revision) });
    }
    if (entityMatch && method === "DELETE") {
      expect(await request.headerValue("if-match")).toBe(`"${revision}"`);
      const [, environmentID, type, id] = entityMatch;
      const references = type === "frequency_policy" ? placements.filter((item) => item.frequency_policy_id === id).map((item) => ({ entity_ref: `entity:mobile-ad-monetization/v1:placement:${item.id}`, entity_type: "placement", entity_id: item.id, path: "/frequency_policy_id" })) : [];
      if (type === "frequency_policy" && mode === "referenced" && references.length) return json(route, { error: { code: "entity_referenced", message: "频控策略仍被广告位引用", request_id: "req_referenced", current_revision: revision, references } }, 409);
      if (type === "frequency_policy") { const index = policies.findIndex((item) => item.id === id); if (index >= 0) policies.splice(index, 1); }
      if (type === "placement") { const index = placements.findIndex((item) => item.id === id); if (index >= 0) placements.splice(index, 1); }
      dirty = true; revision += 1;
      return json(route, { data: view(type, id, {}, true), meta: meta(revision) });
    }
    if (listMatch && method === "POST") return mutateEntity(route, request, listMatch[1], (request.postDataJSON() as { entity_type: string }).entity_type, undefined);
    if (entityMatch && method === "PUT") return mutateEntity(route, request, entityMatch[1], entityMatch[2], entityMatch[3]);
    return json(route, { error: { code: "route_not_found", message: "missing test route", request_id: "req_missing" } }, 404);

    async function mutateEntity(target: Route, targetRequest: import("@playwright/test").Request, environmentID: string, type: string, pathID: string | undefined) {
      expect(await targetRequest.headerValue("if-match")).toBe(`"${revision}"`);
      const input = targetRequest.postDataJSON() as { entity: { id: string; fields: Record<string, unknown> }; write_scope: string };
      if (mode === "conflict" && type === "placement") return json(target, { error: { code: "revision_mismatch", message: "草稿已变化", request_id: "req_conflict", current_revision: revision + 1, current_source_revision: "source_2", conflict_scope: input.write_scope, current_state: draft(environmentID, placements, true) } }, 412);
      if (mode === "validation" && type === "placement") return json(target, { error: { code: "validation_failed", message: "字段不合法", request_id: "req_validation", details: [{ code: "value_not_allowed", path: "/placements/ad_app_open_001/load_timeout_ms", entity_ref: "entity:mobile-ad-monetization/v1:placement:ad_app_open_001", scope: "baseline", message: "加载超时需在 1000–10000 毫秒之间" }] } }, 422);
      if (type === "placement") {
        const next = { id: input.entity.id, ...input.entity.fields } as Placement;
        const index = placements.findIndex((item) => item.id === (pathID ?? input.entity.id));
        if (index >= 0) placements[index] = next; else placements.push(next);
        dirty = true; revision += 1;
        return json(target, { data: view("placement", next.id, fieldsOf(next), true), meta: meta(revision) }, pathID ? 200 : 201);
      }
      if (type === "frequency_policy") {
        const next = { id: input.entity.id, ...input.entity.fields } as FrequencyPolicy;
        const index = policies.findIndex((item) => item.id === (pathID ?? input.entity.id));
        if (index >= 0) policies[index] = next; else policies.push(next);
        dirty = true; revision += 1;
        return json(target, { data: view("frequency_policy", next.id, fieldsOf(next), true), meta: meta(revision) });
      }
      if (type === "feature_switch") {
        const next = { id: input.entity.id, ...input.entity.fields } as FeatureSwitch;
        const index = switches.findIndex((item) => item.id === (pathID ?? input.entity.id));
        if (index >= 0) switches[index] = next; else switches.push(next);
        dirty = true; revision += 1;
        return json(target, { data: view("feature_switch", next.id, fieldsOf(next), true), meta: meta(revision) });
      }
      const existing = bindings[environmentID] ?? (bindings[environmentID] = []);
      const index = existing.findIndex((item) => item.id === input.entity.id);
      if (index >= 0) existing[index] = input.entity; else existing.push(input.entity);
      dirty = true; revision += 1;
      return json(target, { data: view("unit_binding", input.entity.id, input.entity.fields, true), meta: meta(revision) }, pathID ? 200 : 201);
    }

    function validationResult(environmentID: string, status: "fresh" | "stale") {
      if (mode === "empty" && placements.length > 0 && policies.length > 0) return { data: { environment_id: environmentID, validated_draft_revision: revision, validated_at: "2026-07-12T09:30:00Z", status, readiness: "ready", diagnostics: [] }, meta: meta(revision) };
      return { data: { environment_id: environmentID, validated_draft_revision: 1, validated_at: "2026-07-11T09:30:00Z", status, readiness: "blocked", diagnostics: [
        { code: "placement_frequency_policy_invalid", path: "/placements/ad_app_open_001/frequency_policy_id", severity: "error", message: "广告位绑定的频控策略不适用于当前广告类型。", entity_ref: "entity:mobile-ad-monetization/v1:placement:ad_app_open_001", fix_suggestion: "选择兼容的频控策略后重新运行校验。" },
        { code: "frequency_policy_needs_review", path: "/frequency_policies/inter_global_cap", severity: "warning", message: "频控策略的周期上限接近风险阈值。", entity_ref: "entity:mobile-ad-monetization/v1:frequency_policy:inter_global_cap", fix_suggestion: "确认业务指标后保留或降低上限。" },
        { code: "binding_review", path: "/unit_bindings/ub_production_ios_ad_app_open_001", severity: "info", message: "Production iOS 单元 ID 建议在发布前复核。", entity_ref: "entity:mobile-ad-monetization/v1:unit_binding:ub_production_ios_ad_app_open_001", fix_suggestion: "确认单元 ID 对应 Production 项目。" },
      ] }, meta: meta(revision) };
    }
  });
}

test("列表加载后可按类型和文本筛选", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration");
  await expect(page.getByRole("row", { name: /app_open_cold_start/ })).toBeVisible();
  await page.getByLabel("按类型筛选").selectOption("native");
  await expect(page.getByRole("row", { name: /app_open_cold_start/ })).toHaveCount(0);
  await page.getByLabel("按类型筛选").selectOption("all"); await page.getByPlaceholder("搜索名称或 key").fill("warm_resume");
  await expect(page.getByRole("row", { name: /app_open_warm_resume/ })).toBeVisible();
});

test("运行校验显示摘要、行锚点和广告位详情诊断", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration");
  await page.getByRole("button", { name: /运行校验/ }).first().click();
  await expect(page.getByText("存在阻断", { exact: true })).toBeVisible();
  await expect(page.getByText("阻断 1 · 警告 1 · 建议 1", { exact: true })).toBeVisible();
  const row = page.getByRole("row", { name: /app_open_cold_start/ });
  await expect(row.getByLabel("阻断 1 项")).toBeVisible();
  await row.getByRole("button", { name: "编辑 app_open_cold_start" }).click();
  const diagnostics = page.getByLabel("此广告位的校验问题");
  await expect(diagnostics).toContainText("广告位绑定的频控策略不适用于当前广告类型。");
  await expect(diagnostics).toContainText("建议：选择兼容的频控策略后重新运行校验。");
});

test("历史校验结果为 stale 时提示重新运行", async ({ page }) => {
  await mockConfigurationAPI(page, "stale"); await page.goto("/#configuration");
  await expect(page.getByText("结果可能已过期", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "重新运行校验" }).first().click();
  await expect(page.getByText("结果可能已过期", { exact: true })).toHaveCount(0);
});

test("新建广告位后回到列表并显示未发布修改", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration"); await page.getByRole("button", { name: "新建广告位" }).click();
  await page.getByLabel("稳定 ID").fill("ad_interstitial_011"); await page.getByLabel("广告位键").fill("interstitial_test_entry");
  await page.getByLabel("频控策略").selectOption("inter_global_cap"); await page.getByRole("button", { name: "保存修改" }).click();
  const createdRow = page.getByRole("row", { name: /interstitial_test_entry/ });
  await expect(createdRow).toBeVisible(); await expect(createdRow.getByText("未发布修改", { exact: true })).toBeVisible();
});

test("编辑保存后在列表标记未发布修改", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration"); await page.getByRole("button", { name: "编辑 app_open_cold_start" }).click();
  await page.getByLabel("加载超时（毫秒）").fill("4800"); await page.getByRole("button", { name: "保存修改" }).click(); await page.getByLabel("返回配置列表").click();
  await expect(page.getByRole("row", { name: /app_open_cold_start/ }).getByText("未发布修改")).toBeVisible();
});

test("412 显示实体级对照且不覆盖当前输入", async ({ page }) => {
  await mockConfigurationAPI(page, "conflict"); await page.goto("/#configuration"); await page.getByRole("button", { name: "编辑 app_open_cold_start" }).click();
  await page.getByLabel("加载超时（毫秒）").fill("4800"); await page.getByRole("button", { name: "保存修改" }).click();
  await expect(page.getByRole("dialog")).toContainText("广告位已被其他操作修改"); await expect(page.getByRole("dialog")).toContainText("4800");
  await expect(page.getByLabel("加载超时（毫秒）")).toHaveValue("4800");
});

test("422 将字段错误定位到对应表单行", async ({ page }) => {
  await mockConfigurationAPI(page, "validation"); await page.goto("/#configuration"); await page.getByRole("button", { name: "编辑 app_open_cold_start" }).click();
  await page.getByRole("button", { name: "保存修改" }).click();
  await expect(page.getByText("加载超时需在 1000–10000 毫秒之间")).toBeVisible(); await expect(page.getByRole("button", { name: "保存修改（1 项错误）" })).toBeVisible();
});

test("环境绑定矩阵以环境覆盖范围写入", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration"); await page.getByRole("button", { name: "编辑 app_open_cold_start" }).click();
  await page.getByLabel("编辑 Development ios 绑定").click(); await page.getByLabel("Development ios 单元 ID").fill("ios_dev_changed"); await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByText("ios_dev_changed")).toBeVisible();
});

test("频控抽屉可编辑并展示受影响广告位", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration"); await page.getByRole("tab", { name: "频控策略" }).click();
  await page.getByRole("button", { name: "编辑频控策略 inter_global_cap" }).click();
  const drawer = page.getByRole("dialog", { name: "编辑频控策略 inter_global_cap" });
  await expect(drawer.getByRole("heading", { name: "引用此策略的广告位" })).toBeVisible(); await expect(drawer.getByText("ad_interstitial_001")).toBeVisible();
  await drawer.getByLabel("冷却时间（毫秒）").fill("45000"); await drawer.getByRole("button", { name: "保存策略" }).click();
  await expect(page.getByRole("dialog", { name: "编辑频控策略 inter_global_cap" })).toHaveCount(0);
});

test("功能开关翻转后保存", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration"); await page.getByRole("tab", { name: "功能开关" }).click();
  const toggle = page.getByRole("switch", { name: "切换 use_amazon_bidding" });
  await expect(toggle).toHaveAttribute("aria-checked", "false"); await toggle.click(); await expect(toggle).toHaveAttribute("aria-checked", "true");
});

test("删除被引用频控时阻断并可跳到引用广告位", async ({ page }) => {
  await mockConfigurationAPI(page, "referenced"); await page.goto("/#configuration"); await page.getByRole("tab", { name: "频控策略" }).click();
  await page.getByRole("button", { name: "删除频控策略 inter_global_cap" }).click(); await page.getByRole("button", { name: "确认删除" }).click();
  const dialog = page.getByRole("dialog"); await expect(dialog).toContainText("无法删除 inter_global_cap"); await expect(dialog).toContainText("存在引用时不允许继续删除");
  await dialog.getByText("ad_interstitial_001", { exact: true }).first().click(); await expect(page.getByRole("heading", { name: "interstitial_open_document" })).toBeVisible();
});

test("未被引用频控可删除", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration"); await page.getByRole("tab", { name: "频控策略" }).click();
  await page.getByRole("button", { name: "删除频控策略 legacy_campaign_cap" }).click(); await page.getByRole("button", { name: "确认删除" }).click();
  await expect(page.getByText("legacy_campaign_cap", { exact: true })).toHaveCount(0);
});

test("创建后的广告位键与稳定 ID 为只读通用值", async ({ page }) => {
  await mockConfigurationAPI(page); await page.goto("/#configuration"); await page.getByRole("button", { name: "新建广告位" }).click();
  await page.getByLabel("稳定 ID").fill("ad_interstitial_011"); await page.getByLabel("广告位键").fill("interstitial_test_entry"); await page.getByLabel("频控策略").selectOption("inter_global_cap"); await page.getByRole("button", { name: "保存修改" }).click();
  await page.getByRole("button", { name: "编辑 interstitial_test_entry" }).click(); await expect(page.getByLabel("广告位键")).toHaveAttribute("readonly", ""); await expect(page.getByText("所有环境一致，不可修改").first()).toBeVisible();
});

test("全空引导可创建频控和广告位并通过校验", async ({ page }) => {
  await mockConfigurationAPI(page, "empty"); await page.goto("/#configuration");
  const guide = page.getByLabel("配置创建引导");
  await expect(guide.getByRole("button", { name: "创建频控策略" })).toBeVisible();
  await expect(guide.getByRole("button", { name: "配置环境绑定" })).toBeDisabled();
  await guide.getByRole("button", { name: "创建频控策略" }).click();
  const policyDrawer = page.getByRole("dialog", { name: "新建频控策略" });
  await policyDrawer.getByLabel("频控策略稳定 ID").fill("first_frequency"); await policyDrawer.getByLabel("适用位置").fill("document_entry"); await policyDrawer.getByRole("button", { name: "创建策略" }).click();
  await expect(policyDrawer).toHaveCount(0);
  await page.getByRole("button", { name: "新建广告位" }).click();
  await page.getByLabel("稳定 ID").fill("ad_interstitial_first"); await page.getByLabel("广告位键").fill("interstitial_first_entry"); await page.getByLabel("频控策略").selectOption("first_frequency"); await page.getByRole("button", { name: "保存修改" }).click();
  await expect(page.getByRole("row", { name: /interstitial_first_entry/ })).toBeVisible();
  await page.getByRole("button", { name: /运行校验/ }).first().click(); await expect(page.getByText("可发布", { exact: true })).toBeVisible();
});

test("广告位空频控提示可直达创建并自动回填", async ({ page }) => {
  await mockConfigurationAPI(page, "empty"); await page.goto("/#configuration");
  await page.getByRole("button", { name: "创建广告位" }).click();
  await expect(page.getByText("还没有频控策略——先创建一个", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "新建频控策略" }).click();
  const policyDrawer = page.getByRole("dialog", { name: "新建频控策略" });
  await policyDrawer.getByLabel("频控策略稳定 ID").fill("placement_frequency"); await policyDrawer.getByRole("button", { name: "创建策略" }).click();
  await expect(page.getByRole("dialog", { name: "新建频控策略" })).toHaveCount(0);
  await expect(page.getByRole("combobox", { name: "频控策略" })).toHaveValue("placement_frequency");
  await page.getByLabel("稳定 ID").fill("ad_interstitial_direct"); await page.getByLabel("广告位键").fill("interstitial_direct_entry"); await page.getByRole("button", { name: "保存修改" }).click();
  await expect(page.getByRole("row", { name: /interstitial_direct_entry/ })).toBeVisible();
});

test("功能开关可从创建抽屉保存", async ({ page }) => {
  await mockConfigurationAPI(page, "empty"); await page.goto("/#configuration"); await page.getByRole("tab", { name: "功能开关" }).click();
  await page.getByRole("button", { name: "新建开关" }).click();
  const drawer = page.getByRole("dialog", { name: "新建开关" });
  await drawer.getByLabel("开关稳定 ID").fill("enable_first_entry"); await drawer.getByLabel("开关键").fill("enable_first_entry"); await drawer.getByLabel("默认启用").click(); await drawer.getByRole("button", { name: "创建开关" }).click();
  await expect(page.getByRole("switch", { name: "切换 enable_first_entry" })).toHaveAttribute("aria-checked", "true");
});

function fieldsOf(value: { id: string; [key: string]: unknown }) { const { id: _id, ...fields } = value; return fields; }
function meta(revision: number) { return { request_id: "req_configuration", revision }; }
function view(type: string, id: string, fields: Record<string, unknown>, dirty: boolean) { const record = { id, fields }; return { entity_ref: `entity:mobile-ad-monetization/v1:${type}:${id}`, entity_type: type, entity_id: id, source: { present: true, value: record }, draft: dirty ? { present: true, value: record } : { present: false }, resolved: { present: true, value: record }, effective: { present: true, value: record }, origin: dirty ? "draft_baseline" : "baseline", source_revision: "source_1" }; }
function draft(environmentID: string, placements: Placement[], dirty: boolean) { return { environment_id: environmentID, pack_ref: "mobile-ad-monetization/v1", source_revision: "source_1", dirty, dirty_scopes: dirty ? ["baseline"] : [], baseline: { source: { present: true, value: {} }, draft: { present: dirty, value: dirty ? {} : undefined }, resolved: { present: true, value: {} }, dirty }, environment_override: { source: { present: false }, draft: { present: false }, resolved: { present: false }, dirty: false }, effective: { placements: placements.map((item) => ({ id: item.id, fields: fieldsOf(item) })) }, field_states: placements.flatMap((placement) => Object.entries(fieldsOf(placement)).map(([name, value]) => ({ path: `/placements/${placement.id}/${name}`, pack_default: { present: false }, baseline: { present: true, value }, draft_baseline: { present: dirty, value }, environment_override: { present: false }, draft_environment_override: { present: false }, effective: { present: true, value }, origin: dirty ? "draft_baseline" : "baseline", environment_override_allowed: false, is_environment_overridden: false, source_revision: "source_1", nullable: false }))), affected_environments: [] }; }
async function json(route: Route, body: unknown, status = 200) { await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) }); }
