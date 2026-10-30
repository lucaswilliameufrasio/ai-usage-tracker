# ADR 0021 — Datas no fuso do dispositivo

- Status: aceito
- Data: 2026-10-30

## Contexto

O fio guarda instantes em UTC. A tela formatava esse instante com o rótulo UTC, então o horário visto não era o do aparelho.

## Decisão

- A API continua em UTC.
- A tela formata o instante no fuso do dispositivo, com o nome curto desse fuso.
- O texto do setup no terminal usa o fuso local da máquina.

## Alternativas rejeitadas

- Guardar o horário já convertido.
- Fixar um fuso da instalação para todas as pessoas.

## Consequências

Dois aparelhos no mesmo instante podem mostrar horas diferentes. A repaginação de UI, UX e DX continua em aberto; este fuso é o primeiro ajuste dela.

## Quando revisitar

Se a repaginação passar a mostrar o fuso escolhido nas preferências de alerta em vez do fuso do dispositivo.
