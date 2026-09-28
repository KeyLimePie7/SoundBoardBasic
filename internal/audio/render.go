package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

const eventWaitTimeoutMs = 200

// renderSession owns an open WASAPI shared-mode render stream for one
// device. Its methods must run on the goroutine that created it.
type renderSession struct {
	enumerator   *wca.IMMDeviceEnumerator
	device       *wca.IMMDevice
	client       *wca.IAudioClient
	renderClient *wca.IAudioRenderClient
	format       *wca.WAVEFORMATEX
	bufferFrames uint32
	event        uintptr
}

func (s *renderSession) SampleRate() int    { return int(s.format.NSamplesPerSec) }
func (s *renderSession) Channels() int      { return int(s.format.NChannels) }
func (s *renderSession) BitsPerSample() int { return int(s.format.WBitsPerSample) }

// openRenderSession initializes COM on the current thread, locates the
// device by ID, and starts an event-driven shared-mode render stream using
// the device's own mix format.
func openRenderSession(deviceID string) (session *renderSession, err error) {
	if err = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return nil, fmt.Errorf("initializing COM: %w", err)
	}
	defer func() {
		if err != nil {
			ole.CoUninitialize()
		}
	}()

	enumerator, err := newDeviceEnumerator()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			enumerator.Release()
		}
	}()

	device, err := findDeviceByID(enumerator, deviceID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			device.Release()
		}
	}()

	client, format, bufferFrames, err := activateClient(device)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			client.Release()
		}
	}()
	defer func() {
		if err != nil && format != nil {
			ole.CoTaskMemFree(uintptr(unsafe.Pointer(format)))
		}
	}()

	event := wca.CreateEventExA(0, 0, 0, wca.EVENT_ALL_ACCESS)
	if event == 0 {
		err = fmt.Errorf("creating render event")
		return nil, err
	}
	defer func() {
		if err != nil {
			wca.CloseHandle(event)
		}
	}()

	if err = client.SetEventHandle(event); err != nil {
		return nil, fmt.Errorf("setting render event: %w", err)
	}

	var renderClient *wca.IAudioRenderClient
	if err = client.GetService(wca.IID_IAudioRenderClient, &renderClient); err != nil {
		return nil, fmt.Errorf("getting render client: %w", err)
	}
	defer func() {
		if err != nil {
			renderClient.Release()
		}
	}()

	if err = client.Start(); err != nil {
		return nil, fmt.Errorf("starting render client: %w", err)
	}

	return &renderSession{
		enumerator:   enumerator,
		device:       device,
		client:       client,
		renderClient: renderClient,
		format:       format,
		bufferFrames: bufferFrames,
		event:        event,
	}, nil
}

// activateClient initializes an event-driven shared-mode stream using the
// device's own mix format.
func activateClient(device *wca.IMMDevice) (client *wca.IAudioClient, format *wca.WAVEFORMATEX, bufferFrames uint32, err error) {
	if err = device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &client); err != nil {
		return nil, nil, 0, fmt.Errorf("activating audio client: %w", err)
	}

	if err = client.GetMixFormat(&format); err != nil {
		client.Release()
		if format != nil {
			ole.CoTaskMemFree(uintptr(unsafe.Pointer(format)))
		}
		return nil, nil, 0, fmt.Errorf("getting mix format: %w", err)
	}

	freeFormat := func() {
		if format != nil {
			ole.CoTaskMemFree(uintptr(unsafe.Pointer(format)))
			format = nil
		}
	}

	var defaultPeriod wca.REFERENCE_TIME
	if err = client.GetDevicePeriod(&defaultPeriod, nil); err != nil {
		client.Release()
		freeFormat()
		return nil, nil, 0, fmt.Errorf("getting device period: %w", err)
	}

	if err = client.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_EVENTCALLBACK, defaultPeriod, 0, format, nil); err != nil {
		client.Release()
		freeFormat()
		return nil, nil, 0, fmt.Errorf("initializing audio client: %w", err)
	}

	if err = client.GetBufferSize(&bufferFrames); err != nil {
		client.Release()
		freeFormat()
		return nil, nil, 0, fmt.Errorf("getting buffer size: %w", err)
	}

	return client, format, bufferFrames, nil
}

// runMixLoop continuously feeds m's mixed output into the device until
// closeCh is closed or shouldStop returns true after an idle mix.
func (s *renderSession) runMixLoop(m *mixer, closeCh <-chan struct{}, shouldStop func() bool) {
	channels := s.Channels()
	scratch := make([]float32, 0, int(s.bufferFrames)*channels)

	for {
		select {
		case <-closeCh:
			return
		default:
		}

		if result := wca.WaitForSingleObject(s.event, eventWaitTimeoutMs); result != 0 {
			continue
		}

		var padding uint32
		if err := s.client.GetCurrentPadding(&padding); err != nil {
			return
		}
		available := int(s.bufferFrames - padding)
		if available <= 0 {
			continue
		}

		needed := available * channels
		if cap(scratch) < needed {
			scratch = make([]float32, needed)
		}
		scratch = scratch[:needed]
		m.fill(scratch)

		frameBytes := renderFrameBytes(PCM{Samples: scratch, Channels: channels}, s.format.WBitsPerSample)

		var buffer *byte
		if err := s.renderClient.GetBuffer(uint32(available), &buffer); err != nil {
			return
		}
		dst := unsafe.Slice(buffer, len(frameBytes))
		copy(dst, frameBytes)
		if err := s.renderClient.ReleaseBuffer(uint32(available), 0); err != nil {
			return
		}

		if shouldStop != nil && shouldStop() {
			wca.WaitForSingleObject(s.event, eventWaitTimeoutMs)
			return
		}
	}
}

func (s *renderSession) close() {
	_ = s.client.Stop()
	s.renderClient.Release()
	s.client.Release()
	s.device.Release()
	s.enumerator.Release()
	wca.CloseHandle(s.event)
	if s.format != nil {
		ole.CoTaskMemFree(uintptr(unsafe.Pointer(s.format)))
	}
	ole.CoUninitialize()
}

// renderFrameBytes packs interleaved float32 samples into the raw byte layout
// WASAPI expects for the given bit depth.
func renderFrameBytes(pcm PCM, bitsPerSample uint16) []byte {
	if bitsPerSample == 32 {
		out := make([]byte, len(pcm.Samples)*4)
		for i, sample := range pcm.Samples {
			binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(sample))
		}
		return out
	}

	out := make([]byte, len(pcm.Samples)*2)
	for i, sample := range pcm.Samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(int16(clamp(sample, -1, 1)*32767)))
	}
	return out
}

func clamp(value, minimum, maximum float32) float32 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
