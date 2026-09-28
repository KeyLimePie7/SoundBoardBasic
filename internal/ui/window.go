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
	cfg       *config.Config
	engine    *audio.Engine
	devices   []audio.Device
	soundsDir string

	mainWindow      *walk.MainWindow
	statusLabel     *walk.Label
	soundsContainer *walk.Composite

	deviceCheckBoxes map[string]*walk.CheckBox
	soundRows        []*soundRow
}

type soundRow struct {
	container *walk.Composite
	label     *walk.Label
	button    *walk.PushButton
	sound     *sounds.Sound
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

// Run builds the main window and blocks until it is closed.
func (a *App) Run() error {
	var refreshButton, stopAllButton *walk.PushButton
	var outputsToggleButton *walk.PushButton
	var outputsGroup *walk.GroupBox
	var soundsContainer *walk.Composite
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
						Layout:   declarative.VBox{},
					},
				},
			},
			declarative.PushButton{
				AssignTo:    &outputsToggleButton,
				Text:        "Audio Outputs [-]",
				ToolTipText: supportedFormatsTooltip,
				OnClicked: func() {
					expanded := !outputsGroup.Visible()
					outputsGroup.SetVisible(expanded)
					if expanded {
						outputsToggleButton.SetText("Audio Outputs [-]")
					} else {
						outputsToggleButton.SetText("Audio Outputs [+]")
					}
				},
			},
			declarative.GroupBox{
				AssignTo:    &outputsGroup,
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

// onRefreshSounds re-scans the sounds folder and rebuilds the list. It must
// be called on the UI thread.
func (a *App) onRefreshSounds() {
	for _, row := range a.soundRows {
		a.soundsContainer.Children().Remove(row.container)
		row.container.Dispose()
	}
	a.soundRows = a.soundRows[:0]

	found, err := sounds.Scan(a.soundsDir)
	if err != nil {
		a.setStatus(fmt.Sprintf("Failed to scan sounds folder: %v", err))
		return
	}

	for _, sound := range found {
		sound := sound

		rowContainer, err := walk.NewComposite(a.soundsContainer)
		if err != nil {
			continue
		}
		if err := rowContainer.SetLayout(walk.NewHBoxLayout()); err != nil {
			rowContainer.Dispose()
			continue
		}

		label, err := walk.NewLabel(rowContainer)
		if err != nil {
			rowContainer.Dispose()
			continue
		}
		label.SetText(sound.Name)

		button, err := walk.NewPushButton(rowContainer)
		if err != nil {
			label.Dispose()
			rowContainer.Dispose()
			continue
		}
		button.SetText("Play")
		button.Clicked().Attach(func() {
			a.onPlaySound(sound)
		})

		a.soundRows = append(a.soundRows, &soundRow{
			container: rowContainer,
			label:     label,
			button:    button,
			sound:     sound,
		})
	}

	a.setStatus(fmt.Sprintf("Loaded %d sound(s)", len(found)))
}

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
	for _, output := range a.cfg.Outputs {
		if output.ID == deviceID {
			return output.Enabled
		}
	}
	return false
}

func (a *App) deviceName(deviceID string) string {
	for _, device := range a.devices {
		if device.ID == deviceID {
			return device.Name
		}
	}
	return deviceID
}

func (a *App) saveConfig() {
	if err := config.Save(*a.cfg); err != nil {
		a.setStatus(fmt.Sprintf("Failed to save settings: %v", err))
	}
}

func (a *App) setStatus(text string) {
	if a.mainWindow == nil {
		return
	}
	a.mainWindow.Synchronize(func() {
		a.statusLabel.SetText(text)
	})
}
