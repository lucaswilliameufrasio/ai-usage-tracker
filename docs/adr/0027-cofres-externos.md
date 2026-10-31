# ADR 0027 — Cofres externos

- Status: aceito
- Data: 2026-10-31
- Relacionado: ADR 0007

## Contexto

O ADR 0007 nomeou Ciphera, HashiCorp Vault no KV, AWS Secrets Manager e Infisical, e deixou os clientes para uma fatia seguinte. O cofre próprio já estava no processo. A pessoa validou a etapa do comando `ai-usage` e pediu esses cofres em seguida. A frase do plano sobre validar credenciais descreve o contrato do adapter, não uma espera por outro aceite.

## Decisão

- Um processo abre só os backends cujo material de abertura está completo no ambiente. O ativo continua sendo o registro `secret_backend`. Não há segundo backend em uso e não há cache do valor.
- A troca pela API só acontece quando o destino está aberto neste processo e o cofre atual responde que os nomes de servidor estão vazios. Segredo já gravado não é copiado. Isso continua sendo migração explícita.
- O token, a chave de serviço, o client secret e a access key não entram no formulário nem no SQLite.
- Ciphera usa `GET /v1/projects/{id}/secrets/{environment}`, `POST .../secrets/batch` e `DELETE .../secrets/{environment}/{key}`, com `Authorization: Bearer`. Se só houver chave de conta de serviço, cada chamada troca essa chave em `POST /v1/auth/service-account/token`. A lista do ambiente é a leitura documentada; o token deve ser o mais estreito que o Ciphera permitir.
- HashiCorp usa KV v2 em `/v1/{mount}/data/{caminho}`, com `X-Vault-Token`. Transit não entra.
- AWS Secrets Manager usa `CreateSecret`, `PutSecretValue`, `GetSecretValue` e `DeleteSecret` com `ForceDeleteWithoutRecovery`. A assinatura é SigV4 com a região e as variáveis `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` e, se houver, `AWS_SESSION_TOKEN`. A cadeia não lê arquivo de perfil nem o IMDS.
- Infisical troca a identidade de máquina em `POST /api/v1/auth/universal-auth/login` a cada chamada e usa `GET`, `POST`, `PATCH` e `DELETE /api/v4/secrets/{nome}`. Resposta só com `approval` não conta como gravação.
- Erro remoto não devolve o corpo. `401` e `403` selam. O resto indisponível fecha a função. O teste fala com um falso local e um segredo sintético.

## Alternativas rejeitadas

- Pedir o token na tela de Acesso.
- Copiar os segredos automaticamente ao trocar o kind.
- Guardar o access token do Infisical ou do Ciphera entre chamadas.
- Tratar aprovação do Infisical como segredo gravado.

## Consequências

O processo que já está no ar não abre esses cofres até o próximo arranque, com o ambiente correspondente. A leitura do painel mostra o kind ativo e se uma consulta respondeu.

## Quando revisitar

Quando existir uma migração que copie os nomes de um cofre para o outro, ou uma política de cache publicada.
