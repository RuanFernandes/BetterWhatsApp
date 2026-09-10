# BetterWhatsApp — arquitetura da sessão única

## Topologia

~~~text
Wails host process
├── SystemTray
│   ├── Mostrar / ocultar BetterWhatsApp
│   └── Fechar BetterWhatsApp → App.Quit()
│
├── WebviewWindow: whatsapp (remota, frameless)
│   └── https://web.whatsapp.com
│       ├── frame e navbar do BetterWhatsApp injetados localmente
│       ├── WA-JS/WPPConnect local
│       ├── CSS global local
│       └── plugins JavaScript locais
│
├── WebviewWindow: betterwhatsapp-themes (local, frameless)
└── WebviewWindow: betterwhatsapp-plugins (local, frameless)
~~~

O aplicativo possui uma única sessão do WhatsApp Web. O login, cookies,
IndexedDB e cache ficam em um único `WebviewUserDataPath`; não existe mais
criação, seleção, alternância ou processo filho por perfil.

## Inicialização

1. O Go carrega `settings.json` com escrita atômica e garante os catálogos
   globais de plugins e temas.
2. O host cria uma única `WebviewWindow` remota, frameless, apontando para
   `https://web.whatsapp.com`.
3. O injector local serializa WA-JS, plugins habilitados, CSS habilitado e o
   estado do painel em um script inicial.
4. O bootstrap monta a frame/navbar do BetterWhatsApp dentro da própria página
   remota e encaminha apenas comandos allowlisted para o host.
5. As telas de Themes e Plugins são janelas locais separadas, também
   frameless, e usam os bindings Go somente para operações de configuração.

Alterações de plugins, temas ou do injector são persistidas e recarregam a
mesma sessão. Nenhuma operação cria outra WebView do WhatsApp.

## Separação de segurança

- O URL remoto permitido é sempre `https://web.whatsapp.com`.
- A página remota não recebe os bindings de serviço diretamente: o bootstrap
  protege `window._wails` e aceita somente mensagens nativas allowlisted.
- O serviço Go valida que chamadas de configuração vêm de uma janela local do
  BetterWhatsApp (`whatsapp` ou uma superfície `betterwhatsapp-*`).
- A navbar remota só publica comandos de janela, reload, tray, unread e ações
  de configuração previamente validadas pelo host.
- Plugins e temas são carregados de arquivos locais; o plugin continua sendo
  código confiável e não possui sandbox de permissões nesta versão.

## Fechamento e tray

O hook de fechamento cancela X, Alt+F4 e o comando de fechar da frame e oculta
a janela. O processo só encerra quando o usuário escolhe `Fechar
BetterWhatsApp` no menu de contexto da tray. A segunda abertura do executável é
tratada pelo `SingleInstance` do Wails e apenas restaura/foca a janela atual.

## Migração

Os campos antigos de perfil ainda são lidos internamente apenas para manter a
sessão legada existente durante a migração. Eles não são expostos no estado,
nas bindings, no frontend nem no runtime, e não podem criar uma nova seção.
