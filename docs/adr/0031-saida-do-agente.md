# ADR 0031 — Saída do agente por item

- Status: aceito
- Data: 2026-11-01
- Relacionado: ADR 0003, ADR 0016

## Contexto

Depois de cada passagem o agente escrevia só contadores: `aceitas 2 duplicadas 1 rejeitadas 0 pendentes 0`. Quando algo era rejeitado ou ficava pendente, o log não dizia qual provedor nem qual tipo de item. Além disso as mensagens estavam fixas em português, sem nenhum mecanismo de idioma.

## Decisão

- `agent.Result` ganha `Outcomes`. Cada item enviado vira `Outcome{Kind, Provider, Status, Code}`. `Status` é `accepted`, `duplicate`, `rejected`, `pending` ou `blocked`. `Code` é o `error_code` da API numa rejeição.
- O provedor sai do corpo canônico do item. Nada novo é lido do disco nem enviado ao servidor.
- O comando escreve uma linha por item, `provedor tipo estado [code=...]`, e termina com `total accepted=N duplicate=N rejected=N pending=N`. Os campos são chaves fixas em inglês, fáceis de filtrar com `grep`.
- As mensagens da função `report` passam a inglês. Não há i18n: o comando escreve num único idioma. As demais mensagens do agente (setup, enrolamento, uso) continuam em português até haver decisão sobre elas.

## Alternativas rejeitadas

- Adicionar um mecanismo de idioma ao agente. Um binário pessoal com uma pessoa operando não justifica o custo.
- Imprimir só os itens com problema. Quem lê o log também quer ver o que deu certo.

## Consequências

Quem lia `aceitas`, `duplicadas`, `rejeitadas` e `pendentes` em script precisa trocar para `accepted`, `duplicate`, `rejected` e `pending`. Não há outro consumidor no repositório.

## Quando revisitar

Se o agente passar a ter mais de uma pessoa operando em idiomas diferentes, ou se as mensagens restantes forem migradas.
