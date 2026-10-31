# ADR 0028 — Envio pelo Sabiá

- Status: aceito
- Data: 2026-10-31
- Relacionado: ADR 0019, ADR 0024

## Contexto

O ADR 0019 publicou o template `usage-alert-v1` e deixou o envio para quando o alerta saísse por `POST /v1/messages`. Esse corte foi uma decisão deste repositório, não um limite do Sabiá. O gateway já aceita a mensagem com a API key da aplicação. Essa chave é outra credencial, distinta da chave de deploy. O e-mail da instalação continua saindo pelo provedor escolhido em Acesso. O produto público continua sem depender do Sabiá.

## Decisão

- A chave de envio fica no cofre, no nome `sabia.send`. Não é a chave `sabia.deploy`. O GET não devolve nenhuma das duas. Omitir a chave na gravação mantém a que já está.
- O destinatário fica em `sabia_link` e aparece na tela. Sem origem, destinatário ou chave de envio, nenhuma linha é criada e o Sabiá não é chamado. Web Push e o e-mail da instalação seguem iguais.
- O alerta chama `POST {origem}/v1/messages` com `Authorization: Bearer`, `Idempotency-Key` e o corpo `channel`, `to`, `template`, `locale` e `data`. O template é a chave `usage.alert`, o locale é `pt-BR`, e `data` traz `provider`, `window`, `used` e `resets_at`. O campo `from` não vai: o remetente é o do canal no Sabiá.
- HTTP 202 grava `queued` aqui. Isso é aceite na fila de lá. `sent` ou `delivered` no corpo não viram entrega na caixa. Estado `failed` nesse 202 grava falha e não tenta de novo.
- 429, 503, 5xx e rede usam a mesma chave e os mesmos limites da fila de e-mail. 401, 403 e os outros 4xx são finais. A troca da chave, da origem ou do destinatário marca o pendente e não reenvia com a credencial nova.
- O teste fala com um servidor local. Não chama o Sabiá real.

## Alternativas rejeitadas

- Usar a chave de deploy para enviar.
- Tratar o 202 como mensagem entregue.
- Fazer o e-mail da instalação cair no Sabiá quando o provedor próprio falha.
- Consultar um estado final que o Sabiá ainda não publica para o aplicativo.

## Consequências

O esquema passa para `user_version` 7. O processo que já está no ar só migra no próximo arranque. Quem não configura o envio não percebe o Sabiá. A tela diz que o 202 não confirma a caixa de entrada.

## Quando revisitar

Quando o Sabiá publicar uma consulta autenticada do estado da entrega, ou quando houver outro canal além do e-mail.
