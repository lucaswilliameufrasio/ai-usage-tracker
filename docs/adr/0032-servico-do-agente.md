# ADR 0032 — Serviço do agente

- Status: aceito
- Data: 2026-11-01
- Relacionado: ADR 0016, ADR 0018, ADR 0031

## Contexto

O ADR 0016 deixou a repetição como um comando em primeiro plano e recusou tratá-la como serviço instalado, com a condição de revisitar se a repetição virasse serviço do sistema. A pessoa quer o agente rodando em segundo plano sem montar arquivos de systemd ou launchd à mão, e quer poder desfazer e diagnosticar.

## Decisão

- O comando ganha `ai-usage service install`, `service uninstall` e `service doctor`.
- Linux usa um serviço de usuário do systemd (`~/.config/systemd/user/ai-usage.service`). macOS usa um LaunchAgent (`~/Library/LaunchAgents/dev.ai-usage.agent.plist`). Nada exige root. Windows continua sem suporte (ADR 0018) e o comando diz isso.
- O serviço chama `ai-usage run`, o mesmo laço do ADR 0016. Não há segundo caminho de envio.
- `install` só continua com a configuração pronta (`ai-usage setup`) e com o token gravado. Recusa binário temporário de `go run`. Grava o caminho absoluto do binário que está rodando.
- O PATH do serviço é montado na hora da instalação: a pasta do binário, as pastas de `codex`, `agy` e `node` achadas no PATH do terminal, `~/.local/bin` e os diretórios padrão. Serviços começam com PATH mínimo, e sem isso a leitura ao vivo do Codex cai para o arquivo.
- Reinstalar reescreve o arquivo e reinicia. `--dry-run` mostra o arquivo e os comandos sem mudar nada.
- `uninstall` para e desativa o serviço e apaga só o arquivo do serviço. Configuração, token e fila local ficam. Repetir não dá erro.
- `doctor` só lê. Confere plataforma, configuração, token, binário, arquivo do serviço (e se aponta para este binário), estado do serviço, linger no Linux, ferramentas no PATH do serviço, `/healthz` do servidor e a fila local (incluindo itens bloqueados por revogação). Imprime `ok`, `warn` ou `fail` com a correção, e sai com 1 se houver `fail`.
- No Linux o linger não é ligado sozinho. `install` e `doctor` avisam e dizem o comando, porque mexe numa configuração da conta.

## Alternativas rejeitadas

- Modo daemon próprio (fork e pidfile). Reimplementa o que o sistema já faz melhor.
- Serviço de sistema com root. A leitura é de arquivos do usuário e não precisa.
- Ligar o linger automaticamente.

## Consequências

No macOS o launchd reinicia o agente também quando o dispositivo foi revogado (saída 3), no máximo uma vez por minuto. O `doctor` mostra o bloqueio. No Linux a saída 3 não reinicia. Mudar de pasta do `ai-usage` ou de gerenciador do `codex` exige `service install` de novo, e o `doctor` avisa quando o serviço aponta para outro binário.

## Quando revisitar

Se Windows entrar no suporte, ou se o serviço precisar de mais de uma instância por máquina.
