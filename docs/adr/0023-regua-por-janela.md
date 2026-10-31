# ADR 0023 — Uma régua por janela

- Status: aceito
- Data: 2026-10-30

## Contexto

O cartão desenhava uma régua vertical por provedor e um número grande para a janela principal. Claude e Codex cabem nisso. O Antigravity, ligado no setup, pode trazer mais de uma família na mesma amostra. A frase "Duas medições" da visão da pessoa, registrada no ADR 0017, conta os provedores e deixa de valer quando o terceiro cartão aparece.

## Decisão

- A visão da pessoa se chama "Medições". O operador continua em "Contas".
- Cada janela tem a própria régua horizontal, o percentual e o horário de reinício. A marca de latão continua em 80%.
- Sem janela, a régua fica vazia e o texto de ausência permanece.
- O alerta e o frescor no servidor não mudam: a projeção segue preferindo a janela de 5 horas.

## Alternativas rejeitadas

- Manter uma régua só e a lista das outras janelas em texto: a família que não é a principal some do nível.
- Continuar o título "Duas medições" e esconder o Antigravity: o coletor já foi confirmado nesta máquina.
- Reescrever o corpo do ADR 0017: o registro antigo permanece; este corte substitui só o título da pessoa.

## Consequências

Três cartões entram no quadro quando há espaço. Uma amostra com duas janelas mostra dois níveis. Quem já tem o painel aberto precisa atualizar a página para ver o JavaScript novo. O processo do rastreador não precisa reiniciar por causa desta tela.

## Quando revisitar

Se a repaginação passar a escolher outra janela como nível principal do alerta, ou se uma amostra passar de oito janelas e o cartão deixar de caber.
