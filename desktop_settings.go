//go:build desktop

package main

import (
	"encoding/json"
	"errors"
)

func decodeDesktopPreferences(data []byte, p *DesktopPreferences) error {
	if err := json.Unmarshal(data, p); err != nil {
		return err
	}
	// Ignore fullscreen saved by older versions; every launch is windowed.
	p.Fullscreen = false
	if p.UIScale == 0 {
		p.UIScale = 100
	}
	if p.HealthDisplay == "" {
		p.HealthDisplay = "ship"
	}
	if p.ScanlineSpacing == 0 {
		p.ScanlineSpacing = 12
	}
	if !validDesktopPreferences(*p) {
		return errors.New("unsupported display preferences")
	}
	return nil
}

// Only durable appearance choices go on disk. Fullscreen remains in the
// runtime preferences DTO because the frontend needs its current value.
func writeDesktopPreferences(path string, p DesktopPreferences) error {
	stored := struct {
		UIScale             int    `json:"uiScale"`
		Version             int    `json:"version"`
		Theme               string `json:"theme"`
		Layout              string `json:"layout"`
		Scanlines           bool   `json:"scanlines"`
		ControlColumns      int    `json:"controlColumns"`
		AutoAddShipControls bool   `json:"autoAddShipControls"`
		ScanlineSpacing     int    `json:"scanlineSpacing"`
		HealthDisplay       string `json:"healthDisplay"`
	}{p.UIScale, p.Version, p.Theme, p.Layout, p.Scanlines, p.ControlColumns, p.AutoAddShipControls, p.ScanlineSpacing, p.HealthDisplay}
	return writeJSONAtomic(path, stored)
}
