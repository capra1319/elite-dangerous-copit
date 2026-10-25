//go:build desktop

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopControlColumnsPersistAndLegacySettingsRemainCompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	a := newDesktopApp(defaultDesktopPreferences(), path)
	initialStatus := a.model.frame.ControlStatus
	for _, columns := range []int{0, 1, 3, 5, 10} {
		p := a.prefs
		p.ControlColumns = columns
		if err := a.SavePreferences(p); err != nil {
			t.Fatal(err)
		}
		loaded, err := loadDesktopPreferences(path)
		if err != nil || loaded.ControlColumns != columns || loaded.Theme != p.Theme || loaded.Layout != p.Layout {
			t.Fatal("row width did not survive a restart", loaded, err)
		}
		if a.model.frame.ControlStatus != initialStatus || len(a.model.requests) != 0 {
			t.Fatal("appearance preference affected controls")
		}
	}
	legacy := []byte("{\"version\":1,\"theme\":\"flight-console\",\"layout\":\"portrait\",\"scanlines\":true}")
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDesktopPreferences(path)
	if err != nil || loaded.ControlColumns != 0 || loaded.Theme != "flight-console" || loaded.Layout != "portrait" || !loaded.Scanlines {
		t.Fatal("existing appearance settings were lost", loaded, err)
	}
}

func TestDesktopControlColumnsRejectInvalidValuesWithoutSaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	a := newDesktopApp(defaultDesktopPreferences(), path)
	if err := a.SavePreferences(a.prefs); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	for _, columns := range []int{-1, 11, 1000} {
		p := a.prefs
		p.ControlColumns = columns
		if err := a.SavePreferences(p); err == nil {
			t.Fatal("invalid columns accepted", columns)
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(before) || a.prefs.ControlColumns != 0 {
			t.Fatal("rejected column count changed settings")
		}
	}
	var decoded DesktopPreferences
	if err := decodeDesktopPreferences([]byte("{\"version\":1,\"theme\":\"command-deck\",\"layout\":\"auto\",\"controlColumns\":2.5}"), &decoded); err == nil {
		t.Fatal("fractional row width accepted")
	}
}
