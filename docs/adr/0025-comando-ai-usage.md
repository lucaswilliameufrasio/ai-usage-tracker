# ADR 0025 — Comando instalado `ai-usage`

- Status: aceito
- Data: 2026-10-31
- Relacionado: ADR 0018, ADR 0020

## Contexto

A v0.1.0 instalava o coletor em `~/.local/bin/agent`. Esse nome já era o do Grok Build nesta máquina: `~/.local/bin/grok` e `~/.local/bin/agent` apontavam para `~/.grok/bin`. O instalador perguntou e a pessoa confirmou a troca. O binário do Grok em `~/.grok/bin/agent` continuou intacto; o elo em `~/.local/bin/agent` é que foi coberto.

## Decisão

- O comando instalado é `ai-usage`, em `~/.local/bin/ai-usage`, a menos que `--bin-dir` diga outro diretório.
- O pacote se chama `ai-usage_<versão>_<os>_<arch>.tar.gz` e contém o arquivo `ai-usage`.
- O instalador não cria nem substitui um arquivo chamado `agent`.
- A atualização opt-in continua gravando por cima do executável que está em execução (ADR 0020). O diretório do código segue `cmd/agent`. `go run ./cmd/agent` não muda.
- O texto de uso usa o nome do executável, então o binário instalado mostra `ai-usage`.

## Alternativas rejeitadas

- Manter `agent` e só documentar o conflito.
- Instalar como `tracker` ou `usage`: curto demais e fácil de cobrir outro comando.
- Renomear o pacote Go `cmd/agent`: o conflito é o nome no `PATH`, não o diretório do código.

## Consequências

A v0.1.0 continua publicada e ainda grava `agent`. A tag seguinte, `v0.2.0`, é a que o `releases/latest` deve servir. Quem já instalou a v0.1.0 troca o arquivo manualmente ou reinstala pela tag nova. O comando `agent` desta máquina volta a ser o elo do Grok.

## Quando revisitar

Se o comando passar a ser distribuído por um gerenciador de pacotes com outro nome.
