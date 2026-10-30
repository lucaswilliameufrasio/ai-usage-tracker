# Rastreador de uso de IA

O agente lê, na máquina do usuário, o cache ou o status line do Claude e os `rate_limits` dos rollouts do Codex. O servidor guarda só percentuais, plano e horários de reset. O PWA mostra visão, histórico, dispositivos e alertas.

Credenciais de IA, prompts, conversas e caminhos não são enviados. E-mail BYOK, Ciphera, Sabiá e cobrança continuam fora.

O MVP pessoal foi validado em 2026-10-30. Cada pessoa entra por Gitea, GitHub, Google ou Apple e vê os próprios dispositivos. O administrador configura esses logins em Acesso e continua com o token do operador. O segredo vai para o cofre próprio e não volta para a tela (ADR 0006, ADR 0007 e ADR 0008). A URL de retorno é a origem da instalação mais `/v1/login/github/callback`, e o mesmo caminho com `gitea`, `google` ou `apple`. Ciphera, HashiCorp Vault, AWS Secrets Manager e Infisical continuam sem adapter.

## Executar localmente

```sh
cd web && pnpm install && pnpm build
go run ./cmd/tracker --addr 127.0.0.1:8787 --data data --web web/dist
```

Abra `http://127.0.0.1:8787` e crie a instalação. O token do operador aparece uma vez. Fora de loopback, defina `TRACKER_SETUP_TOKEN`; se a variável existir, o setup exige o cabeçalho `X-Setup-Token` também em loopback. As telas ficam em `/`, `/historico`, `/dispositivos`, `/alertas` e `/acesso`. Um endereço antigo com `#/` abre a mesma tela (ADR 0009).

No PWA, gere um código de enrolamento e rode o agente com um arquivo que você escolheu. Sem `--claude-file` ou `--codex-dir`, o agente não lê `~/.claude` nem `~/.codex`.

```sh
go run ./cmd/agent enroll --api http://127.0.0.1:8787 --code CODIGO --name notebook --out device.token
go run ./cmd/agent run --api http://127.0.0.1:8787 --token-file device.token --spool spool.json --claude-file CAMINHO --codex-dir DIRETORIO
```

`data/`, `device.token`, `spool.json` e a chave VAPID ficam fora do Git. Não commite credenciais nem capturas pessoais.

## Cofre e login

Sem `TRACKER_VAULT_KEY`, a coleta continua. Gravar um login exige o cofre aberto. A chave são 32 bytes em base64 e fica fora do diretório `data/vault`. Uma chave inválida impede o processo de subir. Se a variável estiver ausente, o servidor sobe e a área Acesso informa que o cofre não está aberto.

Estas variáveis só semeiam um provedor que ainda não tem linha. Uma gravação pela interface não é reescrita no próximo arranque. Um provedor pela metade, com só o id ou só o segredo, também impede a subida.

- `TRACKER_LOGIN_GITHUB_CLIENT_ID` e `TRACKER_LOGIN_GITHUB_CLIENT_SECRET`
- `TRACKER_LOGIN_GITEA_BASE_URL`, `TRACKER_LOGIN_GITEA_CLIENT_ID` e `TRACKER_LOGIN_GITEA_CLIENT_SECRET`
- `TRACKER_LOGIN_GOOGLE_CLIENT_ID` e `TRACKER_LOGIN_GOOGLE_CLIENT_SECRET`
- `TRACKER_LOGIN_APPLE_TEAM_ID`, `TRACKER_LOGIN_APPLE_KEY_ID`, `TRACKER_LOGIN_APPLE_SERVICES_ID` e `TRACKER_LOGIN_APPLE_PRIVATE_KEY`

A chave da Apple é o arquivo `.p8`, não um JWT. Não coloque credencial de IA, prompt ou caminho nessas variáveis.

A pessoa entra pelo provedor ativo. A sessão fica num cookie por 14 dias. `Secure` entra quando o endereço já é HTTPS. Sem a chave do cofre, a coleta continua e o login novo falha fechado.

## Verificação

O quality gate está em `TESTING_STANDARD.md`. A jornada sintética e os limites do que não foi provado estão em `docs/evidence/slice-1.md`. O plano completo continua em `DEVELOPMENT_PLAN.md`.
