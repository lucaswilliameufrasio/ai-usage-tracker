# Plano de desenvolvimento — Rastreador de uso de IA

**Status:** MVP pessoal validado em 2026-10-30. Conta, sessão e retorno do login estão no código, em `docs/adr/0008-sessao-e-retorno-do-login.md`. O template do Sabiá se edita em Acesso (ADR 0019). A atualização do agente é opt-in (ADR 0020). As datas do PWA usam o fuso do dispositivo (ADR 0021). O Antigravity, quando ligado no setup, é lido pelo comando `agy` (ADR 0022). A pessoa confirmou esse coletor nesta máquina. Cada janela do cartão e do histórico tem a própria régua (ADR 0023). OpenCode e Grok continuam sem coletor. E-mail BYOK da instalação e os cofres externos estão decididos e sem código. Termos públicos e cobrança continuam fora. A repaginação do restante da interface continua aberta; o fuso e a régua por janela são os dois primeiros itens dela.

**Nome do repositório:** funcional/provisório.

**Escopo:** plano autocontido deste repositório; não requer os documentos de planejamento locais de outros projetos.

## Objetivo e limites

Permitir que uma pessoa veja o consumo e os resets dos seus provedores de IA sem entregar as credenciais deles ao servidor. O agente coleta localmente; o servidor recebe apenas métricas normalizadas; o PWA mostra histórico, projeções e alertas.

### Prioridade e dependências — decisão de 2026-10-29

Este é o projeto prioritário: validar a utilidade pessoal e os coletores Claude/Codex antes de expandir. O `terms-policy-manager` não é pré-requisito para lançamento público: redigir/revisar documentos externamente e começar com publicação versionada e aceites no próprio tracker. O terceiro projeto é o Sabiá existente, não um novo gateway; ele continua opcional para instalações privadas, preservando os adapters BYOK públicos deste app.

### Fora do MVP pessoal

- Cadastro público, cobrança, workspaces, login social e limites por plano.
- App nativo: começar com PWA; validar sua experiência em iOS/Android antes de considerar outro cliente.
- Serviço genérico de notificações separado. O app mantém a orquestração de alertas e os adapters de entrega necessários; e-mail usa provedores configurados pelo próprio operador/usuário (BYOK). O Sabiá pode ser uma opção em instalações privadas, mas não é dependência do produto público.
- Acesso do servidor aos provedores de IA ou coleta remota de cookies/tokens; isso não impede envio server-side aos provedores de e-mail configurados.
- Modo E2E dos dados de uso, auto-update do agente e suporte a todos os provedores.
- Base comum como produto/repositório separado; reconsiderar após uso real por mais de um app.

## Atores e jornadas

### Usuário e agente local

1. O usuário instala o agente por um script publicado na GitHub Release deste repositório público e autoriza explicitamente os coletores para Claude e Codex.
2. O agente lê as fontes locais suportadas, normaliza janelas/limites/reset e valida os dados.
3. Sem rede, mantém uma fila local limitada; ao reconectar, envia amostras idempotentes e remove apenas as confirmadas.
4. O usuário revoga um dispositivo/token; envios seguintes são recusados e a fila não tenta reativar a credencial revogada.

### Usuário do PWA

1. Enrola um dispositivo com um segredo de uso único, mostrado uma vez.
2. Consulta cards por provedor, histórico e previsão; estados sem dados, fonte desatualizada e provedor indisponível são distintos.
3. Configura limites/horário silencioso e escolhe canais habilitados.
4. Recebe Web Push quando suportado; erro/expiração da inscrição é visível e recuperável. No iOS, o PWA precisa estar instalado na tela inicial e as entregas podem atrasar; não prometer alerta crítico. Telegram/e-mail ficam para uma fase posterior.

### Operador

Na fase pessoal, é o próprio usuário: configura backup e inspeciona saúde sem visualizar ou registrar credenciais dos provedores. Administração multiusuário só entra após uma decisão de abertura pública.

## Dados, contratos e fronteiras

- **Agente:** coletores isolados por provedor, normalização, spool offline limitado e envio autenticado. Testar processos/CLIs externos com timeout, orçamento e cancelamento; credenciais ficam no mecanismo local do provedor.
- **API de ingestão:** aceita apenas `provider`, janela, usado, limite, reset, plano e timestamp validados; autentica dispositivo revogável, aplica limite/tamanho, deduplica por ID da amostra e não aceita campos livres arbitrários.
- **Persistência:** SQLite é uma proposta do plano original, ainda sujeita a decisão técnica. Toda consulta de conta deve carregar ownership explícito antes de existir multi-tenant.
- **Projeção:** calcula previsão a partir de amostras recentes, marcando ausência/atraso de dados; não inventa leitura atual do provedor.
- **Alertas:** o rastreador decide quando notificar (limiar/reset/horário silencioso); entrega é por adapter e não bloqueia ingestão. Para Web Push, fila limitada, idempotência, retry/backoff, expiração e remoção de inscrição morta.
- **PWA:** leitura do último estado sincronizado offline; mostrar quando os dados foram coletados e não tratar cache como dado atual.
- **Observabilidade:** logs sem tokens, cookies, payloads sensíveis ou identificadores desnecessários; métricas de saúde e atraso sem conteúdo do usuário.

## Entrega de alertas — e-mail BYOK e Web Push

O produto público não pode depender do Sabiá, um gateway de acesso privado. Cada instalação/usuário precisa poder escolher e configurar o próprio provedor de e-mail, sem exigir conta ou credenciais do Sabiá. O Sabiá pode ser oferecido como adapter opcional em instalações privadas, nunca como rota obrigatória ou fallback implícito.

- Implementar no rastreador uma interface de envio de e-mail e adapters diretos para **Resend**, **MillionSend**, **Cloudflare Email (Send Email)** e **Amazon SES**. O usuário escolhe o adapter e informa suas credenciais e identidade remetente; validar a configuração antes de ativá-la.
- Correção da inspeção em 2026-10-29: a `main` do Sabiá já despacha MillionSend via `sendResendEmail` com base URL própria, apesar de não haver arquivo de adapter separado. Usar modo compatível e validar endpoint, autenticação, limites e respostas com a documentação oficial antes de considerar equivalência completa; isso não torna o Sabiá uma dependência deste app.
- A consulta anterior encontrou adapters Resend e Cloudflare no Sabiá, mas não um adapter SES. SES deve ser implementado/qualificado diretamente neste app; não presumir que existe código reutilizável nem que o Sabiá o suporta atualmente.
- Guardar credenciais por referência a secret store quando disponível; sem isso, definir antes da implementação um mecanismo de armazenamento cifrado e sua rotação. Nunca devolver valores secretos à UI depois da gravação; oferecer manter/substituir/remover, redigir logs e não incluir credenciais em auditoria, traces ou fixtures.
- Configurar e validar um provedor não significa que ele esteja ativo: publicar uma configuração validada, mostrar o estado efetivamente adotado pelo worker e falhar de modo explícito se não houver provedor válido. Não trocar silenciosamente para outro provedor em caso de erro. O escopo da configuração (instalação privada ou workspace/conta no produto público) deve ser fechado antes de habilitar multiusuário.
- Implementar Web Push como adapter próprio e estreito do rastreador para o PWA. Não criar API de tópicos, compatibilidade ntfy, broker genérico ou serviço de notificações separado. O Sabiá é opcional apenas para quem já tem acesso privado e escolhe explicitamente essa rota.
- Se uma instalação privada escolher o Sabiá, tratar `202` como `accepted/enqueued`, nunca como “entregue”; confirmar entrega somente pelo contrato de status que estiver autorizado e disponível.
- `event-routing-platform` roteia eventos de analytics e `alert-manager` trata alertas de monitoramento; nenhum substitui a jornada de consumo de IA nem os adapters de e-mail próprios deste app.

## Ciphera — integração como secret store (não KMS)

O ADR 0007 coloca o Ciphera como um backend opcional do porto de segredo, ao lado do cofre próprio, do HashiCorp Vault (KV), do AWS Secrets Manager e do Infisical. O contrato desta seção continua valendo para o adapter Ciphera. Uma instalação não precisa dele para subir.

Ciphera é uma integração opcional de gestão de segredos com envelope encryption interno, KEKs configuradas no servidor e DEKs por projeto. O contrato avaliado lê segredos autorizados e devolve o valor plaintext ao cliente; não fornece operações remotas `wrap/unwrap` nem API KMS/HSM documentadas. Ciphera protege segredos que armazena, não cifra dados arbitrários em nome deste app. A instalação pública não deve exigir acesso a um serviço privado: manter o contrato de armazenamento seguro explícito e validar a disponibilidade da integração antes de habilitá-la.

### Integração proposta

- Implementar cliente HTTP Go tipado `CipheraSecretStore` contra operações documentadas de leitura (por exemplo `GET /v1/projects/{project_id}/secrets/{environment}`), com TLS, timeout, limite de resposta, erros redigidos e mapeamento explícito de `401/403/404/429/5xx`.
- Se o MVP precisar ler segredos server-side do Ciphera, definir service token read-only escopado a projeto/ambiente/prefixos mínimos, expiração/rotação/revogação e injeção por secret store do host. Não provisionar nem solicitar credenciais até haver ambiente autorizado; se não houver segredos dependentes, manter Ciphera fora do MVP.
- Ciphera retorna valores em plaintext ao cliente autorizado. Manter segredos necessários apenas em memória pelo TTL decidido; não copiá-los para SQLite, traces ou logs. Se indisponível e o cache expirou, falhar fechado nas funções dependentes; nunca usar valor vazio/default ou fallback em claro.
- Se a aplicação decidir cifrar dados próprios em SQLite, uma chave buscada como secret no Ciphera ainda será entregue plaintext ao processo: isso pode proteger contra cópia isolada do DB, não contra comprometimento do processo/host. Definir AAD, escopo, cache, backup/recuperação e rotação da chave da aplicação em decisão separada; não chamar isso de cifra remota/KMS.
- Não usar `ciphera run` como isolamento: ele injeta plaintext no ambiente do processo filho. O backend deve ser tratado como workload autorizado que consegue ler os segredos que busca.
- Desenvolvimento local usa fake HTTP de protocolo e dados sintéticos; teste de integração contra instância autorizada somente como operação separada, nunca contra produção em suíte local. Cobrir auth negada, timeout, resposta inválida, revogação/expiração, cache e indisponibilidade sem fallback.

### Dados que não devem ir ao Ciphera por padrão

Tokens/cookies dos provedores de IA ficam na máquina e nunca chegam ao backend. Ciphera não participa da coleta nem recebe prompts ou métricas. Pode armazenar segredos de servidor (por exemplo VAPID ou credenciais de provider) se houver necessidade operacional; a API devolverá esses valores ao backend.

## Fases e critérios observáveis

### Fase 0 — validar domínio, fontes e fronteiras

- Fechar o contrato tipado de configuração/adapters de e-mail; validar documentação oficial e usar mocks/fixtures sintéticas. Exercitar envio somente em sandbox ou conta de teste autorizada, nunca contra produção em suíte local. Sabiá permanece opcional para instalações privadas.
- Decidir o adapter Web Push do MVP sem criar um gateway genérico.
- Validar formato disponível no CodexBar/CLIs e permissões/termos dos provedores; gravar fixtures sintéticas, nunca credenciais reais.
- Se houver segredos server-side no MVP, validar o desenho de token Ciphera read-only por projeto/ambiente/prefixo em ambiente autorizado; caso contrário, declarar integração fora do MVP. Não tratar como KMS nem inventar wrap/unwrap.
- Fechar o contrato da amostra, revogação do dispositivo, deduplicação e retenção.

**Pronto quando:** limites de dados são verificáveis por contrato/teste; escolha de Web Push e fronteira BYOK/Sabiá documentadas; token Ciphera tem escopo mínimo e fonte segura, ou funções dependentes de segredos estão explicitamente fora do MVP.

### Fase 1 — MVP pessoal de ponta a ponta

- Agente para macOS/Linux com coletores Claude e Codex; nenhuma credencial de provedor de IA no backend.
- API single-user, enrolamento/revogação de dispositivos, SQLite/histórico e ingestão idempotente.
- PWA instalável com cartões, timestamp de atualização, estado stale/offline e Web Push para reset/limiar básico pelo adapter estreito do rastreador.
- Setup e health check; configuração local de segredos somente por referências seguras para desenvolvimento/execução.

**Pronto quando:** fixtures sem dados reais provam a normalização; teste de integração confirma que payloads excluem prompts, tokens e caminhos; amostra duplicada não duplica histórico; dispositivo revogado recebe negação; usuário usa no celular por duas semanas e consegue distinguir dado atual de desatualizado.

### Fase 2 — confiabilidade e operação

- Testar backup e restauração isolada do SQLite, inclusive integridade e contagens.
- Fila limitada dos adapters de entrega com retry/backoff, idempotência, expiração e remoção de inscrições Web Push mortas; implementar e qualificar envio BYOK direto por Resend, MillionSend, Cloudflare Email e Amazon SES. O adapter Sabiá, se mantido, é opcional e específico para instalações privadas.
- Configuração de e-mail validada e versionada; troca de credencial substitui o cliente sem misturar snapshots, e revogação/remoção não mantém credenciais antigas ativas. Sem provedor válido, e-mail fica explicitamente indisponível.
- Política de upgrade/migration compatível, shutdown/drain e recuperação da fila local do agente.
- Publicar o binário do agente numa GitHub Release. O script de instalação é um arquivo dessa release, como em acari e tailmux: `curl` baixa o script em `releases/latest/download`, e o script baixa o pacote e o SHA256 da mesma release antes de colocar o binário em `~/.local/bin`. O repositório é público, então a release pode servir o script. O servidor do painel continua self-hosted e não entra nesse script. A atualização do agente é opt-in (ADR 0020). O script não procura outra release. A tag ainda não foi publicada.
- Integrar leitura de segredos via Ciphera somente após validar credenciais, escopo e política de cache.

**Pronto quando:** drill restaura snapshot em ambiente isolado; indisponibilidade de Ciphera/push/e-mail não causa fallback de provedor silencioso, exposição de credencial ou perda silenciosa; testes de contrato sintéticos cobrem cada adapter habilitado; shutdown não confirma envio ainda não persistido.

### Fase 3 — mais provedores

Adicionar Cursor, Gemini, Copilot e outros a partir de demanda, cada um com contrato isolado, testes contra respostas gravadas/sintéticas, limites legais e degradação independente.

**Pronto quando:** cada coletor tem fixtures versionadas, cobertura de resposta ausente/malformada e não impede coletores saudáveis.

### Fase 4 — abertura multiusuário

Após preparar termos e política revisados externamente: login, workspaces/roles, isolamento por workspace, publicação versionada local e aceite explícito, exportação/exclusão LGPD, limites antiabuso e página de status. Não esperar pela implementação do gestor de termos.

- Publicar snapshot imutável do texto com identificador/hash, vigência e URL fixa por versão, além da URL vigente; preservar histórico e changelog.
- Registrar aceite com usuário autenticado, versão exata e instante, de forma idempotente; recuperar o texto associado ao registro e permitir exportação conforme retenção definida. Não usar apenas um booleano de aceite nem confundir hash com prova de identidade/validade jurídica.
- Separar publicação, vigência e aceite. Mudanças que exigem reaceite geram pendência sem reescrever o histórico; política de reaceite e eventual restrição de acesso exigem decisão explícita/revisão jurídica.
- Manter o tracker como única fonte de verdade dos aceites nesta etapa. Avaliar migração para o gestor somente com demanda de múltiplos apps/necessidade operacional, contrato estável, reconciliação e validação dos registros históricos.

**Pronto quando:** testes de autorização impedem leitura cruzada entre workspaces; usuário consulta o texto versionado e registra aceite explícito; duplicatas não duplicam registros; nova vigência exige reaceite quando previsto na política sem alterar aceites anteriores; exportação recupera registro e texto exatos; exclusão/retenção em backups têm evidência; restauração e rotação de chaves foram exercitadas. A jornada funciona sem gestor de termos ou Sabiá.

### Fase 5 — avaliação de extrações

Não extrair um serviço de notificações: a decisão de alertar e os adapters requeridos permanecem neste app, com BYOK direto para e-mail e Sabiá somente como opção privada. Reavaliar tópicos/ntfy/UnifiedPush apenas com demanda independente e consumidores reais. Considerar modo E2E/widget somente com necessidade demonstrada.

## Verificação e riscos

- Unit: parsing/normalização, projeção, políticas de alerta, transições de token e revogação; AAD/crypto apenas se uma decisão futura aprovar cifra própria dos dados do app.
- Integração local: API/router + SQLite real, idempotência, fila limitada e cliente Ciphera contra fake de protocolo.
- E2E: setup, envio por agente de teste, leitura no PWA e registro de push/efeito persistido; não usar apenas mocks para provar a jornada.
- Regressões de privacidade: payload e logs sem credenciais/prompt/caminho; isolamento entre contas antes de habilitar multi-tenant.
- Riscos principais: mudanças/termos dos provedores de IA e e-mail, fontes de dados frágeis, atraso do Web Push no iOS e indisponibilidade/revogação do Ciphera. Cada coletor e adapter deve falhar isoladamente; uma credencial de provider pertence ao escopo de configuração definido e não pode vazar entre contas/workspaces.

## Decisões ainda abertas

- Qualificação concreta de MillionSend, Cloudflare Email e Amazon SES. O e-mail BYOK é da instalação e ainda não tem código.
- O estado final de entrega quando uma mensagem sair pelo Sabiá. O template já sincroniza por plan/apply (ADR 0019) e este corte não envia.
- Retenção além de manter o histórico, termos do lançamento público, cobrança e a repaginação visual de UI, UX e DX. O fuso do dispositivo já está no ADR 0021.
- A leitura ao vivo do Codex ficou no ADR 0015: comando local `codex app-server`, sem o aplicativo CodexBar e sem `auth.json`. O Antigravity usa `agy -p /usage` (ADR 0022). OpenCode e Grok Build ainda não têm comando local de percentual e reset que deixe o login com o CLI.
- Não há cache de segredo. Com o backend indisponível, a função que depende dele fecha (ADR 0007). Web Push é o adapter estreito deste app (ADR 0004).

### Ponteiro de 2026-10-29

A fatia 1 fechou, para o uso pessoal, a stack, as fontes locais, o contrato da amostra e o Web Push estreito. Ver `docs/adr/0001-stack-do-mvp-pessoal.md`, `docs/adr/0002-fontes-locais-claude-codex.md`, `docs/adr/0003-amostra-dispositivo-e-historico.md`, `docs/adr/0004-web-push-estreito.md` e `docs/source-inspection.md`.

Continuam abertas: escopo BYOK de e-mail, detalhes de MillionSend, Cloudflare Email e SES, o estado final de entrega quando uma mensagem sair pelo Sabiá, cache do backend ativo de segredos, retenção além de manter o histórico, termos do lançamento público, cobrança e a repaginação de UI, UX e DX. O aplicativo CodexBar não é fonte. A leitura ao vivo usa o `codex app-server` local (ADR 0015). O porto e o cofre próprio estão implementados (ADR 0007). Ciphera, HashiCorp Vault, AWS Secrets Manager e Infisical continuam sem adapter. O retorno dos quatro provedores e a sessão da pessoa estão implementados (ADR 0008).

### Validação pessoal — 2026-10-30

Quem usou o produto encerrou o MVP pessoal. O critério de duas semanas no celular não bloqueia mais o avanço. O ADR 0005 separa o administrador das outras pessoas. Os logins configuráveis estão no ADR 0006 e os cofres no ADR 0007. O aplicativo tem conta por pessoa, sessão por cookie e a URL de retorno `/v1/login/{provider}/callback` (ADR 0008). As telas do PWA usam pathname, e o mesmo processo devolve o shell (ADR 0009). O administrador continua em Acesso. O backup do SQLite grava um snapshot e a restauração só ocorre num diretório sem banco (ADR 0010). A recuperação de desastre fica no sqlite-replication, sem download do banco pela tela (ADR 0011). O ensaio isolado passou: snapshot LTX e restore num banco temporário, com a origem seguindo depois e a cópia parada nas duas amostras. O `Open` passa a pedir WAL. O processo que já está no ar conserva o journal anterior até o próximo arranque. O PWA valida o JSON com Valibot. A fila de Web Push cabe em 32 pendentes, manda até 8 por passagem e espera 1, 2, 4 e 8 minutos antes de tentar de novo (ADR 0013). A quinta falha, o 404/410 e o prazo de 24 horas ficam visíveis na tela de alertas. A inscrição morta permanece. O esquema sobe para user_version 4 no próximo arranque deste código. O e-mail BYOK continua fora, e o escopo fechado para quando ele entrar é o da instalação. Os cofres externos ainda não entram. `go run ./cmd/agent run` sem flags abre o setup no terminal e grava a escolha (ADR 0014). Os flags continuam o caminho de script. A pasta do Codex é lida com as subpastas de data; sessão sem `rate_limits` não vira amostra nova. Com o Codex configurado, o envio tenta antes o `codex app-server` local (ADR 0015). Se essa leitura falha, a observação do arquivo permanece. A pessoa confirmou essa leitura nesta máquina: o terminal mostrou `codex lido pelo comando local`, com uma amostra aceita e uma duplicada. Sem flags e com a configuração pronta, `go run ./cmd/agent run` repete a cada 5 minutos até Ctrl+C ou SIGTERM. O sinal espera o envio atual, no máximo 40 segundos, e não começa outra coleta. A fila local só esquece o item depois da confirmação do servidor (ADR 0016). O `run` com flags e o fim do setup continuam um envio só. A visão do operador passa a agrupar por conta da pessoa: o rótulo é o contato do login, dispositivo antigo fica em "Sem conta", e a pessoa continua vendo só os próprios provedores (ADR 0017). Histórico e dispositivos do operador seguem da instalação inteira. OpenCode, Antigravity e Grok Build foram inspecionados por nome e documentação; não há contrato local de percentual e reset, então não há coletor para eles. O guia do Grok descreve telemetria externa de tokens, desligada por padrão, e não um arquivo de janela. A instalação do agente está em `scripts/agent-installer.sh`, `scripts/package-agent.sh` e `.github/workflows/release.yml` (ADR 0018). A tag `vX.Y.Z` compila só o agente para Linux e macOS, em amd64 e arm64, e publica o script, os pacotes e `checksums.txt`. O teste sobe uma release local e não chama o GitHub. Nenhuma tag foi publicada, então o endereço `releases/latest/download` ainda não funciona. O painel não entra nesse script. A atualização do agente é opt-in (ADR 0020): o setup pergunta, Enter deixa desligada, e o binário `dev` não consulta a rede. Com a opção ligada, cada passagem confere a release estável, o SHA256 e substitui o executável antes do envio. Uma falha nessa conferência não cancela o envio. O esquema deste corte sobe para user_version 5. Em Acesso, o administrador edita um template de e-mail e sincroniza com o Sabiá por plan/apply (ADR 0019). A chave de deploy fica no cofre próprio, no nome `sabia.deploy`, e não volta para a tela. Esse corte não envia mensagem e não grava chave de envio. O PWA e o texto do setup mostram datas no fuso do dispositivo; a API continua em UTC (ADR 0021). A repaginação de UI, UX e DX fica em aberto, e o fuso é o primeiro item dela.

E-mail BYOK da instalação, pelos provedores Resend, MillionSend, Cloudflare Email e Amazon SES, está decidido e sem código. Os adapters Ciphera, HashiCorp Vault no KV, AWS Secrets Manager e Infisical também. O cofre em uso é o próprio.

O Sabiá não saiu do produto. O aplicativo público não depende dele. O adapter de template está em Acesso para esta instalação (ADR 0019). Ele não envia mensagem. Um 202 desse gateway, quando o envio existir, é item aceito na fila de lá, não entrega ao destinatário.

OpenCode e Grok Build não foram recusados como produto. A inspeção de 2026-10-30 não achou, neles, um arquivo local de percentual da janela e de horário de reset. O comando local que existe devolve tokens e custo da sessão. A janela do plano, nas outras barras, sai de ler o login e chamar o provedor. Sem um comando em que o CLI guarde o login, o coletor não inventa a amostra. O Antigravity passou a ter esse comando e está no ADR 0022.
