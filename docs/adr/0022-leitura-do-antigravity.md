# ADR 0022 — Leitura do Antigravity pelo CLI

- Status: aceito
- Data: 2026-10-30

## Contexto

O cartão precisa de percentual e de horário de reset. A inspeção de nomes não achou um arquivo local com esses dois campos. As barras de uso leem essa janela de dois jeitos: sondam o language server local, ou abrem o token OAuth e chamam a API do Google. O segundo caminho é o que o ADR 0015 recusou para o `auth.json` do Codex.

Nesta máquina o `agy` é 1.4.7. O comando `agy -p /usage --output-format json` devolve `command.name` igual a `usage` e `command.data.groups`. Cada grupo traz `buckets` com `id`, `window`, `remaining_fraction` e `reset_time`. A fração vai de 0 a 1 e é o que resta. `num_turns` veio 0 e os tokens da chamada vieram 0: não foi uma conversa com modelo. Um plano pode trazer só a janela semanal.

## Decisão

- O setup pergunta. Enter mantém desligado. `s` grava `agy_command` como `agy`. `n` apaga.
- Com o comando configurado, cada passagem do agente executa `agy --print-timeout 20s --output-format json -p /usage`.
- Entram na amostra o percentual usado, o nome da janela e o reset. O percentual usado é o complemento da fração restante.
- Os nomes ficam `gemini_five_hour`, `gemini_seven_day`, `claude_gpt_five_hour`, `claude_gpt_seven_day` e, se o balde for Argon, `gemini_argon_five_hour` ou `gemini_argon_seven_day`.
- Texto da resposta, e-mail, identificador de conversa e contagem de tokens são descartados. Uma resposta sem o comando `usage`, ou com `num_turns` maior que zero, não vira amostra.
- Falha do comando não gera relatório novo. A última amostra permanece até envelhecer.
- O processo não lê token, keyring nem banco do Antigravity, e não chama a API do Google.
- O teste usa um executável falso. A conta real não entra no teste.
- OpenCode e Grok Build continuam fora. A janela que as outras barras mostram desses dois sai de ler o login e chamar o provedor.

## Alternativas rejeitadas

- Ler o token e chamar `retrieveUserQuotaSummary`: a credencial passaria por este processo.
- Depender do aplicativo CodexBar ou do language server aberto: o contrato é o comando do `agy`.
- Tratar `agy -p` de um prompt comum como janela: esse envelope traz tokens da conversa, não o plano.
- Inventar a janela de 5 horas quando o comando só devolve a semanal.

## Consequências

O provedor `antigravity` passa a ser aceito na amostra. O painel mostra o cartão quando existe amostra ou relatório. Claude e Codex continuam sempre visíveis. O teste não prova a conta real; a próxima execução do agente, com a opção ligada, é que pode atualizar o cartão.

## Quando revisitar

Se o `agy` deixar de responder `/usage` nesse JSON, ou se a pessoa pedir para parar de consultar o provedor por esse comando.
