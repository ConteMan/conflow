import { expect, test, type Page, type Route } from "@playwright/test";

const placement = {
  id: "interstitial_main",
  fields: {
    client_id: "interstitial_main", key: "main_interstitial", ad_type: "interstitial", description: null,
    enabled_switch_id: "ads_enabled", frequency_policy_type: "preset", frequency_policy_id: "global_cap", custom_frequency_policy: null,
    network_mode: null, load_timeout_ms: 4000, cache_policy: "memory", cache_ttl: null, fallback_behavior: "continue",
  },
};

const placementFields = [
  field("client_id", "string", "客户端 ID", "input", ""), field("key", "string", "广告位键", "input", ""), field("ad_type", "string", "广告类型", "select", "interstitial", ["app_open", "interstitial", "native"]), field("description", "string", "描述", "input", null, [], false, true),
  field("enabled_switch_id", "reference", "启用开关", "feature_switch_ref", ""), field("frequency_policy_type", "string", "频控类型", "select", "preset", ["preset", "custom"]), field("network_mode", "string", "广告链路", "select", null, ["admob", "max"], false, true), field("frequency_policy_id", "reference", "频控策略", "select", null, [], true),
  field("custom_frequency_policy", "object", "自定义频控", "object", null, [], true), field("load_timeout_ms", "integer", "加载超时（毫秒）", "number", 4000), field("cache_ttl", "object", "缓存有效期", "duration", null, [], true), field("fallback_behavior", "string", "兜底行为", "select", "continue", ["continue", "skip_slot", "show_empty_safe"]),
];

function field(name: string, type: string, label: string, control: string, defaultValue: unknown, values: string[] = [], nullable = false, required = true) {
  return { name, type, required, nullable, default: defaultValue, sensitivity: "public", ui: { label, description: "测试字段", control, group: "投放", order: placementFieldsOrder(name) }, validation: { enum: values } };
}

function placementFieldsOrder(name: string) { return ["client_id", "key", "ad_type", "description", "enabled_switch_id", "frequency_policy_type", "network_mode", "frequency_policy_id", "custom_frequency_policy", "load_timeout_ms", "cache_ttl", "fallback_behavior"].indexOf(name); }

test("v2 编辑保存会剔除遗留 cache_policy", async ({ page }) => {
  let submittedFields: Record<string, unknown> | undefined;
  await mockV2ConfigurationAPI(page, (fields) => { submittedFields = fields; });
  await page.goto("/#configuration");
  await page.getByRole("button", { name: "编辑 main_interstitial" }).click();

  await expect(page.getByLabel("缓存策略")).toHaveCount(0);
  await page.getByRole("button", { name: "保存修改" }).click();
  await expect.poll(() => submittedFields).toBeDefined();
  expect(submittedFields).not.toHaveProperty("cache_policy");
  expect(submittedFields).toMatchObject({ cache_ttl: null, fallback_behavior: "continue" });
});

test("v2 自定义参数可创建、编辑和删除", async ({ page }) => {
  const events: string[] = [];
  await mockV2ConfigurationAPI(page, () => undefined, events);
  await page.goto("/#configuration");
  await page.getByRole("tab", { name: "自定义参数" }).click();
  await page.getByRole("button", { name: "新建参数" }).click();
  await page.getByLabel("参数键").fill("min_supported_version");
  await page.getByLabel("参数值").fill("2.4.0");
  await page.getByLabel("描述").fill("最低支持版本");
  await page.getByRole("button", { name: "创建参数" }).click();
  await expect(page.getByRole("button", { name: "编辑自定义参数 min_supported_version" })).toBeVisible();

  await page.getByRole("button", { name: "编辑自定义参数 min_supported_version" }).click();
  await page.getByLabel("参数值").fill("2.5.0");
  await page.getByRole("button", { name: "保存参数" }).click();
  await expect(page.getByText("2.5.0")).toBeVisible();

  await page.getByRole("button", { name: "删除自定义参数 min_supported_version" }).click();
  await page.getByRole("button", { name: "确认删除" }).click();
  await expect(page.getByText("还没有自定义参数。")).toBeVisible();
  expect(events).toEqual(["create:min_supported_version", "replace:min_supported_version", "delete:min_supported_version"]);
});

async function mockV2ConfigurationAPI(page: Page, onSave: (fields: Record<string, unknown>) => void, events: string[] = []) {
  let revision = 1;
  const customParameters: Array<{ id: string; fields: Record<string, unknown> }> = [];
  const environment = { id: "development", name: "Development", kind: "development", provider: { type: "firebase-remote-config", project_id: "photo-editor-dev" }, publish: { requires_confirmation: false } };
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const method = request.method();
    if (path === "/api/v1/bootstrap") return json(route, { data: { project: { id: "photo-editor", name: "Photo Editor", pack_ref: "mobile-ad-monetization/v2", source_type: "managed-file" }, environments: [environment], capabilities: { project_edit: true, environment_manage: true } }, meta: meta(revision) });
    if (path === "/api/v1/packs/mobile-ad-monetization/versions/v2/schema") return json(route, { data: { version: 2, entities: [{ name: "placement", fields: placementFields }], migrations: [] }, meta: meta(revision) });
    if (path === "/api/v1/drafts/development/diagnostics") return json(route, { error: { code: "validation_not_found", message: "尚未运行完整校验", request_id: "req_validation" } }, 404);
    if (path === "/api/v1/drafts/development") return json(route, { data: draft(), meta: meta(revision) });
    if (path === "/api/v1/drafts/development/entities" && method === "GET") {
      const entityType = new URL(request.url()).searchParams.get("entity_type");
      if (entityType === "placement") return json(route, { data: [view("placement", placement)], meta: meta(revision) });
      if (entityType === "frequency_policy") return json(route, { data: [view("frequency_policy", { id: "global_cap", fields: {} })], meta: meta(revision) });
      if (entityType === "feature_switch") return json(route, { data: [view("feature_switch", { id: "ads_enabled", fields: {} })], meta: meta(revision) });
      if (entityType === "unit_binding") return json(route, { data: [], meta: meta(revision) });
      if (entityType === "custom_parameter") return json(route, { data: customParameters.map((parameter) => view("custom_parameter", parameter, "modified")), meta: meta(revision) });
    }
    if (path === "/api/v1/drafts/development/entities" && method === "POST") {
      const input = request.postDataJSON() as { entity_type: string; entity: { id: string; fields: Record<string, unknown> } };
      if (input.entity_type !== "custom_parameter") return json(route, { error: { code: "route_not_found", message: "missing test route", request_id: "req_missing" } }, 404);
      customParameters.push(input.entity); events.push(`create:${input.entity.id}`); revision += 1;
      return json(route, { data: view("custom_parameter", input.entity, "created"), meta: meta(revision) }, 201);
    }
    if (path.startsWith("/api/v1/drafts/development/entities/custom_parameter/") && method === "PUT") {
      const input = request.postDataJSON() as { entity: { id: string; fields: Record<string, unknown> } };
      const id = path.split("/").at(-1)!;
      const index = customParameters.findIndex((parameter) => parameter.id === id);
      customParameters[index] = input.entity; events.push(`replace:${id}`); revision += 1;
      return json(route, { data: view("custom_parameter", input.entity, "modified"), meta: meta(revision) });
    }
    if (path.startsWith("/api/v1/drafts/development/entities/custom_parameter/") && method === "DELETE") {
      const id = path.split("/").at(-1)!;
      const index = customParameters.findIndex((parameter) => parameter.id === id);
      const [removed] = customParameters.splice(index, 1); events.push(`delete:${id}`); revision += 1;
      return json(route, { data: view("custom_parameter", removed, "modified"), meta: meta(revision) });
    }
    if (path === "/api/v1/drafts/development/entities/placement/interstitial_main" && method === "GET") return json(route, { data: view("placement", placement), meta: meta(revision) });
    if (path === "/api/v1/drafts/development/entities/placement/interstitial_main" && method === "PUT") {
      const input = request.postDataJSON() as { entity: { fields: Record<string, unknown> } };
      onSave(input.entity.fields);
      placement.fields = input.entity.fields as typeof placement.fields;
      revision += 1;
      return json(route, { data: view("placement", placement, "modified"), meta: meta(revision) });
    }
    return json(route, { error: { code: "route_not_found", message: "missing test route", request_id: "req_missing" } }, 404);
  });
}

function view(type: string, entity: { id: string; fields: Record<string, unknown> }, changeStatus = "unchanged") {
  const record = { id: entity.id, fields: entity.fields };
  return { change_status: changeStatus, entity_ref: `entity:mobile-ad-monetization/v2:${type}:${entity.id}`, entity_type: type, entity_id: entity.id, source: { present: true, value: record }, draft: { present: changeStatus !== "unchanged", value: record }, resolved: { present: true, value: record }, effective: { present: true, value: record }, origin: changeStatus === "unchanged" ? "baseline" : "draft_baseline", source_revision: "source_1" };
}

function draft() {
  return { environment_id: "development", pack_ref: "mobile-ad-monetization/v2", source_revision: "source_1", dirty: false, dirty_scopes: [], baseline: { source: { present: true, value: {} }, draft: { present: false }, resolved: { present: true, value: {} }, dirty: false }, environment_override: { source: { present: false }, draft: { present: false }, resolved: { present: false }, dirty: false }, effective: { placements: [{ id: placement.id, fields: placement.fields }] }, field_states: [], affected_environments: [] };
}

function meta(revision: number) { return { request_id: "req_configuration", revision }; }
async function json(route: Route, body: unknown, status = 200) { await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) }); }
