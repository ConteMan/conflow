import { expect, test, type Page, type Route } from "@playwright/test";

type Environment = {
  id: string;
  name: string;
  kind: "development" | "staging" | "production" | "custom";
  provider: { type: "firebase-remote-config"; project_id: string };
  publish: { requires_confirmation: boolean };
};

const initialEnvironments: Environment[] = [
  { id: "development", name: "Development", kind: "development", provider: { type: "firebase-remote-config", project_id: "photo-editor-dev" }, publish: { requires_confirmation: false } },
  { id: "production", name: "Production", kind: "production", provider: { type: "firebase-remote-config", project_id: "photo-editor-prod" }, publish: { requires_confirmation: true } },
];

function bootstrap(environments = initialEnvironments) {
  return {
    data: {
      project: { id: "photo-editor", name: "Photo Editor", pack_ref: "mobile-ad-monetization/v1", source_type: "managed-file" },
      environments,
      capabilities: { project_edit: true, environment_manage: true },
    },
    meta: { request_id: "req_bootstrap", revision: 1 },
  };
}

async function mockAPI(page: Page, options: { failBootstrapOnce?: boolean; conflictProject?: boolean; conflictEnvironment?: boolean; readOnly?: boolean } = {}) {
  const state = bootstrap(structuredClone(initialEnvironments));
  if (options.readOnly) state.data.capabilities = { project_edit: false, environment_manage: false };
  let bootstrapFailures = 0;
  let providerConnected = false;
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/api/v1/bootstrap") {
      if (options.failBootstrapOnce && bootstrapFailures < 2) { bootstrapFailures += 1; await json(route, { error: { code: "project_unavailable", message: "ignored by UI", request_id: "req_unavailable" } }, 503); return; }
      await json(route, state); return;
    }
    if (path === "/api/v1/packs/mobile-ad-monetization/versions/v1") {
      await json(route, { data: { ref: "mobile-ad-monetization/v1", name: "mobile-ad-monetization", version: "v1", description: "Mobile ad placement and frequency controls.", capabilities: ["environment_overrides", "semantic_diff"], schema_version: 1, entity_types: [] }, meta: { request_id: "req_pack", revision: 1 } }); return;
    }
    if (path === "/api/v1/environments/development/provider" && request.method() === "GET") {
      await json(route, { data: { environment_id: "development", provider_type: "firebase-remote-config", status: providerConnected ? "connected" : "not_configured", credentials_path_display: providerConnected ? "…/firebase.json" : undefined, capabilities: { pull: true, validate: true, publish: true, rollback: true } }, meta: { request_id: "req_provider", revision: 1 } }); return;
    }
    if (path === "/api/v1/environments/development/provider:connect" && request.method() === "POST") {
      expect(request.headers()["content-type"]).toContain("application/json");
      expect(request.postDataJSON()).toEqual({ credentials_path: "/private/keys/firebase.json" });
      providerConnected = true;
      await json(route, { data: { operation_id: "op_connect", operation_type: "provider_connect", status: "pending", stage: "queued", remote_state: "unchanged", created_at: "2026-07-11T10:00:00Z", updated_at: "2026-07-11T10:00:00Z" }, meta: { request_id: "req_connect", revision: 1 } }, 202); return;
    }
    if (path === "/api/v1/operations/op_connect") {
      await json(route, { data: { operation_id: "op_connect", operation_type: "provider_connect", status: "succeeded", stage: "completed", remote_state: "unchanged", created_at: "2026-07-11T10:00:00Z", updated_at: "2026-07-11T10:00:01Z" }, meta: { request_id: "req_operation", revision: 1 } }); return;
    }
    if (path === "/api/v1/project" && request.method() === "PUT") {
      await expectRevision(request, state.meta.revision);
      const input = request.postDataJSON() as { id: string; name: string };
      if (options.conflictProject) {
        await json(route, { error: { code: "revision_mismatch", message: "ignored by UI", request_id: "req_conflict", current_revision: 2, current_state: { project: { ...state.data.project, name: "Photo Editor from Git" }, environments: state.data.environments } } }, 412, { ETag: '"2"' }); return;
      }
      state.data.project = { ...state.data.project, ...input };
      state.meta.revision += 1;
      await json(route, { data: state.data.project, meta: { request_id: "req_project", revision: state.meta.revision } }); return;
    }
    if (path === "/api/v1/environments" && request.method() === "POST") {
      await expectRevision(request, state.meta.revision);
      const environment = request.postDataJSON() as Environment;
      state.data.environments.push(environment); state.meta.revision += 1;
      await json(route, { data: environment, meta: { request_id: "req_create", revision: state.meta.revision } }, 201); return;
    }
    const environmentMatch = path.match(/^\/api\/v1\/environments\/([^/]+)$/);
    if (environmentMatch && request.method() === "PUT") {
      await expectRevision(request, state.meta.revision);
      const id = decodeURIComponent(environmentMatch[1]);
      const input = request.postDataJSON() as Pick<Environment, "name" | "provider" | "publish">;
      if (options.conflictEnvironment) {
        const environments = state.data.environments.filter((item) => item.id !== id);
        await json(route, { error: { code: "revision_mismatch", message: "ignored by UI", request_id: "req_environment_conflict", current_revision: 2, current_state: { project: state.data.project, environments } } }, 412, { ETag: '"2"' }); return;
      }
      const current = state.data.environments.find((item) => item.id === id)!;
      const updated = { ...current, ...input };
      state.data.environments = state.data.environments.map((item) => item.id === id ? updated : item); state.meta.revision += 1;
      await json(route, { data: updated, meta: { request_id: "req_update", revision: state.meta.revision } }); return;
    }
    if (environmentMatch && request.method() === "DELETE") {
      await expectRevision(request, state.meta.revision);
      const id = decodeURIComponent(environmentMatch[1]);
      state.data.environments = state.data.environments.filter((item) => item.id !== id); state.meta.revision += 1;
      await json(route, { data: { deleted_id: id }, meta: { request_id: "req_delete", revision: state.meta.revision } }); return;
    }
    await json(route, { error: { code: "route_not_found", message: "missing test route", request_id: "req_missing" } }, 404);
  });
}

test("bootstrap renders project overview from API data", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Photo Editor" })).toBeVisible();
  await expect(page.getByText("mobile-ad-monetization/v1")).toBeVisible();
  await expect(page.getByText("Provider 状态").locator("../..").getByText("查看连接卡")).toBeVisible();
});

test("production identity comes from environment kind and remains explicit", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/");
  await page.getByLabel("切换环境").selectOption("production");
  await expect(page.getByTestId("app-topbar")).toHaveClass(/app-topbar--production/);
  await expect(page.getByTestId("production-marker")).toHaveText("Production 环境");
  await expect(page.getByText("你正在查看 Production")).toBeVisible();
});

test("creates and edits an environment through the manifest API", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/#environments");
  await page.getByRole("button", { name: "新建环境" }).click();
  await page.getByLabel("显示名称").fill("Staging");
  await page.getByLabel("环境 ID").fill("staging");
  await page.getByLabel("环境类型").selectOption("staging");
  await page.getByLabel("Firebase 项目").fill("photo-editor-staging");
  await page.getByRole("button", { name: "保存环境" }).click();
  await expect(page.getByRole("row", { name: /Staging staging/ })).toBeVisible();
  await page.getByLabel("显示名称").fill("QA Staging");
  await page.getByRole("button", { name: "保存环境" }).click();
  await expect(page.getByRole("row", { name: /QA Staging staging/ })).toBeVisible();
  await expect(page.getByLabel("环境类型")).toBeDisabled();
});

test("overview creates an environment through the primary entry point", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/");
  await page.getByRole("button", { name: "新建环境" }).click();
  await expect(page.getByRole("heading", { name: "新建环境" })).toBeVisible();
});

test("Firebase card submits a path without retaining its directory", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/");
  await page.getByLabel("服务账号 JSON 路径").fill("/private/keys/firebase.json");
  await page.getByRole("button", { name: "连接并验证" }).click();
  await expect(page.getByText("已配置：")).toContainText("…/firebase.json");
  await expect(page.getByLabel("服务账号 JSON 路径")).toHaveValue("");
  await expect(page.locator("body")).not.toContainText("/private/keys/firebase.json");
});

test("updates project details through the manifest API", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/#project");
  await page.getByLabel("项目名称").fill("Photo Editor Studio");
  await page.getByRole("button", { name: "保存项目资料" }).click();
  await page.getByRole("button", { name: "概览" }).click();
  await expect(page.getByRole("heading", { name: "Photo Editor Studio" })).toBeVisible();
});

test("deletes an environment and selects the remaining environment", async ({ page }) => {
  await mockAPI(page);
  await page.goto("/#environments");
  await page.getByRole("button", { name: "删除", exact: true }).click();
  await page.getByRole("button", { name: "确认删除" }).click();
  await expect(page.getByRole("row", { name: /Development development/ })).toHaveCount(0);
  await expect(page.getByTestId("production-marker")).toHaveText("Production 环境");
  await expect(page.getByRole("button", { name: "删除", exact: true })).toBeDisabled();
});

test("renders project and environment forms as read-only from server capabilities", async ({ page }) => {
  await mockAPI(page, { readOnly: true });
  await page.goto("/#project");
  await expect(page.getByText("当前为只读模式")).toBeVisible();
  await expect(page.getByRole("button", { name: "保存项目资料" })).toBeDisabled();
  await page.getByRole("button", { name: "环境" }).click();
  await expect(page.getByText("当前为只读模式")).toBeVisible();
  await expect(page.getByRole("button", { name: "新建环境" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "保存环境" })).toBeDisabled();
});

test("typed 412 shows authoritative current state and reloads it", async ({ page }) => {
  await mockAPI(page, { conflictProject: true });
  await page.goto("/#project");
  await page.getByLabel("项目名称").fill("My local name");
  await page.getByRole("button", { name: "保存项目资料" }).click();
  await expect(page.getByRole("dialog")).toContainText("Photo Editor from Git");
  await expect(page.getByRole("dialog")).toContainText("My local name");
  await page.getByRole("button", { name: "重新加载当前值" }).click();
  await expect(page.getByLabel("项目名称")).toHaveValue("Photo Editor from Git");
});

test("environment conflict reports a remotely deleted resource", async ({ page }) => {
  await mockAPI(page, { conflictEnvironment: true });
  await page.goto("/#environments");
  await page.getByLabel("显示名称").fill("My local development");
  await page.getByRole("button", { name: "保存环境" }).click();
  await expect(page.getByRole("dialog")).toContainText("My local development");
  await expect(page.getByRole("dialog")).toContainText("资源已删除");
  await expect(page.getByRole("dialog")).not.toContainText("服务端当前值Photo Editor");
  await page.getByRole("button", { name: "重新加载当前值" }).click();
  await expect(page.getByTestId("production-marker")).toHaveText("Production 环境");
});

test("API unavailable state can recover without reloading the browser", async ({ page }) => {
  await mockAPI(page, { failBootstrapOnce: true });
  await page.goto("/");
  await expect(page.getByTestId("service-unavailable")).toBeVisible();
  await expect(page.getByTestId("service-unavailable")).toContainText("req_unavailable");
  await page.getByRole("button", { name: "重新连接" }).click();
  await expect(page.getByRole("heading", { name: "Photo Editor" })).toBeVisible();
});

async function json(route: Route, body: unknown, status = 200, headers: Record<string, string> = {}) {
  await route.fulfill({ status, contentType: "application/json", headers, body: JSON.stringify(body) });
}

async function expectRevision(request: import("@playwright/test").Request, revision: number) {
  expect(await request.headerValue("if-match")).toBe(`"${revision}"`);
}
