import { spawn } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import net from "node:net";
import { tmpdir } from "node:os";
import path from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { JSDOM } from "jsdom";

import { readingCopy } from "../src/view.ts";

const webRoot = path.resolve(import.meta.dirname, "..");
const repoRoot = path.resolve(webRoot, "..");
const canaries = ["CANARY_PROMPT", "CANARY_TOKEN", "acct_canary_uuid", "/home/canary/secret-project", "CANARY_SESSION"];
let serverStop: (() => void) | undefined;
process.on("unhandledRejection", (error: unknown) => {
  const message = error instanceof Error ? error.message : "rejeição não tratada";
  process.stderr.write(`${redact(message)}\n`);
  serverStop?.();
  process.exit(1);
});

async function main(): Promise<void> {
  const port = await freePort();
  const data = await mkdtemp(path.join(tmpdir(), "tracker-ui-"));
  const work = await mkdtemp(path.join(tmpdir(), "tracker-agent-"));
  const origin = `http://127.0.0.1:${port}`;
  const trackerBin = path.join(work, "tracker");
  const agentBin = path.join(work, "agent");
  await runAgent(repoRoot, ["go", "build", "-o", trackerBin, "./cmd/tracker"], 0);
  await runAgent(repoRoot, ["go", "build", "-o", agentBin, "./cmd/agent"], 0);
  const server = spawn(
    trackerBin,
    ["--addr", `127.0.0.1:${port}`, "--data", data, "--web", path.join(webRoot, "dist")],
    { cwd: repoRoot, detached: true, stdio: ["ignore", "pipe", "pipe"] },
  );
  serverStop = () => {
    if (server.pid === undefined) {
      return;
    }
    try {
      process.kill(-server.pid, "SIGTERM");
    } catch {
      server.kill("SIGTERM");
    }
  };
  let serverLog = "";
  server.stdout.on("data", (chunk: Buffer) => {
    serverLog += chunk.toString("utf8");
  });
  server.stderr.on("data", (chunk: Buffer) => {
    serverLog += chunk.toString("utf8");
  });
  try {
    await waitHealth(origin);
    const pages = await exercise(origin, work, agentBin);
    await shootGate(origin, pages);
    process.stdout.write("jornada da interface ok\n");
  } catch (error) {
    const message = error instanceof Error ? error.message : "falha na jornada";
    process.stderr.write(`${message}\n`);
    process.stderr.write(`${redact(serverLog)}\n`);
    process.exitCode = 1;
  } finally {
    serverStop?.();
    await rm(data, { recursive: true, force: true });
    await rm(work, { recursive: true, force: true });
  }
}

async function exercise(origin: string, work: string, agentBin: string): Promise<{ gate: string; reading: string }> {
  const dom = new JSDOM(`<!doctype html><html><body><div id="app"></div></body></html>`, {
    url: `${origin}/`,
    pretendToBeVisual: true,
  });
  const view = dom.window;
  const root = view.document.querySelector("#app");
  if (!(root instanceof view.HTMLElement)) {
    throw new Error("raiz ausente");
  }
  const app = await import("../src/app.ts");
  app.start(root);
  const gate = root.innerHTML;
  click(root, "[data-action='create-setup']");
  await waitFor(root, (text) => text.includes("Contas") && text.includes("Nenhuma conta com dispositivo."), "visão vazia");
  click(root, "[data-action='close-overlay']");
  await openScreen(view, root, "/dispositivos", "devices");
  click(root, "[data-action='enroll']");
  await waitFor(root, () => root.querySelector("[data-enrollment-code]") !== null, "código de enrolamento");
  const codeNode = root.querySelector("[data-enrollment-code]");
  const code = codeNode?.textContent?.trim() ?? "";
  if (code === "") {
    throw new Error("código vazio");
  }
  const tokenFile = path.join(work, "device.token");
  const spool = path.join(work, "spool.json");
  const isolatedPath = path.join(work, "bin-vazio");
  await mkdir(isolatedPath);
  // PATH vazio: o comando local `codex` desta máquina não substitui a fixture.
  const fixtureEnv = { ...process.env, PATH: isolatedPath };
  await runAgent(repoRoot, [
    agentBin, "enroll",
    "--api", origin,
    "--code", code,
    "--name", "notebook",
    "--out", tokenFile,
  ], 0, fixtureEnv);
  await runAgent(repoRoot, [
    agentBin, "run",
    "--api", origin,
    "--token-file", tokenFile,
    "--spool", spool,
    "--claude-file", path.join(repoRoot, "fixtures/synthetic/claude-cache.json"),
    "--codex-dir", path.join(repoRoot, "fixtures/synthetic/codex"),
  ], 0, fixtureEnv);
  click(root, "[data-action='close-overlay']");
  await openScreen(view, root, "/", "overview");
  await waitFor(root, (text) => text.includes("86%") && text.includes("25%"), "percentuais na visão");
  const reading = root.innerHTML;
  const operator = view.localStorage.getItem("tracker.operator_token") ?? "";
  if (operator === "") {
    throw new Error("token do operador ausente no navegador");
  }
  const overview = await readJson(origin, "/v1/overview", operator);
  const providers = overviewProviders(overview);
  for (const item of providers) {
    if (!isRecord(item)) {
      continue;
    }
    const provider = stringField(item, "provider");
    const headline = root.querySelector(`[data-provider='${provider}'] [data-headline]`);
    const expected = readingCopy(stringField(item, "freshness"), stringField(item, "observed_at"), stringField(item, "reason_code"), false);
    if (headline?.textContent !== expected.headline) {
      throw new Error(`rótulo de ${provider}: ${headline?.textContent ?? ""}`);
    }
  }
  const visible = root.textContent ?? "";
  for (const canary of canaries) {
    if (visible.includes(canary)) {
      throw new Error("canário visível na interface");
    }
  }
  const history = await readJson(origin, "/v1/samples?limit=20", operator);
  const encoded = JSON.stringify(history);
  for (const canary of canaries) {
    if (encoded.includes(canary)) {
      throw new Error("canário no histórico da API");
    }
  }
  await openScreen(view, root, "/historico", "history");
  await waitFor(root, (text) => text.includes("86%"), "histórico");
  await openScreen(view, root, "/alertas", "alerts");
  await waitFor(root, (text) => text.includes("Sem inscrição de push"), "estado do push");
  await openScreen(view, root, "/dispositivos", "devices");
  await waitFor(root, (text) => text.includes("notebook"), "dispositivo");
  click(root, "[data-action='revoke']");
  click(root, "[data-action='confirm-revoke']");
  await waitFor(root, (text) => text.includes("Revogado"), "revogação");
  await runAgent(repoRoot, [
    agentBin, "run",
    "--api", origin,
    "--token-file", tokenFile,
    "--spool", spool,
    "--claude-file", path.join(repoRoot, "fixtures/synthetic/claude-cache.json"),
    "--codex-dir", path.join(repoRoot, "fixtures/synthetic/codex"),
  ], 3, fixtureEnv);
  const cached = view.localStorage.getItem("tracker.overview.cache.operator");
  if (cached === null || cached === "") {
    throw new Error("a visão não foi guardada para uso offline");
  }
  const liveFetch = globalThis.fetch;
  globalThis.fetch = async () => {
    throw new Error("sem rede");
  };
  try {
    await openScreen(view, root, "/", "overview");
    await waitFor(root, (text) => text.includes("não são a leitura atual"), "cópia offline");
    const headlines = root.querySelectorAll("[data-headline]");
    if (headlines.length !== 2) {
      throw new Error("a cópia offline não mostrou os dois provedores");
    }
    for (const headline of headlines) {
      if (headline.textContent !== "Dados em cache") {
        throw new Error(`cópia offline rotulada como ${headline.textContent ?? ""}`);
      }
    }
  } finally {
    globalThis.fetch = liveFetch;
  }
  dom.window.close();
  return { gate, reading };
}

async function shootGate(origin: string, pages: { gate: string; reading: string }): Promise<void> {
  const firefox = process.env.FIREFOX_BIN ?? "firefox";
  const previews = [
    path.join(webRoot, "dist", "preview-reading.html"),
    path.join(webRoot, "dist", "preview-reading-dark.html"),
    path.join(webRoot, "dist", "preview-gate-dark.html"),
  ];
  try {
    await writePreview(previews[0] ?? "", pages.reading, "light");
    await writePreview(previews[1] ?? "", pages.reading, "dark");
    await writePreview(previews[2] ?? "", pages.gate, "dark");
    const shots: Array<[string, string, number]> = [
      [path.join(tmpdir(), "tracker-gate.png"), origin, 1280],
      [path.join(tmpdir(), "tracker-gate-dark.png"), `${origin}/preview-gate-dark.html`, 1280],
      [path.join(tmpdir(), "tracker-reading.png"), `${origin}/preview-reading.html`, 1680],
      [path.join(tmpdir(), "tracker-reading-dark.png"), `${origin}/preview-reading-dark.html`, 1680],
      [path.join(tmpdir(), "tracker-reading-dark-mobile.png"), `${origin}/preview-reading-dark.html`, 390],
    ];
    for (const [image, url, width] of shots) {
      const height = width === 390 ? 844 : width >= 1600 ? 1000 : 900;
      const result = await command(
        firefox,
        ["--headless", "--screenshot", image, `--window-size=${width},${height}`, url],
        20000,
      );
      if (result.code !== 0) {
        process.stdout.write("firefox não concluiu a captura da tela inicial\n");
        return;
      }
    }
    process.stdout.write("firefox capturou a tela inicial\n");
  } catch {
    process.stdout.write("firefox não pôde abrir a tela inicial\n");
  } finally {
    for (const file of previews) {
      await rm(file, { force: true });
    }
  }
}

async function writePreview(file: string, body: string, theme: "light" | "dark"): Promise<void> {
  const index = await readFile(path.join(webRoot, "dist", "index.html"), "utf8");
  const css = index.match(/href="([^"]+\.css)"/);
  const script = index.match(/<script>[\s\S]*?<\/script>/);
  if (css === null || css[1] === undefined || script === null) {
    throw new Error("css do build ausente");
  }
  const attr = theme === "dark"
    ? ' data-theme="dark" style="color-scheme: dark"'
    : ' data-theme="light" style="color-scheme: light"';
  const color = theme === "dark" ? "#0d1117" : "#f7f9fb";
  const page = `<!doctype html><html lang="pt-BR"${attr}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="theme-color" content="${color}"><link rel="stylesheet" href="${css[1]}"></head><body><div id="app">${body}</div></body></html>`;
  await writeFile(file, page);
}

async function openScreen(view: Window, root: HTMLElement, path: string, route: string): Promise<void> {
  const anchor = root.querySelector(`a[href='${path}']`);
  if (anchor !== null && "click" in anchor) {
    anchor.click();
  }
  if (root.querySelector("main")?.getAttribute("data-route") !== route) {
    view.history.pushState({}, "", path);
    view.dispatchEvent(new view.PopStateEvent("popstate"));
  }
  await waitFor(root, () => root.querySelector("main")?.getAttribute("data-route") === route, route);
}

function click(root: ParentNode, selector: string): void {
  const node = root.querySelector(selector);
  if (node === null || !("click" in node) || typeof node.click !== "function") {
    throw new Error(`controle ausente: ${selector}`);
  }
  node.click();
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

async function waitFor(root: HTMLElement, ready: (text: string) => boolean, label: string): Promise<void> {
  const started = Date.now();
  while (Date.now() - started < 8000) {
    if (ready(root.textContent ?? "")) {
      return;
    }
    await delay(40);
  }
  throw new Error(`${label}: ${redact(root.textContent ?? "").slice(0, 400)}`);
}

async function readJson(origin: string, pathName: string, token: string): Promise<unknown> {
  const response = await fetch(new URL(pathName, origin), {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!response.ok) {
    throw new Error(`GET ${pathName} ${response.status}`);
  }
  return await response.json();
}

function overviewProviders(value: unknown): unknown[] {
  if (!isRecord(value)) {
    throw new Error("visão sem provedores");
  }
  const accounts = value["accounts"];
  if (Array.isArray(accounts) && accounts.length > 0) {
    const first = accounts[0];
    if (!isRecord(first) || !Array.isArray(first["providers"])) {
      throw new Error("visão sem provedores");
    }
    return first["providers"];
  }
  const providers = value["providers"];
  if (!Array.isArray(providers)) {
    throw new Error("visão sem provedores");
  }
  return providers;
}

function stringField(record: Record<string, unknown>, key: string): string | undefined {
  const value = record[key];
  if (typeof value !== "string") {
    return undefined;
  }
  return value;
}

async function runAgent(cwd: string, args: string[], expected: number, env?: NodeJS.ProcessEnv): Promise<void> {
  const executable = args[0] ?? "go";
  const result = await command(executable, args.slice(1), 120000, cwd, env);
  if (result.code !== expected) {
    throw new Error(`agente saiu ${result.code}, esperado ${expected}: ${redact(result.output)}`);
  }
}

function command(
  bin: string,
  args: string[],
  timeoutMs: number,
  cwd?: string,
  env?: NodeJS.ProcessEnv,
): Promise<{ code: number; output: string }> {
  return new Promise((resolve, reject) => {
    const child = spawn(bin, args, { cwd, env, stdio: ["ignore", "pipe", "pipe"] });
    let output = "";
    child.stdout.on("data", (chunk: Buffer) => {
      output += chunk.toString("utf8");
    });
    child.stderr.on("data", (chunk: Buffer) => {
      output += chunk.toString("utf8");
    });
    const timer = setTimeout(() => {
      child.kill("SIGTERM");
      reject(new Error(`tempo esgotado: ${bin}`));
    }, timeoutMs);
    child.on("error", (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      resolve({ code: code ?? 1, output });
    });
  });
}

async function waitHealth(origin: string): Promise<void> {
  const started = Date.now();
  while (Date.now() - started < 90000) {
    try {
      const response = await fetch(`${origin}/healthz`);
      if (response.ok) {
        return;
      }
    } catch {
      await delay(200);
      continue;
    }
    await delay(200);
  }
  throw new Error("o servidor não respondeu /healthz");
}

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (typeof address !== "object" || address === null) {
        reject(new Error("porta indisponível"));
        return;
      }
      const port = address.port;
      server.close(() => resolve(port));
    });
    server.on("error", reject);
  });
}

function redact(value: string): string {
  return value
    .replace(/op_[A-Za-z0-9_-]+/g, "op_redacted")
    .replace(/enr_[A-Za-z0-9_-]+/g, "enr_redacted")
    .replace(/dev_[A-Za-z0-9_-]+/g, "dev_redacted");
}

await main();
