# BetterWhatsApp

![Plataforma](https://img.shields.io/badge/plataforma-Windows%20%7C%20WebView2-0f6d3d?style=flat-square)
![Go](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat-square&logo=go&logoColor=white)
![Wails](https://img.shields.io/badge/Wails-3%20alpha-50e58b?style=flat-square)
![Frontend](https://img.shields.io/badge/frontend-TypeScript%20%2B%20Vite-3178C6?style=flat-square&logo=typescript&logoColor=white)
[![Windows installer](https://github.com/RuanFernandes/BetterWhatsApp/actions/workflows/windows-installer.yml/badge.svg)](https://github.com/RuanFernandes/BetterWhatsApp/actions/workflows/windows-installer.yml)

Uma shell desktop extensível para o WhatsApp Web, construída com Wails 3, Go, TypeScript, Vite e WebView2.

O BetterWhatsApp adiciona uma camada local de controle sobre o WhatsApp Web: frame próprio, tray, uma sessão única, temas CSS globais, plugins JavaScript, editor Monaco e uma separação explícita entre conteúdo remoto e serviços nativos.

> **Status:** experimental. É um laboratório funcional e ainda não deve ser tratado como software estável para uso crítico.

O projeto não é afiliado, patrocinado ou endossado pelo WhatsApp, Meta ou WPPConnect.

## Visão geral

O WhatsApp Web roda em uma única WebView2 remota, enquanto a coordenação sensível fica no host local em Go. Login, cookies, cache e IndexedDB pertencem a uma sessão única e estável.

## Funcionalidades

- Shell local frameless com frame desenhado no frontend.
- WhatsApp Web carregado diretamente de https://web.whatsapp.com.
- Uma única sessão persistente do WhatsApp Web, sem processos filhos ou tabs de perfis.
- Tray com mostrar/ocultar e fechamento definitivo pelo menu de contexto.
- Ícone alternativo e som para novas mensagens quando o app está oculto.
- Contagem de mensagens não lidas sem incluir chats arquivados.
- WA-JS/WPPConnect empacotado localmente, sem download de JavaScript na inicialização.
- Plugins JavaScript com estado global.
- Temas CSS globais editados com Monaco.
- Superfícies dedicadas para Control, Plugins e Themes.
- Documentação da API de plugins dentro da aplicação.
- Abertura do projeto de um plugin diretamente no VS Code.
- Instância única do host e mensagens nativas allowlisted entre a WebView remota e o host.

## Arquitetura

~~~text
BetterWhatsApp
├── Host Wails + Go
│   ├── shell frameless
│   ├── frame, navbar e ações locais
│   ├── configuração persistida atomicamente
│   ├── tray e notificações
│   └── AppService com métodos allowlisted
│
├── WebView2 UserDataPath único
│   └── WhatsApp Web + WA-JS + plugins + temas
│
└── Superfícies locais frameless
    ├── themes.html
    └── plugins.html
~~~

O shell local não entrega bindings Go ao documento remoto. A página do WhatsApp recebe somente o runtime necessário para WA-JS, plugins, temas e notificações.

Veja a topologia completa em [docs/architecture.md](docs/architecture.md).

### Sessão única

O injector é construído uma vez por inicialização e aplicado à única WebView do
WhatsApp. Plugins e temas habilitados são globais para essa sessão; nenhuma
ação do frontend cria ou alterna outra seção de login.

### Fronteira de segurança

- O URL remoto é limitado a https://web.whatsapp.com.
- O documento remoto não recebe acesso direto a arquivos, configurações ou comandos arbitrários do Go.
- O AppService valida a janela chamadora antes de executar operações locais.
- Caminhos de plugins e temas são normalizados dentro dos diretórios permitidos.
- Plugins são código confiável e podem manipular o DOM e usar WPPConnect na sessão. Esta versão ainda não fornece sandbox completa para extensões.

## Requisitos

O alvo principal atual é o Windows:

- Windows com WebView2 Runtime.
- Go 1.25 ou compatível com go.mod.
- Node.js 20 ou superior.
- Wails 3 CLI.
- Git.

O frame frameless e a integração com a tray usam APIs nativas do Windows nesta versão.

## Desenvolvimento

~~~powershell
git clone https://github.com/RuanFernandes/BetterWhatsApp.git
cd BetterWhatsApp

npm install
npm run prepare:wa-js
npm --prefix frontend install
wails3 generate bindings -ts -i -clean=true
npm --prefix frontend run build
wails3 dev
~~~

O script prepare:wa-js copia o runtime do pacote instalado para assets/injector/wa-js.js. O bundle resultante pode ser auditado antes da execução.

## Build

~~~powershell
wails3 build
~~~

O executável Windows é gerado em bin/betterwhatsapp.exe.

Para gerar o instalador NSIS localmente:

~~~powershell
wails3 package GOOS=windows GOARCH=amd64
~~~

Cada push na branch `main` executa automaticamente o workflow Windows installer. O instalador fica disponível como artifact da execução e também é publicado como uma GitHub Release de prévia (`build-<número-da-execução>`), com o `.exe` anexado na aba **Releases**.

## Atualizações

O aplicativo consulta as Releases públicas do GitHub, sem exigir uma API própria ou chave de acesso. Quando uma Release Windows mais nova é encontrada, o instalador é baixado automaticamente para a pasta privada de dados do BetterWhatsApp. O app não executa o instalador sozinho: a instalação continua sendo uma ação explícita do usuário.

Validação local:

~~~powershell
npx --prefix frontend --no-install tsc --noEmit
go test ./...
go vet ./...
wails3 build
~~~

## Criando um plugin

Plugins são diretórios com manifest.json e o arquivo JavaScript declarado em entry.

Manifesto:

~~~json
{
  "id": "meu-plugin",
  "name": "Meu plugin",
  "version": "0.1.0",
  "description": "Exemplo de extensão do BetterWhatsApp.",
  "author": "Seu nome",
  "entry": "index.js"
}
~~~

Entrypoint:

~~~js
BetterWhatsApp.registerPlugin("meu-plugin", ({ addStyle, observe, log }) => {
  addStyle(
    "meu-plugin-style",
    [
      "[data-better-whatsapp='meu-plugin'] {",
      "  color: #50e58b;",
      "  font-weight: 700;",
      "}",
    ].join("\n"),
  );

  observe("[contenteditable='true']", (composer) => {
    if (composer.parentElement?.querySelector("[data-better-whatsapp='meu-plugin']")) {
      return;
    }

    const button = document.createElement("button");
    button.type = "button";
    button.dataset.betterWhatsapp = "meu-plugin";
    button.textContent = "BW";
    button.title = "Ação do meu plugin";
    button.addEventListener("click", () => {
      composer.focus();
      log("ação executada");
    });

    composer.parentElement?.append(button);
  });

  log("meu-plugin carregado");
});
~~~

APIs disponíveis no factory:

| API | Uso |
| --- | --- |
| BetterWhatsApp.registerPlugin(id, factory) | Registra a extensão |
| addStyle(id, css) | Injeta ou atualiza CSS |
| observe(selector, callback) | Observa elementos criados pelo WhatsApp Web |
| log(message, error?) | Registra diagnóstico no console |
| WPP | Acesso ao WA-JS/WPPConnect quando disponível |

O exemplo incluído está em [assets/plugins/quick-reactions](assets/plugins/quick-reactions).

Plugins do usuário ficam em:

~~~text
%AppData%\\BetterWhatsApp\\plugins\\<id>\\
├── manifest.json
└── index.js
~~~

Revise o código de qualquer extensão antes de instalá-la.

Também é possível criar um projeto inicial pela superfície **Plugins**: clique
em **Novo template**, informe o nome e o BetterWhatsApp cria a pasta completa
em `%AppData%\BetterWhatsApp\plugins\<id>`, com manifesto, `index.js` e
`README.md`. O plugin entra desligado para que você possa revisar o código
antes de ativá-lo.

## Criando temas CSS

Os temas são arquivos CSS globais gerenciados pela tela Themes. O editor Monaco fornece destaque e autocomplete para CSS.

~~~css
/* Exemplo: reduzir o contraste das conversas */
[data-testid="conversation-panel-wrapper"] {
  background: #0b120f !important;
}

[data-testid="conversation-panel-body"] {
  background: #07100b !important;
}
~~~

Temas do usuário ficam em:

~~~text
%AppData%\\BetterWhatsApp\\themes\\<id>\\
├── manifest.json
└── theme.css
~~~

Como os seletores do WhatsApp Web podem mudar, temas baseados em classes internas podem precisar de manutenção.

## Estrutura

~~~text
.
├── assets/
│   ├── branding/              # Logo e ícones
│   ├── injector/              # Bootstrap, WA-JS e licença
│   └── plugins/               # Plugins distribuídos com o app
├── frontend/
│   ├── src/main.ts            # Superfície principal
│   ├── src/standalone.ts      # Themes e Plugins
│   └── public/style.css       # Design system local
├── internal/
│   ├── appservice/            # Serviços e validações
│   ├── config/                # Configuração persistida
│   ├── desktop/               # Wails, tray, IPC e embed Win32
│   ├── extensions/            # Catálogo de extensões
│   ├── injector/              # Montagem da sessão única
│   ├── model/                 # Modelos de estado
│   ├── plugins/               # Gerenciamento de plugins
│   └── themes/                # Gerenciamento de temas
├── docs/architecture.md       # Decisões e topologia
├── main.go                    # Bootstrap do host
└── Taskfile.yml               # Tarefas Wails
~~~

## Contribuindo

Contribuições são especialmente úteis em compatibilidade com novas versões do WhatsApp Web, acessibilidade, responsividade, testes de isolamento, plugins revisáveis e observabilidade.

### Fluxo recomendado

1. Abra uma issue com ambiente e passos para reproduzir.
2. Crie uma branch curta:

   ~~~powershell
   git checkout -b fix/nome-do-problema
   ~~~

3. Faça a menor mudança coerente com a arquitetura.
4. Atualize a documentação quando alterar comportamento público.
5. Execute:

   ~~~powershell
   npx --prefix frontend --no-install tsc --noEmit
   go test ./...
   go vet ./...
   wails3 build
   ~~~

6. Abra um pull request explicando problema, solução e validação.

### Checklist de pull request

- [ ] Nenhum binding Go foi exposto ao documento remoto.
- [ ] Entradas vindas do frontend continuam validadas no serviço.
- [ ] O isolamento entre perfis foi preservado.
- [ ] Plugins e temas continuam limitados aos diretórios e APIs permitidos.
- [ ] Foram incluídos testes para regras novas ou casos de borda relevantes.
- [ ] A documentação foi atualizada quando necessário.
- [ ] Não foram incluídos tokens, cookies, sessões, builds locais ou credenciais.

## Reportando vulnerabilidades

Não publique tokens, cookies, dumps de sessão, números de telefone ou detalhes exploráveis em uma issue pública. Para uma vulnerabilidade, use os recursos privados de segurança do GitHub ou solicite um canal privado ao mantenedor.

## Roadmap

- Permissões explícitas para plugins.
- Testes automatizados de integração do host e dos perfis.
- Releases Windows assinadas.
- Atualizador com verificação de integridade.
- Catálogo de extensões com revisão.
- Suporte a mais plataformas quando o embedding isolado estiver maduro.

## Licenciamento e terceiros

O código deste repositório ainda está sem uma licença de distribuição definida. Até que uma licença seja adicionada, não presuma autorização para redistribuir ou incorporar o projeto em outro produto.

O bundle do WA-JS acompanha a licença correspondente em [assets/injector/wa-js.LICENSE.txt](assets/injector/wa-js.LICENSE.txt). WhatsApp, WhatsApp Web, Meta e WPPConnect são marcas e projetos de seus respectivos titulares.
