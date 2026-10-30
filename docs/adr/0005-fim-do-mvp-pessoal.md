# ADR 0005 — Fim do MVP pessoal

- Status: aceito
- Data: 2026-10-30

A frase abaixo que fixa o GitHub como único login e deixa o Google fora dessa etapa foi substituída em 2026-10-30 pelo ADR 0006. O restante permanece.

## Contexto

A fatia pessoal provou coleta local, ingestão, histórico e o PWA para uma pessoa. Em 2026-10-30 essa pessoa concluiu a validação e declarou que era o cliente até então. O ADR 0001 pedia uma decisão nova de autenticação antes de mais de uma pessoa no mesmo servidor. O plano deixava login social fora do MVP pessoal e previa login na fase de abertura.

## Decisão

- O MVP pessoal está encerrado. O uso de duas semanas no celular deixa de ser condição para seguir.
- O próximo produto é para outras pessoas. O operador único e o token copiado no navegador não são o modelo de conta.
- Quem instala o servidor continua sendo o administrador. O segredo de setup é só desse papel, na primeira abertura fora de loopback. As outras pessoas não o recebem.
- Cada pessoa entra com a própria conta. O primeiro login do produto é GitHub. Google fica fora dessa etapa.
- O código curto de pareamento continua, e pertence ao agente daquela pessoa. A sessão do navegador não autoriza o agente.
- Lançamento público, termos versionados, cobrança e workspaces continuam numa etapa posterior. Contas para outras pessoas podem existir numa instalação antes disso.
- A fronteira de privacidade permanece: credenciais de IA, prompts e caminhos não vão ao servidor.

## Alternativas rejeitadas

- Mais uma rodada só para uma pessoa, com o mesmo token de operador: a validação pedida já aconteceu.
- Tratar o segredo de setup como senha de todo usuário: mistura administração da instalação com uso diário.
- Substituir o pareamento do agente pela sessão do GitHub: o agente não tem o navegador da pessoa.
- Abrir cadastro público junto com o primeiro login: termos, isolamento e cobrança ainda não estão decididos.

## Consequências

O código em execução continua o da fatia pessoal até a etapa de contas. SQLite e o processo único permanecem até a decisão de hospedar o banco fora da máquina do administrador. O aplicativo OAuth do GitHub é pré-requisito para o login funcionar de verdade.

## Quando revisitar

Mais de uma instalação ou cobrança. O gatilho de outro login além do GitHub ocorreu em 2026-10-30 e está no ADR 0006.
