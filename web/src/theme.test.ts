import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import path from "node:path";

import { readThemeChoice, resolveTheme, themeColor, themeStorageKey } from "./theme.ts";

function memoryStorage(initial: Record<string, string> = {}): Storage {
  const data = new Map(Object.entries(initial));
  return {
    get length() {
      return data.size;
    },
    clear() {
      data.clear();
    },
    getItem(key: string) {
      return data.get(key) ?? null;
    },
    key(index: number) {
      return [...data.keys()][index] ?? null;
    },
    removeItem(key: string) {
      data.delete(key);
    },
    setItem(key: string, value: string) {
      data.set(key, value);
    },
  };
}

describe("tema", () => {
  it("assume o dispositivo quando não há escolha válida", () => {
    assert.equal(readThemeChoice(memoryStorage()), "system");
    assert.equal(readThemeChoice(memoryStorage({ [themeStorageKey]: "sepia" })), "system");
    assert.equal(readThemeChoice(memoryStorage({ [themeStorageKey]: "dark" })), "dark");
  });

  it("resolve um único valor", () => {
    assert.equal(resolveTheme("system", false), "light");
    assert.equal(resolveTheme("system", true), "dark");
    assert.equal(resolveTheme("light", true), "light");
    assert.equal(resolveTheme("dark", false), "dark");
    assert.equal(themeColor("light"), "#d7e0ea");
    assert.equal(themeColor("dark"), "#070a0e");
  });

  it("o script inicial usa a mesma chave e as mesmas cores", () => {
    const html = readFileSync(path.join(import.meta.dirname, "../index.html"), "utf8");
    assert.equal(html.includes(themeStorageKey), true);
    assert.equal(html.includes('"(prefers-color-scheme: dark)"'), true);
    assert.equal(html.includes(themeColor("light")), true);
    assert.equal(html.includes(themeColor("dark")), true);
    assert.equal(html.indexOf(themeStorageKey) < html.indexOf('src="/src/main.ts"'), true);
  });
});
