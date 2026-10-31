# ADR 0019 — Templates do Sabiá nesta instalação

- Status: aceito
- Data: 2026-10-30
- Atualização 2026-10-31: o envio por POST /v1/messages está no ADR 0028. O texto abaixo registra o corte que só publicava o template.

## Contexto

O Sabiá é o gateway privado de quem administra esta instalação. O produto público não depende dele. O Sabiá já publica templates como código: `POST /v1/template-deployments/plan` e `POST /v1/template-deployments/apply`, com chave de deploy. A pessoa quer editar o template e sincronizar nesta tela, sem abrir a interface do Sabiá. Hoje só ela tem esse acesso.

## Decisão

- A configuração fica em Acesso, só para o administrador.
- A origem e o id da aplicação ficam no SQLite. A chave de deploy fica no cofre próprio, no nome `sabia.deploy`, e não volta para a tela nem para o banco.
- O template editável é um só, de e-mail, com id estável `usage-alert-v1` e chave `usage.alert`. O assunto, o HTML e o texto são editados aqui. As variáveis fixas são `provider`, `window`, `used` e `resets_at`.
- Planejar chama o plan e mostra a operação. Sincronizar chama o apply com o manifesto gravado. O apply não apaga template ausente do manifesto.
- Um `202` de envio do Sabiá, quando o envio existir, significa aceito na fila de lá, não entrega ao destinatário. Este corte publica o template. Não envia mensagem e não guarda chave de envio.
- O teste fala com um servidor local. Não chama o Sabiá real.

## Alternativas rejeitadas

- Depender do Sabiá para o produto público.
- Colar a chave de deploy no SQLite.
- Editar o template só na interface do Sabiá.
- Tratar o apply como exclusão dos templates que ficaram de fora do manifesto.

## Consequências

Quem não configura o Sabiá continua com o Web Push. A chave de deploy não serve para enviar. O envio do alerta por e-mail espera uma chave de envio e o contrato de status.

## Quando revisitar

Quando o alerta passar a ser enviado por `POST /v1/messages`, ou quando houver mais de um template editável.
