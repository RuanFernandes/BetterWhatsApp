package themes

import (
	"testing"

	"betterwhatsapp/internal/extensions"
	"betterwhatsapp/internal/model"
)

func TestManagerCreatesAndPersistsUserCSS(t *testing.T) {
	manager := NewManager(extensions.NewCatalog(nil, t.TempDir(), ".css"))
	settings := model.Settings{
		Themes: map[string]model.ThemeState{},
	}

	id, err := manager.Create("Graphite & compact", "body { color: red; }")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id != "graphite-compact" {
		t.Fatalf("Create() id = %q, want graphite-compact", id)
	}
	settings.Themes[id] = model.ThemeState{Enabled: true}

	list, err := manager.List(settings)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || !list[0].Editable || !list[0].Enabled {
		t.Fatalf("List() = %#v, want one editable enabled theme", list)
	}

	enabled, err := manager.Enabled(settings)
	if err != nil {
		t.Fatalf("Enabled() error = %v", err)
	}
	if len(enabled) != 1 || enabled[0] != "body { color: red; }" {
		t.Fatalf("Enabled() = %#v, want persisted CSS", enabled)
	}

	if err := manager.Update(id, "Graphite refined", "body { color: green; }"); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	source, err := manager.Read(id)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if source != "body { color: green; }" {
		t.Fatalf("Read() = %q, want updated CSS", source)
	}

	if err := manager.Delete(id); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	list, err = manager.List(settings)
	if err != nil {
		t.Fatalf("List() after Delete() error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List() after Delete() = %#v, want empty", list)
	}
}

func TestEnsureDefaultsRemovesStaleThemeState(t *testing.T) {
	manager := NewManager(extensions.NewCatalog(nil, t.TempDir(), ".css"))
	settings := model.Settings{
		Themes:        map[string]model.ThemeState{"midnight": {Enabled: true}},
		ActiveThemeID: "midnight",
	}

	if err := manager.EnsureDefaults(&settings); err != nil {
		t.Fatalf("EnsureDefaults() error = %v", err)
	}
	if len(settings.Themes) != 0 || settings.ActiveThemeID != "" {
		t.Fatalf("settings after EnsureDefaults() = %#v, want no stale theme state", settings)
	}
}
