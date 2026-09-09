# BetterWhatsApp — arquitetura de shell e perfis isolados

## Topologia

~~~text
Wails host process
├── SystemTray
│   ├── Mostrar / ocultar shell
│   └── Fechar BetterWhatsApp → App.Quit()
│
├── WebviewWindow: whatsapp (local, frameless)
│   └── tabs.html
│       ├── custom frame
│       ├── profile tabs
│       └── actions → AppService Go
│
├── WebviewWindow: betterwhatsapp-themes (local, frameless)
└── WebviewWindow: betterwhatsapp-plugins (local, frameless)

one child process per profile
├── WebviewWindow: whatsapp (remote, initially top-level)
│   └── https://web.whatsapp.com
│       ├── WA-JS/WPPConnect
│       ├── effective CSS themes
│       └── effective JavaScript plugins
└── WebView2 UserDataPath: one directory per profile
~~~

O shell é a única janela visível do processo host. Cada WebView remoto roda em um processo filho próprio e é reparentado como child window Win32 dentro do host, abaixo do frame HTML. Isso mantém cookies, localStorage, cache e o login separados mesmo quando duas tabs estão abertas ao mesmo tempo.

## Inicialização

1. O Go carrega ou cria `settings.json` com escrita atômica e normaliza a lista ordenada de perfis.
2. O host cria o shell local frameless e inicia primeiro o perfil ativo.
3. Para cada perfil, o host gera um token efêmero e inicia o mesmo executável com `--profile-window`, `--parent-hwnd` e `--ipc-token`.
4. O filho valida que o perfil existe, cria/usa somente o seu `WebviewUserDataPath`, constrói o injector com `BuildForProfile` e cria a janela remota frameless.
5. O host encontra a janela do PID filho, remove decoração nativa, define o shell como parent e redimensiona a área abaixo dos 92px do frame.
6. O `InjectorBuilder` resolve o override do perfil e serializa WA-JS, plugins e temas efetivos em um único script. O bootstrap não monta uma segunda toolbar no filho.
7. A tab ativa apenas mostra/foca o HWND correspondente. O estado do WhatsApp não é transferido entre perfis.
8. Ao alterar plugin, tema ou injector, a configuração é persistida e os processos de perfil são reiniciados para reconstruir os runtimes.

## Estado de plugins por perfil

O estado global continua no catálogo do plugin; `Profile.PluginOverrides` guarda apenas exceções explícitas:

~~~text
override[id] existe? ── sim → ligado/desligado definido pelo perfil
                  └── não → estado global do plugin
~~~

O resultado é calculado no host antes da injeção. O documento remoto recebe apenas o bundle já resolvido para o seu perfil. Uma extensão habilitada globalmente entra em todos os perfis que herdam o estado; uma exceção `Forçar desligado` afeta somente aquela sessão.

## IPC de baixa confiança

O filho não registra `AppService`, não recebe `service` no `RawMessageHandler` e não tem caminho para executar comandos privilegiados. Eventos de unread e nova mensagem são enviados ao host via `WM_COPYDATA`. O interceptor do host exige simultaneamente:

- tipo de evento allowlisted;
- `ProfileID` existente no host;
- token aleatório igual ao token registrado para o perfil;
- HWND remetente pertencente ao PID do processo registrado para aquele perfil.

Mensagens inválidas são ignoradas. Encerramento ou remoção de um perfil também remove seu contador da tray.

## Fechamento e tray

O hook `events.Common.WindowClosing` do shell cancela o fechamento iniciado pelo X, Alt+F4 ou pelo binding local e chama `Hide()`. O processo só libera o fechamento quando a ação `Fechar BetterWhatsApp` do menu do tray define a flag interna e chama `App.Quit()`. O host encerra os filhos antes de sair.

## Segurança

- O URL remoto permitido é sempre `https://web.whatsapp.com`.
- O guard do documento remoto bloqueia chamadas de serviço e mensagens nativas arbitrárias. Somente as mensagens de interação do frame são encaminhadas.
- O serviço Go aceita bindings apenas do shell local; o documento remoto não conhece o `WindowKey` nem recebe comandos privilegiados.
- Cada processo filho recebe apenas o injector pré-construído para o próprio perfil e uma rota de IPC de notificações.
- O diretório de dados de cada perfil é derivado e validado sob `.../BetterWhatsApp/profiles`; não é aceito caminho arbitrário vindo da WebView.
- O código dos plugins continua confiável e não possui sandbox/permissões por extensão nesta versão.
