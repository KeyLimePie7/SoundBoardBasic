package sounds

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanListsSupportedFilesAlphabetically(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Beta.MP3", "alpha.wav", "Charlie.m4a", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "nested.wav"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("Scan returned %d sounds, want 3", len(got))
	}
	if got[0].Name != "alpha.wav" || got[1].Name != "Beta.MP3" || got[2].Name != "Charlie.m4a" {
		t.Fatalf("sound order = [%s, %s, %s], want [alpha.wav, Beta.MP3, Charlie.m4a]", got[0].Name, got[1].Name, got[2].Name)
	}
}
