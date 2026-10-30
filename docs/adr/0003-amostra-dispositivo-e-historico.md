# ADR 0003 — Amostra, dispositivo e histórico

- Status: aceito
- Data: 2026-10-29

## Contexto

A fase 1 pede API single-user, enrolamento e revogação, ingestão idempotente e histórico. O servidor não pode aceitar campos livres nem reconstruir uma leitura que o coletor não tenha.

## Decisão

- Há um operador por instalação. `POST /v1/setup` só funciona uma vez e só devolve o token do operador nesse momento. Se `TRACKER_SETUP_TOKEN` estiver definido, ele é exigido em qualquer bind, inclusive loopback. Sem esse token, o setup fica aberto apenas em loopback.
- O operador emite um código de uso único e curta validade. O agente troca esse código por um token de dispositivo, mostrado uma vez. O servidor guarda apenas o hash SHA-256.
- O token de dispositivo só ingere amostras e relatórios. O token do operador lê, configura, revoga e gerencia inscrição de push. Revogação grava `revoked_at`; o próximo envio recebe `401` e `DEVICE_REVOKED`.
- A amostra aceita somente `sample_id`, `provider` (`claude` ou `codex`), `collected_at`, `source_observed_at` opcional, `plan` opcional e `windows`. Cada janela tem `name`, `used`, `limit`, `unit` e `resets_at`. Nesta fatia `unit` é `percent` e `limit` é 100, porque as fontes locais expõem percentual, não um teto absoluto de tokens.
- Campo desconhecido ou nome proibido (prompt, caminho, token, conta, transcript) é `422`. O servidor persiste o JSON canônico validado, não o corpo cru.
- O mesmo `sample_id` com o mesmo conteúdo semântico é sucesso idempotente e não cria outra linha. Conteúdo diferente no mesmo id é `409 SAMPLE_CONFLICT`. A comparação ignora só uma variação de `collected_at`, para uma releitura do mesmo snapshot não duplicar histórico.
- Um relatório `unavailable` com o mesmo `report_id` (provedor + motivo) atualiza `observed_at` e não cria outra linha. A checagem mais recente continua visível sem acumular uma linha por varredura.
- Relatório de fonte (`unavailable` + `reason_code`) é outro recurso. O overview distingue `empty`, `unavailable`, `stale` e `current`. Projeção só existe com duas amostras da mesma janela e nunca é apresentada como leitura ao vivo; amostra atrasada marca a projeção como atrasada.
- A fila local do agente é limitada. Só some item confirmado pelo HTTP 2xx. Credencial revogada congela a fila e não pede outro token.
- Não há exclusão automática de histórico nesta fatia.

## Alternativas rejeitadas

- Um token compartilhado entre PWA e agente: revogar o telefone derrubaria a coleta, ou o inverso.
- Guardar o corpo original para “não perder dados”: preservaria justamente os campos que o contrato recusa.
- Apagar a fila quando o dispositivo é revogado: perderia a evidência local e poderia incentivar reenrolamento silencioso.

## Consequências

Quem perde o token do operador precisa de um procedimento de recuperação que esta fatia não automatiza: parar o servidor e remover o registro do operador no SQLite local é uma operação manual, fora da API. Percentual não é saldo de tokens.

## Quando revisitar

Abertura multiusuário, política de retenção/exclusão, ou uma fonte que publique uso e teto numa unidade que não seja percentual.
