# ADR 0012 — Validação do PWA com Valibot

- Status: aceito
- Data: 2026-10-30

## Contexto

O PWA lia o JSON da API com checagens manuais. Um campo a mais, como segredo, prompt ou caminho, tinha de ser ignorado em cada função. A validação do servidor continua em Go.

## Decisão

- Os parsers de `web/src/view.ts` usam Valibot 1.5.0.
- Chave desconhecida sai do objeto. Segredo, prompt e caminho não entram na tela.
- Item ruim numa lista é pulado. O restante da lista permanece.
- Zod não entra. A API em Go não usa Valibot.

## Alternativas rejeitadas

- Continuar a validação manual: cada payload novo repetia o mesmo predicado.
- Zod: a escolha explícita foi Valibot.

## Consequências

`web/` ganha uma dependência de produção. As mensagens de erro continuam em português. Um campo opcional com tipo errado não derruba o objeto pai quando o contrato antigo o ignorava.

## Quando revisitar

Quando o contrato JSON do servidor mudar de forma que o schema deixe de representar a tela.
