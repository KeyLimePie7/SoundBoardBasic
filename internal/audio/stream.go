package audio

import (
	"runtime"
	"sync"
)

// deviceStream owns one continuously running WASAPI session and mixer on a
// dedicated goroutine, since its COM objects are apartment-threaded.
type deviceStream struct {
	mixer   *mixer
	closeCh chan struct{}
	closed  chan struct{}

	mu        sync.Mutex
	closeOnce sync.Once
	stopping  bool

	sampleRate    int
	channels      int
	bitsPerSample int
}

func newDeviceStream(deviceID string, warm func() bool) (*deviceStream, error) {
	ready := make(chan error, 1)
	stream := &deviceStream{
		mixer:   &mixer{},
		closeCh: make(chan struct{}),
		closed:  make(chan struct{}),
	}

	go stream.run(deviceID, warm, ready)

	if err := <-ready; err != nil {
		return nil, err
	}
	return stream, nil
}

func (s *deviceStream) run(deviceID string, warm func() bool, ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() {
		s.mu.Lock()
		s.stopping = true
		s.mu.Unlock()
		close(s.closed)
	}()

	if revertPriority, err := beginProAudioPriority(); err == nil {
		defer revertPriority()
	}

	session, err := openRenderSession(deviceID)
	if err == nil {
		s.sampleRate = session.SampleRate()
		s.channels = session.Channels()
		s.bitsPerSample = session.BitsPerSample()
	}
	ready <- err
	if err != nil {
		return
	}
	defer session.close()

	shouldStop := func() bool {
		if warm() {
			return false
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		if s.stopping {
			return true
		}
		if s.mixer.idle() {
			s.stopping = true
			return true
		}
		return false
	}

	session.runMixLoop(s.mixer, s.closeCh, shouldStop)
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
}

// play adds a new overlapping voice unless this stream has started shutting
// down. The caller can reopen the stream when false is returned.
func (s *deviceStream) play(samples []float32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return false
	}
	s.mixer.addVoice(samples)
	return true
}

func (s *deviceStream) stopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopping
}

func (s *deviceStream) close() {
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
	s.closeOnce.Do(func() {
		close(s.closeCh)
	})
	<-s.closed
}
