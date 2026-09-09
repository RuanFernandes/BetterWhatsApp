package themes

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"betterwhatsapp/internal/extensions"
	"betterwhatsapp/internal/model"
)

const (
	MaxSourceSize = 2 << 20
	themeVersion  = "1.0.0"
)

type Manager struct {
	catalog *extensions.Catalog
}

func NewManager(catalog *extensions.Catalog) *Manager {
	return &Manager{catalog: catalog}
}

func (m *Manager) List(settings model.Settings) ([]model.ThemeInfo, error) {
	manifests, err := m.catalog.Load()
	if err != nil {
		return nil, err
	}
	result := make([]model.ThemeInfo, 0, len(manifests))
	for _, manifest := range manifests {
		state := settings.Themes[manifest.ID]
		info := extensions.ToThemeInfo(manifest, state, false)
		info.Editable, err = m.catalog.IsUser(manifest.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, info)
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
		if !settings.Themes[manifest.ID].Enabled {
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

func (m *Manager) Create(name, source string) (string, error) {
	name, err := validateThemeName(name)
	if err != nil {
		return "", err
	}
	if err := validateSource(source); err != nil {
		return "", err
	}
	manifests, err := m.catalog.Load()
	if err != nil {
		return "", err
	}
	id := nextThemeID(slugify(name), manifests)
	if err := m.catalog.CreateUser(extensions.Manifest{
		ID:          id,
		Name:        name,
		Version:     themeVersion,
		Description: "Arquivo CSS global do usuário.",
		Author:      "Você",
		Entry:       "theme.css",
	}, []byte(source)); err != nil {
		return "", err
	}
	return id, nil
}

func (m *Manager) Update(id, name, source string) error {
	if err := validateThemeID(id); err != nil {
		return err
	}
	name, err := validateThemeName(name)
	if err != nil {
		return err
	}
	if err := validateSource(source); err != nil {
		return err
	}
	editable, err := m.catalog.IsUser(id)
	if err != nil {
		return err
	}
	if !editable {
		return errors.New("only user CSS files can be edited")
	}
	return m.catalog.UpdateUser(extensions.Manifest{
		ID:          id,
		Name:        name,
		Version:     themeVersion,
		Description: "Arquivo CSS global do usuário.",
		Author:      "Você",
		Entry:       "theme.css",
	}, []byte(source))
}

func (m *Manager) Read(id string) (string, error) {
	if err := validateThemeID(id); err != nil {
		return "", err
	}
	editable, err := m.catalog.IsUser(id)
	if err != nil {
		return "", err
	}
	if !editable {
		return "", errors.New("only user CSS files can be read")
	}
	source, err := m.catalog.LoadEntry(id, "theme.css")
	if err != nil {
		return "", err
	}
	return string(source), nil
}

func (m *Manager) Delete(id string) error {
	if err := validateThemeID(id); err != nil {
		return err
	}
	editable, err := m.catalog.IsUser(id)
	if err != nil {
		return err
	}
	if !editable {
		return errors.New("only user CSS files can be deleted")
	}
	return m.catalog.DeleteUser(id)
}

func (m *Manager) EnsureDefaults(settings *model.Settings) error {
	manifests, err := m.catalog.Load()
	if err != nil {
		return err
	}
	if settings.Themes == nil {
		settings.Themes = map[string]model.ThemeState{}
	}

	validIDs := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		validIDs[manifest.ID] = struct{}{}
	}
	for id := range settings.Themes {
		if _, ok := validIDs[id]; !ok {
			delete(settings.Themes, id)
		}
	}
	for _, manifest := range manifests {
		if _, ok := settings.Themes[manifest.ID]; !ok {
			settings.Themes[manifest.ID] = model.ThemeState{Enabled: false}
		}
	}
	settings.ActiveThemeID = ""
	return nil
}

func validateThemeID(id string) error {
	if id == "" || strings.TrimSpace(id) != id {
		return errors.New("theme id is invalid")
	}
	for index, char := range id {
		if char >= 'a' && char <= 'z' ||
			char >= '0' && char <= '9' ||
			index > 0 && (char == '.' || char == '_' || char == '-') {
			continue
		}
		return errors.New("theme id is invalid")
	}
	return nil
}

func validateThemeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("theme name is required")
	}
	if len([]byte(name)) > 80 {
		return "", errors.New("theme name must be at most 80 bytes")
	}
	return name, nil
}

func validateSource(source string) error {
	if len([]byte(source)) > MaxSourceSize {
		return errors.New("CSS source exceeds the 2 MiB limit")
	}
	if strings.IndexByte(source, 0) >= 0 {
		return errors.New("CSS source contains an invalid null byte")
	}
	return nil
}

func slugify(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, char := range strings.ToLower(value) {
		switch {
		case char >= 'a' && char <= 'z':
			if builder.Len() < 48 {
				builder.WriteRune(char)
				lastDash = false
			}
		case char >= '0' && char <= '9':
			if builder.Len() < 48 {
				builder.WriteRune(char)
				lastDash = false
			}
		case builder.Len() > 0 && !lastDash:
			builder.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(builder.String(), "-")
	if id == "" {
		return "theme"
	}
	return id
}

func nextThemeID(base string, manifests []extensions.Manifest) string {
	used := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		used[manifest.ID] = struct{}{}
	}
	if _, ok := used[base]; !ok {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := base + "-" + strconv.Itoa(suffix)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}
