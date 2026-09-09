import { Service } from "../bindings/betterwhatsapp/internal/appservice/index.js";
import type {
  AppState,
  ProfileInfo,
} from "../bindings/betterwhatsapp/internal/model/models.js";
import betterWhatsAppLogo from "./assets/betterwhatsapp-logo.png";

type WailsBridge = {
  invoke: (message: string) => unknown;
};

declare global {
  interface Window {
    _wails?: WailsBridge;
  }
}

const rootElement = document.querySelector<HTMLDivElement>("#app");
if (!rootElement) {
  throw new Error("BetterWhatsApp shell root was not found");
}
const root: HTMLDivElement = rootElement;

root.innerHTML = `<div class="tabs-shell">
  <header class="tabs-titlebar" data-drag-region="true">
    <div class="tabs-brand" data-drag-region="true">
      <img src="${betterWhatsAppLogo}" alt="BetterWhatsApp" />
      <div>
        <strong>BETTER<span>WHATSAPP</span></strong>
        <small>ISOLATED SESSION HOST</small>
      </div>
    </div>
    <div class="tabs-runtime" data-drag-region="true">
      <span class="tabs-runtime-dot"></span>
      <span>LOCAL SHELL</span>
      <i></i>
      <span>REMOTE PROFILES</span>
    </div>
    <div class="tabs-title-actions">
      <button class="tabs-window-button" data-window-action="minimise" type="button" aria-label="Enviar para a bandeja" title="Enviar para a bandeja">−</button>
      <button class="tabs-window-button" data-window-action="maximise" type="button" aria-label="Maximizar janela" title="Maximizar janela">□</button>
      <button class="tabs-window-button tabs-window-close" data-window-action="close" type="button" aria-label="Enviar para a bandeja" title="Enviar para a bandeja">×</button>
    </div>
  </header>

  <section class="profile-tabbar">
    <div class="profile-tabbar-label">
      <span class="tabs-kicker">WORKSPACES</span>
      <strong>WhatsApp profiles</strong>
    </div>
    <nav class="profile-tabs" id="profile-tabs" aria-label="Perfis do WhatsApp"></nav>
    <button class="profile-add-button" id="add-profile" type="button" title="Criar um novo perfil">
      <span>+</span><strong>Novo perfil</strong>
    </button>
    <div class="tabs-shell-actions">
      <button class="tabs-shell-action tabs-shell-action-active" data-shell-action="control" type="button"><span>⌘</span> Control</button>
      <button class="tabs-shell-action" data-shell-action="plugins" type="button"><span>✦</span> Plugins</button>
      <button class="tabs-shell-action" data-shell-action="themes" type="button"><span>◌</span> Themes</button>
      <button class="tabs-shell-action tabs-shell-action-icon" data-shell-action="reload" type="button" aria-label="Recarregar todos os perfis" title="Recarregar todos os perfis">↻</button>
    </div>
  </section>

  <main class="tabs-stage" id="profile-stage">
    <div class="tabs-stage-placeholder">
      <div class="tabs-stage-mark"><img src="${betterWhatsAppLogo}" alt="" /></div>
      <span class="tabs-kicker">PROFILE RUNTIME</span>
      <h1 id="stage-title">Preparando o WhatsApp Web</h1>
      <p id="stage-copy">Cada guia usa uma sessão WebView2 independente. O conteúdo remoto ocupa esta área quando o processo do perfil estiver pronto.</p>
      <div class="tabs-isolation-note"><span></span><strong>isolated process</strong><small>plugins e login permanecem nesta seção</small></div>
    </div>
  </main>

  <footer class="tabs-statusbar">
    <div class="tabs-status" aria-live="polite">
      <span class="tabs-status-dot"></span>
      <span id="shell-status">Carregando perfis…</span>
    </div>
    <div class="tabs-active-summary">
      <span class="tabs-kicker">ACTIVE PROFILE</span>
      <strong id="active-profile-summary">—</strong>
      <span class="tabs-security-mark">● isolated</span>
    </div>
  </footer>

  <div class="profile-dialog-backdrop" id="profile-dialog" role="dialog" aria-modal="true" aria-labelledby="profile-dialog-title" hidden>
    <form class="profile-dialog" id="profile-form">
      <div class="profile-dialog-heading">
        <span class="tabs-kicker">NEW ISOLATED SECTION</span>
        <h2 id="profile-dialog-title">Adicionar perfil</h2>
        <p>O nome identifica a guia. O login do WhatsApp será feito dentro da nova sessão.</p>
      </div>
      <label class="profile-field">
        <span>Nome do perfil</span>
        <input id="profile-name-input" name="name" type="text" maxlength="80" placeholder="Ex.: Emprego" autocomplete="off" required />
      </label>
      <div class="profile-dialog-actions">
        <button class="tabs-dialog-button" id="profile-cancel" type="button">Cancelar</button>
        <button class="tabs-dialog-button tabs-dialog-button-primary" type="submit">Criar seção</button>
      </div>
    </form>
  </div>
</div>`;

let currentState: AppState | null = null;
let pending = false;

const query = <T extends Element>(selector: string): T => {
  const element = root.querySelector<T>(selector);
  if (!element) {
    throw new Error("Missing shell element: " + selector);
  }
  return element;
};

function setStatus(message: string, tone: "neutral" | "success" | "error" = "neutral") {
  const element = query<HTMLElement>("#shell-status");
  element.textContent = message;
  element.dataset.tone = tone;
}

function setBusy(value: boolean) {
  pending = value;
  root.querySelector(".tabs-shell")?.setAttribute("data-busy", String(value));
  root.querySelectorAll<HTMLButtonElement | HTMLInputElement>("button, input").forEach((element) => {
    element.disabled = value;
  });
}

function invokeNative(message: string) {
  try {
    void Promise.resolve(window._wails?.invoke(message));
  } catch (error) {
    console.warn("[BetterWhatsApp] native command failed", error);
  }
}

function isDragTarget(target: EventTarget | null) {
  return target instanceof Element &&
    Boolean(target.closest("[data-drag-region]")) &&
    !target.closest("button, input, select, textarea, a");
}

function bindFrame() {
  const titlebar = query<HTMLElement>(".tabs-titlebar");
  titlebar.addEventListener("mousedown", (event) => {
    if (event.button !== 0 || !isDragTarget(event.target)) {
      return;
    }
    event.preventDefault();
    invokeNative("wails:drag");
  });
  titlebar.addEventListener("dblclick", (event) => {
    if (!isDragTarget(event.target)) {
      return;
    }
    event.preventDefault();
    void Service.ToggleMaximise();
  });

  root.querySelectorAll<HTMLButtonElement>("[data-window-action]").forEach((button) => {
    button.addEventListener("click", () => {
      switch (button.dataset.windowAction) {
        case "minimise":
          void Service.HideWindow();
          break;
        case "maximise":
          void Service.ToggleMaximise();
          break;
        case "close":
          void Service.RequestClose();
          break;
      }
    });
  });
}

function renderProfiles(profiles: ProfileInfo[], activeID: string) {
  const navigation = query<HTMLElement>("#profile-tabs");
  navigation.replaceChildren();
  const canDelete = profiles.length > 1;

  profiles.forEach((profile) => {
    const tab = document.createElement("button");
    tab.type = "button";
    tab.className = "profile-tab";
    tab.dataset.active = String(profile.id === activeID);
    tab.dataset.profileID = profile.id;
    tab.style.setProperty("--profile-accent", profile.accent || "#50e58b");
    tab.title = "Abrir perfil " + profile.name;

    const mark = document.createElement("span");
    mark.className = "profile-tab-mark";
    mark.textContent = profile.name.slice(0, 1).toUpperCase();
    const copy = document.createElement("span");
    copy.className = "profile-tab-copy";
    const name = document.createElement("strong");
    name.textContent = profile.name;
    const meta = document.createElement("small");
    meta.textContent = profile.id === activeID ? "ACTIVE · ISOLATED" : "ISOLATED";
    copy.append(name, meta);
    tab.append(mark, copy);

    if (canDelete) {
      const close = document.createElement("span");
      close.className = "profile-tab-close";
      close.setAttribute("role", "button");
      close.setAttribute("aria-label", "Excluir " + profile.name);
      close.textContent = "×";
      close.addEventListener("click", (event) => {
        event.stopPropagation();
        void deleteProfile(profile);
      });
      tab.append(close);
    }

    tab.addEventListener("click", () => void selectProfile(profile));
    navigation.append(tab);
  });

  query<HTMLElement>("#active-profile-summary").textContent =
    profiles.find((profile) => profile.id === activeID)?.name ?? "—";
}

function renderStage(profiles: ProfileInfo[], activeID: string) {
  const active = profiles.find((profile) => profile.id === activeID);
  if (!active) {
    return;
  }
  query<HTMLElement>("#stage-title").textContent = active.name;
  query<HTMLElement>("#stage-copy").textContent =
    "Sessão isolada de " + active.name + ". O login, cache, IndexedDB e plugins efetivos desta guia não são compartilhados com os outros perfis.";
}

function renderState(nextState: AppState) {
  const profiles = nextState.profiles ?? [];
  renderProfiles(profiles, nextState.activeProfileId);
  renderStage(profiles, nextState.activeProfileId);
}

async function refreshState() {
  currentState = await Service.GetState();
  renderState(currentState);
}

async function runAction(label: string, action: () => Promise<unknown>) {
  if (pending) {
    return;
  }
  setBusy(true);
  setStatus(label);
  try {
    await action();
    await refreshState();
    setStatus("Shell sincronizada", "success");
  } catch (error) {
    setStatus(error instanceof Error ? error.message : String(error), "error");
  } finally {
    setBusy(false);
  }
}

async function selectProfile(profile: ProfileInfo) {
  if (pending || currentState?.activeProfileId === profile.id) {
    return;
  }
  await runAction("Abrindo " + profile.name + "…", () => Service.SelectProfile(profile.id));
}

async function deleteProfile(profile: ProfileInfo) {
  if (pending || !window.confirm("Excluir a seção " + profile.name + "? O login ficará reservado no disco, mas a guia será removida da configuração.")) {
    return;
  }
  await runAction("Removendo " + profile.name + "…", () => Service.DeleteProfile(profile.id));
}

async function createProfile(name: string) {
  if (!name || pending) {
    return;
  }
  await runAction("Criando seção isolada…", async () => {
    const profile = await Service.CreateProfile(name);
    await Service.SelectProfile(profile.id);
  });
}

function closeProfileDialog() {
  const backdrop = query<HTMLDivElement>("#profile-dialog");
  backdrop.hidden = true;
  root.querySelector(".tabs-shell")?.removeAttribute("data-modal-open");
}

function openProfileDialog() {
  const backdrop = query<HTMLDivElement>("#profile-dialog");
  const input = query<HTMLInputElement>("#profile-name-input");

  backdrop.hidden = false;
  root.querySelector(".tabs-shell")?.setAttribute("data-modal-open", "true");
  input.value = "Emprego";
  input.focus();
  input.select();
}

function bindProfileDialog() {
  const backdrop = query<HTMLDivElement>("#profile-dialog");
  const form = query<HTMLFormElement>("#profile-form");
  const input = query<HTMLInputElement>("#profile-name-input");

  query<HTMLButtonElement>("#add-profile").addEventListener("click", openProfileDialog);
  query<HTMLButtonElement>("#profile-cancel").addEventListener("click", closeProfileDialog);

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const name = input.value.trim();
    if (!name) {
      input.focus();
      return;
    }

    closeProfileDialog();
    void createProfile(name);
  });

  backdrop.addEventListener("click", (event) => {
    if (event.target === backdrop) {
      closeProfileDialog();
    }
  });

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && !backdrop.hidden) {
      closeProfileDialog();
    }
  });
}

function bindShellActions() {
  root.querySelectorAll<HTMLButtonElement>("[data-shell-action]").forEach((button) => {
    button.addEventListener("click", () => {
      const action = button.dataset.shellAction;
      if (action === "plugins" || action === "themes") {
        void runAction("Abrindo " + action + "…", () => Service.OpenSurface(action));
      } else if (action === "reload") {
        void runAction("Recarregando todos os perfis…", () => Service.ReloadWhatsApp());
      } else if (action === "control") {
        setStatus("A shell local é o painel de controle", "success");
      }
    });
  });
}

async function mount() {
  bindFrame();
  bindProfileDialog();
  bindShellActions();
  try {
    await refreshState();
    setStatus("Perfis isolados prontos", "success");
  } catch (error) {
    setStatus(error instanceof Error ? error.message : String(error), "error");
  }
}

void mount();
