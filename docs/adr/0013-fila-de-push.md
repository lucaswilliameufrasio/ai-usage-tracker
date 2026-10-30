# ADR 0013 — Fila de push

- Status: aceito
- Data: 2026-10-30

## Contexto

O ADR 0004 grava cada aviso e marca a inscrição que responde 404 ou 410. O envio ainda trata a primeira falha de rede como definitiva. A fase 2 pede fila limitada, nova tentativa, idempotência, expiração e saída da inscrição morta. E-mail BYOK continua sem escopo.

## Decisão

- A fila é a linha `pending` em `push_deliveries`. Ela sobrevive ao processo. Não há fila só em memória.
- No máximo 32 avisos ficam pendentes. O excedente é gravado como `failed` com `QUEUE_FULL`.
- Cada passagem envia no máximo 8 avisos vencidos. O relógio do processo dispara uma passagem por minuto, e a ingestão também dispara.
- Uma falha transitória mantém `pending`, sobe a tentativa e espera 1, 2, 4 e 8 minutos. Na quinta falha o aviso fica `failed` com `SEND_FAILED`.
- 404 e 410 expiram a inscrição e os avisos pendentes dela. A linha da inscrição permanece para a tela mostrar que expirou. Resposta 4xx definitiva não entra de novo. Rede, 429 e 5xx entram.
- Aviso pendente com mais de 24 horas fica `failed` com `DELIVERY_EXPIRED` e não é enviado.
- A chave de idempotência continua única. O estado `sent` só é gravado depois que o envio retorna sucesso. Se o processo cair no meio, o aviso pode sair outra vez.
- E-mail, Sabiá e troca de canal ficam de fora.

## Alternativas rejeitadas

- Apagar a inscrição morta: a tela perderia o motivo para reinscrever.
- Fila sem teto: uma queda do push cresceria sem limite dentro do processo.
- Tratar erro de rede como falha final: um instante sem rede perderia o aviso.

## Consequências

O esquema passa para `user_version` 4. O snapshot seguinte exige essa versão. O processo em execução só migra no próximo arranque.

## Quando revisitar

Quando o escopo do e-mail BYOK fechar e o mesmo contrato de fila puder servir a esse canal.
