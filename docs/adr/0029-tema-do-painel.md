# ADR 0029 — Tema do painel

- Status: aceito
- Data: 2026-10-31
- Relacionado: ADR 0026

## Contexto

A leitura do ADR 0026 deixou o número e a régua no lugar certo, mas a tela continuou um documento claro, com pouco corpo de produto. A pessoa pediu pesquisa, uma referência e três escolhas de tema: o do dispositivo, claro e escuro.

A busca de sistema visual da ferramenta devolveu modo escuro OLED, índigo `#4338CA`, violeta `#7C3AED`, fundo `#0F172A`, fonte Inter e o padrão de página Hero + Testimonials + CTA. Isso descreve um site de marketing, não este painel. Fica rejeitado, como o azul empresarial `#1E40AF` já tinha ficado no ADR 0026.

## Decisão

- A referência de estrutura é o redesenho do Linear de 2024: trilho lateral, densidade, tema claro e escuro tratados como duas escalas de superfície, acento só na ação, no foco e na seleção, separação por fio e não por sombra. A descrição pública está em <https://linear.app/now/how-we-redesigned-the-linear-ui>.
- A referência do campo claro é o painel da Stripe: canvas frio, superfície branca, fio fino, tinta azul-marinho e número tabular. O painel web da Stripe não nasceu com tema escuro. O escuro daqui não inverte o claro: a superfície sobe de luminância e o acento clareia, no mesmo princípio do Linear.
- O acento não é o índigo do Linear nem o violeta da Stripe. Continua verde de instrumento, porque o assunto da tela é a régua de uso.
- Não há fonte baixada de uma CDN. A família é a do sistema.
- A escolha é um único valor: `system`, `light` ou `dark`. O padrão é `system`, lido de `prefers-color-scheme`. Fica em `localStorage` na chave `ai-usage-theme`, por navegador, não por conta.
- O script no `index.html` aplica a escolha antes da pintura. O controle, com o nome visível "Tema" e as opções "Dispositivo", "Claro" e "Escuro", aparece no portão e no trilho. Não entra na barra de seções. Sair continua fora dessa barra.
- `theme-color` acompanha o tema resolvido. O manifest é estático e guarda o canvas claro.

## Alternativas rejeitadas

- Publicar o kit OLED, índigo e Inter devolvido pela busca.
- Manter o papel frio do ADR 0026 e só acrescentar um interruptor.
- Três booleanos, um para cada tema.
- Guardar a escolha no servidor ou no cofre.

## Consequências

Atualizar o painel no navegador mostra o tema novo, porque o servidor entrega os arquivos do `web/dist`. Quem já tinha o service worker antigo recebe o shell na próxima rede; a cache do shell passou a `shell-v2`. Rotas de cofre ou de envio continuam exigindo reinício do processo.

## Quando revisitar

Se a régua deixar de ser o elemento mais forte, ou se o manifest precisar de um tema dinâmico.
