package desktop

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"unsafe"

	"betterwhatsapp/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	remoteOrigin       = "https://web.whatsapp.com"
	whatsappWindowName = "whatsapp"
)

type Controller struct {
	app      *application.App
	injector func(model.Settings) (string, error)

	closeAllowed atomic.Bool
	closing      atomic.Bool
	reloadQueued atomic.Bool
	mu           sync.Mutex
	window       *application.WebviewWindow
	surfaces     map[string]*application.WebviewWindow
}

func NewController(app *application.App, injector func(model.Settings) (string, error)) *Controller {
	return &Controller{
		app:      app,
		injector: injector,
		surfaces: make(map[string]*application.WebviewWindow),
	}
}

// CreateWhatsAppWindow creates the single application window. The local
// BetterWhatsApp toolbar and control surface are injected into this same
// remote WebView, so there is no second shell window or cross-process embed.
func (c *Controller) CreateWhatsAppWindow(settings model.Settings) error {
	if c == nil || c.app == nil || c.injector == nil {
		return errors.New("BetterWhatsApp window controller is not initialized")
	}
	script, err := c.injector(settings)
	if err != nil {
		return fmt.Errorf("build WhatsApp injector: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.window != nil {
		return nil
	}

	window := c.app.Window.NewWithOptions(remoteWindowOptions(settings, script))
	if window == nil {
		return errors.New("create WhatsApp window: Wails returned a nil window")
	}
	c.window = window
	c.installCloseToTrayHook(window)
	return nil
}

// CreateShellWindow is retained as a compatibility name for callers from the
// early scaffold. There is intentionally no separate shell window anymore.
func (c *Controller) CreateShellWindow(settings model.Settings) error {
	return c.CreateWhatsAppWindow(settings)
}

func (c *Controller) OpenWhatsApp() error {
	c.mu.Lock()
	hwnd := c.windowHandleLocked()
	c.mu.Unlock()
	if hwnd == 0 {
		return errors.New("BetterWhatsApp native window is not ready")
	}
	showNativeWindow(hwnd)
	return nil
}

func (c *Controller) HideWhatsApp() error {
	c.mu.Lock()
	hwnd := c.windowHandleLocked()
	c.mu.Unlock()
	if hwnd == 0 {
		return errors.New("BetterWhatsApp native window is not ready")
	}
	hideNativeWindow(hwnd)
	return nil
}

func (c *Controller) ToggleWhatsApp() error {
	c.mu.Lock()
	hwnd := c.windowHandleLocked()
	c.mu.Unlock()
	if hwnd == 0 {
		return errors.New("BetterWhatsApp native window is not ready")
	}
	if nativeWindowVisible(hwnd) {
		hideNativeWindow(hwnd)
	} else {
		showNativeWindow(hwnd)
	}
	return nil
}

func (c *Controller) ToggleMaximise() error {
	c.mu.Lock()
	hwnd := c.windowHandleLocked()
	c.mu.Unlock()
	return toggleNativeMaximise(hwnd)
}

// HandleWindowCommand is used by the injected toolbar so window operations go
// through the native handle and never block the remote WebView message path.
func (c *Controller) HandleWindowCommand(command string) error {
	switch command {
	case "hide", "close":
		return c.HideWhatsApp()
	case "maximise":
		return c.ToggleMaximise()
	default:
		return fmt.Errorf("unknown BetterWhatsApp window command %q", command)
	}
}

// ReloadWhatsApp never blocks the caller on WebView2. Wails' Reload method is
// synchronous internally, so it must stay outside the service/message thread.
func (c *Controller) ReloadWhatsApp() error {
	if c == nil || c.closing.Load() {
		return errors.New("BetterWhatsApp controller is shutting down")
	}
	if !c.reloadQueued.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	window := c.window
	c.mu.Unlock()
	if window == nil {
		c.reloadQueued.Store(false)
		return errors.New("BetterWhatsApp window is not initialized")
	}
	go func() {
		defer c.reloadQueued.Store(false)
		window.Reload()
	}()
	return nil
}

func (c *Controller) Window() application.Window {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.window
}

func (c *Controller) NativeHandle() uintptr {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.windowHandleLocked()
}

// WhatsAppNeedsNotification reads only native window state and is safe to use
// from the remote page message callback.
func (c *Controller) WhatsAppNeedsNotification() bool {
	c.mu.Lock()
	hwnd := c.windowHandleLocked()
	c.mu.Unlock()
	return hwnd == 0 || !nativeWindowVisible(hwnd) || nativeWindowMinimised(hwnd)
}

func (c *Controller) Quit() {
	c.closeAllowed.Store(true)
	c.Close()
	if c.app != nil {
		c.app.Quit()
	}
}

func (c *Controller) Close() {
	if c == nil || !c.closing.CompareAndSwap(false, true) {
		return
	}
	c.closeAllowed.Store(true)
}

func (c *Controller) OpenSurface(surface string) error {
	name, title, ok := surfaceDetails(surface)
	if !ok {
		return fmt.Errorf("unknown BetterWhatsApp surface %q", surface)
	}

	c.mu.Lock()
	if window := c.surfaces[name]; window != nil {
		c.mu.Unlock()
		showWindow(window)
		return nil
	}

	window := c.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                       name,
		Title:                      title,
		Width:                      1180,
		Height:                     800,
		MinWidth:                   920,
		MinHeight:                  620,
		Frameless:                  true,
		URL:                        "/" + surface + ".html",
		Hidden:                     false,
		DefaultContextMenuDisabled: false,
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:                   false,
			DisableFramelessWindowDecorations: true,
		},
	})
	if window == nil {
		c.mu.Unlock()
		return fmt.Errorf("create %s window: Wails returned a nil window", surface)
	}

	c.surfaces[name] = window
	window.RegisterHook(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		c.mu.Lock()
		if c.surfaces[name] == window {
			delete(c.surfaces, name)
		}
		c.mu.Unlock()
	})
	c.mu.Unlock()
	return nil
}

func surfaceDetails(surface string) (string, string, bool) {
	switch surface {
	case "themes":
		return "betterwhatsapp-themes", "BetterWhatsApp — Themes", true
	case "plugins":
		return "betterwhatsapp-plugins", "BetterWhatsApp — Plugins", true
	default:
		return "", "", false
	}
}

func showWindow(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	window.Show()
	window.Show()
	focusNativeWindow(nativeWindowHandle(window))
}

func (c *Controller) windowHandleLocked() uintptr {
	if c.window == nil {
		return 0
	}
	return nativeWindowHandle(c.window)
}

func (c *Controller) installCloseToTrayHook(window *application.WebviewWindow) {
	closeToTray := func(event *application.WindowEvent) {
		if c.closeAllowed.Load() {
			return
		}
		event.Cancel()
		hideNativeWindow(nativeWindowHandle(window))
	}
	window.RegisterHook(events.Windows.WindowClosing, closeToTray)
	window.RegisterHook(events.Common.WindowClosing, closeToTray)
}

func (c *Controller) closeForReplacementLocked(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	c.closeAllowed.Store(true)
	window.Close()
	c.closeAllowed.Store(false)
}

func remoteWindowOptions(settings model.Settings, script string) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:                       whatsappWindowName,
		Title:                      "BetterWhatsApp",
		Width:                      1440,
		Height:                     920,
		MinWidth:                   960,
		MinHeight:                  640,
		Frameless:                  true,
		Hidden:                     false,
		URL:                        settings.WhatsAppURL,
		HTML:                       redirectHTML(settings.WhatsAppURL),
		JS:                         script,
		AllowSimpleEventEmit:       false,
		EnableFileDrop:             false,
		DefaultContextMenuDisabled: false,
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:                   false,
			DisableFramelessWindowDecorations: true,
		},
		Permissions: map[application.PermissionType]application.Permission{
			application.PermissionMicrophone:    application.PermissionDefault,
			application.PermissionCamera:        application.PermissionDefault,
			application.PermissionNotifications: application.PermissionDefault,
			application.PermissionClipboardRead: application.PermissionAllow,
		},
	}
}

func redirectHTML(remoteURL string) string {
	return "<!doctype html><html><head><meta charset=\"utf-8\"></head><body><script>location.replace(" +
		strconv.Quote(remoteURL) +
		");</script></body></html>"
}

func nativeWindowHandle(window application.Window) uintptr {
	if window == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(window.NativeWindow()))
}

func RemoteOrigin() string {
	return remoteOrigin
}

func logDesktop(format string, args ...any) {
	fmt.Printf("[BetterWhatsApp] "+format+"\n", args...)
}
