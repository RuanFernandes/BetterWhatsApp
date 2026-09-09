import betterWhatsAppLogo from "./assets/betterwhatsapp-logo.png";

import type {
  AppState,
  PluginInfo,
} from "../bindings/betterwhatsapp/internal/model/models.js";

type ControlBridge = {
  state: AppState;
  command: (message: string) => boolean;
};

declare global {
  interface Window {
    BetterWhatsAppControl?: ControlBridge;
  }
}

const app = document.querySelector<HTMLDivElement>("#betterwhatsapp-panel-app");
if (!app) {
  throw new Error("BetterWhatsApp root element was not found");
}
const appRoot = app;

const bridge = window.BetterWhatsAppControl;
if (!bridge) {
  throw new Error("BetterWhatsApp control bridge was not found");
}
const controlBridge: ControlBridge = bridge;

appRoot.innerHTML = `
  <div class="control-panel">
    <header class="panel-windowbar">
      <div class="panel-windowbar-brand">
        <img class="panel-logo" src="${betterWhatsAppLogo}" alt="" aria-hidden="true" />
        <div>
          <strong>BetterWhatsApp</strong>
          <span>LOCAL CONTROL SURFACE</span>
        </div>
      </div>
      <div class="panel-windowbar-actions">
        <button class="window-button" data-window-action="hide" type="button" aria-label="Enviar para a bandeja" title="Enviar para a bandeja">−</button>
        <button class="window-button" data-window-action="maximise" type="button" aria-label="Maximizar janela" title="Maximizar janela">□</button>
        <button class="window-button window-button-close" data-window-action="close" type="button" aria-label="Fechar para a bandeja" title="Fechar para a bandeja">×</button>
      </div>
    </header>

    <main class="panel-scroll">
      <section class="panel-hero">
        <div class="hero-copy">
          <p class="eyebrow">BETTERWHATSAPP / CONTROL PLANE</p>
          <h1>Seu Web.<br /><em>Seu frame.</em></h1>
          <p class="hero-description">
            A sessão continua no WhatsApp Web. Esta superfície local cuida do que entra nela — temas, plugins e runtime.
          </p>
        </div>
        <div class="boundary-chip">
          <span class="boundary-dot"></span>
          <span>REMOTE / ISOLATED</span>
        </div>
      </section>

      <section class="connection-strip" aria-label="Estado da conexão">
        <div>
          <span class="strip-label">REMOTE ORIGIN</span>
          <strong id="remote-origin">—</strong>
        </div>
        <div>
          <span class="strip-label">APP VERSION</span>
          <strong id="app-version">—</strong>
        </div>
      </section>

      <section class="control-section" id="runtime-section">
        <div class="section-heading">
          <div>
            <p class="eyebrow">01 / RUNTIME</p>
            <h2>Injection layer</h2>
          </div>
          <span class="state-chip" id="injector-state">—</span>
        </div>
        <div class="runtime-card">
          <div>
            <strong>WA-JS + extension loader</strong>
            <p>O bundle local é preparado antes de a sessão remota assumir a janela.</p>
          </div>
          <label class="switch">
            <input id="injector-toggle" type="checkbox" aria-label="Ativar a camada de injeção" />
            <span class="switch-track"><span class="switch-thumb"></span></span>
          </label>
        </div>
      </section>

      <section class="control-section" id="plugins-section">
        <div class="section-heading">
          <div>
            <p class="eyebrow">02 / EXTENSIONS</p>
            <h2>Plugins</h2>
          </div>
          <span class="section-count" id="plugin-count">—</span>
        </div>
        <p class="section-lead">Comportamentos opcionais executados dentro da sessão do WhatsApp.</p>
        <div class="extension-list" id="plugin-list"></div>
      </section>

    </main>

    <footer class="panel-footer">
      <div class="footer-status" aria-live="polite">
        <span class="footer-status-dot"></span>
        <span id="status-text">Carregando estado local…</span>
      </div>
      <button class="footer-reload" id="reload-session" type="button">Recarregar sessão <span>↻</span></button>
    </footer>
    <div class="error-banner" id="error-banner" role="alert" hidden></div>
  </div>
`;

let pending = false;

const query = <T extends Element>(selector: string): T => {
  const element = appRoot.querySelector<T>(selector);
  if (!element) {
    throw new Error("Missing UI element: " + selector);
  }
  return element;
};

function postParent(type: string, extra: Record<string, unknown> = {}) {
  window.parent.postMessage(
    {
      source: "betterwhatsapp-panel",
      type,
      ...extra,
    },
    "*",
  );
}

function setBusy(value: boolean) {
  pending = value;
  appRoot.dataset.busy = String(value);
  appRoot.querySelectorAll<HTMLButtonElement | HTMLInputElement | HTMLTextAreaElement>("button, input, textarea").forEach((element) => {
    element.disabled = value;
  });
}

function setStatus(message: string, tone: "neutral" | "success" | "error" = "neutral") {
  const status = query<HTMLElement>("#status-text");
  status.textContent = message;
  status.dataset.tone = tone;
}

function clearError() {
  query<HTMLDivElement>("#error-banner").hidden = true;
}

function showError(error: unknown) {
  const banner = query<HTMLDivElement>("#error-banner");
  banner.textContent = error instanceof Error ? error.message : String(error);
  banner.hidden = false;
  setStatus("Falha na ação", "error");
}

function createToggle(
  checked: boolean,
  label: string,
  onChange: (enabled: boolean) => void,
): HTMLLabelElement {
  const wrapper = document.createElement("label");
  wrapper.className = "switch";
  const input = document.createElement("input");
  input.type = "checkbox";
  input.checked = checked;
  input.setAttribute("aria-label", label);
  input.addEventListener("change", () => onChange(input.checked));
  const track = document.createElement("span");
  track.className = "switch-track";
  track.innerHTML = '<span class="switch-thumb"></span>';
  wrapper.append(input, track);
  return wrapper;
}

function createEmptyState(message: string): HTMLDivElement {
  const empty = document.createElement("div");
  empty.className = "empty-state";
  empty.textContent = message;
  return empty;
}

function sendCommand(message: string): Promise<void> {
  if (!controlBridge.command(message)) {
    return Promise.reject(new Error("O comando nativo foi rejeitado pelo host"));
  }
  return Promise.resolve();
}

function sendExtensionCommand(
  kind: "plugin",
  id: string,
  enabled: boolean,
): Promise<void> {
  return sendCommand(
    `betterwhatsapp:settings:${kind}:${id}:${enabled ? "1" : "0"}`,
  );
}

function sendInjectorCommand(enabled: boolean): Promise<void> {
  return sendCommand(`betterwhatsapp:settings:injector:${enabled ? "1" : "0"}`);
}


function renderPlugins(plugins: PluginInfo[]) {
  const container = query<HTMLDivElement>("#plugin-list");
  container.replaceChildren();
  query<HTMLElement>("#plugin-count").textContent = `${plugins.filter((plugin) => plugin.enabled).length}/${plugins.length}`;

  if (plugins.length === 0) {
    container.append(createEmptyState("Nenhum plugin no catálogo local."));
    return;
  }

  plugins.forEach((plugin) => {
    const row = document.createElement("article");
    row.className = "extension-row";

    const marker = document.createElement("span");
    marker.className = "extension-marker";
    marker.dataset.state = plugin.enabled ? "enabled" : "disabled";
    marker.textContent = plugin.enabled ? "ON" : "—";

    const copy = document.createElement("div");
    copy.className = "extension-copy";
    const name = document.createElement("h3");
    name.textContent = plugin.name;
    const description = document.createElement("p");
    description.textContent = plugin.description || "Sem descrição.";
    copy.append(name, description);

    const identity = document.createElement("div");
    identity.className = "extension-identity";
    identity.append(marker, copy);

    const meta = document.createElement("div");
    meta.className = "extension-meta";
    const version = document.createElement("span");
    version.textContent = "v" + plugin.version;
    const toggle = createToggle(plugin.enabled, "Ativar " + plugin.name, (enabled) => {
      void runAction(() => sendExtensionCommand("plugin", plugin.id, enabled));
    });
    meta.append(version, toggle);

    row.append(identity, meta);
    container.append(row);
  });
}

function renderState(nextState: AppState) {
  const plugins = nextState.plugins ?? [];

  query<HTMLInputElement>("#injector-toggle").checked = nextState.injector.enabled;
  query<HTMLElement>("#injector-state").textContent = nextState.injector.enabled ? "ACTIVE" : "PAUSED";
  query<HTMLElement>("#injector-state").dataset.state = nextState.injector.enabled ? "active" : "paused";
  query<HTMLElement>("#remote-origin").textContent = nextState.remoteOrigin;
  query<HTMLElement>("#app-version").textContent = nextState.appVersion;

  renderPlugins(plugins);
}

async function refreshState() {
  if (pending) {
    return;
  }
  setBusy(true);
  clearError();
  setStatus("Sincronizando estado local…");
  try {
    renderState(controlBridge.state);
    setStatus("Pronto · estado local", "success");
  } catch (error) {
    showError(error);
  } finally {
    setBusy(false);
  }
}

async function runAction(action: () => Promise<unknown>) {
  if (pending) {
    return;
  }
  setBusy(true);
  clearError();
  setStatus("Salvando configuração local…");
  try {
    await action();
    setStatus("Aplicado · recarregando sessão", "success");
    window.setTimeout(() => postParent("reload-whatsapp"), 180);
  } catch (error) {
    showError(error);
  } finally {
    setBusy(false);
  }
}

async function runWindowAction(action: () => Promise<unknown>, successMessage: string) {
  if (pending) {
    return;
  }
  setBusy(true);
  clearError();
  try {
    await action();
    setStatus(successMessage, "success");
  } catch (error) {
    showError(error);
  } finally {
    setBusy(false);
  }
}

query<HTMLInputElement>("#injector-toggle").addEventListener("change", (event) => {
  const input = event.currentTarget as HTMLInputElement;
  void runAction(() => sendInjectorCommand(input.checked));
});

query<HTMLButtonElement>("#reload-session").addEventListener("click", () => {
  postParent("reload-whatsapp");
});

appRoot.querySelectorAll<HTMLButtonElement>("[data-window-action]").forEach((button) => {
  button.addEventListener("click", () => {
    const action = button.dataset.windowAction;
    if (action === "hide") {
      void runWindowAction(() => sendCommand("betterwhatsapp:window:hide"), "Janela enviada para a bandeja");
    } else if (action === "maximise") {
      void runWindowAction(() => sendCommand("betterwhatsapp:window:maximise"), "Estado da janela alternado");
    } else if (action === "close") {
      void runWindowAction(() => sendCommand("betterwhatsapp:window:close"), "Janela enviada para a bandeja");
    }
  });
});

window.addEventListener("message", (event) => {
  const message = event.data;
  if (event.source !== window.parent) {
    return;
  }
  if (!message || message.source !== "betterwhatsapp-navbar") {
    return;
  }
  if (message.type === "window-command") {
    if (message.command === "minimise") {
      void runWindowAction(() => sendCommand("betterwhatsapp:window:hide"), "Janela enviada para a bandeja");
    } else if (message.command === "maximise") {
      void runWindowAction(() => sendCommand("betterwhatsapp:window:maximise"), "Estado da janela alternado");
    } else if (message.command === "close") {
      void runWindowAction(() => sendCommand("betterwhatsapp:window:close"), "Janela enviada para a bandeja");
    }
  } else if (message.type === "focus-section" && message.section === "plugins") {
    document.getElementById(message.section + "-section")?.scrollIntoView({
      behavior: "smooth",
      block: "start",
    });
  }
});

void refreshState();
