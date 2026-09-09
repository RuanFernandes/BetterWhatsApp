package main

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"

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

//go:embed build/windows/icon.ico
var windowsTrayIcon []byte

//go:embed build/windows/icon-unread.ico
var windowsTrayUnreadIcon []byte

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
	showExistingWhatsApp := func() {
		if !whatsappReady.Load() {
			secondInstancePending.Store(true)
			return
		}
		if windows == nil {
			log.Printf("[BetterWhatsApp] second instance requested focus before controller initialization")
			return
		}
		if err := windows.OpenWhatsApp(); err != nil {
			log.Printf("[BetterWhatsApp] focus existing instance: %v", err)
		}
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

	if launch, ok, err := parseProfileLaunchArguments(os.Args[1:]); err != nil {
		log.Fatal(err)
	} else if ok {
		if err := runProfileProcess(
			launch,
			store.Snapshot(),
			runtimeDirectory,
			builder,
		); err != nil {
			log.Fatal(err)
		}
		return
	}

	shellDataPath := filepath.Join(runtimeDirectory, "shell")
	if err := os.MkdirAll(shellDataPath, 0o700); err != nil {
		log.Fatal(fmt.Errorf("prepare BetterWhatsApp shell profile: %w", err))
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
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(frontendAssets),
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			WebviewUserDataPath:           shellDataPath,
			WndProcInterceptor: func(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
				if windows == nil {
					return 0, false
				}
				interceptor := windows.WndProcInterceptor()
				if interceptor == nil {
					return 0, false
				}
				return interceptor(hwnd, msg, wParam, lParam)
			},
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		RawMessageHandler: func(window application.Window, message string, originInfo *application.OriginInfo) {
			handleRawMessage(service, window, message, originInfo, func() error {
				if windows == nil {
					return errors.New("WhatsApp window controller is not initialized")
				}
				return windows.CreateWhatsAppWindow(store.Snapshot())
			}, func(surface string) error {
				if windows == nil {
					return errors.New("WhatsApp window controller is not initialized")
				}
				return windows.OpenSurface(surface)
			}, func(count int) {
				if notificationCenter != nil {
					notificationCenter.SetUnreadCount(count)
				}
			}, func(notificationWindow application.Window) {
				if notificationCenter != nil {
					notificationCenter.HandleNewMessage(notificationWindow)
				}
			})
		},
		ErrorHandler: func(err error) {
			log.Printf("[BetterWhatsApp] %v", err)
		},
	})

	windows = desktop.NewController(wailsApp, builder.Build)
	windows.SetRuntimeDirectory(runtimeDirectory)
	service = appservice.NewWithReloadAndOperations(
		store,
		pluginManager,
		themeManager,
		appVersion,
		func() error {
			if windows == nil {
				return errors.New("WhatsApp window controller is not initialized")
			}
			return windows.RestartProfiles(store.Snapshot())
		},
		windows,
		windows.OpenSurface,
		updater.New(filepath.Join(runtimeDirectory, "updates")),
	)
	wailsApp.RegisterService(application.NewService(service))

	if err := windows.CreateShellWindow(store.Snapshot()); err != nil {
		log.Fatal(err)
	}
	whatsappReady.Store(true)
	if secondInstancePending.Swap(false) {
		showExistingWhatsApp()
	}

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
	notificationCenter = desktop.NewNotificationCenter(trayIcon(false), trayIcon(true))
	notificationCenter.AttachTray(tray, unreadMenuItem)
	windows.SetProfileRemovedHandler(notificationCenter.ClearProfileUnreadCount)
	windows.SetProfileEventHandler(func(event desktop.ProfileEvent) {
		switch event.Type {
		case "unread":
			notificationCenter.SetProfileUnreadCount(event.ProfileID, event.Count)
		case "new-message":
			notificationCenter.HandleNewMessage(windows.Window())
		}
	})
	tray.OnClick(func() {
		if err := windows.ToggleWhatsApp(); err != nil {
			log.Printf("[BetterWhatsApp] tray toggle: %v", err)
		}
	})
	if err := windows.StartProfiles(store.Snapshot()); err != nil {
		log.Printf("[BetterWhatsApp] profile startup: %v", err)
	}

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}

type profileLaunchArguments struct {
	ProfileID    string
	ParentHandle uintptr
	IPCToken     string
}

func parseProfileLaunchArguments(args []string) (profileLaunchArguments, bool, error) {
	if len(args) == 0 {
		return profileLaunchArguments{}, false, nil
	}
	if len(args) != 6 || args[0] != "--profile-window" || args[2] != "--parent-hwnd" || args[4] != "--ipc-token" {
		return profileLaunchArguments{}, false, fmt.Errorf("invalid profile process arguments")
	}
	if args[1] == "" || args[5] == "" {
		return profileLaunchArguments{}, false, errors.New("profile process identity is incomplete")
	}
	parentHandle, err := strconv.ParseUint(args[3], 10, 64)
	if err != nil || parentHandle == 0 {
		return profileLaunchArguments{}, false, errors.New("profile process parent handle is invalid")
	}
	if len(args[5]) != 48 {
		return profileLaunchArguments{}, false, errors.New("profile process IPC token is invalid")
	}
	if _, err := hex.DecodeString(args[5]); err != nil {
		return profileLaunchArguments{}, false, errors.New("profile process IPC token is invalid")
	}
	return profileLaunchArguments{
		ProfileID:    args[1],
		ParentHandle: uintptr(parentHandle),
		IPCToken:     args[5],
	}, true, nil
}

func runProfileProcess(
	launch profileLaunchArguments,
	settings model.Settings,
	runtimeDirectory string,
	builder *injector.Builder,
) error {
	profile, ok := settings.Profiles[launch.ProfileID]
	if !ok {
		return fmt.Errorf("profile %q is not configured", launch.ProfileID)
	}
	userDataPath, err := profileWebviewDataPath(runtimeDirectory, profile)
	if err != nil {
		return err
	}
	if userDataPath != "" {
		if err := os.MkdirAll(userDataPath, 0o700); err != nil {
			return fmt.Errorf("prepare profile %q WebView data: %w", profile.ID, err)
		}
	}

	var profileController *desktop.Controller
	sendEvent := func(event desktop.ProfileEvent) {
		event.ProfileID = launch.ProfileID
		event.Token = launch.IPCToken
		senderHandle := uintptr(0)
		if profileController != nil {
			senderHandle = profileController.NativeHandle()
		}
		if err := desktop.SendProfileEvent(launch.ParentHandle, senderHandle, event); err != nil {
			log.Printf("[BetterWhatsApp] profile %q IPC event failed: %v", launch.ProfileID, err)
		}
	}
	profileApp := application.New(application.Options{
		Name:        "BetterWhatsApp",
		Description: "BetterWhatsApp isolated WhatsApp profile",
		Icon:        appIcon,
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(frontendAssets),
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
			WebviewUserDataPath:           userDataPath,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		RawMessageHandler: func(window application.Window, message string, originInfo *application.OriginInfo) {
			handleRawMessage(
				nil,
				window,
				message,
				originInfo,
				nil,
				nil,
				func(count int) {
					sendEvent(desktop.ProfileEvent{Type: "unread", Count: count})
				},
				func(application.Window) {
					sendEvent(desktop.ProfileEvent{Type: "new-message"})
				},
			)
		},
		ErrorHandler: func(err error) {
			log.Printf("[BetterWhatsApp] profile %q: %v", launch.ProfileID, err)
		},
	})
	profileController = desktop.NewController(profileApp, func(current model.Settings) (string, error) {
		return builder.BuildForProfile(current, launch.ProfileID)
	})
	if err := profileController.CreateProfileWindow(settings); err != nil {
		return err
	}
	if err := profileApp.Run(); err != nil {
		return fmt.Errorf("run profile %q: %w", launch.ProfileID, err)
	}
	return nil
}

func profileWebviewDataPath(runtimeDirectory string, profile model.Profile) (string, error) {
	if profile.LegacyWebviewData {
		return "", nil
	}
	base, err := filepath.Abs(filepath.Join(runtimeDirectory, "profiles"))
	if err != nil {
		return "", fmt.Errorf("resolve profile data directory: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(base, profile.ID))
	if err != nil {
		return "", fmt.Errorf("resolve profile %q data directory: %w", profile.ID, err)
	}
	relative, err := filepath.Rel(base, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("profile %q data path escapes the profile directory", profile.ID)
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
		AppVersion:      appVersion,
		Injector:        settings.Injector,
		Plugins:         pluginsList,
		Themes:          themesList,
		ActiveThemeID:   settings.ActiveThemeID,
		WhatsAppURL:     settings.WhatsAppURL,
		RemoteOrigin:    desktop.RemoteOrigin(),
		Profiles:        config.ProfileInfos(settings),
		ActiveProfileID: settings.ActiveProfileID,
	}, nil
}

func handleRawMessage(
	service *appservice.Service,
	window application.Window,
	message string,
	originInfo *application.OriginInfo,
	reload func() error,
	openSurface func(string) error,
	setUnreadCount func(int),
	handleNewMessage func(application.Window),
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
			handleNewMessage(window)
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
			handleNewMessage(window)
		}
		return
	}

	switch message {
	case "betterwhatsapp:window:hide":
		window.Hide()
		return
	case "betterwhatsapp:window:maximise":
		window.ToggleMaximise()
		return
	case "betterwhatsapp:window:close":
		window.Close()
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
	if runtime.GOOS == "windows" {
		if hasUnread && len(windowsTrayUnreadIcon) > 0 {
			return windowsTrayUnreadIcon
		}
		if len(windowsTrayIcon) > 0 {
			return windowsTrayIcon
		}
	}
	if hasUnread && len(unreadTrayIcon) > 0 {
		return unreadTrayIcon
	}
	return appIcon
}
