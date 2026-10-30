# ADR 0008 — Sessão da pessoa e URL de retorno

- Status: aceito
- Data: 2026-10-30

## Contexto

O ADR 0005 separa o administrador das outras pessoas. O ADR 0006 define os quatro logins, a identidade por provedor e `subject`, e que uma tentativa já iniciada permanece na revisão com que começou. A configuração e o cofre próprio já existem. Falta o retorno do provedor e a sessão de quem não é administrador.

A instalação que está em validação responde por HTTP no Tailscale. Um cookie `Secure` obrigatório não voltaria nesse endereço.

## Decisão

- A URL de retorno é a origem do pedido mais `/v1/login/{provider}/callback`, com `provider` em `github`, `gitea`, `google` ou `apple`. O servidor monta essa URL. O navegador não envia outra.
- O fluxo é authorization code com PKCE S256. O estado e o nonce duram dez minutos e servem uma vez.
- No início, o processo copia o segredo do cofre para uma referência só daquela tentativa. A troca do código usa essa cópia, não o valor publicado depois. A cópia sai do cofre ao fim da tentativa. O SQLite guarda a referência, não o segredo, nem o access token do provedor.
- GitHub e Gitea identificam pelo id numérico da API. Google e Apple identificam pelo `sub` do `id_token`, com emissor, audiência e nonce conferidos. E-mail, se vier, é contato. Não funde contas.
- O client secret enviado à Apple é um JWT ES256 cunhado na hora, com validade de cinco minutos, a partir do `.p8` da tentativa.
- A sessão da pessoa é um cookie `tracker_session`, `HttpOnly`, `SameSite=Lax`, por 14 dias. `Secure` entra quando o pedido já é HTTPS. O token do operador continua sendo o do administrador e não é substituído por esse cookie.
- A pessoa vê e revoga só os dispositivos da própria conta, e gera o código de pareamento só para o agente dela. O administrador continua vendo a instalação inteira. A sessão da pessoa não abre Acesso nem as rotas de administração. O token de dispositivo não abre a sessão.
- Testes usam um provedor falso e segredo sintético. Não há chamada real a GitHub, Gitea, Google ou Apple.

## Alternativas rejeitadas

- Outro token copiado no navegador para a pessoa: repete o modelo que o ADR 0005 encerrou para o uso diário.
- Cookie `Secure` em todo caso: a validação atual em HTTP no Tailscale ficaria sem sessão.
- Gravar o client secret da tentativa no SQLite: o banco passaria a conter o segredo que o ADR 0006 tirou de lá.
- Usar o segredo publicado no momento do retorno: uma rotação no meio do login mudaria a tentativa já iniciada.
- Mostrar o uso de toda a instalação a quem acabou de entrar: mistura as pessoas antes de existir workspace.

## Consequências

Cada aplicativo OAuth precisa dessa URL de retorno. O administrador continua em Acesso. Cobrança, termos e workspaces ficam para depois. O agente segue com o próprio token.

## Quando revisitar

Sessão renovável, mais de um administrador, ou a instalação passar a exigir HTTPS.
