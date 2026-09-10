import * as monaco from "../node_modules/monaco-editor/esm/vs/editor/editor.api.js";
import editorWorker from "../node_modules/monaco-editor/esm/vs/editor/editor.worker.js?worker&inline";
import "../node_modules/monaco-editor/dev/vs/style.css";
import { Service } from "../bindings/betterwhatsapp/internal/appservice/index.js";
import type {
  AppState,
  PluginInfo,
  ThemeInfo,
} from "../bindings/betterwhatsapp/internal/model/models.js";
import betterWhatsAppLogo from "./assets/betterwhatsapp-logo.png";

type SurfaceKind = "themes" | "plugins";
type Tone = "neutral" | "success" | "error";

type MonacoEnvironment = {
  getWorker: (moduleId: string, label: string) => Worker;
};

const surfaceValue = document.body.dataset.surface;
if (surfaceValue !== "themes" && surfaceValue !== "plugins") {
  throw new Error("BetterWhatsApp surface is missing or invalid");
}
const surface = surfaceValue as SurfaceKind;
const rootElement = document.querySelector<HTMLDivElement>("#app");
if (!rootElement) {
  throw new Error("BetterWhatsApp surface root was not found");
}
const root: HTMLDivElement = rootElement;

const globalWithMonaco = globalThis as typeof globalThis & {
  MonacoEnvironment?: MonacoEnvironment;
};
globalWithMonaco.MonacoEnvironment = {
  getWorker() {
    return new editorWorker();
  },
};

const cssLanguageID = "betterwhatsapp-css-workbench";
const editorThemeID = "betterwhatsapp-workbench";
const cssPropertyDocs: Record<string, string> = {
  background: "Define o fundo de um elemento; aceita cor, imagem, posição e repetição.",
  color: "Define a cor do texto.",
  display: "Escolhe o modelo de layout do elemento, como flex, grid ou block.",
  gap: "Define o espaço entre itens de flexbox ou grid.",
  height: "Define a altura do elemento.",
  margin: "Define o espaço externo do elemento.",
  opacity: "Define a opacidade entre 0 e 1.",
  padding: "Define o espaço interno do elemento.",
  position: "Escolhe como o elemento participa do posicionamento da página.",
  width: "Define a largura do elemento.",
  "border-radius": "Arredonda os cantos do elemento.",
  "font-family": "Define a família tipográfica.",
  "font-size": "Define o tamanho do texto.",
  "font-weight": "Define o peso do texto.",
  "line-height": "Define a altura da linha.",
  "z-index": "Define a ordem de empilhamento do elemento.",
};

const cssProperties = [
  ["background", "Fundo, cor ou imagem"],
  ["background-color", "Cor de fundo"],
  ["border", "Borda completa"],
  ["border-radius", "Cantos arredondados"],
  ["box-shadow", "Sombra do elemento"],
  ["color", "Cor do texto"],
  ["cursor", "Cursor do mouse"],
  ["display", "Modelo de layout"],
  ["filter", "Filtros visuais"],
  ["font-family", "Família da fonte"],
  ["font-size", "Tamanho da fonte"],
  ["font-weight", "Peso da fonte"],
  ["gap", "Espaçamento entre itens"],
  ["height", "Altura"],
  ["letter-spacing", "Espaçamento entre letras"],
  ["line-height", "Altura da linha"],
  ["margin", "Margem externa"],
  ["max-height", "Altura máxima"],
  ["max-width", "Largura máxima"],
  ["min-height", "Altura mínima"],
  ["opacity", "Opacidade"],
  ["overflow", "Comportamento do overflow"],
  ["padding", "Espaçamento interno"],
  ["position", "Posicionamento"],
  ["text-align", "Alinhamento do texto"],
  ["text-decoration", "Decoração do texto"],
  ["transform", "Transformação visual"],
  ["transition", "Transição"],
  ["user-select", "Seleção de texto"],
  ["visibility", "Visibilidade"],
  ["white-space", "Quebra de texto"],
  ["width", "Largura"],
  ["z-index", "Camada visual"],
] as const;

const cssValues: Record<string, string[]> = {
  display: ["block", "flex", "grid", "inline-flex", "none"],
  position: ["static", "relative", "absolute", "fixed", "sticky"],
  overflow: ["visible", "hidden", "auto", "scroll"],
  "font-weight": ["400", "500", "600", "700", "800"],
  "text-align": ["left", "center", "right", "justify"],
  "user-select": ["none", "text", "all", "auto"],
  visibility: ["visible", "hidden", "collapse"],
  cursor: ["default", "pointer", "text", "not-allowed", "grab"],
  "white-space": ["normal", "nowrap", "pre", "pre-wrap"],
};

function registerCSSLanguage() {
  monaco.languages.register({
    id: cssLanguageID,
    aliases: ["BetterWhatsApp CSS", "bwcss"],
    extensions: [".css"],
  });
  monaco.languages.setLanguageConfiguration(cssLanguageID, {
    comments: { blockComment: ["/*", "*/"] },
    brackets: [
      ["{", "}"],
      ["[", "]"],
      ["(", ")"],
    ],
    autoClosingPairs: [
      { open: "{", close: "}" },
      { open: "[", close: "]" },
      { open: "(", close: ")" },
      { open: '"', close: '"' },
      { open: "'", close: "'" },
    ],
    surroundingPairs: [
      { open: "{", close: "}" },
      { open: "[", close: "]" },
      { open: "(", close: ")" },
      { open: '"', close: '"' },
      { open: "'", close: "'" },
    ],
  });
  monaco.languages.setMonarchTokensProvider(cssLanguageID, {
    tokenizer: {
      root: [
        [/\/\*/, "comment", "@comment"],
        [/[{}[\]()]/, "delimiter.bracket"],
        [/--[a-z0-9_-]+/i, "variable"],
        [/[.#][a-z_][a-z0-9_-]*/i, "type.identifier"],
        [/::?[a-z-]+/i, "keyword"],
        [/[a-z-][a-z0-9_-]*(?=\s*:)/i, "attribute.name"],
        [/#(?:[0-9a-f]{3,8})\b/i, "number.hex"],
        [/\b\d+(?:\.\d+)?(?:px|rem|em|vh|vw|vmin|vmax|ch|%|s|ms|deg)?\b/i, "number"],
        [/"[^"]*"|'[^']*'/, "string"],
        [/\b(?:!important|inherit|initial|unset|var|calc)\b/i, "keyword"],
        [/[a-z_][a-z0-9_-]*/i, "identifier"],
      ],
      comment: [
        [/[^*/]+/, "comment"],
        [/\*\//, "comment", "@pop"],
        [/[*\/]/, "comment"],
      ],
    },
  });
  monaco.editor.defineTheme(editorThemeID, {
    base: "vs-dark",
    inherit: true,
    rules: [
      { token: "comment", foreground: "5D806B", fontStyle: "italic" },
      { token: "delimiter.bracket", foreground: "8FA99A" },
      { token: "type.identifier", foreground: "C8A6F7" },
      { token: "keyword", foreground: "FFB86C" },
      { token: "attribute.name", foreground: "82D9FF" },
      { token: "variable", foreground: "F2D27C" },
      { token: "number.hex", foreground: "A6E3A1" },
      { token: "number", foreground: "B4D4FF" },
      { token: "string", foreground: "F5C2E7" },
      { token: "identifier", foreground: "D8E9DD" },
    ],
    colors: {
      "editor.background": "#08110c",
      "editor.foreground": "#d8e9dd",
      "editorLineNumber.foreground": "#466354",
      "editorLineNumber.activeForeground": "#75f3a6",
      "editorCursor.foreground": "#50e58b",
      "editor.selectionBackground": "#1d5334",
      "editor.inactiveSelectionBackground": "#143523",
      "editor.lineHighlightBackground": "#0d1c13",
      "editorIndentGuide.background": "#16291e",
      "editorIndentGuide.activeBackground": "#2e6442",
      "editorWidget.background": "#0d1912",
      "editorWidget.border": "#31533d",
      "editorSuggestWidget.background": "#0d1912",
      "editorSuggestWidget.border": "#31533d",
      "editorSuggestWidget.selectedBackground": "#173d27",
    },
  });
  monaco.languages.registerCompletionItemProvider(cssLanguageID, {
    triggerCharacters: [":", "-", " "],
    provideCompletionItems(model, position) {
      const linePrefix = model.getLineContent(position.lineNumber).slice(0, position.column - 1);
      const sourceBeforeCursor = model.getValueInRange({
        startLineNumber: 1,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      });
      const insideBlock =
        (sourceBeforeCursor.match(/{/g)?.length ?? 0) >
        (sourceBeforeCursor.match(/}/g)?.length ?? 0);
      const word = model.getWordUntilPosition(position);
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: word.startColumn,
        endColumn: position.column,
      };

      if (!insideBlock) {
        const selectors = [
          [":root", "Variáveis globais do tema"],
          ["body", "Corpo da página"],
          ["#app", "Raiz do WhatsApp Web"],
          ["[data-testid=\"conversation-panel-body\"]", "Corpo da conversa"],
          ["[contenteditable=\"true\"]", "Campo de mensagem"],
        ];
        return {
          suggestions: selectors.map(([label, detail]) => ({
            label,
            kind: monaco.languages.CompletionItemKind.Keyword,
            insertText: label,
            detail,
            range,
          })),
        };
      }

      const valueMatch = linePrefix.match(/([a-z-]+)\s*:\s*([a-z-]*)$/i);
      if (valueMatch) {
        const property = valueMatch[1].toLowerCase();
        const values = cssValues[property] ?? [
          "transparent",
          "none",
          "auto",
          "inherit",
          "initial",
          "unset",
          "var(--bw-accent)",
          "#50e58b",
          "rgba(0, 0, 0, 0.4)",
        ];
        return {
          suggestions: values.map((value) => ({
            label: value,
            kind: value.startsWith("#") || value.startsWith("rgba")
              ? monaco.languages.CompletionItemKind.Color
              : monaco.languages.CompletionItemKind.Value,
            insertText: value,
            detail: cssPropertyDocs[property] ?? "Valor CSS",
            range,
          })),
        };
      }

      return {
        suggestions: cssProperties.map(([label, detail]) => ({
          label,
          kind: monaco.languages.CompletionItemKind.Property,
            insertText: label + ": ${1};",
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          detail,
          documentation: cssPropertyDocs[label] ?? detail,
          range,
        })),
      };
    },
  });
  monaco.languages.registerHoverProvider(cssLanguageID, {
    provideHover(model, position) {
      const word = model.getWordAtPosition(position);
      if (!word) {
        return null;
      }
      const key = word.word.toLowerCase();
      const description = cssPropertyDocs[key];
      if (!description) {
        return null;
      }
      return {
        range: new monaco.Range(
          position.lineNumber,
          word.startColumn,
          position.lineNumber,
          word.endColumn,
        ),
        contents: [
          { value: "**" + key + "**" },
          { value: description },
        ],
      };
    },
  });
}

let currentState: AppState | null = null;
let busy = false;
let editor: monaco.editor.IStandaloneCodeEditor | null = null;
let editingThemeID: string | null = null;

function query<T extends Element>(selector: string): T {
  const element = document.querySelector<T>(selector);
  if (!element) {
    throw new Error("Elemento da superfície não encontrado: " + selector);
  }
  return element;
}

function pluginDocsMarkup(): string {
  return `
    <section class="plugin-docs" aria-labelledby="plugin-docs-title">
      <div class="plugin-docs-heading">
        <div>
          <span class="surface-kicker">PLUGIN HANDBOOK</span>
          <h2 id="plugin-docs-title">Construa extensões que respeitam a sessão.</h2>
          <p>Uma referência prática para escrever plugins locais, observar o DOM do WhatsApp e usar o WPP sem acoplar seu código aos serviços Go.</p>
        </div>
        <span class="plugin-docs-badge">LOCAL JS / REMOTE DOM</span>
      </div>
      <nav class="plugin-doc-index" aria-label="Índice da documentação">
        <a href="#plugin-doc-quickstart">01 Quick start</a>
        <a href="#plugin-doc-styling">02 Estilos</a>
        <a href="#plugin-doc-dom">03 DOM e eventos</a>
        <a href="#plugin-doc-wpp">04 WPP</a>
        <a href="#plugin-doc-manifest">05 Manifest</a>
        <a href="#plugin-doc-safety">06 Segurança</a>
      </nav>
      <div class="plugin-doc-grid">
        <article class="plugin-doc-section plugin-doc-section-wide" id="plugin-doc-quickstart">
          <div class="plugin-doc-section-heading">
            <span class="plugin-doc-number">01</span>
            <div>
              <span class="surface-kicker">FIRST RUN</span>
              <h3>Seu primeiro plugin</h3>
              <p>O entrypoint deve registrar um único plugin com um ID estável. O factory recebe a API quando o injector carregar a extensão.</p>
            </div>
          </div>
          <pre class="plugin-doc-code"><code>BetterWhatsApp.registerPlugin("meu-plugin", ({ WPP, addStyle, observe, log }) => {
  log("Plugin carregado");

  addStyle("meu-plugin-style", [
    "[data-better-whatsapp='meu-plugin'] {",
    "  color: var(--bw-accent, #5de39d);",
    "}"
  ].join("\\n"));
});</code></pre>
          <div class="plugin-doc-note">
            <code>registerPlugin</code>
            <span>O runtime chama seu factory uma vez por carregamento da sessão. Mantenha o código idempotente porque o WhatsApp pode reconstruir partes do DOM.</span>
          </div>
        </article>

        <article class="plugin-doc-section" id="plugin-doc-styling">
          <div class="plugin-doc-section-heading">
            <span class="plugin-doc-number">02</span>
            <div>
              <span class="surface-kicker">VISUAL LAYER</span>
              <h3>Estilos controlados</h3>
              <p>Use um ID próprio para atualizar ou substituir seu CSS sem criar dezenas de tags style.</p>
            </div>
          </div>
          <pre class="plugin-doc-code"><code>addStyle("compact-conversation", [
  "[data-testid='conversation-panel-body'] {",
  "  --bw-message-gap: 4px;",
  "}",
  "[data-testid='conversation-panel-body'] [role='row'] {",
  "  margin-block: var(--bw-message-gap);",
  "}"
].join("\\n"));</code></pre>
          <div class="plugin-doc-rule"><strong>Prefira:</strong><span>atributos <code>data-testid</code>, classes próprias e seletores curtos.</span></div>
          <div class="plugin-doc-rule"><strong>Evite:</strong><span>seletores baseados em classes internas minificadas ou regras globais para <code>div</code>.</span></div>
        </article>

        <article class="plugin-doc-section" id="plugin-doc-dom">
          <div class="plugin-doc-section-heading">
            <span class="plugin-doc-number">03</span>
            <div>
              <span class="surface-kicker">DOM LIFECYCLE</span>
              <h3>Elementos que aparecem depois</h3>
              <p><code>observe</code> observa mutações e entrega o primeiro elemento que corresponde ao seletor.</p>
            </div>
          </div>
          <pre class="plugin-doc-code"><code>const stop = observe("[contenteditable='true']", (composer) => {
  if (composer.querySelector("[data-better-whatsapp='toolbar']")) {
    return;
  }

  const toolbar = document.createElement("span");
  toolbar.dataset.betterWhatsapp = "toolbar";
  toolbar.textContent = "Plugin ativo";
  composer.append(toolbar);
});

// Quando seu plugin precisar ser desmontado:
// stop();</code></pre>
          <div class="plugin-doc-note">
            <code>observe(selector, callback)</code>
            <span>Retorna uma função de cleanup. Guarde-a se o plugin criar listeners, timers ou uma tela própria.</span>
          </div>
        </article>

        <article class="plugin-doc-section" id="plugin-doc-wpp">
          <div class="plugin-doc-section-heading">
            <span class="plugin-doc-number">04</span>
            <div>
              <span class="surface-kicker">WA-JS BRIDGE</span>
              <h3>Usando o WPP</h3>
              <p><code>WPP</code> é a ponte do WA-JS/WPPConnect. As APIs disponíveis podem variar conforme a versão do WhatsApp Web.</p>
            </div>
          </div>
          <pre class="plugin-doc-code"><code>BetterWhatsApp.registerPlugin("wpp-inspector", ({ WPP, log }) => {
  if (!WPP) {
    log("WPP ainda não está disponível");
    return;
  }

  log("Módulos WPP disponíveis", Object.keys(WPP));
  log("Chat API disponível", WPP.chat);
});</code></pre>
          <div class="plugin-doc-rule"><strong>Compatibilidade:</strong><span>confira a existência do módulo/método antes de chamar APIs internas.</span></div>
          <div class="plugin-doc-rule"><strong>Dados:</strong><span>não persista tokens, contatos ou mensagens fora do contexto autorizado pelo usuário.</span></div>
        </article>

        <article class="plugin-doc-section" id="plugin-doc-manifest">
          <div class="plugin-doc-section-heading">
            <span class="plugin-doc-number">05</span>
            <div>
              <span class="surface-kicker">PROJECT FILE</span>
              <h3>Manifest do projeto</h3>
              <p>O gerenciador cria uma cópia editável de plugins bundled e abre a pasta no VS Code.</p>
            </div>
          </div>
          <pre class="plugin-doc-code"><code>{
  "id": "meu-plugin",
  "name": "Meu plugin",
  "version": "0.1.0",
  "description": "Uma extensão local",
  "entry": "index.js"
}</code></pre>
          <div class="plugin-doc-file-tree">
            <code>meu-plugin/</code><span>manifest.json</span>
            <code>↳</code><span>index.js</span>
            <code>↳</code><span>README.md</span>
          </div>
        </article>

        <article class="plugin-doc-section" id="plugin-doc-safety">
          <div class="plugin-doc-section-heading">
            <span class="plugin-doc-number">06</span>
            <div>
              <span class="surface-kicker">BOUNDARY</span>
              <h3>Limites e segurança</h3>
              <p>O plugin executa na WebView remota. Ele não recebe acesso direto aos serviços Go ou ao filesystem.</p>
            </div>
          </div>
          <div class="plugin-doc-callout">
            <strong>Use somente a superfície documentada.</strong>
            <p>Não injete scripts remotos, não use <code>eval</code> com conteúdo externo e não coloque credenciais no código. Valide dados vindos do DOM antes de agir.</p>
          </div>
          <div class="plugin-doc-api-list">
            <div><code>registerPlugin</code><span>registra a extensão no runtime</span></div>
            <div><code>WPP</code><span>ponte opcional do WA-JS</span></div>
            <div><code>addStyle</code><span>CSS controlado por ID</span></div>
            <div><code>observe</code><span>observação do DOM com cleanup</span></div>
            <div><code>log</code><span>diagnóstico no console</span></div>
          </div>
        </article>
      </div>
    </section>
  `;
}

function shellMarkup(kind: SurfaceKind) {
  const title = kind === "themes" ? "Themes" : "Plugins";
  const context = kind === "themes" ? "CSS WORKBENCH" : "EXTENSION LAB";
  const body =
    kind === "themes"
      ? `
        <div class="surface-layout">
          <aside class="surface-sidebar">
            <div class="surface-sidebar-heading">
              <div>
                <span class="surface-kicker">WORKSPACE</span>
                <strong>CSS files</strong>
              </div>
              <button class="surface-icon-button" id="theme-new" type="button" aria-label="Novo arquivo CSS">+</button>
            </div>
            <p class="surface-sidebar-copy">Arquivos locais carregados globalmente no WhatsApp Web.</p>
            <div class="surface-file-list" id="theme-list"></div>
            <div class="surface-sidebar-foot">
              <span class="surface-status-dot"></span>
              <span id="theme-sidebar-status">Sincronizando arquivos</span>
            </div>
          </aside>
          <main class="surface-content">
            <div class="surface-hero">
              <div>
                <span class="surface-kicker">THEMES / GLOBAL CSS</span>
                <h1>O visual começa<br /><em>no código.</em></h1>
                <p>Edite uma folha CSS local com autocomplete, tokens coloridos e aplicação global na sessão.</p>
              </div>
              <div class="surface-hero-stat">
                <strong id="theme-count">0/0</strong>
                <span>arquivos ativos</span>
              </div>
            </div>
            <section class="surface-editor-card">
              <div class="surface-section-heading">
                <div>
                  <span class="surface-kicker" id="theme-editor-kicker">NEW CSS FILE</span>
                  <h2 id="theme-editor-title">Novo arquivo CSS</h2>
                </div>
                <span class="surface-file-path" id="theme-file-path">themes/&lt;novo&gt;/theme.css</span>
              </div>
              <div class="surface-form-row">
                <label class="surface-field">
                  <span>Nome do arquivo</span>
                  <input id="theme-name" type="text" maxlength="80" autocomplete="off" placeholder="Ex.: Conversas compactas" />
                </label>
                <div class="surface-editor-tip">
                  <span class="surface-tip-icon">⌘</span>
                  <span>Ctrl + espaço abre sugestões CSS. Passe o mouse sobre uma propriedade para ver a descrição.</span>
                </div>
              </div>
              <div class="surface-code-label">
                <span>CSS GLOBAL</span>
                <span id="editor-language-status">BetterWhatsApp CSS</span>
              </div>
              <div id="theme-source-editor" class="surface-editor" aria-label="Editor CSS global"></div>
              <textarea id="theme-source-fallback" class="surface-editor-fallback" spellcheck="false" hidden></textarea>
              <div class="surface-editor-footer">
                <span class="surface-file-path" id="theme-editor-footer-path">themes/&lt;novo&gt;/theme.css</span>
                <div class="surface-actions">
                  <button class="surface-button surface-button-danger" id="theme-delete" type="button" hidden>Excluir</button>
                  <button class="surface-button surface-button-accent" id="theme-save" type="button">Salvar CSS</button>
                </div>
              </div>
            </section>
            <section class="surface-help-grid">
              <article class="surface-help-card">
                <span class="surface-kicker">SELECTORS</span>
                <strong>Comece pelo que você vê</strong>
                <p>Use atributos estáveis do WhatsApp e prefira regras pequenas com !important apenas quando necessário.</p>
              </article>
              <article class="surface-help-card">
                <span class="surface-kicker">LIFECYCLE</span>
                <strong>Aplicação global</strong>
                <p>Salvar ou ativar um arquivo recria a sessão remota para garantir que o CSS seja carregado desde o início.</p>
              </article>
            </section>
          </main>
        </div>
      `
      : `
        <div class="surface-layout">
          <aside class="surface-sidebar">
            <div class="surface-sidebar-heading">
              <div>
                <span class="surface-kicker">CATALOG</span>
                <strong>Plugins locais</strong>
              </div>
              <span class="surface-count" id="plugin-count">0/0</span>
            </div>
            <p class="surface-sidebar-copy">Código que roda dentro da sessão remota do WhatsApp.</p>
            <div class="surface-file-list" id="plugin-list"></div>
            <div class="surface-sidebar-foot">
              <span class="surface-status-dot"></span>
              <span>API local documentada</span>
            </div>
          </aside>
          <main class="surface-content">
            <div class="surface-hero">
              <div>
                <span class="surface-kicker">PLUGINS / PROJECTS</span>
                <h1>Seu código,<br /><em>seu runtime.</em></h1>
                <p>Ative extensões, abra o projeto no VS Code e consulte a API disponível dentro do WhatsApp Web.</p>
              </div>
              <div class="surface-hero-stat">
                <strong id="plugin-active-count">0</strong>
                <span>plugins ativos</span>
              </div>
            </div>
            <div class="plugin-workspace-grid">
              <section class="surface-panel">
                <div class="surface-section-heading">
                  <div>
                    <span class="surface-kicker">INSTALLED</span>
                    <h2>Catálogo local</h2>
                  </div>
                  <div class="surface-section-heading-actions">
                    <span class="surface-file-path">plugins/ local</span>
                    <button class="surface-button surface-button-accent surface-button-small" id="plugin-new" type="button">Novo template</button>
                  </div>
                </div>
                <div class="plugin-card-list" id="plugin-catalog-list"></div>
              </section>
              <aside class="surface-panel api-panel">
                <div class="surface-section-heading">
                  <div>
                    <span class="surface-kicker">PLUGIN API</span>
                    <h2>Contrato de execução</h2>
                  </div>
                  <button class="surface-button surface-button-quiet" id="copy-api" type="button">Copiar</button>
                </div>
                <p class="surface-panel-lead">A extensão é JavaScript local, mas executa isolada dentro da WebView remota.</p>
                <pre class="surface-code-block" id="api-example"></pre>
                <div class="api-capability-list">
                  <div><code>WPP</code><span>APIs do WA-JS / WPPConnect</span></div>
                  <div><code>addStyle</code><span>CSS controlado no documento</span></div>
                  <div><code>observe</code><span>Elementos que surgem no DOM</span></div>
                  <div><code>log</code><span>Diagnóstico no console</span></div>
                </div>
              </aside>
            </div>
            <section class="surface-project-strip">
              <div>
                <span class="surface-kicker">PROJECT MANAGER</span>
                <strong id="project-status">Selecione um plugin para abrir o projeto</strong>
                <code id="project-path">—</code>
              </div>
              <span class="surface-project-note">O primeiro acesso a um plugin bundled cria uma cópia editável em AppData.</span>
            </section>
            ${pluginDocsMarkup()}
          </main>
        </div>
      `;

  root.innerHTML = `
    <div class="surface-app" data-surface="${kind}">
      <header class="surface-windowbar" style="--wails-draggable: drag">
        <div class="surface-windowbar-brand" style="--wails-draggable: drag">
          <img class="surface-logo" src="${betterWhatsAppLogo}" alt="BetterWhatsApp" />
          <div>
            <strong>BetterWhatsApp <span>/ ${title}</span></strong>
            <small>LOCAL WORKSPACE</small>
          </div>
        </div>
        <div class="surface-context" style="--wails-draggable: drag">
          <span class="surface-live-dot"></span>
          <span>${context}</span>
          <span class="surface-separator"></span>
          <span>ISOLATED CONTROL</span>
        </div>
        <div class="surface-window-actions" style="--wails-draggable: no-drag">
          <button class="surface-window-button" data-surface-window="hide" type="button" aria-label="Ocultar janela" title="Ocultar janela">−</button>
          <button class="surface-window-button" data-surface-window="maximise" type="button" aria-label="Maximizar janela" title="Maximizar janela">□</button>
          <button class="surface-window-button surface-window-close" data-surface-window="close" type="button" aria-label="Fechar janela" title="Fechar janela">×</button>
        </div>
      </header>
      ${body}
      ${kind === "plugins" ? `
        <div class="surface-plugin-dialog-backdrop" id="plugin-template-dialog" role="dialog" aria-modal="true" aria-labelledby="plugin-template-dialog-title" hidden>
          <form class="surface-plugin-dialog" id="plugin-template-form">
            <div class="surface-plugin-dialog-heading">
              <span class="surface-kicker">NEW LOCAL PROJECT</span>
              <h2 id="plugin-template-dialog-title">Criar template de plugin</h2>
              <p>Um projeto editável será criado em <code>%AppData%/BetterWhatsApp/plugins</code>. Ele começa desligado e não substitui nenhum plugin existente.</p>
            </div>
            <label class="surface-plugin-field">
              <span>Nome do plugin</span>
              <input id="plugin-template-name" name="name" type="text" maxlength="80" placeholder="Ex.: Ferramentas da equipe" autocomplete="off" required />
            </label>
            <p class="surface-plugin-dialog-hint">O ID da pasta é derivado automaticamente do nome e recebe um sufixo se já existir.</p>
            <div class="surface-plugin-dialog-actions">
              <button class="surface-button surface-button-quiet" id="plugin-template-cancel" type="button">Cancelar</button>
              <button class="surface-button surface-button-accent" type="submit">Criar projeto</button>
            </div>
          </form>
        </div>
      ` : ""}
      <footer class="surface-footer">
        <div class="surface-footer-status" aria-live="polite">
          <span class="surface-status-dot"></span>
          <span id="surface-status">Carregando estado local…</span>
        </div>
        <button class="surface-footer-action" id="reload-session" type="button">Recarregar WhatsApp <span>↻</span></button>
      </footer>
      <div class="surface-error" id="surface-error" role="alert" hidden></div>
    </div>
  `;
}

function setStatus(message: string, tone: Tone = "neutral") {
  const element = query<HTMLElement>("#surface-status");
  element.textContent = message;
  element.dataset.tone = tone;
}

function showError(error: unknown) {
  const banner = query<HTMLDivElement>("#surface-error");
  banner.textContent = error instanceof Error ? error.message : String(error);
  banner.hidden = false;
  setStatus("Ação não concluída", "error");
}

function clearError() {
  query<HTMLDivElement>("#surface-error").hidden = true;
}

function setBusy(value: boolean) {
  busy = value;
  root.querySelector(".surface-app")?.setAttribute("data-busy", String(value));
  root.querySelectorAll<HTMLButtonElement | HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>("button, input, textarea, select").forEach((element) => {
    element.disabled = value;
  });
  editor?.updateOptions({ readOnly: value });
}

function createToggle(checked: boolean, label: string, onChange: (value: boolean) => void) {
  const wrapper = document.createElement("label");
  wrapper.className = "surface-toggle";
  wrapper.title = label;
  const input = document.createElement("input");
  input.type = "checkbox";
  input.checked = checked;
  input.setAttribute("aria-label", label);
  input.addEventListener("change", () => onChange(input.checked));
  const track = document.createElement("span");
  track.className = "surface-toggle-track";
  track.innerHTML = '<span class="surface-toggle-thumb"></span>';
  wrapper.append(input, track);
  return wrapper;
}

async function refreshState() {
  currentState = await Service.GetState();
  if (surface === "themes") {
    renderThemes(currentState.themes ?? []);
  } else {
    renderPlugins(currentState.plugins ?? []);
  }
}

async function performAction(
  label: string,
  action: () => Promise<unknown>,
  reload = true,
) {
  if (busy) {
    return;
  }
  setBusy(true);
  clearError();
  setStatus(label);
  try {
    await action();
    if (reload) {
      await Service.ReloadWhatsApp();
    }
    await refreshState();
    setStatus("Alterações aplicadas", "success");
  } catch (error) {
    showError(error);
  } finally {
    setBusy(false);
  }
}

function configureEditor() {
  registerCSSLanguage();
  const container = query<HTMLDivElement>("#theme-source-editor");
  try {
    editor = monaco.editor.create(container, {
      model: monaco.editor.createModel("", cssLanguageID),
      theme: editorThemeID,
      automaticLayout: true,
      minimap: { enabled: false },
      fontFamily: '"Cascadia Code", "JetBrains Mono", Consolas, monospace',
      fontSize: 13,
      lineHeight: 21,
      tabSize: 2,
      insertSpaces: true,
      wordWrap: "on",
      scrollBeyondLastLine: false,
      renderLineHighlight: "all",
      roundedSelection: false,
      overviewRulerBorder: false,
      padding: { top: 17, bottom: 17 },
      suggest: {
        showMethods: false,
        showFunctions: false,
        showConstructors: false,
        showDeprecated: false,
        showFields: false,
        showVariables: true,
        showClasses: false,
        showStructs: false,
      },
      quickSuggestions: { other: true, comments: false, strings: true },
      suggestOnTriggerCharacters: true,
      parameterHints: { enabled: false },
    });
    const observer = new ResizeObserver(() => editor?.layout());
    observer.observe(container);
  } catch (error) {
    console.error("[BetterWhatsApp] Monaco failed", error);
    container.hidden = true;
    const fallback = query<HTMLTextAreaElement>("#theme-source-fallback");
    fallback.hidden = false;
    setStatus("Monaco indisponível; fallback ativado", "error");
  }
}

function getEditorValue() {
  return editor?.getValue() ?? query<HTMLTextAreaElement>("#theme-source-fallback").value;
}

function setEditorValue(source: string) {
  if (editor) {
    editor.setValue(source);
  }
  query<HTMLTextAreaElement>("#theme-source-fallback").value = source;
}

function setThemeEditorChrome() {
  const editing = editingThemeID !== null;
  query<HTMLElement>("#theme-editor-kicker").textContent = editing ? "EDITING CSS FILE" : "NEW CSS FILE";
  query<HTMLElement>("#theme-editor-title").textContent = editing ? "Editar arquivo CSS" : "Novo arquivo CSS";
  const path = editing ? "themes/" + editingThemeID + "/theme.css" : "themes/<novo>/theme.css";
  query<HTMLElement>("#theme-file-path").textContent = path;
  query<HTMLElement>("#theme-editor-footer-path").textContent = path;
  query<HTMLButtonElement>("#theme-delete").hidden = !editing;
}

function newTheme() {
  editingThemeID = null;
  query<HTMLInputElement>("#theme-name").value = "";
  setEditorValue(":root {\n  --bw-accent: #50e58b;\n}\n\n/* Escreva suas regras globais aqui. */\n");
  setThemeEditorChrome();
  setStatus("Novo arquivo CSS pronto");
  query<HTMLInputElement>("#theme-name").focus();
}

async function editTheme(theme: ThemeInfo) {
  if (!theme.editable || busy) {
    return;
  }
  setBusy(true);
  clearError();
  setStatus("Lendo " + theme.name + "…");
  try {
    const source = await Service.ReadTheme(theme.id);
    editingThemeID = theme.id;
    query<HTMLInputElement>("#theme-name").value = theme.name;
    setEditorValue(source);
    setThemeEditorChrome();
    editor?.focus();
    setStatus("Editando " + theme.name, "success");
  } catch (error) {
    showError(error);
  } finally {
    setBusy(false);
  }
}

function renderThemes(themes: ThemeInfo[]) {
  const list = query<HTMLDivElement>("#theme-list");
  list.replaceChildren();
  const enabledCount = themes.filter((theme) => theme.enabled).length;
  query<HTMLElement>("#theme-count").textContent = enabledCount + "/" + themes.length;
  query<HTMLElement>("#theme-sidebar-status").textContent = themes.length + " arquivo(s) no workspace";

  if (themes.length === 0) {
    const empty = document.createElement("div");
    empty.className = "surface-empty";
    empty.textContent = "Nenhum arquivo CSS criado.";
    list.append(empty);
    return;
  }

  themes.forEach((theme) => {
    const row = document.createElement("article");
    row.className = "surface-file-row";
    row.dataset.active = editingThemeID === theme.id ? "true" : "false";

    const select = document.createElement("button");
    select.type = "button";
    select.className = "surface-file-select";
    select.disabled = !theme.editable;
    select.addEventListener("click", () => void editTheme(theme));
    const marker = document.createElement("span");
    marker.className = "surface-file-marker";
    marker.dataset.enabled = String(theme.enabled);
    const copy = document.createElement("span");
    copy.className = "surface-file-copy";
    const name = document.createElement("strong");
    name.textContent = theme.name;
    const meta = document.createElement("small");
    meta.textContent = theme.editable ? "theme.css" : "somente leitura";
    copy.append(name, meta);
    select.append(marker, copy);

    const toggle = createToggle(theme.enabled, "Ativar " + theme.name, (enabled) => {
      void performAction(
        enabled ? "Ativando " + theme.name + "…" : "Desativando " + theme.name + "…",
        () => Service.SetThemeEnabled(theme.id, enabled),
      );
    });
    row.append(select, toggle);
    list.append(row);
  });
}

async function saveTheme() {
  const name = query<HTMLInputElement>("#theme-name").value.trim();
  const source = getEditorValue();
  if (!name) {
    showError(new Error("Informe um nome para o arquivo CSS"));
    query<HTMLInputElement>("#theme-name").focus();
    return;
  }
  await performAction("Salvando CSS…", async () => {
    if (editingThemeID) {
      await Service.UpdateTheme(editingThemeID, name, source);
      return;
    }
    editingThemeID = await Service.CreateTheme(name, source);
    setThemeEditorChrome();
  });
}

async function deleteTheme() {
  if (!editingThemeID || busy) {
    return;
  }
  if (!window.confirm("Excluir este arquivo CSS?")) {
    return;
  }
  const id = editingThemeID;
  await performAction("Excluindo CSS…", async () => {
    await Service.DeleteTheme(id);
    newTheme();
  });
}

const pluginApiExample = [
  'BetterWhatsApp.registerPlugin("meu-plugin", ({ WPP, addStyle, observe, log }) => {',
  '  addStyle("meu-plugin-style", `',
  '    [data-testid="conversation-panel-body"] {',
  "      --bw-accent: #50e58b;",
  "    }",
  '  `);',
  "",
  '  observe("[contenteditable=\\"true\\"]", (composer) => {',
  '    log("Composer encontrado", composer);',
  "  });",
  "",
  '  log("Plugin carregado");',
  "});",
].join("\n");

function renderPlugins(plugins: PluginInfo[]) {
  const list = query<HTMLDivElement>("#plugin-list");
  const catalog = query<HTMLDivElement>("#plugin-catalog-list");
  list.replaceChildren();
  catalog.replaceChildren();
  const enabledCount = plugins.filter((plugin) => plugin.enabled).length;
  query<HTMLElement>("#plugin-count").textContent = enabledCount + "/" + plugins.length;
  query<HTMLElement>("#plugin-active-count").textContent = String(enabledCount);

  if (plugins.length === 0) {
    const sidebarEmpty = document.createElement("div");
    sidebarEmpty.className = "surface-empty";
    sidebarEmpty.textContent = "Nenhum plugin no catálogo local.";
    list.append(sidebarEmpty);

    const catalogEmpty = document.createElement("div");
    catalogEmpty.className = "surface-empty";
    catalogEmpty.textContent = "Nenhum plugin criado ainda. Use Novo template para começar.";
    catalog.append(catalogEmpty);
    return;
  }

  plugins.forEach((plugin) => {
    const nav = document.createElement("div");
    nav.className = "plugin-nav-row";
    const navDot = document.createElement("span");
    navDot.className = "plugin-nav-dot";
    if (plugin.enabled) {
      navDot.classList.add("is-enabled");
    }
    const navName = document.createElement("span");
    navName.textContent = plugin.name;
    nav.append(navDot, navName);
    list.append(nav);

    const card = document.createElement("article");
    card.className = "plugin-card";
    const identity = document.createElement("div");
    identity.className = "plugin-card-identity";
    const icon = document.createElement("span");
    icon.className = "plugin-card-icon";
    icon.textContent = "✦";
    const copy = document.createElement("div");
    copy.className = "plugin-card-copy";
    const name = document.createElement("h3");
    name.textContent = plugin.name;
    const description = document.createElement("p");
    description.textContent = plugin.description || "Sem descrição.";
    const metadata = document.createElement("small");
    metadata.textContent = plugin.id + " · v" + plugin.version + " · " + plugin.entry;
    copy.append(name, description, metadata);
    identity.append(icon, copy);

    const controls = document.createElement("div");
    controls.className = "plugin-card-controls";
    const globalToggle = createToggle(plugin.globalEnabled, "Alterar padrão global de " + plugin.name, (enabled) => {
      void performAction(
        enabled ? "Ativando globalmente " + plugin.name + "…" : "Desativando globalmente " + plugin.name + "…",
        () => Service.SetPluginEnabled(plugin.id, enabled),
      );
    });
    const open = document.createElement("button");
    open.type = "button";
    open.className = "surface-button surface-button-accent";
    open.textContent = "Abrir no VS Code";
    open.addEventListener("click", async () => {
      if (busy) {
        return;
      }
      setBusy(true);
      clearError();
      setStatus("Preparando projeto de " + plugin.name + "…");
      try {
        const projectPath = await Service.OpenPluginProject(plugin.id);
        query<HTMLElement>("#project-status").textContent = "Projeto aberto no VS Code";
        query<HTMLElement>("#project-path").textContent = projectPath;
        setStatus("VS Code aberto", "success");
      } catch (error) {
        showError(error);
      } finally {
        setBusy(false);
      }
    });
    controls.append(globalToggle, open);
    card.append(identity, controls);
    catalog.append(card);
  });
}

function bindWindowActions() {
  root.querySelectorAll<HTMLButtonElement>("[data-surface-window]").forEach((button) => {
    button.addEventListener("click", () => {
      const action = button.dataset.surfaceWindow;
      if (action === "hide") {
        void Service.HideWindow();
      } else if (action === "maximise") {
        void Service.ToggleMaximise();
      } else if (action === "close") {
        void Service.RequestClose();
      }
    });
  });
  query<HTMLButtonElement>("#reload-session").addEventListener("click", () => {
    void performAction("Recarregando WhatsApp…", () => Service.ReloadWhatsApp(), false);
  });
}

function bindThemeActions() {
  query<HTMLButtonElement>("#theme-new").addEventListener("click", newTheme);
  query<HTMLButtonElement>("#theme-save").addEventListener("click", () => void saveTheme());
  query<HTMLButtonElement>("#theme-delete").addEventListener("click", () => void deleteTheme());
}

function bindPluginActions() {
  query<HTMLElement>("#api-example").textContent = pluginApiExample;
  bindPluginTemplateDialog();
  query<HTMLButtonElement>("#copy-api").addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(pluginApiExample);
      setStatus("Exemplo copiado", "success");
    } catch {
      setStatus("Não foi possível copiar o exemplo", "error");
    }
  });
}

function closePluginTemplateDialog() {
  query<HTMLDivElement>("#plugin-template-dialog").hidden = true;
}

function openPluginTemplateDialog() {
  if (busy) {
    return;
  }
  clearError();
  const backdrop = query<HTMLDivElement>("#plugin-template-dialog");
  const input = query<HTMLInputElement>("#plugin-template-name");
  backdrop.hidden = false;
  input.value = "";
  input.focus();
}

async function createPluginTemplate(name: string) {
  if (busy || !name) {
    return;
  }
  setBusy(true);
  clearError();
  setStatus("Criando projeto de plugin…");
  try {
    const projectPath = await Service.CreatePluginTemplate(name);
    await refreshState();
    query<HTMLElement>("#project-status").textContent = "Template criado no catálogo local";
    query<HTMLElement>("#project-path").textContent = projectPath;
    closePluginTemplateDialog();
    setStatus("Template pronto para editar", "success");
  } catch (error) {
    showError(error);
  } finally {
    setBusy(false);
  }
}

function bindPluginTemplateDialog() {
  const backdrop = query<HTMLDivElement>("#plugin-template-dialog");
  const form = query<HTMLFormElement>("#plugin-template-form");
  const input = query<HTMLInputElement>("#plugin-template-name");

  query<HTMLButtonElement>("#plugin-new").addEventListener("click", openPluginTemplateDialog);
  query<HTMLButtonElement>("#plugin-template-cancel").addEventListener("click", closePluginTemplateDialog);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const name = input.value.trim();
    if (!name) {
      input.focus();
      return;
    }
    void createPluginTemplate(name);
  });
  backdrop.addEventListener("click", (event) => {
    if (event.target === backdrop) {
      closePluginTemplateDialog();
    }
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !backdrop.hidden) {
      closePluginTemplateDialog();
    }
  });
}

async function mount() {
  shellMarkup(surface);
  bindWindowActions();
  if (surface === "themes") {
    configureEditor();
    bindThemeActions();
    newTheme();
  } else {
    bindPluginActions();
  }
  setStatus("Sincronizando estado local…");
  try {
    await refreshState();
    setStatus("Workspace pronto", "success");
  } catch (error) {
    showError(error);
  }
}

void mount();
