package decode

import (
	"fmt"
	"path/filepath"
	"strings"

	"soundboard/internal/audio"
)

// File decodes audio bytes into interleaved float32 PCM. name is used for
// error messages and as a fallback format hint.
func File(name string, data []byte) (audio.PCM, error) {
	switch detectFormat(name, data) {
	case formatWAV:
		return WAV(data)
	case formatMP3:
		return MP3(data)
	case formatFLAC:
		return FLAC(data)
	case formatOgg:
		return Vorbis(data)
	case formatM4A:
		return M4A(data)
	default:
		return audio.PCM{}, fmt.Errorf("unsupported or unrecognized audio format: %s", filepath.Base(name))
	}
}

type format int

const (
	formatUnknown format = iota
	formatWAV
	formatMP3
	formatFLAC
	formatOgg
	formatM4A
)

func detectFormat(name string, data []byte) format {
	switch {
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		return formatWAV
	case len(data) >= 8 && string(data[4:8]) == "ftyp":
		return formatM4A
	case len(data) >= 4 && string(data[0:4]) == "fLaC":
		return formatFLAC
	case len(data) >= 4 && string(data[0:4]) == "OggS":
		return formatOgg
	case len(data) >= 3 && string(data[0:3]) == "ID3":
		return formatMP3
	case len(data) >= 2 && data[0] == 0xFF && (data[1]&0xE0) == 0xE0:
		return formatMP3
	}

	switch strings.ToLower(filepath.Ext(name)) {
	case ".wav":
		return formatWAV
	case ".mp3":
		return formatMP3
	case ".flac":
		return formatFLAC
	case ".ogg", ".oga":
		return formatOgg
	case ".m4a":
		return formatM4A
	}
	return formatUnknown
}
