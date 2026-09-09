package model

const CurrentSchemaVersion = 2

const DefaultProfileID = "pessoal"

type Settings struct {
	SchemaVersion   int                    `json:"schemaVersion"`
	Injector        InjectorSettings       `json:"injector"`
	Plugins         map[string]PluginState `json:"plugins"`
	Themes          map[string]ThemeState  `json:"themes"`
	ActiveThemeID   string                 `json:"activeThemeId"`
	WhatsAppURL     string                 `json:"whatsappUrl"`
	Profiles        map[string]Profile     `json:"profiles"`
	ProfileOrder    []string               `json:"profileOrder"`
	ActiveProfileID string                 `json:"activeProfileId"`
}

type InjectorSettings struct {
	Enabled bool `json:"enabled"`
}

type PluginState struct {
	Enabled bool `json:"enabled"`
}

type ThemeState struct {
	Enabled bool `json:"enabled"`
}

// Profile is a named, persistent WhatsApp Web account container.
//
// PluginOverrides uses map presence to distinguish "inherit the global
// setting" from an explicit enable/disable decision for this profile.
type Profile struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Accent            string          `json:"accent"`
	LegacyWebviewData bool            `json:"legacyWebviewData,omitempty"`
	PluginOverrides   map[string]bool `json:"pluginOverrides,omitempty"`
}

type ProfileInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Accent string `json:"accent"`
}

type PluginInfo struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Description     string `json:"description"`
	Author          string `json:"author"`
	Enabled         bool   `json:"enabled"`
	GlobalEnabled   bool   `json:"globalEnabled"`
	ProfileOverride *bool  `json:"profileOverride,omitempty"`
	Entry           string `json:"entry"`
}

type ThemeInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Enabled     bool   `json:"enabled"`
	Active      bool   `json:"active"`
	Editable    bool   `json:"editable"`
	Entry       string `json:"entry"`
}

type AppState struct {
	AppVersion      string           `json:"appVersion"`
	Injector        InjectorSettings `json:"injector"`
	Plugins         []PluginInfo     `json:"plugins"`
	Themes          []ThemeInfo      `json:"themes"`
	ActiveThemeID   string           `json:"activeThemeId"`
	WhatsAppURL     string           `json:"whatsappUrl"`
	RemoteOrigin    string           `json:"remoteOrigin"`
	Profiles        []ProfileInfo    `json:"profiles"`
	ActiveProfileID string           `json:"activeProfileId"`
}
