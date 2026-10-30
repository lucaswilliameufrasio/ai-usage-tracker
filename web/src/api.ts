export type ApiResult =
  | { ok: true; status: number; body: unknown }
  | { ok: false; status: number; message: string; errorCode: string };

export async function apiRequest(
  origin: string,
  path: string,
  token: string,
  method: string,
  body?: unknown,
): Promise<ApiResult> {
  const headers = new Headers();
  if (token !== "") {
    headers.set("Authorization", `Bearer ${token}`);
  }
  let payload: string | undefined;
  if (body !== undefined) {
    headers.set("Content-Type", "application/json");
    payload = JSON.stringify(body);
  }
  const init: RequestInit = { method, headers };
  if (payload !== undefined) {
    init.body = payload;
  }
  let response: Response;
  try {
    response = await fetch(new URL(path, origin), init);
  } catch {
    return {
      ok: false,
      status: 0,
      message: "Sem rede para o rastreador.",
      errorCode: "NETWORK",
    };
  }
  const raw = await response.text();
  let parsed: unknown = {};
  if (raw !== "") {
    try {
      parsed = parseJson(raw);
    } catch {
      return {
        ok: false,
        status: response.status,
        message: "Resposta ilegível.",
        errorCode: "MALFORMED_REQUEST",
      };
    }
  }
  if (!response.ok) {
    return {
      ok: false,
      status: response.status,
      message: messageOf(parsed),
      errorCode: codeOf(parsed),
    };
  }
  return { ok: true, status: response.status, body: parsed };
}

function messageOf(value: unknown): string {
  if (!isRecord(value)) {
    return "A operação falhou.";
  }
  const message = value.message;
  if (typeof message !== "string" || message === "") {
    return "A operação falhou.";
  }
  return message;
}

function codeOf(value: unknown): string {
  if (!isRecord(value)) {
    return "UNEXPECTED_ERROR";
  }
  const code = value.error_code;
  if (typeof code !== "string" || code === "") {
    return "UNEXPECTED_ERROR";
  }
  return code;
}

function parseJson(raw: string): unknown {
  return JSON.parse(raw);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
