package config

import (
	"strings"
	"testing"

	"betterwhatsapp/internal/model"
)

func TestNormalizeDefaultsAndValidatesOrigin(t *testing.T) {
	settings, err := Normalize(model.Settings{})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if settings.SchemaVersion != model.CurrentSchemaVersion {
		t.Fatalf("schema version = %d, want %d", settings.SchemaVersion, model.CurrentSchemaVersion)
	}
	if settings.WhatsAppURL != defaultWhatsAppURL {
		t.Fatalf("WhatsAppURL = %q, want %q", settings.WhatsAppURL, defaultWhatsAppURL)
	}
	if !settings.Injector.Enabled {
		t.Fatal("injector should be enabled by default")
	}
}

func TestNormalizeRejectsNonOfficialOrigin(t *testing.T) {
	settings := DefaultSettings()
	settings.WhatsAppURL = "https://example.com/"

	_, err := Normalize(settings)
	if err == nil || !strings.Contains(err.Error(), "web.whatsapp.com") {
		t.Fatalf("Normalize() error = %v, want official-origin validation error", err)
	}
}

func TestNormalizeRejectsCredentialsAndQuery(t *testing.T) {
	settings := DefaultSettings()
	settings.WhatsAppURL = "https://user:pass@web.whatsapp.com/?token=secret"

	if _, err := Normalize(settings); err == nil {
		t.Fatal("Normalize() accepted credentials/query parameters")
	}
}
