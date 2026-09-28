package audio

import "sync"

// voice is a single active playback instance being mixed into a device's
// output. samples must already be resampled/remapped to the device's
// native sample rate and channel count before being added.
type voice struct {
	samples []float32
	pos     int
}

// mixer combines any number of concurrently active voices into one
// continuous stream for a single output device.
type mixer struct {
	mu         sync.Mutex
	voices     []*voice
	everPlayed bool
}

// addVoice starts mixing samples into the output, overlapping with anything
// already playing.
func (m *mixer) addVoice(samples []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.voices = append(m.voices, &voice{samples: samples})
	m.everPlayed = true
}

// stopAll immediately silences every currently active voice.
func (m *mixer) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.voices = m.voices[:0]
}

// fill writes exactly len(dst) mixed samples into dst, advancing and removing
// finished voices, summing overlapping voices, and clipping to [-1, 1].
func (m *mixer) fill(dst []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range dst {
		dst[i] = 0
	}

	active := m.voices[:0]
	for _, v := range m.voices {
		remaining := len(v.samples) - v.pos
		n := len(dst)
		if n > remaining {
			n = remaining
		}
		for i := 0; i < n; i++ {
			dst[i] += v.samples[v.pos+i]
		}
		v.pos += n
		if v.pos < len(v.samples) {
			active = append(active, v)
		}
	}
	m.voices = active

	for i, sample := range dst {
		if sample > 1 {
			dst[i] = 1
		} else if sample < -1 {
			dst[i] = -1
		}
	}
}

// idle reports whether nothing is playing but something has played before.
func (m *mixer) idle() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.everPlayed && len(m.voices) == 0
}
