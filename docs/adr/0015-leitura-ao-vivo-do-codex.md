# ADR 0015 — Leitura ao vivo do Codex pelo CLI

- Status: aceito
- Data: 2026-10-30

## Contexto

Os rollouts novos deixaram de gravar `rate_limits`. A última observação em arquivo fica antiga e o cartão não anda. O ADR 0002 deixou `account/rateLimits/read` de fora até uma autorização explícita. A pessoa pediu para seguir o caminho do CodexBar.

Há dois programas com esse nome. Um lê `~/.codex/auth.json`, renova o token e chama a API do ChatGPT. O outro sobe `codex app-server` na máquina e lê `account/rateLimits/read`. Nesse segundo, o Codex continua dono da credencial e o aplicativo não lê `auth.json`.

## Decisão

- Com o Codex configurado, o agente tenta o comando local `codex app-server` antes do arquivo.
- O protocolo é JSON-RPC em stdio: `initialize`, a notificação `initialized`, depois `account/rateLimits/read`.
- Entram na amostra só plano, percentual, duração da janela e `resetsAt`. E-mail, conta, créditos, título e o restante da resposta são descartados.
- O instante observado da leitura ao vivo é o relógio da coleta. Isso separa a amostra do arquivo antigo.
- Se o comando falta, falha ou não traz janela com reset, vale a leitura do rollout. Essa falha não gera um relatório mais novo, para o cartão não esconder a última observação utilizável.
- Este processo não lê `auth.json` e não chama a API HTTP do provedor.
- O teste usa um executável falso. O binário real e a conta real não entram no teste.

## Alternativas rejeitadas

- Ler `auth.json` e chamar o ChatGPT: o token passaria pelo nosso processo.
- Depender do aplicativo CodexBar: ele não é o contrato e não precisa estar instalado.
- Apagar a leitura de arquivo: uma falha do CLI zeraria o cartão.
- Emitir `unavailable` com a data de agora quando o CLI falha e o arquivo tem amostra: o frescor trata o relatório mais novo como vitória e esconde o limite antigo.

## Consequências

O ADR 0002 segue para o que o arquivo contém. A chamada ao vivo usa a sessão do Codex e fala com a OpenAI. O rastreador continua sem a credencial. O teste não prova a conta real; a próxima execução do agente na máquina da pessoa é que pode atualizar o cartão.

## Quando revisitar

Se o Codex mudar o método, ou se a pessoa pedir para parar de consultar o provedor.
