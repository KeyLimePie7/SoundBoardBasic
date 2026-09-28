package decode

import "testing"

func TestDetectFormatRecognizesM4A(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "sound.m4a"},
		{name: "renamed.audio", data: []byte{0, 0, 0, 24, 'f', 't', 'y', 'p'}},
	}

	for _, test := range tests {
		if got := detectFormat(test.name, test.data); got != formatM4A {
			t.Errorf("detectFormat(%q) = %v, want M4A", test.name, got)
		}
	}
}
