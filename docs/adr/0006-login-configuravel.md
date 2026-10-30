# ADR 0006 — Login configurável pelo administrador

- Status: aceito
- Data: 2026-10-30
- Substitui: a frase do ADR 0005 que fixa o GitHub como único login e deixa o Google fora dessa etapa. O restante do ADR 0005 permanece.

A frase abaixo que nomeia o Ciphera como a implementação do porto, e a consequência que trata o cofre local como espera pelo Ciphera, foram substituídas em 2026-10-30 pelo ADR 0007. Os logins e o porto permanecem.

## Contexto

O administrador de uma instalação não é, necessariamente, usuário do GitHub. Em 2026-10-30 ficou decidido configurar o login uma vez por variáveis de ambiente e, depois, trocá-lo na área de administração, com um cofre para os segredos. Os provedores pedidos são Gitea, GitHub, Google e Apple. O plano já nomeia o Ciphera como cofre futuro e deixa a política de cache em aberto. O código em execução ainda é o da fatia de um operador.

## Decisão

- A área de administração configura Gitea, GitHub, Google e Apple. Cada um é opcional. A instalação precisa manter ao menos um método de login habilitado. Desligar o último é recusado.
- O segredo de setup continua só com o administrador, na primeira abertura. Ele não é senha das outras pessoas e não substitui um login desligado.
- Variáveis de ambiente semeiam um provedor apenas quando ele ainda não tem configuração gravada. Um reinício não sobrescreve o que a área de administração publicou. Depois do primeiro save, a revisão publicada é a fonte da verdade.
- O SQLite guarda a configuração não secreta e uma referência de segredo: habilitado, identificador público do cliente, URL base do Gitea, e, na Apple, Team ID, Key ID e Services ID. Não guarda client secret, chave `.p8` nem JWT.
- O login fala com um porto estreito de segredo: gravar, ler e apagar por referência. A leitura devolve o valor só ao processo do servidor. A área de administração vê presença, identificador e data de troca, nunca o valor. Trocar e remover são ações explícitas. O formulário não devolve o segredo guardado.
- O Ciphera é a implementação prevista desse porto. Ele não está integrado. Enquanto a política de cache seguir aberta no plano, indisponibilidade do cofre fecha o login que depende dele. Não há fallback para valor vazio, para texto puro no SQLite nem de volta para a variável de ambiente.
- A pessoa é identificada por provedor e `subject`. E-mail, inclusive o relay privado da Apple, é contato. Contas de provedores diferentes não se fundem por e-mail.
- GitHub entra como OAuth App no fluxo authorization code, com client id e client secret. O produto só pede identidade. GitHub App fica de fora enquanto não houver ação em repositório.
- Gitea usa o mesmo fluxo OAuth2 contra a URL base informada pelo administrador. O emissor é essa URL.
- Google entra por OpenID Connect, com client id e client secret. O servidor valida emissor, audiência e nonce.
- Apple entra por Sign in with Apple na web. O segredo custodiado é a chave privada `.p8`, com Team ID, Key ID e Services ID. O client secret enviado à Apple é um JWT ES256 cunhado na hora do uso, com validade curta. A Apple recusa um `exp` maior que 15777000 segundos, cerca de seis meses. A interface não pede um JWT colado, nem um lembrete semestral, enquanto a chave estiver no cofre. Trocar a Apple é substituir essa chave. A Apple só permite baixar o `.p8` uma vez: perder a cópia do cofre exige uma chave nova no portal da Apple.
- Uma tentativa de login já iniciada permanece na revisão com que começou. Tentativas novas usam a revisão publicada. Uma troca cujo resultado se perdeu fica incompleta até reconciliação. Um segredo revogado não volta sozinho.
- Termos públicos, cobrança e workspaces continuam numa etapa posterior. Credenciais de IA, prompts e caminhos continuam fora do servidor.
- Testes usam segredo sintético e um cofre falso. Não há chamada real a esses provedores nesta decisão.

## Alternativas rejeitadas

- Manter só o GitHub, como na frase substituída do ADR 0005: a instalação pode viver em Gitea, Google ou Apple.
- Deixar a variável de ambiente como fonte permanente: o reinício desfazia a rotação feita na interface.
- Gravar o segredo em claro no mesmo SQLite do uso: uma cópia do banco entregaria todos os provedores.
- Um único campo de client secret para os quatro: na Apple a autoridade é a chave de assinatura, e o JWT é derivado.
- Fundir contas pelo e-mail: o mesmo endereço pode existir em provedores diferentes, e a Apple pode ocultá-lo.
- Implementar o Ciphera dentro deste repositório, ou impedir o porto até o Ciphera existir: o porto é o contrato. A política de cache continua uma decisão própria.
- Usar o segredo de setup como login diário quando todos os provedores estiverem desligados.

## Consequências

O código continua sem essas contas até a etapa de implementação. Cada provedor ligado exige o aplicativo correspondente criado pelo administrador, com a URL de retorno desta instalação. Gitea exige a URL base. Apple exige conta de desenvolvedor e a chave `.p8`. O cofre local, se for necessário antes do Ciphera, entra como backend explícito desse porto, com material de abertura separado do arquivo de uso. Não é uma coluna em claro escolhida na implementação.

## Quando revisitar

Um quinto provedor de login ou vínculo entre contas da mesma pessoa. A escolha do cofre ocorreu em 2026-10-30 e está no ADR 0007.
