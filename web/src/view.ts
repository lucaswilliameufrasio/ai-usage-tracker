export const operatorTokenKey = "tracker.operator_token";
export const overviewCacheKey = "tracker.overview.cache";

export type RouteName = "overview" | "history" | "devices" | "alerts" | "access";

export type LoginProviderName = "github" | "gitea" | "google" | "apple";

export type LoginProviderView = {
  provider: LoginProviderName;
  enabled: boolean;
  configured: boolean;
  secretPresent: boolean;
  clientId?: string;
  issuerUrl?: string;
  teamId?: string;
  keyId?: string;
  servicesId?: string;
  updatedAt?: string;
};

export type SecretBackendView = {
  kind: string;
  ready: boolean;
};

export type WindowView = {
  name: string;
  used: number;
  resetsAt: string;
};

export type ProjectionView = {
  window: string;
  estimatedUsed: number;
  resetsAt: string;
  dataQuality: string;
};

export type ProviderCard = {
  provider: "claude" | "codex";
  freshness: string;
  reasonCode?: string;
  observedAt?: string;
  plan?: string;
  windows: WindowView[];
  projection?: ProjectionView;
};

export type Overview = {
  generatedAt: string;
  staleAfterSeconds: number;
  providers: ProviderCard[];
};

export type HistorySample = {
  sampleId: string;
  provider: string;
  collectedAt: string;
  plan?: string;
  windows: WindowView[];
};

export type HistoryPage = {
  samples: HistorySample[];
  hasMore: boolean;
  nextCursor: string;
};

export type DeviceView = {
  id: string;
  name: string;
  createdAt: string;
  revokedAt?: string;
};

export type SettingsView = {
  staleAfterSeconds: number;
  alertThresholdPercent: number;
  timezone: string;
  quietStart: string;
  quietEnd: string;
};

export type SubscriptionView = {
  id: string;
  endpoint: string;
  createdAt: string;
  status: string;
  expiredAt?: string;
  lastError?: string;
};

export type DeliveryView = {
  id: string;
  kind: string;
  provider: string;
  title: string;
  body: string;
  status: string;
  createdAt: string;
  detail?: string;
};

export type ParseOk<T> = { ok: true; value: T };
export type ParseFail = { ok: false; message: string };
export type Parsed<T> = ParseOk<T> | ParseFail;

export type ReadingCopy = {
  headline: string;
  detail: string;
  banner?: string;
  showAsCurrent: boolean;
};

export function screenPath(route: RouteName): string {
  if (route === "history") {
    return "/historico";
  }
  if (route === "devices") {
    return "/dispositivos";
  }
  if (route === "alerts") {
    return "/alertas";
  }
  if (route === "access") {
    return "/acesso";
  }
  return "/";
}

export function parseRoute(pathname: string, hash = ""): RouteName {
  const legacy = legacyRoute(hash);
  if (legacy !== undefined) {
    return legacy;
  }
  return pathRoute(pathname);
}

function legacyRoute(hash: string): RouteName | undefined {
  if (!hash.startsWith("#")) {
    return undefined;
  }
  const body = hash.slice(1);
  if (body === "" || body === "/") {
    return "overview";
  }
  return namedRoute(firstSegment(body));
}

function pathRoute(pathname: string): RouteName {
  const route = namedRoute(firstSegment(pathname));
  if (route === undefined) {
    return "overview";
  }
  return route;
}

function firstSegment(value: string): string {
  const raw = value.startsWith("/") ? value.slice(1) : value;
  const name = raw.split("/")[0] ?? "";
  return name;
}

function namedRoute(name: string): RouteName | undefined {
  if (name === "historico" || name === "history") {
    return "history";
  }
  if (name === "dispositivos" || name === "devices") {
    return "devices";
  }
  if (name === "alertas" || name === "alerts") {
    return "alerts";
  }
  if (name === "acesso" || name === "access") {
    return "access";
  }
  return undefined;
}

const loginProviders: LoginProviderName[] = ["github", "gitea", "google", "apple"];

export type LoginGuide = {
  steps: string[];
  example: string;
};

const loginGuides: Record<LoginProviderName, LoginGuide> = {
  github: {
    steps: [
      "No GitHub, abra Settings, Developer settings, OAuth Apps e crie um OAuth App. Não crie um GitHub App.",
      "Copie o Client ID que aparece na página do aplicativo.",
      "Gere um client secret. O GitHub mostra esse valor uma vez.",
    ],
    example: "Client id: Iv1.exemploNaoUsar\nSegredo: exemplo-segredo-github",
  },
  gitea: {
    steps: [
      "No Gitea, abra Configurações, Aplicativos, e crie um aplicativo OAuth2.",
      "A URL base é o endereço do Gitea, com http ou https, sem usuário e sem caminho.",
      "Copie o Client ID e o Client Secret da página do aplicativo.",
    ],
    example: "URL base: https://git.exemplo.com\nClient id: 7c9e1a20-0000-4000-8000-000000000000\nSegredo: exemplo-segredo-gitea",
  },
  google: {
    steps: [
      "No Google Cloud Console, abra Credenciais e crie um ID do cliente OAuth do tipo Aplicativo da Web.",
      "Copie o ID do cliente. Ele termina em .apps.googleusercontent.com.",
      "Copie o segredo do cliente mostrado na criação.",
    ],
    example: "Client id: 000000000000-exemplo.apps.googleusercontent.com\nSegredo: exemplo-segredo-google",
  },
  apple: {
    steps: [
      "Em developer.apple.com, o Team ID fica no canto da conta. São 10 letras e números.",
      "Crie um Services ID. Esse identificador é o client id. O campo Client id desta tela pode ficar vazio.",
      "Em Keys, crie uma chave com Sign in with Apple. Anote o Key ID. Baixe o .p8 nessa hora: a Apple não deixa baixar de novo.",
      "Cole o texto do arquivo, com BEGIN PRIVATE KEY e END PRIVATE KEY. Não transforme em outro base64 e não cole um JWT.",
    ],
    example: "Team ID: AB12CD34EF\nKey ID: FG56HI78JK\nServices ID: com.exemplo.rastreio\n-----BEGIN PRIVATE KEY-----\nMIGHExemploNaoEUmaChave\n-----END PRIVATE KEY-----",
  },
};

export function loginGuide(provider: string): LoginGuide | undefined {
  if (!isLoginProvider(provider)) {
    return undefined;
  }
  return loginGuides[provider];
}

export function loginCallbackPath(provider: string): string | undefined {
  if (!isLoginProvider(provider)) {
    return undefined;
  }
  return `/v1/login/${provider}/callback`;
}

export function parsePublicLogins(value: unknown): string[] {
  if (!isRecord(value) || !Array.isArray(value.data)) {
    return [];
  }
  const names: string[] = [];
  for (const item of value.data) {
    if (!isRecord(item)) {
      continue;
    }
    const provider = item.provider;
    if (typeof provider === "string" && isLoginProvider(provider)) {
      names.push(provider);
    }
  }
  return names;
}

export function parseSession(value: unknown): string | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  const provider = value.provider;
  if (typeof provider !== "string" || !isLoginProvider(provider)) {
    return undefined;
  }
  return provider;
}

export function loginLabel(provider: string): string {
  if (provider === "github") {
    return "GitHub";
  }
  if (provider === "gitea") {
    return "Gitea";
  }
  if (provider === "google") {
    return "Google";
  }
  if (provider === "apple") {
    return "Apple";
  }
  return "Login";
}

export function parseSecretBackend(value: unknown): Parsed<SecretBackendView> {
  if (!isRecord(value)) {
    return fail("O cofre veio ilegível.");
  }
  const kind = readString(value, "kind");
  const ready = readBool(value, "ready");
  if (kind === undefined || kind === "" || ready === undefined) {
    return fail("O cofre veio ilegível.");
  }
  return { ok: true, value: { kind, ready } };
}

export function parseLoginCatalog(value: unknown): Parsed<LoginProviderView[]> {
  if (!isRecord(value) || !Array.isArray(value.data)) {
    return fail("A lista de logins veio ilegível.");
  }
  const byName = new Map<LoginProviderName, LoginProviderView>();
  for (const item of value.data) {
    const parsed = parseLoginProvider(item);
    if (!parsed.ok) {
      return fail("A lista de logins veio incompleta.");
    }
    byName.set(parsed.value.provider, parsed.value);
  }
  const items: LoginProviderView[] = [];
  for (const name of loginProviders) {
    const item = byName.get(name);
    if (item === undefined) {
      return fail("A lista de logins veio incompleta.");
    }
    items.push(item);
  }
  return { ok: true, value: items };
}

export function parseLoginProvider(value: unknown): Parsed<LoginProviderView> {
  if (!isRecord(value)) {
    return fail("O login veio ilegível.");
  }
  const provider = readString(value, "provider");
  const enabled = readBool(value, "enabled");
  const configured = readBool(value, "configured");
  const secretPresent = readBool(value, "secret_present");
  if (
    !isLoginProvider(provider) ||
    enabled === undefined ||
    configured === undefined ||
    secretPresent === undefined
  ) {
    return fail("O login veio ilegível.");
  }
  const view: LoginProviderView = { provider, enabled, configured, secretPresent };
  const clientId = readString(value, "client_id");
  const issuerUrl = readString(value, "issuer_url");
  const teamId = readString(value, "team_id");
  const keyId = readString(value, "key_id");
  const servicesId = readString(value, "services_id");
  const updatedAt = readString(value, "updated_at");
  if (clientId !== undefined && clientId !== "") {
    view.clientId = clientId;
  }
  if (issuerUrl !== undefined && issuerUrl !== "") {
    view.issuerUrl = issuerUrl;
  }
  if (teamId !== undefined && teamId !== "") {
    view.teamId = teamId;
  }
  if (keyId !== undefined && keyId !== "") {
    view.keyId = keyId;
  }
  if (servicesId !== undefined && servicesId !== "") {
    view.servicesId = servicesId;
  }
  if (updatedAt !== undefined && updatedAt !== "") {
    view.updatedAt = updatedAt;
  }
  return { ok: true, value: view };
}

function isLoginProvider(value: string | undefined): value is LoginProviderName {
  return value === "github" || value === "gitea" || value === "google" || value === "apple";
}

export function formatInstant(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "instante desconhecido";
  }
  const day = String(date.getUTCDate()).padStart(2, "0");
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const year = String(date.getUTCFullYear());
  const hour = String(date.getUTCHours()).padStart(2, "0");
  const minute = String(date.getUTCMinutes()).padStart(2, "0");
  return `${day}/${month}/${year} ${hour}:${minute} UTC`;
}

export function formatUsed(value: number): string {
  if (!Number.isFinite(value)) {
    return "0";
  }
  const rounded = Math.round(value * 10) / 10;
  if (Number.isInteger(rounded)) {
    return String(rounded);
  }
  return rounded.toFixed(1);
}

export function providerLabel(provider: string): string {
  if (provider === "claude") {
    return "Claude";
  }
  if (provider === "codex") {
    return "Codex";
  }
  return "Provedor";
}

export function windowLabel(name: string): string {
  if (name === "five_hour") {
    return "5 horas";
  }
  if (name === "seven_day") {
    return "7 dias";
  }
  if (name.startsWith("minutes_")) {
    return `${name.slice("minutes_".length)} minutos`;
  }
  return name;
}

export function reasonDetail(reasonCode: string | undefined): string {
  if (reasonCode === "LIMITS_ABSENT") {
    return "A fonte local não trouxe limites.";
  }
  if (reasonCode === "RESET_UNKNOWN") {
    return "A fonte local não trouxe o instante de reset.";
  }
  if (reasonCode === "SOURCE_UNREADABLE") {
    return "Não foi possível ler a fonte local.";
  }
  return "O coletor não conseguiu uma leitura deste provedor.";
}

export function readingCopy(
  freshness: string,
  observedAt: string | undefined,
  reasonCode: string | undefined,
  fromCache: boolean,
): ReadingCopy {
  const when = observedAt === undefined || observedAt === "" ? "" : formatInstant(observedAt);
  if (fromCache) {
    const detail = when === ""
      ? "Estes dados estão em cache e não são a leitura atual."
      : `Coletado em ${when}. Estes dados estão em cache e não são a leitura atual.`;
    return {
      headline: "Dados em cache",
      detail,
      banner: "Estes dados estão em cache e não são a leitura atual.",
      showAsCurrent: false,
    };
  }
  if (freshness === "current") {
    return {
      headline: "Atual",
      detail: when === "" ? "A coleta está dentro do prazo." : `Coletado em ${when}.`,
      showAsCurrent: true,
    };
  }
  if (freshness === "stale") {
    const detail = when === ""
      ? "A última coleta passou do prazo."
      : `Coletado em ${when}.`;
    return {
      headline: "Desatualizado",
      detail,
      showAsCurrent: false,
    };
  }
  if (freshness === "unavailable") {
    return {
      headline: "Provedor indisponível",
      detail: reasonDetail(reasonCode),
      showAsCurrent: false,
    };
  }
  return {
    headline: "Sem dados",
    detail: "Nenhuma amostra deste provedor chegou ainda.",
    showAsCurrent: false,
  };
}

export function staffLevel(used: number): number {
  if (!Number.isFinite(used) || used <= 0) {
    return 0;
  }
  if (used >= 100) {
    return 100;
  }
  return used;
}

export function staffTone(used: number): "calm" | "watch" | "flood" {
  if (used >= 95) {
    return "flood";
  }
  if (used >= 80) {
    return "watch";
  }
  return "calm";
}

export function primaryWindow(windows: WindowView[]): WindowView | undefined {
  for (const item of windows) {
    if (item.name === "five_hour") {
      return item;
    }
  }
  return windows[0];
}

export function projectionCopy(projection: ProjectionView): string {
  const base = projection.dataQuality === "stale"
    ? "A base já está desatualizada."
    : "A base é o histórico recente.";
  return `Estimativa de ${formatUsed(projection.estimatedUsed)}% em ${windowLabel(projection.window)} até ${formatInstant(projection.resetsAt)}. ${base} Não é uma leitura do provedor.`;
}

export function deliveryLabel(status: string): string {
  if (status === "sent") {
    return "Enviado";
  }
  if (status === "pending") {
    return "Na fila";
  }
  if (status === "failed") {
    return "Falhou";
  }
  if (status === "suppressed") {
    return "Suprimido no horário silencioso";
  }
  if (status === "no_subscription") {
    return "Sem inscrição de push";
  }
  if (status === "unconfigured") {
    return "Push não configurado";
  }
  if (status === "subscription_expired") {
    return "Inscrição expirada";
  }
  return "Estado desconhecido";
}

export function subscriptionLabel(status: string): string {
  if (status === "active") {
    return "Ativa";
  }
  if (status === "expired") {
    return "Expirada";
  }
  return "Estado desconhecido";
}

export function parseOverview(value: unknown): Parsed<Overview> {
  if (!isRecord(value)) {
    return fail("A visão geral chegou ilegível.");
  }
  const providers = value.providers;
  if (!Array.isArray(providers)) {
    return fail("A visão geral não tem provedores.");
  }
  const found = new Map<"claude" | "codex", ProviderCard>();
  for (const item of providers) {
    const card = parseCard(item);
    if (card) {
      found.set(card.provider, card);
    }
  }
  const claude = found.get("claude") ?? emptyCard("claude");
  const codex = found.get("codex") ?? emptyCard("codex");
  return {
    ok: true,
    value: {
      generatedAt: readString(value, "generated_at") ?? "",
      staleAfterSeconds: readNumber(value, "stale_after_seconds") ?? 0,
      providers: [claude, codex],
    },
  };
}

export function parseCachedOverview(value: unknown): Parsed<Overview> {
  if (!isRecord(value)) {
    return fail("A cópia local está ilegível.");
  }
  return parseOverview(value.overview);
}

export function parseHistory(value: unknown): Parsed<HistoryPage> {
  if (!isRecord(value) || !Array.isArray(value.data)) {
    return fail("O histórico chegou ilegível.");
  }
  const samples: HistorySample[] = [];
  for (const item of value.data) {
    const sample = parseSample(item);
    if (sample) {
      samples.push(sample);
    }
  }
  return {
    ok: true,
    value: {
      samples,
      hasMore: value.has_more === true,
      nextCursor: readString(value, "next_cursor") ?? "",
    },
  };
}

export function parseDevices(value: unknown): Parsed<DeviceView[]> {
  if (!isRecord(value) || !Array.isArray(value.data)) {
    return fail("A lista de dispositivos chegou ilegível.");
  }
  const devices: DeviceView[] = [];
  for (const item of value.data) {
    if (!isRecord(item)) {
      continue;
    }
    const id = readString(item, "id");
    const name = readString(item, "name");
    const createdAt = readString(item, "created_at");
    if (id === undefined || name === undefined || createdAt === undefined) {
      continue;
    }
    const device: DeviceView = { id, name, createdAt };
    const revokedAt = readString(item, "revoked_at");
    if (revokedAt !== undefined) {
      device.revokedAt = revokedAt;
    }
    devices.push(device);
  }
  return { ok: true, value: devices };
}

export function parseSettings(value: unknown): Parsed<SettingsView> {
  if (!isRecord(value)) {
    return fail("As preferências chegaram ilegíveis.");
  }
  const stale = readNumber(value, "stale_after_seconds");
  const threshold = readNumber(value, "alert_threshold_percent");
  const timezone = readString(value, "timezone");
  if (stale === undefined || threshold === undefined || timezone === undefined) {
    return fail("As preferências chegaram incompletas.");
  }
  const settings: SettingsView = {
    staleAfterSeconds: stale,
    alertThresholdPercent: threshold,
    timezone,
    quietStart: "",
    quietEnd: "",
  };
  const quiet = value.quiet_hours;
  if (isRecord(quiet)) {
    settings.quietStart = readString(quiet, "start") ?? "";
    settings.quietEnd = readString(quiet, "end") ?? "";
  }
  return { ok: true, value: settings };
}

export function parseSubscriptions(value: unknown): Parsed<SubscriptionView[]> {
  if (!isRecord(value) || !Array.isArray(value.data)) {
    return fail("As inscrições chegaram ilegíveis.");
  }
  const items: SubscriptionView[] = [];
  for (const item of value.data) {
    if (!isRecord(item)) {
      continue;
    }
    const id = readString(item, "id");
    const endpoint = readString(item, "endpoint");
    const createdAt = readString(item, "created_at");
    const status = readString(item, "status");
    if (id === undefined || endpoint === undefined || createdAt === undefined || status === undefined) {
      continue;
    }
    const subscription: SubscriptionView = { id, endpoint, createdAt, status };
    const expiredAt = readString(item, "expired_at");
    const lastError = readString(item, "last_error");
    if (expiredAt !== undefined) {
      subscription.expiredAt = expiredAt;
    }
    if (lastError !== undefined) {
      subscription.lastError = lastError;
    }
    items.push(subscription);
  }
  return { ok: true, value: items };
}

export function parseDeliveries(value: unknown): Parsed<DeliveryView[]> {
  if (!isRecord(value) || !Array.isArray(value.data)) {
    return fail("Os avisos chegaram ilegíveis.");
  }
  const items: DeliveryView[] = [];
  for (const item of value.data) {
    if (!isRecord(item)) {
      continue;
    }
    const id = readString(item, "id");
    const kind = readString(item, "kind");
    const provider = readString(item, "provider");
    const title = readString(item, "title");
    const body = readString(item, "body");
    const status = readString(item, "status");
    const createdAt = readString(item, "created_at");
    if (
      id === undefined ||
      kind === undefined ||
      provider === undefined ||
      title === undefined ||
      body === undefined ||
      status === undefined ||
      createdAt === undefined
    ) {
      continue;
    }
    const delivery: DeliveryView = { id, kind, provider, title, body, status, createdAt };
    const detail = readString(item, "detail");
    if (detail !== undefined) {
      delivery.detail = detail;
    }
    items.push(delivery);
  }
  return { ok: true, value: items };
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function fail(message: string): ParseFail {
  return { ok: false, message };
}

function emptyCard(provider: "claude" | "codex"): ProviderCard {
  return { provider, freshness: "empty", windows: [] };
}

function parseCard(value: unknown): ProviderCard | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  const provider = readString(value, "provider");
  if (provider !== "claude" && provider !== "codex") {
    return undefined;
  }
  const card: ProviderCard = {
    provider,
    freshness: readString(value, "freshness") ?? "empty",
    windows: [],
  };
  const reasonCode = readString(value, "reason_code");
  const observedAt = readString(value, "observed_at");
  if (reasonCode !== undefined) {
    card.reasonCode = reasonCode;
  }
  if (observedAt !== undefined) {
    card.observedAt = observedAt;
  }
  const lastSample = value.last_sample;
  if (isRecord(lastSample)) {
    const plan = readString(lastSample, "plan");
    if (plan !== undefined) {
      card.plan = plan;
    }
    card.windows = parseWindows(lastSample.windows);
  }
  const projection = parseProjection(value.projection);
  if (projection) {
    card.projection = projection;
  }
  return card;
}

function parseSample(value: unknown): HistorySample | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  const sampleId = readString(value, "sample_id");
  const provider = readString(value, "provider");
  const collectedAt = readString(value, "collected_at");
  if (sampleId === undefined || provider === undefined || collectedAt === undefined) {
    return undefined;
  }
  const sample: HistorySample = {
    sampleId,
    provider,
    collectedAt,
    windows: parseWindows(value.windows),
  };
  const plan = readString(value, "plan");
  if (plan !== undefined) {
    sample.plan = plan;
  }
  return sample;
}

function parseWindows(value: unknown): WindowView[] {
  if (!Array.isArray(value)) {
    return [];
  }
  const windows: WindowView[] = [];
  for (const item of value) {
    if (!isRecord(item)) {
      continue;
    }
    const name = readString(item, "name");
    const used = readNumber(item, "used");
    const resetsAt = readString(item, "resets_at");
    if (name === undefined || used === undefined || resetsAt === undefined) {
      continue;
    }
    windows.push({ name, used, resetsAt });
  }
  return windows;
}

function parseProjection(value: unknown): ProjectionView | undefined {
  if (!isRecord(value)) {
    return undefined;
  }
  const windowName = readString(value, "window");
  const estimatedUsed = readNumber(value, "estimated_used");
  const resetsAt = readString(value, "resets_at");
  const dataQuality = readString(value, "data_quality");
  if (windowName === undefined || estimatedUsed === undefined || resetsAt === undefined || dataQuality === undefined) {
    return undefined;
  }
  return { window: windowName, estimatedUsed, resetsAt, dataQuality };
}

function readString(record: Record<string, unknown>, key: string): string | undefined {
  const value = record[key];
  if (typeof value !== "string") {
    return undefined;
  }
  return value;
}

function readBool(record: Record<string, unknown>, key: string): boolean | undefined {
  const value = record[key];
  if (typeof value !== "boolean") {
    return undefined;
  }
  return value;
}

function readNumber(record: Record<string, unknown>, key: string): number | undefined {
  const value = record[key];
  if (typeof value !== "number" || !Number.isFinite(value)) {
    return undefined;
  }
  return value;
}
