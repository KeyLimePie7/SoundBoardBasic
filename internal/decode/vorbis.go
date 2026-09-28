package decode

import (
	"bytes"
	"fmt"

	"github.com/jfreymuth/oggvorbis"

	"soundboard/internal/audio"
)

// Vorbis decodes an OGG/Vorbis file's bytes into interleaved float32 PCM
// samples.
func Vorbis(data []byte) (audio.PCM, error) {
	samples, format, err := oggvorbis.ReadAll(bytes.NewReader(data))
	if err != nil {
		return audio.PCM{}, fmt.Errorf("decoding ogg/vorbis: %w", err)
	}

	return audio.PCM{
		Samples:    samples,
		SampleRate: format.SampleRate,
		Channels:   format.Channels,
	}, nil
}
