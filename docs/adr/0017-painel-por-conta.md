# ADR 0017 — Painel do operador por conta

- Status: aceito
- Data: 2026-10-30

## Contexto

O administrador vai acompanhar mais de uma pessoa. Cada pessoa já tem conta, sessão e dispositivos próprios (ADR 0008). A visão da pessoa já filtra pelo `account_id`. A visão do operador ainda escolhia a amostra mais nova de cada provedor em toda a instalação, então a leitura de outra máquina substituía a primeira. O e-mail do login é contato e não funde contas. O identificador do provedor de login não pode ir para o painel.

OpenCode, Antigravity e Grok Build estão nesta máquina. A inspeção de nomes e da documentação local não achou um arquivo de percentual e reset no mesmo contrato de Claude e Codex. Eles não entram neste corte. O detalhe está em `docs/source-inspection.md`.

## Decisão

- `GET /v1/overview`, do operador, responde `accounts`. Cada grupo tem `id`, `label`, `login` e `providers`. Não há mais o arranjo `providers` na raiz.
- O rótulo é o contato do login, aparado. Sem contato, o rótulo é GitHub, Gitea, Google ou Apple. Sem um desses, é "Conta". `login` é o provedor do login. `subject` não sai.
- Conta sem dispositivo não aparece. Dispositivo com `account_id` vazio fica no grupo "Sem conta", por último, e não se mistura com outra pessoa.
- `GET /v1/account/overview` continua com `providers` na raiz e só os dispositivos daquela conta.
- A cópia offline da visão usa chaves separadas para operador e pessoa. Sair ou perder a sessão apaga as duas e a chave antiga.
- Histórico e lista de dispositivos do operador continuam da instalação inteira.

## Alternativas rejeitadas

- Manter um cartão por provedor com a amostra mais nova: a segunda pessoa apaga a primeira da tela.
- Mostrar o `subject` ou o e-mail da conta de IA: o primeiro é identificador do login; o segundo não entra no contrato da amostra.
- Esperar workspaces ou filtrar o administrador a uma pessoa só: o pedido é ver várias contas no mesmo painel.
- Tratar `account_id` vazio como se fosse uma conta já existente.

## Consequências

O PWA do operador desenha uma seção por conta. A pessoa continua na visão "Duas medições". Quem já tem o processo no ar só vê o painel novo depois de reiniciar o servidor. A resposta antiga, só com `providers`, ainda desenha o quadro único.

## Quando revisitar

Se o histórico ou a lista de dispositivos também precisarem de seção por conta, ou se aparecer um contrato local de limite e reset para outro provedor.
