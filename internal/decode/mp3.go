package decode

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/hajimehoshi/go-mp3"

	"soundboard/internal/audio"
)

// MP3 decodes an MP3 file's bytes into interleaved float32 PCM samples.
func MP3(data []byte) (audio.PCM, error) {
	decoder, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return audio.PCM{}, fmt.Errorf("decoding mp3: %w", err)
	}

	raw, err := io.ReadAll(decoder)
	if err != nil {
		return audio.PCM{}, fmt.Errorf("reading mp3 samples: %w", err)
	}

	const channels = 2
	samples := make([]float32, len(raw)/2)
	for i := range samples {
		value := int16(binary.LittleEndian.Uint16(raw[i*2:]))
		samples[i] = float32(value) / 32768
	}

	return audio.PCM{
		Samples:    samples,
		SampleRate: decoder.SampleRate(),
		Channels:   channels,
	}, nil
}
