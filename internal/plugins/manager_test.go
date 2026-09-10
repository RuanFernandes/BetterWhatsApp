package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"betterwhatsapp/internal/extensions"
)

func TestCreateTemplateCreatesUniqueEditableProject(t *testing.T) {
	userRoot := t.TempDir()
	catalog := extensions.NewCatalog(fstest.MapFS{}, userRoot, ".js")
	manager := NewManager(catalog)

	firstPath, err := manager.CreateTemplate("Reações rápidas")
	if err != nil {
		t.Fatalf("CreateTemplate() error = %v", err)
	}
	if filepath.Base(firstPath) != "reacoes-rapidas" {
		t.Fatalf("first project path = %q, want reacoes-rapidas", firstPath)
	}

	manifest, err := os.ReadFile(filepath.Join(firstPath, "manifest.json"))
	if err != nil {
		t.Fatalf("ReadFile(manifest) error = %v", err)
	}
	if !strings.Contains(string(manifest), `"id": "reacoes-rapidas"`) {
		t.Fatalf("manifest = %s, want generated id", manifest)
	}
	entry, err := os.ReadFile(filepath.Join(firstPath, "index.js"))
	if err != nil {
		t.Fatalf("ReadFile(entry) error = %v", err)
	}
	if !strings.Contains(string(entry), `registerPlugin("reacoes-rapidas"`) {
		t.Fatalf("entry does not contain the generated plugin id")
	}
	if _, err := os.Stat(filepath.Join(firstPath, "README.md")); err != nil {
		t.Fatalf("README.md was not created: %v", err)
	}

	secondPath, err := manager.CreateTemplate("Reações rápidas")
	if err != nil {
		t.Fatalf("second CreateTemplate() error = %v", err)
	}
	if filepath.Base(secondPath) != "reacoes-rapidas-2" {
		t.Fatalf("second project path = %q, want reacoes-rapidas-2", secondPath)
	}
}

func TestPluginTemplateIDHasFallbackAndLengthLimit(t *testing.T) {
	if got := pluginTemplateID("!!!"); got != "meu-plugin" {
		t.Fatalf("pluginTemplateID(!!!) = %q, want meu-plugin", got)
	}
	if got := pluginTemplateID(strings.Repeat("a", 100)); len(got) > 55 {
		t.Fatalf("pluginTemplateID() length = %d, want <= 55", len(got))
	}
}
