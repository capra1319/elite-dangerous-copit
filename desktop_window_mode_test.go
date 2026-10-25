//go:build desktop

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopLegacyWindowModeFlagsNeverRestoreFullscreen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	legacy := "{\"version\":1,\"theme\":\"retro-ops\",\"layout\":\"portrait\",\"scanlines\":true,\"controlColumns\":3,\"fullscreen\":true,\"borderless\":true}"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := loadDesktopPreferences(path)
	if err != nil || p.Fullscreen || p.Theme != "retro-ops" || p.Layout != "portrait" || !p.Scanlines || p.ControlColumns != 3 {
		t.Fatal("old window mode was restored or unrelated appearance preferences were lost", p, err)
	}
	p.Fullscreen = true
	a := newDesktopApp(p, path)
	if a.prefs.Fullscreen {
		t.Fatal("new desktop instance inherited fullscreen from its caller")
	}
}

func TestDesktopFullscreenIsRuntimeOnlyEvenWhenAppearanceChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window.json")
	a := newDesktopApp(defaultDesktopPreferences(), path)
	calls := []bool{}
	a.fullscreenWindow = func(value bool) { calls = append(calls, value) }
	if err := a.SetFullscreen(true); err != nil || !a.prefs.Fullscreen {
		t.Fatal("could not enter runtime fullscreen", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("fullscreen toggle wrote preferences")
	}
	p := a.prefs
	p.Theme, p.ControlColumns, p.Scanlines = "flight-console", 4, true
	p.Fullscreen = false // a stale UI save must not alter the actual session mode.
	if err := a.SavePreferences(p); err != nil || !a.prefs.Fullscreen {
		t.Fatal("appearance save changed the session mode", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "fullscreen") || strings.Contains(string(data), "borderless") {
		t.Fatal("window mode leaked to disk", string(data), err)
	}
	reloaded, err := loadDesktopPreferences(path)
	if err != nil || reloaded.Fullscreen || reloaded.Theme != "flight-console" || reloaded.ControlColumns != 4 || !reloaded.Scanlines {
		t.Fatal("session mode restored or appearance was lost", reloaded, err)
	}
	before := string(data)
	if err := a.SetFullscreen(false); err != nil || a.prefs.Fullscreen {
		t.Fatal("could not return to windowed mode", err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != before || len(calls) != 2 || !calls[0] || calls[1] {
		t.Fatal("window mode changed saved preferences or called wrong window action")
	}
}
