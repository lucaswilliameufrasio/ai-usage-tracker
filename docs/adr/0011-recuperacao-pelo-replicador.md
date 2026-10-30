# ADR 0011 — Recuperação pelo replicador

- Status: aceito
- Data: 2026-10-30

## Contexto

O ADR 0010 entrega um snapshot local pelo comando `tracker backup`. Isso serve para um drill isolado. Não é a recuperação de desastre do produto, e a tela não deve oferecer o arquivo do banco para download: o SQLite contém hashes de sessão e de dispositivo.

O repositório `sqlite-replication` já implementa snapshot LTX, restauração num caminho novo e, em parte, captura de WAL. O rastreador só guarda histórico de uso. Ele é a cobaia adequada. O banco atual não está em modo WAL, e o replicador exige um escritor único com write-gate. O README desse projeto ainda marca parte do trabalho como experimental.

## Decisão

- A recuperação do produto é o `sqlite-replication`, não um botão no PWA.
- O comando do ADR 0010 continua como drill local. Ele não substitui o replicador e não restaura por cima de `data/`.
- Ligar o processo que está no ar ao replicador só depois de um drill isolado: banco temporário em WAL, snapshot LTX e restore num caminho novo, sem abrir `data/tracker.db`.
- O cofre e a chave continuam fora do banco e fora do snapshot LTX.

## Alternativas rejeitadas

- Tela de backup que baixa `tracker.db`: entrega o histórico e os hashes ao navegador e não prova restauração.
- Apontar o replicador experimental para o `data/` em uso nesta fatia: o journal ainda não é WAL e um erro de captura atingiria o único histórico.

## Consequências

A próxima fatia de recuperação muda o journal para WAL e chama o replicador num banco de teste. O processo em validação não muda de modo até esse drill passar. E-mail BYOK continua à espera do escopo.

## Quando revisitar

Quando o drill isolado passar e o replicador tiver o contrato de write-gate que o processo do rastreador consiga cumprir.
