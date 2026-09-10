package extensions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"betterwhatsapp/internal/model"
)

const maxExtensionSize = 2 << 20

var validID = regexp.MustCompile("^[a-z0-9][a-z0-9._-]{0,63}$")

type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Entry       string `json:"entry"`
}

type Catalog struct {
	bundledRoot fs.FS
	userRoot    string
	extension   string
}

func NewCatalog(bundledRoot fs.FS, userRoot, extension string) *Catalog {
	return &Catalog{
		bundledRoot: bundledRoot,
		userRoot:    userRoot,
		extension:   extension,
	}
}

func (c *Catalog) Load() ([]Manifest, error) {
	bundled, err := c.loadBundled()
	if err != nil {
		return nil, err
	}
	user, err := c.loadUser()
	if err != nil {
		return nil, err
	}

	byID := make(map[string]Manifest, len(bundled)+len(user))
	for _, manifest := range bundled {
		byID[manifest.ID] = manifest
	}
	for _, manifest := range user {
		byID[manifest.ID] = manifest
	}

	result := make([]Manifest, 0, len(byID))
	for _, manifest := range byID {
		result = append(result, manifest)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (c *Catalog) LoadEntry(id string, entry string) ([]byte, error) {
	if !validID.MatchString(id) {
		return nil, errors.New("invalid extension id")
	}
	if entry == "" {
		entry = "index" + c.extension
	}
	if !validEntry(entry, c.extension) {
		return nil, errors.New("invalid extension entry")
	}

	if c.userRoot != "" {
		root, err := safeJoin(c.userRoot, id)
		if err != nil {
			return nil, err
		}
		manifestPath := filepath.Join(root, "manifest.json")
		if _, err := os.Stat(manifestPath); err == nil {
			filePath, err := safeJoin(root, filepath.FromSlash(entry))
			if err != nil {
				return nil, err
			}
			data, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("read user extension %q: %w", id, err)
			}
			return limitSize(data)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect user extension %q: %w", id, err)
		}
	}

	if c.bundledRoot != nil {
		bundledPath := filepath.ToSlash(filepath.Join(id, entry))
		if data, err := fs.ReadFile(c.bundledRoot, bundledPath); err == nil {
			return limitSize(data)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("read bundled extension %q: %w", id, err)
		}
	}
	return nil, fmt.Errorf("extension %q entry %q was not found", id, entry)
}

func (c *Catalog) IsUser(id string) (bool, error) {
	if !validID.MatchString(id) {
		return false, errors.New("invalid extension id")
	}
	if c.userRoot == "" {
		return false, nil
	}
	root, err := safeJoin(c.userRoot, id)
	if err != nil {
		return false, err
	}
	manifestPath := filepath.Join(root, "manifest.json")
	info, err := os.Stat(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect user extension %q: %w", id, err)
	}
	return !info.IsDir(), nil
}

func (c *Catalog) UserPath(id string) (string, error) {
	if !validID.MatchString(id) {
		return "", errors.New("invalid extension id")
	}
	if c.userRoot == "" {
		return "", errors.New("user extension directory is not configured")
	}
	root, err := safeJoin(c.userRoot, id)
	if err != nil {
		return "", err
	}
	isUser, err := c.IsUser(id)
	if err != nil {
		return "", err
	}
	if !isUser {
		return "", fmt.Errorf("user extension %q was not found", id)
	}
	return root, nil
}

func (c *Catalog) CreateUser(manifest Manifest, source []byte) error {
	normalized, err := normalizeManifest(manifest, c.extension)
	if err != nil {
		return err
	}
	if _, err := limitSize(source); err != nil {
		return err
	}
	if c.userRoot == "" {
		return errors.New("user extension directory is not configured")
	}

	root, err := safeJoin(c.userRoot, normalized.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(root); err == nil {
		return fmt.Errorf("user extension %q already exists", normalized.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect user extension %q: %w", normalized.ID, err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create user extension %q: %w", normalized.ID, err)
	}

	entryPath, err := safeJoin(root, filepath.FromSlash(normalized.Entry))
	if err != nil {
		_ = os.RemoveAll(root)
		return err
	}
	if err := writeUserFileAtomic(entryPath, source, 0o600); err != nil {
		_ = os.RemoveAll(root)
		return fmt.Errorf("write user extension %q entry: %w", normalized.ID, err)
	}
	if err := writeUserManifest(root, normalized); err != nil {
		_ = os.RemoveAll(root)
		return err
	}
	return nil
}

func (c *Catalog) UpdateUser(manifest Manifest, source []byte) error {
	normalized, err := normalizeManifest(manifest, c.extension)
	if err != nil {
		return err
	}
	if _, err := limitSize(source); err != nil {
		return err
	}
	if c.userRoot == "" {
		return errors.New("user extension directory is not configured")
	}

	root, err := safeJoin(c.userRoot, normalized.ID)
	if err != nil {
		return err
	}
	isUser, err := c.IsUser(normalized.ID)
	if err != nil {
		return err
	}
	if !isUser {
		return fmt.Errorf("user extension %q was not found", normalized.ID)
	}

	entryPath, err := safeJoin(root, filepath.FromSlash(normalized.Entry))
	if err != nil {
		return err
	}
	if err := writeUserFileAtomic(entryPath, source, 0o600); err != nil {
		return fmt.Errorf("write user extension %q entry: %w", normalized.ID, err)
	}
	return writeUserManifest(root, normalized)
}

func (c *Catalog) DeleteUser(id string) error {
	if !validID.MatchString(id) {
		return errors.New("invalid extension id")
	}
	if c.userRoot == "" {
		return errors.New("user extension directory is not configured")
	}
	isUser, err := c.IsUser(id)
	if err != nil {
		return err
	}
	if !isUser {
		return fmt.Errorf("user extension %q was not found", id)
	}
	root, err := safeJoin(c.userRoot, id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(root); err != nil {
		return fmt.Errorf("delete user extension %q: %w", id, err)
	}
	return nil
}

func (c *Catalog) loadBundled() ([]Manifest, error) {
	if c.bundledRoot == nil {
		return nil, nil
	}
	entries, err := fs.ReadDir(c.bundledRoot, ".")
	if err != nil {
		return nil, fmt.Errorf("list bundled extensions: %w", err)
	}
	manifests := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validID.MatchString(entry.Name()) {
			continue
		}
		manifestPath := filepath.ToSlash(filepath.Join(entry.Name(), "manifest.json"))
		manifest, err := c.readManifest(func(filePath string) ([]byte, error) {
			return fs.ReadFile(c.bundledRoot, filePath)
		}, manifestPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if manifest.ID != entry.Name() {
			return nil, fmt.Errorf("bundled extension directory %q does not match manifest id %q", entry.Name(), manifest.ID)
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func (c *Catalog) loadUser() ([]Manifest, error) {
	if c.userRoot == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(c.userRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list user extensions: %w", err)
	}

	manifests := make([]Manifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validID.MatchString(entry.Name()) {
			continue
		}
		root, err := safeJoin(c.userRoot, entry.Name())
		if err != nil {
			return nil, err
		}
		manifest, err := c.readManifest(func(filePath string) ([]byte, error) {
			return os.ReadFile(filePath)
		}, filepath.Join(root, "manifest.json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if manifest.ID != entry.Name() {
			return nil, fmt.Errorf("user extension directory %q does not match manifest id %q", entry.Name(), manifest.ID)
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func (c *Catalog) readManifest(read func(string) ([]byte, error), location string) (Manifest, error) {
	data, err := read(location)
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxExtensionSize {
		return Manifest{}, errors.New("extension manifest is too large")
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode extension manifest: %w", err)
	}
	return normalizeManifest(manifest, c.extension)
}

func normalizeManifest(manifest Manifest, extension string) (Manifest, error) {
	manifest.ID = strings.TrimSpace(manifest.ID)
	manifest.Name = strings.TrimSpace(manifest.Name)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.Entry = strings.TrimSpace(manifest.Entry)
	if !validID.MatchString(manifest.ID) {
		return Manifest{}, errors.New("extension manifest has an invalid id")
	}
	if manifest.Name == "" || manifest.Version == "" {
		return Manifest{}, errors.New("extension manifest requires name and version")
	}
	if manifest.Entry == "" {
		manifest.Entry = "index" + extension
	}
	if !validEntry(manifest.Entry, extension) {
		return Manifest{}, errors.New("extension manifest has an invalid entry")
	}
	return manifest, nil
}

func writeUserManifest(root string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode user extension manifest: %w", err)
	}
	data = append(data, '\n')
	if err := writeUserFileAtomic(filepath.Join(root, "manifest.json"), data, 0o600); err != nil {
		return fmt.Errorf("write user extension manifest: %w", err)
	}
	return nil
}

func writeUserFileAtomic(path string, data []byte, mode fs.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporaryFile, err := os.CreateTemp(directory, ".extension-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporaryFile.Name()
	cleanup := func() {
		_ = temporaryFile.Close()
		_ = os.Remove(temporaryPath)
	}

	if err := temporaryFile.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := temporaryFile.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temporaryFile.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temporaryFile.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}

	if err := os.Rename(temporaryPath, path); err == nil {
		return nil
	}

	backupPath := path + ".bak"
	_ = os.Remove(backupPath)
	if err := os.Rename(path, backupPath); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		_ = os.Remove(temporaryPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func validEntry(entry, extension string) bool {
	normalized := filepath.ToSlash(entry)
	if extension != ".js" && extension != ".css" {
		return false
	}
	if normalized != entry || path.IsAbs(normalized) || path.Clean(normalized) != normalized {
		return false
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") || strings.Contains(normalized, "/../") {
		return false
	}
	return filepath.Ext(entry) == extension
}

func safeJoin(root, child string) (string, error) {
	if root == "" {
		return "", errors.New("extension directory is not configured")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve extension directory: %w", err)
	}
	childAbs, err := filepath.Abs(filepath.Join(rootAbs, child))
	if err != nil {
		return "", fmt.Errorf("resolve extension path: %w", err)
	}
	if !within(rootAbs, childAbs) {
		return "", errors.New("extension path escapes its directory")
	}

	realRoot, err := filepath.EvalSymlinks(rootAbs)
	if errors.Is(err, os.ErrNotExist) {
		return childAbs, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve extension directory links: %w", err)
	}

	realChild, err := filepath.EvalSymlinks(childAbs)
	if err == nil {
		if !within(realRoot, realChild) {
			return "", errors.New("extension path escapes its directory through a symlink")
		}
		return childAbs, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve extension path links: %w", err)
	}

	realParent, err := filepath.EvalSymlinks(filepath.Dir(childAbs))
	if errors.Is(err, os.ErrNotExist) {
		return childAbs, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve extension parent links: %w", err)
	}
	if !within(realRoot, realParent) {
		return "", errors.New("extension path escapes its directory through a symlink")
	}
	return childAbs, nil
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func limitSize(data []byte) ([]byte, error) {
	if len(data) > maxExtensionSize {
		return nil, errors.New("extension entry is too large")
	}
	return data, nil
}

func ToPluginInfo(manifest Manifest, state model.PluginState) model.PluginInfo {
	return model.PluginInfo{
		ID:            manifest.ID,
		Name:          manifest.Name,
		Version:       manifest.Version,
		Description:   manifest.Description,
		Author:        manifest.Author,
		Enabled:       state.Enabled,
		GlobalEnabled: state.Enabled,
		Entry:         manifest.Entry,
	}
}

func ToThemeInfo(manifest Manifest, state model.ThemeState, active bool) model.ThemeInfo {
	return model.ThemeInfo{
		ID:          manifest.ID,
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Author:      manifest.Author,
		Enabled:     state.Enabled,
		Active:      active,
		Entry:       manifest.Entry,
	}
}
