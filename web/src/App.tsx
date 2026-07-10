import { useEffect, useState } from "react";

import { getBootstrap, type BootstrapResponse } from "./api/client";

export default function App() {
  const [bootstrap, setBootstrap] = useState<BootstrapResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    void getBootstrap(controller.signal)
      .then(setBootstrap)
      .catch((reason: unknown) => {
        if (reason instanceof DOMException && reason.name === "AbortError") return;
        setError(reason instanceof Error ? reason.message : "Unknown error");
      });
    return () => controller.abort();
  }, []);

  return (
    <main className="min-h-screen bg-stone-50 px-6 py-10 text-stone-950 dark:bg-stone-950 dark:text-stone-50">
      <section className="mx-auto max-w-4xl rounded-2xl border border-stone-200 bg-white p-8 shadow-sm dark:border-stone-800 dark:bg-stone-900">
        <p className="text-sm font-semibold tracking-[0.2em] text-emerald-700 uppercase dark:text-emerald-400">Conflow</p>
        <h1 className="mt-3 text-4xl font-semibold tracking-tight">本地 ConfigOps 工作台</h1>
        <p className="mt-4 max-w-2xl text-lg leading-8 text-stone-600 dark:text-stone-300">
          M1 骨架已就绪。后续将在这里管理项目、环境、配置包、发布计划和审计记录。
        </p>

        <div className="mt-8 grid gap-4 md:grid-cols-3">
          <Status title="本地服务" value={bootstrap ? "正常" : "连接中"} />
          <Status title="当前项目" value={bootstrap?.data.project.name ?? "—"} />
          <Status title="环境数量" value={bootstrap ? String(bootstrap.data.environments.length) : "—"} />
        </div>

        {error ? <p className="mt-6 rounded-lg bg-red-50 p-4 text-red-700 dark:bg-red-950 dark:text-red-200">API 连接失败：{error}</p> : null}
      </section>
    </main>
  );
}

function Status({ title, value }: { title: string; value: string }) {
  return (
    <article className="rounded-xl border border-stone-200 p-4 dark:border-stone-700">
      <p className="text-sm text-stone-500 dark:text-stone-400">{title}</p>
      <p className="mt-2 text-lg font-medium">{value}</p>
    </article>
  );
}
