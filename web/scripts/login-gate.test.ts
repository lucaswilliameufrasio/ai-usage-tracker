import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { JSDOM } from "jsdom";

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

async function waitFor(ready: () => boolean): Promise<void> {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    if (ready()) {
      return;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error("tempo esgotado");
}

describe("entrada da pessoa", () => {
  it("oferece o provedor configurado e explica a falha", async () => {
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      const url = input instanceof URL ? input.toString() : String(input);
      if (url.endsWith("/v1/session")) {
        return json({ message: "Autenticação necessária.", error_code: "UNAUTHORIZED" }, 401);
      }
      if (url.endsWith("/v1/login/providers")) {
        return json({ data: [{ provider: "github" }, { provider: "gitea" }] });
      }
      return json({ message: "não encontrado", error_code: "NOT_FOUND" }, 404);
    }) as typeof fetch;
    const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
      url: "http://127.0.0.1:8787/?login=falha",
      pretendToBeVisual: true,
    });
    try {
      const view = dom.window;
      const root = view.document.querySelector("#app");
      if (!(root instanceof view.HTMLElement)) {
        throw new Error("raiz ausente");
      }
      const app = await import("../src/app.ts");
      app.start(root);
      await waitFor(() => root.querySelector("a.enter") !== null);
      assert.equal(root.textContent?.includes("O login não foi concluído."), true);
      const links = [...root.querySelectorAll("a.enter")].map((node) => ({
        href: node.getAttribute("href"),
        text: node.textContent,
      }));
      assert.deepEqual(links, [
        { href: "/v1/login/github/start", text: "Entrar com GitHub" },
        { href: "/v1/login/gitea/start", text: "Entrar com Gitea" },
      ]);
      assert.equal(root.querySelector("a[href='/acesso']"), null);
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });

  it("entra na própria conta sem abrir Acesso nem Alertas", async () => {
    const calls: string[] = [];
    let signedIn = true;
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof URL ? input.toString() : String(input);
      const method = init?.method ?? "GET";
      calls.push(`${method} ${url}`);
      if (url.endsWith("/v1/session") && method === "GET") {
        if (!signedIn) {
          return json({ message: "Autenticação necessária.", error_code: "UNAUTHORIZED" }, 401);
        }
        return json({ provider: "github" });
      }
      if (url.endsWith("/v1/login/providers")) {
        return json({ data: [{ provider: "github" }] });
      }
      if (url.endsWith("/v1/account/overview")) {
        return json({
          generated_at: "2026-10-30T12:00:00Z",
          stale_after_seconds: 1800,
          providers: [],
        });
      }
      if (url.endsWith("/v1/session/logout")) {
        signedIn = false;
        return new Response(null, { status: 204 });
      }
      return json({ message: "não encontrado", error_code: "NOT_FOUND" }, 404);
    }) as typeof fetch;
    const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
      url: "http://127.0.0.1:8787/",
      pretendToBeVisual: true,
    });
    try {
      const view = dom.window;
      const root = view.document.querySelector("#app");
      if (!(root instanceof view.HTMLElement)) {
        throw new Error("raiz ausente");
      }
      const app = await import("../src/app.ts");
      app.start(root);
      await waitFor(() => calls.some((call) => call.includes("/v1/account/overview")));
      view.history.pushState({}, "", "/acesso");
      view.dispatchEvent(new view.PopStateEvent("popstate"));
      await waitFor(() => root.textContent?.includes("Área do administrador") === true);
      assert.equal(calls.some((call) => call.includes("/v1/admin")), false);
      assert.equal(calls.some((call) => call.includes("/v1/account/overview")), true);
      assert.equal(root.querySelector("a[href='/acesso']"), null);
      assert.equal(root.querySelector("a[href='/alertas']"), null);
      const exit = root.querySelector("[data-action='sign-out']");
      if (!(exit instanceof view.HTMLButtonElement)) {
        throw new Error("saída ausente");
      }
      exit.click();
      await waitFor(() => calls.some((call) => call.includes("/v1/session/logout")));
      await waitFor(() => root.querySelector("#operator-token") !== null);
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });
});
