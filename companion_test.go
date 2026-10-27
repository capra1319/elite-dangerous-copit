package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFeaturePageShortcutsAndFlightInputIsolation(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "control", controls: "numpad"}, testNow)
	requests := make(chan controlRequest, 4)
	m.requests = requests
	for i, code := range []rune{tea.KeyF6, tea.KeyF7, tea.KeyF8, tea.KeyF9, tea.KeyF10} {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: code})
		if m.page != pageShip+i || m.scroll != 0 {
			t.Fatal("feature shortcut opened wrong page")
		}
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyKp1})
		if m.frame.State != nil || len(requests) != 0 {
			t.Fatal("feature page sent or invented flight input")
		}
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.page != pageCockpit {
		t.Fatal("Esc must return to cockpit")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.page != pageHelp {
		t.Fatal("reverse Tab must reach last page")
	}
	for i := 0; i < pageCount; i++ {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	if m.page != pageHelp {
		t.Fatal("Tab cycle does not include every page")
	}
}

func TestPopulatedFeaturePagesFitAllThemesAndSanitizeJournalText(t *testing.T) {
	for theme := range tuiThemes {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
			m := newCockpit(cockpitOptions{mode: "control", controls: "numpad"}, testNow)
			m.themeChoice, m.savedTheme, m.width, m.height = theme, theme, size[0], size[1]
			m.companion = mockCompanionData(testNow)
			m.companion.Commander = "Commander\x1b]2;malicious-title\a\nInjected"
			m.goals["raw/arsenic"] = 20
			for page := pageShip; page <= pageHardpoints; page++ {
				m.page = page
				view := m.View().Content
				if strings.Contains(view, "malicious-title") || strings.Contains(view, "\x1b]2;") {
					t.Fatal("journal text escaped terminal cells")
				}
				rows := strings.Split(view, "\n")
				if len(rows) > size[1] {
					t.Fatal("feature page exceeds terminal height")
				}
				for _, row := range rows {
					if ansi.StringWidth(row) > size[0] {
						t.Fatal("feature page wraps terminal rows")
					}
				}
				if !strings.Contains(ansi.Strip(view), "DISARMED") || !strings.Contains(ansi.Strip(view), "Ctrl+Alt+F10") {
					t.Fatal("feature page hid control status or stop key")
				}
			}
		}
	}
}

func TestNavigationAndMissionViewsDoNotInventMissingData(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "live"}, testNow)
	if view := ansi.Strip(m.navigationPage(100)); !strings.Contains(view, "unavailable") || !strings.Contains(view, "No plotted route") {
		t.Fatal("missing navigation data must remain unknown")
	}
	if view := ansi.Strip(m.missionBoardPage(100)); !strings.Contains(view, "summary not yet seen") {
		t.Fatal("no journal must not imply an empty authoritative mission list")
	}
	m.companion.MissionsKnown = true
	if !strings.Contains(ansi.Strip(m.missionBoardPage(100)), "No active missions reported") {
		t.Fatal("known empty mission list not shown")
	}
	m.companion = mockCompanionData(testNow)
	if view := ansi.Strip(m.navigationPage(100)); !strings.Contains(view, "18.60 LY") || !strings.Contains(view, "Scoopable class") {
		t.Fatal("route coordinates/target class not displayed")
	}
	if view := ansi.Strip(m.missionBoardPage(100)); !strings.Contains(view, "24 / 36 delivered") {
		t.Fatal("reported delivery progress not displayed")
	}
}

func TestEngineeringSearchTargetSaveProgressAndUnpin(t *testing.T) {
	dir := t.TempDir()
	goalsPath, settingsPath := filepath.Join(dir, "goals.json"), filepath.Join(dir, "settings.json")
	if err := saveUISettings(settingsPath, 2); err != nil {
		t.Fatal(err)
	}
	settingsBefore, _ := os.ReadFile(settingsPath)
	m := newCockpit(cockpitOptions{mode: "live", controls: "commands", settings: settingsPath, goals: goalsPath}, testNow)
	m.companion = mockCompanionData(testNow)
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF9})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: '/'})
	m = updateCockpit(t, m, tea.PasteMsg{Content: "arsenic"})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if rows := m.materialRows(); len(rows) != 1 || rows[0].Name != "arsenic" {
		t.Fatal("material search did not find selected symbol")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: '2', Text: "20"})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.goalEditing || m.goals["raw/arsenic"] != 20 || !strings.Contains(ansi.Strip(m.engineeringPage(80)), "12 / 20 / 8") {
		t.Fatal("target save/progress failed", m.goalMessage)
	}
	fresh := newCockpit(cockpitOptions{mode: "live", goals: goalsPath}, testNow)
	if fresh.goals["raw/arsenic"] != 20 {
		t.Fatal("engineering targets not restored on relaunch")
	}
	if view := ansi.Strip(fresh.engineeringPage(80)); !strings.Contains(view, "? / 20 / ?") {
		t.Fatal("unknown inventory invented a zero count or completed goal")
	}
	applyRecord(t, &m.companion, `{"event":"MaterialCollected","Category":"Raw","Name":"arsenic","Count":8}`)
	if !strings.Contains(ansi.Strip(m.engineeringPage(80)), "[✓]") {
		t.Fatal("reported inventory did not complete checklist target")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: '0', Text: "0"})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.goalEditing || m.goals["raw/arsenic"] != 20 {
		t.Fatal("invalid target changed saved goal")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyDelete})
	if len(m.goals) != 0 {
		t.Fatal("Delete did not unpin the material")
	}
	if saved, err := loadMaterialGoals(goalsPath); err != nil || len(saved) != 0 {
		t.Fatal("unpin did not persist", err)
	}
	settingsAfter, _ := os.ReadFile(settingsPath)
	if string(settingsAfter) != string(settingsBefore) {
		t.Fatal("engineering preferences overwrote the theme")
	}
}

func TestMaterialCatalogueAndTargetFailures(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "live", goals: t.TempDir()}, testNow)
	if len(materialCatalogue) < 100 {
		t.Fatal("offline material catalogue missing")
	}
	m.materialFilter = "Datamined Wake Exceptions"
	rows := m.materialRows()
	if len(rows) != 1 || rows[0].Key != "encoded/dataminedwake" {
		t.Fatal("display name not mapped to journal material symbol")
	}
	m.saveTarget(rows[0].Key, 20)
	if len(m.goals) != 0 || !strings.Contains(m.goalMessage, "Could not save") {
		t.Fatal("write failure claimed goal was saved")
	}
	for _, value := range []string{`{"version":2,"targets":{}}`, `{"version":1,"targets":{"raw/arsenic":0}}`, `{"version":1,"targets":{"bad/arsenic":20}}`, `{"version":1,"targets":{"raw/arsenic":20}} {}`} {
		path := filepath.Join(t.TempDir(), "goals.json")
		writeFixture(t, path, value)
		if _, err := loadMaterialGoals(path); err == nil {
			t.Fatal("invalid goals accepted", value)
		}
	}
}
