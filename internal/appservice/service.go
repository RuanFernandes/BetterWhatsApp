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
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	controlWindowName    = "whatsapp"
	controlSurfacePrefix = "betterwhatsapp-"
)

type Service struct {
	store    *config.Store
	plugins  *plugins.Manager
	themes   *themes.Manager
	appState string
	reload   func() error
	profiles ProfileOperations
	surface  func(string) error

	mu sync.Mutex
}

type ProfileOperations interface {
	EnsureProfile(profile model.Profile) error
	ActivateProfile(profileID string) error
	RemoveProfile(profileID string)
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
	)
}

func NewWithReloadAndOperations(
	store *config.Store,
	pluginsManager *plugins.Manager,
	themesManager *themes.Manager,
	appState string,
	reload func() error,
	profileOperations ProfileOperations,
	surfaceOpener func(string) error,
) *Service {
	return &Service{
		store:    store,
		plugins:  pluginsManager,
		themes:   themesManager,
		appState: appState,
		reload:   reload,
		profiles: profileOperations,
		surface:  surfaceOpener,
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

func (s *Service) SetPluginEnabledForProfile(ctx context.Context, profileID, id string, enabled bool) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	return s.updateSettings(func(settings *model.Settings) error {
		if _, ok := settings.Profiles[profileID]; !ok {
			return fmt.Errorf("profile %q was not found", profileID)
		}
		installed, err := s.pluginInstalled(*settings, id)
		if err != nil {
			return err
		}
		if !installed {
			return fmt.Errorf("plugin %q is not installed", id)
		}
		profile := settings.Profiles[profileID]
		if profile.PluginOverrides == nil {
			profile.PluginOverrides = map[string]bool{}
		}
		profile.PluginOverrides[id] = enabled
		settings.Profiles[profileID] = profile
		return nil
	})
}

func (s *Service) ClearPluginOverride(ctx context.Context, profileID, id string) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if err := validateID(id); err != nil {
		return err
	}
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	return s.updateSettings(func(settings *model.Settings) error {
		profile, ok := settings.Profiles[profileID]
		if !ok {
			return fmt.Errorf("profile %q was not found", profileID)
		}
		delete(profile.PluginOverrides, id)
		settings.Profiles[profileID] = profile
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
	window.Hide()
	return nil
}

func (s *Service) ToggleMaximise(ctx context.Context) error {
	window, err := callerWindow(ctx)
	if err != nil {
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
	window.Close()
	return nil
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

func (s *Service) GetProfileState(ctx context.Context, profileID string) (model.AppState, error) {
	if err := requireControlWindow(ctx); err != nil {
		return model.AppState{}, err
	}
	if err := validateProfileID(profileID); err != nil {
		return model.AppState{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	settings := s.store.Snapshot()
	if _, ok := settings.Profiles[profileID]; !ok {
		return model.AppState{}, fmt.Errorf("profile %q was not found", profileID)
	}
	settings.ActiveProfileID = profileID
	return s.stateFromSettings(settings)
}

func (s *Service) CreateProfile(ctx context.Context, name string) (model.ProfileInfo, error) {
	if err := requireControlWindow(ctx); err != nil {
		return model.ProfileInfo{}, err
	}
	name, err := normalizeProfileName(name)
	if err != nil {
		return model.ProfileInfo{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings := s.store.Snapshot()
	id := nextProfileID(settings, name)
	profile := model.Profile{
		ID:              id,
		Name:            name,
		Accent:          nextProfileAccent(settings),
		PluginOverrides: map[string]bool{},
	}
	updated, err := s.store.Update(func(next *model.Settings) error {
		next.Profiles[id] = profile
		next.ProfileOrder = append(next.ProfileOrder, id)
		return nil
	})
	if err != nil {
		return model.ProfileInfo{}, err
	}
	if s.profiles != nil {
		if err := s.profiles.EnsureProfile(profile); err != nil {
			_, _ = s.store.Update(func(next *model.Settings) error {
				delete(next.Profiles, id)
				next.ProfileOrder = removeProfileID(next.ProfileOrder, id)
				if next.ActiveProfileID == id {
					next.ActiveProfileID = model.DefaultProfileID
				}
				return nil
			})
			return model.ProfileInfo{}, fmt.Errorf("prepare profile %q: %w", name, err)
		}
	}
	created := updated.Profiles[id]
	return model.ProfileInfo{ID: created.ID, Name: created.Name, Accent: created.Accent}, nil
}

func (s *Service) RenameProfile(ctx context.Context, profileID, name string) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if err := validateProfileID(profileID); err != nil {
		return err
	}
	name, err := normalizeProfileName(name)
	if err != nil {
		return err
	}
	return s.updateSettings(func(settings *model.Settings) error {
		profile, ok := settings.Profiles[profileID]
		if !ok {
			return fmt.Errorf("profile %q was not found", profileID)
		}
		profile.Name = name
		settings.Profiles[profileID] = profile
		return nil
	})
}

func (s *Service) SelectProfile(ctx context.Context, profileID string) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if err := validateProfileID(profileID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings := s.store.Snapshot()
	if _, ok := settings.Profiles[profileID]; !ok {
		return fmt.Errorf("profile %q was not found", profileID)
	}
	previousID := settings.ActiveProfileID
	if previousID == profileID {
		if s.profiles != nil {
			return s.profiles.ActivateProfile(profileID)
		}
		return nil
	}
	if _, err := s.store.Update(func(next *model.Settings) error {
		next.ActiveProfileID = profileID
		return nil
	}); err != nil {
		return err
	}
	if s.profiles != nil {
		if err := s.profiles.ActivateProfile(profileID); err != nil {
			_, _ = s.store.Update(func(next *model.Settings) error {
				next.ActiveProfileID = previousID
				return nil
			})
			return fmt.Errorf("activate profile %q: %w", profileID, err)
		}
	}
	return nil
}

func (s *Service) DeleteProfile(ctx context.Context, profileID string) error {
	if err := requireControlWindow(ctx); err != nil {
		return err
	}
	if err := validateProfileID(profileID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings := s.store.Snapshot()
	if len(settings.Profiles) <= 1 {
		return errors.New("at least one WhatsApp profile must remain")
	}
	if _, ok := settings.Profiles[profileID]; !ok {
		return fmt.Errorf("profile %q was not found", profileID)
	}
	nextActiveID := settings.ActiveProfileID
	if nextActiveID == profileID {
		for _, candidate := range settings.ProfileOrder {
			if candidate != profileID {
				nextActiveID = candidate
				break
			}
		}
		if _, err := s.store.Update(func(next *model.Settings) error {
			next.ActiveProfileID = nextActiveID
			return nil
		}); err != nil {
			return err
		}
		if s.profiles != nil {
			if err := s.profiles.ActivateProfile(nextActiveID); err != nil {
				_, _ = s.store.Update(func(next *model.Settings) error {
					next.ActiveProfileID = profileID
					return nil
				})
				return fmt.Errorf("activate fallback profile: %w", err)
			}
		}
	}

	if _, err := s.store.Update(func(next *model.Settings) error {
		delete(next.Profiles, profileID)
		next.ProfileOrder = removeProfileID(next.ProfileOrder, profileID)
		return nil
	}); err != nil {
		return err
	}
	if s.profiles != nil {
		s.profiles.RemoveProfile(profileID)
	}
	return nil
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
		AppVersion:      s.appState,
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

func validateProfileID(id string) error {
	if id == "" {
		return errors.New("profile id is required")
	}
	for index, char := range id {
		if char >= 'a' && char <= 'z' ||
			char >= '0' && char <= '9' ||
			char == '-' || char == '_' || char == '.' {
			if index == 0 && char == '-' {
				return errors.New("profile id is invalid")
			}
			continue
		}
		return errors.New("profile id is invalid")
	}
	return nil
}

func normalizeProfileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("profile name is required")
	}
	if len([]rune(name)) > 80 {
		return "", errors.New("profile name is too long")
	}
	return name, nil
}

func nextProfileID(settings model.Settings, name string) string {
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(name) {
		switch {
		case char >= 'a' && char <= 'z' || char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		case !lastDash && builder.Len() > 0:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(builder.String(), "-")
	if id == "" {
		id = "perfil"
	}
	base := id
	for suffix := 2; ; suffix++ {
		if _, exists := settings.Profiles[id]; !exists {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func nextProfileAccent(settings model.Settings) string {
	return []string{"#50e58b", "#82d9ff", "#c8a6f7", "#ffb86c", "#f5c2e7"}[len(settings.Profiles)%5]
}

func removeProfileID(ids []string, target string) []string {
	result := make([]string, 0, len(ids)-1)
	for _, id := range ids {
		if id != target {
			result = append(result, id)
		}
	}
	return result
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
