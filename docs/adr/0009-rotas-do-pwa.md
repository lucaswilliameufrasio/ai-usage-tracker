# ADR 0009 — Rotas do PWA

- Status: aceito
- Data: 2026-10-30

## Contexto

O PWA tem cinco telas planas: Visão, Histórico, Dispositivos, Alertas e Acesso. Elas eram escolhidas pelo fragmento (`#/acesso`). O processo Go só entregava um arquivo que existisse em `web/dist`, então `/acesso` respondia 404 e o fragmento nunca precisava chegar ao servidor.

A barra de endereço não mostrava a tela. Recarregar `/acesso` também não tinha como funcionar. O ADR 0001 mantém o PWA em TypeScript, servido pelo mesmo processo, sem framework de interface.

## Decisão

- As telas canônicas são `/`, `/historico`, `/dispositivos`, `/alertas` e `/acesso`.
- Um GET que não é `/healthz` nem `/v1/`, e que não aponta para um arquivo existente, devolve `index.html` quando o caminho não tem extensão. Arquivo ausente com extensão continua 404.
- O cliente lê `location.pathname`. Um fragmento antigo `#/historico`, `#/dispositivos`, `#/alertas`, `#/acesso` ou `#/` ainda abre a tela e a barra passa ao caminho canônico. Os aliases em inglês (`/history`, `/devices`, `/alerts`, `/access`) também abrem e são substituídos pelo caminho em português.
- A navegação interna usa a History API. Abrir em outra aba ou recarregar pede o caminho ao processo Go.
- `/v1/login/{provider}/callback` continua sendo rota do servidor, como no ADR 0008.

## Alternativas rejeitadas

- Manter o fragmento: a URL não identifica a tela e um link copiado depende do hash.
- TanStack Router: o produto em uso é o adapter de React. As cinco telas não têm parâmetro, loader nem search param. Adotar o roteador seria adotar o framework.
- SvelteKit: o Kit quer ser o servidor da aplicação. Sessão, cookie, SQLite e o retorno do provedor ficam no processo Go, na mesma origem.

## Consequências

Recarregar `/acesso` exige o binário que devolve o shell. O bundle novo, servido pelo binário antigo, pede `/acesso` e recebe 404 até o processo reiniciar. O ADR 0001 permanece: não entra framework de interface.

## Quando revisitar

Quando uma tela precisar de parâmetro na URL, ou quando existir um segundo cliente web.
