package injector

import (
	"strings"
	"testing"

	"betterwhatsapp/internal/config"
	"betterwhatsapp/internal/model"
)

func testControlAssets() ControlAssets {
	return ControlAssets{
		Script:      "window.BetterWhatsAppControl = {};",
		Style:       ".control-panel { color: white; }",
		LogoDataURI: "data:image/png;base64,logo",
	}
}

func testStateLoader(model.Settings) (model.AppState, error) {
	return model.AppState{AppVersion: "test"}, nil
}

func TestBuilderSerializesLocalSources(t *testing.T) {
	builder := NewBuilder(
		"console.log('bootstrap')\n/* BETTERWHATSAPP_CONTROL_SURFACE */",
		"window.WPP = { loader: { onReady() {} } };",
		func(model.Settings) ([]string, error) {
			return []string{"BetterWhatsApp.registerPlugin('demo', () => {})"}, nil
		},
		func(model.Settings) ([]string, error) {
			return []string{":root { --bw-accent: #25d366; }"}, nil
		},
		testControlAssets(),
		testStateLoader,
	)

	script, err := builder.Build(config.DefaultSettings())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, expected := range []string{
		"window.__BETTER_WHATSAPP_INJECTOR__",
		"enabled:true",
		"logo:\"data:image/png;base64,logo\"",
		"BetterWhatsApp.registerPlugin",
		"--bw-accent",
		"window.BetterWhatsAppControl",
		"appVersion",
		"console.log('bootstrap')",
	} {
		if !strings.Contains(script, expected) {
			t.Errorf("Build() result does not contain %q", expected)
		}
	}
}

func TestBuilderSerializesDisabledInjectorAsBoolean(t *testing.T) {
	builder := NewBuilder(
		"void 0\n/* BETTERWHATSAPP_CONTROL_SURFACE */",
		"void 0",
		func(model.Settings) ([]string, error) { return nil, nil },
		func(model.Settings) ([]string, error) { return nil, nil },
		testControlAssets(),
		testStateLoader,
	)
	settings := config.DefaultSettings()
	settings.Injector.Enabled = false

	script, err := builder.Build(settings)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(script, "enabled:false") {
		t.Fatalf("Build() did not serialize a boolean disabled flag: %s", script)
	}
}
