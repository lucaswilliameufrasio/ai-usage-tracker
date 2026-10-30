# ADR 0007 — Cofres da instalação

- Status: aceito
- Data: 2026-10-30
- Substitui: a frase do ADR 0006 que nomeia o Ciphera como a implementação do porto de segredo, e a consequência que trata o cofre local como espera pelo Ciphera. O restante do ADR 0006 permanece.

## Contexto

O produto é self-hosted. Cada instalação escolhe onde guarda os segredos do servidor. O plano descreve o Ciphera como cofre opcional e deixa claro que ele devolve o valor ao processo, sem operação remota de KMS. O ADR 0006 fez o login depender de um porto estreito e ainda apontou o Ciphera como a implementação prevista. Em 2026-10-30 o administrador pediu integração com Ciphera, um cofre próprio, HashiCorp Vault, AWS Secrets Manager, Infisical e outros do mesmo tipo.

## Decisão

- O porto do ADR 0006 continua: gravar, ler e apagar por referência. O valor sai só para o processo do servidor. A interface vê presença, backend e data de troca.
- Uma instalação tem um backend ativo. Outra instalação pode escolher outro. Não há segundo backend de reserva: se o ativo falha, o segredo dependente falha fechado. Não há queda para texto puro, para o SQLite nem para a variável de ambiente.
- Os backends nomeados são o cofre próprio da instalação, Ciphera, HashiCorp Vault no caminho KV, AWS Secrets Manager e Infisical. Outro entra depois se cumprir o mesmo porto, inclusive devolver o valor ao processo. KMS, Transit ou qualquer API que só embrulha dados sem devolver o segredo fica fora deste porto.
- O cofre próprio é o padrão quando não há store externo. A instalação sobe sem Ciphera, Vault, AWS ou Infisical. O material de abertura vem do operador e fica separado do arquivo cifrado. Uma chave ao lado do arquivo não conta como essa separação. O formato cifrado concreto fica para a fatia que implementar esse backend.
- Ciphera permanece um adapter opcional, com o contrato já escrito no plano: leitura autorizada, valor em memória, sem `wrap/unwrap`, sem `ciphera run` como isolamento. Ele não é obrigatório e não é o único.
- HashiCorp Vault usa o KV da instalação. O modo de autenticação do processo é escolha de deploy. Transit sozinho não serve.
- AWS Secrets Manager usa a cadeia de credencial do processo e a região configurada. A interface não recebe access key. AWS KMS sozinho não serve.
- Infisical aceita a URL do serviço, inclusive uma instância self-hosted, mais o projeto e o ambiente. A identidade de máquina entra na abertura do processo.
- O acesso ao backend ativo é configuração de deploy: endereço, papel, região, token ou material de abertura. Esse acesso não pode morar dentro do cofre que ele abre. A área de administração mostra qual backend está ativo e se a leitura responde. Trocar de backend é uma migração explícita: gravar no novo, republicar as referências, só então apagar no antigo. Uma referência da revisão anterior não é relida em outro backend.
- Segredos de servidor que podem usar o porto: client secret dos logins, chave `.p8` da Apple e, nas fases já previstas, credencial de e-mail e chave VAPID. Credenciais de IA, prompts e caminhos continuam fora de todos esses cofres.
- A política de cache quando o backend remoto não responde continua aberta. Até ela existir, cache vencido e backend indisponível fecham a função. A política vale para o backend ativo, não só para o Ciphera.
- Nenhum desses clientes está implementado. Teste de adapter usa fake de protocolo e segredo sintético. Chamada a cofre real fica fora da suíte local.

## Alternativas rejeitadas

- Fixar o Ciphera como único cofre: uma instalação self-hosted pode não ter esse serviço.
- Exigir HashiCorp, AWS ou Infisical para ligar o produto: o cofre próprio existe para a instalação pequena.
- Vários backends ativos com fallback entre eles: uma falha passaria a ler segredo de outro lugar, ou a misturar revisões.
- Tratar AWS KMS ou Vault Transit como este porto: o login precisa apresentar o client secret, e esses modos não o devolvem.
- Colar token de Vault, chave de AWS ou identidade do Infisical no formulário e gravá-los no SQLite: o segredo que abre o cofre ficaria no mesmo arquivo do uso.
- Implementar os cinco clientes nesta decisão.

## Consequências

O ADR 0006 segue valendo para os quatro logins e para o porto. O backup do SQLite não recupera sozinho os segredos. Restaurar o cofre próprio exige o arquivo cifrado e o material de abertura. Restaurar um backend externo exige que esse serviço ainda entregue as mesmas referências. A fatia de código começa pelo porto e pelo cofre próprio. Cada adapter externo entra com o contrato do plano: timeout, limite de resposta, erro redigido e falha fechada.

## Quando revisitar

Mais de um backend ativo na mesma instalação, uma política de cache publicada, ou um serviço que só ofereça cifra remota e ainda assim precise guardar segredo do servidor.
