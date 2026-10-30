# ADR 0001 — Stack do MVP pessoal

- Status: aceito
- Data: 2026-10-29

## Contexto

A fatia 1 precisa provar coleta local, ingestão, persistência e visualização num PWA single-user. O plano cita um cliente HTTP Go futuro para o Ciphera, nomeia SQLite na fase 1 e deixa stack, versões e retenção em aberto. Não há aplicação prévia neste repositório. E-mail BYOK, Sabiá, Ciphera, cobrança e multiusuário ficam fora desta fatia.

## Decisão

- API e agente em Go, um módulo `ai-usage-tracker`, com dois comandos (`tracker` e `agent`).
- Persistência em SQLite, via driver puro Go `modernc.org/sqlite`, um arquivo local por instalação.
- PWA em TypeScript, servido pelo mesmo processo na origem da API. Sem framework de UI.
- Segredos de servidor desta fatia limitam-se ao token do operador, tokens de dispositivo e o par VAPID gerado no diretório de dados. Ciphera fica fora do MVP.
- Não há gateway genérico, kit compartilhado nem serviço de notificação separado.

## Alternativas rejeitadas

- Postgres ou outro servidor de banco: desnecessário para uma instalação pessoal e aumenta o custo de backup local.
- Node ou Python na API: o plano já aponta Go para o cliente de segredos e um binário único serve melhor ao agente em Linux/macOS.
- Ler provedores de IA a partir do backend: viola a fronteira de credenciais.

## Consequências

A escolha vale para o uso pessoal local. Antes de multiusuário ou de hospedar o banco longe da máquina do operador, a persistência e o modelo de autenticação precisam de uma decisão nova. Retenção automática não entra nesta fatia: o histórico permanece no SQLite até uma política explícita.

## Quando revisitar

Mais de uma pessoa no mesmo servidor, ou volume em que um único arquivo SQLite deixe de ser operável.

O primeiro gatilho ocorreu em 2026-10-30. A decisão de conta está no ADR 0005. A stack desta fatia permanece.
