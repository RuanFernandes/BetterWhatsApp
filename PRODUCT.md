# Product

## Register

product

## Users

Pessoas que usam o WhatsApp Web em desktop e precisam manter controle sobre
plugins JavaScript,
temas CSS e ferramentas locais. O usuário principal tem familiaridade técnica
suficiente para editar CSS e criar extensões, mas a interface deve continuar
segura e compreensível sem exigir conhecimento do backend.

## Product Purpose

BetterWhatsApp é uma shell desktop extensível para o WhatsApp Web. Ela mantém o
conteúdo remoto isolado dos serviços locais em Go e oferece uma sessão única,
plugins, temas, tray e superfícies locais de configuração. Sucesso significa que
o WhatsApp continua utilizável em qualquer tamanho de janela, enquanto as
customizações podem ser criadas, ativadas e mantidas sem travar a sessão.

## Brand Personality

Técnica, controlada e direta. A interface deve transmitir confiança de ferramenta
local, explicar claramente o escopo de cada ação e priorizar legibilidade sobre
efeitos decorativos.

## Anti-references

- Não parecer um painel experimental com texto sobreposto, controles espremidos ou
  estados que desaparecem em janelas menores.
- Não esconder ações importantes em uma sidebar vazia ou criar navegação que não
  corresponda ao fluxo real.
- Não misturar o estilo do WhatsApp remoto com o shell local a ponto de deixar
  claro quais controles pertencem a cada camada.

## Design Principles

- O conteúdo principal sempre vence: layouts devem preservar leitura e uso antes
  de encaixar controles na mesma linha.
- Escopo explícito: cada plugin e ação deve deixar claro onde será aplicado.
- Densidade progressiva: mostrar o essencial primeiro e revelar detalhes técnicos
  sem comprimir o catálogo.
- Feedback verificável: toda criação, ativação, falha e recarga deve ter estado
  visível e recuperável.
- Isolamento por padrão: ações locais nunca devem atravessar a fronteira da
  WebView remota sem uma operação explícita do host.

## Accessibility & Inclusion

O alvo é uma interface utilizável em janelas redimensionadas, com contraste
suficiente no tema escuro, foco visível, controles acionáveis por teclado,
mensagens de erro compreensíveis e suporte a `prefers-reduced-motion`. Nenhum
texto de catálogo deve depender apenas de cor ou de uma largura fixa.
