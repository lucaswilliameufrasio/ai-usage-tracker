import {
  array,
  boolean,
  fallback,
  finite,
  minLength,
  number,
  object,
  optional,
  picklist,
  pipe,
  safeParse,
  string,
  transform,
  unknown,
} from "valibot";

export const operatorTokenKey = "tracker.operator_token";
export const overviewCacheKey = "tracker.overview.cache";

export function overviewCacheStorageKey(person: boolean): string {
  if (person) {
    return `${overviewCacheKey}.person`;
  }
  return `${overviewCacheKey}.operator`;
}

export function clearOverviewCache(storage: Pick<Storage, "removeItem">): void {
  storage.removeItem(overviewCacheKey);
  storage.removeItem(overviewCacheStorageKey(true));
  storage.removeItem(overviewCacheStorageKey(false));
}

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

export type SabiaChangeView = {
  id: string;
  operation: string;
  confirmationRequired: boolean;
};

export type SabiaView = {
  origin: string;
  applicationId: string;
  deployKeyPresent: boolean;
  name: string;
  subject: string;
  html: string;
  text: string;
  changes?: SabiaChangeView[];
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

export type AccountView = {
  id: string;
  label: string;
  login: string;
  providers: ProviderCard[];
};

export type Overview = {
  generatedAt: string;
  staleAfterSeconds: number;
  providers: ProviderCard[];
  accounts?: AccountView[];
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

const loginNames = ["github", "gitea", "google", "apple"] as const;
const loginNameSchema = picklist(loginNames);
const usageProviderSchema = picklist(["claude", "codex"] as const);
const softString = optional(
  pipe(
    unknown(),
    transform((value): string | undefined => {
      if (typeof value === "string") {
        return value;
      }
      return undefined;
    }),
  ),
);

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

const publicLoginSchema = object({
  data: array(unknown()),
});

const publicLoginItemSchema = object({
  provider: loginNameSchema,
});

const sessionSchema = object({
  provider: loginNameSchema,
});

const secretBackendSchema = object({
  kind: pipe(string(), minLength(1)),
  ready: boolean(),
});

const sabiaSchema = object({
  origin: optional(string()),
  application_id: optional(string()),
  deploy_key_present: boolean(),
  template: object({
    name: string(),
    subject: string(),
    html: string(),
    text: string(),
  }),
});

const sabiaPlanSchema = object({
  changes: array(object({
    id: string(),
    operation: string(),
    confirmation_required: boolean(),
  })),
});

const sabiaApplySchema = object({
  applied: pipe(number(), finite()),
  unchanged: pipe(number(), finite()),
});

const loginProviderSchema = object({
  provider: loginNameSchema,
  enabled: boolean(),
  configured: boolean(),
  secret_present: boolean(),
  client_id: softString,
  issuer_url: softString,
  team_id: softString,
  key_id: softString,
  services_id: softString,
  updated_at: softString,
});

const loginCatalogSchema = object({
  data: array(unknown()),
});

export function parsePublicLogins(value: unknown): string[] {
  const parsed = safeParse(publicLoginSchema, value);
  if (!parsed.success) {
    return [];
  }
  const names: string[] = [];
  for (const item of parsed.output.data) {
    const provider = safeParse(publicLoginItemSchema, item);
    if (provider.success) {
      names.push(provider.output.provider);
    }
  }
  return names;
}

export function parseSession(value: unknown): string | undefined {
  const parsed = safeParse(sessionSchema, value);
  if (!parsed.success) {
    return undefined;
  }
  return parsed.output.provider;
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
  const parsed = safeParse(secretBackendSchema, value);
  if (!parsed.success) {
    return fail("O cofre veio ilegível.");
  }
  return { ok: true, value: { kind: parsed.output.kind, ready: parsed.output.ready } };
}

export function parseSabia(value: unknown): Parsed<SabiaView> {
  const parsed = safeParse(sabiaSchema, value);
  if (!parsed.success) {
    return fail("O Sabiá veio ilegível.");
  }
  const item = parsed.output;
  return {
    ok: true,
    value: {
      origin: item.origin ?? "",
      applicationId: item.application_id ?? "",
      deployKeyPresent: item.deploy_key_present,
      name: item.template.name,
      subject: item.template.subject,
      html: item.template.html,
      text: item.template.text,
    },
  };
}

export function parseSabiaPlan(value: unknown): Parsed<SabiaChangeView[]> {
  const parsed = safeParse(sabiaPlanSchema, value);
  if (!parsed.success) {
    return fail("O plano do Sabiá veio ilegível.");
  }
  const changes: SabiaChangeView[] = [];
  for (const item of parsed.output.changes) {
    changes.push({
      id: item.id,
      operation: item.operation,
      confirmationRequired: item.confirmation_required,
    });
  }
  return { ok: true, value: changes };
}

export function parseSabiaApply(value: unknown): Parsed<{ applied: number; unchanged: number }> {
  const parsed = safeParse(sabiaApplySchema, value);
  if (!parsed.success) {
    return fail("A sincronização do Sabiá veio ilegível.");
  }
  return { ok: true, value: { applied: parsed.output.applied, unchanged: parsed.output.unchanged } };
}

export function parseLoginCatalog(value: unknown): Parsed<LoginProviderView[]> {
  const parsed = safeParse(loginCatalogSchema, value);
  if (!parsed.success) {
    return fail("A lista de logins veio ilegível.");
  }
  const byName = new Map<LoginProviderName, LoginProviderView>();
  for (const item of parsed.output.data) {
    const provider = parseLoginProvider(item);
    if (!provider.ok) {
      return fail("A lista de logins veio incompleta.");
    }
    byName.set(provider.value.provider, provider.value);
  }
  const items: LoginProviderView[] = [];
  for (const name of loginNames) {
    const item = byName.get(name);
    if (item === undefined) {
      return fail("A lista de logins veio incompleta.");
    }
    items.push(item);
  }
  return { ok: true, value: items };
}

export function parseLoginProvider(value: unknown): Parsed<LoginProviderView> {
  const parsed = safeParse(loginProviderSchema, value);
  if (!parsed.success) {
    return fail("O login veio ilegível.");
  }
  const item = parsed.output;
  const view: LoginProviderView = {
    provider: item.provider,
    enabled: item.enabled,
    configured: item.configured,
    secretPresent: item.secret_present,
  };
  if (presentString(item.client_id)) {
    view.clientId = item.client_id;
  }
  if (presentString(item.issuer_url)) {
    view.issuerUrl = item.issuer_url;
  }
  if (presentString(item.team_id)) {
    view.teamId = item.team_id;
  }
  if (presentString(item.key_id)) {
    view.keyId = item.key_id;
  }
  if (presentString(item.services_id)) {
    view.servicesId = item.services_id;
  }
  if (presentString(item.updated_at)) {
    view.updatedAt = item.updated_at;
  }
  return { ok: true, value: view };
}

function isLoginProvider(value: string | undefined): value is LoginProviderName {
  const parsed = safeParse(loginNameSchema, value);
  return parsed.success;
}

function presentString(value: string | undefined): value is string {
  return value !== undefined && value !== "";
}

export function formatInstant(iso: string, timeZone?: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "instante desconhecido";
  }
  const zone = timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone;
  const parts = new Intl.DateTimeFormat("pt-BR", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
    timeZoneName: "short",
  }).formatToParts(date);
  const day = partText(parts, "day");
  const month = partText(parts, "month");
  const year = partText(parts, "year");
  const hour = partText(parts, "hour");
  const minute = partText(parts, "minute");
  const zoneName = partText(parts, "timeZoneName");
  const clock = `${day}/${month}/${year} ${hour}:${minute}`;
  if (zoneName === "") {
    return clock;
  }
  return `${clock} ${zoneName}`;
}

function partText(parts: Intl.DateTimeFormatPart[], type: Intl.DateTimeFormatPartTypes): string {
  for (const part of parts) {
    if (part.type === type) {
      return part.value.replace(/[\u202f\u00a0]/g, "");
    }
  }
  return "";
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

export function deliveryDetail(code: string): string {
  if (code === "QUEUE_FULL") {
    return "A fila de avisos está cheia.";
  }
  if (code === "DELIVERY_EXPIRED") {
    return "O aviso passou do prazo.";
  }
  if (code === "SEND_FAILED") {
    return "O envio falhou depois das tentativas.";
  }
  if (code === "PUSH_UNCONFIGURED") {
    return "O push não está configurado.";
  }
  if (code === "SUBSCRIPTION_EXPIRED") {
    return "A inscrição não aceita mais avisos.";
  }
  if (code === "QUIET_HOURS") {
    return "O aviso ficou no horário silencioso.";
  }
  return "";
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

const finiteNumber = pipe(number(), finite());

const windowSchema = object({
  name: string(),
  used: finiteNumber,
  resets_at: string(),
});

const projectionSchema = object({
  window: string(),
  estimated_used: finiteNumber,
  resets_at: string(),
  data_quality: string(),
});

const lastSampleSchema = object({
  plan: softString,
  windows: optional(unknown()),
});

const cardSchema = object({
  provider: usageProviderSchema,
  freshness: optional(fallback(string(), "empty")),
  reason_code: softString,
  observed_at: softString,
  last_sample: optional(unknown()),
  projection: optional(unknown()),
});

const accountSchema = object({
  id: string(),
  label: string(),
  login: optional(fallback(string(), "")),
  providers: optional(array(unknown())),
});

const overviewSchema = object({
  generated_at: optional(fallback(string(), "")),
  stale_after_seconds: optional(fallback(finiteNumber, 0)),
  providers: optional(array(unknown())),
  accounts: optional(array(unknown())),
});

const cachedOverviewSchema = object({
  overview: optional(unknown()),
});

const historySchema = object({
  data: array(unknown()),
  has_more: optional(unknown()),
  next_cursor: optional(fallback(string(), "")),
});

const sampleSchema = object({
  sample_id: string(),
  provider: string(),
  collected_at: string(),
  plan: softString,
  windows: optional(unknown()),
});

const listSchema = object({
  data: array(unknown()),
});

const deviceSchema = object({
  id: string(),
  name: string(),
  created_at: string(),
  revoked_at: softString,
});

const settingsSchema = object({
  stale_after_seconds: finiteNumber,
  alert_threshold_percent: finiteNumber,
  timezone: string(),
  quiet_hours: optional(unknown()),
});

const quietSchema = object({
  start: optional(fallback(string(), "")),
  end: optional(fallback(string(), "")),
});

const subscriptionSchema = object({
  id: string(),
  endpoint: string(),
  created_at: string(),
  status: string(),
  expired_at: softString,
  last_error: softString,
});

const deliverySchema = object({
  id: string(),
  kind: string(),
  provider: string(),
  title: string(),
  body: string(),
  status: string(),
  created_at: string(),
  detail: softString,
});

export function parseOverview(value: unknown): Parsed<Overview> {
  if (!isRecord(value)) {
    return fail("A visão geral chegou ilegível.");
  }
  const parsed = safeParse(overviewSchema, value);
  if (!parsed.success) {
    return fail("A visão geral não tem provedores.");
  }
  const generatedAt = parsed.output.generated_at ?? "";
  const staleAfterSeconds = parsed.output.stale_after_seconds ?? 0;
  if (parsed.output.accounts !== undefined) {
    const accounts: AccountView[] = [];
    for (const item of parsed.output.accounts) {
      const account = parseAccount(item);
      if (account !== undefined) {
        accounts.push(account);
      }
    }
    return {
      ok: true,
      value: {
        generatedAt,
        staleAfterSeconds,
        providers: [],
        accounts,
      },
    };
  }
  if (parsed.output.providers === undefined) {
    return fail("A visão geral não tem provedores.");
  }
  return {
    ok: true,
    value: {
      generatedAt,
      staleAfterSeconds,
      providers: normalizeCards(parsed.output.providers),
    },
  };
}

function parseAccount(value: unknown): AccountView | undefined {
  const parsed = safeParse(accountSchema, value);
  if (!parsed.success) {
    return undefined;
  }
  const label = parsed.output.label.trim();
  if (label === "") {
    return undefined;
  }
  return {
    id: parsed.output.id,
    label,
    login: parsed.output.login ?? "",
    providers: normalizeCards(parsed.output.providers ?? []),
  };
}

function normalizeCards(items: unknown[]): ProviderCard[] {
  const found = new Map<"claude" | "codex", ProviderCard>();
  for (const item of items) {
    const card = parseCard(item);
    if (card !== undefined) {
      found.set(card.provider, card);
    }
  }
  const claude = found.get("claude") ?? emptyCard("claude");
  const codex = found.get("codex") ?? emptyCard("codex");
  return [claude, codex];
}

export function parseCachedOverview(value: unknown): Parsed<Overview> {
  const parsed = safeParse(cachedOverviewSchema, value);
  if (!parsed.success) {
    return fail("A cópia local está ilegível.");
  }
  return parseOverview(parsed.output.overview);
}

export function parseHistory(value: unknown): Parsed<HistoryPage> {
  const parsed = safeParse(historySchema, value);
  if (!parsed.success) {
    return fail("O histórico chegou ilegível.");
  }
  const samples: HistorySample[] = [];
  for (const item of parsed.output.data) {
    const sample = parseSample(item);
    if (sample !== undefined) {
      samples.push(sample);
    }
  }
  return {
    ok: true,
    value: {
      samples,
      hasMore: parsed.output.has_more === true,
      nextCursor: parsed.output.next_cursor ?? "",
    },
  };
}

export function parseDevices(value: unknown): Parsed<DeviceView[]> {
  const parsed = safeParse(listSchema, value);
  if (!parsed.success) {
    return fail("A lista de dispositivos chegou ilegível.");
  }
  const devices: DeviceView[] = [];
  for (const item of parsed.output.data) {
    const device = safeParse(deviceSchema, item);
    if (!device.success) {
      continue;
    }
    const view: DeviceView = {
      id: device.output.id,
      name: device.output.name,
      createdAt: device.output.created_at,
    };
    if (device.output.revoked_at !== undefined) {
      view.revokedAt = device.output.revoked_at;
    }
    devices.push(view);
  }
  return { ok: true, value: devices };
}

export function parseSettings(value: unknown): Parsed<SettingsView> {
  if (!isRecord(value)) {
    return fail("As preferências chegaram ilegíveis.");
  }
  const parsed = safeParse(settingsSchema, value);
  if (!parsed.success) {
    return fail("As preferências chegaram incompletas.");
  }
  const settings: SettingsView = {
    staleAfterSeconds: parsed.output.stale_after_seconds,
    alertThresholdPercent: parsed.output.alert_threshold_percent,
    timezone: parsed.output.timezone,
    quietStart: "",
    quietEnd: "",
  };
  if (isRecord(parsed.output.quiet_hours)) {
    const quiet = safeParse(quietSchema, parsed.output.quiet_hours);
    if (quiet.success) {
      settings.quietStart = quiet.output.start ?? "";
      settings.quietEnd = quiet.output.end ?? "";
    }
  }
  return { ok: true, value: settings };
}

export function parseSubscriptions(value: unknown): Parsed<SubscriptionView[]> {
  const parsed = safeParse(listSchema, value);
  if (!parsed.success) {
    return fail("As inscrições chegaram ilegíveis.");
  }
  const items: SubscriptionView[] = [];
  for (const item of parsed.output.data) {
    const subscription = safeParse(subscriptionSchema, item);
    if (!subscription.success) {
      continue;
    }
    const view: SubscriptionView = {
      id: subscription.output.id,
      endpoint: subscription.output.endpoint,
      createdAt: subscription.output.created_at,
      status: subscription.output.status,
    };
    if (subscription.output.expired_at !== undefined) {
      view.expiredAt = subscription.output.expired_at;
    }
    if (subscription.output.last_error !== undefined) {
      view.lastError = subscription.output.last_error;
    }
    items.push(view);
  }
  return { ok: true, value: items };
}

export function parseDeliveries(value: unknown): Parsed<DeliveryView[]> {
  const parsed = safeParse(listSchema, value);
  if (!parsed.success) {
    return fail("Os avisos chegaram ilegíveis.");
  }
  const items: DeliveryView[] = [];
  for (const item of parsed.output.data) {
    const delivery = safeParse(deliverySchema, item);
    if (!delivery.success) {
      continue;
    }
    const view: DeliveryView = {
      id: delivery.output.id,
      kind: delivery.output.kind,
      provider: delivery.output.provider,
      title: delivery.output.title,
      body: delivery.output.body,
      status: delivery.output.status,
      createdAt: delivery.output.created_at,
    };
    if (delivery.output.detail !== undefined) {
      view.detail = delivery.output.detail;
    }
    items.push(view);
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
  const parsed = safeParse(cardSchema, value);
  if (!parsed.success) {
    return undefined;
  }
  const item = parsed.output;
  const card: ProviderCard = {
    provider: item.provider,
    freshness: item.freshness ?? "empty",
    windows: [],
  };
  if (item.reason_code !== undefined) {
    card.reasonCode = item.reason_code;
  }
  if (item.observed_at !== undefined) {
    card.observedAt = item.observed_at;
  }
  if (isRecord(item.last_sample)) {
    const sample = safeParse(lastSampleSchema, item.last_sample);
    if (sample.success) {
      if (sample.output.plan !== undefined) {
        card.plan = sample.output.plan;
      }
      card.windows = parseWindows(sample.output.windows);
    }
  }
  const projection = parseProjection(item.projection);
  if (projection !== undefined) {
    card.projection = projection;
  }
  return card;
}

function parseSample(value: unknown): HistorySample | undefined {
  const parsed = safeParse(sampleSchema, value);
  if (!parsed.success) {
    return undefined;
  }
  const sample: HistorySample = {
    sampleId: parsed.output.sample_id,
    provider: parsed.output.provider,
    collectedAt: parsed.output.collected_at,
    windows: parseWindows(parsed.output.windows),
  };
  if (parsed.output.plan !== undefined) {
    sample.plan = parsed.output.plan;
  }
  return sample;
}

function parseWindows(value: unknown): WindowView[] {
  const parsed = safeParse(array(unknown()), value);
  if (!parsed.success) {
    return [];
  }
  const windows: WindowView[] = [];
  for (const item of parsed.output) {
    const windowItem = safeParse(windowSchema, item);
    if (!windowItem.success) {
      continue;
    }
    windows.push({
      name: windowItem.output.name,
      used: windowItem.output.used,
      resetsAt: windowItem.output.resets_at,
    });
  }
  return windows;
}

function parseProjection(value: unknown): ProjectionView | undefined {
  const parsed = safeParse(projectionSchema, value);
  if (!parsed.success) {
    return undefined;
  }
  return {
    window: parsed.output.window,
    estimatedUsed: parsed.output.estimated_used,
    resetsAt: parsed.output.resets_at,
    dataQuality: parsed.output.data_quality,
  };
}
