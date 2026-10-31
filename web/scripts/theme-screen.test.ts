import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { JSDOM } from "jsdom";

import { operatorTokenKey } from "../src/view.ts";
import { themeColor, themeStorageKey } from "../src/theme.ts";

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

function installFetch(): void {
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = input instanceof URL ? input.toString() : String(input);
    if (url.endsWith("/v1/session")) {
      return json({ message: "Autenticação necessária.", error_code: "UNAUTHORIZED" }, 401);
    }
    if (url.endsWith("/v1/login/providers")) {
      return json({ data: [] });
    }
    if (url.endsWith("/v1/overview")) {
      return json({
        generated_at: "2026-10-30T12:00:00Z",
        stale_after_seconds: 1800,
        providers: [],
      });
    }
    return json({ message: "não encontrado", error_code: "NOT_FOUND" }, 404);
  }) as typeof fetch;
}

describe("tema do painel", () => {
  it("oferece dispositivo, claro e escuro no portão e no trilho", async () => {
    const previous = globalThis.fetch;
    installFetch();
    const dom = new JSDOM(
      `<!doctype html><html><head><meta name="theme-color" content="#e7edf2"></head><body><div id="app"></div></body></html>`,
      { url: "http://127.0.0.1:8787/", pretendToBeVisual: true },
    );
    let systemIsDark = false;
    let listener: ((event: Event) => void) | undefined;
    const view = dom.window;
    view.matchMedia = ((query: string) => {
      return {
        matches: query.includes("dark") && systemIsDark,
        media: query,
        onchange: null,
        addListener() {},
        removeListener() {},
        addEventListener(_type: string, callback: (event: Event) => void) {
          listener = callback;
        },
        removeEventListener() {},
        dispatchEvent() {
          return false;
        },
      };
    }) as typeof view.matchMedia;
    try {
      const root = view.document.querySelector("#app");
      if (!(root instanceof view.HTMLElement)) {
        throw new Error("raiz ausente");
      }
      const app = await import("../src/app.ts");
      app.start(root);
      await waitFor(() => root.querySelector("#operator-token") !== null);
      const labels = [...root.querySelectorAll(".theme-option")].map((node) => node.textContent);
      assert.deepEqual(labels, ["Dispositivo", "Claro", "Escuro"]);
      assert.equal(root.querySelector(".theme")?.getAttribute("role"), "radiogroup");
      assert.equal(root.querySelector(".theme-label")?.textContent, "Tema");
      assert.equal(root.querySelector(".theme-option[aria-checked='true']")?.textContent, "Dispositivo");
      assert.equal(view.document.documentElement.dataset.theme, "light");
      assert.equal(view.document.querySelector("meta[name='theme-color']")?.getAttribute("content"), themeColor("light"));

      const dark = [...root.querySelectorAll(".theme-option")].find((node) => node.textContent === "Escuro");
      if (!(dark instanceof view.HTMLButtonElement)) {
        throw new Error("opção escura ausente");
      }
      dark.click();
      assert.equal(view.localStorage.getItem(themeStorageKey), "dark");
      assert.equal(view.document.documentElement.dataset.theme, "dark");
      assert.equal(view.document.documentElement.dataset.themeChoice, "dark");
      assert.equal(view.document.querySelector("meta[name='theme-color']")?.getAttribute("content"), themeColor("dark"));
      assert.equal(root.querySelector(".theme-option[aria-checked='true']")?.textContent, "Escuro");

      const light = [...root.querySelectorAll(".theme-option")].find((node) => node.textContent === "Claro");
      if (!(light instanceof view.HTMLButtonElement)) {
        throw new Error("opção clara ausente");
      }
      light.click();
      assert.equal(view.localStorage.getItem(themeStorageKey), "light");
      assert.equal(view.document.documentElement.dataset.theme, "light");

      const system = [...root.querySelectorAll(".theme-option")].find((node) => node.textContent === "Dispositivo");
      if (!(system instanceof view.HTMLButtonElement)) {
        throw new Error("opção do dispositivo ausente");
      }
      systemIsDark = true;
      system.click();
      assert.equal(view.localStorage.getItem(themeStorageKey), "system");
      assert.equal(view.document.documentElement.dataset.themeChoice, "system");
      assert.equal(view.document.documentElement.dataset.theme, "dark");
      assert.equal(root.querySelector(".theme-option[aria-checked='true']")?.textContent, "Dispositivo");

      systemIsDark = false;
      listener?.(new view.Event("change"));
      assert.equal(view.document.documentElement.dataset.theme, "light");
      assert.equal(root.querySelector(".theme-option[aria-checked='true']")?.textContent, "Dispositivo");

      view.localStorage.setItem(operatorTokenKey, "op_teste");
      app.start(root);
      await waitFor(() => root.querySelector("main") !== null);
      assert.equal(root.querySelector("nav [role='radiogroup']"), null);
      assert.equal(root.querySelector("nav [data-action='sign-out']"), null);
      assert.equal(root.querySelector(".topbar .theme-option[aria-checked='true']")?.textContent, "Dispositivo");
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });
});
