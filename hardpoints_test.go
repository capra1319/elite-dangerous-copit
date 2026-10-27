package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func hardpointTestModel(t *testing.T, path string) cockpitModel {
	t.Helper()
	m := newCockpit(cockpitOptions{mode: "live", firegroups: path}, testNow)
	applyRecord(t, &m.companion, `{"event":"Commander","FID":"TEST-FID","Name":"Test commander"}`)
	applyRecord(t, &m.companion, `{"event":"Loadout","Ship":"krait_mkii","ShipID":7,"Modules":[
		{"Slot":"TinyHardpoint1","Item":"hpt_chafflauncher_turret_tiny","On":true},
		{"Slot":"LargeHardpoint1","Item":"hpt_pulselaser_fixed_large","On":true,"Health":0.98,"Priority":0,"AmmoInClip":0,"AmmoInHopper":12},
		{"Slot":"FrameShiftDrive","Item":"hyperdrive","On":true}]}`)
	m.page = pageHardpoints
	return m
}

func TestHardpointsJournalDataAndUnknownFireGroups(t *testing.T) {
	m := hardpointTestModel(t, "")
	modules := m.hardpointModules()
	if len(modules) != 2 || modules[0].Slot != "LargeHardpoint1" || modules[1].Slot != "TinyHardpoint1" {
		t.Fatal("hardpoints and utilities not isolated and ordered by mount size")
	}
	m.frame.State = &Snapshot{Flags: u64(mainShip | hardpoints), FireGroup: integer(2)}
	view := ansi.Strip(m.hardpointsPage(100))
	for _, want := range []string{"group C", "DEPLOYED", "Pulse laser Fixed large", "Chaff", "health 98%", "priority 1", "ammo 0 / 12", "[?]", "Manual reference", "not a live counter"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %s", want, view)
		}
	}
	m.frame.State.Flags = u64(fighter)
	if strings.Contains(ansi.Strip(m.hardpointsPage(100)), "group C") {
		t.Fatal("fighter group presented as mothership group")
	}
	m.hardpointIndex = 1
	if view = ansi.Strip(m.hardpointsPage(100)); !strings.Contains(view, "ammo not reported") {
		t.Fatal("unknown ammo shown as zero")
	}
	stamp := m.companion.Ship.ModulesAt
	applyRecord(t, &m.companion, `{"event":"LoadGame","Ship":"krait_mkii","ShipID":7}`)
	if !m.companion.Ship.ModulesAt.Equal(stamp) {
		t.Fatal("LoadGame refreshed the loadout timestamp")
	}
	applyRecord(t, &m.companion, `{"event":"Loadout","Ship":"federation_fighter","ShipID":2,"Modules":[]}`)
	if len(m.hardpointModules()) != 2 {
		t.Fatal("fighter erased mothership hardpoints")
	}
}

func TestHardpointReferencesPersistAndIsolateShipCommanderAndModule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.json")
	m := hardpointTestModel(t, path)
	requests := make(chan controlRequest, 3)
	m.requests = requests
	for _, code := range []rune{'1', tea.KeyRight, '2', tea.KeyRight, '0'} {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: code})
	}
	if len(requests) != 0 {
		t.Fatal("reference edits sent flight actions")
	}
	fresh := hardpointTestModel(t, path)
	moduleKey := hardpointModuleKey(fresh.hardpointModules()[0])
	groups := fresh.firegroups.Ships[fresh.fireGroupShipKey()][moduleKey]
	if len(groups) != 3 || groups["A"] != 1 || groups["B"] != 2 || groups["C"] != 0 {
		t.Fatal("saved triggers not restored", groups)
	}
	for _, change := range []func(*cockpitModel){
		func(m *cockpitModel) { m.companion.FID = "another commander" },
		func(m *cockpitModel) { m.companion.Ship.ID++ },
		func(m *cockpitModel) { m.companion.Ship.Modules[1].Item = "hpt_beamlaser_fixed_large" },
	} {
		other := hardpointTestModel(t, path)
		change(&other)
		if strings.Contains(ansi.Strip(other.hardpointsPage(100)), "[1]") {
			t.Fatal("another account/ship/module inherited assignments")
		}
	}
	fresh = updateCockpit(t, fresh, tea.KeyPressMsg{Code: tea.KeyDelete})
	if _, exists := fresh.firegroups.Ships[fresh.fireGroupShipKey()][moduleKey]["A"]; exists {
		t.Fatal("Delete did not restore unknown assignment")
	}
	fresh = hardpointTestModel(t, path)
	if _, exists := fresh.firegroups.Ships[fresh.fireGroupShipKey()][moduleKey]["A"]; exists {
		t.Fatal("Delete was not persisted")
	}
}

func TestHardpointReferenceFailuresAndMockIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "groups.json")
	writeFixture(t, path, `{"version":99,"ships":{}}`)
	m := hardpointTestModel(t, path)
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: '1'})
	data, _ := os.ReadFile(path)
	if string(data) != `{"version":99,"ships":{}}` || !m.fireGroupLoadError {
		t.Fatal("bad file overwritten")
	}
	m = hardpointTestModel(t, t.TempDir())
	m.fireGroupLoadError = false // Exercise a save failure after a successful in-memory load.
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: '1'})
	if len(m.firegroups.Ships) != 0 || !strings.Contains(m.fireGroupMessage, "Could not save") {
		t.Fatal("failed save changed reference")
	}
	before, _ := os.ReadFile(path)
	m = hardpointTestModel(t, "")
	m.opts.mode, m.opts.firegroups = "mock", path
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: '1'})
	after, _ := os.ReadFile(path)
	if string(before) != string(after) || !strings.Contains(m.fireGroupMessage, "not saved") {
		t.Fatal("simulation modified real reference file")
	}
	for _, body := range []string{
		`{"version":1,"ships":{"ship":{"module":{"A":3}}}}`,
		`{"version":1,"ships":{"ship":{"module":{"Z":1}}}}`,
		`{"version":1,"ships":{}} {}`,
	} {
		writeFixture(t, path, body)
		if _, err := loadFireGroups(path); err == nil {
			t.Fatal("accepted invalid reference", body)
		}
	}
}

func TestHardpointSelectionAndShortcutsStayInTerminal(t *testing.T) {
	m := hardpointTestModel(t, filepath.Join(t.TempDir(), "groups.json"))
	m.page = pageCockpit
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF10, Mod: tea.ModCtrl | tea.ModAlt})
	if m.page != pageCockpit {
		t.Fatal("stop shortcut opened Hardpoints")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF10})
	if m.page != pageHardpoints {
		t.Fatal("F10 did not open Hardpoints")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.groupIndex != 7 {
		t.Fatal("group selection did not wrap")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.hardpointIndex != 1 {
		t.Fatal("module selection did not clamp")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyKp1})
	if len(m.firegroups.Ships) != 0 {
		t.Fatal("flight numpad key edited reference")
	}
	for theme := range tuiThemes {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
			m.themeChoice, m.width, m.height = theme, size[0], size[1]
			view := ansi.Strip(m.View().Content)
			if !strings.Contains(view, "[H]") || !strings.Contains(view, "ammo not reported") {
				t.Fatalf("selected group/module hidden at %v: %s", size, view)
			}
		}
	}
}
