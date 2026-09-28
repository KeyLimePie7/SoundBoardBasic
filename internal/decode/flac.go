package decode

import (
	"bytes"
	"fmt"
	"io"

	"github.com/mewkiz/flac"

	"soundboard/internal/audio"
)

// FLAC decodes a FLAC file's bytes into interleaved float32 PCM samples.
func FLAC(data []byte) (audio.PCM, error) {
	stream, err := flac.New(bytes.NewReader(data))
	if err != nil {
		return audio.PCM{}, fmt.Errorf("opening flac stream: %w", err)
	}
	defer stream.Close()

	channels := int(stream.Info.NChannels)
	bitsPerSample := int(stream.Info.BitsPerSample)
	if channels == 0 || bitsPerSample < 1 || bitsPerSample > 32 {
		return audio.PCM{}, fmt.Errorf("flac file missing valid format information")
	}
	scale := float32(uint64(1) << uint(bitsPerSample-1))

	var samples []float32
	for {
		frame, err := stream.ParseNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return audio.PCM{}, fmt.Errorf("decoding flac frame: %w", err)
		}
		if len(frame.Subframes) != channels {
			return audio.PCM{}, fmt.Errorf("decoding flac frame: channel count mismatch")
		}

		frameSamples := len(frame.Subframes[0].Samples)
		for _, subframe := range frame.Subframes {
			if len(subframe.Samples) != frameSamples {
				return audio.PCM{}, fmt.Errorf("decoding flac frame: sample count mismatch")
			}
		}
		for i := 0; i < frameSamples; i++ {
			for channel := 0; channel < channels; channel++ {
				samples = append(samples, float32(frame.Subframes[channel].Samples[i])/scale)
			}
		}
	}

	return audio.PCM{
		Samples:    samples,
		SampleRate: int(stream.Info.SampleRate),
		Channels:   channels,
	}, nil
}
