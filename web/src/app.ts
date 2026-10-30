import { apiRequest, type ApiResult } from "./api.ts";
import {
  deliveryLabel,
  formatInstant,
  formatUsed,
  operatorTokenKey,
  overviewCacheKey,
  parseCachedOverview,
  parseDeliveries,
  parseDevices,
  parseHistory,
  isRecord,
  parseOverview,
  parseRoute,
  parseSettings,
  parseSubscriptions,
  primaryWindow,
  projectionCopy,
  providerLabel,
  readingCopy,
  staffLevel,
  staffTone,
  subscriptionLabel,
  windowLabel,
  type DeliveryView,
  type DeviceView,
  type HistorySample,
  type Overview,
  type ProviderCard,
  type RouteName,
  type SettingsView,
  type SubscriptionView,
} from "./view.ts";

type ScreenState = { type: "signed-out" } | { type: "ready" };

type OverlayState =
  | { type: "none" }
  | { type: "operator-token"; token: string }
  | { type: "enrollment-code"; code: string; expiresAt: string }
  | { type: "revoke-device"; id: string; name: string };

type PageState =
  | { route: "overview"; overview?: Overview; fromCache: boolean; problem: string }
  | { route: "history"; samples: HistorySample[]; hasMore: boolean; nextCursor: string; problem: string }
  | { route: "devices"; devices: DeviceView[]; problem: string }
  | {
      route: "alerts";
      settings?: SettingsView;
      subscriptions: SubscriptionView[];
      deliveries: DeliveryView[];
      push: "loading" | "ready" | "unconfigured";
      problem: string;
    };

type AppState = {
  token: string;
  screen: ScreenState;
  overlay: OverlayState;
  page: PageState;
  notice: string;
  loadId: number;
};

export function start(root: HTMLElement): void {
  const view = root.ownerDocument.defaultView;
  if (view === null) {
    throw new Error("janela ausente");
  }
  const state: AppState = {
    token: view.localStorage.getItem(operatorTokenKey) ?? "",
    screen: { type: "signed-out" },
    overlay: { type: "none" },
    page: { route: "overview", fromCache: false, problem: "" },
    notice: "",
    loadId: 0,
  };
  if (state.token !== "") {
    state.screen = { type: "ready" };
  }
  const paint = () => {
    render(root, view, state);
  };
  view.addEventListener("hashchange", () => {
    if (state.screen.type !== "ready") {
      return;
    }
    void loadRoute(view, state, parseRoute(view.location.hash), paint);
  });
  paint();
  if (state.screen.type === "ready") {
    void loadRoute(view, state, parseRoute(view.location.hash), paint);
  }
  registerWorker(view);
}

function render(root: HTMLElement, view: Window, state: AppState): void {
  const doc = root.ownerDocument;
  root.replaceChildren();
  if (state.screen.type === "signed-out") {
    root.append(signedOut(doc, view, state, () => render(root, view, state)));
    return;
  }
  const shell = doc.createElement("div");
  shell.className = "shell";
  shell.append(topbar(doc, view, state, () => render(root, view, state)));
  const main = doc.createElement("main");
  main.id = "conteudo";
  main.dataset.route = state.page.route;
  if (state.notice !== "") {
    main.append(note(doc, state.notice, "notice"));
  }
  if (state.page.route === "overview") {
    main.append(overviewView(doc, view, state, () => render(root, view, state)));
  } else if (state.page.route === "history") {
    main.append(historyView(doc, view, state, () => render(root, view, state)));
  } else if (state.page.route === "devices") {
    main.append(devicesView(doc, view, state, () => render(root, view, state)));
  } else {
    main.append(alertsView(doc, view, state, () => render(root, view, state)));
  }
  shell.append(main);
  const overlay = overlayView(doc, view, state, () => render(root, view, state));
  if (overlay) {
    shell.append(overlay);
  }
  root.append(shell);
  const realm = root.ownerDocument.defaultView;
  const dialog = root.querySelector("[role='dialog']");
  if (realm !== null && dialog instanceof realm.HTMLElement) {
    dialog.focus();
  }
}

function signedOut(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const section = doc.createElement("section");
  section.className = "gate";
  section.append(el(doc, "p", "mark", "Rastreador"));
  section.append(el(doc, "h1", "gate-title", "Uso local, leitura no navegador"));
  section.append(el(doc, "p", "lede", "O agente fica na sua máquina. Este navegador só guarda o token do operador e percentuais já coletados."));
  if (state.notice !== "") {
    section.append(note(doc, state.notice, "notice"));
  }
  const form = doc.createElement("form");
  form.className = "stack";
  const label = el(doc, "label", "", "Token do operador");
  label.setAttribute("for", "operator-token");
  const input = doc.createElement("input");
  input.id = "operator-token";
  input.name = "token";
  input.autocomplete = "off";
  input.spellcheck = false;
  const enter = button(doc, "Entrar", "primary");
  enter.type = "submit";
  form.append(label, input, enter);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const token = input.value.trim();
    if (token === "") {
      state.notice = "Cole o token do operador.";
      paint();
      return;
    }
    state.token = token;
    view.localStorage.setItem(operatorTokenKey, token);
    state.screen = { type: "ready" };
    state.notice = "";
    state.overlay = { type: "none" };
    paint();
    void loadRoute(view, state, parseRoute(view.location.hash), paint);
  });
  const create = button(doc, "Criar esta instalação", "");
  create.type = "button";
  create.dataset.action = "create-setup";
  create.addEventListener("click", () => {
    void createSetup(view, state, paint);
  });
  section.append(form, create);
  return section;
}

function topbar(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const header = doc.createElement("header");
  header.className = "topbar";
  const brand = el(doc, "a", "mark", "Rastreador");
  brand.setAttribute("href", "#/");
  const nav = doc.createElement("nav");
  nav.append(
    link(doc, "#/", "Visão", state.page.route === "overview"),
    link(doc, "#/historico", "Histórico", state.page.route === "history"),
    link(doc, "#/dispositivos", "Dispositivos", state.page.route === "devices"),
    link(doc, "#/alertas", "Alertas", state.page.route === "alerts"),
  );
  const exit = button(doc, "Sair deste navegador", "quiet");
  exit.dataset.action = "sign-out";
  exit.addEventListener("click", () => {
    state.loadId += 1;
    state.token = "";
    state.screen = { type: "signed-out" };
    state.overlay = { type: "none" };
    state.notice = "";
    state.page = { route: "overview", fromCache: false, problem: "" };
    view.localStorage.removeItem(operatorTokenKey);
    view.localStorage.removeItem(overviewCacheKey);
    paint();
  });
  header.append(brand, nav, exit);
  return header;
}

function overviewView(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const page = state.page.route === "overview" ? state.page : { route: "overview" as const, fromCache: false, problem: "" };
  const wrap = doc.createElement("section");
  wrap.append(el(doc, "h1", "", "Duas medições"));
  wrap.append(el(doc, "p", "lede", "Cada régua sobe com o percentual usado. A linha de latão marca 80%."));
  const refresh = button(doc, "Atualizar", "");
  refresh.dataset.action = "refresh";
  refresh.addEventListener("click", () => {
    void loadOverview(view, state, paint);
  });
  wrap.append(refresh);
  if (page.problem !== "") {
    wrap.append(note(doc, page.problem, "problem"));
  }
  if (page.overview === undefined) {
    wrap.append(el(doc, "p", "", "Carregando a visão."));
    return wrap;
  }
  if (page.fromCache) {
    wrap.append(note(doc, "Estes dados estão em cache e não são a leitura atual.", "cache"));
  }
  if (page.overview.generatedAt !== "" && !page.fromCache) {
    wrap.append(el(doc, "p", "meta", `Servidor consultado em ${formatInstant(page.overview.generatedAt)}.`));
  }
  const board = doc.createElement("div");
  board.className = "board";
  for (const card of page.overview.providers) {
    board.append(providerCard(doc, card, page.fromCache));
  }
  wrap.append(board);
  return wrap;
}

function providerCard(doc: Document, card: ProviderCard, fromCache: boolean): HTMLElement {
  const article = doc.createElement("article");
  article.className = "provider";
  article.dataset.provider = card.provider;
  const copy = readingCopy(card.freshness, card.observedAt, card.reasonCode, fromCache);
  const primary = primaryWindow(card.windows);
  const used = primary === undefined ? 0 : primary.used;
  const staff = doc.createElement("div");
  staff.className = "staff";
  staff.dataset.tone = primary === undefined ? "calm" : staffTone(used);
  staff.style.setProperty("--level", String(staffLevel(used)));
  const water = doc.createElement("div");
  water.className = "staff-water";
  const mark = doc.createElement("div");
  mark.className = "staff-mark";
  mark.title = "Limiar de 80%";
  staff.append(water, mark);
  const text = doc.createElement("div");
  text.className = "provider-copy";
  text.append(el(doc, "h2", "", providerLabel(card.provider)));
  const headline = el(doc, "p", "headline", copy.headline);
  headline.dataset.headline = copy.headline;
  text.append(headline);
  text.append(el(doc, "p", "", copy.detail));
  if (primary !== undefined) {
    const figure = el(doc, "p", "figure", `${formatUsed(used)}%`);
    figure.dataset.used = formatUsed(used);
    text.append(figure);
  }
  if (card.plan !== undefined && card.plan !== "") {
    text.append(el(doc, "p", "meta", `Plano ${card.plan}.`));
  }
  const list = doc.createElement("ul");
  for (const item of card.windows) {
    const row = doc.createElement("li");
    row.textContent = `${windowLabel(item.name)}: ${formatUsed(item.used)}% · reinicia ${formatInstant(item.resetsAt)}`;
    list.append(row);
  }
  if (list.childElementCount > 0) {
    text.append(list);
  }
  if (card.freshness === "unavailable" && card.windows.length > 0 && !fromCache) {
    text.append(el(doc, "p", "meta", "A última amostra conhecida continua visível. Não é a leitura atual."));
  }
  if (card.projection !== undefined && (card.freshness === "current" || card.freshness === "stale" || fromCache)) {
    text.append(el(doc, "p", "projection", projectionCopy(card.projection)));
  }
  article.append(staff, text);
  return article;
}

function historyView(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const page = state.page.route === "history"
    ? state.page
    : { route: "history" as const, samples: [], hasMore: false, nextCursor: "", problem: "" };
  const wrap = doc.createElement("section");
  wrap.append(el(doc, "h1", "", "Histórico"));
  wrap.append(el(doc, "p", "lede", "Cada linha é uma amostra aceita. Reenviar o mesmo snapshot não cria outra."));
  if (page.problem !== "") {
    wrap.append(note(doc, page.problem, "problem"));
  }
  if (page.samples.length === 0 && page.problem === "") {
    wrap.append(el(doc, "p", "", "Nenhuma amostra no histórico."));
  }
  const list = doc.createElement("ol");
  list.className = "history";
  for (const sample of page.samples) {
    const item = doc.createElement("li");
    const title = el(doc, "h2", "", `${providerLabel(sample.provider)} · ${formatInstant(sample.collectedAt)}`);
    item.append(title);
    if (sample.plan !== undefined && sample.plan !== "") {
      item.append(el(doc, "p", "meta", `Plano ${sample.plan}.`));
    }
    const windows = doc.createElement("ul");
    for (const window of sample.windows) {
      const row = doc.createElement("li");
      row.textContent = `${windowLabel(window.name)}: ${formatUsed(window.used)}% · reinicia ${formatInstant(window.resetsAt)}`;
      windows.append(row);
    }
    item.append(windows);
    list.append(item);
  }
  wrap.append(list);
  if (page.hasMore && page.nextCursor !== "") {
    const more = button(doc, "Mostrar mais", "");
    more.dataset.action = "more-history";
    more.addEventListener("click", () => {
      void loadHistory(view, state, page.nextCursor, paint);
    });
    wrap.append(more);
  }
  return wrap;
}

function devicesView(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const page = state.page.route === "devices" ? state.page : { route: "devices" as const, devices: [], problem: "" };
  const wrap = doc.createElement("section");
  wrap.append(el(doc, "h1", "", "Dispositivos"));
  wrap.append(el(doc, "p", "lede", "O código de enrolamento aparece uma vez e vale quinze minutos. Revogar congela a fila daquele token."));
  const enroll = button(doc, "Gerar código de enrolamento", "primary");
  enroll.dataset.action = "enroll";
  enroll.addEventListener("click", () => {
    void createEnrollment(view, state, paint);
  });
  wrap.append(enroll);
  if (page.problem !== "") {
    wrap.append(note(doc, page.problem, "problem"));
  }
  if (page.devices.length === 0) {
    wrap.append(el(doc, "p", "", "Nenhum dispositivo enrolado."));
  }
  const list = doc.createElement("ul");
  list.className = "devices";
  for (const device of page.devices) {
    const item = doc.createElement("li");
    item.append(el(doc, "h2", "", device.name));
    item.append(el(doc, "p", "meta", `Criado em ${formatInstant(device.createdAt)}.`));
    if (device.revokedAt !== undefined) {
      item.append(el(doc, "p", "headline", `Revogado em ${formatInstant(device.revokedAt)}.`));
    } else {
      const revoke = button(doc, "Revogar", "");
      revoke.dataset.action = "revoke";
      revoke.dataset.deviceId = device.id;
      revoke.addEventListener("click", () => {
        state.overlay = { type: "revoke-device", id: device.id, name: device.name };
        paint();
      });
      item.append(revoke);
    }
    list.append(item);
  }
  wrap.append(list);
  return wrap;
}

function alertsView(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const page = state.page.route === "alerts"
    ? state.page
    : { route: "alerts" as const, subscriptions: [], deliveries: [], push: "loading" as const, problem: "" };
  const wrap = doc.createElement("section");
  wrap.append(el(doc, "h1", "", "Alertas"));
  wrap.append(el(doc, "p", "lede", "O aviso usa só título, texto, provedor e tipo. No iPhone ele depende do aplicativo na tela de início e pode atrasar."));
  if (page.problem !== "") {
    wrap.append(note(doc, page.problem, "problem"));
  }
  if (page.push === "unconfigured") {
    wrap.append(note(doc, "Web Push não está configurado nesta instalação.", "problem"));
  } else if (page.push === "ready") {
    wrap.append(el(doc, "p", "", "Web Push está ativo nesta instalação."));
    const subscribe = button(doc, "Ativar avisos neste navegador", "primary");
    subscribe.dataset.action = "subscribe";
    subscribe.addEventListener("click", () => {
      void subscribeHere(view, state, paint);
    });
    wrap.append(subscribe);
  }
  if (page.settings !== undefined) {
    wrap.append(settingsForm(doc, view, state, page.settings, paint));
  }
  wrap.append(el(doc, "h2", "", "Inscrições"));
  if (page.subscriptions.length === 0) {
    wrap.append(el(doc, "p", "", "Nenhuma inscrição."));
  }
  const subs = doc.createElement("ul");
  for (const item of page.subscriptions) {
    const row = doc.createElement("li");
    row.textContent = `${subscriptionLabel(item.status)} · ${item.endpoint} · ${formatInstant(item.createdAt)}`;
    if (item.lastError !== undefined && item.lastError !== "") {
      row.append(el(doc, "span", "meta", ` ${item.lastError}`));
    }
    const remove = button(doc, "Remover", "quiet");
    remove.addEventListener("click", () => {
      void removeSubscription(view, state, item.id, paint);
    });
    row.append(remove);
    subs.append(row);
  }
  wrap.append(subs);
  wrap.append(el(doc, "h2", "", "Entregas"));
  if (page.deliveries.length === 0) {
    wrap.append(el(doc, "p", "", "Nenhum aviso registrado."));
  }
  const deliveries = doc.createElement("ul");
  deliveries.className = "deliveries";
  for (const item of page.deliveries) {
    const row = doc.createElement("li");
    const status = el(doc, "p", "headline", deliveryLabel(item.status));
    status.dataset.deliveryStatus = item.status;
    row.append(status);
    row.append(el(doc, "p", "", `${providerLabel(item.provider)} · ${item.title}`));
    row.append(el(doc, "p", "", item.body));
    row.append(el(doc, "p", "meta", formatInstant(item.createdAt)));
    deliveries.append(row);
  }
  wrap.append(deliveries);
  return wrap;
}

function settingsForm(doc: Document, view: Window, state: AppState, settings: SettingsView, paint: () => void): HTMLFormElement {
  const form = doc.createElement("form");
  form.className = "stack";
  form.append(field(doc, "Desatualizado após (minutos)", "number", "stale-minutes", String(Math.round(settings.staleAfterSeconds / 60))));
  form.append(field(doc, "Limiar do aviso (%)", "number", "threshold", String(settings.alertThresholdPercent)));
  form.append(field(doc, "Fuso horário", "text", "timezone", settings.timezone));
  const quietLabel = el(doc, "label", "check", "Horário silencioso");
  const quiet = doc.createElement("input");
  quiet.type = "checkbox";
  quiet.name = "quiet-enabled";
  quiet.checked = settings.quietStart !== "" && settings.quietEnd !== "";
  quietLabel.prepend(quiet);
  form.append(quietLabel);
  form.append(field(doc, "Início", "time", "quiet-start", settings.quietStart));
  form.append(field(doc, "Fim", "time", "quiet-end", settings.quietEnd));
  const save = button(doc, "Guardar preferências", "primary");
  save.type = "submit";
  form.append(save);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void saveSettings(view, state, new FormData(form), paint);
  });
  return form;
}

function overlayView(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement | undefined {
  if (state.overlay.type === "none") {
    return undefined;
  }
  const overlay = state.overlay;
  const backdrop = doc.createElement("div");
  backdrop.className = "backdrop";
  const dialog = doc.createElement("div");
  dialog.className = "dialog";
  dialog.role = "dialog";
  dialog.ariaModal = "true";
  dialog.tabIndex = -1;
  const close = button(doc, "Fechar", "quiet");
  close.dataset.action = "close-overlay";
  close.addEventListener("click", () => {
    state.overlay = { type: "none" };
    paint();
  });
  if (overlay.type === "operator-token") {
    dialog.append(el(doc, "h2", "", "Guarde o token do operador"));
    dialog.append(el(doc, "p", "", "O servidor mostra este token uma única vez. Ele também ficou neste navegador. Quem perder os dois perde o acesso por esta API."));
    const code = el(doc, "p", "secret", overlay.token);
    code.dataset.operatorToken = "shown";
    dialog.append(code);
  } else if (overlay.type === "enrollment-code") {
    dialog.append(el(doc, "h2", "", "Código de enrolamento"));
    dialog.append(el(doc, "p", "", `Válido até ${formatInstant(overlay.expiresAt)}. Use no agente e não o envie de novo.`));
    const code = el(doc, "p", "secret", overlay.code);
    code.dataset.enrollmentCode = overlay.code;
    dialog.append(code);
    dialog.append(el(doc, "p", "command", `go run ./cmd/agent enroll --api ${view.location.origin} --code ${overlay.code} --name notebook --out device.token`));
  } else {
    dialog.append(el(doc, "h2", "", `Revogar ${overlay.name}?`));
    dialog.append(el(doc, "p", "", "O agente com este token passa a ser recusado. A fila local não reenvia com a mesma credencial."));
    const confirm = button(doc, "Revogar dispositivo", "primary");
    confirm.dataset.action = "confirm-revoke";
    confirm.addEventListener("click", () => {
      void confirmRevoke(view, state, overlay.id, paint);
    });
    dialog.append(confirm);
  }
  dialog.append(close);
  backdrop.append(dialog);
  backdrop.addEventListener("click", (event) => {
    if (event.target !== backdrop) {
      return;
    }
    state.overlay = { type: "none" };
    paint();
  });
  return backdrop;
}

async function createSetup(view: Window, state: AppState, paint: () => void): Promise<void> {
  const result = await apiRequest(view.location.origin, "/v1/setup", "", "POST");
  if (!result.ok) {
    state.notice = result.message;
    paint();
    return;
  }
  const token = readToken(result, "operator_token");
  if (token === "") {
    state.notice = "A instalação não devolveu o token.";
    paint();
    return;
  }
  state.token = token;
  view.localStorage.setItem(operatorTokenKey, token);
  state.screen = { type: "ready" };
  state.notice = "";
  state.overlay = { type: "operator-token", token };
  paint();
  void loadRoute(view, state, parseRoute(view.location.hash), paint);
}

async function loadRoute(view: Window, state: AppState, route: RouteName, paint: () => void): Promise<void> {
  if (route === "overview") {
    await loadOverview(view, state, paint);
    return;
  }
  if (route === "history") {
    state.page = { route: "history", samples: [], hasMore: false, nextCursor: "", problem: "" };
    paint();
    await loadHistory(view, state, "", paint);
    return;
  }
  if (route === "devices") {
    await loadDevices(view, state, paint);
    return;
  }
  await loadAlerts(view, state, paint);
}

async function loadOverview(view: Window, state: AppState, paint: () => void): Promise<void> {
  const loadId = begin(state);
  state.page = { route: "overview", fromCache: false, problem: "" };
  paint();
  const result = await apiRequest(view.location.origin, "/v1/overview", state.token, "GET");
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    if (result.errorCode === "NETWORK") {
      const cached = readCache(view);
      if (cached) {
        state.page = { route: "overview", overview: cached, fromCache: true, problem: "" };
        paint();
        return;
      }
    }
    state.page = { route: "overview", fromCache: false, problem: result.message };
    paint();
    return;
  }
  const parsed = parseOverview(result.body);
  if (!parsed.ok) {
    state.page = { route: "overview", fromCache: false, problem: parsed.message };
    paint();
    return;
  }
  view.localStorage.setItem(overviewCacheKey, JSON.stringify({ saved_at: new Date().toISOString(), overview: result.body }));
  state.page = { route: "overview", overview: parsed.value, fromCache: false, problem: "" };
  paint();
}

async function loadHistory(view: Window, state: AppState, cursor: string, paint: () => void): Promise<void> {
  const loadId = begin(state);
  const path = cursor === "" ? "/v1/samples?limit=20" : `/v1/samples?limit=20&cursor=${encodeURIComponent(cursor)}`;
  const result = await apiRequest(view.location.origin, path, state.token, "GET");
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    state.page = { route: "history", samples: [], hasMore: false, nextCursor: "", problem: result.message };
    paint();
    return;
  }
  const parsed = parseHistory(result.body);
  if (!parsed.ok) {
    state.page = { route: "history", samples: [], hasMore: false, nextCursor: "", problem: parsed.message };
    paint();
    return;
  }
  const previous = state.page.route === "history" ? state.page.samples : [];
  const samples = cursor === "" ? parsed.value.samples : previous.concat(parsed.value.samples);
  state.page = {
    route: "history",
    samples,
    hasMore: parsed.value.hasMore,
    nextCursor: parsed.value.nextCursor,
    problem: "",
  };
  paint();
}

async function loadDevices(view: Window, state: AppState, paint: () => void): Promise<void> {
  const loadId = begin(state);
  const result = await apiRequest(view.location.origin, "/v1/devices", state.token, "GET");
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    state.page = { route: "devices", devices: [], problem: result.message };
    paint();
    return;
  }
  const parsed = parseDevices(result.body);
  if (!parsed.ok) {
    state.page = { route: "devices", devices: [], problem: parsed.message };
    paint();
    return;
  }
  state.page = { route: "devices", devices: parsed.value, problem: "" };
  paint();
}

async function loadAlerts(view: Window, state: AppState, paint: () => void): Promise<void> {
  const loadId = begin(state);
  state.page = { route: "alerts", subscriptions: [], deliveries: [], push: "loading", problem: "" };
  paint();
  const [settingsResult, pushResult, subscriptionResult, deliveryResult] = await Promise.all([
    apiRequest(view.location.origin, "/v1/settings", state.token, "GET"),
    apiRequest(view.location.origin, "/v1/push/public-key", state.token, "GET"),
    apiRequest(view.location.origin, "/v1/push/subscriptions", state.token, "GET"),
    apiRequest(view.location.origin, "/v1/push/deliveries", state.token, "GET"),
  ]);
  if (loadId !== state.loadId) {
    return;
  }
  const failure = firstAuthFailure([settingsResult, pushResult, subscriptionResult, deliveryResult]);
  if (failure) {
    loseSession(view, state, failure.message, paint);
    return;
  }
  const subscriptions = subscriptionResult.ok ? parseSubscriptions(subscriptionResult.body) : undefined;
  const deliveries = deliveryResult.ok ? parseDeliveries(deliveryResult.body) : undefined;
  let problem = "";
  let settings: SettingsView | undefined;
  if (!settingsResult.ok) {
    problem = settingsResult.message;
  } else {
    const parsed = parseSettings(settingsResult.body);
    if (!parsed.ok) {
      problem = parsed.message;
    } else {
      settings = parsed.value;
    }
  }
  state.page = {
    route: "alerts",
    subscriptions: subscriptions !== undefined && subscriptions.ok ? subscriptions.value : [],
    deliveries: deliveries !== undefined && deliveries.ok ? deliveries.value : [],
    push: pushState(pushResult),
    problem,
  };
  if (settings !== undefined) {
    state.page.settings = settings;
  }
  paint();
}

async function createEnrollment(view: Window, state: AppState, paint: () => void): Promise<void> {
  const result = await apiRequest(view.location.origin, "/v1/enrollment-codes", state.token, "POST");
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    if (state.page.route === "devices") {
      state.page.problem = result.message;
    }
    paint();
    return;
  }
  const code = readToken(result, "code");
  const expiresAt = readField(result.body, "expires_at");
  if (code === "" || expiresAt === "") {
    if (state.page.route === "devices") {
      state.page.problem = "O código de enrolamento chegou incompleto.";
    }
    paint();
    return;
  }
  state.overlay = { type: "enrollment-code", code, expiresAt };
  paint();
}

async function confirmRevoke(view: Window, state: AppState, id: string, paint: () => void): Promise<void> {
  const result = await apiRequest(view.location.origin, `/v1/devices/${encodeURIComponent(id)}/revoke`, state.token, "POST");
  state.overlay = { type: "none" };
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    state.notice = result.message;
    paint();
    return;
  }
  state.notice = "Dispositivo revogado.";
  paint();
  await loadDevices(view, state, paint);
}

async function saveSettings(view: Window, state: AppState, form: FormData, paint: () => void): Promise<void> {
  const minutes = Number(form.get("stale-minutes"));
  const threshold = Number(form.get("threshold"));
  const timezone = String(form.get("timezone") ?? "").trim();
  const body: Record<string, unknown> = {
    stale_after_seconds: Math.round(minutes * 60),
    alert_threshold_percent: threshold,
    timezone,
  };
  if (form.get("quiet-enabled") !== null) {
    body.quiet_hours = {
      start: String(form.get("quiet-start") ?? ""),
      end: String(form.get("quiet-end") ?? ""),
    };
  }
  const result = await apiRequest(view.location.origin, "/v1/settings", state.token, "PUT", body);
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    state.notice = result.message;
    paint();
    return;
  }
  state.notice = "Preferências guardadas.";
  await loadAlerts(view, state, paint);
}

async function subscribeHere(view: Window, state: AppState, paint: () => void): Promise<void> {
  if (typeof Notification === "undefined" || !("serviceWorker" in view.navigator)) {
    state.notice = "Este navegador não expõe Web Push.";
    paint();
    return;
  }
  const keyResult = await apiRequest(view.location.origin, "/v1/push/public-key", state.token, "GET");
  if (!keyResult.ok) {
    state.notice = keyResult.message;
    paint();
    return;
  }
  const publicKey = readField(keyResult.body, "public_key");
  if (publicKey === "") {
    state.notice = "A chave pública de push não chegou.";
    paint();
    return;
  }
  const permission = await Notification.requestPermission();
  if (permission !== "granted") {
    state.notice = "A permissão de aviso foi recusada.";
    paint();
    return;
  }
  try {
    const registration = await view.navigator.serviceWorker.ready;
    const subscription = await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToBytes(publicKey),
    });
    const json = subscription.toJSON();
    const endpoint = json.endpoint ?? "";
    const p256dh = json.keys?.p256dh ?? "";
    const auth = json.keys?.auth ?? "";
    const saved = await apiRequest(view.location.origin, "/v1/push/subscriptions", state.token, "POST", {
      endpoint,
      p256dh,
      auth,
    });
    if (!saved.ok) {
      state.notice = saved.message;
      paint();
      return;
    }
    state.notice = "Avisos ativados neste navegador.";
  } catch {
    state.notice = "Não foi possível inscrever este navegador.";
  }
  await loadAlerts(view, state, paint);
}

async function removeSubscription(view: Window, state: AppState, id: string, paint: () => void): Promise<void> {
  const result = await apiRequest(view.location.origin, `/v1/push/subscriptions/${encodeURIComponent(id)}`, state.token, "DELETE");
  if (!result.ok) {
    state.notice = result.message;
    paint();
    return;
  }
  state.notice = "Inscrição removida.";
  await loadAlerts(view, state, paint);
}

function begin(state: AppState): number {
  state.loadId += 1;
  return state.loadId;
}

function loseSession(view: Window, state: AppState, message: string, paint: () => void): void {
  state.loadId += 1;
  state.token = "";
  state.screen = { type: "signed-out" };
  state.overlay = { type: "none" };
  state.notice = message;
  view.localStorage.removeItem(operatorTokenKey);
  paint();
}

function readCache(view: Window): Overview | undefined {
  const raw = view.localStorage.getItem(overviewCacheKey);
  if (raw === null || raw === "") {
    return undefined;
  }
  try {
    const parsed = parseCachedOverview(parseJson(raw));
    if (!parsed.ok) {
      return undefined;
    }
    return parsed.value;
  } catch {
    return undefined;
  }
}

function pushState(result: ApiResult): "ready" | "unconfigured" {
  if (result.ok) {
    return "ready";
  }
  return "unconfigured";
}

function firstAuthFailure(results: ApiResult[]): Extract<ApiResult, { ok: false }> | undefined {
  for (const result of results) {
    if (!result.ok && result.status === 401) {
      return result;
    }
  }
  return undefined;
}

function readToken(result: ApiResult, key: string): string {
  if (!result.ok) {
    return "";
  }
  return readField(result.body, key);
}

function readField(value: unknown, key: string): string {
  if (!isRecord(value)) {
    return "";
  }
  const field = value[key];
  if (typeof field !== "string") {
    return "";
  }
  return field;
}

function registerWorker(view: Window): void {
  if (!("serviceWorker" in view.navigator)) {
    return;
  }
  void view.navigator.serviceWorker.register("/sw.js").then(
    () => undefined,
    () => undefined,
  );
}

function urlBase64ToBytes(value: string): ArrayBuffer {
  const padding = "=".repeat((4 - (value.length % 4)) % 4);
  const base64 = (value + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const buffer = new ArrayBuffer(raw.length);
  const bytes = new Uint8Array(buffer);
  for (let index = 0; index < raw.length; index += 1) {
    const code = raw.codePointAt(index);
    bytes[index] = code ?? 0;
  }
  return buffer;
}

function parseJson(raw: string): unknown {
  return JSON.parse(raw);
}

function el(doc: Document, tag: string, className: string, text: string): HTMLElement {
  const node = doc.createElement(tag);
  if (className !== "") {
    node.className = className;
  }
  node.textContent = text;
  return node;
}

function button(doc: Document, text: string, className: string): HTMLButtonElement {
  const node = doc.createElement("button");
  node.type = "button";
  node.textContent = text;
  if (className !== "") {
    node.className = className;
  }
  return node;
}

function link(doc: Document, href: string, text: string, current: boolean): HTMLAnchorElement {
  const node = doc.createElement("a");
  node.href = href;
  node.textContent = text;
  if (current) {
    node.setAttribute("aria-current", "page");
  }
  return node;
}

function note(doc: Document, text: string, className: string): HTMLElement {
  const node = el(doc, "p", className, text);
  node.setAttribute("role", "status");
  return node;
}

function field(doc: Document, labelText: string, type: string, name: string, value: string): HTMLLabelElement {
  const label = doc.createElement("label");
  label.textContent = labelText;
  const input = doc.createElement("input");
  input.type = type;
  input.name = name;
  input.value = value;
  label.append(input);
  return label;
}
