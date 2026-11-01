# ADR 0030 — Redesenho visual do painel

- Status: aceito
- Data: 2026-11-01
- Relacionado: ADR 0026, ADR 0029

## Contexto

A pessoa avaliou o painel como feio e sem cara de produto sério. O ADR 0029 deixou a leitura correta, mas a superfície continuou plana: texto solto sobre o fundo, régua fina, trilho sem identidade e um escuro azul-acinzentado claro demais.

## Decisão

- Só a camada visual muda. A marcação, as rotas, os textos e os testes ficam como estavam. O redesenho vive em `web/src/styles.css`.
- Cada provedor é um cartão com borda, sombra suave e uma faixa de estado no topo (verde, âmbar ou coral). O selo do provedor é desenhado por CSS a partir de `data-provider`, sem imagem nem fonte externa.
- Cada janela é um bloco próprio, com o percentual em corpo grande, o reinício no canto e a régua grossa com gradiente. A marca de 80% continua visível.
- O estado da leitura é uma pílula com ponto, e a cor sempre acompanha o texto.
- O trilho ganha ícones por máscara CSS, indicador da seção atual e o seletor de tema no rodapé. No celular, vira barra inferior com ícone e rótulo.
- O escuro passa a `#0d1117`, perto de preto, com superfícies em camadas. O claro mantém `#f7f9fb`. `theme-color` escuro acompanha o novo fundo.
- Sem CDN, sem biblioteca de componentes, sem fonte baixada.

## Segunda passada (2026-11-01)

Pesquisa de referências (padrões de cota e limite de uso, redesenho do Linear, dashboards modulares) e as skills `ui-ux-pro-max` e `dataviz`. O gerador de sistema visual devolveu de novo glassmorphism e Fira via CDN. Fica rejeitado pelo mesmo motivo do ADR 0029. Foi aproveitado: o medidor leva número e alvo ao lado, o estado aparece em texto e nunca só em cor, e a projeção não promete tempo exato.

- A visão ganha uma faixa de resumo: janela mais perto do limite, próximo reinício e leituras atuais. Sai de `summarize` em `view.ts`, sem pedir dado novo ao servidor. Some quando os dados vêm do cache.
- Cada janela mostra o estado em texto (Folga, Atenção, Crítico) e quanto resta, além da cor da régua. O reinício fica sob a régua.
- Entrada animada só por deslocamento, nunca por opacidade, e desligada com movimento reduzido.
- Régua sem leitura não desenha preenchimento mínimo.

## Tendência por janela (2026-11-01)

- Cada janela da visão ganha um sparkline com as últimas leituras (até 24). O front busca `/v1/samples?limit=60` (ou `/v1/account/samples`) depois que a visão carrega. O contrato do servidor não muda.
- A escala vertical é sempre de 0 a 100, então a mesma altura quer dizer o mesmo uso em toda janela. A cor segue o tom da régua. O SVG leva `role="img"` e um texto como "Tendência: de 10% a 42% em 3 leituras.".
- Limite conhecido: a amostra não traz a conta. Com mais de uma conta na visão agrupada, não há sparkline, para não misturar dados. Para ter tendência por conta, o servidor precisa expor a conta na amostra ou aceitar filtro por conta.
- A busca da tendência é secundária. Se falhar, a visão aparece como antes. Com menos de dois pontos em instantes diferentes, a janela fica sem sparkline.

## Alternativas rejeitadas

- Reescrever a marcação em componentes novos. Custo alto e nenhum ganho de leitura.
- Adotar um kit de marketing (índigo, violeta, Inter), já rejeitado nos ADRs 0026 e 0029.

## Consequências

Ícones de navegação dependem da ordem das seções (`nav a:nth-child`). Mudar a ordem exige mudar o CSS. O service worker entrega o CSS novo na próxima rede.

## Quando revisitar

Se entrar uma seção nova na navegação, ou se a leitura deixar de ser o primeiro elemento da tela.
