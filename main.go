// Command soundboard is a portable GUI soundboard that plays files from a
// sounds folder to one or more selected Windows audio outputs.
package main

import (
	"fmt"
	"os"

	"github.com/lxn/walk"

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
