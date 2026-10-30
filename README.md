# Rastreador de uso de IA

O agente lê, na máquina do usuário, o cache ou o status line do Claude e os `rate_limits` dos rollouts do Codex. O servidor guarda só percentuais, plano e horários de reset. O PWA mostra visão, histórico, dispositivos e alertas.

Credenciais de IA, prompts, conversas e caminhos não são enviados. E-mail BYOK, Ciphera, Sabiá e cobrança continuam fora.

O MVP pessoal foi validado em 2026-10-30. O programa que roda hoje ainda tem um operador só. A etapa de contas está decidida no ADR 0005. O login configurável, Gitea, GitHub, Google e Apple, está no ADR 0006. Os cofres, o próprio e os externos, estão no ADR 0007. Nenhum deles está implementado.

## Executar localmente

```sh
cd web && pnpm install && pnpm build
go run ./cmd/tracker --addr 127.0.0.1:8787 --data data --web web/dist
```

Abra `http://127.0.0.1:8787` e crie a instalação. O token do operador aparece uma vez. Fora de loopback, defina `TRACKER_SETUP_TOKEN`; se a variável existir, o setup exige o cabeçalho `X-Setup-Token` também em loopback.

No PWA, gere um código de enrolamento e rode o agente com um arquivo que você escolheu. Sem `--claude-file` ou `--codex-dir`, o agente não lê `~/.claude` nem `~/.codex`.

```sh
go run ./cmd/agent enroll --api http://127.0.0.1:8787 --code CODIGO --name notebook --out device.token
go run ./cmd/agent run --api http://127.0.0.1:8787 --token-file device.token --spool spool.json --claude-file CAMINHO --codex-dir DIRETORIO
```

`data/`, `device.token`, `spool.json` e a chave VAPID ficam fora do Git. Não commite credenciais nem capturas pessoais.

## Verificação

O quality gate está em `TESTING_STANDARD.md`. A jornada sintética e os limites do que não foi provado estão em `docs/evidence/slice-1.md`. O plano completo continua em `DEVELOPMENT_PLAN.md`.
