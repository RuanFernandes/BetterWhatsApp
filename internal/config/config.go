package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"betterwhatsapp/internal/model"
)

const defaultWhatsAppURL = "https://web.whatsapp.com/"
const maxProfiles = 32
const maxProfileNameLength = 80

var ErrUnsupportedSchema = errors.New("unsupported settings schema")
var validProfileID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

func DefaultSettings() model.Settings {
	return model.Settings{
		SchemaVersion: model.CurrentSchemaVersion,
		Injector: model.InjectorSettings{
			Enabled: true,
		},
		Plugins:     map[string]model.PluginState{},
		Themes:      map[string]model.ThemeState{},
		WhatsAppURL: defaultWhatsAppURL,
		Profiles: map[string]model.Profile{
			model.DefaultProfileID: {
				ID:                model.DefaultProfileID,
				Name:              "Pessoal",
				Accent:            "#50e58b",
				LegacyWebviewData: true,
				PluginOverrides:   map[string]bool{},
			},
		},
		ProfileOrder:    []string{model.DefaultProfileID},
		ActiveProfileID: model.DefaultProfileID,
	}
}

func Normalize(settings model.Settings) (model.Settings, error) {
	switch settings.SchemaVersion {
	case 0:
		settings.SchemaVersion = 1
		settings.Injector.Enabled = true
		fallthrough
	case 1:
		// Schema 1 had a single WebView session. Keep that session as the
		// first profile so existing logins are not silently lost.
		settings.SchemaVersion = model.CurrentSchemaVersion
		if settings.Profiles == nil {
			settings.Profiles = map[string]model.Profile{
				model.DefaultProfileID: {
					ID:                model.DefaultProfileID,
					Name:              "Pessoal",
					Accent:            "#50e58b",
					LegacyWebviewData: true,
					PluginOverrides:   map[string]bool{},
				},
			}
			settings.ProfileOrder = []string{model.DefaultProfileID}
			settings.ActiveProfileID = model.DefaultProfileID
		}
	case model.CurrentSchemaVersion:
	default:
		return model.Settings{}, fmt.Errorf("%w: %d", ErrUnsupportedSchema, settings.SchemaVersion)
	}

	if settings.Plugins == nil {
		settings.Plugins = map[string]model.PluginState{}
	}
	if settings.Themes == nil {
		settings.Themes = map[string]model.ThemeState{}
	}
	if err := normalizeProfiles(&settings); err != nil {
		return model.Settings{}, err
	}

	settings.WhatsAppURL = strings.TrimSpace(settings.WhatsAppURL)
	if settings.WhatsAppURL == "" {
		settings.WhatsAppURL = defaultWhatsAppURL
	}
	if err := validateWhatsAppURL(settings.WhatsAppURL); err != nil {
		return model.Settings{}, err
	}

	return settings, nil
}

func normalizeProfiles(settings *model.Settings) error {
	if settings.Profiles == nil || len(settings.Profiles) == 0 {
		settings.Profiles = map[string]model.Profile{
			model.DefaultProfileID: {
				ID:                model.DefaultProfileID,
				Name:              "Pessoal",
				Accent:            "#50e58b",
				LegacyWebviewData: true,
				PluginOverrides:   map[string]bool{},
			},
		}
	}
	if len(settings.Profiles) > maxProfiles {
		return fmt.Errorf("at most %d profiles are supported", maxProfiles)
	}

	normalizedProfiles := make(map[string]model.Profile, len(settings.Profiles))
	for mapID, profile := range settings.Profiles {
		id := strings.TrimSpace(mapID)
		if !validProfileID.MatchString(id) {
			return fmt.Errorf("profile %q has an invalid id", mapID)
		}
		profile.ID = id
		profile.Name = strings.TrimSpace(profile.Name)
		if profile.Name == "" {
			profile.Name = humanizeProfileID(id)
		}
		if len([]rune(profile.Name)) > maxProfileNameLength {
			return fmt.Errorf("profile %q name is too long", id)
		}
		profile.Accent = strings.TrimSpace(profile.Accent)
		if profile.Accent == "" {
			profile.Accent = profileAccent(len(normalizedProfiles))
		}
		if profile.PluginOverrides == nil {
			profile.PluginOverrides = map[string]bool{}
		}
		for pluginID := range profile.PluginOverrides {
			if !validProfileID.MatchString(pluginID) {
				return fmt.Errorf("profile %q has an invalid plugin override id %q", id, pluginID)
			}
		}
		normalizedProfiles[id] = profile
	}
	settings.Profiles = normalizedProfiles

	ordered := make([]string, 0, len(settings.Profiles))
	seen := make(map[string]struct{}, len(settings.Profiles))
	for _, id := range settings.ProfileOrder {
		if _, ok := settings.Profiles[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ordered = append(ordered, id)
	}
	missing := make([]string, 0, len(settings.Profiles)-len(ordered))
	for id := range settings.Profiles {
		if _, ok := seen[id]; !ok {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	ordered = append(ordered, missing...)
	if len(ordered) == 0 {
		return errors.New("at least one profile is required")
	}
	settings.ProfileOrder = ordered

	if _, ok := settings.Profiles[settings.ActiveProfileID]; !ok {
		settings.ActiveProfileID = ordered[0]
	}
	return nil
}

func humanizeProfileID(id string) string {
	parts := strings.FieldsFunc(id, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[index] = string(runes)
	}
	if len(parts) == 0 {
		return "Perfil"
	}
	return strings.Join(parts, " ")
}

func profileAccent(index int) string {
	accents := []string{"#50e58b", "#82d9ff", "#c8a6f7", "#ffb86c", "#f5c2e7"}
	return accents[index%len(accents)]
}

func OrderedProfiles(settings model.Settings) []model.Profile {
	result := make([]model.Profile, 0, len(settings.ProfileOrder))
	for _, id := range settings.ProfileOrder {
		if profile, ok := settings.Profiles[id]; ok {
			profile.PluginOverrides = cloneBoolMap(profile.PluginOverrides)
			result = append(result, profile)
		}
	}
	return result
}

func ProfileInfos(settings model.Settings) []model.ProfileInfo {
	profiles := OrderedProfiles(settings)
	result := make([]model.ProfileInfo, 0, len(profiles))
	for _, profile := range profiles {
		result = append(result, model.ProfileInfo{
			ID:     profile.ID,
			Name:   profile.Name,
			Accent: profile.Accent,
		})
	}
	return result
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	if source == nil {
		return map[string]bool{}
	}
	cloned := make(map[string]bool, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func validateWhatsAppURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid WhatsApp URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "web.whatsapp.com" {
		return errors.New("WhatsApp URL must use https://web.whatsapp.com")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("WhatsApp URL cannot contain credentials, query parameters, or fragments")
	}
	return nil
}
