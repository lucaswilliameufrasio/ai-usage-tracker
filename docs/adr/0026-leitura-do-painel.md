# ADR 0026 — Leitura do painel

- Status: aceito
- Data: 2026-10-31
- Relacionado: ADR 0009, ADR 0021, ADR 0023

## Contexto

O fuso do dispositivo (ADR 0021) e a régua por janela (ADR 0023) entraram no painel, mas a tela continuou um formulário escuro com título de cartaz. A pessoa pediu um produto de verdade: intuitivo, com instrução na tela, área de administrador separada e a leitura fácil de usar. Uma busca de sistema visual devolveu o kit azul de painel empresarial. Esse kit não descreve este produto.

## Decisão

- O assunto da tela é quanto ainda resta antes do reset. O número e a régua são o elemento forte. O restante fica quieto.
- A navegação é uma coluna à esquerda no desktop e uma barra inferior no celular, com no máximo cinco seções. Sair fica fora dessa barra.
- Quem não administra vê os provedores de entrada primeiro. O token do operador fica no bloco "Sou o administrador", aberto só quando não há provedor configurado.
- Acesso coloca o passo a passo ao lado do formulário no desktop e antes dos campos no celular.
- A marca visível na régua é 80%. A cor acompanha o número e não substitui o texto.
- A paleta é papel frio, tinta e pinheiro. Sem cartão com sombra, sem fonte externa e sem caixa alta de rótulo.
- Datas continuam no fuso do dispositivo. Rotas continuam por caminho.

## Alternativas rejeitadas

- Adotar o azul `#1E40AF` e a Fira Code sugeridos pela busca genérica de painel.
- Manter o fundo verde-escuro e o latão, só trocando o tamanho do título.
- Trazer Tailwind ou uma biblioteca de componentes para parecer produto.

## Consequências

Atualizar o painel no navegador mostra a leitura nova, porque o servidor entrega os arquivos do `web/dist`. Rotas novas de cofre ou de envio continuam exigindo reinício do processo.

## Quando revisitar

Se a leitura deixar de ser a primeira coisa que a pessoa procura ao abrir o painel.
