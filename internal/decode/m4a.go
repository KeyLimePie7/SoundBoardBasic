package decode

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"

	"soundboard/internal/audio"
)

const (
	mfVersion                      = 0x00020070
	mfStartupFull                  = 0
	mfSourceReaderFirstAudioStream = uintptr(0xfffffffd)
	mfSourceReaderEndOfStream      = 0x00000002
	pcmFormatTag                   = 1
	pcmBitsPerSample               = 16
	waveFormatExSize               = 18
)

var (
	mfDecoderMu sync.Mutex

	mfplatDLL      = syscall.NewLazyDLL("mfplat.dll")
	mfreadwriteDLL = syscall.NewLazyDLL("mfreadwrite.dll")

	procMFStartup                       = mfplatDLL.NewProc("MFStartup")
	procMFShutdown                      = mfplatDLL.NewProc("MFShutdown")
	procMFCreateMediaType               = mfplatDLL.NewProc("MFCreateMediaType")
	procMFInitMediaTypeFromWaveFormatEx = mfplatDLL.NewProc("MFInitMediaTypeFromWaveFormatEx")
	procMFCreateSourceReaderFromURL     = mfreadwriteDLL.NewProc("MFCreateSourceReaderFromURL")

	mfAudioNumChannels      = ole.NewGUID("37e48bf5-645e-4c5b-89de-ada9e29b696a")
	mfAudioSamplesPerSecond = ole.NewGUID("5faeeae7-0290-4c31-9e8a-c534f68d9dba")
)

// M4A decodes an MP4/M4A audio file through Windows Media Foundation and the
// system AAC decoder, returning interleaved 16-bit PCM as float32 samples.
func M4A(data []byte) (audio.PCM, error) {
	if len(data) == 0 {
		return audio.PCM{}, fmt.Errorf("m4a file is empty")
	}

	tempFile, err := os.CreateTemp("", "soundboard-*.m4a")
	if err != nil {
		return audio.PCM{}, fmt.Errorf("creating temporary m4a file: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return audio.PCM{}, fmt.Errorf("writing temporary m4a file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return audio.PCM{}, fmt.Errorf("closing temporary m4a file: %w", err)
	}

	pcm, err := decodeM4APath(tempPath)
	if err != nil {
		return audio.PCM{}, fmt.Errorf("decoding m4a: %w", err)
	}
	return pcm, nil
}

func decodeM4APath(path string) (audio.PCM, error) {
	mfDecoderMu.Lock()
	defer mfDecoderMu.Unlock()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		return audio.PCM{}, fmt.Errorf("initializing COM for Media Foundation: %w", err)
	}
	defer ole.CoUninitialize()

	if err := startMediaFoundation(); err != nil {
		return audio.PCM{}, err
	}
	defer stopMediaFoundation()

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return audio.PCM{}, fmt.Errorf("encoding m4a path: %w", err)
	}

	if err := procMFCreateSourceReaderFromURL.Find(); err != nil {
		return audio.PCM{}, fmt.Errorf("loading Media Foundation source reader: %w", err)
	}
	var reader unsafe.Pointer
	hr, _, _ := procMFCreateSourceReaderFromURL.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		uintptr(unsafe.Pointer(&reader)),
	)
	runtime.KeepAlive(pathPtr)
	runtime.KeepAlive(&reader)
	if err := checkHRESULT("opening m4a source", hr); err != nil {
		releaseCOM(reader)
		return audio.PCM{}, err
	}
	if reader == nil {
		return audio.PCM{}, fmt.Errorf("opening m4a source returned no reader")
	}
	defer releaseCOM(reader)

	return decodeM4ASourceReader(reader)
}

func decodeM4ASourceReader(reader unsafe.Pointer) (audio.PCM, error) {
	nativeType, err := getNativeAudioType(reader)
	if err != nil {
		return audio.PCM{}, err
	}
	defer releaseCOM(nativeType)

	sampleRate, err := getUINT32Attribute(nativeType, mfAudioSamplesPerSecond)
	if err != nil {
		return audio.PCM{}, fmt.Errorf("reading m4a sample rate: %w", err)
	}
	channels, err := getUINT32Attribute(nativeType, mfAudioNumChannels)
	if err != nil {
		return audio.PCM{}, fmt.Errorf("reading m4a channel count: %w", err)
	}
	if sampleRate == 0 || channels == 0 || channels > 8 {
		return audio.PCM{}, fmt.Errorf("m4a has invalid audio format: %d Hz, %d channels", sampleRate, channels)
	}

	format := wca.WAVEFORMATEX{
		WFormatTag:      pcmFormatTag,
		NChannels:       uint16(channels),
		NSamplesPerSec:  sampleRate,
		NAvgBytesPerSec: sampleRate * channels * (pcmBitsPerSample / 8),
		NBlockAlign:     uint16(channels * (pcmBitsPerSample / 8)),
		WBitsPerSample:  pcmBitsPerSample,
	}

	if err := procMFCreateMediaType.Find(); err != nil {
		return audio.PCM{}, fmt.Errorf("loading Media Foundation media type factory: %w", err)
	}
	var outputType unsafe.Pointer
	hr, _, _ := procMFCreateMediaType.Call(uintptr(unsafe.Pointer(&outputType)))
	runtime.KeepAlive(&outputType)
	if err := checkHRESULT("creating PCM media type", hr); err != nil {
		releaseCOM(outputType)
		return audio.PCM{}, err
	}
	if outputType == nil {
		return audio.PCM{}, fmt.Errorf("creating PCM media type returned no type")
	}
	defer releaseCOM(outputType)

	if err := procMFInitMediaTypeFromWaveFormatEx.Find(); err != nil {
		return audio.PCM{}, fmt.Errorf("loading Media Foundation PCM format helper: %w", err)
	}
	hr, _, _ = procMFInitMediaTypeFromWaveFormatEx.Call(
		uintptr(outputType),
		uintptr(unsafe.Pointer(&format)),
		waveFormatExSize,
	)
	runtime.KeepAlive(outputType)
	runtime.KeepAlive(&format)
	if err := checkHRESULT("initializing PCM media type", hr); err != nil {
		return audio.PCM{}, err
	}

	if err := setCurrentMediaType(reader, outputType); err != nil {
		return audio.PCM{}, err
	}

	blockAlign := int(channels) * (pcmBitsPerSample / 8)
	samples := make([]float32, 0)
	for {
		flags, sample, err := readAudioSample(reader)
		if err != nil {
			return audio.PCM{}, err
		}
		if sample != nil {
			raw, sampleErr := mediaSampleBytes(sample)
			releaseCOM(sample)
			if sampleErr != nil {
				return audio.PCM{}, sampleErr
			}
			if len(raw)%blockAlign != 0 {
				return audio.PCM{}, fmt.Errorf("m4a decoder returned a partial PCM frame")
			}
			for offset := 0; offset < len(raw); offset += 2 {
				value := int16(binary.LittleEndian.Uint16(raw[offset:]))
				samples = append(samples, float32(value)/32768)
			}
		}
		if flags&mfSourceReaderEndOfStream != 0 {
			break
		}
	}

	if len(samples) == 0 {
		return audio.PCM{}, fmt.Errorf("m4a source produced no audio samples")
	}
	return audio.PCM{Samples: samples, SampleRate: int(sampleRate), Channels: int(channels)}, nil
}

func startMediaFoundation() error {
	if err := procMFStartup.Find(); err != nil {
		return fmt.Errorf("loading Media Foundation: %w", err)
	}
	hr, _, _ := procMFStartup.Call(mfVersion, mfStartupFull)
	return checkHRESULT("starting Media Foundation", hr)
}

func stopMediaFoundation() {
	if procMFShutdown.Find() == nil {
		_, _, _ = procMFShutdown.Call()
	}
}

func getNativeAudioType(reader unsafe.Pointer) (unsafe.Pointer, error) {
	var mediaType unsafe.Pointer
	hr, _, _ := syscall.SyscallN(
		comMethod(reader, 5),
		uintptr(reader),
		mfSourceReaderFirstAudioStream,
		0,
		uintptr(unsafe.Pointer(&mediaType)),
	)
	runtime.KeepAlive(reader)
	runtime.KeepAlive(&mediaType)
	if err := checkHRESULT("reading native m4a audio type", hr); err != nil {
		releaseCOM(mediaType)
		return nil, err
	}
	if mediaType == nil {
		return nil, fmt.Errorf("m4a source has no audio media type")
	}
	return mediaType, nil
}

func getUINT32Attribute(mediaType unsafe.Pointer, attribute *ole.GUID) (uint32, error) {
	var value uint32
	hr, _, _ := syscall.SyscallN(
		comMethod(mediaType, 7),
		uintptr(mediaType),
		uintptr(unsafe.Pointer(attribute)),
		uintptr(unsafe.Pointer(&value)),
	)
	runtime.KeepAlive(mediaType)
	runtime.KeepAlive(attribute)
	runtime.KeepAlive(&value)
	if err := checkHRESULT("reading Media Foundation audio attribute", hr); err != nil {
		return 0, err
	}
	return value, nil
}

func setCurrentMediaType(reader, mediaType unsafe.Pointer) error {
	hr, _, _ := syscall.SyscallN(
		comMethod(reader, 7),
		uintptr(reader),
		mfSourceReaderFirstAudioStream,
		0,
		uintptr(mediaType),
	)
	runtime.KeepAlive(reader)
	runtime.KeepAlive(mediaType)
	return checkHRESULT("selecting PCM output for m4a", hr)
}

func readAudioSample(reader unsafe.Pointer) (uint32, unsafe.Pointer, error) {
	var flags uint32
	var sample unsafe.Pointer
	hr, _, _ := syscall.SyscallN(
		comMethod(reader, 9),
		uintptr(reader),
		mfSourceReaderFirstAudioStream,
		0,
		0,
		uintptr(unsafe.Pointer(&flags)),
		0,
		uintptr(unsafe.Pointer(&sample)),
	)
	runtime.KeepAlive(reader)
	runtime.KeepAlive(&flags)
	runtime.KeepAlive(&sample)
	if err := checkHRESULT("reading m4a audio sample", hr); err != nil {
		releaseCOM(sample)
		return 0, nil, err
	}
	return flags, sample, nil
}

func mediaSampleBytes(sample unsafe.Pointer) ([]byte, error) {
	var buffer unsafe.Pointer
	hr, _, _ := syscall.SyscallN(
		comMethod(sample, 41),
		uintptr(sample),
		uintptr(unsafe.Pointer(&buffer)),
	)
	runtime.KeepAlive(sample)
	runtime.KeepAlive(&buffer)
	if err := checkHRESULT("getting contiguous audio buffer", hr); err != nil {
		releaseCOM(buffer)
		return nil, err
	}
	if buffer == nil {
		return nil, fmt.Errorf("m4a sample returned no audio buffer")
	}
	defer releaseCOM(buffer)

	var data *byte
	var maxLength uint32
	var currentLength uint32
	hr, _, _ = syscall.SyscallN(
		comMethod(buffer, 3),
		uintptr(buffer),
		uintptr(unsafe.Pointer(&data)),
		uintptr(unsafe.Pointer(&maxLength)),
		uintptr(unsafe.Pointer(&currentLength)),
	)
	runtime.KeepAlive(buffer)
	runtime.KeepAlive(&data)
	runtime.KeepAlive(&maxLength)
	runtime.KeepAlive(&currentLength)
	if err := checkHRESULT("locking decoded audio buffer", hr); err != nil {
		return nil, err
	}

	locked := true
	defer func() {
		if locked {
			_, _, _ = syscall.SyscallN(comMethod(buffer, 4), uintptr(buffer))
		}
	}()
	if currentLength > maxLength || (currentLength > 0 && data == nil) {
		return nil, fmt.Errorf("Media Foundation returned an invalid audio buffer")
	}
	decoded := make([]byte, int(currentLength))
	if currentLength > 0 {
		copy(decoded, unsafe.Slice(data, int(currentLength)))
	}

	hr, _, _ = syscall.SyscallN(comMethod(buffer, 4), uintptr(buffer))
	locked = false
	if err := checkHRESULT("unlocking decoded audio buffer", hr); err != nil {
		return nil, err
	}
	return decoded, nil
}

func comMethod(object unsafe.Pointer, index uintptr) uintptr {
	vtable := *(*unsafe.Pointer)(object)
	return *(*uintptr)(unsafe.Add(vtable, index*unsafe.Sizeof(uintptr(0))))
}

func releaseCOM(object unsafe.Pointer) {
	if object == nil {
		return
	}
	_, _, _ = syscall.SyscallN(comMethod(object, 2), uintptr(object))
	runtime.KeepAlive(object)
}

func checkHRESULT(operation string, result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("%s: HRESULT 0x%08X", operation, uint32(result))
	}
	return nil
}
