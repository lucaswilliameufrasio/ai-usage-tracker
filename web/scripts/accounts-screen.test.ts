import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { JSDOM } from "jsdom";

import { operatorTokenKey } from "../src/view.ts";

describe("medicoes da pessoa", () => {
  it("mostra uma régua para cada janela", async () => {
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      const url = input instanceof URL ? input.toString() : String(input);
      if (url.endsWith("/v1/session")) {
        return json({ provider: "github" });
      }
      if (url.endsWith("/v1/account/overview")) {
        return json({
          generated_at: "2026-10-30T12:00:00Z",
          stale_after_seconds: 1800,
          providers: [
            {
              provider: "antigravity",
              freshness: "current",
              observed_at: "2026-10-30T12:00:00Z",
              last_sample: {
                windows: [
                  { name: "gemini_seven_day", used: 50, limit: 100, unit: "percent", resets_at: "2026-11-06T15:00:00Z" },
                  { name: "claude_gpt_seven_day", used: 80, limit: 100, unit: "percent", resets_at: "2026-11-06T15:00:00Z" },
                ],
              },
            },
          ],
        });
      }
      return json({ message: "não usado" }, 404);
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
      await waitFor(() => root.querySelector("h1")?.textContent === "Medições", root);
      const card = root.querySelector("[data-provider='antigravity']");
      assert.equal(card?.querySelectorAll(".staff").length, 2);
      assert.equal(card?.textContent?.includes("50%"), true);
      assert.equal(card?.textContent?.includes("80%"), true);
      assert.equal(card?.textContent?.includes("Gemini, 7 dias"), true);
      assert.equal(card?.textContent?.includes("Claude e GPT, 7 dias"), true);
      assert.equal(root.textContent?.includes("Duas medições"), false);
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });
});

describe("histórico", () => {
  it("repete a régua de cada janela", async () => {
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      const url = input instanceof URL ? input.toString() : String(input);
      if (url.includes("/v1/samples")) {
        return json({
          data: [
            {
              sample_id: "smp_um",
              provider: "antigravity",
              collected_at: "2026-10-30T12:00:00Z",
              windows: [
                { name: "gemini_seven_day", used: 50, resets_at: "2026-11-06T15:00:00Z" },
                { name: "claude_gpt_seven_day", used: 80, resets_at: "2026-11-06T15:00:00Z" },
              ],
            },
          ],
          has_more: false,
        });
      }
      return json({ message: "não usado" }, 404);
    }) as typeof fetch;
    const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
      url: "http://127.0.0.1:8787/historico",
      pretendToBeVisual: true,
    });
    try {
      const view = dom.window;
      view.localStorage.setItem(operatorTokenKey, "op_teste");
      const root = view.document.querySelector("#app");
      if (!(root instanceof view.HTMLElement)) {
        throw new Error("raiz ausente");
      }
      const app = await import("../src/app.ts");
      app.start(root);
      await waitFor(() => root.querySelector("[data-window='gemini_seven_day'] .staff") !== null, root);
      const gemini = root.querySelector("[data-window='gemini_seven_day']");
      const gpt = root.querySelector("[data-window='claude_gpt_seven_day']");
      assert.equal(gemini?.textContent?.includes("50%"), true);
      assert.equal(gpt?.textContent?.includes("80%"), true);
      assert.equal(gemini?.querySelector(".staff") !== null, true);
      assert.equal(gpt?.querySelector(".staff") !== null, true);
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });
});

describe("painel por conta", () => {
  it("mostra cada conta com a própria medição", async () => {
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      const url = input instanceof URL ? input.toString() : String(input);
      if (url.endsWith("/v1/overview")) {
        return json({
          generated_at: "2026-10-30T12:00:00Z",
          stale_after_seconds: 1800,
          accounts: [
            account("acct_ana", "ana@exemplo.test", 11),
            account("acct_bia", "bia@exemplo.test", 22),
          ],
        });
      }
      return json({ message: "não usado" }, 404);
    }) as typeof fetch;
    const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
      url: "http://127.0.0.1:8787/",
      pretendToBeVisual: true,
    });
    try {
      const view = dom.window;
      view.localStorage.setItem(operatorTokenKey, "op_teste");
      const root = view.document.querySelector("#app");
      if (!(root instanceof view.HTMLElement)) {
        throw new Error("raiz ausente");
      }
      const app = await import("../src/app.ts");
      app.start(root);
      await waitFor(() => root.querySelectorAll("[data-account]").length === 2, root);
      const ana = root.querySelector("[data-account='acct_ana']");
      const bia = root.querySelector("[data-account='acct_bia']");
      const anaText = ana?.textContent ?? "";
      const biaText = bia?.textContent ?? "";
      assert.equal(anaText.includes("11%"), true);
      assert.equal(anaText.includes("22%"), false);
      assert.equal(biaText.includes("22%"), true);
      assert.equal(root.textContent?.includes("Duas medições"), false);
      assert.equal(root.querySelector("h1")?.textContent, "Contas");
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });
});

function account(id: string, label: string, used: number): unknown {
  return {
    id,
    label,
    login: "github",
    providers: [
      {
        provider: "codex",
        freshness: "current",
        observed_at: "2026-10-30T12:00:00Z",
        last_sample: {
          plan: "pro",
          windows: [
            { name: "five_hour", used, limit: 100, unit: "percent", resets_at: "2026-10-30T17:00:00Z" },
          ],
        },
      },
    ],
  };
}

function json(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "content-type": "application/json" },
  });
}

async function waitFor(ready: () => boolean, root: HTMLElement): Promise<void> {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    if (ready()) {
      return;
    }
    await new Promise((resolve) => {
      setTimeout(resolve, 10);
    });
  }
  throw new Error(root.textContent ?? "tela vazia");
}
