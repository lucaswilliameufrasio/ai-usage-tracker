export const themeStorageKey = "ai-usage-theme";

export type ThemeChoice = "system" | "light" | "dark";

export function readThemeChoice(storage: Storage): ThemeChoice {
  let raw: string | null = null;
  try {
    raw = storage.getItem(themeStorageKey);
  } catch {
    return "system";
  }
  if (raw === "light" || raw === "dark" || raw === "system") {
    return raw;
  }
  return "system";
}

export function resolveTheme(choice: ThemeChoice, systemIsDark: boolean): "light" | "dark" {
  if (choice === "dark") {
    return "dark";
  }
  if (choice === "light") {
    return "light";
  }
  if (systemIsDark) {
    return "dark";
  }
  return "light";
}

export function themeColor(resolved: "light" | "dark"): string {
  if (resolved === "dark") {
    return "#070a0e";
  }
  return "#d7e0ea";
}

export function prefersDark(view: Window): boolean {
  if (typeof view.matchMedia !== "function") {
    return false;
  }
  return view.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function applyTheme(doc: Document, choice: ThemeChoice, systemIsDark: boolean): void {
  const resolved = resolveTheme(choice, systemIsDark);
  const root = doc.documentElement;
  root.dataset.theme = resolved;
  root.dataset.themeChoice = choice;
  root.style.colorScheme = resolved;
  const meta = doc.querySelector("meta[name='theme-color']");
  if (meta !== null) {
    meta.setAttribute("content", themeColor(resolved));
  }
}

export function bindSystemTheme(view: Window, current: () => ThemeChoice): void {
  if (typeof view.matchMedia !== "function") {
    return;
  }
  const media = view.matchMedia("(prefers-color-scheme: dark)");
  const onChange = (event: Event) => {
    if (current() !== "system") {
      return;
    }
    applyTheme(view.document, "system", matched(event, media.matches));
  };
  media.addEventListener("change", onChange);
}

function matched(event: Event, fallback: boolean): boolean {
  const record: object = event;
  if (!("matches" in record)) {
    return fallback;
  }
  const value: unknown = record.matches;
  if (typeof value === "boolean") {
    return value;
  }
  return fallback;
}
