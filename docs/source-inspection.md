# Inspeção das fontes locais — 2026-10-29

Inspeção somente de nomes, formatos e documentação. Valores de uso, tokens, caminhos de projeto e credenciais não foram copiados para este repositório.

## Ambiente

- Binários locais presentes: `claude`, `codex`.
- `codexbar`: ausente.
- Diretórios de cliente presentes: configuração do Claude Code e do Codex CLI. O conteúdo sensível não faz parte do contrato versionado.

## Claude

| Fonte | O que contém | Serve como limite e reset? |
| --- | --- | --- |
| `stats-cache.json` | Agregados locais de sessão, modelo e tokens | Não. Não há janela de plano nem `resets_at`. |
| Status line oficial | `rate_limits.five_hour` e `rate_limits.seven_day` com `used_percentage` e `resets_at` em segundos Unix | Sim, quando o cliente oficial envia esse JSON. Ausente em chave de API e antes da primeira resposta. Documentação: [status line](https://code.claude.com/docs/en/statusline). |
| `cachedUsageUtilization` no arquivo de estado do Claude Code | `five_hour` e `seven_day` com `utilization` e `resets_at` ISO-8601, mais `fetchedAtMs` | Sim, como cache já gravado pelo cliente. O arquivo também guarda OAuth e caminhos; o coletor não pode reenviar o documento. |
| `/usage` dentro do CLI | Barras do plano, via conta | Fora da fatia. Exigiria chamada autenticada. |

Nesta máquina, `cachedUsageUtilization.five_hour.resets_at` e `seven_day.resets_at` são strings ISO-8601. Há também uma lista `limits` com kinds `session`, `weekly_all` e `weekly_scoped`. O coletor usa os objetos nomeados `five_hour` e `seven_day` para não depender de rótulos experimentais no mesmo documento.

## Codex

| Fonte | O que contém | Serve como limite e reset? |
| --- | --- | --- |
| `rollout-*.jsonl` | Em pelo menos um arquivo local, `payload.rate_limits` não nulo com `used_percent`, `window_minutes`, `resets_at`, `plan_type`, `limit_id` | Sim, como efeito colateral da sessão. Outros arquivos não tinham o campo. O transcript está no mesmo JSONL. |
| `auth.json` | Credencial local | Não ler e não enviar. |
| `codex app-server` `account/rateLimits/read` | `rateLimits.primary` e `secondary`, com `usedPercent`, `windowDurationMins` e `resetsAt` em segundos Unix | Sim, desde o ADR 0015. O CLI local fala com a OpenAI. Este processo não lê o token. O teste usa um executável falso. |
| CodexBar que lê `auth.json` | Token local reenviado à API do ChatGPT | Rejeitado. |
| CodexBar que usa o app-server | O mesmo RPC do CLI, sem ler `auth.json` | O método foi adotado. O aplicativo não é dependência. |

O formato acompanha o `RateLimitSnapshot` / `RateLimitWindow` publicados no protocolo do Codex (`usedPercent`, `windowDurationMins`, `resetsAt`), com os nomes em snake_case nos rollouts já gravados (`used_percent`, `window_minutes`, `resets_at`).

## Permissão desta fatia

A coleta de arquivo continua opt-in, por caminho explícito, restrita aos campos de janela acima. O ADR 0015 autoriza só a leitura em que o comando `codex` fala com a OpenAI. Não há autorização para ler `auth.json`, chamar a API HTTP daqui, nem gravar fixture a partir dos arquivos pessoais. Os testes usam `fixtures/synthetic/` e um executável falso.

## OpenCode, Antigravity e Grok Build — 2026-10-30

Inspeção de nomes e da documentação local. Os arquivos de autenticação, conta e banco desses clientes não foram abertos. Nenhum percentual foi copiado.

| Cliente | O que apareceu | Serve como limite e reset? |
| --- | --- | --- |
| OpenCode | Binário local e arquivos de conta, autenticação e banco. Nenhum nome de arquivo de uso, limite ou reset. | Não. Sem contrato local de janela. |
| Antigravity | Binário `agy` 1.4.7. O comando `agy -p /usage --output-format json` devolve `command.data.groups[].buckets` com `id`, `window`, `remaining_fraction` e `reset_time`. A fração é o que resta, de 0 a 1. Um plano pode trazer só a semana. | Sim, pelo comando. O token não é lido. ADR 0022. |
| Grok Build | Binário `grok`. O guia local de monitoramento descreve OpenTelemetry externo, desligado por padrão, com contagem de tokens. Pode anexar e-mail. Não é um arquivo de percentual e reset. | Não. Não ligar o exportador e não ler a sessão. |

O Antigravity entra pelo comando do próprio `agy`, no mesmo espírito do Codex: o CLI guarda o login. OpenCode e Grok Build continuam fora. `opencode stats` e `grok usage` trazem tokens e custo de sessão, não a janela do plano. A janela que as outras barras mostram desses dois sai de ler o arquivo de login e chamar o provedor. Esse caminho não foi adotado.
