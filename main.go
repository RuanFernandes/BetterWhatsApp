package main

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"betterwhatsapp/internal/appservice"
	"betterwhatsapp/internal/config"
	"betterwhatsapp/internal/desktop"
	"betterwhatsapp/internal/extensions"
	"betterwhatsapp/internal/injector"
	"betterwhatsapp/internal/model"
	"betterwhatsapp/internal/plugins"
	"betterwhatsapp/internal/themes"
	"betterwhatsapp/internal/updater"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

var appVersion = "0.1.0-dev"

const (
	maxRawMessageSize  = themes.MaxSourceSize + 64*1024
	themeCommandSource = "betterwhatsapp-theme-editor"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

//go:embed assets/injector/bootstrap.js assets/injector/wa-js.js assets/injector/wa-js.LICENSE.txt assets/plugins
var extensionAssets embed.FS

//go:embed build/appicon.png
var appIcon []byte

//go:embed frontend/src/assets/betterwhatsapp-logo.png
var betterWhatsAppLogo []byte

//go:embed assets/branding/betterwhatsapp-logo-unread.png
var unreadTrayIcon []byte

func main() {
	userConfigDirectory, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(fmt.Errorf("resolve user config directory: %w", err))
	}
	runtimeDirectory := filepath.Join(userConfigDirectory, "BetterWhatsApp")

	store, err := config.NewStoreAt(filepath.Join(runtimeDirectory, "settings.json"))
	if err != nil {
		log.Fatal(err)
	}

	pluginFS, err := fs.Sub(extensionAssets, "assets/plugins")
	if err != nil {
		log.Fatal(fmt.Errorf("open bundled plugins: %w", err))
	}
	pluginCatalog := extensions.NewCatalog(pluginFS, filepath.Join(runtimeDirectory, "plugins"), ".js")
	themeCatalog := extensions.NewCatalog(nil, filepath.Join(runtimeDirectory, "themes"), ".css")
	pluginManager := plugins.NewManager(pluginCatalog)
	themeManager := themes.NewManager(themeCatalog)
	if _, err := store.Update(func(settings *model.Settings) error {
		if err := pluginManager.EnsureDefaults(settings); err != nil {
			return err
		}
		return themeManager.EnsureDefaults(settings)
	}); err != nil {
		log.Fatal(fmt.Errorf("initialize extension defaults: %w", err))
	}

	var windows *desktop.Controller
	var service *appservice.Service
	var whatsappReady atomic.Bool
	var secondInstancePending atomic.Bool
	var shellOpenPending atomic.Bool
	openShellWhenReady := func() {
		if !shellOpenPending.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer shellOpenPending.Store(false)
			deadline := time.Now().Add(15 * time.Second)
			for time.Now().Before(deadline) {
				if windows != nil {
					hwnd := windows.NativeHandle()
					if hwnd == 0 {
						time.Sleep(50 * time.Millisecond)
						continue
					}
					if err := windows.OpenWhatsApp(); err != nil {
						log.Printf("[BetterWhatsApp] show existing shell: %v", err)
					}
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}()
	}
	showExistingWhatsApp := func() {
		if !whatsappReady.Load() {
			secondInstancePending.Store(true)
			return
		}
		if windows == nil {
			log.Printf("[BetterWhatsApp] second instance requested focus before controller initialization")
			return
		}
		openShellWhenReady()
	}
	bootstrap, err := fs.ReadFile(extensionAssets, "assets/injector/bootstrap.js")
	if err != nil {
		log.Fatal(fmt.Errorf("read injector bootstrap: %w", err))
	}
	wppSource, err := fs.ReadFile(extensionAssets, "assets/injector/wa-js.js")
	if err != nil {
		log.Fatal(fmt.Errorf("read WA-JS bundle: %w", err))
	}
	controlScript, controlStyle, err := readControlAssets(frontendAssets)
	if err != nil {
		log.Fatal(err)
	}

	builder := injector.NewBuilder(
		string(bootstrap),
		string(wppSource),
		pluginManager.Enabled,
		themeManager.Enabled,
		injector.ControlAssets{
			Script:      string(controlScript),
			Style:       string(controlStyle),
			LogoDataURI: "data:image/png;base64," + base64.StdEncoding.EncodeToString(betterWhatsAppLogo),
		},
		func(settings model.Settings) (model.AppState, error) {
			return buildControlState(settings, pluginManager, themeManager)
		},
	)

	settings := store.Snapshot()
	webviewDataPath, err := sessionWebviewDataPath(runtimeDirectory, settings)
	if err != nil {
		log.Fatal(err)
	}
	if webviewDataPath != "" {
		if err := os.MkdirAll(webviewDataPath, 0o700); err != nil {
			log.Fatal(fmt.Errorf("prepare BetterWhatsApp WebView data: %w", err))
		}
	}

	var notificationCenter *desktop.NotificationCenter
	wailsApp := application.New(application.Options{
		Name:        "BetterWhatsApp",
		Description: "A local shell for WhatsApp Web customization",
		Icon:        appIcon,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.betterwhatsapp.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				showExistingWhatsApp()
			},
		},
		OnShutdown: func() {
			if windows != nil {
				windows.Close()
			}
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(frontendAssets),
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			WebviewUserDataPath:           webviewDataPath,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		RawMessageHandler: func(window application.Window, message string, originInfo *application.OriginInfo) {
			handleRawMessage(service, window, message, originInfo, func() error {
				if windows == nil {
					return errors.New("WhatsApp window controller is not initialized")
				}
				return windows.ReloadWhatsApp()
			}, func(surface string) error {
				if windows == nil {
					return errors.New("WhatsApp window controller is not initialized")
				}
				return windows.OpenSurface(surface)
			}, func(command string) error {
				if windows == nil {
					return errors.New("WhatsApp window controller is not initialized")
				}
				return windows.HandleWindowCommand(command)
			}, func(count int) {
				if notificationCenter != nil {
					notificationCenter.SetUnreadCount(count)
				}
			}, func() {
				if notificationCenter != nil {
					hidden := windows == nil || windows.WhatsAppNeedsNotification()
					notificationCenter.HandleNewMessage(hidden)
				}
			})
		},
		ErrorHandler: func(err error) {
			log.Printf("[BetterWhatsApp] %v", err)
		},
	})

	windows = desktop.NewController(wailsApp, builder.Build)
	service = appservice.NewWithReloadAndOperations(
		store,
		pluginManager,
		themeManager,
		appVersion,
		func() error {
			if windows == nil {
				return errors.New("WhatsApp window controller is not initialized")
			}
			return windows.ReloadWhatsApp()
		},
		windows,
		windows.OpenSurface,
		updater.New(filepath.Join(runtimeDirectory, "updates")),
	)
	wailsApp.RegisterService(application.NewService(service))

	if err := windows.CreateWhatsAppWindow(settings); err != nil {
		log.Fatal(err)
	}
	whatsappReady.Store(true)
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		// Wails creates pending windows asynchronously. Showing the native
		// container here is safe because OpenWhatsApp uses only Win32 calls;
		// it does not enter the WebView2 dispatcher while startup is settling.
		openShellWhenReady()
		if secondInstancePending.Swap(false) {
			openShellWhenReady()
		}
	})

	tray := wailsApp.SystemTray.New()

	trayMenu := wailsApp.NewMenu()
	unreadMenuItem := trayMenu.Add("Nenhuma mensagem não lida")
	unreadMenuItem.SetTooltip("Estado das mensagens recebidas")
	trayMenu.AddSeparator()
	trayMenu.Add("Mostrar / ocultar BetterWhatsApp").OnClick(func(_ *application.Context) {
		if err := windows.ToggleWhatsApp(); err != nil {
			log.Printf("[BetterWhatsApp] tray toggle: %v", err)
		}
	})
	trayMenu.AddSeparator()
	trayMenu.Add("Fechar BetterWhatsApp").OnClick(func(_ *application.Context) {
		windows.Quit()
	})
	tray.SetMenu(trayMenu)
	// Set the normal icon before Run so the deferred Wails tray has a valid
	// image on its first native initialization. Show it again after the
	// platform loop starts in case Windows restored it as hidden.
	tray.SetIcon(trayIcon(false))
	tray.SetTooltip("BetterWhatsApp — Nenhuma mensagem não lida")
	notificationCenter = desktop.NewNotificationCenter(trayIcon(false), trayIcon(true))
	notificationCenter.AttachTray(tray, unreadMenuItem)
	tray.OnClick(func() {
		if err := windows.ToggleWhatsApp(); err != nil {
			log.Printf("[BetterWhatsApp] tray toggle: %v", err)
		}
	})
	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		tray.Show()
		notificationCenter.Start()
	})

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}

func sessionWebviewDataPath(runtimeDirectory string, settings model.Settings) (string, error) {
	profile, ok := settings.Profiles[settings.ActiveProfileID]
	if !ok {
		for _, id := range settings.ProfileOrder {
			if candidate, exists := settings.Profiles[id]; exists {
				profile = candidate
				ok = true
				break
			}
		}
	}
	if !ok || profile.LegacyWebviewData {
		// Empty means Wails' historical default %APPDATA%\\betterwhatsapp.exe,
		// which is where the original single-session build stored the login.
		return "", nil
	}
	base, err := filepath.Abs(filepath.Join(runtimeDirectory, "profiles"))
	if err != nil {
		return "", fmt.Errorf("resolve WebView data directory: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(base, profile.ID))
	if err != nil {
		return "", fmt.Errorf("resolve session data directory: %w", err)
	}
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("session data path escapes the BetterWhatsApp directory")
	}
	return target, nil
}

func readControlAssets(assets fs.FS) ([]byte, []byte, error) {
	style, err := fs.ReadFile(assets, "frontend/dist/style.css")
	if err != nil {
		return nil, nil, fmt.Errorf("read control surface stylesheet: %w", err)
	}
	styles := append([]byte(nil), style...)

	controlEntries, controlErr := fs.ReadDir(assets, "frontend/dist/control")
	if controlErr == nil {
		var script []byte
		for _, entry := range controlEntries {
			if entry.IsDir() {
				continue
			}
			assetPath := "frontend/dist/control/" + entry.Name()
			switch filepath.Ext(entry.Name()) {
			case ".css":
				css, err := fs.ReadFile(assets, assetPath)
				if err != nil {
					return nil, nil, fmt.Errorf("read control surface stylesheet: %w", err)
				}
				styles = append(styles, '\n')
				styles = append(styles, css...)
			case ".js":
				if script != nil {
					continue
				}
				script, err = fs.ReadFile(assets, assetPath)
				if err != nil {
					return nil, nil, fmt.Errorf("read control surface script: %w", err)
				}
			}
		}
		if script != nil {
			return script, styles, nil
		}
	}

	entries, err := fs.ReadDir(assets, "frontend/dist/assets")
	if err != nil {
		return nil, nil, fmt.Errorf("list control surface assets: %w", err)
	}

	var script []byte
	scriptFound := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		assetPath := "frontend/dist/assets/" + entry.Name()
		switch {
		case filepath.Ext(entry.Name()) == ".css":
			css, err := fs.ReadFile(assets, assetPath)
			if err != nil {
				return nil, nil, fmt.Errorf("read control surface stylesheet asset: %w", err)
			}
			styles = append(styles, '\n')
			styles = append(styles, css...)
		case !scriptFound &&
			(strings.HasPrefix(entry.Name(), "index-") || strings.HasPrefix(entry.Name(), "main-")) &&
			filepath.Ext(entry.Name()) == ".js":
			script, err = fs.ReadFile(assets, assetPath)
			if err != nil {
				return nil, nil, fmt.Errorf("read control surface script: %w", err)
			}
			scriptFound = true
		}
	}

	if !scriptFound {
		return nil, nil, errors.New("control surface script was not found in frontend/dist/assets")
	}
	return script, styles, nil
}

func buildControlState(
	settings model.Settings,
	pluginManager *plugins.Manager,
	themeManager *themes.Manager,
) (model.AppState, error) {
	pluginsList, err := pluginManager.List(settings)
	if err != nil {
		return model.AppState{}, fmt.Errorf("list plugins for control surface: %w", err)
	}
	themesList, err := themeManager.List(settings)
	if err != nil {
		return model.AppState{}, fmt.Errorf("list themes for control surface: %w", err)
	}
	return model.AppState{
		AppVersion:    appVersion,
		Injector:      settings.Injector,
		Plugins:       pluginsList,
		Themes:        themesList,
		ActiveThemeID: settings.ActiveThemeID,
		WhatsAppURL:   settings.WhatsAppURL,
		RemoteOrigin:  desktop.RemoteOrigin(),
	}, nil
}

func handleRawMessage(
	service *appservice.Service,
	window application.Window,
	message string,
	originInfo *application.OriginInfo,
	reload func() error,
	openSurface func(string) error,
	windowCommand func(string) error,
	setUnreadCount func(int),
	handleNewMessage func(),
) {
	if window == nil ||
		window.Name() != "whatsapp" ||
		originInfo == nil ||
		!isTrustedRemoteOrigin(originInfo.Origin) ||
		!isTrustedRemoteOrigin(originInfo.TopOrigin) ||
		len(message) > maxRawMessageSize {
		return
	}

	if service == nil {
		if count, ok := decodeUnreadCountMessage(message); ok && setUnreadCount != nil {
			setUnreadCount(count)
		}
		if message == "betterwhatsapp:notifications:new-message" && handleNewMessage != nil {
			handleNewMessage()
		}
		return
	}

	if command, ok := decodeThemeCommand(message); ok {
		handleThemeCommand(service, window, command)
		return
	}

	if count, ok := decodeUnreadCountMessage(message); ok {
		if setUnreadCount != nil {
			setUnreadCount(count)
		}
		return
	}
	if message == "betterwhatsapp:notifications:new-message" {
		if handleNewMessage != nil {
			handleNewMessage()
		}
		return
	}

	switch message {
	case "betterwhatsapp:window:hide":
		if windowCommand != nil {
			if err := windowCommand("hide"); err != nil {
				log.Printf("[BetterWhatsApp] hide window failed: %v", err)
			}
		} else {
			window.Hide()
		}
		return
	case "betterwhatsapp:window:maximise":
		if windowCommand != nil {
			if err := windowCommand("maximise"); err != nil {
				log.Printf("[BetterWhatsApp] maximise window failed: %v", err)
			}
		} else {
			window.ToggleMaximise()
		}
		return
	case "betterwhatsapp:window:close":
		if windowCommand != nil {
			if err := windowCommand("close"); err != nil {
				log.Printf("[BetterWhatsApp] close window failed: %v", err)
			}
		} else {
			window.Hide()
		}
		return
	case "betterwhatsapp:window:reload":
		if reload == nil {
			return
		}
		go func() {
			if err := reload(); err != nil {
				log.Printf("[BetterWhatsApp] window reload failed: %v", err)
			}
		}()
		return
	case "betterwhatsapp:surface:plugins":
		if openSurface != nil {
			if err := openSurface("plugins"); err != nil {
				log.Printf("[BetterWhatsApp] open plugins surface failed: %v", err)
			}
		}
		return
	case "betterwhatsapp:surface:themes":
		if openSurface != nil {
			if err := openSurface("themes"); err != nil {
				log.Printf("[BetterWhatsApp] open themes surface failed: %v", err)
			}
		}
		return
	}

	parts := strings.Split(message, ":")
	if len(parts) < 3 || parts[0] != "betterwhatsapp" || parts[1] != "settings" {
		return
	}

	ctx := context.WithValue(context.Background(), application.WindowKey, window)
	var err error

	switch parts[2] {
	case "injector":
		if len(parts) != 4 {
			return
		}
		enabled, ok := parseCommandBool(parts[3])
		if !ok {
			return
		}
		err = service.SetInjectorEnabled(ctx, enabled)
	case "plugin", "theme":
		if len(parts) != 5 {
			return
		}
		enabled, ok := parseCommandBool(parts[4])
		if !ok {
			return
		}
		if parts[2] == "plugin" {
			err = service.SetPluginEnabled(ctx, parts[3], enabled)
		} else {
			err = service.SetThemeEnabled(ctx, parts[3], enabled)
		}
	default:
		return
	}

	if err != nil {
		log.Printf("[BetterWhatsApp] raw command %q failed: %v", message, err)
	}
}

type themeCommand struct {
	Source    string `json:"source"`
	RequestID string `json:"requestId"`
	Action    string `json:"action"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	CSS       string `json:"css,omitempty"`
}

type themeCommandResult struct {
	Source    string `json:"source"`
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	Action    string `json:"action"`
	OK        bool   `json:"ok"`
	ThemeID   string `json:"themeId,omitempty"`
	CSS       string `json:"css,omitempty"`
	Error     string `json:"error,omitempty"`
}

func decodeThemeCommand(message string) (themeCommand, bool) {
	if len(message) == 0 || message[0] != '{' {
		return themeCommand{}, false
	}
	var command themeCommand
	if err := json.Unmarshal([]byte(message), &command); err != nil {
		return themeCommand{}, false
	}
	if command.Source != themeCommandSource ||
		!validRequestID(command.RequestID) {
		return themeCommand{}, false
	}
	switch command.Action {
	case "create":
		return command, command.Name != ""
	case "read", "delete":
		return command, command.ID != ""
	case "update":
		return command, command.ID != "" && command.Name != ""
	default:
		return themeCommand{}, false
	}
}

func handleThemeCommand(service *appservice.Service, window application.Window, command themeCommand) {
	ctx := context.WithValue(context.Background(), application.WindowKey, window)
	result := themeCommandResult{
		Source:    "betterwhatsapp-host",
		Type:      "theme-result",
		RequestID: command.RequestID,
		Action:    command.Action,
	}

	var err error
	switch command.Action {
	case "create":
		result.ThemeID, err = service.CreateTheme(ctx, command.Name, command.CSS)
	case "read":
		result.CSS, err = service.ReadTheme(ctx, command.ID)
	case "update":
		err = service.UpdateTheme(ctx, command.ID, command.Name, command.CSS)
	case "delete":
		err = service.DeleteTheme(ctx, command.ID)
	default:
		err = errors.New("unsupported theme action")
	}
	if err != nil {
		result.Error = err.Error()
	} else {
		result.OK = true
	}
	emitThemeCommandResult(window, result)
}

func emitThemeCommandResult(window application.Window, result themeCommandResult) {
	data, err := json.Marshal(result)
	if err != nil {
		log.Printf("[BetterWhatsApp] encode theme command result: %v", err)
		return
	}
	window.ExecJS("window.postMessage(" + string(data) + ", location.origin);")
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' ||
			char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' ||
			char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func isTrustedRemoteOrigin(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil ||
		parsed.Scheme != "https" ||
		!strings.EqualFold(parsed.Hostname(), "web.whatsapp.com") ||
		parsed.User != nil {
		return false
	}

	port := parsed.Port()
	return port == "" || port == "443"
}

func parseCommandBool(value string) (bool, bool) {
	switch value {
	case "0":
		return false, true
	case "1":
		return true, true
	default:
		return false, false
	}
}

func decodeUnreadCountMessage(message string) (int, bool) {
	const prefix = "betterwhatsapp:notifications:unread:"
	if !strings.HasPrefix(message, prefix) {
		return 0, false
	}

	rawCount := strings.TrimPrefix(message, prefix)
	if rawCount == "" || len(rawCount) > 6 {
		return 0, false
	}
	count, err := strconv.Atoi(rawCount)
	if err != nil || count < 0 || count > 999999 {
		return 0, false
	}
	return count, true
}

func trayIcon(hasUnread bool) []byte {
	if hasUnread && len(unreadTrayIcon) > 0 {
		return unreadTrayIcon
	}
	if len(betterWhatsAppLogo) > 0 {
		return betterWhatsAppLogo
	}
	return appIcon
}
