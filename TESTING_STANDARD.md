# Padrão de testes

Não há workflow de CI neste repositório. O gate local abaixo é o contrato. Não há limiar de cobertura.

## Ambiente verificado

- Go 1.27.3, módulo `ai-usage-tracker`. A linha `go` em `go.mod` pede esse toolchain.
- Node.js 26.13.0, pedido em `mise.toml` e `.node-version`, e pnpm 12.16.1, pedido em `mise.toml`
- TypeScript 7.0.2 e Vite 8.3.5, em `web/`
- SQLite embutido por `modernc.org/sqlite` v1.61.0, com `modernc.org/libc` v1.79.0; os testes usam arquivo temporário
- Python 3 só para gerar os ícones em `web/scripts/make-icons.py`
- Firefox do sistema, se existir, apenas para a captura opcional da tela inicial

Não use credenciais reais, serviços externos de IA nem banco compartilhado. Os testes não leem `~/.claude` nem `~/.codex`.

## Gate

Na raiz:

```sh
gofmt -l .
go vet ./...
go test ./...
cd web && pnpm install --frozen-lockfile
cd web && pnpm test
cd web && pnpm build
```

`gofmt -l .` precisa terminar sem linhas. O `pnpm build` executa `tsc --noEmit` e o Vite. `make quality` repete essa sequência. O pnpm é o 12.16.1 do mise; rode o gate com o mise ativo neste diretório.

## Jornada da interface

Depois do build:

```sh
cd web && node --experimental-strip-types scripts/verify-journey.ts
```

O script sobe o `tracker` em porta local, com diretório temporário, e percorre setup, enrolamento, ingestão das fixtures sintéticas, visão, histórico, push sem inscrição, revogação, saída 3 do agente e rótulo de cache offline. Se o Firefox existir, grava uma captura da tela inicial em arquivo temporário. Não é o clique dentro do Firefox: o ambiente não tem WebDriver.

## Fora do gate

Não há suíte de integração com rede externa, teste de carga, limiar de cobertura nem verificação de duas semanas no celular. O teste em `internal/release` publica os arquivos num servidor local. Ele não chama o GitHub. `go build ./cmd/agent` não precisa do diretório `../sqlite-replication/go`.
