# ADR 0016 — Repetição do agente e fila no desligamento

- Status: aceito
- Data: 2026-10-30

## Contexto

O `agent run` enviava uma vez e saía. O cartão só andava quando a pessoa lembrava de rodar de novo. A fase 2 pede o esvaziamento da fila local no desligamento. A fila já grava o item antes do envio e só o apaga depois de uma resposta 2xx. O modo com flags continua o contrato de script do ADR 0014.

## Decisão

- `agent run` sem flags, com a configuração pronta, repete a cada 5 minutos até SIGINT ou SIGTERM.
- O primeiro envio acontece na hora. Ctrl+C durante a espera não começa outra coleta e sai com a fila como está.
- Ctrl+C durante um envio deixa esse envio terminar. O prazo desse envio é 40 segundos. Esgotado o prazo, a coleta ao vivo e o HTTP são cancelados. O item sem confirmação continua no arquivo.
- Dispositivo revogado encerra com código 3 e não entra de novo. Token ilegível encerra com código 1. Falha de rede imprime que a fila foi preservada e tenta de novo no próximo intervalo.
- `agent run` com flags e o envio no fim do setup continuam uma vez só.
- SIGKILL, queda da máquina e um segundo sinal não são um esvaziamento. O arquivo da fila é a recuperação. Um item já aceito pelo servidor pode ser reenviado e volta como duplicata.

## Alternativas rejeitadas

- Repetir também o modo com flags: um script que espera o processo sair passaria a ficar preso.
- Apagar a fila ao receber o sinal: perderia o que o servidor não confirmou.
- Esperar o envio sem prazo: um CLI travado seguraria o desligamento.
- Tratar a repetição como serviço instalado: a pessoa ainda roda o comando no terminal.

## Consequências

Cada passagem do Codex ao vivo pode gravar uma amostra nova, porque o instante observado muda. O aviso do mesmo limiar e da mesma janela não dispara outra vez. O processo em primeiro plano é o que mantém o cartão atual. Quem fechar o terminal para a repetição.

## Quando revisitar

Se a repetição passar a ser um serviço do sistema, ou se o histórico de leituras iguais ficar grande demais.
