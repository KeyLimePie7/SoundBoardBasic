# SoundBoardBasic — Implementation Plan

**Audience for this document:** an AI coding agent implementing this from scratch.
Follow it literally and in order. Where exact code is given, use it as-is unless it
fails to compile — the APIs referenced here have been verified against real package
documentation, not guessed. Where a step says "verify" or "test", actually run the
build/test before moving to the next phase.

## 0. What this app is

A portable Windows GUI soundboard: it scans a `sounds` folder beside its own
executable, lists every supported audio file with a Play button, lets multiple
sounds overlap when played, and routes playback to one or more selected Windows
audio output devices simultaneously (e.g. physical speakers + a virtual cable like
VB-Cable/Voicemeeter). It reuses the GUI toolkit, WASAPI output engine design, and
portable-config conventions from a sibling project, **VoiceboxExtensions**
(`d:\Workspace\VoiceboxExtensions`), which you should treat as a reference
implementation for anything not fully specified here — but do not import its
`internal` packages directly (Go forbids that across modules); this is a separate
module with its own copies of the shared low-level pieces.

## 1. Confirmed requirements (do not deviate without asking)

- Go, Walk GUI (`github.com/lxn/walk`), pure-Go WASAPI audio (no cgo).
- On launch: look beside the `.exe` for a folder named `sounds`; create it if
  missing.
- Scan that folder for supported audio files, list them alphabetically, one row
  per file: **filename** + a **Play** button.
- A **Refresh Sounds** button above the list re-scans the folder without
  restarting the app (confirmed necessary — no file-watcher, manual refresh only).
- Multiple sounds may play **simultaneously/overlapping** (this is the core
  soundboard requirement and drives the architecture — see §3).
- A **Stop All** button immediately silences everything currently playing.
- Below the sound list: an **Audio Outputs** section — one checkbox per active
  Windows playback device, plus a **"Keep outputs open (lower latency)"**
  checkbox, exactly mirroring VoiceboxExtensions' UI pattern.
- A tooltip near the top of the Audio Outputs section listing every supported
  audio format on hover.
- Supported formats for v1: **WAV, MP3, FLAC, OGG/Vorbis, and M4A/AAC**.
  M4A/AAC decoding uses the Windows Media Foundation Source Reader and the
  system AAC decoder, with no cgo or external codec binary.
- Settings persist to a portable `soundboard.json` beside the executable
  (selected output devices, keep-open flag, window bounds) — same pattern as
  VoiceboxExtensions' `vbedb.json`.

## 2. Two hard-won lessons from VoiceboxExtensions — apply both from the start

These are not optional polish; skipping either one will produce a broken or
crashing app, because they already happened once during that project's
development.

### 2.1 The GUI will crash on startup without a manifest

`lxn/walk` fails with `TTM_ADDTOOL failed` on window creation unless the `.exe`
has a Windows manifest declaring Common Controls v6. Fix, done once:

1. Create `app.manifest` (exact content in §5.1).
2. Install the pure-Go resource compiler: `go install github.com/akavel/rsrc@latest`
   (no C compiler involved).
3. Generate the embedded resource: `rsrc -manifest app.manifest -o rsrc_windows_amd64.syso`
   run from the project root. Go automatically links any `*.syso` file found in
   the package directory.
4. Build with `.\build.ps1` (or `go build -ldflags "-H windowsgui" -o soundboard.exe .`).
   The `windowsgui` subsystem prevents Explorer from opening a console window.
5. Re-run step 3 only if `app.manifest` ever changes.

### 2.2 Watch for a duplicated `package` line after creating new files

During VoiceboxExtensions' development, editor tooling occasionally inserted an
extra `package X` line at the very top of freshly created `.go` files that
already started with a doc comment + `package X`, producing a syntax error
(`non-declaration statement outside function body`). **After creating each new
`.go` file, open it and confirm there is exactly one `package` line, at the very
top.** If there are two, delete the extra one.

## 3. Architecture — why this can't just copy VoiceboxExtensions' engine verbatim

VoiceboxExtensions' `Engine`/`deviceStream` design processes one `play(pcm)` call
at a time to full completion before accepting the next one for that device (see
its `internal/audio/stream.go`: a single goroutine loop that blocks inside
`session.play()` until a clip finishes). That is correct for "speak one sentence
at a time" but **cannot overlap two sounds on the same device** — a hard
requirement here.

The fix: replace "one clip owns the stream until done" with a **per-device
mixer** that keeps the WASAPI stream continuously running (feeding silence when
idle) and sums any number of concurrently-active "voices" into every buffer.
Everything below (`mixer.go`, the rewritten `render.go`, `engine.go`) implements
this. The low-level device-enumeration and MMCSS code is copied unchanged from
VoiceboxExtensions since it's already generic.

## 4. Project setup

```powershell
cd d:\Workspace\SoundBoardBasic
go mod init soundboard
go get github.com/lxn/walk@v0.0.0-20210112085537-c389da54e794
go get github.com/lxn/win@v0.0.0-20210218163916-a377121e959e
go get github.com/moutend/go-wca@v0.3.0
go get github.com/go-ole/go-ole@v1.2.6
go get github.com/go-audio/wav@v1.1.0
go get github.com/hajimehoshi/go-mp3@v0.3.4
go get github.com/mewkiz/flac@v1.0.14
go get github.com/jfreymuth/oggvorbis@v1.0.5
go install github.com/akavel/rsrc@latest
go mod tidy
```

Pin the same `lxn/walk`/`lxn/win`/`go-wca`/`go-ole` versions VoiceboxExtensions
uses — they're verified working together (including the manifest fix). Final
`go.mod` should end up with `github.com/go-audio/wav`, `github.com/go-ole/go-ole`,
`github.com/lxn/walk`, `github.com/moutend/go-wca`, `github.com/hajimehoshi/go-mp3`,
`github.com/mewkiz/flac`, `github.com/jfreymuth/oggvorbis` as direct requires.

### Target project structure

```
SoundBoardBasic/
├── go.mod / go.sum
├── app.manifest
├── build.ps1
├── rsrc_windows_amd64.syso        (generated, see §2.1)
├── .gitignore                     (*.exe, soundboard.json)
├── main.go
└── internal/
    ├── audio/
    │   ├── pcm.go
    │   ├── resample.go
    │   ├── devices.go
    │   ├── mmcss.go
    │   ├── render.go
    │   ├── mixer.go
    │   ├── stream.go
    │   └── engine.go
    ├── decode/
    │   ├── decode.go
    │   ├── wav.go
    │   ├── mp3.go
    │   ├── flac.go
    │   ├── vorbis.go
    │   └── m4a.go                (Windows Media Foundation AAC decoder)
    ├── sounds/
    │   └── sounds.go
    ├── config/
    │   └── config.go
    └── ui/
        └── window.go
```

## 5. Build order (do these phases in sequence, verifying each with `go build ./...` before moving on)

1. Scaffolding: `go.mod`, `app.manifest`, `rsrc_windows_amd64.syso`, `.gitignore`.
2. `internal/audio`: `pcm.go`, `resample.go`, `devices.go`, `mmcss.go` (copied
   near-verbatim from VoiceboxExtensions — see §6).
3. `internal/audio`: `render.go` + `mixer.go` + `engine.go` (the new mixer
   architecture — see §7). This is the highest-risk phase architecturally;
   build and manually sanity-check before continuing.
4. `internal/decode`: `wav.go`, `mp3.go`, `vorbis.go`, `flac.go`, `m4a.go`,
   `decode.go` (see §8). Build each decoder file individually if possible;
   FLAC and the Media Foundation interop need extra verification.
5. `internal/sounds/sounds.go` (see §9).
6. `internal/config/config.go` (see §10).
7. `internal/ui/window.go` + `main.go` (see §11–12).
8. Final build, manual smoke test checklist (§13).

---

## 6. `internal/audio` — pieces copied from VoiceboxExtensions

These four files are copied with only cosmetic changes (comments referencing
"speech" adjusted to be generic). Their APIs are already proven; do not modify
their logic.

### 6.1 `internal/audio/pcm.go`

```go
// Package audio enumerates Windows playback devices and renders decoded PCM
// audio to one or more of them concurrently via WASAPI, without cgo.
package audio

// PCM is interleaved, decoded audio ready for resampling and playback.
type PCM struct {
	Samples    []float32
	SampleRate int
	Channels   int
}
```

### 6.2 `internal/audio/resample.go`

```go
package audio

// Resample converts src to the destination sample rate and channel count.
// It uses linear interpolation for rate conversion, which is sufficient
// fidelity for short sound-effect clips, and duplicates or averages channels
// as needed to reach the destination channel count.
func Resample(src PCM, dstSampleRate, dstChannels int) PCM {
	converted := src
	if src.SampleRate != dstSampleRate {
		converted = resampleRate(converted, dstSampleRate)
	}
	if converted.Channels != dstChannels {
		converted = remapChannels(converted, dstChannels)
	}
	return converted
}

// resampleRate performs per-channel linear interpolation to change the
// sample rate while preserving the existing channel layout.
func resampleRate(src PCM, dstSampleRate int) PCM {
	channels := src.Channels
	srcFrames := len(src.Samples) / channels
	ratio := float64(src.SampleRate) / float64(dstSampleRate)
	dstFrames := int(float64(srcFrames) / ratio)

	out := make([]float32, dstFrames*channels)
	for frame := 0; frame < dstFrames; frame++ {
		srcPos := float64(frame) * ratio
		lo := int(srcPos)
		hi := lo + 1
		frac := float32(srcPos - float64(lo))
		if lo >= srcFrames {
			lo = srcFrames - 1
		}
		if hi >= srcFrames {
			hi = srcFrames - 1
		}
		for ch := 0; ch < channels; ch++ {
			a := src.Samples[lo*channels+ch]
			b := src.Samples[hi*channels+ch]
			out[frame*channels+ch] = a + (b-a)*frac
		}
	}
	return PCM{Samples: out, SampleRate: dstSampleRate, Channels: channels}
}

// remapChannels adapts the channel count: sources are averaged down to a
// single value per frame, then duplicated across every destination channel.
func remapChannels(src PCM, dstChannels int) PCM {
	srcFrames := len(src.Samples) / src.Channels
	out := make([]float32, srcFrames*dstChannels)

	for frame := 0; frame < srcFrames; frame++ {
		var sum float32
		for ch := 0; ch < src.Channels; ch++ {
			sum += src.Samples[frame*src.Channels+ch]
		}
		avg := sum / float32(src.Channels)
		for ch := 0; ch < dstChannels; ch++ {
			out[frame*dstChannels+ch] = avg
		}
	}
	return PCM{Samples: out, SampleRate: src.SampleRate, Channels: dstChannels}
}
```

### 6.3 `internal/audio/devices.go`

```go
package audio

import (
	"fmt"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// Device describes an available Windows audio playback endpoint, such as
// speakers, headphones, or a virtual cable like VB-Cable.
type Device struct {
	ID   string
	Name string
}

// EnumerateOutputDevices lists all active audio render (playback) endpoints.
func EnumerateOutputDevices() ([]Device, error) {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return nil, fmt.Errorf("initializing COM: %w", err)
	}
	defer ole.CoUninitialize()

	enumerator, err := newDeviceEnumerator()
	if err != nil {
		return nil, err
	}
	defer enumerator.Release()

	return listRenderDevices(enumerator)
}

func newDeviceEnumerator() (*wca.IMMDeviceEnumerator, error) {
	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		return nil, fmt.Errorf("creating device enumerator: %w", err)
	}
	return enumerator, nil
}

func listRenderDevices(enumerator *wca.IMMDeviceEnumerator) ([]Device, error) {
	var collection *wca.IMMDeviceCollection
	if err := enumerator.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return nil, fmt.Errorf("enumerating render endpoints: %w", err)
	}
	defer collection.Release()

	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, fmt.Errorf("counting render endpoints: %w", err)
	}

	devices := make([]Device, 0, count)
	for i := uint32(0); i < count; i++ {
		device, err := describeDevice(collection, i)
		if err != nil {
			continue
		}
		devices = append(devices, device)
	}
	return devices, nil
}

func describeDevice(collection *wca.IMMDeviceCollection, index uint32) (Device, error) {
	var mmDevice *wca.IMMDevice
	if err := collection.Item(index, &mmDevice); err != nil {
		return Device{}, err
	}
	defer mmDevice.Release()

	var id string
	if err := mmDevice.GetId(&id); err != nil {
		return Device{}, err
	}

	name, err := friendlyName(mmDevice)
	if err != nil {
		return Device{}, err
	}
	return Device{ID: id, Name: name}, nil
}

func friendlyName(mmDevice *wca.IMMDevice) (string, error) {
	var store *wca.IPropertyStore
	if err := mmDevice.OpenPropertyStore(wca.STGM_READ, &store); err != nil {
		return "", err
	}
	defer store.Release()

	var value wca.PROPVARIANT
	if err := store.GetValue(&wca.PKEY_Device_FriendlyName, &value); err != nil {
		return "", err
	}
	return value.String(), nil
}

// findDeviceByID re-enumerates active render endpoints to locate one by its
// stable endpoint ID. go-wca doesn't expose IMMDeviceEnumerator::GetDevice,
// so lookups are done by scanning the active collection instead. The caller
// takes ownership of the returned device and must Release it.
func findDeviceByID(enumerator *wca.IMMDeviceEnumerator, id string) (*wca.IMMDevice, error) {
	var collection *wca.IMMDeviceCollection
	if err := enumerator.EnumAudioEndpoints(wca.ERender, wca.DEVICE_STATE_ACTIVE, &collection); err != nil {
		return nil, fmt.Errorf("enumerating render endpoints: %w", err)
	}
	defer collection.Release()

	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, fmt.Errorf("counting render endpoints: %w", err)
	}

	for i := uint32(0); i < count; i++ {
		var mmDevice *wca.IMMDevice
		if err := collection.Item(i, &mmDevice); err != nil {
			continue
		}
		var candidateID string
		if err := mmDevice.GetId(&candidateID); err != nil {
			mmDevice.Release()
			continue
		}
		if candidateID == id {
			return mmDevice, nil
		}
		mmDevice.Release()
	}
	return nil, fmt.Errorf("device %s not found (unplugged or disabled?)", id)
}
```

### 6.4 `internal/audio/mmcss.go`

```go
package audio

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	avrt                                = syscall.NewLazyDLL("avrt.dll")
	procAvSetMmThreadCharacteristicsW   = avrt.NewProc("AvSetMmThreadCharacteristicsW")
	procAvRevertMmThreadCharacteristics = avrt.NewProc("AvRevertMmThreadCharacteristics")
)

// beginProAudioPriority registers the calling OS thread with Windows' MMCSS
// scheduler under the "Pro Audio" task category, so the scheduler gives it
// priority over ordinary background work. Without this, concurrent render
// threads can preempt each other into missing their buffer deadlines, which
// is audible as crackling/garbled output on stricter downstream consumers
// (e.g. virtual audio cables). The returned revert func must be called
// before the thread exits.
func beginProAudioPriority() (revert func(), err error) {
	taskName, err := syscall.UTF16PtrFromString("Pro Audio")
	if err != nil {
		return nil, fmt.Errorf("encoding task name: %w", err)
	}

	var taskIndex uint32
	handle, _, callErr := procAvSetMmThreadCharacteristicsW.Call(
		uintptr(unsafe.Pointer(taskName)),
		uintptr(unsafe.Pointer(&taskIndex)),
	)
	if handle == 0 {
		return nil, fmt.Errorf("AvSetMmThreadCharacteristics failed: %w", callErr)
	}

	return func() {
		procAvRevertMmThreadCharacteristics.Call(handle)
	}, nil
}
```

---

## 7. `internal/audio` — the new mixer architecture

### 7.1 `internal/audio/mixer.go` (new)

```go
package audio

import "sync"

// voice is a single active playback instance being mixed into a device's
// output. samples must already be resampled/remapped to the device's
// native sample rate and channel count before being added.
type voice struct {
	samples []float32
	pos     int // next sample index (not frame index) to read
}

// mixer combines any number of concurrently active voices into one
// continuous stream for a single output device, which is what makes
// overlapping playback (multiple soundboard buttons at once) possible on a
// single WASAPI stream.
type mixer struct {
	mu         sync.Mutex
	voices     []*voice
	everPlayed bool
}

// addVoice starts mixing samples (already converted to the device's native
// format) into the output, overlapping with anything already playing.
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

// fill writes exactly len(dst) mixed samples (interleaved) into dst,
// advancing and removing finished voices, summing overlapping voices, and
// clipping to [-1, 1] to avoid harsh digital overflow when several loud
// sounds overlap. Silence (0) is written wherever no voice has data.
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

	for i, s := range dst {
		if s > 1 {
			dst[i] = 1
		} else if s < -1 {
			dst[i] = -1
		}
	}
}

// idle reports whether nothing is currently playing but something has
// played before (used to decide whether a "keep outputs open = false"
// stream should close itself).
func (m *mixer) idle() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.everPlayed && len(m.voices) == 0
}
```

### 7.2 `internal/audio/render.go` (new — low-level WASAPI primitives + continuous mix loop)

This keeps the same event-driven-WASAPI + MMCSS approach VoiceboxExtensions
settled on (do not go back to timer-driven `time.Sleep` polling — that
architecture was already tried and replaced because it glitches once more than
one render stream runs concurrently, which for this app is the common case).

```go
package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// eventWaitTimeoutMs bounds each wait on the render event, so the mix loop
// still gets a chance to notice a closed cancel channel even if the engine
// were to stop signaling for some reason.
const eventWaitTimeoutMs = 200

// renderSession owns an open WASAPI shared-mode render stream for one
// device. All of its methods must be called from the single goroutine that
// created it, since the underlying COM objects are apartment-threaded.
type renderSession struct {
	enumerator   *wca.IMMDeviceEnumerator
	device       *wca.IMMDevice
	client       *wca.IAudioClient
	renderClient *wca.IAudioRenderClient
	format       *wca.WAVEFORMATEX
	bufferFrames uint32
	event        uintptr
}

// SampleRate, Channels, and BitsPerSample expose the device's native mix
// format, needed by callers to resample voices before adding them.
func (s *renderSession) SampleRate() int    { return int(s.format.NSamplesPerSec) }
func (s *renderSession) Channels() int      { return int(s.format.NChannels) }
func (s *renderSession) BitsPerSample() int { return int(s.format.WBitsPerSample) }

// openRenderSession initializes COM on the current thread, locates the
// device by ID, and starts an event-driven shared-mode render stream
// matched to the device's own mix format (always a supported format,
// avoiding any guesswork about what the endpoint will accept).
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

// activateClient activates IAudioClient on device and initializes an
// event-driven shared-mode stream using the device's own mix format.
func activateClient(device *wca.IMMDevice) (client *wca.IAudioClient, format *wca.WAVEFORMATEX, bufferFrames uint32, err error) {
	if err = device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &client); err != nil {
		return nil, nil, 0, fmt.Errorf("activating audio client: %w", err)
	}

	if err = client.GetMixFormat(&format); err != nil {
		client.Release()
		return nil, nil, 0, fmt.Errorf("getting mix format: %w", err)
	}

	var defaultPeriod wca.REFERENCE_TIME
	if err = client.GetDevicePeriod(&defaultPeriod, nil); err != nil {
		client.Release()
		return nil, nil, 0, fmt.Errorf("getting device period: %w", err)
	}

	if err = client.Initialize(wca.AUDCLNT_SHAREMODE_SHARED, wca.AUDCLNT_STREAMFLAGS_EVENTCALLBACK, defaultPeriod, 0, format, nil); err != nil {
		client.Release()
		return nil, nil, 0, fmt.Errorf("initializing audio client: %w", err)
	}

	if err = client.GetBufferSize(&bufferFrames); err != nil {
		client.Release()
		return nil, nil, 0, fmt.Errorf("getting buffer size: %w", err)
	}

	return client, format, bufferFrames, nil
}

// runMixLoop continuously feeds m's mixed output into the device until
// closeCh is closed or shouldStop returns true after an idle mix (used to
// implement "keep outputs open = false"). It writes silence whenever m has
// nothing to play, which is what allows the stream to stay open (warm) with
// no audible artifacts between sounds.
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
			return
		}
	}
}

// close stops playback, releases every COM resource this session holds, and
// uninitializes COM for the goroutine that owns it.
func (s *renderSession) close() {
	_ = s.client.Stop() // best-effort; nothing actionable if it fails during shutdown
	s.renderClient.Release()
	s.client.Release()
	s.device.Release()
	s.enumerator.Release()
	wca.CloseHandle(s.event)
	ole.CoTaskMemFree(uintptr(unsafe.Pointer(s.format)))
	ole.CoUninitialize()
}

// renderFrameBytes packs interleaved float32 samples into the raw byte
// layout WASAPI expects for the given bit depth. 32-bit IEEE float is the
// overwhelmingly common case for a WASAPI shared-mode mix format; 16-bit
// integer PCM is supported as a fallback for uncommon drivers.
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

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
```

**Note on `renderFrameBytes`'s `PCM{..., Channels: channels}` call above:** the
`SampleRate` field is irrelevant at this point (samples are already in the
device's native format) — it's only included because `PCM` is a shared type;
leaving `SampleRate` zero here is fine since `renderFrameBytes` never reads it.

### 7.3 `internal/audio/stream.go` (new — per-device goroutine owning one mixer)

```go
package audio

import "runtime"

// deviceStream owns one continuously-running WASAPI render session and its
// mixer for one device, on a single dedicated goroutine (the underlying COM
// objects are apartment-threaded and must only ever be used from the thread
// that created them). It keeps running — feeding silence when idle — for as
// long as warm mode is on, enabling near-instant overlapping playback.
type deviceStream struct {
	mixer   *mixer
	closeCh chan struct{}
	closed  chan struct{}

	sampleRate    int
	channels      int
	bitsPerSample int
}

// newDeviceStream opens a render session on a dedicated OS thread and
// returns once it's ready to accept voices (or the open failed). warm
// controls whether the stream closes itself once idle (see run()).
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
	defer close(s.closed)

	// Best-effort: if MMCSS registration fails, fall back to normal thread
	// priority rather than refusing to play at all.
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
		return !warm() && s.mixer.idle()
	}
	session.runMixLoop(s.mixer, s.closeCh, shouldStop)
}

// play adds pcm (already resampled to this stream's native format) as a new
// overlapping voice.
func (s *deviceStream) play(samples []float32) {
	s.mixer.addVoice(samples)
}

// stopped reports whether this stream's goroutine has exited (e.g. it
// self-closed due to "keep outputs open" being off and going idle).
func (s *deviceStream) stopped() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

// close signals the dedicated goroutine to stop and waits for its COM
// resources to be released. Safe to call more than once.
func (s *deviceStream) close() {
	select {
	case <-s.closeCh:
	default:
		close(s.closeCh)
	}
	<-s.closed
}
```

### 7.4 `internal/audio/engine.go` (new — manages one stream per selected device)

```go
package audio

import "sync"

// Engine manages one continuously-running mixer stream per output device
// that's currently in use, allowing overlapping playback across multiple
// simultaneous sounds and across multiple simultaneously selected devices.
type Engine struct {
	mu      sync.Mutex
	streams map[string]*deviceStream
	warm    bool

	// OnError is invoked (from an arbitrary goroutine) whenever opening a
	// device stream fails, so the UI layer can surface it.
	OnError func(deviceID string, err error)
}

// NewEngine creates an Engine. warm controls whether device streams stay
// open once they go idle; see SetWarm.
func NewEngine(warm bool) *Engine {
	return &Engine{streams: make(map[string]*deviceStream), warm: warm}
}

// SetWarm toggles whether idle device streams stay open. Turning it off
// does not immediately close currently-playing streams; each stream closes
// itself the next time it finishes everything queued on it, if warm is
// still off at that moment. Turning it on prevents any further self-close.
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
	stream, err := e.getOrOpenStream(deviceID)
	if err != nil {
		e.reportError(deviceID, err)
		return
	}
	converted := Resample(pcm, stream.sampleRate, stream.channels)
	stream.play(converted.Samples)
}

func (e *Engine) getOrOpenStream(deviceID string) (*deviceStream, error) {
	e.mu.Lock()
	stream, ok := e.streams[deviceID]
	e.mu.Unlock()
	if ok && !stream.stopped() {
		return stream, nil
	}

	opened, err := newDeviceStream(deviceID, e.isWarm)
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.streams[deviceID] = opened
	e.mu.Unlock()
	return opened, nil
}

// StopAll immediately silences playback on every currently open device
// stream, without closing the streams themselves.
func (e *Engine) StopAll() {
	e.mu.Lock()
	streams := make([]*deviceStream, 0, len(e.streams))
	for _, s := range e.streams {
		streams = append(streams, s)
	}
	e.mu.Unlock()

	for _, s := range streams {
		s.mixer.stopAll()
	}
}

func (e *Engine) reportError(deviceID string, err error) {
	if e.OnError != nil {
		e.OnError(deviceID, err)
	}
}

// Close releases every currently open device stream. Call this once on
// application shutdown.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, stream := range e.streams {
		stream.close()
		delete(e.streams, id)
	}
}
```

**Verification for phase 3:** `go build ./...` must succeed with just these
audio files plus a placeholder `main.go` (`package main; func main() {}`) before
moving on. There is no runnable behavior to test yet without the UI, but a
clean build here catches type errors early where they're cheapest to fix.

---

## 8. `internal/decode` — format decoding

All decoders convert into `audio.PCM{Samples []float32, SampleRate int, Channels int}`,
matching VoiceboxExtensions' `decode.WAV` convention.

### 8.1 `internal/decode/wav.go`

```go
// Package decode turns encoded audio bytes into raw PCM the audio package
// can play.
package decode

import (
	"bytes"
	"fmt"

	"github.com/go-audio/wav"

	"soundboard/internal/audio"
)

// WAV decodes a WAV file's bytes into interleaved float32 PCM samples.
func WAV(data []byte) (audio.PCM, error) {
	decoder := wav.NewDecoder(bytes.NewReader(data))

	buf, err := decoder.FullPCMBuffer()
	if err != nil {
		return audio.PCM{}, fmt.Errorf("decoding wav: %w", err)
	}
	if buf.Format == nil || buf.Format.NumChannels == 0 || buf.Format.SampleRate == 0 {
		return audio.PCM{}, fmt.Errorf("wav file missing format information")
	}

	f32 := buf.AsFloat32Buffer()
	return audio.PCM{
		Samples:    f32.Data,
		SampleRate: f32.Format.SampleRate,
		Channels:   f32.Format.NumChannels,
	}, nil
}
```

### 8.2 `internal/decode/mp3.go`

Verified API: `mp3.NewDecoder(io.Reader) (*mp3.Decoder, error)`. The decoded
stream is **always** 16-bit little-endian, **always 2 channels**, regardless of
the source's actual channel count — this is documented library behavior, not a
bug to work around.

```go
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

	// go-mp3 always outputs 16-bit little-endian, 2-channel PCM.
	const channels = 2
	samples := make([]float32, len(raw)/2)
	for i := range samples {
		v := int16(binary.LittleEndian.Uint16(raw[i*2:]))
		samples[i] = float32(v) / 32768
	}

	return audio.PCM{
		Samples:    samples,
		SampleRate: decoder.SampleRate(),
		Channels:   channels,
	}, nil
}
```

### 8.3 `internal/decode/vorbis.go`

Verified API: `oggvorbis.ReadAll(io.Reader) ([]float32, *oggvorbis.Format, error)`
is a convenience one-shot decode, exactly matching our "decode a short clip
fully into memory" use case — no manual buffering loop needed.

```go
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
```

### 8.4 `internal/decode/flac.go`

**This is the highest-risk decoder in this plan — read this section fully
before writing the code, and test it against a real `.flac` file before
considering this phase done.**

Verified API (from `github.com/mewkiz/flac` v1.0.14 docs):

- `flac.New(io.Reader) (*flac.Stream, error)` parses the signature + the
  mandatory `StreamInfo` metadata block only (faster than `flac.Parse`, which
  parses every metadata block — we don't need tags/pictures/etc., so use `New`).
- `stream.Info` is `*meta.StreamInfo` with fields `SampleRate uint32`,
  `NChannels uint8`, `BitsPerSample uint8` — these are the authoritative
  values for the whole file; use these, not anything per-frame.
- `stream.ParseNext() (*frame.Frame, error)` returns `io.EOF` at the end.
  **Important:** `ParseNext` internally calls `Frame.Parse()`, whose
  documented behavior is: *"If the samples are inter-channel decorrelated
  between the subframes, it correlates them."* This means **you do not need
  to manually undo FLAC's mid-side/left-side/right-side stereo
  decorrelation** — by the time `ParseNext` returns, `frame.Subframes[ch].Samples`
  already contains final, correct per-channel PCM values at the stream's bit
  depth.
- Each `frame.Subframes[ch]` is a `*frame.Subframe` with `Samples []int32`
  (length equal to that frame's block size) — one slice per channel.

```go
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
	if channels == 0 || bitsPerSample == 0 {
		return audio.PCM{}, fmt.Errorf("flac file missing format information")
	}
	// Full-scale magnitude for a signed integer at this bit depth, used to
	// normalize samples into the [-1, 1] float32 range.
	scale := float32(int32(1) << uint(bitsPerSample-1))

	var samples []float32
	for {
		f, err := stream.ParseNext()
		if err == io.EOF {
			break
		}
		if err != nil {
			return audio.PCM{}, fmt.Errorf("decoding flac frame: %w", err)
		}

		frameSamples := len(f.Subframes[0].Samples)
		for i := 0; i < frameSamples; i++ {
			for ch := 0; ch < channels; ch++ {
				samples = append(samples, float32(f.Subframes[ch].Samples[i])/scale)
			}
		}
	}

	return audio.PCM{
		Samples:    samples,
		SampleRate: int(stream.Info.SampleRate),
		Channels:   channels,
	}, nil
}
```

**If this produces distorted/garbled audio when tested:** the most likely
culprit is the normalization `scale` (some encoders store certain subframes at
a slightly different effective bit depth during decorrelation). If you hit
this, check `f.Header.BitsPerSample` per-frame as a cross-check against
`stream.Info.BitsPerSample`, and consult the library's own examples at
`https://github.com/mewkiz/flac` before guessing further. **If FLAC playback
sounds wrong and can't be quickly fixed, it is acceptable to drop FLAC from
the supported-formats list for v1** (update the tooltip and `SupportedExtensions`
in §9 accordingly) rather than ship distorted audio — WAV/MP3/OGG already
satisfy "common formats".

### 8.5 `internal/decode/decode.go` — format dispatch

Detects format primarily by content (magic bytes), extension only as fallback,
so a mislabeled file doesn't get handed to the wrong decoder.

```go
// Package decode turns encoded audio bytes into raw PCM the audio package
// can play, dispatching to a format-specific decoder.
package decode

import (
	"fmt"
	"path/filepath"
	"strings"

	"soundboard/internal/audio"
)

// File decodes an audio file's bytes into interleaved float32 PCM. name is
// used only for error messages and as a fallback format hint.
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
		return formatMP3 // MPEG frame sync without a leading ID3 tag
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
```

---

## 9. `internal/sounds/sounds.go` — scanning the sounds folder + decode caching

Decoding happens lazily on first `Play`, then is cached in memory, since a
soundboard implies rapid repeat clicks on the same file — re-decoding from disk
every click would be wasteful and add latency.

```go
// Package sounds scans the sounds folder and caches decoded audio for quick,
// repeated playback.
package sounds

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"soundboard/internal/audio"
	"soundboard/internal/decode"
)

// SupportedExtensions lists every file extension this app can decode. Keep
// this in sync with internal/decode's dispatch table and the UI tooltip.
var SupportedExtensions = []string{".wav", ".mp3", ".flac", ".ogg", ".oga", ".m4a"}

// Sound is one playable entry found in the sounds folder.
type Sound struct {
	Name string // filename shown in the UI, including extension
	Path string // absolute path on disk

	pcm    audio.PCM
	loaded bool
	err    error
}

// Dir returns the sounds folder path beside the running executable,
// creating it if it doesn't exist yet.
func Dir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating executable path: %w", err)
	}
	dir := filepath.Join(filepath.Dir(exe), "sounds")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating sounds folder: %w", err)
	}
	return dir, nil
}

// Scan lists every supported audio file directly inside dir (subdirectories
// are ignored), sorted alphabetically by filename, case-insensitively.
func Scan(dir string) ([]*Sound, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading sounds folder: %w", err)
	}

	var found []*Sound
	for _, entry := range entries {
		if entry.IsDir() || !isSupported(entry.Name()) {
			continue
		}
		found = append(found, &Sound{
			Name: entry.Name(),
			Path: filepath.Join(dir, entry.Name()),
		})
	}

	sort.Slice(found, func(i, j int) bool {
		return strings.ToLower(found[i].Name) < strings.ToLower(found[j].Name)
	})
	return found, nil
}

func isSupported(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, supported := range SupportedExtensions {
		if ext == supported {
			return true
		}
	}
	return false
}

// PCM returns this sound's decoded audio, decoding and caching it on first
// call; later calls are instant. A decode error is cached and returned again
// on subsequent calls without retrying — call Scan again (via Refresh) to
// get a fresh *Sound that will retry.
func (s *Sound) PCM() (audio.PCM, error) {
	if s.loaded {
		return s.pcm, s.err
	}
	s.loaded = true

	data, err := os.ReadFile(s.Path)
	if err != nil {
		s.err = fmt.Errorf("reading file: %w", err)
		return audio.PCM{}, s.err
	}

	s.pcm, s.err = decode.File(s.Name, data)
	return s.pcm, s.err
}
```

---

## 10. `internal/config/config.go`

Same portable, atomic-write pattern as VoiceboxExtensions, adapted (no
`ProfileID`/`BackendURL` fields — this app has no backend).

```go
// Package config loads and saves the application's persisted settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// fileName is the settings file, always kept beside the running executable
// so the app stays portable.
const fileName = "soundboard.json"

// OutputDevice is a persisted audio output selection.
type OutputDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// WindowState persists the main window's last position and size.
type WindowState struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Config is the full set of persisted application settings.
type Config struct {
	KeepOutputsOpen bool           `json:"keep_outputs_open"`
	Outputs         []OutputDevice `json:"outputs"`
	Window          WindowState    `json:"window"`
}

// Default returns a Config populated with sensible defaults for a first run.
func Default() Config {
	return Config{
		KeepOutputsOpen: true,
		Window:          WindowState{X: 100, Y: 100, W: 420, H: 520},
	}
}

func path() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating executable path: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), fileName), nil
}

// Load reads the config file beside the executable. If it doesn't exist yet,
// it returns Default() without error so first runs work out of the box.
func Load() (Config, error) {
	p, err := path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", p, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing %s: %w", p, err)
	}
	return cfg, nil
}

// Save writes the config file atomically (temp file + rename) so a crash or
// power loss mid-write can't corrupt the settings file.
func Save(cfg Config) error {
	p, err := path()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp config: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		return fmt.Errorf("replacing config file: %w", err)
	}
	return nil
}
```

---

## 11. `internal/ui/window.go`

### 11.1 Layout

```
MainWindow ("SoundBoardBasic")
  VBox
    Composite (HBox): [Refresh Sounds]  [Stop All]
    ScrollView                                  <- sounds list, scrollable
      Composite (Grid, 2 columns), AssignTo &soundsContainer
        (rows added procedurally: Label(name), PushButton("Play"))
		PushButton "Audio Outputs [-]/[+]"         <- collapses/expands the section
		GroupBox (ToolTipText = supported-formats text)
      Composite (VBox), AssignTo &outputsContainer
        (rows added procedurally: one CheckBox per device)
        CheckBox "Keep outputs open (lower latency)"
    Label (status), AssignTo &statusLabel
```

The Audio Outputs header remains visible while the GroupBox is collapsed. Its
label changes between `Audio Outputs [-]` and `Audio Outputs [+]`, and the
GroupBox is hidden/shown to reflow the remaining layout.

Two things are built **procedurally** (via `walk.NewLabel(parent)` /
`walk.NewPushButton(parent)` / `walk.NewCheckBox(parent)`, not the declarative
`Children: []declarative.Widget{...}` slice) rather than declaratively, because
only Refresh Sounds needs to rebuild a widget list at runtime:

- The sounds list (rebuilt every Refresh).
- The output device checkboxes (built once at startup from the initial device
  scan — declarative is fine here too, but building it the same procedural way
  as the sounds list keeps the code consistent; either approach works, use
  whichever is simpler to implement correctly).

**Two-widgets-per-row note:** the sounds list uses `declarative.Grid{Columns: 2}`
/ a `walk.NewGridLayout()`-backed composite, adding a `Label` then a
`PushButton` per sound in that order. This relies on Walk's grid layout
auto-flowing children into rows in insertion order (already proven for
1-widget-per-row lists in VoiceboxExtensions' output checklist and profile
list). **This specific 2-widgets-per-row usage has not been directly verified
running** — after implementing, visually confirm each sound's name and Play
button land on the same row, side by side. If they don't (e.g. Play buttons
stack in a second column block after all labels), fall back to wrapping each
row in its own `walk.NewComposite(soundsContainer)` with an `HBox` layout,
containing that row's Label and PushButton — this is guaranteed to lay out
correctly regardless of grid auto-flow behavior.

### 11.2 Full code

```go
// Package ui builds and drives the Walk-based main window.
package ui

import (
	"fmt"

	"github.com/lxn/walk"
	declarative "github.com/lxn/walk/declarative"

	"soundboard/internal/audio"
	"soundboard/internal/config"
	"soundboard/internal/sounds"
)

const supportedFormatsTooltip = "Supported formats: WAV, MP3, FLAC, OGG/Vorbis, M4A/AAC"

// App wires the GUI to the application's config, sounds folder, and audio
// engine.
type App struct {
	cfg        *config.Config
	engine     *audio.Engine
	devices    []audio.Device
	soundsDir  string

	mainWindow       *walk.MainWindow
	statusLabel      *walk.Label
	soundsContainer  *walk.Composite
	deviceCheckBoxes map[string]*walk.CheckBox

	soundRows []*soundRow
}

type soundRow struct {
	label  *walk.Label
	button *walk.PushButton
	sound  *sounds.Sound
}

// New creates the application UI wiring for the given dependencies.
func New(cfg *config.Config, engine *audio.Engine, devices []audio.Device, soundsDir string) *App {
	app := &App{
		cfg:              cfg,
		engine:           engine,
		devices:          devices,
		soundsDir:        soundsDir,
		deviceCheckBoxes: make(map[string]*walk.CheckBox),
	}
	engine.OnError = app.onPlaybackError
	return app
}

// Run builds the main window and blocks until it's closed.
func (a *App) Run() error {
	var refreshButton, stopAllButton *walk.PushButton
	var soundsContainer *walk.Composite
	var outputsGroup *walk.GroupBox
	var keepOpenBox *walk.CheckBox
	var statusLabel *walk.Label
	var mw *walk.MainWindow

	checkBoxes := make([]*walk.CheckBox, len(a.devices))
	outputChildren := make([]declarative.Widget, len(a.devices))
	for i, device := range a.devices {
		i, device := i, device
		outputChildren[i] = declarative.CheckBox{
			AssignTo: &checkBoxes[i],
			Text:     device.Name,
			Checked:  a.isEnabled(device.ID),
			OnCheckedChanged: func() {
				a.onOutputToggled(device.ID, checkBoxes[i].Checked())
			},
		}
	}

	err := declarative.MainWindow{
		AssignTo: &mw,
		Title:    "SoundBoardBasic",
		Bounds: declarative.Rectangle{
			X: a.cfg.Window.X, Y: a.cfg.Window.Y,
			Width: a.cfg.Window.W, Height: a.cfg.Window.H,
		},
		Layout: declarative.VBox{},
		Children: []declarative.Widget{
			declarative.Composite{
				Layout: declarative.HBox{},
				Children: []declarative.Widget{
					declarative.PushButton{
						AssignTo: &refreshButton,
						Text:     "Refresh Sounds",
						OnClicked: func() {
							a.onRefreshSounds()
						},
					},
					declarative.PushButton{
						AssignTo: &stopAllButton,
						Text:     "Stop All",
						OnClicked: func() {
							a.engine.StopAll()
						},
					},
				},
			},
			declarative.ScrollView{
				Layout: declarative.VBox{},
				Children: []declarative.Widget{
					declarative.Composite{
						AssignTo: &soundsContainer,
						Layout:   declarative.Grid{Columns: 2},
					},
				},
			},
			declarative.GroupBox{
				AssignTo:    &outputsGroup,
				Title:       "Audio Outputs",
				ToolTipText: supportedFormatsTooltip,
				Layout:      declarative.VBox{},
				Children: append(outputChildren, declarative.CheckBox{
					AssignTo: &keepOpenBox,
					Text:     "Keep outputs open (lower latency)",
					Checked:  a.cfg.KeepOutputsOpen,
					OnCheckedChanged: func() {
						a.onKeepOpenChanged(keepOpenBox.Checked())
					},
				}),
			},
			declarative.Label{AssignTo: &statusLabel, Text: ""},
		},
	}.Create()
	if err != nil {
		return fmt.Errorf("creating window: %w", err)
	}

	a.mainWindow = mw
	a.statusLabel = statusLabel
	a.soundsContainer = soundsContainer
	for i, device := range a.devices {
		a.deviceCheckBoxes[device.ID] = checkBoxes[i]
	}

	a.onRefreshSounds()

	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		a.onClosing()
	})

	mw.Run()
	return nil
}

// onRefreshSounds re-scans the sounds folder and rebuilds the list. Safe to
// call repeatedly; must be called on the UI thread.
func (a *App) onRefreshSounds() {
	for _, row := range a.soundRows {
		a.soundsContainer.Children().Remove(row.label)
		a.soundsContainer.Children().Remove(row.button)
		row.label.Dispose()
		row.button.Dispose()
	}
	a.soundRows = a.soundRows[:0]

	found, err := sounds.Scan(a.soundsDir)
	if err != nil {
		a.setStatus(fmt.Sprintf("Failed to scan sounds folder: %v", err))
		return
	}

	for _, sound := range found {
		sound := sound

		label, err := walk.NewLabel(a.soundsContainer)
		if err != nil {
			continue
		}
		label.SetText(sound.Name)

		button, err := walk.NewPushButton(a.soundsContainer)
		if err != nil {
			continue
		}
		button.SetText("Play")
		button.Clicked().Attach(func() {
			a.onPlaySound(sound)
		})

		a.soundRows = append(a.soundRows, &soundRow{label: label, button: button, sound: sound})
	}

	a.setStatus(fmt.Sprintf("Loaded %d sound(s)", len(found)))
}

// onPlaySound decodes (if not already cached) and plays sound on every
// selected output device, without blocking the UI thread.
func (a *App) onPlaySound(sound *sounds.Sound) {
	deviceIDs := a.selectedDeviceIDs()
	if len(deviceIDs) == 0 {
		a.setStatus("Select at least one audio output first")
		return
	}

	go func() {
		pcm, err := sound.PCM()
		if err != nil {
			a.setStatus(fmt.Sprintf("Could not play %s: %v", sound.Name, err))
			return
		}
		a.engine.Play(deviceIDs, pcm)
	}()
}

func (a *App) selectedDeviceIDs() []string {
	var ids []string
	for id, box := range a.deviceCheckBoxes {
		if box.Checked() {
			ids = append(ids, id)
		}
	}
	return ids
}

func (a *App) onOutputToggled(deviceID string, checked bool) {
	for i := range a.cfg.Outputs {
		if a.cfg.Outputs[i].ID == deviceID {
			a.cfg.Outputs[i].Enabled = checked
			a.saveConfig()
			return
		}
	}
	if name := a.deviceName(deviceID); name != deviceID {
		a.cfg.Outputs = append(a.cfg.Outputs, config.OutputDevice{ID: deviceID, Name: name, Enabled: checked})
		a.saveConfig()
	}
}

func (a *App) onKeepOpenChanged(checked bool) {
	a.cfg.KeepOutputsOpen = checked
	a.engine.SetWarm(checked)
	a.saveConfig()
}

func (a *App) onClosing() {
	bounds := a.mainWindow.Bounds()
	a.cfg.Window = config.WindowState{X: bounds.X, Y: bounds.Y, W: bounds.Width, H: bounds.Height}
	a.saveConfig()
	a.engine.Close()
}

func (a *App) onPlaybackError(deviceID string, err error) {
	a.setStatus(fmt.Sprintf("Playback error on %s: %v", a.deviceName(deviceID), err))
}

func (a *App) isEnabled(deviceID string) bool {
	for _, o := range a.cfg.Outputs {
		if o.ID == deviceID {
			return o.Enabled
		}
	}
	return false
}

func (a *App) deviceName(deviceID string) string {
	for _, d := range a.devices {
		if d.ID == deviceID {
			return d.Name
		}
	}
	return deviceID
}

func (a *App) saveConfig() {
	if err := config.Save(*a.cfg); err != nil {
		a.setStatus(fmt.Sprintf("Failed to save settings: %v", err))
	}
}

// setStatus updates the status label, marshaling onto the UI thread if
// needed. It's a no-op if called before the window has been created.
func (a *App) setStatus(text string) {
	if a.mainWindow == nil {
		return
	}
	a.mainWindow.Synchronize(func() {
		a.statusLabel.SetText(text)
	})
}
```

**Note:** keep `supportedFormatsTooltip` in sync with `sounds.SupportedExtensions`
— if FLAC ends up dropped per §8.4's fallback, remove it from this string too.

---

## 12. `main.go`

```go
// Command soundboard is a portable GUI soundboard that plays sound files
// from a "sounds" folder to one or more selected Windows audio outputs,
// with overlapping playback.
package main

import (
	"fmt"
	"os"

	"soundboard/internal/audio"
	"soundboard/internal/config"
	"soundboard/internal/sounds"
	"soundboard/internal/ui"
)

func main() {
	if err := run(); err != nil {
		walk.MsgBox(nil, "SoundBoardBasic", err.Error(), walk.MsgBoxOK|walk.MsgBoxIconError)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	soundsDir, err := sounds.Dir()
	if err != nil {
		return fmt.Errorf("preparing sounds folder: %w", err)
	}

	devices, err := audio.EnumerateOutputDevices()
	if err != nil {
		return fmt.Errorf("enumerating audio devices: %w", err)
	}

	engine := audio.NewEngine(cfg.KeepOutputsOpen)

	app := ui.New(&cfg, engine, devices, soundsDir)
	return app.Run()
}
```

---

## 13. Final verification checklist

Run these in order; do not report the app "done" until every item passes.

1. `.\build.ps1` must build `soundboard.exe` with exit code 0 and the Windows GUI subsystem, so opening it does not create a console window.
2. `go vet ./...` — must produce no output.
3. Launch `soundboard.exe`:
   - Confirm a `sounds` folder was created beside the exe if it didn't exist.
   - Confirm the window opens without crashing (this is where the manifest
     issue from §2.1 would show up as `TTM_ADDTOOL failed` if missed).
   - Confirm every active Windows playback device appears as a checkbox.
4. Drop short test files into `sounds/` covering `.wav`, `.mp3`, and `.m4a`
   (and `.flac`/`.ogg` if available), click **Refresh Sounds**, confirm
   they appear alphabetically with a Play button each, without restarting the app.
5. Check at least one output device (e.g. default speakers), click Play on a
   sound, confirm audio is audible and not distorted/pitch-shifted.
6. Click Play on two **different** sounds in quick succession — confirm they
   audibly overlap rather than one waiting for the other to finish.
7. Click Play repeatedly on the **same** sound rapidly — confirm each click
   plays its own overlapping instance.
8. Click **Stop All** while sounds are playing — confirm audio stops
   immediately.
9. Toggle **Keep outputs open** off, replay a sound, confirm it still plays
   (slightly higher latency on the first play after being idle is expected
   and fine).
10. Close and relaunch the app — confirm output device selection and the
    keep-open checkbox restored from `soundboard.json`, and the window
    reopened at its previous position/size.
11. Clean up test artifacts (`soundboard.exe`, `soundboard.json`) from the
    repo root before committing, same as VoiceboxExtensions' `.gitignore`
    convention (`*.exe`, `soundboard.json`).

If FLAC playback in step 5 sounds wrong, apply the fallback in §8.4 (drop FLAC
support, update the tooltip and `SupportedExtensions`) rather than leave it
half-working.

---

## 14. `app.manifest` (verbatim, identical to VoiceboxExtensions)

```xml
<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <dependency>
    <dependentAssembly>
      <assemblyIdentity
        type="win32"
        name="Microsoft.Windows.Common-Controls"
        version="6.0.0.0"
        processorArchitecture="*"
        publicKeyToken="6595b64144ccf1df"
        language="*" />
    </dependentAssembly>
  </dependency>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true</dpiAware>
    </windowsSettings>
  </application>
</assembly>
```

## 15. `.gitignore`

```
*.exe
soundboard.json
soundboard.json.tmp
```

---

## 16. Explicitly out of scope for v1 (do not implement unless asked)

- Live filesystem watching of the sounds folder (manual Refresh only).
- Live re-enumeration of audio devices (matches VoiceboxExtensions' own
  current limitation — a device plugged in after launch requires an app
  restart to appear).
- Per-row volume control, drag-to-reorder, hotkeys/global shortcuts, or
  subfolder support in `sounds/` — none of these were requested.
