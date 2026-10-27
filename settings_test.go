package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestAppearanceSettingsPersistenceAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences", "settings.json")
	if index, err := loadUISettings(path); err != nil || index != 0 {
		t.Fatal("first launch should use Command Deck", index, err)
	}
	for i := range tuiThemes {
		if err := saveUISettings(path, i); err != nil {
			t.Fatal(err)
		}
		m := newCockpit(cockpitOptions{mode: "live", settings: path}, testNow)
		if m.themeChoice != i || m.savedTheme != i || m.settingsMessage != "" || m.mock != nil {
			t.Fatal("relaunch failed to restore appearance independently of mode")
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveUISettings(path, -1); err == nil {
		t.Fatal("unknown theme accepted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("invalid selection changed saved preferences")
	}
	for _, invalid := range []string{"{", `{"version":1,"theme":"missing"}`, `{"version":2,"theme":"command-deck"}`, `{"version":1,"theme":"command-deck"} {}`, strings.Repeat(" ", 4097)} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		m := newCockpit(cockpitOptions{mode: "live", settings: path}, testNow)
		if m.themeChoice != 0 || !strings.Contains(m.settingsMessage, "Could not load") || !m.scanning() {
			t.Fatal("bad preferences must keep real scanning available with an honest error")
		}
	}
}

func TestSettingsPreviewCancelApplyAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	m := newCockpit(cockpitOptions{mode: "mock", controls: "numpad", settings: path}, testNow)
	other := newCockpit(cockpitOptions{mode: "mock"}, testNow)
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF5})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.page != pageSettings || m.themeChoice != 1 || m.savedTheme != 0 || other.themeChoice != 0 || other.palette().ID == m.palette().ID {
		t.Fatal("theme preview must be model-local and unsaved")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyKp1})
	if m.mock.State.has(gear) {
		t.Fatal("settings input triggered a simulated flight control")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("preview wrote preferences")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.page != pageCockpit || m.themeChoice != 0 {
		t.Fatal("Escape must cancel the preview")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF5})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.themeChoice != 2 {
		t.Fatal("theme list must wrap to Retro Operations")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF5})
	if m.page != pageCockpit || m.themeChoice != 2 || m.savedTheme != 2 {
		t.Fatal("saved theme must remain active after leaving settings")
	}
	// The same saved appearance is loaded by fresh live/control/mock sessions.
	for _, mode := range []string{"control", "live", "mock"} {
		fresh := newCockpit(cockpitOptions{mode: mode, settings: path}, testNow)
		if fresh.themeChoice != 2 || fresh.frame.Mode != mode {
			t.Fatal("appearance preferences changed the requested mode")
		}
	}
}

func TestSettingsSaveFailureAndAlternateNavigation(t *testing.T) {
	// A directory cannot be replaced by a JSON file on any supported platform.
	m := newCockpit(cockpitOptions{mode: "live", settings: t.TempDir()}, testNow)
	for i := 0; i < pagePosition(pageSettings); i++ {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.settingsMessage, "Could not save") || m.savedTheme != 0 {
		t.Fatal("save failure must not claim the preview was saved")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Could not save") {
		t.Fatal("save failure must remain visible in the footer")
	}
	for _, key := range []rune{tea.KeyTab, tea.KeyF1, tea.KeyF5, tea.KeyEscape} {
		m.page, m.themeChoice = pageSettings, 1
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: key})
		if m.themeChoice != m.savedTheme || m.page == pageSettings {
			t.Fatal("leaving settings must discard unsaved previews")
		}
	}
}

func TestAllThemePagesFitAndKeepFeedback(t *testing.T) {
	for selection := range tuiThemes {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
			for page := pageCockpit; page < pageCount; page++ {
				m := newCockpit(cockpitOptions{mode: "control", controls: "numpad"}, testNow)
				m.themeChoice, m.savedTheme = selection, selection
				m.page, m.width, m.height = page, size[0], size[1]
				// Verify both scanner and connected cockpit states.
				for _, connected := range []bool{false, true} {
					if connected {
						m.frame.State = cloneSnapshot(&newMock(testNow).State)
						m.presence = gamePresence{Checked: true, Known: true, Running: true}
						m.updateConnection()
					}
					view := m.View().Content
					rows := strings.Split(view, "\n")
					if len(rows) > size[1] {
						t.Fatalf("%s %v page %d exceeds terminal height", m.palette().Name, size, page)
					}
					for _, row := range rows {
						if ansi.StringWidth(row) > size[0] {
							t.Fatal("themed page wraps terminal rows")
						}
					}
					plain := ansi.Strip(view)
					for _, label := range []string{"DISARMED", "Ctrl+Alt+F10"} {
						if !strings.Contains(plain, label) {
							t.Fatalf("%s page %d: missing %s", m.palette().Name, page, label)
						}
					}
					if page == pageSettings {
						if strings.Contains(plain, "Enlarge this window") {
							t.Fatal("settings must fit without clipping a panel")
						}
						for _, theme := range tuiThemes {
							if !strings.Contains(plain, theme.Name) {
								t.Fatal("theme choices hidden on a small terminal")
							}
						}
					}
				}
			}
		}
	}
}
