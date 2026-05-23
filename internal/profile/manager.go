package profile

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Manager handles lifecycle of browser profiles on disk.
type Manager struct {
	BasePath string // root directory that contains all profile subdirectories
}

// NewManager returns a Manager rooted at basePath.
func NewManager(basePath string) *Manager {
	return &Manager{BasePath: basePath}
}

// ProfileDir returns the directory path for a given profile ID.
func (m *Manager) ProfileDir(id string) string {
	return filepath.Join(m.BasePath, id)
}

// configPath returns the path to config.json inside a profile directory.
func (m *Manager) configPath(id string) string {
	return filepath.Join(m.ProfileDir(id), "config.json")
}

// LoadAll reads all profile subdirectories and returns their configurations.
func (m *Manager) LoadAll() ([]*Config, error) {
	entries, err := os.ReadDir(m.BasePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("profile: list base dir: %w", err)
	}

	var configs []*Config
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cfg, err := m.Get(entry.Name())
		if err != nil {
			// Skip unreadable/invalid profiles rather than aborting.
			continue
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// Get loads and returns a single profile by ID.
func (m *Manager) Get(id string) (*Config, error) {
	data, err := os.ReadFile(m.configPath(id))
	if err != nil {
		return nil, fmt.Errorf("profile: read config for %q: %w", id, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("profile: parse config for %q: %w", id, err)
	}
	return &cfg, nil
}

// Create creates a new profile with the given name, persists it, and returns it.
func (m *Manager) Create(name string) (*Config, error) {
	cfg, err := NewConfig(name)
	if err != nil {
		return nil, fmt.Errorf("profile: new config: %w", err)
	}

	if err := os.MkdirAll(m.ProfileDir(cfg.ID), 0o755); err != nil {
		return nil, fmt.Errorf("profile: mkdir for %q: %w", cfg.ID, err)
	}

	if err := m.Save(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes cfg to its config.json on disk (atomic via temp file).
func (m *Manager) Save(cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("profile: marshal config %q: %w", cfg.ID, err)
	}

	dest := m.configPath(cfg.ID)
	tmp := dest + ".tmp"

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("profile: write temp config %q: %w", cfg.ID, err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("profile: rename config %q: %w", cfg.ID, err)
	}
	return nil
}

// Delete removes a profile and all its data from disk.
func (m *Manager) Delete(id string) error {
	dir := m.ProfileDir(id)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("profile: %q not found", id)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("profile: delete %q: %w", id, err)
	}
	return nil
}

// Export zips the profile directory into destZipPath.
func (m *Manager) Export(id, destZipPath string) error {
	profileDir := m.ProfileDir(id)

	if _, err := os.Stat(profileDir); os.IsNotExist(err) {
		return fmt.Errorf("profile: export %q: profile not found", id)
	}

	zipFile, err := os.Create(destZipPath)
	if err != nil {
		return fmt.Errorf("profile: export create zip: %w", err)
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	err = filepath.WalkDir(profileDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Compute the relative path inside the zip (rooted at the profile ID dir).
		rel, err := filepath.Rel(profileDir, path)
		if err != nil {
			return err
		}

		// Use forward slashes in zip entries.
		zipEntry := filepath.ToSlash(filepath.Join(id, rel))

		if d.IsDir() {
			// Add directory entry.
			if _, err := w.Create(zipEntry + "/"); err != nil {
				return err
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = zipEntry
		header.Method = zip.Deflate

		writer, err := w.CreateHeader(header)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(writer, f)
		return err
	})

	if err != nil {
		return fmt.Errorf("profile: export walk %q: %w", id, err)
	}
	return nil
}

// Import unzips a profile zip into BasePath. If the profile ID already exists
// the imported profile is stored under id+"_imported".
func (m *Manager) Import(zipPath string) (*Config, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("profile: import open zip: %w", err)
	}
	defer r.Close()

	// Extract to a temp directory first.
	tmpDir, err := os.MkdirTemp("", "alterego_import_*")
	if err != nil {
		return nil, fmt.Errorf("profile: import mktemp: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, f := range r.File {
		// Guard against zip-slip.
		target := filepath.Join(tmpDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(target, filepath.Clean(tmpDir)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("profile: import zip-slip detected for %q", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, fmt.Errorf("profile: import mkdir: %w", err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, fmt.Errorf("profile: import mkdir parent: %w", err)
		}

		if err := extractZipFile(f, target); err != nil {
			return nil, fmt.Errorf("profile: import extract %q: %w", f.Name, err)
		}
	}

	// Find the first directory inside tmpDir — that is the profile directory.
	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) == 0 {
		return nil, fmt.Errorf("profile: import: no top-level directory found in zip")
	}

	var profileTmpDir string
	for _, e := range entries {
		if e.IsDir() {
			profileTmpDir = filepath.Join(tmpDir, e.Name())
			break
		}
	}
	if profileTmpDir == "" {
		return nil, fmt.Errorf("profile: import: no profile directory in zip")
	}

	// Read config.json from the extracted temp directory.
	cfgData, err := os.ReadFile(filepath.Join(profileTmpDir, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("profile: import read config.json: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return nil, fmt.Errorf("profile: import parse config.json: %w", err)
	}

	destID := cfg.ID
	destDir := m.ProfileDir(destID)

	// If a profile with this ID already exists, use an "_imported" suffix.
	if _, err := os.Stat(destDir); err == nil {
		destID = cfg.ID + "_imported"
		cfg.ID = destID
		destDir = m.ProfileDir(destID)
	}

	// Move the extracted profile into BasePath/id.
	if err := os.MkdirAll(m.BasePath, 0o755); err != nil {
		return nil, fmt.Errorf("profile: import mkdir base: %w", err)
	}

	if err := os.Rename(profileTmpDir, destDir); err != nil {
		// Rename can fail across volumes — fall back to copy+remove.
		if copyErr := copyDir(profileTmpDir, destDir); copyErr != nil {
			return nil, fmt.Errorf("profile: import copy dir: %w", copyErr)
		}
	}

	// Persist the (possibly updated) config.
	if err := m.Save(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// extractZipFile extracts a single zip.File to dest path.
func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}

// copyDir recursively copies src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		return copyFile(path, target)
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
