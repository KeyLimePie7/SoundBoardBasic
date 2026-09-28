package audio

import (
	"slices"
	"testing"
	"time"
)

func TestMixerMixesOverlappingVoicesAndClips(t *testing.T) {
	var m mixer
	m.addVoice([]float32{0.75, 0.25, -0.5})
	m.addVoice([]float32{0.5, -0.75, -0.75, 0.4})

	got := make([]float32, 5)
	m.fill(got)
	want := []float32{1, -0.5, -1, 0.4, 0}
	if !slices.Equal(got, want) {
		t.Fatalf("mixed samples = %v, want %v", got, want)
	}
	if !m.idle() {
		t.Fatal("mixer should be idle after all voices finish")
	}
}

func TestMixerStopAllClearsVoices(t *testing.T) {
	var m mixer
	m.addVoice([]float32{0.25, -0.25})
	m.stopAll()

	got := make([]float32, 2)
	m.fill(got)
	want := []float32{0, 0}
	if !slices.Equal(got, want) {
		t.Fatalf("samples after stop = %v, want %v", got, want)
	}
	if !m.idle() {
		t.Fatal("mixer should be idle after Stop All")
	}
}

func TestEngineCloseDoesNotHoldLockWhileClosingStreams(t *testing.T) {
	engine := NewEngine(false)
	stream := &deviceStream{
		mixer:   &mixer{},
		closeCh: make(chan struct{}),
		closed:  make(chan struct{}),
	}
	engine.streams["device"] = stream

	go func() {
		<-stream.closeCh
		engine.isWarm()
		close(stream.closed)
	}()

	done := make(chan struct{})
	go func() {
		engine.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Engine.Close deadlocked while waiting for a stream")
	}
}
