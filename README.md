# Rastreador de uso de IA

O agente lê, na máquina do usuário, o cache ou o status line do Claude, os `rate_limits` dos rollouts do Codex e, se estiver ligado, as janelas do Antigravity pelo comando `agy`. O servidor guarda só percentuais, plano e horários de reset. O PWA mostra visão, histórico, dispositivos e alertas. Cada janela do cartão e do histórico tem a própria régua (ADR 0023). Quem administra vê uma seção por conta; cada pessoa vê só a própria (ADR 0017).

Credenciais de IA, prompts, conversas e caminhos não são enviados. O e-mail da instalação sai por Resend, MillionSend, Cloudflare Email ou Amazon SES, com a chave no cofre próprio (ADR 0024). Os cofres externos continuam sem código. O Sabiá não é dependência do produto público. Em Acesso, esta instalação edita um template e sincroniza com o Sabiá; a chave de deploy não volta para a tela (ADR 0019). Cobrança continua fora. O PWA mostra horários no fuso do dispositivo. O servidor guarda UTC (ADR 0021).

O MVP pessoal foi validado em 2026-10-30. Cada pessoa entra por Gitea, GitHub, Google ou Apple e vê os próprios dispositivos. O administrador configura esses logins em Acesso e continua com o token do operador. O segredo vai para o cofre próprio e não volta para a tela (ADR 0006, ADR 0007 e ADR 0008). A URL de retorno é a origem da instalação mais `/v1/login/github/callback`, e o mesmo caminho com `gitea`, `google` ou `apple`. Ciphera, HashiCorp Vault, AWS Secrets Manager e Infisical continuam sem adapter.

## Executar localmente

```sh
cd web && pnpm install && pnpm build
go run ./cmd/tracker --addr 127.0.0.1:8787 --data data --web web/dist
```

Abra `http://127.0.0.1:8787` e crie a instalação. O token do operador aparece uma vez. Fora de loopback, defina `TRACKER_SETUP_TOKEN`; se a variável existir, o setup exige o cabeçalho `X-Setup-Token` também em loopback. As telas ficam em `/`, `/historico`, `/dispositivos`, `/alertas` e `/acesso`. Um endereço antigo com `#/` abre a mesma tela (ADR 0009).

No PWA, gere um código de enrolamento. No terminal, `go run ./cmd/agent run` abre o setup na primeira vez: endereço, código, Claude, Codex, Antigravity e atualização automática. Enter deixa o Antigravity e a atualização desligados. Ele propõe `~/.claude.json` e `~/.codex/sessions` e só grava o que você confirmar (ADR 0014 e ADR 0020). A configuração fica em `~/.config/ai-usage-tracker/agent.json`. O próximo `run` sem flags fica em execução e repete a cada 5 minutos. Ctrl+C espera o envio atual; o que o servidor não confirmou continua na fila (ADR 0016). `ai-usage setup`, ou `go run ./cmd/agent setup`, refaz a escolha e, como o `run` com flags, envia uma vez e sai. Com o Codex configurado, o envio pergunta o limite ao comando local `codex app-server` e só usa a pasta de sessões se essa leitura falhar (ADR 0015). O agente não lê o token do Codex. Com o Antigravity ligado, cada passagem chama `agy -p /usage` e também não lê o token (ADR 0022). Para rodar em segundo plano, `ai-usage service install` registra o agente como serviço de usuário (systemd no Linux, launchd no macOS), `ai-usage service uninstall` remove só o serviço e `ai-usage service doctor` diagnostica e diz a correção (ADR 0032). `install --dry-run` mostra o arquivo e os comandos sem mudar nada. Windows não tem suporte. Para trocar o agente de servidor: gere um código em Dispositivos no servidor novo, rode `ai-usage setup`, troque o endereço em Servidor e cole o código. O token antigo só vale no servidor antigo, então ao mudar o endereço o setup pede o código de novo e substitui o token. A fila local não é apagada. Se o serviço em segundo plano estiver instalado, rode `ai-usage service install` depois para ele usar a configuração nova, e `ai-usage service doctor` para conferir. O `doctor` também pergunta ao servidor se ele aceita o token (`GET /v1/device`); servidores anteriores a esta rota respondem 404 e saem como aviso.

Os flags continuam o caminho de script. Nesse modo o agente não procura o diretório pessoal.

```sh
go run ./cmd/agent run
go run ./cmd/agent enroll --api http://127.0.0.1:8787 --code CODIGO --name notebook --out device.token
go run ./cmd/agent run --api http://127.0.0.1:8787 --token-file device.token --spool spool.json --claude-file CAMINHO --codex-dir DIRETORIO
```

## Instalar o binário

O script da release instala só o comando `ai-usage` em `~/.local/bin`. Ele não grava um comando chamado `agent`. O painel continua neste host. Este script não procura outra release. A atualização fica no agente, desligada por padrão; `go run` não se atualiza (ADR 0018, ADR 0020 e ADR 0025).

A primeira release foi a tag `v0.1.0` e instalava em `agent`. A `v0.2.0` muda o comando para `ai-usage`. Não houve release anterior à `v0.1.0`, e o contrato público ainda muda, então a versão fica em `0.y.z`.

```sh
curl -fsSL https://github.com/lucaswilliameufrasio/ai-usage-tracker/releases/latest/download/agent-installer.sh | sh
```

O script baixa o pacote e o `checksums.txt` da mesma release, confere o SHA256 e grava o binário. O checksum protege contra arquivo corrompido no download. Não protege contra uma release alterada por quem publica. Há pacote para Linux e macOS, em amd64 e arm64. Quem tem o código deste repositório continua com `go run ./cmd/agent`.

`data/`, `device.token`, `spool.json` e a chave VAPID ficam fora do Git. Não commite credenciais nem capturas pessoais.

O backup do SQLite é um snapshot consistente. Ele não inclui o cofre nem a chave. A restauração só grava num diretório que ainda não tem `tracker.db` (ADR 0010). A tela não baixa o banco. A recuperação do produto é o sqlite-replication (ADR 0011). O processo abre o banco em WAL. O ensaio do replicador fica no teste e usa o módulo local `../sqlite-replication/go`; ele não abre o `data/` em uso. O aviso de push entra numa fila de até 32 pendentes, com nova tentativa e prazo de 24 horas (ADR 0013). O e-mail da instalação usa os mesmos limites, numa fila própria (ADR 0024). O alerta configurado no Sabiá pede `POST /v1/messages` e um 202 fica aceito na fila de lá, sem confirmar a caixa (ADR 0028). O banco aberto por este código fica na user_version 7.

```sh
go run ./cmd/tracker backup --data data --out snapshot.db
go run ./cmd/tracker restore --snapshot snapshot.db --dest restaurado
```

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
