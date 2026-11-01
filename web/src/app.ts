import { apiRequest, type ApiResult } from "./api.ts";
import {
  deliveryDetail,
  deliveryLabel,
  formatInstant,
  formatUsed,
  remainingLabel,
  sparkShape,
  summarize,
  toneLabel,
  trendLabel,
  trendPoints,
  type TrendPoint,
  loginCallbackPath,
  loginGuide,
  loginLabel,
  clearOverviewCache,
  operatorTokenKey,
  overviewCacheKey,
  overviewCacheStorageKey,
  parseCachedOverview,
  parseDeliveries,
  parseDevices,
  parseEmail,
  emailGuide,
  parseHistory,
  parseLoginCatalog,
  parseLoginProvider,
  parsePublicLogins,
  parseSabia,
  parseSabiaApply,
  parseSabiaPlan,
  parseSecretBackend,
  parseSession,
  isRecord,
  parseOverview,
  parseRoute,
  screenPath,
  parseSettings,
  parseSubscriptions,
  projectionCopy,
  providerLabel,
  readingCopy,
  staffLevel,
  staffTone,
  subscriptionLabel,
  windowLabel,
  type AccountView,
  type DeliveryView,
  type DeviceView,
  type EmailView,
  type HistorySample,
  type LoginProviderView,
  type Overview,
  type ProviderCard,
  type RouteName,
  type SabiaChangeView,
  type SabiaView,
  type SecretBackendView,
  type SettingsView,
  type WindowView,
  type SubscriptionView,
} from "./view.ts";
import {
  applyTheme,
  bindSystemTheme,
  prefersDark,
  readThemeChoice,
  themeStorageKey,
  type ThemeChoice,
} from "./theme.ts";

type ScreenState =
  | { type: "signed-out" }
  | { type: "ready"; role: "operator" }
  | { type: "ready"; role: "person"; provider: string };

type OverlayState =
  | { type: "none" }
  | { type: "operator-token"; token: string }
  | { type: "enrollment-code"; code: string; expiresAt: string }
  | { type: "revoke-device"; id: string; name: string };

type PageState =
  | { route: "overview"; overview?: Overview; fromCache: boolean; problem: string; trend?: HistorySample[] }
  | { route: "history"; samples: HistorySample[]; hasMore: boolean; nextCursor: string; problem: string }
  | { route: "devices"; devices: DeviceView[]; problem: string }
  | {
      route: "alerts";
      settings?: SettingsView;
      subscriptions: SubscriptionView[];
      deliveries: DeliveryView[];
      push: "loading" | "ready" | "unconfigured";
      problem: string;
    }
  | {
      route: "access";
      backend?: SecretBackendView;
      providers: LoginProviderView[];
      sabia?: SabiaView;
      email?: EmailView;
      problem: string;
    };

type AppState = {
  token: string;
  theme: ThemeChoice;
  screen: ScreenState;
  loginChoices: string[];
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
    theme: readThemeChoice(view.localStorage),
    screen: { type: "signed-out" },
    loginChoices: [],
    overlay: { type: "none" },
    page: { route: "overview", fromCache: false, problem: "" },
    notice: "",
    loadId: 0,
  };
  applyTheme(view.document, state.theme, prefersDark(view));
  bindSystemTheme(view, () => state.theme);
  if (new URLSearchParams(view.location.search).get("login") === "falha") {
    state.notice = "O login não foi concluído.";
  }
  const paint = () => {
    render(root, view, state);
  };
  view.addEventListener("popstate", () => {
    if (state.screen.type !== "ready") {
      return;
    }
    void loadRoute(view, state, currentRoute(view), paint);
  });
  canonicalize(view);
  if (state.token !== "") {
    state.screen = { type: "ready", role: "operator" };
    paint();
    void loadRoute(view, state, currentRoute(view), paint);
    registerWorker(view);
    return;
  }
  paint();
  void enterAsPerson(view, state, paint);
  registerWorker(view);
}

async function enterAsPerson(view: Window, state: AppState, paint: () => void): Promise<void> {
  const session = await apiRequest(view.location.origin, "/v1/session", "", "GET");
  if (session.ok) {
    const provider = parseSession(session.body);
    if (provider !== undefined) {
      state.screen = { type: "ready", role: "person", provider };
      if (state.notice === "O login não foi concluído.") {
        state.notice = "";
      }
      paint();
      void loadRoute(view, state, currentRoute(view), paint);
      return;
    }
  }
  const listed = await apiRequest(view.location.origin, "/v1/login/providers", "", "GET");
  if (listed.ok) {
    state.loginChoices = parsePublicLogins(listed.body);
  }
  paint();
}

function render(root: HTMLElement, view: Window, state: AppState): void {
  const doc = root.ownerDocument;
  applyTheme(doc, state.theme, prefersDark(view));
  root.replaceChildren();
  if (state.screen.type === "signed-out") {
    root.append(signedOut(doc, view, state, () => render(root, view, state)));
    return;
  }
  const shell = doc.createElement("div");
  shell.className = "shell";
  const skip = doc.createElement("a");
  skip.className = "skip";
  skip.href = "#conteudo";
  skip.textContent = "Ir para o conteúdo";
  shell.append(skip, topbar(doc, view, state, () => render(root, view, state)));
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
  } else if (state.page.route === "alerts") {
    if (personView(state)) {
      main.append(adminArea(doc));
    } else {
      main.append(alertsView(doc, view, state, () => render(root, view, state)));
    }
  } else if (personView(state)) {
    main.append(adminArea(doc));
  } else {
    main.append(accessView(doc, view, state, () => render(root, view, state)));
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
  const panel = doc.createElement("div");
  panel.className = "gate-panel";
  panel.append(el(doc, "p", "mark", "Uso"));
  panel.append(themeControl(doc, view, state, paint));
  panel.append(el(doc, "h1", "gate-title", "Ver o uso"));
  panel.append(el(doc, "p", "lede", "O agente fica na sua máquina. Quem tem conta entra pelo provedor configurado. O token fica só com o administrador."));
  if (state.loginChoices.length > 0) {
    const choices = doc.createElement("div");
    choices.className = "choices";
    for (const provider of state.loginChoices) {
      const anchor = doc.createElement("a");
      anchor.className = "enter";
      anchor.href = `/v1/login/${provider}/start`;
      anchor.textContent = `Entrar com ${loginLabel(provider)}`;
      choices.append(anchor);
    }
    panel.append(choices);
  }
  if (state.notice !== "") {
    panel.append(note(doc, state.notice, "notice"));
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
    state.screen = { type: "ready", role: "operator" };
    state.notice = "";
    state.overlay = { type: "none" };
    paint();
    void loadRoute(view, state, currentRoute(view), paint);
  });
  const create = button(doc, "Criar esta instalação", "");
  create.type = "button";
  create.dataset.action = "create-setup";
  create.addEventListener("click", () => {
    void createSetup(view, state, paint);
  });
  const admin = doc.createElement("details");
  admin.className = "admin-entry";
  if (state.loginChoices.length === 0) {
    admin.open = true;
  }
  const summary = doc.createElement("summary");
  summary.textContent = "Sou o administrador";
  admin.append(summary, form, create);
  panel.append(admin);
  section.append(panel);
  return section;
}

function topbar(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const header = doc.createElement("header");
  header.className = "topbar";
  const brandline = doc.createElement("div");
  brandline.className = "brandline";
  const brand = screenLink(doc, view, state, paint, "overview", "Uso");
  brand.className = "mark";
  brand.removeAttribute("aria-current");
  const exit = button(doc, "Sair", "quiet");
  exit.dataset.action = "sign-out";
  exit.setAttribute("aria-label", "Sair deste navegador");
  exit.addEventListener("click", () => {
    void signOut(view, state, paint);
  });
  brandline.append(brand, exit);
  const nav = doc.createElement("nav");
  nav.setAttribute("aria-label", "Seções");
  const pages: Array<[RouteName, string]> = [
    ["overview", "Visão"],
    ["history", "Histórico"],
    ["devices", "Dispositivos"],
  ];
  if (!personView(state)) {
    pages.push(["alerts", "Alertas"]);
    pages.push(["access", "Acesso"]);
  }
  for (const [route, label] of pages) {
    nav.append(screenLink(doc, view, state, paint, route, label));
  }
  header.append(brandline, nav, themeControl(doc, view, state, paint));
  return header;
}

function themeControl(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const group = doc.createElement("div");
  group.className = "theme";
  group.setAttribute("role", "radiogroup");
  group.setAttribute("aria-labelledby", "theme-label");
  const legend = el(doc, "p", "theme-label", "Tema");
  legend.id = "theme-label";
  group.append(legend);
  const choices: Array<[ThemeChoice, string]> = [
    ["system", "Dispositivo"],
    ["light", "Claro"],
    ["dark", "Escuro"],
  ];
  for (const [choice, label] of choices) {
    const option = button(doc, label, "theme-option");
    option.setAttribute("role", "radio");
    const selected = state.theme === choice;
    option.setAttribute("aria-checked", selected ? "true" : "false");
    option.tabIndex = selected ? 0 : -1;
    option.addEventListener("click", () => {
      chooseTheme(doc, view, state, choice, paint);
    });
    option.addEventListener("keydown", (event) => {
      const forward = event.key === "ArrowRight" || event.key === "ArrowDown";
      const backward = event.key === "ArrowLeft" || event.key === "ArrowUp";
      if (!forward && !backward) {
        return;
      }
      event.preventDefault();
      const index = choices.findIndex(([item]) => item === state.theme);
      const step = forward ? 1 : -1;
      const next = choices[(index + step + choices.length) % choices.length];
      if (next === undefined) {
        return;
      }
      chooseTheme(doc, view, state, next[0], paint);
    });
    group.append(option);
  }
  return group;
}

function chooseTheme(doc: Document, view: Window, state: AppState, choice: ThemeChoice, paint: () => void): void {
  state.theme = choice;
  try {
    view.localStorage.setItem(themeStorageKey, choice);
  } catch {
    // A escolha vale nesta visita mesmo se o navegador recusar gravar.
  }
  applyTheme(doc, choice, prefersDark(view));
  paint();
  const realm = doc.defaultView;
  const selected = doc.querySelector(".theme [aria-checked='true']");
  if (realm !== null && selected instanceof realm.HTMLElement) {
    selected.focus();
  }
}

async function signOut(view: Window, state: AppState, paint: () => void): Promise<void> {
  if (personView(state)) {
    const result = await apiRequest(view.location.origin, "/v1/session/logout", "", "POST");
    if (!result.ok && result.errorCode === "NETWORK") {
      state.notice = "Sem rede para encerrar a sessão.";
      paint();
      return;
    }
  }
  state.loadId += 1;
  state.token = "";
  state.screen = { type: "signed-out" };
  state.loginChoices = [];
  state.overlay = { type: "none" };
  state.notice = "";
  state.page = { route: "overview", fromCache: false, problem: "" };
  view.localStorage.removeItem(operatorTokenKey);
  clearOverviewCache(view.localStorage);
  paint();
  void enterAsPerson(view, state, paint);
}

function personView(state: AppState): boolean {
  return state.screen.type === "ready" && state.screen.role === "person";
}

function adminArea(doc: Document): HTMLElement {
  const wrap = doc.createElement("section");
  wrap.append(pageHead(doc, "Área do administrador", "Esta área fica com quem administra a instalação."));
  return wrap;
}

function overviewRequest(state: AppState): string {
  if (personView(state)) {
    return "/v1/account/overview";
  }
  return "/v1/overview";
}

function trendRequest(state: AppState): string {
  const base = personView(state) ? "/v1/account/samples" : "/v1/samples";
  return `${base}?limit=60`;
}

function samplesRequest(state: AppState, cursor: string): string {
  const base = personView(state) ? "/v1/account/samples" : "/v1/samples";
  if (cursor === "") {
    return `${base}?limit=20`;
  }
  return `${base}?limit=20&cursor=${encodeURIComponent(cursor)}`;
}

function devicesRequest(state: AppState): string {
  if (personView(state)) {
    return "/v1/account/devices";
  }
  return "/v1/devices";
}

function enrollmentRequest(state: AppState): string {
  if (personView(state)) {
    return "/v1/account/enrollment-codes";
  }
  return "/v1/enrollment-codes";
}

function revokeRequest(state: AppState, id: string): string {
  const encoded = encodeURIComponent(id);
  if (personView(state)) {
    return `/v1/account/devices/${encoded}/revoke`;
  }
  return `/v1/devices/${encoded}/revoke`;
}

function overviewView(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const page = state.page.route === "overview" ? state.page : { route: "overview" as const, fromCache: false, problem: "" };
  const accounts = page.overview?.accounts;
  const grouped = accounts !== undefined;
  const wrap = doc.createElement("section");
  const refresh = button(doc, "Atualizar", "");
  refresh.dataset.action = "refresh";
  refresh.addEventListener("click", () => {
    void loadOverview(view, state, paint);
  });
  if (grouped) {
    wrap.append(pageHead(doc, "Contas", "Cada conta mostra as próprias medições. A marca na régua é 80%.", refresh));
  } else {
    wrap.append(pageHead(doc, "Medições", "Cada janela tem a própria régua. A marca na régua é 80%.", refresh));
  }
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
  // A amostra não traz a conta. Com mais de uma conta, a tendência misturaria dados.
  const singleAccountTrend = page.trend !== undefined && (accounts === undefined || accounts.length === 1)
    ? page.trend
    : undefined;
  if (!page.fromCache) {
    const cards = grouped && accounts !== undefined
      ? accounts.flatMap((account) => account.providers)
      : page.overview.providers;
    if (cards.length > 0) {
      wrap.append(summaryStrip(doc, cards));
    }
  }
  if (grouped && accounts !== undefined) {
    if (accounts.length === 0) {
      wrap.append(el(doc, "p", "empty", "Nenhuma conta com dispositivo. Gere um código em Dispositivos e enrole o agente nesta máquina."));
      return wrap;
    }
    for (const account of accounts) {
      wrap.append(accountSection(doc, account, page.fromCache, singleAccountTrend));
    }
    return wrap;
  }
  wrap.append(providerBoard(doc, page.overview.providers, page.fromCache, singleAccountTrend));
  return wrap;
}

function summaryStrip(doc: Document, cards: ProviderCard[]): HTMLElement {
  const summary = summarize(cards);
  const list = doc.createElement("ul");
  list.className = "summary";
  const tile = (label: string, value: string, detail: string, tone?: string): void => {
    const item = doc.createElement("li");
    if (tone !== undefined) {
      item.dataset.tone = tone;
    }
    item.append(el(doc, "p", "summary-label", label), el(doc, "p", "summary-value", value), el(doc, "p", "meta", detail));
    list.append(item);
  };
  if (summary.tightest !== undefined) {
    tile(
      "Mais perto do limite",
      `${formatUsed(summary.tightest.used)}%`,
      `${providerLabel(summary.tightest.provider)}, ${windowLabel(summary.tightest.window)}. ${toneLabel(summary.tightest.used)}.`,
      staffTone(summary.tightest.used),
    );
  } else {
    tile("Mais perto do limite", "Sem leitura", "Nenhuma janela chegou ainda.");
  }
  if (summary.nextReset !== undefined) {
    tile(
      "Próximo reinício",
      formatInstant(summary.nextReset.resetsAt),
      `${providerLabel(summary.nextReset.provider)}, ${windowLabel(summary.nextReset.window)}.`,
    );
  } else {
    tile("Próximo reinício", "Sem data", "Nenhum reinício conhecido.");
  }
  tile("Leituras atuais", `${summary.current} de ${summary.total}`, summary.current === summary.total ? "Tudo em dia." : "Há leituras antigas ou indisponíveis.");
  return list;
}

function accountSection(doc: Document, account: AccountView, fromCache: boolean, trend?: HistorySample[]): HTMLElement {
  const section = doc.createElement("section");
  section.className = "account";
  section.dataset.account = account.id;
  section.append(el(doc, "h2", "account-label", account.label));
  if (account.login !== "") {
    section.append(el(doc, "p", "meta", loginLabel(account.login)));
  }
  section.append(providerBoard(doc, account.providers, fromCache, trend));
  return section;
}

function providerBoard(doc: Document, cards: ProviderCard[], fromCache: boolean, trend?: HistorySample[]): HTMLElement {
  const board = doc.createElement("div");
  board.className = "board";
  for (const card of cards) {
    board.append(providerCard(doc, card, fromCache, trend));
  }
  return board;
}

function providerCard(doc: Document, card: ProviderCard, fromCache: boolean, trend?: HistorySample[]): HTMLElement {
  const article = doc.createElement("article");
  article.className = "provider";
  article.dataset.provider = card.provider;
  article.dataset.freshness = fromCache ? "cached" : card.freshness;
  const copy = readingCopy(card.freshness, card.observedAt, card.reasonCode, fromCache);
  const text = doc.createElement("div");
  text.className = "provider-copy";
  text.append(el(doc, "h2", "", providerLabel(card.provider)));
  const headline = el(doc, "p", "headline", copy.headline);
  headline.dataset.headline = copy.headline;
  text.append(headline);
  text.append(el(doc, "p", "", copy.detail));
  if (card.plan !== undefined && card.plan !== "") {
    text.append(el(doc, "p", "meta", `Plano ${card.plan}.`));
  }
  if (card.windows.length === 0) {
    text.append(gauge(doc, 0));
  } else {
    const lookup = trend === undefined
      ? undefined
      : (name: string): TrendPoint[] => trendPoints(trend, card.provider, name);
    text.append(windowRows(doc, card.windows, lookup));
  }
  if (card.freshness === "unavailable" && card.windows.length > 0 && !fromCache) {
    text.append(el(doc, "p", "meta", "A última amostra conhecida continua visível. Não é a leitura atual."));
  }
  if (card.projection !== undefined && (card.freshness === "current" || card.freshness === "stale" || fromCache)) {
    text.append(el(doc, "p", "projection", projectionCopy(card.projection)));
  }
  article.append(text);
  return article;
}

function windowRows(doc: Document, windows: WindowView[], trendFor?: (name: string) => TrendPoint[]): HTMLElement {
  const list = doc.createElement("ul");
  list.className = "windows";
  for (const item of windows) {
    const row = doc.createElement("li");
    row.className = "window";
    row.dataset.window = item.name;
    row.append(el(doc, "p", "", windowLabel(item.name)));
    const figure = el(doc, "p", "figure", `${formatUsed(item.used)}%`);
    figure.dataset.used = formatUsed(item.used);
    row.append(figure);
    row.append(el(doc, "p", "meta", `Reinicia ${formatInstant(item.resetsAt)}.`));
    const tone = el(doc, "p", "tone", toneLabel(item.used));
    tone.dataset.tone = staffTone(item.used);
    row.append(tone);
    row.append(el(doc, "p", "left", remainingLabel(item.used)));
    const spark = trendFor === undefined ? undefined : sparkline(doc, trendFor(item.name), item.used);
    if (spark !== undefined) {
      row.append(spark);
    }
    row.append(gauge(doc, item.used));
    list.append(row);
  }
  return list;
}

const svgNs = "http://www.w3.org/2000/svg";

function sparkline(doc: Document, points: TrendPoint[], used: number): HTMLElement | undefined {
  const shape = sparkShape(points, 160, 36);
  if (shape === undefined) {
    return undefined;
  }
  const wrap = doc.createElement("div");
  wrap.className = "spark";
  wrap.dataset.tone = staffTone(used);
  const svg = doc.createElementNS(svgNs, "svg");
  svg.setAttribute("viewBox", "0 0 160 36");
  svg.setAttribute("preserveAspectRatio", "none");
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", trendLabel(points));
  const area = doc.createElementNS(svgNs, "path");
  area.setAttribute("class", "spark-area");
  area.setAttribute("d", shape.area);
  const line = doc.createElementNS(svgNs, "path");
  line.setAttribute("class", "spark-line");
  line.setAttribute("d", shape.line);
  line.setAttribute("vector-effect", "non-scaling-stroke");
  svg.append(area, line);
  const dot = doc.createElement("span");
  dot.className = "spark-dot";
  dot.style.setProperty("--x", `${(shape.end.x / 160) * 100}%`);
  dot.style.setProperty("--y", `${(shape.end.y / 36) * 100}%`);
  wrap.append(svg, dot);
  return wrap;
}

function gauge(doc: Document, used: number): HTMLElement {
  const staff = doc.createElement("div");
  staff.className = "staff";
  staff.dataset.tone = staffTone(used);
  const level = staffLevel(used);
  staff.style.setProperty("--level", String(level));
  staff.setAttribute("role", "meter");
  staff.setAttribute("aria-valuemin", "0");
  staff.setAttribute("aria-valuemax", "100");
  staff.setAttribute("aria-valuenow", String(Math.round(level)));
  staff.setAttribute("aria-label", `${formatUsed(used)} por cento usado. O aviso começa em 80 por cento.`);
  const water = doc.createElement("div");
  water.className = "staff-water";
  const mark = doc.createElement("div");
  mark.className = "staff-mark";
  const limit = doc.createElement("span");
  limit.className = "staff-limit";
  limit.textContent = "80%";
  staff.append(water, mark, limit);
  return staff;
}

function historyView(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const page = state.page.route === "history"
    ? state.page
    : { route: "history" as const, samples: [], hasMore: false, nextCursor: "", problem: "" };
  const wrap = doc.createElement("section");
  wrap.append(pageHead(doc, "Histórico", "Cada linha é uma amostra aceita. Reenviar o mesmo snapshot não cria outra."));
  if (page.problem !== "") {
    wrap.append(note(doc, page.problem, "problem"));
  }
  if (page.samples.length === 0 && page.problem === "") {
    wrap.append(el(doc, "p", "empty", "Nenhuma amostra no histórico. A primeira aparece quando o agente envia uma leitura."));
  }
  const list = doc.createElement("ol");
  list.className = "history";
  for (const sample of page.samples) {
    const item = doc.createElement("li");
    item.append(el(doc, "h2", "", providerLabel(sample.provider)));
    item.append(el(doc, "p", "meta", formatInstant(sample.collectedAt)));
    if (sample.plan !== undefined && sample.plan !== "") {
      item.append(el(doc, "p", "meta", `Plano ${sample.plan}.`));
    }
    if (sample.windows.length > 0) {
      item.append(windowRows(doc, sample.windows));
    }
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
  const enroll = button(doc, "Gerar código de enrolamento", "primary");
  enroll.dataset.action = "enroll";
  enroll.addEventListener("click", () => {
    void createEnrollment(view, state, paint);
  });
  wrap.append(pageHead(doc, "Dispositivos", "O código de enrolamento aparece uma vez e vale quinze minutos. Revogar congela a fila daquele token.", enroll));
  if (page.problem !== "") {
    wrap.append(note(doc, page.problem, "problem"));
  }
  if (page.devices.length === 0) {
    wrap.append(el(doc, "p", "empty", "Nenhum dispositivo enrolado. Gere um código e cole no agente desta máquina."));
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
  wrap.append(pageHead(doc, "Alertas", "O aviso usa só título, texto, provedor e tipo. No iPhone ele depende do aplicativo na tela de início e pode atrasar."));
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
    wrap.append(el(doc, "p", "empty", "Nenhuma inscrição. Ative os avisos neste navegador para receber o alerta aqui."));
  }
  const subs = doc.createElement("ul");
  for (const item of page.subscriptions) {
    const row = doc.createElement("li");
    row.append(el(doc, "p", "", subscriptionLabel(item.status)));
    row.append(el(doc, "p", "meta", item.endpoint));
    row.append(el(doc, "p", "meta", formatInstant(item.createdAt)));
    const subscriptionReason = deliveryDetail(item.lastError ?? "");
    if (subscriptionReason !== "") {
      row.append(el(doc, "span", "meta", ` ${subscriptionReason}`));
    } else if (item.lastError !== undefined && item.lastError !== "") {
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
    wrap.append(el(doc, "p", "empty", "Nenhum aviso registrado. O alerta aparece aqui depois do primeiro envio."));
  }
  const deliveries = doc.createElement("ul");
  deliveries.className = "deliveries";
  for (const item of page.deliveries) {
    const row = doc.createElement("li");
    const status = el(doc, "p", "headline", deliveryLabel(item.status));
    status.dataset.deliveryStatus = item.status;
    row.append(status);
    const reason = deliveryDetail(item.detail ?? "");
    if (reason !== "") {
      row.append(el(doc, "p", "meta", reason));
    }
    row.append(el(doc, "p", "", providerLabel(item.provider)));
    row.append(el(doc, "p", "", item.title));
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

function accessView(doc: Document, view: Window, state: AppState, paint: () => void): HTMLElement {
  const page = state.page.route === "access" ? state.page : { route: "access" as const, providers: [], problem: "" };
  const wrap = doc.createElement("section");
  wrap.append(pageHead(doc, "Acesso", "O administrador configura o login. O segredo entra no cofre e não volta para esta tela. Quem entra por um provedor configurado vê o próprio uso. Esta tela continua só com o administrador."));
  wrap.append(el(doc, "p", "meta", "O token que abre o cofre fica na configuração do processo, não nesta tela. Trocar de cofre só vale quando esse processo já abre o destino e ainda não há segredo gravado."));
  if (page.backend !== undefined) {
    wrap.append(el(doc, "p", "", backendCopy(page.backend)));
  }
  if (page.problem !== "") {
    wrap.append(note(doc, page.problem, "problem"));
  }
  if (page.email !== undefined) {
    wrap.append(emailForm(doc, view, state, page.email, paint));
  }
  if (page.sabia !== undefined) {
    wrap.append(sabiaForm(doc, view, state, page.sabia, paint));
  }
  for (const item of page.providers) {
    wrap.append(loginForm(doc, view, state, item, paint));
  }
  return wrap;
}

function emailForm(doc: Document, view: Window, state: AppState, item: EmailView, paint: () => void): HTMLFormElement {
  const form = doc.createElement("form");
  form.className = "stack access-form";
  form.dataset.panel = "email";
  form.append(el(doc, "h2", "", "E-mail da instalação"));
  const kept = emailKept(item);
  form.append(el(doc, "p", "meta", kept));
  form.append(el(doc, "p", "", "Um remetente e um destinatário para toda a instalação. A chave entra no cofre e não volta para esta tela."));
  form.append(el(doc, "p", "", "Este aviso sai pelo provedor acima. Sem canal ativo, o e-mail fica indisponível. A chave do Sabiá não entra neste canal."));
  if (item.lastStatus !== undefined) {
    form.append(el(doc, "p", "meta", `Último envio: ${emailStatus(item.lastStatus, item.lastDetail)}`));
  }
  const guideHost = doc.createElement("div");
  guideHost.dataset.emailGuide = "1";
  form.append(guideHost);
  const providerLabel = doc.createElement("label");
  providerLabel.textContent = "Provedor";
  const provider = doc.createElement("select");
  provider.name = "provider";
  const choices: Array<{ value: string; label: string }> = [
    { value: "resend", label: "Resend" },
    { value: "millionsend", label: "MillionSend" },
    { value: "cloudflare", label: "Cloudflare Email" },
    { value: "ses", label: "Amazon SES" },
  ];
  for (const choice of choices) {
    const option = doc.createElement("option");
    option.value = choice.value;
    option.textContent = choice.label;
    provider.append(option);
  }
  provider.value = item.provider === "" ? "resend" : item.provider;
  providerLabel.append(provider);
  form.append(providerLabel);
  form.append(field(doc, "Remetente", "text", "from_address", item.fromAddress));
  form.append(field(doc, "Destinatário", "email", "to_address", item.toAddress));
  const account = doc.createElement("label");
  account.dataset.emailAccount = "1";
  const accountText = doc.createElement("span");
  accountText.dataset.emailAccountLabel = "1";
  const accountInput = doc.createElement("input");
  accountInput.type = "text";
  accountInput.name = "account_ref";
  accountInput.value = item.accountRef;
  account.append(accountText, accountInput);
  form.append(account);
  const activeLabel = el(doc, "label", "check", "E-mail ativo");
  const active = doc.createElement("input");
  active.type = "checkbox";
  active.name = "active";
  active.checked = item.active;
  activeLabel.prepend(active);
  form.append(activeLabel);
  form.append(emailSecret(doc, "Chave da API", "api_key", "api"));
  form.append(emailSecret(doc, "Access key id", "access_key_id", "ses"));
  form.append(emailSecret(doc, "Chave secreta", "secret_access_key", "ses"));
  const save = button(doc, "Guardar", "primary");
  save.type = "submit";
  form.append(save);
  if (item.provider !== "" || item.secretPresent || item.fromAddress !== "") {
    const remove = button(doc, "Remover o e-mail", "quiet");
    remove.dataset.action = "remove-email";
    remove.addEventListener("click", () => {
      void removeEmail(view, state, paint);
    });
    form.append(remove);
  }
  const sync = () => {
    paintEmailProvider(doc, view, form, provider.value);
  };
  provider.addEventListener("change", sync);
  sync();
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void saveEmail(view, state, form, paint);
  });
  return form;
}

function emailKept(item: EmailView): string {
  if (!item.active) {
    return "E-mail indisponível.";
  }
  if (item.secretPresent) {
    return "E-mail ativo. Chave guardada.";
  }
  return "E-mail ativo. Nenhuma chave guardada.";
}

function emailStatus(status: string, detail: string | undefined): string {
  if (status === "sent") {
    return "enviado";
  }
  if (status === "pending") {
    return "na fila";
  }
  if (status === "suppressed") {
    return "em horário silencioso";
  }
  if (status === "failed") {
    if (detail !== undefined && detail !== "") {
      return `falhou (${detail})`;
    }
    return "falhou";
  }
  return status;
}

function emailSecret(doc: Document, labelText: string, name: string, kind: string): HTMLLabelElement {
  const label = doc.createElement("label");
  label.dataset.emailKind = kind;
  label.textContent = labelText;
  const input = doc.createElement("input");
  input.type = "password";
  input.name = name;
  input.autocomplete = "off";
  input.spellcheck = false;
  input.placeholder = "Deixe vazio para manter a chave guardada.";
  input.value = "";
  input.dataset.secret = "email";
  label.append(input);
  return label;
}

function paintEmailProvider(doc: Document, view: Window, form: HTMLFormElement, provider: string): void {
  const realm = doc.defaultView;
  if (realm === null) {
    return;
  }
  const host = form.querySelector("[data-email-guide]");
  if (host instanceof realm.HTMLElement) {
    host.replaceChildren();
    const guide = emailGuide(provider);
    if (guide !== undefined) {
      host.append(loginGuideView(doc, view, provider, guide.steps, guide.example));
    }
  }
  const account = form.querySelector("[data-email-account]");
  const accountText = form.querySelector("[data-email-account-label]");
  const showAccount = provider === "cloudflare" || provider === "ses";
  if (account instanceof realm.HTMLElement) {
    account.hidden = !showAccount;
  }
  if (accountText instanceof realm.HTMLElement) {
    accountText.textContent = provider === "ses" ? "Região do SES" : "Conta da Cloudflare";
  }
  const secrets = form.querySelectorAll("[data-email-kind]");
  for (const node of secrets) {
    if (!(node instanceof realm.HTMLElement)) {
      continue;
    }
    const kind = node.dataset.emailKind;
    node.hidden = provider === "ses" ? kind !== "ses" : kind !== "api";
  }
}

function sabiaForm(doc: Document, view: Window, state: AppState, item: SabiaView, paint: () => void): HTMLFormElement {
  const form = doc.createElement("form");
  form.className = "stack access-form";
  form.dataset.panel = "sabia";
  form.append(el(doc, "h2", "", "Sabiá"));
  const deploy = item.deployKeyPresent ? "Chave de deploy guardada." : "Nenhuma chave de deploy guardada.";
  const send = item.sendKeyPresent ? "Chave de envio guardada." : "Nenhuma chave de envio guardada.";
  form.append(el(doc, "p", "meta", `${deploy} ${send}`));
  form.append(el(doc, "p", "", "Edite o template aqui e sincronize sem abrir a plataforma. A chave entra no cofre e não volta para esta tela."));
  form.append(el(doc, "p", "", "Sincronizar publica no Sabiá. Não apaga templates que não estão aqui."));
  form.append(el(doc, "p", "", "O alerta pede POST /v1/messages com a chave de envio. Um 202 fica aceito na fila do Sabiá. Isso não confirma a caixa de entrada."));
  if (item.lastStatus !== undefined) {
    form.append(el(doc, "p", "meta", `Último pedido: ${sabiaStatus(item.lastStatus, item.lastDetail)}`));
  }
  form.append(field(doc, "Endereço", "url", "origin", item.origin));
  form.append(field(doc, "Id da aplicação", "text", "application_id", item.applicationId));
  form.append(field(doc, "Destinatário", "email", "recipient", item.recipient));
  form.append(sabiaKeyField(doc, "Chave de envio", "send_key", "sabia-send"));
  form.append(sabiaKeyField(doc, "Chave de deploy", "deploy_key", "sabia"));
  form.append(field(doc, "Nome", "text", "name", item.name));
  form.append(field(doc, "Assunto", "text", "subject", item.subject));
  form.append(textBlock(doc, "HTML", "html", item.html));
  form.append(textBlock(doc, "Texto", "text", item.text));
  if (item.changes !== undefined) {
    if (item.changes.length === 0) {
      form.append(el(doc, "p", "", "Nenhuma mudança."));
    }
    for (const change of item.changes) {
      form.append(sabiaChange(doc, change));
    }
  }
  const save = button(doc, "Guardar", "primary");
  save.type = "submit";
  const plan = button(doc, "Ver plano", "");
  plan.dataset.action = "sabia-plan";
  const sync = button(doc, "Sincronizar", "primary");
  sync.dataset.action = "sabia-apply";
  form.append(save, plan, sync);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void saveSabia(view, state, form, paint);
  });
  plan.addEventListener("click", () => {
    void planSabia(view, state, form, paint);
  });
  sync.addEventListener("click", () => {
    void applySabia(view, state, form, paint);
  });
  return form;
}

function sabiaChange(doc: Document, change: SabiaChangeView): HTMLElement {
  const row = doc.createElement("div");
  row.append(el(doc, "p", "", `${change.operation}: ${change.id}`));
  if (change.confirmationRequired) {
    const label = el(doc, "label", "check", `Confirmar ${change.id}`);
    const input = doc.createElement("input");
    input.type = "checkbox";
    input.value = change.id;
    input.dataset.confirm = change.id;
    label.prepend(input);
    row.append(label);
  }
  return row;
}

function sabiaStatus(status: string, detail: string | undefined): string {
  if (status === "queued") {
    return "aceito na fila do Sabiá";
  }
  if (status === "pending") {
    return "na fila daqui";
  }
  if (status === "suppressed") {
    return "em horário silencioso";
  }
  if (status === "failed") {
    if (detail !== undefined && detail !== "") {
      return `falhou (${detail})`;
    }
    return "falhou";
  }
  return status;
}

function sabiaKeyField(doc: Document, labelText: string, name: string, secret: string): HTMLLabelElement {
  const label = doc.createElement("label");
  label.textContent = labelText;
  const input = doc.createElement("input");
  input.type = "password";
  input.name = name;
  input.autocomplete = "off";
  input.spellcheck = false;
  input.placeholder = "Deixe vazio para manter a chave guardada.";
  input.value = "";
  input.dataset.secret = secret;
  label.append(input);
  return label;
}

function textBlock(doc: Document, labelText: string, name: string, value: string): HTMLLabelElement {
  const label = doc.createElement("label");
  label.textContent = labelText;
  const input = doc.createElement("textarea");
  input.name = name;
  input.rows = 6;
  input.value = value;
  label.append(input);
  return label;
}

function loginForm(
  doc: Document,
  view: Window,
  state: AppState,
  item: LoginProviderView,
  paint: () => void,
): HTMLFormElement {
  const form = doc.createElement("form");
  form.className = "stack access-form";
  form.dataset.provider = item.provider;
  form.append(el(doc, "h2", "", loginLabel(item.provider)));
  const kept = item.secretPresent ? "Segredo guardado." : "Nenhum segredo guardado.";
  const complete = item.configured ? "Configuração completa." : "Configuração incompleta.";
  form.append(el(doc, "p", "meta", `${kept} ${complete}`));
  const guide = loginGuide(item.provider);
  if (guide !== undefined) {
    form.append(loginGuideView(doc, view, item.provider, guide.steps, guide.example));
  }
  const enabledLabel = el(doc, "label", "check", "Login ativo");
  const enabled = doc.createElement("input");
  enabled.type = "checkbox";
  enabled.name = "enabled";
  enabled.checked = item.enabled;
  enabledLabel.prepend(enabled);
  form.append(enabledLabel);
  form.append(field(doc, "Client id", "text", "client_id", item.clientId ?? ""));
  if (item.provider === "gitea") {
    form.append(field(doc, "URL base", "url", "issuer_url", item.issuerUrl ?? ""));
  }
  if (item.provider === "apple") {
    form.append(field(doc, "Team ID", "text", "team_id", item.teamId ?? ""));
    form.append(field(doc, "Key ID", "text", "key_id", item.keyId ?? ""));
    form.append(field(doc, "Services ID", "text", "services_id", item.servicesId ?? ""));
  }
  form.append(item.provider === "apple" ? appleSecretField(doc) : secretField(doc));
  const save = button(doc, "Guardar", "primary");
  save.type = "submit";
  form.append(save);
  if (item.configured || item.secretPresent || item.clientId !== undefined) {
    const remove = button(doc, "Remover este login", "quiet");
    remove.dataset.action = "remove-login";
    remove.dataset.provider = item.provider;
    remove.addEventListener("click", () => {
      void removeLogin(view, state, item.provider, paint);
    });
    form.append(remove);
  }
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void saveLogin(view, state, item.provider, form, paint);
  });
  return form;
}

function loginGuideView(doc: Document, view: Window, provider: string, steps: string[], example: string): HTMLElement {
  const block = doc.createElement("div");
  block.className = "guide";
  block.append(el(doc, "h3", "", "Onde obter"));
  const list = doc.createElement("ol");
  for (const step of steps) {
    const item = doc.createElement("li");
    item.textContent = step;
    list.append(item);
  }
  block.append(list);
  const callback = loginCallbackPath(provider);
  if (callback !== undefined) {
    block.append(el(doc, "p", "", "Cadastre esta URL de retorno no aplicativo."));
    block.append(el(doc, "p", "meta", "URL de retorno"));
    block.append(el(doc, "p", "callback", `${view.location.origin}${callback}`));
  }
  block.append(el(doc, "p", "meta", "Exemplo fictício"));
  const sample = el(doc, "pre", "example", example);
  block.append(sample);
  return block;
}

function secretField(doc: Document): HTMLLabelElement {
  const label = doc.createElement("label");
  label.textContent = "Segredo";
  const input = doc.createElement("input");
  input.type = "password";
  input.name = "secret";
  input.autocomplete = "off";
  input.spellcheck = false;
  input.placeholder = "Deixe vazio para manter o segredo guardado.";
  input.value = "";
  input.dataset.secret = "login";
  label.append(input);
  return label;
}

function appleSecretField(doc: Document): HTMLLabelElement {
  const label = doc.createElement("label");
  label.textContent = "Chave .p8";
  const input = doc.createElement("textarea");
  input.name = "secret";
  input.rows = 8;
  input.autocomplete = "off";
  input.spellcheck = false;
  input.placeholder = "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----";
  input.value = "";
  input.dataset.secret = "login";
  label.append(input);
  return label;
}

function backendCopy(backend: SecretBackendView): string {
  if (backend.kind === "own" && backend.ready) {
    return "Cofre próprio aberto.";
  }
  if (backend.kind === "own") {
    return "O cofre próprio não está aberto.";
  }
  const label = backendLabel(backend.kind);
  if (backend.ready) {
    return `${label} aberto. A leitura respondeu.`;
  }
  return `${label} não está aberto neste processo.`;
}

function backendLabel(kind: string): string {
  if (kind === "ciphera") {
    return "Ciphera";
  }
  if (kind === "hashicorp") {
    return "HashiCorp Vault";
  }
  if (kind === "aws") {
    return "AWS Secrets Manager";
  }
  if (kind === "infisical") {
    return "Infisical";
  }
  return "Cofre";
}

async function loadAccess(view: Window, state: AppState, paint: () => void): Promise<void> {
  const loadId = begin(state);
  state.page = { route: "access", providers: [], problem: "" };
  paint();
  const backendResult = await apiRequest(view.location.origin, "/v1/admin/secret-backend", state.token, "GET");
  if (loadId !== state.loadId) {
    return;
  }
  if (!backendResult.ok) {
    if (backendResult.status === 401) {
      loseSession(view, state, backendResult.message, paint);
      return;
    }
    state.page = { route: "access", providers: [], problem: backendResult.message };
    paint();
    return;
  }
  const backend = parseSecretBackend(backendResult.body);
  if (!backend.ok) {
    state.page = { route: "access", providers: [], problem: backend.message };
    paint();
    return;
  }
  const providersResult = await apiRequest(view.location.origin, "/v1/admin/login-providers", state.token, "GET");
  if (loadId !== state.loadId) {
    return;
  }
  if (!providersResult.ok) {
    if (providersResult.status === 401) {
      loseSession(view, state, providersResult.message, paint);
      return;
    }
    state.page = accessState(backend.value, [], providersResult.message, undefined, undefined);
    paint();
    return;
  }
  const providers = parseLoginCatalog(providersResult.body);
  if (!providers.ok) {
    state.page = accessState(backend.value, [], providers.message, undefined, undefined);
    paint();
    return;
  }
  const sabiaResult = await apiRequest(view.location.origin, "/v1/admin/sabia", state.token, "GET");
  if (loadId !== state.loadId) {
    return;
  }
  if (!sabiaResult.ok) {
    if (sabiaResult.status === 401) {
      loseSession(view, state, sabiaResult.message, paint);
      return;
    }
    state.page = accessState(backend.value, providers.value, "", undefined, undefined);
    paint();
    return;
  }
  const sabia = parseSabia(sabiaResult.body);
  if (!sabia.ok) {
    state.page = accessState(backend.value, providers.value, sabia.message, undefined, undefined);
    paint();
    return;
  }
  const emailResult = await apiRequest(view.location.origin, "/v1/admin/email", state.token, "GET");
  if (loadId !== state.loadId) {
    return;
  }
  if (!emailResult.ok) {
    if (emailResult.status === 401) {
      loseSession(view, state, emailResult.message, paint);
      return;
    }
    if (emailResult.status === 404) {
      state.page = accessState(backend.value, providers.value, "", sabia.value, undefined);
      paint();
      return;
    }
    state.page = accessState(backend.value, providers.value, emailResult.message, sabia.value, undefined);
    paint();
    return;
  }
  const email = parseEmail(emailResult.body);
  if (!email.ok) {
    state.page = accessState(backend.value, providers.value, email.message, sabia.value, undefined);
    paint();
    return;
  }
  state.page = accessState(backend.value, providers.value, "", sabia.value, email.value);
  paint();
}

async function saveLogin(
  view: Window,
  state: AppState,
  provider: string,
  form: HTMLFormElement,
  paint: () => void,
): Promise<void> {
  const loadId = begin(state);
  const current = currentAccess(state);
  const data = readForm(form);
  const enabledInput = inputNamed(form, "enabled");
  const body: Record<string, unknown> = {
    enabled: enabledInput !== undefined && enabledInput.checked,
  };
  assignText(body, "client_id", formText(data, "client_id"));
  assignText(body, "issuer_url", formText(data, "issuer_url"));
  assignText(body, "team_id", formText(data, "team_id"));
  assignText(body, "key_id", formText(data, "key_id"));
  assignText(body, "services_id", formText(data, "services_id"));
  const secret = secretValue(form);
  if (secret !== "") {
    body.secret = secret;
  }
  const result = await apiRequest(view.location.origin, `/v1/admin/login-providers/${provider}`, state.token, "PUT", body);
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    state.page = accessState(current.backend, current.providers, result.message, current.sabia, current.email);
    paint();
    return;
  }
  const parsed = parseLoginProvider(result.body);
  if (!parsed.ok) {
    state.page = accessState(current.backend, current.providers, parsed.message, current.sabia, current.email);
    paint();
    return;
  }
  state.notice = "Login guardado. O segredo não volta para esta tela.";
  state.page = accessState(current.backend, replaceProvider(current.providers, parsed.value), "", current.sabia, current.email);
  paint();
}

async function removeLogin(view: Window, state: AppState, provider: string, paint: () => void): Promise<void> {
  const loadId = begin(state);
  const current = currentAccess(state);
  const result = await apiRequest(view.location.origin, `/v1/admin/login-providers/${provider}`, state.token, "DELETE");
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    state.page = accessState(current.backend, current.providers, result.message, current.sabia, current.email);
    paint();
    return;
  }
  const parsed = parseLoginProvider(result.body);
  if (!parsed.ok) {
    state.page = accessState(current.backend, current.providers, parsed.message, current.sabia, current.email);
    paint();
    return;
  }
  state.notice = "Login removido.";
  state.page = accessState(current.backend, replaceProvider(current.providers, parsed.value), "", current.sabia, current.email);
  paint();
}

function accessState(
  backend: SecretBackendView | undefined,
  providers: LoginProviderView[],
  problem: string,
  sabia: SabiaView | undefined,
  email: EmailView | undefined,
): Extract<PageState, { route: "access" }> {
  const page: Extract<PageState, { route: "access" }> = { route: "access", providers, problem };
  if (backend !== undefined) {
    page.backend = backend;
  }
  if (sabia !== undefined) {
    page.sabia = sabia;
  }
  if (email !== undefined) {
    page.email = email;
  }
  return page;
}

function currentAccess(state: AppState): { backend?: SecretBackendView; providers: LoginProviderView[]; sabia?: SabiaView; email?: EmailView } {
  if (state.page.route !== "access") {
    return { providers: [] };
  }
  const access: { backend?: SecretBackendView; providers: LoginProviderView[]; sabia?: SabiaView; email?: EmailView } = {
    providers: state.page.providers,
  };
  if (state.page.backend !== undefined) {
    access.backend = state.page.backend;
  }
  if (state.page.sabia !== undefined) {
    access.sabia = state.page.sabia;
  }
  if (state.page.email !== undefined) {
    access.email = state.page.email;
  }
  return access;
}

async function saveEmail(view: Window, state: AppState, form: HTMLFormElement, paint: () => void): Promise<void> {
  const loadId = begin(state);
  const current = currentAccess(state);
  const saved = await putEmail(view, state, form, loadId, current, paint);
  if (saved === undefined) {
    return;
  }
  state.notice = "E-mail guardado. A chave não volta para esta tela.";
  state.page = accessState(current.backend, current.providers, "", current.sabia, saved);
  paint();
}

async function removeEmail(view: Window, state: AppState, paint: () => void): Promise<void> {
  const loadId = begin(state);
  const current = currentAccess(state);
  const result = await apiRequest(view.location.origin, "/v1/admin/email", state.token, "DELETE");
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    state.page = accessState(current.backend, current.providers, result.message, current.sabia, current.email);
    paint();
    return;
  }
  const parsed = parseEmail(result.body);
  if (!parsed.ok) {
    state.page = accessState(current.backend, current.providers, parsed.message, current.sabia, current.email);
    paint();
    return;
  }
  state.notice = "E-mail removido.";
  state.page = accessState(current.backend, current.providers, "", current.sabia, parsed.value);
  paint();
}

async function putEmail(
  view: Window,
  state: AppState,
  form: HTMLFormElement,
  loadId: number,
  current: { backend?: SecretBackendView; providers: LoginProviderView[]; sabia?: SabiaView; email?: EmailView },
  paint: () => void,
): Promise<EmailView | undefined> {
  const data = readForm(form);
  const provider = formText(data, "provider");
  const body: Record<string, unknown> = {
    provider,
    from_address: formText(data, "from_address"),
    to_address: formText(data, "to_address"),
    account_ref: provider === "cloudflare" || provider === "ses" ? formText(data, "account_ref") : "",
    active: inputNamed(form, "active")?.checked === true,
  };
  if (provider === "ses") {
    const accessKey = formText(data, "access_key_id");
    const secretKey = formText(data, "secret_access_key");
    if (accessKey !== "") {
      body.access_key_id = accessKey;
    }
    if (secretKey !== "") {
      body.secret_access_key = secretKey;
    }
  } else {
    const apiKey = formText(data, "api_key");
    if (apiKey !== "") {
      body.api_key = apiKey;
    }
  }
  const result = await apiRequest(view.location.origin, "/v1/admin/email", state.token, "PUT", body);
  if (loadId !== state.loadId) {
    return undefined;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return undefined;
    }
    state.page = accessState(current.backend, current.providers, result.message, current.sabia, current.email);
    paint();
    return undefined;
  }
  const parsed = parseEmail(result.body);
  if (!parsed.ok) {
    state.page = accessState(current.backend, current.providers, parsed.message, current.sabia, current.email);
    paint();
    return undefined;
  }
  return parsed.value;
}

async function saveSabia(view: Window, state: AppState, form: HTMLFormElement, paint: () => void): Promise<void> {
  const loadId = begin(state);
  const current = currentAccess(state);
  const saved = await putSabia(view, state, form, loadId, current, paint);
  if (saved === undefined) {
    return;
  }
  state.notice = "Template guardado. A chave de deploy não volta para esta tela.";
  state.page = accessState(current.backend, current.providers, "", saved, current.email);
  paint();
}

async function planSabia(view: Window, state: AppState, form: HTMLFormElement, paint: () => void): Promise<void> {
  const loadId = begin(state);
  const current = currentAccess(state);
  const saved = await putSabia(view, state, form, loadId, current, paint);
  if (saved === undefined) {
    return;
  }
  const result = await apiRequest(view.location.origin, "/v1/admin/sabia/plan", state.token, "POST");
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    state.page = accessState(current.backend, current.providers, result.message, saved, current.email);
    paint();
    return;
  }
  const plan = parseSabiaPlan(result.body);
  if (!plan.ok) {
    state.page = accessState(current.backend, current.providers, plan.message, saved, current.email);
    paint();
    return;
  }
  state.notice = "Plano do Sabiá recebido.";
  state.page = accessState(current.backend, current.providers, "", withChanges(saved, plan.value), current.email);
  paint();
}

async function applySabia(view: Window, state: AppState, form: HTMLFormElement, paint: () => void): Promise<void> {
  const confirmations = selectedConfirmations(form);
  const loadId = begin(state);
  const current = currentAccess(state);
  const saved = await putSabia(view, state, form, loadId, current, paint);
  if (saved === undefined) {
    return;
  }
  const result = await apiRequest(view.location.origin, "/v1/admin/sabia/apply", state.token, "POST", { confirmations });
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    const changes = current.sabia?.changes;
    const next = changes === undefined ? saved : withChanges(saved, changes);
    state.page = accessState(current.backend, current.providers, result.message, next, current.email);
    paint();
    return;
  }
  const applied = parseSabiaApply(result.body);
  if (!applied.ok) {
    state.page = accessState(current.backend, current.providers, applied.message, saved, current.email);
    paint();
    return;
  }
  state.notice = `Sincronizado com o Sabiá. Aplicado: ${applied.value.applied}. Sem mudança: ${applied.value.unchanged}.`;
  state.page = accessState(current.backend, current.providers, "", saved, current.email);
  paint();
}

async function putSabia(
  view: Window,
  state: AppState,
  form: HTMLFormElement,
  loadId: number,
  current: { backend?: SecretBackendView; providers: LoginProviderView[]; sabia?: SabiaView; email?: EmailView },
  paint: () => void,
): Promise<SabiaView | undefined> {
  const data = readForm(form);
  const body: Record<string, unknown> = {
    origin: formText(data, "origin"),
    application_id: formText(data, "application_id"),
    recipient: formText(data, "recipient"),
    template: {
      name: formText(data, "name"),
      subject: formText(data, "subject"),
      html: formText(data, "html"),
      text: formText(data, "text"),
    },
  };
  const keyInput = inputNamed(form, "deploy_key");
  const deployKey = keyInput === undefined ? "" : keyInput.value.trim();
  if (deployKey !== "") {
    body.deploy_key = deployKey;
  }
  const sendInput = inputNamed(form, "send_key");
  const sendKey = sendInput === undefined ? "" : sendInput.value.trim();
  if (sendKey !== "") {
    body.send_key = sendKey;
  }
  const result = await apiRequest(view.location.origin, "/v1/admin/sabia", state.token, "PUT", body);
  if (loadId !== state.loadId) {
    return undefined;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return undefined;
    }
    state.page = accessState(current.backend, current.providers, result.message, current.sabia, current.email);
    paint();
    return undefined;
  }
  const parsed = parseSabia(result.body);
  if (!parsed.ok) {
    state.page = accessState(current.backend, current.providers, parsed.message, current.sabia, current.email);
    paint();
    return undefined;
  }
  return parsed.value;
}

function withChanges(sabia: SabiaView, changes: SabiaChangeView[]): SabiaView {
  const next: SabiaView = {
    origin: sabia.origin,
    applicationId: sabia.applicationId,
    recipient: sabia.recipient,
    deployKeyPresent: sabia.deployKeyPresent,
    sendKeyPresent: sabia.sendKeyPresent,
    name: sabia.name,
    subject: sabia.subject,
    html: sabia.html,
    text: sabia.text,
    changes,
  };
  if (sabia.lastStatus !== undefined) {
    next.lastStatus = sabia.lastStatus;
  }
  if (sabia.lastDetail !== undefined) {
    next.lastDetail = sabia.lastDetail;
  }
  return next;
}

function selectedConfirmations(form: HTMLFormElement): string[] {
  const ids: string[] = [];
  const realm = form.ownerDocument.defaultView;
  if (realm === null) {
    return ids;
  }
  const nodes = form.querySelectorAll("[data-confirm]");
  for (const node of nodes) {
    if (node instanceof realm.HTMLInputElement && node.checked && node.value !== "") {
      ids.push(node.value);
    }
  }
  return ids;
}

function replaceProvider(providers: LoginProviderView[], next: LoginProviderView): LoginProviderView[] {
  const out: LoginProviderView[] = [];
  let found = false;
  for (const item of providers) {
    if (item.provider === next.provider) {
      out.push(next);
      found = true;
    } else {
      out.push(item);
    }
  }
  if (!found) {
    out.push(next);
  }
  return out;
}

function secretValue(form: HTMLFormElement): string {
  const realm = form.ownerDocument.defaultView;
  const node = form.elements.namedItem("secret");
  if (realm === null) {
    return "";
  }
  if (node instanceof realm.HTMLInputElement || node instanceof realm.HTMLTextAreaElement) {
    return node.value;
  }
  return "";
}

function inputNamed(form: HTMLFormElement, name: string): HTMLInputElement | undefined {
  const realm = form.ownerDocument.defaultView;
  const node = form.elements.namedItem(name);
  if (realm === null || !(node instanceof realm.HTMLInputElement)) {
    return undefined;
  }
  return node;
}

function readForm(form: HTMLFormElement): FormData {
  const realm = form.ownerDocument.defaultView;
  if (realm === null) {
    throw new Error("janela ausente");
  }
  return new realm.FormData(form);
}

function formText(data: FormData, key: string): string {
  const value = data.get(key);
  if (typeof value !== "string") {
    return "";
  }
  return value.trim();
}

function assignText(body: Record<string, unknown>, key: string, value: string): void {
  if (value !== "") {
    body[key] = value;
  }
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
  state.screen = { type: "ready", role: "operator" };
  state.notice = "";
  state.overlay = { type: "operator-token", token };
  paint();
  void loadRoute(view, state, currentRoute(view), paint);
}

function currentRoute(view: Window): RouteName {
  return parseRoute(view.location.pathname, view.location.hash);
}

function canonicalize(view: Window): void {
  const path = screenPath(currentRoute(view));
  if (view.location.pathname === path && view.location.hash === "") {
    return;
  }
  view.history.replaceState({}, "", `${path}${view.location.search}`);
}

function visit(view: Window, state: AppState, path: string, paint: () => void): void {
  const here = view.location.pathname === path && view.location.hash === "" && view.location.search === "";
  if (!here) {
    view.history.pushState({}, "", path);
  }
  if (state.screen.type !== "ready") {
    return;
  }
  void loadRoute(view, state, currentRoute(view), paint);
}

function screenLink(
  doc: Document,
  view: Window,
  state: AppState,
  paint: () => void,
  route: RouteName,
  text: string,
): HTMLAnchorElement {
  const node = link(doc, screenPath(route), text, state.page.route === route);
  node.addEventListener("click", (event) => {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
      return;
    }
    event.preventDefault();
    visit(view, state, screenPath(route), paint);
  });
  return node;
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
  if (personView(state) && (route === "alerts" || route === "access")) {
    if (route === "alerts") {
      state.page = { route: "alerts", subscriptions: [], deliveries: [], push: "unconfigured", problem: "" };
    } else {
      state.page = { route: "access", providers: [], problem: "" };
    }
    paint();
    return;
  }
  if (route === "alerts") {
    await loadAlerts(view, state, paint);
    return;
  }
  await loadAccess(view, state, paint);
}

async function loadOverview(view: Window, state: AppState, paint: () => void): Promise<void> {
  const loadId = begin(state);
  state.page = { route: "overview", fromCache: false, problem: "" };
  paint();
  const result = await apiRequest(view.location.origin, overviewRequest(state), state.token, "GET");
  if (loadId !== state.loadId) {
    return;
  }
  if (!result.ok) {
    if (result.status === 401) {
      loseSession(view, state, result.message, paint);
      return;
    }
    if (result.errorCode === "NETWORK") {
      const cached = readCache(view, personView(state));
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
  view.localStorage.removeItem(overviewCacheKey);
  view.localStorage.setItem(
    overviewCacheStorageKey(personView(state)),
    JSON.stringify({ saved_at: new Date().toISOString(), overview: result.body }),
  );
  state.page = { route: "overview", overview: parsed.value, fromCache: false, problem: "" };
  paint();
  void loadTrend(view, state, loadId, paint);
}

// A tendência é um enfeite da leitura. Se falhar, a visão continua como está.
async function loadTrend(view: Window, state: AppState, loadId: number, paint: () => void): Promise<void> {
  const result = await apiRequest(view.location.origin, trendRequest(state), state.token, "GET");
  if (!result.ok || loadId !== state.loadId || state.page.route !== "overview" || state.page.fromCache) {
    return;
  }
  const parsed = parseHistory(result.body);
  if (!parsed.ok) {
    return;
  }
  state.page = { ...state.page, trend: parsed.value.samples };
  paint();
}

async function loadHistory(view: Window, state: AppState, cursor: string, paint: () => void): Promise<void> {
  const loadId = begin(state);
  const path = samplesRequest(state, cursor);
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
  const result = await apiRequest(view.location.origin, devicesRequest(state), state.token, "GET");
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
  const result = await apiRequest(view.location.origin, enrollmentRequest(state), state.token, "POST");
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
  const result = await apiRequest(view.location.origin, revokeRequest(state, id), state.token, "POST");
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
  clearOverviewCache(view.localStorage);
  paint();
}

function readCache(view: Window, person: boolean): Overview | undefined {
  const raw = view.localStorage.getItem(overviewCacheStorageKey(person));
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

function pageHead(doc: Document, title: string, lede: string, action?: HTMLElement): HTMLElement {
  const head = doc.createElement("div");
  head.className = "page-head";
  const copy = doc.createElement("div");
  copy.append(el(doc, "h1", "", title), el(doc, "p", "lede", lede));
  head.append(copy);
  if (action !== undefined) {
    head.append(action);
  }
  return head;
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
  node.setAttribute("href", href);
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
