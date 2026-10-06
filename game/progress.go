package game

import (
	"encoding/json"
	"os"
	"path/filepath"
)

var progress map[string][][]string

func loadProgress() {
	progress = make(map[string][][]string)
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	progPath := filepath.Join(cfgDir, "connections", "progress.json")
	data, err := os.ReadFile(progPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &progress)
}

func saveProgress() {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	progDir := filepath.Join(cfgDir, "connections")
	if err := os.MkdirAll(progDir, 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(progress, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(progDir, "progress.json"), data, 0644)
}
