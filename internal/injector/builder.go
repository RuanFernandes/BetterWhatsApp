package injector

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"betterwhatsapp/internal/model"
)

const maxBundleSize = 8 << 20
const controlSurfaceMarker = "/* BETTERWHATSAPP_CONTROL_SURFACE */"

type ControlAssets struct {
	Script      string
	Style       string
	LogoDataURI string
}

type StateLoader func(model.Settings) (model.AppState, error)

type Builder struct {
	bootstrap string
	wppSource string
	plugins   func(model.Settings) ([]string, error)
	themes    func(model.Settings) ([]string, error)
	control   ControlAssets
	state     StateLoader
}

func NewBuilder(
	bootstrap string,
	wppSource string,
	plugins func(model.Settings) ([]string, error),
	themes func(model.Settings) ([]string, error),
	control ControlAssets,
	state StateLoader,
) *Builder {
	return &Builder{
		bootstrap: bootstrap,
		wppSource: wppSource,
		plugins:   plugins,
		themes:    themes,
		control:   control,
		state:     state,
	}
}

func (b *Builder) Build(settings model.Settings) (string, error) {
	return b.build(settings, true)
}

// BuildForProfile creates the runtime for a child WebView. The profile ID is
// selected by the host, never by the remote page, so plugins and session data
// are resolved against one deterministic namespace.
func (b *Builder) BuildForProfile(settings model.Settings, profileID string) (string, error) {
	settings.ActiveProfileID = profileID
	return b.build(settings, false)
}

func (b *Builder) build(settings model.Settings, showToolbar bool) (string, error) {
	if strings.TrimSpace(b.bootstrap) == "" {
		return "", errors.New("injector bootstrap is empty")
	}
	if b.plugins == nil || b.themes == nil {
		return "", errors.New("injector extension loaders are not configured")
	}
	if strings.TrimSpace(b.control.Script) == "" || strings.TrimSpace(b.control.Style) == "" {
		return "", errors.New("local control surface assets are not configured")
	}
	if b.state == nil {
		return "", errors.New("local control surface state loader is not configured")
	}
	if strings.TrimSpace(b.wppSource) == "" && settings.Injector.Enabled {
		return "", errors.New("WA-JS source is empty")
	}
	if !strings.Contains(b.bootstrap, controlSurfaceMarker) {
		return "", errors.New("injector bootstrap is missing the control surface marker")
	}

	bootstrapSource := strings.Replace(b.bootstrap, controlSurfaceMarker, b.control.Script, 1)

	pluginSources, err := b.plugins(settings)
	if err != nil {
		return "", fmt.Errorf("load plugins: %w", err)
	}
	themeSources, err := b.themes(settings)
	if err != nil {
		return "", fmt.Errorf("load themes: %w", err)
	}
	controlState, err := b.state(settings)
	if err != nil {
		return "", fmt.Errorf("load control surface state: %w", err)
	}

	enabledLiteral, err := json.Marshal(settings.Injector.Enabled)
	if err != nil {
		return "", fmt.Errorf("encode injector settings: %w", err)
	}
	showToolbarLiteral, err := json.Marshal(showToolbar)
	if err != nil {
		return "", fmt.Errorf("encode control toolbar setting: %w", err)
	}
	wppLiteral, err := json.Marshal(b.wppSource)
	if err != nil {
		return "", fmt.Errorf("encode WA-JS source: %w", err)
	}
	pluginsLiteral, err := json.Marshal(nonNilSources(pluginSources))
	if err != nil {
		return "", fmt.Errorf("encode plugin sources: %w", err)
	}
	themesLiteral, err := json.Marshal(nonNilSources(themeSources))
	if err != nil {
		return "", fmt.Errorf("encode theme sources: %w", err)
	}
	controlStyleLiteral, err := json.Marshal(b.control.Style)
	if err != nil {
		return "", fmt.Errorf("encode control surface style: %w", err)
	}
	logoDataURILiteral, err := json.Marshal(b.control.LogoDataURI)
	if err != nil {
		return "", fmt.Errorf("encode BetterWhatsApp logo: %w", err)
	}
	controlStateLiteral, err := json.Marshal(controlState)
	if err != nil {
		return "", fmt.Errorf("encode control surface state: %w", err)
	}

	var builder strings.Builder
	builder.Grow(len(bootstrapSource) + len(b.wppSource) + len(b.control.Style) + len(b.control.LogoDataURI) + 8192)
	builder.WriteString("window.__BETTER_WHATSAPP_INJECTOR__ = {")
	builder.WriteString("enabled:")
	_, _ = builder.Write(enabledLiteral)
	builder.WriteString(",showToolbar:")
	_, _ = builder.Write(showToolbarLiteral)
	builder.WriteString(",logo:")
	_, _ = builder.Write(logoDataURILiteral)
	builder.WriteString(",wpp:")
	_, _ = builder.Write(wppLiteral)
	builder.WriteString(",plugins:")
	_, _ = builder.Write(pluginsLiteral)
	builder.WriteString(",themes:")
	_, _ = builder.Write(themesLiteral)
	builder.WriteString(",control:{style:")
	_, _ = builder.Write(controlStyleLiteral)
	builder.WriteString(",state:")
	_, _ = builder.Write(controlStateLiteral)
	builder.WriteString("}};")
	builder.WriteString(bootstrapSource)

	result := builder.String()
	if len(result) > maxBundleSize {
		return "", fmt.Errorf("injector bundle exceeds %d bytes", maxBundleSize)
	}
	return result, nil
}

func nonNilSources(sources []string) []string {
	if sources == nil {
		return []string{}
	}
	return sources
}
