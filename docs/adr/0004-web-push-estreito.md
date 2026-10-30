# ADR 0004 — Web Push como adapter estreito

- Status: aceito
- Data: 2026-10-29

## Contexto

O plano coloca Web Push no MVP pessoal e deixa e-mail BYOK para a fase seguinte. O produto não pode depender do Sabiá nem criar um broker de notificações.

## Decisão

- O rastreador decide o alerta: limiar de percentual por janela e aviso de reset dentro de 60 minutos. Horário silencioso suprime o envio e grava esse estado.
- A entrega implementa a interface `Sender` com um único adapter Web Push (VAPID, inscrição do navegador, TTL curto). Não há tópico, ntfy, e-mail nem fallback entre canais.
- Chaves VAPID são geradas no diretório de dados na primeira execução, com arquivo fora do Git. A API só devolve a chave pública. Sem chave utilizável, o push fica `unconfigured` e a ingestão continua.
- Cada alerta tem chave de idempotência por provedor, janela e instante de reset. Falha ou expiração da inscrição fica visível; inscrição com resposta 404/410 é marcada expirada para o usuário reinscrever.
- O corpo do push leva título, provedor, janela e percentual. Não leva amostra bruta, prompt nem caminho.
- E-mail BYOK, adapter Sabiá e Ciphera não são implementados nesta fatia.

## Alternativas rejeitadas

- Serviço de notificação separado ou API de tópicos: o plano veda gateway genérico.
- Tratar ausência de VAPID como sucesso silencioso: o PWA precisa mostrar que o canal não está ativo.
- Enviar e-mail “já que o push pode falhar no iOS”: e-mail permanece na fase prevista.

## Consequências

No iOS o push continua limitado ao PWA instalado e pode atrasar; a interface não chama isso de alerta crítico. Provedor de e-mail não é configurável ainda.

## Quando revisitar

Fase 2, quando os adapters BYOK entrarem com o mesmo princípio: falha explícita e sem troca silenciosa de canal.
