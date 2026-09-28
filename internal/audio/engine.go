package audio

import "sync"

// Engine manages one continuously running mixer stream per output device.
type Engine struct {
	mu      sync.Mutex
	streams map[string]*deviceStream
	warm    bool
	closed  bool

	OnError func(deviceID string, err error)
}

func NewEngine(warm bool) *Engine {
	return &Engine{streams: make(map[string]*deviceStream), warm: warm}
}

func (e *Engine) SetWarm(warm bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.warm = warm
}

func (e *Engine) isWarm() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.warm
}

// Play resamples pcm to each listed device's native format and starts it
// playing there, overlapping with anything already playing on that device.
func (e *Engine) Play(deviceIDs []string, pcm PCM) {
	for _, id := range deviceIDs {
		go e.playOne(id, pcm)
	}
}

func (e *Engine) playOne(deviceID string, pcm PCM) {
	for {
		stream, err := e.getOrOpenStream(deviceID)
		if err != nil {
			e.reportError(deviceID, err)
			return
		}
		if stream == nil {
			return
		}

		converted := Resample(pcm, stream.sampleRate, stream.channels)
		if stream.play(converted.Samples) {
			return
		}
	}
}

func (e *Engine) getOrOpenStream(deviceID string) (*deviceStream, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil, nil
	}
	if stream := e.streams[deviceID]; stream != nil && !stream.stopped() {
		return stream, nil
	}

	stream, err := newDeviceStream(deviceID, e.isWarm)
	if err != nil {
		return nil, err
	}
	e.streams[deviceID] = stream
	return stream, nil
}

// StopAll immediately silences every currently open device stream.
func (e *Engine) StopAll() {
	e.mu.Lock()
	streams := make([]*deviceStream, 0, len(e.streams))
	for _, stream := range e.streams {
		streams = append(streams, stream)
	}
	e.mu.Unlock()

	for _, stream := range streams {
		stream.mixer.stopAll()
	}
}

func (e *Engine) reportError(deviceID string, err error) {
	if e.OnError != nil {
		e.OnError(deviceID, err)
	}
}

// Close releases every open device stream. Call this once on application
// shutdown; repeated calls are harmless.
func (e *Engine) Close() {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	streams := make([]*deviceStream, 0, len(e.streams))
	for _, stream := range e.streams {
		streams = append(streams, stream)
	}
	e.streams = make(map[string]*deviceStream)
	e.mu.Unlock()

	for _, stream := range streams {
		stream.close()
	}
}
