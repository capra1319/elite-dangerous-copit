//go:build desktop

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopUIScaleLegacyPersistenceValidationAndClosing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"theme":"command-deck","layout":"auto","fullscreen":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := loadDesktopPreferences(path)
	if err != nil || p.UIScale != 100 || p.Fullscreen {
		t.Fatal("legacy scale default", p, err)
	}
	a := newDesktopApp(p, path)
	for _, scale := range []int{100, 125, 150, 175, 200} {
		p.UIScale = scale
		if err := a.SavePreferences(p); err != nil {
			t.Fatal(err)
		}
		loaded, err := loadDesktopPreferences(path)
		if err != nil || loaded.UIScale != scale || loaded.Fullscreen {
			t.Fatal("saved scale", loaded, err)
		}
	}
	before, _ := os.ReadFile(path)
	for _, scale := range []int{-1, 50, 99, 101, 124, 201, 1000} {
		p.UIScale = scale
		if err := a.SavePreferences(p); err == nil {
			t.Fatal("invalid scale accepted", scale)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("invalid preference changed file")
	}
	p.UIScale = 0
	if err := a.SavePreferences(p); err != nil || a.prefs.UIScale != 100 {
		t.Fatal("old bridge default", err)
	}
	a.closing = true
	p.UIScale = 150
	if err := a.SavePreferences(p); err == nil {
		t.Fatal("closing save accepted")
	}
	if len(a.model.requests) != 0 {
		t.Fatal("appearance sent game input")
	}
	for _, raw := range []string{`{"version":1,"theme":"command-deck","layout":"auto","uiScale":1.5}`, `{"version":1,"theme":"command-deck","layout":"auto","uiScale":true}`} {
		var decoded DesktopPreferences
		if err := decodeDesktopPreferences([]byte(raw), &decoded); err == nil {
			t.Fatal("noninteger scale accepted")
		}
	}
}

func TestDesktopModuleNamesMatchHardpointsAndRetainSourceIdentifier(t *testing.T) {
	module := shipModule{Slot: "MediumHardpoint1", Item: "hpt_pulselaser_fixed_large"}
	rows := shipModuleRows([]shipModule{module})
	if rows[0].Name != hardpointLabel(module) || strings.Contains(rows[0].Name, "hpt_") || rows[0].Item == "" {
		t.Fatal("inconsistent display name", rows)
	}
	module.Item = "hpt_chafflauncher_turret_tiny"
	if !strings.HasPrefix(shipModuleRows([]shipModule{module})[0].Name, "Chaff") {
		t.Fatal("utility display name")
	}
}
