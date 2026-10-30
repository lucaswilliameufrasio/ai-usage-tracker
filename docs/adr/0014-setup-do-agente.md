# ADR 0014 — Setup do agente no terminal

- Status: aceito
- Data: 2026-10-30

## Contexto

O ADR 0002 manda o agente ler só o arquivo e o diretório recebidos na linha de comando. Isso evita varrer o diretório pessoal em todo envio. Na prática, a pessoa precisa repetir endereço, token, Claude e Codex em cada `run`. O Codex grava `rollout-*.jsonl` em pastas `ano/mês/dia`. Um diretório pai, passado certo, ainda assim não via os arquivos. Sessão nova sem `rate_limits` não é leitura mais recente.

## Decisão

- `agent run` com `--api`, `--token-file` e uma fonte continua o contrato de script. Nada é descoberto nesse modo.
- `agent run` sem esses flags usa `~/.config/ai-usage-tracker/agent.json` quando ela já está completa. Na primeira vez, e em `agent setup`, o terminal conduz o setup.
- O setup propõe `~/.claude.json` e `~/.codex/sessions`. Ele mostra se há limite e a data observada. Só grava o caminho confirmado. Não mostra percentual, prompt, credencial nem identificador de conta.
- O token do dispositivo fica em arquivo próprio, modo `0600`. A configuração guarda o caminho, não o token.
- A pasta de Codex confirmada inclui as subpastas. Vale o `rate_limits` mais recente entre os arquivos lidos. Arquivo sem esse campo não substitui uma observação anterior.
- Sem terminal, a ausência de flags e de configuração falha com instrução. Não há pergunta interativa em pipe.
- Não entra framework de TUI, tela web, nem leitura autenticada no provedor.

## Alternativas rejeitadas

- Exigir os flags para sempre: o enrolamento fica difícil de repetir e de explicar.
- Varredura do diretório pessoal em todo `run`: o ADR 0002 separa execução de descoberta.
- Tratar a sessão mais nova sem `rate_limits` como ausência de limite: isso apaga a última observação utilizável.
- Bubble Tea ou outra interface: o setup são telas no próprio processo, cobertas por teste sem terminal gráfico.

## Consequências

O ADR 0002 segue valendo para o modo com flags e para o que pode ser extraído do arquivo. A descoberta fica restrita ao setup e aos dois lugares conhecidos. Uma sessão do Codex que não grave `rate_limits` continua sem atualizar o cartão. Credencial de provedor não é chamada.

## Quando revisitar

Se o Claude ou o Codex mudarem o lugar do arquivo local, ou se uma leitura autenticada for autorizada.
