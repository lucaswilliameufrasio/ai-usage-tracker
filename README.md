# Rastreador de uso de IA

Projeto para coletar localmente métricas de uso de provedores de IA e exibi-las num PWA com alertas. O agente não envia credenciais, prompts, conversas nem caminhos de arquivos ao servidor.

## Estado

Planejamento inicial. Este repositório contém o plano de desenvolvimento; ainda não contém aplicação nem agente implementados.

É o projeto prioritário. O lançamento público não depende do gestor de termos: documentos revisados externamente serão publicados como versões imutáveis no tracker, com registro de aceite e reaceite conforme política explícita. O Sabiá existente é o terceiro projeto, mas não uma dependência pública deste app.

## Plano

- [Plano de desenvolvimento individual](DEVELOPMENT_PLAN.md)

## Limite de confiança

As credenciais dos provedores de IA permanecem na máquina do usuário. Para e-mail, o produto prevê configuração BYOK e adapters diretos para Resend, MillionSend, Cloudflare Email e Amazon SES; Sabiá é uma opção apenas para instalações privadas e não é requisito para usuários públicos. Web Push tem adapter próprio do rastreador. Ciphera é um secret store cuja API retorna plaintext a clientes autorizados, não um KMS remoto. Nunca adicione tokens, `.env` real, dados de uso pessoais ou credenciais ao Git.
