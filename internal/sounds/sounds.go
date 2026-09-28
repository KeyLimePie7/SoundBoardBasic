package sounds

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"soundboard/internal/audio"
	"soundboard/internal/decode"
)

var SupportedExtensions = []string{".wav", ".mp3", ".flac", ".ogg", ".oga", ".m4a"}

// Sound is one playable entry found in the sounds folder.
type Sound struct {
	Name string
	Path string

	mu     sync.Mutex
	pcm    audio.PCM
	loaded bool
	err    error
}

// Dir returns the sounds folder beside the running executable, creating it
// if it does not exist yet.
func Dir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating executable path: %w", err)
	}
	dir := filepath.Join(filepath.Dir(exe), "sounds")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating sounds folder: %w", err)
	}
	return dir, nil
}

// Scan lists supported audio files directly inside dir, sorted alphabetically
// by filename, case-insensitively.
func Scan(dir string) ([]*Sound, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading sounds folder: %w", err)
	}

	var found []*Sound
	for _, entry := range entries {
		if entry.IsDir() || !isSupported(entry.Name()) {
			continue
		}
		found = append(found, &Sound{
			Name: entry.Name(),
			Path: filepath.Join(dir, entry.Name()),
		})
	}

	sort.Slice(found, func(i, j int) bool {
		left := strings.ToLower(found[i].Name)
		right := strings.ToLower(found[j].Name)
		if left == right {
			return found[i].Name < found[j].Name
		}
		return left < right
	})
	return found, nil
}

func isSupported(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, supported := range SupportedExtensions {
		if ext == supported {
			return true
		}
	}
	return false
}

// PCM decodes and caches this sound's audio on first call. Decode errors are
// cached until the sound is scanned again.
func (s *Sound) PCM() (audio.PCM, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return s.pcm, s.err
	}
	s.loaded = true

	data, err := os.ReadFile(s.Path)
	if err != nil {
		s.err = fmt.Errorf("reading file: %w", err)
		return audio.PCM{}, s.err
	}

	s.pcm, s.err = decode.File(s.Name, data)
	return s.pcm, s.err
}
