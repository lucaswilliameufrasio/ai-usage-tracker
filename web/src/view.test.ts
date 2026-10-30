import assert from "node:assert/strict";
import { describe, it } from "node:test";

import {
  deliveryLabel,
  parseOverview,
  parseRoute,
  projectionCopy,
  readingCopy,
  staffTone,
} from "./view.ts";

describe("rotas", () => {
  it("reconhece os quatro destinos", () => {
    assert.equal(parseRoute(""), "overview");
    assert.equal(parseRoute("#/historico"), "history");
    assert.equal(parseRoute("#/dispositivos"), "devices");
    assert.equal(parseRoute("#/alertas"), "alerts");
  });
});

describe("frescor", () => {
  it("separa atual, desatualizado, indisponível e vazio", () => {
    const current = readingCopy("current", "2026-10-29T15:00:00Z", undefined, false);
    assert.equal(current.headline, "Atual");
    assert.equal(current.showAsCurrent, true);
    assert.equal(current.detail.includes("29/10/2026 15:00 UTC"), true);

    const stale = readingCopy("stale", "2026-10-29T12:00:00Z", undefined, false);
    assert.equal(stale.headline, "Desatualizado");
    assert.equal(stale.showAsCurrent, false);
    assert.equal(stale.detail.includes("Coletado em 29/10/2026 12:00 UTC"), true);

    const unavailable = readingCopy("unavailable", undefined, "LIMITS_ABSENT", false);
    assert.equal(unavailable.headline, "Provedor indisponível");
    assert.equal(unavailable.detail.includes("não trouxe limites"), true);

    const empty = readingCopy("empty", undefined, undefined, false);
    assert.equal(empty.headline, "Sem dados");
  });

  it("não trata cópia local como leitura atual", () => {
    const cached = readingCopy("current", "2026-10-29T15:00:00Z", undefined, true);
    assert.equal(cached.headline, "Dados em cache");
    assert.equal(cached.showAsCurrent, false);
    assert.equal(cached.banner, "Estes dados estão em cache e não são a leitura atual.");
    assert.equal(cached.detail.includes("não são a leitura atual"), true);
  });
});

describe("visão geral", () => {
  it("mantém os dois provedores e ignora campo proibido", () => {
    const parsed = parseOverview({
      generated_at: "2026-10-29T15:00:00Z",
      stale_after_seconds: 1800,
      providers: [
        {
          provider: "claude",
          freshness: "current",
          observed_at: "2026-10-29T15:00:00Z",
          prompt: "CANARY_PROMPT",
          last_sample: {
            sample_id: "smp_claude",
            provider: "claude",
            plan: "pro",
            windows: [
              { name: "five_hour", used: 86, limit: 100, unit: "percent", resets_at: "2026-10-29T18:00:00Z", path: "/home/canary/secret-project" },
            ],
          },
          projection: {
            window: "five_hour",
            estimated_used: 90,
            resets_at: "2026-10-29T18:00:00Z",
            data_quality: "current",
          },
        },
      ],
    });
    assert.equal(parsed.ok, true);
    if (!parsed.ok) {
      return;
    }
    assert.equal(parsed.value.providers.length, 2);
    assert.equal(parsed.value.providers[1]?.freshness, "empty");
    const claude = parsed.value.providers[0];
    assert.equal(claude?.plan, "pro");
    assert.equal(claude?.windows[0]?.used, 86);
    const encoded = JSON.stringify(parsed.value);
    assert.equal(encoded.includes("CANARY_PROMPT"), false);
    assert.equal(encoded.includes("secret-project"), false);
    const projection = claude?.projection;
    if (projection === undefined) {
      assert.fail("projeção ausente");
    }
    assert.equal(projectionCopy(projection).includes("Não é uma leitura do provedor."), true);
  });
});

describe("avisos", () => {
  it("nomeia os estados explícitos do adapter", () => {
    assert.equal(deliveryLabel("suppressed"), "Suprimido no horário silencioso");
    assert.equal(deliveryLabel("unconfigured"), "Push não configurado");
    assert.equal(deliveryLabel("subscription_expired"), "Inscrição expirada");
    assert.equal(deliveryLabel("no_subscription"), "Sem inscrição de push");
  });

  it("muda o tom da régua no limiar", () => {
    assert.equal(staffTone(79), "calm");
    assert.equal(staffTone(80), "watch");
    assert.equal(staffTone(95), "flood");
  });
});
