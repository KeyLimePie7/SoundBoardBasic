package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const fileName = "soundboard.json"

// OutputDevice is a persisted audio output selection.
type OutputDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// WindowState persists the main window's last position and size.
type WindowState struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Config is the full set of persisted application settings.
type Config struct {
	KeepOutputsOpen bool           `json:"keep_outputs_open"`
	Outputs         []OutputDevice `json:"outputs"`
	Window          WindowState    `json:"window"`
}

// Default returns sensible settings for a first run.
func Default() Config {
	return Config{
		KeepOutputsOpen: true,
		Window:          WindowState{X: 100, Y: 100, W: 420, H: 520},
	}
}

func path() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), fileName), nil
}

// Load reads the config file beside the executable. A missing file returns
// Default so first runs work out of the box.
func Load() (Config, error) {
	p, err := path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", p, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", p, err)
	}
	return cfg, nil
}

// Save atomically replaces the config file so an interrupted write cannot
// leave a partially written settings file.
func Save(cfg Config) error {
	p, err := path()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp config: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("replacing config file: %w", err)
	}
	return nil
}
