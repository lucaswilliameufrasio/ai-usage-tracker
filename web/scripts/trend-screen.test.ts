import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { JSDOM } from "jsdom";

describe("tendência na visão", () => {
  it("desenha a tendência de cada janela com o histórico da conta", async () => {
    const dom = await mount("/v1/account/overview", { providers: [provider("claude", 42)] }, samples());
    try {
      const { root } = dom;
      await waitFor(() => root.querySelector("[data-window='five_hour'] .spark") !== null, root);
      const label = root.querySelector("[data-window='five_hour'] .spark svg")?.getAttribute("aria-label");
      assert.equal(label, "Tendência: de 10% a 42% em 3 leituras.");
    } finally {
      dom.close();
    }
  });

  it("não desenha tendência com duas contas, porque a amostra não traz a conta", async () => {
    const accounts = [
      { id: "acc_a", label: "A", login: "github", providers: [provider("claude", 42)] },
      { id: "acc_b", label: "B", login: "github", providers: [provider("claude", 50)] },
    ];
    const dom = await mount("/v1/account/overview", { accounts }, samples());
    try {
      const { root } = dom;
      await waitFor(() => root.querySelectorAll("[data-window='five_hour']").length === 2, root);
      await new Promise((resolve) => {
        setTimeout(resolve, 80);
      });
      assert.equal(root.querySelector(".spark"), null);
    } finally {
      dom.close();
    }
  });

  it("mantém a visão quando o histórico falha", async () => {
    const dom = await mount("/v1/account/overview", { providers: [provider("claude", 42)] }, undefined);
    try {
      const { root } = dom;
      await waitFor(() => root.querySelector("[data-window='five_hour']") !== null, root);
      await new Promise((resolve) => {
        setTimeout(resolve, 80);
      });
      assert.equal(root.querySelector(".spark"), null);
      assert.equal(root.querySelector("[data-window='five_hour'] .staff") !== null, true);
    } finally {
      dom.close();
    }
  });
});

type Mounted = { root: HTMLElement; close: () => void };

async function mount(overviewPath: string, overview: Record<string, unknown>, history: unknown): Promise<Mounted> {
  const previous = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = input instanceof URL ? input.toString() : String(input);
    if (url.endsWith("/v1/session")) {
      return json({ provider: "github" });
    }
    if (url.endsWith(overviewPath)) {
      return json({ generated_at: "2026-10-30T12:00:00Z", stale_after_seconds: 1800, ...overview });
    }
    if (url.includes("/v1/account/samples") && history !== undefined) {
      return json(history);
    }
    return json({ message: "não usado" }, 404);
  }) as typeof fetch;
  const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
    url: "http://127.0.0.1:8787/",
    pretendToBeVisual: true,
  });
  const root = dom.window.document.querySelector("#app");
  if (!(root instanceof dom.window.HTMLElement)) {
    throw new Error("raiz ausente");
  }
  const app = await import("../src/app.ts");
  app.start(root);
  return {
    root,
    close: () => {
      globalThis.fetch = previous;
      dom.window.close();
    },
  };
}

function provider(name: string, used: number): Record<string, unknown> {
  return {
    provider: name,
    freshness: "current",
    observed_at: "2026-10-30T12:00:00Z",
    last_sample: {
      windows: [{ name: "five_hour", used, limit: 100, unit: "percent", resets_at: "2026-10-30T17:00:00Z" }],
    },
  };
}

function samples(): unknown {
  const at = (hour: number, used: number, id: string) => ({
    sample_id: id,
    provider: "claude",
    collected_at: `2026-10-30T${String(hour).padStart(2, "0")}:00:00Z`,
    windows: [{ name: "five_hour", used, resets_at: "2026-10-30T17:00:00Z" }],
  });
  return { data: [at(12, 42, "smp_c"), at(10, 10, "smp_a"), at(11, 25, "smp_b")], has_more: false };
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
