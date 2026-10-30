# Evidência — fatia 1

Data: 2026-10-29. Nenhuma credencial real foi lida, enviada ou gravada. Não houve deploy, commit nem push.

## O que a fatia entrega

Uma instalação single-user percorre coleta local, ingestão, SQLite e PWA. O operador cria a instalação, emite um código de uso único e revoga o dispositivo. O agente só lê os caminhos recebidos em `--claude-file` e `--codex-dir`. Amostra repetida não duplica o histórico. Dispositivo revogado recebe `401 DEVICE_REVOKED` e a fila local não pede outro token. A visão separa sem dados, atual, desatualizado, provedor indisponível e cópia em cache.

## Critérios

| Critério | Estado | Prova |
| --- | --- | --- |
| Coleta sintética de Claude e Codex | verificado | `internal/collect` e a jornada da interface, com `fixtures/synthetic` |
| Payload sem prompt, token, caminho ou conta | verificado | `internal/journey` (`StoredPayloads` e log) e o script da interface |
| Persistência SQLite real | verificado | `internal/journey` abre arquivo temporário |
| Ingestão idempotente e histórico | verificado | segunda execução não aumenta a contagem; a interface abre `#/historico` |
| Código de enrolamento de uso único | verificado | `TestPersonalJourney` recusa reutilizar o código |
| Revogação congela a fila | verificado | teste Go e o binário do agente saindo com código 3 |
| Vazio, atual, desatualizado e indisponível | verificado | `internal/journey`, `internal/freshness` e `web/src/view.test.ts` |
| Cache offline não é leitura atual | verificado | o script corta a rede depois da visão e exige o rótulo "Dados em cache" |
| Web Push estreito, sem gateway | parcial | horário silencioso fica `suppressed` no teste Go; sem inscrição a interface mostra "Sem inscrição de push"; entrega real no serviço do navegador não foi feita |
| Sem e-mail, Ciphera, Sabiá, cobrança ou multiusuário | excluído | não há código desses escopos |
| Quality gate | verificado | comandos abaixo |
| Duas semanas no celular | não verificado | fora desta sessão |
| Leitura ao vivo com credencial do provedor | não verificado | proibido nesta sessão; a inspeção está em `docs/source-inspection.md` |

## Comandos

Quality gate, sem cache de teste do Go:

```text
go version go1.27.1 linux/amd64
node v24.21.0
pnpm 11.24.0
tsc 7.0.2
gofmt -l .          sem saída
go vet ./...        ok
go test -count=1 ./...
```

`go test -count=1 ./...` passou em `collect`, `contract`, `freshness`, `httpapi` e `journey`. Os outros pacotes não têm testes.

```text
cd web && pnpm install --frozen-lockfile
cd web && pnpm test     6 testes, 0 falhas
cd web && pnpm build    tsc e Vite 8.3.5 ok
```

Jornada da interface, depois do build:

```text
cd web && node --experimental-strip-types scripts/verify-journey.ts
firefox capturou a tela inicial
jornada da interface ok
```

O Firefox do sistema pintou a tela de entrada. Os cliques de setup, enrolamento, visão, histórico, alertas, revogação e cache offline rodaram pelo jsdom contra o servidor e o binário reais. Não há geckodriver, então essa sequência não foi clicada dentro do Firefox.

## Limites conhecidos

- Quem chega primeiro em `127.0.0.1` pode criar o operador. Se `TRACKER_SETUP_TOKEN` estiver definido, ele passa a ser obrigatório também em loopback.
- Perder o token do operador não tem recuperação pela API.
- Uma amostra que chega no horário silencioso não dispara push depois; só uma amostra nova replaneja o aviso.
- Linha de rollout maior que 2 MiB sem snapshot anterior vira fonte ilegível, sem caminho no relatório.
- O serviço de push do navegador, inclusive resposta 404/410, não foi chamado. A expiração de inscrição está no adaptador e não teve entrega real.
- O PWA não foi usado por duas semanas em um telefone. No iPhone, o aviso depende da instalação na tela de início e pode atrasar.
- As decisões de e-mail BYOK, Sabiá, Ciphera, retenção e abertura multiusuário continuam no plano.
