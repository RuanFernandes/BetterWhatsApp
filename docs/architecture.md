# BetterWhatsApp — arquitetura da janela única

## Topologia

```text
Wails App
├── SystemTray
│   ├── Mostrar / ocultar
│   └── Fechar BetterWhatsApp → App.Quit()
│
└── WebviewWindow: whatsapp (frameless)
    ├── remote top document
    │   └── https://web.whatsapp.com
    │       ├── BetterWhatsApp toolbar
    │       ├── WA-JS/WPPConnect
    │       ├── CSS themes
    │       └── JavaScript plugins
    │
    └── local control iframe
        └── http://wails.localhost
            └── Vite + TypeScript → AppService Go
```

O iframe local é apenas uma superfície embutida na janela remota; não há uma segunda `WebviewWindow`. O documento do WhatsApp continua sendo o topo remoto, e a superfície local fica isolada pela política de mesma origem do navegador.

## Inicialização

1. O Go carrega ou cria `settings.json` com escrita atômica.
2. Os catálogos locais validam manifests, IDs, entrypoints e limites de tamanho.
3. O `InjectorBuilder` serializa o WA-JS, extensões habilitadas e o booleano `enabled` em um único script.
4. A única janela nativa é criada como frameless e recebe um HTML mínimo que redireciona para o WhatsApp Web.
5. O bootstrap valida o host remoto, monta a toolbar, instala o guard do bridge e aplica temas/plugins quando o injector está ativo.
6. Ao abrir `Control`, a toolbar mostra o painel local servido pelo asset server do Wails.
7. O painel chama apenas métodos allowlisted do `AppService`. Depois da persistência, ele solicita o reload do documento remoto para aplicar a configuração.

## Fechamento e tray

O hook `events.Common.WindowClosing` cancela o fechamento iniciado pelo X, Alt+F4 ou pelo binding local e chama `Hide()`. O processo só libera o fechamento quando a ação `Fechar BetterWhatsApp` do menu do tray define a flag interna e chama `App.Quit()`.

## Segurança

- O URL remoto permitido é sempre `https://web.whatsapp.com`.
- O guard do documento remoto bloqueia chamadas de serviço e mensagens nativas arbitrárias. Somente `wails:drag`, `wails:drag:doubleclick` e mensagens de resize são encaminhadas para a interação frameless.
- O serviço Go valida `application.WindowKey` contra o nome nativo `whatsapp`; o nome não é recebido como argumento controlável pelo JavaScript.
- O painel local não recebe um caminho de configuração nem dados de sessão; recebe apenas o estado agregado necessário para renderizar plugins e temas.
- O código dos plugins continua confiável e não possui sandbox/permissões por extensão nesta versão.
