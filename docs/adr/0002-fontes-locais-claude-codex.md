# ADR 0002 — Fontes locais de Claude e Codex

- Status: aceito
- Data: 2026-10-29

## Contexto

A fatia 1 só pode prometer limites e resets se essas leituras existirem na máquina, com um contrato identificável, sem chamar os provedores com as credenciais do usuário. A inspeção desta máquina e a documentação pública estão em `docs/source-inspection.md`.

O que foi verificado:

- Claude Code não grava o limite do plano em `stats-cache.json`. Esse arquivo é agregado local de tokens. O comando `/usage` e a status line oficial expõem `rate_limits.five_hour` e `rate_limits.seven_day` (`used_percentage`, `resets_at`) só para assinatura, depois da primeira resposta, e isso depende do cliente oficial já ter consultado a conta.
- Nesta máquina, `~/.claude.json` contém `cachedUsageUtilization` com `five_hour` e `seven_day` (`utilization`, `resets_at` ISO-8601) e `fetchedAtMs`. O mesmo arquivo contém conta OAuth, caminhos de projeto e outros caches. Não é uma API de limites.
- Codex CLI persiste, em alguns `rollout-*.jsonl`, `payload.rate_limits` com `primary`/`secondary` (`used_percent`, `window_minutes`, `resets_at`), `plan_type` e `limit_id`. O campo também aparece `null`. Os mesmos arquivos contêm o transcript. Não há binário CodexBar nesta máquina. `account/rateLimits/read` do app-server usaria a credencial local para falar com a OpenAI; isso não faz parte desta fatia.
- Não há permissão, nesta fatia, para enviar token, cookie ou sessão aos provedores. A coleta permitida é leitura local, explícita, dos campos de limite já gravados pelos clientes oficiais.

## Decisão

- O agente só lê um arquivo ou diretório passado na linha de comando. Não há leitura implícita do diretório home.
- Claude: aceitar o subdocumento `cachedUsageUtilization` ou o JSON documentado da status line. Extrair só janelas `five_hour` e `seven_day`.
- Codex: varrer JSONL de rollout e extrair só o `rate_limits` não nulo mais recente. Janela de cerca de 300 minutos vira `five_hour`; cerca de 10080 minutos vira `seven_day`.
- Se o arquivo existe e não há janela com reset compreensível, o agente envia um relatório `unavailable`. Não inventa limite nem reset.
- Credencial, prompt, caminho, identificador de conta e o restante do arquivo não entram no corpo enviado ao servidor.
- CodexBar e qualquer chamada autenticada ao provedor ficam fora até uma decisão que autorize esse acesso.

## Alternativas rejeitadas

- Estimar o limite do plano a partir de tokens em transcripts (`ccusage` e `stats-cache`): isso não é o reset oficial e misturaria conteúdo de conversa com a coleta.
- Chamar `/usage`, `account/rateLimits/read` ou o endpoint da conta: usa a credencial e não pode ser testado sem uma chamada real.
- Tratar ausência de `rate_limits` como zero usado: esconderia fonte indisponível.

## Consequências

A leitura fica atrasada em relação ao provedor quando o cliente oficial não atualiza o cache. O PWA precisa mostrar o instante observado e o estado desatualizado. Uma mudança de formato nos clientes quebra o coletor correspondente sem derrubar o outro.

## Quando revisitar

Se a Anthropic ou a OpenAI publicarem um contrato local estável, ou se o usuário autorizar uma leitura autenticada específica, com sandbox e sem encaminhar a credencial ao rastreador.
