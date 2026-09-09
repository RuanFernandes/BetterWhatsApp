package desktop

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"betterwhatsapp/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	remoteOrigin       = "https://web.whatsapp.com"
	whatsappWindowName = "whatsapp"
	profileHostTop     = 92 // CSS px/DIP: 42px titlebar + 50px profile bar.
	profileBootstrapXY = -32000
)

type profileRuntime struct {
	profileID string
	token     string
	command   *exec.Cmd
	pid       uint32
	hwnd      uintptr
}

type profileStart struct {
	done chan struct{}
	err  error
}

type Controller struct {
	app      *application.App
	injector func(model.Settings) (string, error)

	closeAllowed atomic.Bool
	mu           sync.Mutex
	window       *application.WebviewWindow
	surfaces     map[string]*application.WebviewWindow
	shell        bool
	profileHost  uintptr

	runtimeDirectory      string
	profiles              map[string]*profileRuntime
	profileDefinitions    map[string]model.Profile
	starting              map[string]*profileStart
	profileHandler        func(ProfileEvent)
	profileRemovedHandler func(string)
}

func NewController(app *application.App, injector func(model.Settings) (string, error)) *Controller {
	return &Controller{
		app:                app,
		injector:           injector,
		surfaces:           make(map[string]*application.WebviewWindow),
		profiles:           make(map[string]*profileRuntime),
		profileDefinitions: make(map[string]model.Profile),
		starting:           make(map[string]*profileStart),
	}
}

// CreateShellWindow creates the one visible local Wails window. Remote
// WhatsApp Web instances are embedded into this window by the profile host.
func (c *Controller) CreateShellWindow(settings model.Settings) error {
	_ = settings
	c.mu.Lock()
	defer c.mu.Unlock()

	options := application.WebviewWindowOptions{
		Name:                       whatsappWindowName,
		Title:                      "BetterWhatsApp",
		Width:                      1440,
		Height:                     920,
		MinWidth:                   960,
		MinHeight:                  640,
		Frameless:                  true,
		Hidden:                     false,
		URL:                        "/tabs.html",
		DefaultContextMenuDisabled: false,
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:                   false,
			DisableFramelessWindowDecorations: true,
		},
	}
	if c.window != nil {
		wasVisible := c.window.IsVisible()
		c.destroyProfileHostLocked()
		c.closeForReplacementLocked(c.window)
		options.Hidden = !wasVisible
	}

	window := c.app.Window.NewWithOptions(options)
	if window == nil {
		return errors.New("create BetterWhatsApp shell: Wails returned a nil window")
	}
	c.window = window
	c.shell = true
	c.profileHost = 0
	c.installCloseToTrayHook(window)
	window.OnWindowEvent(events.Common.WindowDidResize, func(_ *application.WindowEvent) {
		c.resizeEmbeddedProfiles()
	})
	return nil
}

// CreateWhatsAppWindow remains available for a standalone remote window and
// is used by profile child processes.
func (c *Controller) CreateWhatsAppWindow(settings model.Settings) error {
	return c.createRemoteWindow(settings, false)
}

func (c *Controller) CreateProfileWindow(settings model.Settings) error {
	return c.createRemoteWindow(settings, true)
}

func (c *Controller) createRemoteWindow(settings model.Settings, hidden bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	script, err := c.injector(settings)
	if err != nil {
		return fmt.Errorf("build WhatsApp injector: %w", err)
	}

	options := c.remoteWindowOptions(settings, script, hidden)
	if c.window != nil {
		wasVisible := c.window.IsVisible()
		c.destroyProfileHostLocked()
		c.closeForReplacementLocked(c.window)
		c.window = nil
		if !hidden {
			options.Hidden = !wasVisible
		}
	}

	window := c.app.Window.NewWithOptions(options)
	if window == nil {
		return errors.New("create WhatsApp window: Wails returned a nil window")
	}
	c.window = window
	c.shell = false
	c.profileHost = 0
	if !hidden {
		c.installCloseToTrayHook(window)
	}
	return nil
}

func (c *Controller) OpenWhatsApp() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.window == nil {
		return errors.New("BetterWhatsApp window is not initialized")
	}
	c.window.Show()
	c.window.Focus()
	return nil
}

func (c *Controller) HideWhatsApp() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.window == nil {
		return errors.New("BetterWhatsApp window is not initialized")
	}
	c.window.Hide()
	return nil
}

func (c *Controller) ToggleWhatsApp() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.window == nil {
		return errors.New("BetterWhatsApp window is not initialized")
	}
	if c.window.IsVisible() {
		c.window.Hide()
		return nil
	}
	c.window.Show()
	c.window.Focus()
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

func (c *Controller) SetRuntimeDirectory(directory string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runtimeDirectory = directory
}

func (c *Controller) SetProfileEventHandler(handler func(ProfileEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profileHandler = handler
}

func (c *Controller) SetProfileRemovedHandler(handler func(string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profileRemovedHandler = handler
}

func (c *Controller) WndProcInterceptor() func(hwnd uintptr, msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	return platformWndProcInterceptor(func(sender uintptr, event ProfileEvent) bool {
		c.mu.Lock()
		runtime := c.profiles[event.ProfileID]
		handler := c.profileHandler
		valid := runtime != nil &&
			runtime.token == event.Token &&
			runtime.pid == profileWindowProcessID(sender)
		c.mu.Unlock()
		if !valid {
			return false
		}
		if handler != nil {
			handler(event)
		}
		return true
	})
}

func (c *Controller) StartProfiles(settings model.Settings) error {
	if !profileHostSupported() {
		return errors.New("isolated profile windows are currently supported on Windows only")
	}
	c.setProfileDefinitions(settings)
	go c.startProfilesWhenReady(settings)
	return nil
}

func (c *Controller) startProfilesWhenReady(settings model.Settings) {
	if _, err := c.waitForShellHandle(30 * time.Second); err != nil {
		logDesktop("profile startup skipped: %v", err)
		return
	}

	ordered := append([]string(nil), settings.ProfileOrder...)
	if settings.ActiveProfileID != "" {
		if profile, ok := settings.Profiles[settings.ActiveProfileID]; ok {
			if err := c.EnsureProfile(profile); err != nil {
				logDesktop("active profile %q failed to start: %v", profile.ID, err)
			} else if err := c.ActivateProfile(profile.ID); err != nil {
				logDesktop("active profile %q failed to activate: %v", profile.ID, err)
			}
		}
	}
	for _, id := range ordered {
		if id == settings.ActiveProfileID {
			continue
		}
		profile, ok := settings.Profiles[id]
		if !ok {
			continue
		}
		go func(profile model.Profile) {
			if err := c.EnsureProfile(profile); err != nil {
				logDesktop("profile %q failed to start: %v", profile.ID, err)
			}
		}(profile)
	}
}

func (c *Controller) EnsureProfile(profile model.Profile) error {
	if !profileHostSupported() {
		return errors.New("isolated profile windows are currently supported on Windows only")
	}
	if profile.ID == "" {
		return errors.New("profile id is required")
	}

	c.mu.Lock()
	c.profileDefinitions[profile.ID] = profile
	if runtime := c.profiles[profile.ID]; runtime != nil && runtime.command != nil && runtime.command.Process != nil {
		c.mu.Unlock()
		return nil
	}
	if pending := c.starting[profile.ID]; pending != nil {
		c.mu.Unlock()
		<-pending.done
		return pending.err
	}
	pending := &profileStart{done: make(chan struct{})}
	c.starting[profile.ID] = pending
	parentHandle := c.windowHandleLocked()
	runtimeDirectory := c.runtimeDirectory
	c.mu.Unlock()

	var runtime *profileRuntime
	var err error
	if parentHandle == 0 {
		err = errors.New("BetterWhatsApp shell native window is not ready")
	} else {
		runtime, err = c.launchProfile(profile, runtimeDirectory, parentHandle)
	}

	c.mu.Lock()
	delete(c.starting, profile.ID)
	pending.err = err
	if err == nil {
		c.profiles[profile.ID] = runtime
		c.resizeEmbeddedProfileLocked(runtime)
	}
	close(pending.done)
	c.mu.Unlock()
	return err
}

func (c *Controller) ActivateProfile(profileID string) error {
	if profileID == "" {
		return errors.New("profile id is required")
	}
	c.mu.Lock()
	profile, ok := c.profileDefinitions[profileID]
	c.mu.Unlock()
	if !ok {
		profile = model.Profile{ID: profileID, Name: profileID}
	}
	if err := c.EnsureProfile(profile); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	target := c.profiles[profileID]
	if target == nil {
		return fmt.Errorf("profile %q is not running", profileID)
	}
	for id, runtime := range c.profiles {
		setEmbeddedProfileVisibility(runtime.hwnd, id == profileID)
	}
	c.resizeProfileHostLocked()
	activateEmbeddedProfileWindow(c.windowHandleLocked(), target.hwnd)
	return nil
}

func (c *Controller) RemoveProfile(profileID string) {
	c.mu.Lock()
	runtime := c.profiles[profileID]
	handler := c.profileRemovedHandler
	delete(c.profiles, profileID)
	delete(c.profileDefinitions, profileID)
	c.mu.Unlock()
	stopProfileRuntime(runtime)
	if handler != nil {
		handler(profileID)
	}
}

func (c *Controller) RestartProfiles(settings model.Settings) error {
	if !profileHostSupported() {
		return errors.New("isolated profile windows are currently supported on Windows only")
	}
	c.setProfileDefinitions(settings)
	c.stopAllProfiles()
	if _, err := c.waitForShellHandle(30 * time.Second); err != nil {
		return err
	}
	active, ok := settings.Profiles[settings.ActiveProfileID]
	if !ok {
		return errors.New("active WhatsApp profile is missing")
	}
	if err := c.EnsureProfile(active); err != nil {
		return err
	}
	if err := c.ActivateProfile(active.ID); err != nil {
		return err
	}
	for _, id := range settings.ProfileOrder {
		if id == active.ID {
			continue
		}
		profile, ok := settings.Profiles[id]
		if ok {
			go func(profile model.Profile) {
				if err := c.EnsureProfile(profile); err != nil {
					logDesktop("profile %q failed to restart: %v", profile.ID, err)
				}
			}(profile)
		}
	}
	return nil
}

func (c *Controller) launchProfile(profile model.Profile, runtimeDirectory string, parentHandle uintptr) (*profileRuntime, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve BetterWhatsApp executable: %w", err)
	}
	token, err := newProfileToken()
	if err != nil {
		return nil, fmt.Errorf("create profile IPC token: %w", err)
	}
	command, err := startProfileProcess(
		executable,
		profile.ID,
		parentHandle,
		token,
	)
	if err != nil {
		return nil, err
	}
	pid := uint32(command.Process.Pid)
	hwnd, err := waitForProfileWindow(pid, 30*time.Second)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("wait for profile %q window: %w", profile.ID, err)
	}
	c.mu.Lock()
	hostHandle := c.profileHost
	c.mu.Unlock()
	if hostHandle == 0 {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, errors.New("isolated profile host is not ready")
	}
	width, height := c.profileBounds()
	if err := embedProfileWindow(hwnd, hostHandle, 0, 0, width, height); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("embed profile %q window: %w", profile.ID, err)
	}
	runtime := &profileRuntime{
		profileID: profile.ID,
		token:     token,
		command:   command,
		pid:       pid,
		hwnd:      hwnd,
	}
	go c.watchProfile(runtime)
	return runtime, nil
}

func (c *Controller) watchProfile(runtime *profileRuntime) {
	_ = runtime.command.Wait()
	c.mu.Lock()
	handler := c.profileRemovedHandler
	if c.profiles[runtime.profileID] == runtime {
		delete(c.profiles, runtime.profileID)
	}
	c.mu.Unlock()
	if handler != nil {
		handler(runtime.profileID)
	}
}

func (c *Controller) waitForShellHandle(timeout time.Duration) (uintptr, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		c.mu.Lock()
		handle := c.windowHandleLocked()
		if handle != 0 {
			if err := c.ensureProfileHostLocked(); err == nil {
				c.mu.Unlock()
				return handle, nil
			} else {
				lastErr = err
			}
		}
		c.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, errors.New("BetterWhatsApp shell native window did not become ready")
}

func (c *Controller) ensureProfileHostLocked() error {
	if c.profileHost != 0 {
		return nil
	}
	parent := c.windowHandleLocked()
	if parent == 0 {
		return errors.New("BetterWhatsApp shell native window is not ready")
	}
	host, err := createProfileHost(parent)
	if err != nil {
		return fmt.Errorf("create isolated profile host: %w", err)
	}
	c.profileHost = host
	if err := resizeProfileHost(host, parent, profileHostTop); err != nil {
		destroyProfileHost(host)
		c.profileHost = 0
		return fmt.Errorf("size isolated profile host: %w", err)
	}
	return nil
}

func (c *Controller) destroyProfileHostLocked() {
	if c.profileHost == 0 {
		return
	}
	destroyProfileHost(c.profileHost)
	c.profileHost = 0
}

func (c *Controller) windowHandleLocked() uintptr {
	if c.window == nil {
		return 0
	}
	return nativeWindowHandle(c.window)
}

func (c *Controller) profileBounds() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.profileBoundsLocked()
}

func (c *Controller) profileBoundsLocked() (int, int) {
	width, height := 1280, 800
	if c.profileHost != 0 {
		if hostWidth, hostHeight, ok := profileHostClientSize(c.profileHost); ok {
			width, height = hostWidth, hostHeight
		}
	} else if c.window != nil {
		width, height = c.window.Size()
	}
	if width < 1 {
		width = 1280
	}
	if height < 1 {
		height = 1
	}
	return width, height
}

func (c *Controller) resizeEmbeddedProfiles() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resizeProfileHostLocked()
	for _, runtime := range c.profiles {
		c.resizeEmbeddedProfileLocked(runtime)
	}
}

func (c *Controller) resizeEmbeddedProfileLocked(runtime *profileRuntime) {
	if runtime == nil || c.window == nil || c.profileHost == 0 {
		return
	}
	width, height := c.profileBoundsLocked()
	_ = resizeEmbeddedProfileWindow(runtime.hwnd, c.profileHost, 0, 0, width, height)
}

func (c *Controller) resizeProfileHostLocked() {
	if c.window == nil || c.profileHost == 0 {
		return
	}
	if err := resizeProfileHost(c.profileHost, c.windowHandleLocked(), profileHostTop); err != nil {
		logDesktop("resize isolated profile host failed: %v", err)
	}
}

func (c *Controller) setProfileDefinitions(settings model.Settings) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, profile := range settings.Profiles {
		c.profileDefinitions[id] = profile
	}
}

func (c *Controller) stopAllProfiles() {
	c.mu.Lock()
	runtimes := make([]*profileRuntime, 0, len(c.profiles))
	c.profiles = make(map[string]*profileRuntime)
	for _, runtime := range c.profiles {
		runtimes = append(runtimes, runtime)
	}
	c.mu.Unlock()
	for _, runtime := range runtimes {
		stopProfileRuntime(runtime)
	}
}

func stopProfileRuntime(runtime *profileRuntime) {
	if runtime == nil || runtime.command == nil || runtime.command.Process == nil {
		return
	}
	_ = runtime.command.Process.Kill()
}

func newProfileToken() (string, error) {
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	return hex.EncodeToString(token), nil
}

func (c *Controller) Quit() {
	c.closeAllowed.Store(true)
	c.stopAllProfiles()
	c.mu.Lock()
	c.destroyProfileHostLocked()
	c.mu.Unlock()
	c.app.Quit()
}

func (c *Controller) OpenSurface(surface string) error {
	name, title, ok := surfaceDetails(surface)
	if !ok {
		return fmt.Errorf("unknown BetterWhatsApp surface %q", surface)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if window := c.surfaces[name]; window != nil {
		window.Show()
		window.Focus()
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

func (c *Controller) installCloseToTrayHook(window *application.WebviewWindow) {
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if c.closeAllowed.Load() {
			return
		}
		event.Cancel()
		window.Hide()
	})
}

func (c *Controller) closeForReplacementLocked(window *application.WebviewWindow) {
	c.closeAllowed.Store(true)
	window.Close()
	c.closeAllowed.Store(false)
}

func (c *Controller) remoteWindowOptions(settings model.Settings, script string, hidden bool) application.WebviewWindowOptions {
	initialPosition := application.WindowCentered
	initialX, initialY := 0, 0
	initiallyHidden := hidden
	if hidden {
		// Wails hides the WebView2 controller itself for Hidden windows. That
		// controller remains hidden when the native host is later reparented,
		// leaving a black surface even when its HWND is visible.
		initialPosition = application.WindowXY
		initialX = profileBootstrapXY
		initialY = profileBootstrapXY
		initiallyHidden = false
	}

	return application.WebviewWindowOptions{
		Name:                       whatsappWindowName,
		Title:                      "BetterWhatsApp",
		Width:                      1440,
		Height:                     920,
		MinWidth:                   960,
		MinHeight:                  640,
		InitialPosition:            initialPosition,
		X:                          initialX,
		Y:                          initialY,
		Frameless:                  true,
		Hidden:                     initiallyHidden,
		URL:                        settings.WhatsAppURL,
		HTML:                       redirectHTML(settings.WhatsAppURL),
		JS:                         script,
		AllowSimpleEventEmit:       false,
		EnableFileDrop:             false,
		DefaultContextMenuDisabled: false,
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:                   hidden,
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
	// Kept behind a tiny helper so platform-specific process code does not
	// introduce a logging dependency into the public controller API.
	fmt.Printf("[BetterWhatsApp] "+format+"\n", args...)
}
