import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { JSDOM } from "jsdom";

import { operatorTokenKey } from "../src/view.ts";

describe("tela de alertas", () => {
  it("mostra o motivo da fila e da inscrição morta", async () => {
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      const url = input instanceof URL ? input.toString() : String(input);
      if (url.endsWith("/v1/settings")) {
        return json({
          stale_after_seconds: 1800,
          alert_threshold_percent: 80,
          timezone: "UTC",
          quiet_hours: { start: "", end: "" },
        });
      }
      if (url.endsWith("/v1/push/public-key")) {
        return json({ public_key: "BEl0test-public-key" });
      }
      if (url.endsWith("/v1/push/subscriptions")) {
        return json({
          data: [{
            id: "sub_um",
            endpoint: "https://push.example.test/one",
            created_at: "2026-10-30T12:00:00Z",
            status: "expired",
            last_error: "SUBSCRIPTION_EXPIRED",
          }],
        });
      }
      if (url.endsWith("/v1/push/deliveries")) {
        return json({
          data: [{
            id: "dly_um",
            kind: "threshold",
            provider: "claude",
            title: "Limiar de uso",
            body: "Claude five_hour em 90%.",
            status: "failed",
            created_at: "2026-10-30T12:00:00Z",
            detail: "QUEUE_FULL",
          }],
        });
      }
      return json({ message: "não usado" }, 404);
    }) as typeof fetch;
    const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
      url: "http://127.0.0.1:8787/alertas",
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
      await waitFor(() => root.textContent?.includes("A fila de avisos está cheia.") === true, root);
      const text = root.textContent ?? "";
      assert.equal(text.includes("Falhou"), true);
      assert.equal(text.includes("A inscrição não aceita mais avisos."), true);
      assert.equal(text.includes("QUEUE_FULL"), false);
      assert.equal(text.includes("SUBSCRIPTION_EXPIRED"), false);
      const status = root.querySelector("[data-delivery-status='failed']");
      assert.equal(status?.textContent, "Falhou");
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });
});

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
