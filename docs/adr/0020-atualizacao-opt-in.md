# ADR 0020 — Atualização opt-in do agente

- Status: aceito
- Data: 2026-10-30
- Relacionado: ADR 0018

## Contexto

O ADR 0018 instala uma versão e, na primeira redação, deixava a atualização fora do produto. A pessoa esclareceu que a atualização existe e é opt-in.

## Decisão

- O padrão é desligado. O setup pergunta; Enter mantém desligado. A escolha fica em `auto_update` na configuração local.
- Com a opção ligada, cada passagem do loop, antes do envio, consulta a release estável. Se a tag for maior que a versão embutida no binário, baixa o pacote da mesma release, confere o SHA256 e substitui o executável. O processo então reinicia com os mesmos argumentos.
- A versão `dev`, que é a de `go run`, não consulta a rede.
- O checksum protege contra arquivo corrompido. Não protege contra uma release alterada por quem publica.
- O teste usa um servidor local. Não chama o GitHub.

## Alternativas rejeitadas

- Atualizar sem a pessoa ligar a opção.
- Atualizar uma execução `go run`.
- Reiniciar no meio de um envio já começado.

## Consequências

O binário empacotado recebe a versão pelo `-X` do empacotador. Uma falha na atualização não cancela o envio daquela passagem.

## Quando revisitar

Se a atualização passar a exigir confirmação a cada release, ou se o binário deixar de se chamar `agent`.
