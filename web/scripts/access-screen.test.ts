import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { JSDOM } from "jsdom";

import { operatorTokenKey } from "../src/view.ts";

const storedSecret = "sintetico-segredo-github";
const typedSecret = "segredo-novo-sintetico";

describe("tela de acesso", () => {
  it("mostra o cofre fechado sem fingir que não há segredo", async () => {
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL) => {
      const url = input instanceof URL ? input.toString() : String(input);
      if (url.endsWith("/v1/admin/secret-backend")) {
        return json({ kind: "own", ready: false });
      }
      return json({
        message: "O cofre próprio não está aberto.",
        error_code: "VAULT_UNAVAILABLE",
      }, 412);
    }) as typeof fetch;
    const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
      url: "http://127.0.0.1:8787/acesso",
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
      await waitFor(() => root.textContent?.includes("O cofre próprio não está aberto.") === true, root);
      assert.equal(root.querySelector("[data-secret='login']"), null);
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });

  it("guarda o segredo sem mostrá-lo de novo", async () => {
    const calls: Array<{ method: string; url: string; body: string }> = [];
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof URL ? input.toString() : String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : "";
      calls.push({ method, url, body });
      if (url.endsWith("/v1/admin/secret-backend")) {
        return json({ kind: "own", ready: true });
      }
      if (url.endsWith("/v1/admin/login-providers") && method === "GET") {
        return json({
          data: [
            {
              provider: "github",
              enabled: true,
              configured: true,
              secret_present: true,
              client_id: "id-pub",
              secret: storedSecret,
              updated_at: "2026-10-30T12:00:00Z",
            },
            { provider: "gitea", enabled: false, configured: false, secret_present: false },
            { provider: "google", enabled: false, configured: false, secret_present: false },
            { provider: "apple", enabled: false, configured: false, secret_present: false },
          ],
        });
      }
      if (url.endsWith("/v1/admin/login-providers/github") && method === "PUT") {
        return json({
          provider: "github",
          enabled: true,
          configured: true,
          secret_present: true,
          client_id: "id-pub",
          updated_at: "2026-10-30T13:00:00Z",
        });
      }
      return json({ message: "não encontrado", error_code: "NOT_FOUND" }, 404);
    }) as typeof fetch;

    const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
      url: "http://127.0.0.1:8787/acesso",
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
      await waitFor(() => root.textContent?.includes("Cofre próprio aberto.") === true, root);
      assert.equal(root.textContent?.includes(storedSecret), false);
      assert.equal(root.textContent?.includes("GitHub"), true);
      assert.equal(root.textContent?.includes("Onde obter"), true);
      assert.equal(root.textContent?.includes("OAuth App"), true);
      assert.equal(root.textContent?.includes("https://git.exemplo.com"), true);
      assert.equal(root.textContent?.includes(".apps.googleusercontent.com"), true);
      assert.equal(root.textContent?.includes("-----BEGIN PRIVATE KEY-----"), true);
      assert.equal(root.textContent?.includes("http://127.0.0.1:8787/v1/login/github/callback"), true);
      assert.equal(root.textContent?.includes("http://127.0.0.1:8787/v1/login/apple/callback"), true);
      assert.equal(root.querySelectorAll(".guide").length, 4);
      const form = root.querySelector("form[data-provider='github']");
      if (!(form instanceof view.HTMLFormElement)) {
        throw new Error("formulário ausente");
      }
      const secret = form.querySelector("[data-secret='login']");
      if (!(secret instanceof view.HTMLInputElement)) {
        throw new Error("campo de segredo ausente");
      }
      assert.equal(secret.type, "password");
      assert.equal(secret.value, "");
      const appleSecret = root.querySelector("form[data-provider='apple'] [data-secret='login']");
      if (!(appleSecret instanceof view.HTMLTextAreaElement)) {
        throw new Error("campo da Apple não é texto de várias linhas");
      }
      assert.equal(secret.placeholder, "Deixe vazio para manter o segredo guardado.");
      secret.value = typedSecret;
      form.dispatchEvent(new view.Event("submit", { bubbles: true, cancelable: true }));
      await waitFor(() => {
        const again = root.querySelector("form[data-provider='github'] [data-secret='login']");
        return again instanceof view.HTMLInputElement
          && again.value === ""
          && root.textContent?.includes("não volta para esta tela") === true;
      }, root);
      assert.equal(root.textContent?.includes(typedSecret), false);
      assert.equal(root.textContent?.includes(storedSecret), false);
      const put = calls.find((call) => call.method === "PUT" && call.url.endsWith("/github"));
      assert.equal(put?.body.includes(typedSecret), true);
      assert.equal(put?.body.includes(storedSecret), false);
    } finally {
      globalThis.fetch = previous;
      dom.window.close();
    }
  });

  it("edita o template do Sabiá sem mostrar a chave", async () => {
    const hiddenKey = "sb_deploy_nao_mostrar";
    const calls: Array<{ method: string; url: string; body: string }> = [];
    const previous = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof URL ? input.toString() : String(input);
      const method = init?.method ?? "GET";
      const body = typeof init?.body === "string" ? init.body : "";
      calls.push({ method, url, body });
      if (url.endsWith("/v1/admin/secret-backend")) {
        return json({ kind: "own", ready: true });
      }
      if (url.endsWith("/v1/admin/login-providers") && method === "GET") {
        return json({
          data: [
            { provider: "github", enabled: false, configured: false, secret_present: false },
            { provider: "gitea", enabled: false, configured: false, secret_present: false },
            { provider: "google", enabled: false, configured: false, secret_present: false },
            { provider: "apple", enabled: false, configured: false, secret_present: false },
          ],
        });
      }
      if (url.endsWith("/v1/admin/sabia") && method === "GET") {
        return json({
          origin: "https://sabia.exemplo",
          deploy_key_present: true,
          deploy_key: hiddenKey,
          template: {
            name: "Alerta de uso",
            subject: "Uso de {{provider}}",
            html: "<p>{{used}}</p>",
            text: "{{used}}",
          },
        });
      }
      if (url.endsWith("/v1/admin/sabia") && method === "PUT") {
        return json({
          origin: "https://sabia.exemplo",
          deploy_key_present: true,
          template: {
            name: "Alerta de uso",
            subject: "Uso de {{provider}}",
            html: "<p>{{used}}</p>",
            text: "{{used}}",
          },
        });
      }
      if (url.endsWith("/v1/admin/sabia/plan") && method === "POST") {
        return json({
          changes: [{ id: "usage-alert-v1", operation: "criar", confirmation_required: true }],
        });
      }
      if (url.endsWith("/v1/admin/sabia/apply") && method === "POST") {
        return json({ applied: 1, unchanged: 0 });
      }
      return json({ message: "não encontrado", error_code: "NOT_FOUND" }, 404);
    }) as typeof fetch;
    const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
      url: "http://127.0.0.1:8787/acesso",
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
      await waitFor(() => root.textContent?.includes("Sincronizar publica no Sabiá") === true, root);
      assert.equal(root.textContent?.includes(hiddenKey), false);
      assert.equal(root.textContent?.includes("Não apaga templates"), true);
      const form = root.querySelector("form[data-panel='sabia']");
      if (!(form instanceof view.HTMLFormElement)) {
        throw new Error("formulário do Sabiá ausente");
      }
      const secret = form.querySelector("[data-secret='sabia']");
      if (!(secret instanceof view.HTMLInputElement)) {
        throw new Error("chave ausente");
      }
      assert.equal(secret.type, "password");
      assert.equal(secret.value, "");
      const plan = form.querySelector("[data-action='sabia-plan']");
      if (!(plan instanceof view.HTMLButtonElement)) {
        throw new Error("plano ausente");
      }
      plan.click();
      await waitFor(() => root.textContent?.includes("Confirmar usage-alert-v1") === true, root);
      const box = root.querySelector("[data-confirm='usage-alert-v1']");
      if (!(box instanceof view.HTMLInputElement)) {
        throw new Error("confirmação ausente");
      }
      box.checked = true;
      const sync = root.querySelector("[data-action='sabia-apply']");
      if (!(sync instanceof view.HTMLButtonElement)) {
        throw new Error("sincronizar ausente");
      }
      sync.click();
      await waitFor(() => root.textContent?.includes("Sincronizado com o Sabiá") === true, root);
      assert.equal(root.textContent?.includes(hiddenKey), false);
      assert.equal(root.textContent?.includes("Aplicado: 1"), true);
      const puts = calls.filter((call) => call.method === "PUT" && call.url.endsWith("/v1/admin/sabia"));
      assert.equal(puts.length >= 1, true);
      for (const put of puts) {
        assert.equal(put.body.includes(hiddenKey), false);
      }
      const apply = calls.find((call) => call.method === "POST" && call.url.endsWith("/v1/admin/sabia/apply"));
      assert.equal(apply?.body.includes("usage-alert-v1"), true);
      const planCall = calls.find((call) => call.method === "POST" && call.url.endsWith("/plan"));
      assert.equal(planCall === undefined, false);
      const planIndex = calls.findIndex((call) => call.url.endsWith("/plan"));
      const firstPut = calls.findIndex((call) => call.method === "PUT" && call.url.endsWith("/v1/admin/sabia"));
      assert.equal(firstPut >= 0 && firstPut < planIndex, true);
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
