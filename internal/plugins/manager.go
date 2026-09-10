package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"betterwhatsapp/internal/extensions"
	"betterwhatsapp/internal/model"
)

type Manager struct {
	catalog *extensions.Catalog
}

func NewManager(catalog *extensions.Catalog) *Manager {
	return &Manager{catalog: catalog}
}

const pluginTemplateSource = `(() => {
  "use strict";

  BetterWhatsApp.registerPlugin("__PLUGIN_ID__", ({ WPP, addStyle, observe, log }) => {
    addStyle("__PLUGIN_ID__-style", [
      "[data-better-whatsapp='__PLUGIN_ID__'] {",
      "  color: var(--bw-accent, #50e58b);",
      "  font-weight: 700;",
      "}",
    ].join("\n"));

    observe("[contenteditable='true']", (composer) => {
      if (composer.parentElement?.querySelector("[data-better-whatsapp='__PLUGIN_ID__']")) {
        return;
      }

      const marker = document.createElement("span");
      marker.dataset.betterWhatsapp = "__PLUGIN_ID__";
      marker.textContent = "Plugin ativo";
      marker.title = "Remova este bloco quando não precisar do exemplo";
      composer.parentElement?.append(marker);
    });

    log("__PLUGIN_ID__ carregado", { WPP });
  });
})();
`

// CreateTemplate creates a disabled, editable user plugin without replacing an
// existing bundled plugin or a previous user project.
func (m *Manager) CreateTemplate(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("plugin name is required")
	}
	if len([]rune(name)) > 80 {
		return "", errors.New("plugin name is too long")
	}

	manifests, err := m.catalog.Load()
	if err != nil {
		return "", err
	}
	usedIDs := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		usedIDs[manifest.ID] = struct{}{}
	}

	baseID := pluginTemplateID(name)
	manifest := extensions.Manifest{
		Name:        name,
		Version:     "0.1.0",
		Description: "Plugin local criado pelo template do BetterWhatsApp.",
		Entry:       "index.js",
	}
	sourceFor := func(id string) []byte {
		return []byte(strings.ReplaceAll(pluginTemplateSource, "__PLUGIN_ID__", id))
	}

	for suffix := 1; suffix <= 100; suffix++ {
		id := baseID
		if suffix > 1 {
			id = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		if _, exists := usedIDs[id]; exists {
			continue
		}

		manifest.ID = id
		if err := m.catalog.CreateUser(manifest, sourceFor(id)); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			return "", fmt.Errorf("create plugin template: %w", err)
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

	return "", errors.New("could not allocate a unique plugin id")
}

func pluginTemplateID(name string) string {
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(name) {
		char = foldPluginRune(char)
		switch {
		case char >= 'a' && char <= 'z' || char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		case builder.Len() > 0 && !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}

	id := strings.Trim(builder.String(), "-")
	if id == "" {
		id = "meu-plugin"
	}
	if len(id) > 55 {
		id = strings.TrimRight(id[:55], "-")
	}
	return id
}

func foldPluginRune(char rune) rune {
	switch char {
	case 'á', 'à', 'â', 'ã', 'ä':
		return 'a'
	case 'ç':
		return 'c'
	case 'é', 'è', 'ê', 'ë':
		return 'e'
	case 'í', 'ì', 'î', 'ï':
		return 'i'
	case 'ñ':
		return 'n'
	case 'ó', 'ò', 'ô', 'õ', 'ö':
		return 'o'
	case 'ú', 'ù', 'û', 'ü':
		return 'u'
	case 'ý', 'ÿ':
		return 'y'
	}
	return char
}

func (m *Manager) List(settings model.Settings) ([]model.PluginInfo, error) {
	manifests, err := m.catalog.Load()
	if err != nil {
		return nil, err
	}
	result := make([]model.PluginInfo, 0, len(manifests))
	for _, manifest := range manifests {
		globalState := settings.Plugins[manifest.ID]
		result = append(result, extensions.ToPluginInfo(manifest, globalState))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (m *Manager) Enabled(settings model.Settings) ([]string, error) {
	manifests, err := m.catalog.Load()
	if err != nil {
		return nil, err
	}
	enabled := make([]string, 0, len(manifests))
	for _, manifest := range manifests {
		isEnabled := settings.Plugins[manifest.ID].Enabled
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
