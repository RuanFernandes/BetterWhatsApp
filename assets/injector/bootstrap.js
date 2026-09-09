
(() => {
  "use strict";

  const payload = window.__BETTER_WHATSAPP_INJECTOR__;
  const marker = "__BETTER_WHATSAPP_RUNTIME__";
  const isWhatsAppHost =
    location.protocol === "https:" &&
    location.hostname === "web.whatsapp.com";

  const logoSource =
    typeof payload?.logo === "string" &&
    /^data:image\/png;base64,[A-Za-z0-9+/=]+$/.test(payload.logo)
      ? payload.logo
      : "";

  if (window[marker] || !isWhatsAppHost || !payload) {
    return;
  }

  const styleNodes = new Map();
  const pluginFactories = [];
  let wppReady = false;
  let sendNativeMessage = () => false;

  const log = (message, error) => {
    if (error) {
      console.error("[BetterWhatsApp]", message, error);
      return;
    }
    console.info("[BetterWhatsApp]", message);
  };

  const addStyle = (id, cssText) => {
    if (!id || typeof cssText !== "string") {
      return;
    }

    const apply = () => {
      let style = styleNodes.get(id);
      if (!style) {
        style = document.createElement("style");
        style.dataset.betterWhatsapp = id;
        styleNodes.set(id, style);
        document.head.appendChild(style);
      }
      style.textContent = cssText;
    };

    if (document.head) {
      apply();
    } else {
      document.addEventListener("DOMContentLoaded", apply, { once: true });
    }
  };

  const observe = (selector, callback) => {
    const observer = new MutationObserver(() => {
      const element = document.querySelector(selector);
      if (element) {
        callback(element);
      }
    });
    observer.observe(document.documentElement, {
      childList: true,
      subtree: true,
    });
    return () => observer.disconnect();
  };

  const installImagePasteSupport = () => {
    let syntheticPasteDispatch = false;

    const isVisible = (element) => {
      if (!(element instanceof HTMLElement)) {
        return false;
      }
      const styles = window.getComputedStyle(element);
      return (
        styles.display !== "none" &&
        styles.visibility !== "hidden" &&
        element.getClientRects().length > 0
      );
    };

    const isMessageComposer = (element) => {
      if (
        !(element instanceof Element) ||
        !element.matches('[contenteditable="true"]')
      ) {
        return false;
      }

      if (element.getAttribute("data-tab") === "10") {
        return true;
      }

      if (
        element.closest(
          "footer, [data-testid='conversation-compose-box-input'], [data-testid='conversation-compose-footer']",
        )
      ) {
        return true;
      }

      const marker = [
        element.getAttribute("aria-label"),
        element.getAttribute("aria-placeholder"),
        element.getAttribute("data-placeholder"),
        element.getAttribute("title"),
        element.getAttribute("role"),
      ]
        .filter(Boolean)
        .join(" ");

      return /mensagem|message|digite.*mensagem|type.*message/i.test(marker);
    };

    const resolveMessageComposer = (target) => {
      const candidates = [];
      const addCandidate = (element) => {
        if (element && !candidates.includes(element)) {
          candidates.push(element);
        }
      };

      if (target instanceof Element) {
        addCandidate(target.closest('[contenteditable="true"]'));
      }
      if (document.activeElement instanceof Element) {
        addCandidate(
          document.activeElement.closest('[contenteditable="true"]'),
        );
      }
      document
        .querySelectorAll('[contenteditable="true"]')
        .forEach(addCandidate);

      return (
        candidates.find(
          (element) => isVisible(element) && isMessageComposer(element),
        ) || null
      );
    };

    const getImageFromClipboardData = (clipboardData) => {
      if (!clipboardData) {
        return null;
      }

      const items = clipboardData.items
        ? Array.from(clipboardData.items)
        : [];
      for (const item of items) {
        if (item.kind !== "file" || !/^image\//i.test(item.type || "")) {
          continue;
        }
        const file = item.getAsFile?.();
        if (file) {
          return file;
        }
      }

      const files = clipboardData.files
        ? Array.from(clipboardData.files)
        : [];
      return files.find((file) => /^image\//i.test(file.type || "")) || null;
    };

    const readImageFromClipboard = async () => {
      if (
        !navigator.clipboard ||
        typeof navigator.clipboard.read !== "function"
      ) {
        return null;
      }

      const clipboardItems = await navigator.clipboard.read();
      for (const item of clipboardItems) {
        const imageType = item.types.find((type) =>
          /^image\//i.test(type),
        );
        if (!imageType) {
          continue;
        }

        const blob = await item.getType(imageType);
        if (!blob) {
          continue;
        }

        const mimeType = blob.type || imageType;
        const extension =
          mimeType.split("/")[1]?.replace(/[^a-z0-9]/gi, "") || "png";
        if (typeof File === "function") {
          return new File(
            [blob],
            "betterwhatsapp-pasted-image." + extension,
            { type: mimeType },
          );
        }
        return blob;
      }

      return null;
    };

    const createPasteEvent = (file) => {
      if (typeof DataTransfer !== "function") {
        return null;
      }

      const transfer = new DataTransfer();
      try {
        transfer.items.add(file);
      } catch {
        return null;
      }

      let pasteEvent;
      try {
        pasteEvent = new ClipboardEvent("paste", {
          bubbles: true,
          cancelable: true,
          clipboardData: transfer,
        });
      } catch {
        pasteEvent = new Event("paste", {
          bubbles: true,
          cancelable: true,
        });
      }

      try {
        Object.defineProperty(pasteEvent, "clipboardData", {
          configurable: true,
          value: transfer,
        });
      } catch {
        // ClipboardEvent already exposed a read-only data transfer.
      }
      return pasteEvent;
    };

    const dispatchImagePaste = (composer, file) => {
      const pasteEvent = createPasteEvent(file);
      if (!pasteEvent) {
        return false;
      }

      try {
        composer.focus({ preventScroll: true });
      } catch {
        composer.focus();
      }

      syntheticPasteDispatch = true;
      try {
        composer.dispatchEvent(pasteEvent);
        return true;
      } finally {
        syntheticPasteDispatch = false;
      }
    };

    document.addEventListener(
      "paste",
      (event) => {
        if (syntheticPasteDispatch || event.isTrusted === false) {
          return;
        }

        const composer = resolveMessageComposer(event.target);
        if (!composer) {
          return;
        }

        const directImage = getImageFromClipboardData(event.clipboardData);
        if (directImage) {
          event.preventDefault();
          event.stopImmediatePropagation();
          if (!dispatchImagePaste(composer, directImage)) {
            log("could not dispatch clipboard image to WhatsApp");
          }
          return;
        }

        const items = event.clipboardData?.items
          ? Array.from(event.clipboardData.items)
          : [];
        if (
          items.some((item) => item.kind === "string") ||
          !navigator.clipboard ||
          typeof navigator.clipboard.read !== "function"
        ) {
          return;
        }

        event.preventDefault();
        event.stopImmediatePropagation();
        void readImageFromClipboard()
          .then((file) => {
            if (file && !dispatchImagePaste(composer, file)) {
              log("could not dispatch clipboard fallback to WhatsApp");
            }
          })
          .catch((error) => {
            log("clipboard image fallback failed", error);
          });
      },
      true,
    );
  };

  const installWailsBridgeGuard = () => {
    let nativePostMessage = null;
    const maxThemeMessageLength = 2 * 1024 * 1024 + 64 * 1024;
    const getWebviewPostMessage = () => {
      const webview = window.chrome?.webview;
      if (!webview || typeof webview.postMessage !== "function") {
        return null;
      }
      return webview.postMessage.bind(webview);
    };
    const webview = window.chrome?.webview;
    nativePostMessage = getWebviewPostMessage();

    const isAllowedThemeMessage = (message) => {
      if (typeof message !== "string" || message.length > maxThemeMessageLength || message[0] !== "{") {
        return false;
      }
      let command;
      try {
        command = JSON.parse(message);
      } catch {
        return false;
      }
      if (
        !command ||
        command.source !== "betterwhatsapp-theme-editor" ||
        typeof command.requestId !== "string" ||
        !/^[A-Za-z0-9_-]{1,64}$/.test(command.requestId)
      ) {
        return false;
      }
      if (!["create", "read", "update", "delete"].includes(command.action)) {
        return false;
      }
      if (command.action === "create") {
        return typeof command.name === "string" && typeof command.css === "string";
      }
      if (command.action === "update") {
        return (
          typeof command.id === "string" &&
          /^[a-z0-9][a-z0-9._-]{0,63}$/.test(command.id) &&
          typeof command.name === "string" &&
          typeof command.css === "string"
        );
      }
      return (
        typeof command.id === "string" &&
        /^[a-z0-9][a-z0-9._-]{0,63}$/.test(command.id)
      );
    };

    const isAllowedNativeMessage = (message) => {
      if (typeof message !== "string" || message.length > maxThemeMessageLength) {
        return false;
      }
      return (
        message === "wails:runtime:ready" ||
        message === "wails:drag" ||
        message === "wails:drag:doubleclick" ||
        /^wails:resize:(?:n|s|e|w|ne|nw|se|sw)-resize$/.test(message) ||
        /^betterwhatsapp:window:(?:hide|maximise|close|reload)$/.test(message) ||
        message === "betterwhatsapp:surface:plugins" ||
        message === "betterwhatsapp:surface:themes" ||
        message === "betterwhatsapp:notifications:new-message" ||
        /^betterwhatsapp:notifications:unread:\d{1,6}$/.test(message) ||
        /^betterwhatsapp:settings:(?:injector:[01]|plugin:[a-z0-9][a-z0-9._-]{0,63}:[01]|theme:[a-z0-9][a-z0-9._-]{0,63}:[01])$/.test(message) ||
        isAllowedThemeMessage(message)
      );
    };

    const deniedInvoke = () => {
      console.warn("[BetterWhatsApp] blocked host bridge call from remote window");
      return false;
    };

    const guardedInvoke = (message) => {
      if (!isAllowedNativeMessage(message)) {
        return deniedInvoke();
      }
      try {
        const postMessage = nativePostMessage || getWebviewPostMessage();
        if (!postMessage) {
          return deniedInvoke();
        }
        nativePostMessage = postMessage;
        postMessage(message);
        return true;
      } catch (error) {
        console.warn("[BetterWhatsApp] native message failed", error);
        return false;
      }
    };

    sendNativeMessage = guardedInvoke;

    const protectedBridgeTarget = Object.create(null);
    protectedBridgeTarget.flags = Object.create(null);
    protectedBridgeTarget.environment = Object.create(null);

    const protectedBridge = new Proxy(protectedBridgeTarget, {
      get(target, property) {
        if (property === "invoke") {
          return guardedInvoke;
        }
        return Reflect.get(target, property);
      },
      set(target, property, value) {
        if (property !== "invoke") {
          Reflect.set(target, property, value);
        }
        return true;
      },
      defineProperty(target, property, descriptor) {
        if (property !== "invoke") {
          Reflect.defineProperty(target, property, descriptor);
        }
        return true;
      },
    });

    try {
      Object.defineProperty(window, "_wails", {
        configurable: false,
        enumerable: false,
        get: () => protectedBridge,
        set: () => {},
      });
    } catch {
      try {
        const existingBridge = window._wails;
        if (existingBridge && typeof existingBridge === "object") {
          Object.defineProperty(existingBridge, "invoke", {
            configurable: false,
            enumerable: false,
            writable: false,
            value: guardedInvoke,
          });
        }
      } catch (error) {
        console.warn("[BetterWhatsApp] could not guard the Wails bridge", error);
      }
    }

    if (webview && nativePostMessage) {
      try {
        Object.defineProperty(webview, "postMessage", {
          configurable: false,
          enumerable: true,
          writable: false,
          value: (message) => {
            if (!isAllowedNativeMessage(message)) {
              console.warn("[BetterWhatsApp] blocked remote WebView message");
              return undefined;
            }
            return nativePostMessage(message);
          },
        });
      } catch {
        // Native WebView objects may reject redefining postMessage.
      }
    }
  };

  const mountControlSurface = () => {
    const toolbarStyle = `
      :root {
        --bw-toolbar-height: 42px !important;
      }

      html,
      body {
        height: 100% !important;
        min-height: 100% !important;
        overflow: hidden !important;
      }

      body {
        position: relative !important;
        padding-top: 0 !important;
        box-sizing: border-box !important;
      }

      #app {
        position: absolute !important;
        top: var(--bw-toolbar-height) !important;
        right: 0 !important;
        bottom: 0 !important;
        left: 0 !important;
        width: auto !important;
        height: auto !important;
        margin: 0 !important;
        min-height: 0 !important;
        box-sizing: border-box !important;
      }

      .bw-toolbar,
      .bw-toolbar * {
        box-sizing: border-box;
      }

      .bw-toolbar {
        position: fixed;
        z-index: 2147483000;
        top: 0;
        right: 0;
        left: 0;
        display: flex;
        align-items: center;
        gap: 16px;
        height: var(--bw-toolbar-height);
        padding: 0 12px;
        border-bottom: 1px solid rgba(139, 176, 157, 0.16);
        color: #e7f2eb;
        background: #0a100e;
        box-shadow: 0 8px 24px rgba(0, 0, 0, 0.2);
        font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
        user-select: none;
        --wails-draggable: drag;
      }

      .bw-brand,
      .bw-context,
      .bw-actions {
        display: flex;
        align-items: center;
      }

      .bw-brand {
        flex: 0 0 auto;
        gap: 9px;
        min-width: 166px;
        color: #e7f2eb;
        font-size: 11px;
        font-weight: 750;
        letter-spacing: 0.055em;
      }

      .bw-logo {
        display: block;
        width: 24px;
        height: 24px;
        flex: 0 0 24px;
        object-fit: contain;
      }

      .bw-brand-caption {
        color: #769083;
        font-size: 8px;
        font-weight: 650;
        letter-spacing: 0.14em;
      }

      .bw-context {
        flex: 1 1 auto;
        gap: 8px;
        min-width: 0;
        color: #87a095;
        font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
        font-size: 8px;
        letter-spacing: 0.12em;
        white-space: nowrap;
      }

      .bw-live-dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: #50e58b;
        box-shadow: 0 0 0 4px rgba(80, 229, 139, 0.11);
      }

      .bw-separator {
        width: 1px;
        height: 12px;
        background: rgba(139, 176, 157, 0.2);
      }

      .bw-actions {
        flex: 0 0 auto;
        gap: 4px;
        --wails-draggable: no-drag;
      }

      .bw-action {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: 6px;
        min-width: 31px;
        height: 28px;
        padding: 0 9px;
        border: 1px solid transparent;
        border-radius: 6px;
        color: #99b2a4;
        background: transparent;
        cursor: pointer;
        font: inherit;
        font-size: 10px;
        font-weight: 700;
        line-height: 1;
        --wails-draggable: no-drag;
      }

      .bw-action:hover,
      .bw-action:focus-visible {
        border-color: rgba(80, 229, 139, 0.28);
        color: #effaf2;
        background: rgba(80, 229, 139, 0.1);
        outline: none;
      }

      .bw-action.bw-primary {
        border-color: rgba(80, 229, 139, 0.34);
        color: #06110a;
        background: #50e58b;
      }

      .bw-action.bw-primary:hover,
      .bw-action.bw-primary:focus-visible {
        border-color: #75f3a6;
        color: #06110a;
        background: #75f3a6;
      }

      .bw-window-action {
        width: 29px;
        padding: 0;
        color: #91a79a;
        font-size: 14px;
        font-weight: 500;
      }

      .bw-close:hover,
      .bw-close:focus-visible {
        border-color: rgba(255, 127, 117, 0.45);
        color: #fff3f0;
        background: rgba(255, 127, 117, 0.18);
      }

      .bw-panel {
        position: fixed;
        z-index: 2147482999;
        top: var(--bw-toolbar-height);
        right: 0;
        bottom: 0;
        width: min(438px, calc(100vw - 18px));
        overflow: hidden;
        border-left: 1px solid rgba(139, 176, 157, 0.2);
        background: #0d1512;
        box-shadow: -18px 0 40px rgba(0, 0, 0, 0.32);
        opacity: 0;
        pointer-events: none;
        transform: translateX(18px);
        transition: opacity 150ms ease, transform 150ms ease;
      }

      .bw-panel.is-open {
        opacity: 1;
        pointer-events: auto;
        transform: translateX(0);
      }

      .bw-panel > .bw-control-surface {
        width: 100%;
        height: 100%;
        min-height: 0;
        overflow: hidden;
        background: #0d1512;
      }

      @media (max-width: 760px) {
        .bw-context {
          display: none;
        }

        .bw-brand {
          min-width: auto;
        }

        .bw-action-label {
          display: none;
        }
      }

      @media (prefers-reduced-motion: reduce) {
        .bw-panel {
          transition: none;
        }
      }
    `;

    addStyle("betterwhatsapp-shell", toolbarStyle);

    const mount = () => {
      if (!document.body || document.getElementById("betterwhatsapp-toolbar")) {
        return;
      }

      document.documentElement.style.setProperty("--bw-toolbar-height", "42px");

      const toolbar = document.createElement("header");
      toolbar.id = "betterwhatsapp-toolbar";
      toolbar.className = "bw-toolbar";
      const logoMarkup = logoSource
        ? `<img class="bw-logo" src="${logoSource}" alt="BetterWhatsApp" />`
        : "";
      toolbar.innerHTML = `
        <div class="bw-brand">
          ${logoMarkup}
          <span>BETTER<span class="bw-brand-caption">WHATSAPP</span></span>
        </div>
        <div class="bw-context">
          <span class="bw-live-dot"></span>
          <span>WHATSAPP WEB</span>
          <span class="bw-separator"></span>
          <span>${payload.enabled === true ? "INJECTOR LIVE" : "INJECTOR PAUSED"}</span>
        </div>
        <div class="bw-actions">
          <button class="bw-action bw-primary" data-bw-action="panel" type="button">
            <span>⌘</span><span class="bw-action-label">Control</span>
          </button>
          <button class="bw-action" data-bw-action="plugins" type="button">
            <span>✦</span><span class="bw-action-label">Plugins</span>
          </button>
          <button class="bw-action" data-bw-action="themes" type="button">
            <span>◌</span><span class="bw-action-label">Themes</span>
          </button>
          <button class="bw-action" data-bw-action="reload" type="button" title="Recarregar o WhatsApp Web">
            <span>↻</span><span class="bw-action-label">Reload</span>
          </button>
          <button class="bw-action bw-window-action" data-bw-command="minimise" type="button" aria-label="Enviar para a bandeja" title="Enviar para a bandeja">−</button>
          <button class="bw-action bw-window-action" data-bw-command="maximise" type="button" aria-label="Maximizar janela" title="Maximizar janela">□</button>
          <button class="bw-action bw-window-action bw-close" data-bw-command="close" type="button" aria-label="Fechar para a bandeja" title="Fechar para a bandeja">×</button>
        </div>
      `;

      const panel = document.createElement("aside");
      panel.id = "betterwhatsapp-control-panel";
      panel.className = "bw-panel";
      panel.setAttribute("aria-label", "Controles locais do BetterWhatsApp");

      const mount = document.createElement("div");
      mount.id = "betterwhatsapp-panel-app";
      mount.className = "bw-control-surface";
      panel.appendChild(mount);

      document.body.append(toolbar, panel);

      const setPanelOpen = (open) => {
        panel.classList.toggle("is-open", open);
        toolbar
          .querySelector('[data-bw-action="panel"]')
          ?.setAttribute("aria-expanded", String(open));
      };

      const sendPanelMessage = (type, extra = {}) => {
        window.postMessage(
          {
            source: "betterwhatsapp-navbar",
            type,
            ...extra,
          },
          location.origin,
        );
      };

      const sendWindowCommand = (command) => {
        const nativeCommand = command === "minimise" ? "hide" : command;
        if (nativeCommand === "hide" || nativeCommand === "maximise" || nativeCommand === "close" || nativeCommand === "reload") {
          sendNativeMessage("betterwhatsapp:window:" + nativeCommand);
        }
      };

      const isToolbarDragTarget = (target) => {
        if (!(target instanceof Element) || !target.closest("#betterwhatsapp-toolbar")) {
          return false;
        }
        return !target.closest("button, input, select, textarea, a, [data-bw-no-drag]");
      };

      toolbar.addEventListener("mousedown", (event) => {
        if (event.button !== 0 || !isToolbarDragTarget(event.target)) {
          return;
        }
        event.preventDefault();
        sendNativeMessage("wails:drag");
      });

      toolbar.addEventListener("dblclick", (event) => {
        if (!isToolbarDragTarget(event.target)) {
          return;
        }
        event.preventDefault();
        sendNativeMessage("wails:drag:doubleclick");
      });

      const control = payload.control;
      const controlState =
        control && control.state && typeof control.state === "object"
          ? control.state
          : {};

      window.BetterWhatsAppControl = Object.freeze({
        state: controlState,
        command: (message) => sendNativeMessage(message),
      });

      if (control && typeof control.style === "string" && control.style.trim()) {
        addStyle("betterwhatsapp-control", control.style);
      }

      /* BETTERWHATSAPP_CONTROL_SURFACE */

      toolbar.addEventListener("click", (event) => {
        const target = event.target;
        if (!(target instanceof Element)) {
          return;
        }
        const action = target.closest("[data-bw-action]")?.getAttribute("data-bw-action");
        const command = target.closest("[data-bw-command]")?.getAttribute("data-bw-command");

        if (action === "panel") {
          setPanelOpen(!panel.classList.contains("is-open"));
        } else if (action === "plugins") {
          setPanelOpen(false);
          sendNativeMessage("betterwhatsapp:surface:plugins");
        } else if (action === "themes") {
          setPanelOpen(false);
          sendNativeMessage("betterwhatsapp:surface:themes");
        } else if (action === "reload") {
          sendWindowCommand("reload");
        } else if (command) {
          sendWindowCommand(command);
        }
      });

      window.addEventListener("message", (event) => {
        if (event.source !== window) {
          return;
        }
        const message = event.data;
        if (!message || message.source !== "betterwhatsapp-panel") {
          return;
        }
        if (message.type === "hide-panel") {
          setPanelOpen(false);
        } else if (message.type === "reload-whatsapp") {
          sendWindowCommand("reload");
        }
      });
    };

    if (document.body) {
      mount();
    } else {
      document.addEventListener("DOMContentLoaded", mount, { once: true });
    }
  };

  installWailsBridgeGuard();

  const api = Object.freeze({
    version: "0.1.0",
    get WPP() {
      return window.WPP;
    },
    addStyle,
    observe,
    log,
  });

  const runFactory = (id, factory) => {
    try {
      factory(api);
      log("plugin ready: " + id);
    } catch (error) {
      log("plugin failed: " + id, error);
    }
  };

  const registerPlugin = (id, factory) => {
    if (typeof id !== "string" || typeof factory !== "function") {
      throw new TypeError("BetterWhatsApp.registerPlugin expects an id and a factory");
    }
    pluginFactories.push({ id, factory });
    if (wppReady) {
      runFactory(id, factory);
    }
  };

  const host = Object.freeze({
    version: api.version,
    get WPP() {
      return window.WPP;
    },
    addStyle,
    observe,
    log,
    registerPlugin,
  });

  window.BetterWhatsApp = host;
  window[marker] = true;
  try {
    delete window.__BETTER_WHATSAPP_INJECTOR__;
  } catch {
    window.__BETTER_WHATSAPP_INJECTOR__ = undefined;
  }

  if (payload.showToolbar !== false) {
    mountControlSurface();
  }

  if (payload.enabled !== true) {
    log("injector disabled");
    return;
  }

  installImagePasteSupport();

  const themes = Array.isArray(payload.themes) ? payload.themes : [];
  const plugins = Array.isArray(payload.plugins) ? payload.plugins : [];

  for (let index = 0; index < themes.length; index += 1) {
    addStyle("theme-" + index, themes[index]);
  }

  if (typeof payload.wpp === "string" && payload.wpp.trim()) {
    try {
      (0, eval)(payload.wpp);
    } catch (error) {
      log("WA-JS failed to load", error);
    }
  }

  for (let index = 0; index < plugins.length; index += 1) {
    try {
      (0, eval)(plugins[index]);
    } catch (error) {
      log("plugin source failed at index " + index, error);
    }
  }

  const installUnreadMonitor = (wpp) => {
    if (
      !wpp ||
      typeof wpp.on !== "function" ||
      typeof wpp.chat?.getUnreadChats !== "function"
    ) {
      log("unread monitor is unavailable in this WPP build");
      return;
    }

    let refreshTimer = 0;
    let refreshInFlight = false;
    let refreshQueued = false;
    let baselineLoaded = false;

    const publishUnreadCount = (value) => {
      const numericValue = Number(value);
      const count = Number.isFinite(numericValue)
        ? Math.max(0, Math.min(999999, Math.floor(numericValue)))
        : 0;
      sendNativeMessage("betterwhatsapp:notifications:unread:" + count);
    };

    const isArchivedChat = (chat) => chat?.archive === true;

    const readUnreadCount = async () => {
      const chats = await wpp.chat.getUnreadChats(false);
      if (!Array.isArray(chats)) {
        publishUnreadCount(0);
        return;
      }

      let total = 0;
      for (let index = 0; index < chats.length; index += 1) {
        const chat = chats[index];
        if (isArchivedChat(chat)) {
          continue;
        }

        const unreadCount = Number(chat?.unreadCount);
        total += Number.isFinite(unreadCount) && unreadCount > 0
          ? Math.floor(unreadCount)
          : 1;
      }
      publishUnreadCount(total);
    };

    const refresh = async () => {
      if (refreshInFlight) {
        refreshQueued = true;
        return;
      }

      refreshInFlight = true;
      try {
        await readUnreadCount();
        baselineLoaded = true;
      } catch (error) {
        log("unread count refresh failed", error);
      } finally {
        refreshInFlight = false;
        if (refreshQueued) {
          refreshQueued = false;
          scheduleRefresh();
        }
      }
    };

    const scheduleRefresh = () => {
      if (refreshTimer) {
        window.clearTimeout(refreshTimer);
      }
      refreshTimer = window.setTimeout(() => {
        refreshTimer = 0;
        void refresh();
      }, 180);
    };

    try {
      wpp.on("chat.unread_count_changed", scheduleRefresh);
      wpp.on("chat.new_message", (message) => {
        const fromMe = message?.fromMe === true || message?.id?.fromMe === true;
        if (!fromMe && message?.isNotification !== true) {
          if (baselineLoaded) {
            sendNativeMessage("betterwhatsapp:notifications:new-message");
          }
          scheduleRefresh();
        }
      });
    } catch (error) {
      log("unread monitor could not subscribe to WPP events", error);
      return;
    }

    void refresh();
    window.setInterval(scheduleRefresh, 30000);
  };

  const waitForWPP = () =>
    new Promise((resolve, reject) => {
      let attempts = 0;
      let settled = false;
      let readyHookRegistered = false;
      let timer = 0;

      const finish = (error) => {
        if (settled) {
          return;
        }
        settled = true;
        window.clearInterval(timer);
        if (error) {
          reject(error);
          return;
        }
        resolve(window.WPP);
      };

      const check = () => {
        const wpp = window.WPP;
        if (!wpp) {
          return;
        }
        if (wpp.isReady || typeof wpp.loader?.onReady !== "function") {
          finish();
          return;
        }
        if (!readyHookRegistered) {
          readyHookRegistered = true;
          wpp.loader.onReady(() => finish());
        }
      };

      timer = window.setInterval(() => {
        attempts += 1;
        check();
        if (attempts >= 1200) {
          finish(new Error("WPP did not become ready within 60 seconds"));
        }
      }, 50);
      check();
    });

  waitForWPP()
    .then(() => {
      wppReady = true;
      installUnreadMonitor(window.WPP);
      for (let index = 0; index < pluginFactories.length; index += 1) {
        const plugin = pluginFactories[index];
        runFactory(plugin.id, plugin.factory);
      }
      log("runtime ready");
    })
    .catch((error) => log("runtime did not become ready", error));
})();
