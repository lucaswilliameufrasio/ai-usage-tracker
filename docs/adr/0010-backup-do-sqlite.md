# ADR 0010 — Backup do SQLite

- Status: aceito
- Data: 2026-10-30

## Contexto

A fase 2 pede um drill de backup e restauração isolada do SQLite, com integridade e contagens. O processo em validação já usa `data/tracker.db`. O ADR 0007 guarda os segredos no cofre próprio, fora desse arquivo. Copiar o banco à mão pode pegar um journal incompleto. Restaurar por cima do arquivo em uso apagaria a instalação que está no ar.

## Decisão

- `tracker backup` grava um snapshot com `VACUUM INTO`. O banco de origem permanece. O destino que já existe é recusado, inclusive quando o destino é o próprio `tracker.db`.
- `tracker restore` só grava `tracker.db` num diretório que ainda não tem esse arquivo. Antes de copiar, o snapshot precisa passar em `integrity_check` e estar na `user_version` 3.
- A conferência é a versão do esquema e as contagens de contas, dispositivos, amostras, sessões e provedores de login. O comando não imprime tokens, segredos nem o conteúdo das amostras.
- O snapshot não inclui o cofre, a chave do cofre nem o par VAPID. Restaurar o banco não reabre segredo nenhum.

## Alternativas rejeitadas

- Copiar `tracker.db` com o processo aberto: no modo de journal padrão isso pode gravar um arquivo incompleto.
- Restaurar por cima de `data/`: o drill passaria a ser uma troca do banco que está servindo a instalação.
- Colocar a chave do cofre dentro do snapshot: o arquivo passaria a ser suficiente para ler os segredos.

## Consequências

O comando pode rodar enquanto o servidor está aberto, porque abre outra conexão com `busy_timeout`. O drill automatizado usa diretório temporário e não lê `data/`. Um snapshot de outra versão de esquema é recusado. Cofre, retenção e e-mail continuam fora desta fatia.

## Quando revisitar

Quando a `user_version` passar de 3, ou quando o backup precisar incluir o cofre cifrado como conjunto separado.
