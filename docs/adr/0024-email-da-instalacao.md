# ADR 0024 — E-mail da instalação

- Status: aceito
- Data: 2026-10-31
- Atualização 2026-10-31: o envio pelo Sabiá é outro caminho, no ADR 0028. Este canal de e-mail continua sem usar o Sabiá.

## Contexto

O ADR 0013 deixou o e-mail de fora até o escopo fechar. O escopo fechado é o da instalação: um administrador, um remetente e um destinatário. Não há e-mail por pessoa. O Sabiá continua só no template; ele não envia. A fase 5 não extrai um serviço de notificação.

Os contratos de envio usados aqui são os publicados pelos provedores: Resend e MillionSend em `POST /emails` com Bearer; Cloudflare Email Service em `POST /accounts/{account_id}/email/sending/send` com Bearer; Amazon SES API v2 em `POST /v2/email/outbound-emails` com SigV4, serviço `ses`. O SMTP da Cloudflare não entra.

## Decisão

- Um canal por instalação. O provedor é `resend`, `millionsend`, `cloudflare` ou `ses`. Não há fallback para outro provedor.
- O remetente e o destinatário ficam visíveis na tela Acesso. A chave fica só no cofre próprio, no nome `email.send`. O GET não devolve a chave. Omitir a chave na gravação mantém a que já está no cofre. Ativar sem chave nenhuma responde erro. A gravação não chama o provedor.
- A fila é a tabela `email_deliveries`, separada da de push. Os números são os mesmos: 32 pendentes, 8 por passagem, espera de 1, 2, 4 e 8 minutos, quinta falha `SEND_FAILED`, 24 horas `DELIVERY_EXPIRED`. 429, 5xx e rede tentam de novo. Os outros 4xx são finais. HTTP 2xx da Cloudflare com `success: false` também é final. `queued` conta como aceite.
- O aviso só entra na fila quando o canal está ativo. Horário silencioso grava `suppressed`, como o push. Canal inativo não cria uma linha por amostra.
- Trocar o provedor ou a chave marca o pendente como `EMAIL_REPLACED`. Desativar marca `EMAIL_INACTIVE`. Apagar o canal apaga a chave e marca `EMAIL_REMOVED`. Na hora do envio, cofre ausente vira `EMAIL_UNCONFIGURED`. Nada disso reenvia pela credencial nova.
- A passagem de envio manda o push e, em seguida, o e-mail. O e-mail sai mesmo quando o push não está configurado.

## Alternativas rejeitadas

- Uma tabela só com o push: o id da inscrição não existe no e-mail e as filas passariam a se bloquear.
- Chamar o provedor ao gravar: a ativação passaria a depender da rede e de uma conta real.
- Mandar pelo Sabiá: o produto público não depende desse gateway, e um 202 de lá não é entrega.
- SDK da AWS: a chamada é uma. A assinatura SigV4 fica neste processo.

## Consequências

O esquema passa para `user_version` 6. O processo que já está no ar só migra no próximo arranque. O snapshot seguinte exige essa versão. A chave não entra no backup do SQLite.

## Quando revisitar

Se a instalação passar a ter um destinatário por pessoa, ou se o envio pelo Sabiá deixar de ser só aceite na fila de lá.
