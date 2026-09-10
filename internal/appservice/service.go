package appservice

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"betterwhatsapp/internal/config"
	"betterwhatsapp/internal/desktop"
	"betterwhatsapp/internal/model"
	"betterwhatsapp/internal/plugins"
	"betterwhatsapp/internal/themes"
	"betterwhatsapp/internal/updater"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	controlWindowName    = "whatsapp"
	controlSurfacePrefix = "betterwhatsapp-"
)

type Service struct {
	store       *config.Store
	plugins     *plugins.Manager
	themes      *themes.Manager
	updates     UpdateOperations
	appState    string
	reload      func() error
	nativeShell nativeShellOperations
	surface     func(string) error

	mu sync.Mutex
}

type nativeShellOperations interface {
	HideWhatsApp() error
	ToggleMaximise() error
}

type UpdateOperations interface {
	Check(context.Context, string) (updater.Release, bool, error)
	Download(context.Context, updater.Release) (updater.Downloaded, error)
}

func New(
	store *config.Store,
	pluginsManager *plugins.Manager,
	themesManager *themes.Manager,
	appState string,
) *Service {
	return NewWithReload(store, pluginsManager, themesManager, appState, nil)
}

func NewWithReload(
	store *config.Store,
	pluginsManager *plugins.Manager,
	themesManager *themes.Manager,
	appState string,
	reload func() error,
) *Service {
	return NewWithReloadAndOperations(
		store,
		pluginsManager,
		themesManager,
		appState,
		reload,
		nil,
		nil,
		nil,
	)
}

func NewWithReloadAndOperations(
	store *config.Store,
	pluginsManager *plugins.Manager,
	themesManager *themes.Manager,
	appState string,
	reload func() error,
	nativeShell nativeShellOperations,
	surfaceOpener func(string) error,
	updateOperations UpdateOperations,
) *Service {
	return &Service{
		store:       store,
		plugins:     pluginsManager,
		themes:      themesManager,
		updates:     updateOperations,
		appState:    appState,
		reload:      reload,
		nativeShell: nativeShell,
		surface:     surfaceOpener,
	}
}

func (s *Service) GetState(ctx context.Context) (model.AppState, error) {
	if err := requireControlWindow(ctx); err != nil {
		return model.AppState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked()
}

func (s *Service) CheckForUpdate(ctx context.Context) (model.UpdateInfo, error) {
	if err := requireControlWindow(ctx); err != nil {
		return model.UpdateInfo{}, err
	}

	operations, currentVersion, err := s.updateOperations()
	if err != nil {
		return model.UpdateInfo{}, err
	}
	release, available, err := operations.Check(ctx, currentVersion)
	if err != nil {
		return model.UpdateInfo{}, err
	}
	return makeUpdateInfo(currentVersion, release, available), nil
}

func (s *Service) DownloadUpdate(ctx context.Context) (model.UpdateInfo, error) {
	if err := requireControlWindow(ctx); err != nil {
		return model.UpdateInfo{}, err
	}

	operations, currentVersion, err := s.updateOperations()
	if err != nil {
		return model.UpdateInfo{}, err
	}
	release, available, err := operations.Check(ctx, currentVersion)
	if err != nil {
		return model.UpdateInfo{}, err
	}

	info := makeUpdateInfo(currentVersion, release, available)
	if !available {
		return info, nil
	}

	downloaded, err := operations.Download(ctx, release)
	if err != nil {
		return model.UpdateInfo{}, err
	}
	info.Downloaded = true
	info.DownloadedBytes = downloaded.Size
	return info, nil
}

func (s *Service) SetInjectorEnabled(ctx context.Context, enabled bool) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	return s.updateSettings(func(settings *model.Settings) error {
		settings.Injector.Enabled = enabled
		return nil
	})
}

func (s *Service) SetPluginEnabled(ctx context.Context, id string, enabled bool) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	return s.updateSettings(func(settings *model.Settings) error {
		installed, err := s.pluginInstalled(*settings, id)
		if err != nil {
			return err
		}
		if !installed {
			return fmt.Errorf("plugin %q is not installed", id)
		}
		settings.Plugins[id] = model.PluginState{Enabled: enabled}
		return nil
	})
}

func (s *Service) SetThemeEnabled(ctx context.Context, id string, enabled bool) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	return s.updateSettings(func(settings *model.Settings) error {
		installed, err := s.themeInstalled(*settings, id)
		if err != nil {
			return err
		}
		if !installed {
			return fmt.Errorf("theme %q is not installed", id)
		}
		settings.Themes[id] = model.ThemeState{Enabled: enabled}
		settings.ActiveThemeID = ""
		return nil
	})
}

func (s *Service) CreateTheme(ctx context.Context, name, source string) (string, error) {
	if err := requireControlWindow(ctx); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id, err := s.themes.Create(name, source)
	if err != nil {
		return "", err
	}
	if _, err := s.store.Update(func(settings *model.Settings) error {
		settings.Themes[id] = model.ThemeState{Enabled: true}
		settings.ActiveThemeID = ""
		return nil
	}); err != nil {
		_ = s.themes.Delete(id)
		return "", err
	}
	return id, nil
}

func (s *Service) UpdateTheme(ctx context.Context, id, name, source string) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.themes.Update(id, name, source)
}

func (s *Service) ReadTheme(ctx context.Context, id string) (string, error) {
	if err := requireControlWindow(ctx); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.themes.Read(id)
}

func (s *Service) DeleteTheme(ctx context.Context, id string) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.store.Update(func(settings *model.Settings) error {
		delete(settings.Themes, id)
		if settings.ActiveThemeID == id {
			settings.ActiveThemeID = ""
		}
		return nil
	}); err != nil {
		return err
	}
	return s.themes.Delete(id)
}

func (s *Service) HideWindow(ctx context.Context) error {
	window, err := callerWindow(ctx)
	if err != nil {
		return err
	}
	if handled, err := s.runNativeShellOperation(window, func(operations nativeShellOperations) error {
		return operations.HideWhatsApp()
	}); handled {
		return err
	}
	window.Hide()
	return nil
}

func (s *Service) ToggleMaximise(ctx context.Context) error {
	window, err := callerWindow(ctx)
	if err != nil {
		return err
	}
	if handled, err := s.runNativeShellOperation(window, func(operations nativeShellOperations) error {
		return operations.ToggleMaximise()
	}); handled {
		return err
	}
	window.ToggleMaximise()
	return nil
}

func (s *Service) RequestClose(ctx context.Context) error {
	window, err := callerWindow(ctx)
	if err != nil {
		return err
	}
	if handled, err := s.runNativeShellOperation(window, func(operations nativeShellOperations) error {
		return operations.HideWhatsApp()
	}); handled {
		return err
	}
	window.Hide()
	return nil
}

func (s *Service) runNativeShellOperation(window application.Window, operation func(nativeShellOperations) error) (bool, error) {
	if window == nil || window.Name() != controlWindowName || s.nativeShell == nil {
		return false, nil
	}
	return true, operation(s.nativeShell)
}

func (s *Service) ReloadWhatsApp(ctx context.Context) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if s.reload == nil {
		return errors.New("WhatsApp window reload is not configured")
	}
	return s.reload()
}

func (s *Service) OpenSurface(ctx context.Context, surface string) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if surface != "plugins" && surface != "themes" {
		return fmt.Errorf("unknown BetterWhatsApp surface %q", surface)
	}
	s.mu.Lock()
	opener := s.surface
	s.mu.Unlock()
	if opener == nil {
		return errors.New("BetterWhatsApp surface opener is not configured")
	}
	return opener(surface)
}

func (s *Service) OpenPluginProject(ctx context.Context, id string) (string, error) {
	if err := requireControlWindow(ctx); err != nil {
		return "", err
	}
	if err := validateID(id); err != nil {
		return "", err
	}

	s.mu.Lock()
	projectPath, err := s.plugins.EnsureProject(id)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}

	if err := openInVSCode(projectPath); err != nil {
		return "", fmt.Errorf("projeto preparado em %q, mas não foi possível abrir o VS Code: %w", projectPath, err)
	}
	return projectPath, nil
}

func (s *Service) CreatePluginTemplate(ctx context.Context, name string) (string, error) {
	if err := requireControlWindow(ctx); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.plugins.CreateTemplate(name)
}

func (s *Service) stateLocked() (model.AppState, error) {
	settings := s.store.Snapshot()
	return s.stateFromSettings(settings)
}

func (s *Service) stateFromSettings(settings model.Settings) (model.AppState, error) {
	pluginsList, err := s.plugins.List(settings)
	if err != nil {
		return model.AppState{}, err
	}
	themesList, err := s.themes.List(settings)
	if err != nil {
		return model.AppState{}, err
	}
	return model.AppState{
		AppVersion:    s.appState,
		Injector:      settings.Injector,
		Plugins:       pluginsList,
		Themes:        themesList,
		ActiveThemeID: settings.ActiveThemeID,
		WhatsAppURL:   settings.WhatsAppURL,
		RemoteOrigin:  desktop.RemoteOrigin(),
	}, nil
}

func (s *Service) updateOperations() (UpdateOperations, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.updates == nil {
		return nil, "", errors.New("update checker is not configured")
	}
	return s.updates, s.appState, nil
}

func makeUpdateInfo(currentVersion string, release updater.Release, available bool) model.UpdateInfo {
	return model.UpdateInfo{
		Available:      available,
		CurrentVersion: currentVersion,
		LatestVersion:  release.Version,
		ReleaseName:    release.Name,
		ReleaseURL:     release.URL,
		AssetName:      release.AssetName,
		DownloadURL:    release.DownloadURL,
	}
}

func (s *Service) updateSettings(update func(*model.Settings) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.store.Update(update)
	return err
}

func (s *Service) pluginInstalled(settings model.Settings, id string) (bool, error) {
	pluginsList, err := s.plugins.List(settings)
	if err != nil {
		return false, err
	}
	for _, plugin := range pluginsList {
		if plugin.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) themeInstalled(settings model.Settings, id string) (bool, error) {
	themesList, err := s.themes.List(settings)
	if err != nil {
		return false, err
	}
	for _, theme := range themesList {
		if theme.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func validateID(id string) error {
	if id == "" {
		return errors.New("extension id is required")
	}
	return nil
}

type windowNamer interface {
	Name() string
}

func requireControlWindow(ctx context.Context) error {
	if ctx == nil {
		return errors.New("control window context is required")
	}
	window, ok := ctx.Value(application.WindowKey).(windowNamer)
	if !ok || !isControlWindowName(window.Name()) {
		return errors.New("service is available only to the local control surface")
	}
	return nil
}

func isControlWindowName(name string) bool {
	return name == controlWindowName || strings.HasPrefix(name, controlSurfacePrefix)
}

func openInVSCode(projectPath string) error {
	projectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("resolve plugin project path: %w", err)
	}

	var executable string
	for _, candidate := range []string{"code", "code.cmd", "code-insiders", "code-insiders.cmd"} {
		if path, lookupErr := exec.LookPath(candidate); lookupErr == nil {
			executable = path
			break
		}
	}
	if executable == "" {
		return errors.New("o comando 'code' não está disponível no PATH")
	}

	command := exec.Command(executable, projectPath)
	if strings.HasSuffix(strings.ToLower(filepath.Ext(executable)), ".cmd") {
		command = exec.Command("cmd.exe", "/d", "/c", "call", executable, projectPath)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start VS Code: %w", err)
	}
	return nil
}

func callerWindow(ctx context.Context) (application.Window, error) {
	if err := requireControlWindow(ctx); err != nil {
		return nil, err
	}
	window, ok := ctx.Value(application.WindowKey).(application.Window)
	if !ok {
		return nil, errors.New("control window handle is unavailable")
	}
	return window, nil
}
