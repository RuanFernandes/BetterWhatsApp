package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"betterwhatsapp/internal/extensions"
	"betterwhatsapp/internal/model"
)

type Manager struct {
	catalog *extensions.Catalog
}

func NewManager(catalog *extensions.Catalog) *Manager {
	return &Manager{catalog: catalog}
}

func (m *Manager) List(settings model.Settings) ([]model.PluginInfo, error) {
	return m.ListForProfile(settings, settings.ActiveProfileID)
}

func (m *Manager) ListForProfile(settings model.Settings, profileID string) ([]model.PluginInfo, error) {
	manifests, err := m.catalog.Load()
	if err != nil {
		return nil, err
	}
	profile, hasProfile := settings.Profiles[profileID]
	result := make([]model.PluginInfo, 0, len(manifests))
	for _, manifest := range manifests {
		globalState := settings.Plugins[manifest.ID]
		var override *bool
		if hasProfile {
			if value, ok := profile.PluginOverrides[manifest.ID]; ok {
				valueCopy := value
				override = &valueCopy
			}
		}
		result = append(result, extensions.ToPluginInfoForProfile(manifest, globalState, override))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (m *Manager) Enabled(settings model.Settings) ([]string, error) {
	return m.EnabledForProfile(settings, settings.ActiveProfileID)
}

func (m *Manager) EnabledForProfile(settings model.Settings, profileID string) ([]string, error) {
	manifests, err := m.catalog.Load()
	if err != nil {
		return nil, err
	}
	profile, hasProfile := settings.Profiles[profileID]
	enabled := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		isEnabled := settings.Plugins[manifest.ID].Enabled
		if hasProfile {
			if override, ok := profile.PluginOverrides[manifest.ID]; ok {
				isEnabled = override
			}
		}
		if !isEnabled {
			continue
		}
		source, err := m.catalog.LoadEntry(manifest.ID, manifest.Entry)
		if err != nil {
			return nil, err
		}
		enabled = append(enabled, string(source))
	}
	return enabled, nil
}

func (m *Manager) EnsureDefaults(settings *model.Settings) error {
	manifests, err := m.catalog.Load()
	if err != nil {
		return err
	}
	for _, manifest := range manifests {
		if _, ok := settings.Plugins[manifest.ID]; !ok {
			settings.Plugins[manifest.ID] = model.PluginState{Enabled: false}
		}
	}
	installed := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		installed[manifest.ID] = struct{}{}
	}
	for id, profile := range settings.Profiles {
		if profile.PluginOverrides == nil {
			profile.PluginOverrides = map[string]bool{}
		}
		for pluginID := range profile.PluginOverrides {
			if _, ok := installed[pluginID]; !ok {
				delete(profile.PluginOverrides, pluginID)
			}
		}
		settings.Profiles[id] = profile
	}
	return nil
}

func (m *Manager) EnsureProject(id string) (string, error) {
	manifests, err := m.catalog.Load()
	if err != nil {
		return "", err
	}

	var manifest extensions.Manifest
	found := false
	for _, candidate := range manifests {
		if candidate.ID == id {
			manifest = candidate
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("plugin %q was not found", id)
	}

	isUser, err := m.catalog.IsUser(id)
	if err != nil {
		return "", err
	}
	if !isUser {
		source, err := m.catalog.LoadEntry(id, manifest.Entry)
		if err != nil {
			return "", err
		}
		if err := m.catalog.CreateUser(manifest, source); err != nil {
			return "", fmt.Errorf("create editable project for plugin %q: %w", id, err)
		}
	}

	projectPath, err := m.catalog.UserPath(id)
	if err != nil {
		return "", err
	}
	if err := ensureProjectReadme(projectPath, manifest); err != nil {
		return "", err
	}
	return projectPath, nil
}

func ensureProjectReadme(projectPath string, manifest extensions.Manifest) error {
	readmePath := filepath.Join(projectPath, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect plugin project documentation: %w", err)
	}

	content := fmt.Sprintf(
		"# %s\n\n"+
			"Plugin local do BetterWhatsApp.\n\n"+
			"- ID: %q\n"+
			"- Entry: %q\n\n"+
			"## API disponível\n\n"+
			"BetterWhatsApp.registerPlugin(id, factory) recebe:\n\n"+
			"- WPP: APIs do WA-JS/WPPConnect.\n"+
			"- addStyle(id, css): injeta CSS no WhatsApp Web.\n"+
			"- observe(selector, callback): observa elementos que aparecem no DOM.\n"+
			"- log(message, error?): escreve no console do BetterWhatsApp.\n\n"+
			"O plugin roda dentro da WebView remota do WhatsApp. Evite acessar serviços Go\n"+
			"diretamente; use somente a API documentada e valide qualquer dado externo.\n",
		manifest.Name,
		manifest.ID,
		manifest.Entry,
	)
	if err := os.WriteFile(readmePath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write plugin project documentation: %w", err)
	}
	return nil
}
