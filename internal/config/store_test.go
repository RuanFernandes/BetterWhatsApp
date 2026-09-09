package config

import (
	"os"
	"path/filepath"
	"testing"

	"betterwhatsapp/internal/model"
)

func TestStorePersistsUpdatesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStoreAt(path)
	if err != nil {
		t.Fatalf("NewStoreAt() error = %v", err)
	}

	updated, err := store.Update(func(settings *model.Settings) error {
		settings.Injector.Enabled = false
		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Injector.Enabled {
		t.Fatal("updated injector state is enabled")
	}

	reloaded, err := NewStoreAt(path)
	if err != nil {
		t.Fatalf("reloading store error = %v", err)
	}
	if reloaded.Snapshot().Injector.Enabled {
		t.Fatal("persisted injector state is enabled")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file was not created: %v", err)
	}
}
