package extensions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCatalogLoadsBundledAndUserOverrides(t *testing.T) {
	bundled := fstest.MapFS{
		"quick/manifest.json": &fstest.MapFile{Data: []byte("{\"id\":\"quick\",\"name\":\"Bundled\",\"version\":\"1.0.0\",\"entry\":\"index.js\"}")},
		"quick/index.js":      &fstest.MapFile{Data: []byte("bundled")},
	}
	userRoot := t.TempDir()
	userPluginRoot := filepath.Join(userRoot, "quick")
	if err := os.MkdirAll(userPluginRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(userPluginRoot, "manifest.json"), []byte("{\"id\":\"quick\",\"name\":\"User\",\"version\":\"2.0.0\",\"entry\":\"custom.js\"}"), 0o600); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(userPluginRoot, "custom.js"), []byte("user"), 0o600); err != nil {
		t.Fatalf("WriteFile(entry) error = %v", err)
	}

	catalog := NewCatalog(bundled, userRoot, ".js")
	manifests, err := catalog.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(manifests) != 1 || manifests[0].Name != "User" {
		t.Fatalf("manifests = %#v, want user override", manifests)
	}

	source, err := catalog.LoadEntry("quick", manifests[0].Entry)
	if err != nil {
		t.Fatalf("LoadEntry() error = %v", err)
	}
	if string(source) != "user" {
		t.Fatalf("source = %q, want user override", source)
	}
}

func TestCatalogRejectsPathTraversal(t *testing.T) {
	bundled := fstest.MapFS{
		"quick/manifest.json": &fstest.MapFile{Data: []byte("{\"id\":\"quick\",\"name\":\"Quick\",\"version\":\"1.0.0\"}")},
		"quick/index.js":      &fstest.MapFile{Data: []byte("source")},
	}
	catalog := NewCatalog(bundled, t.TempDir(), ".js")

	if _, err := catalog.LoadEntry("quick", "../escape.js"); err == nil {
		t.Fatal("LoadEntry() accepted a traversal entry")
	}
}

func TestCatalogRejectsManifestDirectoryMismatch(t *testing.T) {
	userRoot := t.TempDir()
	wrongRoot := filepath.Join(userRoot, "quick")
	if err := os.MkdirAll(wrongRoot, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	manifest := []byte("{\"id\":\"different\",\"name\":\"Quick\",\"version\":\"1.0.0\"}")
	if err := os.WriteFile(filepath.Join(wrongRoot, "manifest.json"), manifest, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	catalog := NewCatalog(fstest.MapFS{}, userRoot, ".js")
	_, err := catalog.Load()
	if err == nil || !strings.Contains(err.Error(), "does not match manifest id") {
		t.Fatalf("Load() error = %v, want directory mismatch", err)
	}
}
