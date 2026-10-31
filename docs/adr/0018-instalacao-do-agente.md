# ADR 0018 — Instalação do agente pela release

- Status: aceito
- Data: 2026-10-30
- Atualização 2026-10-31: o comando instalado passou a ser `ai-usage` (ADR 0025). O texto abaixo registra a decisão original.

## Contexto

O agente hoje só roda com `go run` a partir do código. O repositório é público. Nos outros projetos, a pessoa instala com um script que está na GitHub Release: o script baixa o pacote e o SHA256 da mesma release e coloca o binário em `~/.local/bin`.

## Decisão

- A tag `vX.Y.Z` dispara o workflow `.github/workflows/release.yml`. Ele compila só `./cmd/agent` para Linux e macOS, em amd64 e arm64, e publica `agent-installer.sh`, os pacotes `agent_<versão>_<os>_<arch>.tar.gz` e `checksums.txt`.
- O script confere o SHA256 antes de instalar. O binário fica em `~/.local/bin/agent`, a menos que `--bin-dir` diga outro lugar.
- O painel não entra no pacote. A atualização do agente é opt-in e não entra neste corte: o script instala a versão baixada e o processo não procura outra release.
- O checksum protege contra arquivo corrompido. Não protege contra uma release alterada por quem publica.

## Alternativas rejeitadas

- cargo-dist: o formato de uso é o mesmo de acari e tailmux, mas este repositório é Go.
- Publicar também o servidor: a instalação do painel continua no próprio host.
- Binário de Windows: o agente foi feito para Linux e macOS.
- Instalar sem conferir o checksum.

## Consequências

A release só existe depois de um push da tag. Este corte não publica tag. Quem já tem o código continua podendo usar `go run ./cmd/agent`.

O `go build ./cmd/agent` não importa o sqlite-replication. O workflow não executa `make quality`: a suíte do rastreador depende do módulo irmão `../sqlite-replication/go`, e o empacotador não precisa dele.

## Correção no mesmo dia

A primeira redação deste ADR dizia que não havia atualização automática, copiando o limite do MVP pessoal. A pessoa esclareceu que a atualização existe e é opt-in. Esse comportamento está no ADR 0020.

## Quando revisitar

Se o agente passar a ser instalado por um gerenciador de pacotes, ou quando a atualização opt-in for implementada.
