package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"betterwhatsapp/internal/model"
)

type Store struct {
	mu       sync.RWMutex
	path     string
	settings model.Settings
}

func NewStore() (*Store, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user config directory: %w", err)
	}
	return NewStoreAt(filepath.Join(configDirectory, "BetterWhatsApp", "settings.json"))
}

func NewStoreAt(path string) (*Store, error) {
	store := &Store{path: path}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Path() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}

func (s *Store) Snapshot() model.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSettings(s.settings)
}

func (s *Store) Update(update func(*model.Settings) error) (model.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := cloneSettings(s.settings)
	if err := update(&next); err != nil {
		return model.Settings{}, err
	}
	normalized, err := Normalize(next)
	if err != nil {
		return model.Settings{}, err
	}
	if err := writeAtomic(s.path, normalized); err != nil {
		return model.Settings{}, err
	}
	s.settings = normalized
	return cloneSettings(s.settings), nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		defaults := DefaultSettings()
		if err := writeAtomic(s.path, defaults); err != nil {
			return err
		}
		s.settings = defaults
		return nil
	}
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}

	var settings model.Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("decode settings: %w", err)
	}
	needsRewrite := settings.SchemaVersion != model.CurrentSchemaVersion ||
		settings.Profiles == nil ||
		len(settings.ProfileOrder) == 0
	normalized, err := Normalize(settings)
	if err != nil {
		return fmt.Errorf("validate settings: %w", err)
	}
	if needsRewrite {
		if err := writeAtomic(s.path, normalized); err != nil {
			return fmt.Errorf("persist migrated settings: %w", err)
		}
	}
	s.settings = normalized
	return nil
}

func writeAtomic(path string, settings model.Settings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	temporaryFile, err := os.CreateTemp(directory, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary settings file: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	cleanup := func() {
		_ = temporaryFile.Close()
		_ = os.Remove(temporaryPath)
	}

	if err := temporaryFile.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("set settings permissions: %w", err)
	}
	if _, err := temporaryFile.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temporary settings file: %w", err)
	}
	if err := temporaryFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temporary settings file: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("close temporary settings file: %w", err)
	}

	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	}

	backupPath := path + ".bak"
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("prepare settings replacement: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("replace settings: %w", err)
	}
	_ = os.Remove(backupPath)
	return nil
}

func cloneSettings(settings model.Settings) model.Settings {
	cloned := settings
	cloned.Plugins = make(map[string]model.PluginState, len(settings.Plugins))
	for id, state := range settings.Plugins {
		cloned.Plugins[id] = state
	}
	cloned.Themes = make(map[string]model.ThemeState, len(settings.Themes))
	for id, state := range settings.Themes {
		cloned.Themes[id] = state
	}
	cloned.ProfileOrder = append([]string(nil), settings.ProfileOrder...)
	cloned.Profiles = make(map[string]model.Profile, len(settings.Profiles))
	for id, profile := range settings.Profiles {
		profile.PluginOverrides = make(map[string]bool, len(profile.PluginOverrides))
		for pluginID, enabled := range profile.PluginOverrides {
			profile.PluginOverrides[pluginID] = enabled
		}
		cloned.Profiles[id] = profile
	}
	return cloned
}
