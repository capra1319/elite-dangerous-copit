//go:build desktop

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDesktopPermanentControlsAndAutomaticShipUtilities(t *testing.T) {
	a := layoutTestApp(t)
	permanent := []string{"t1", "t2", "t3", "t4", "b1", "b2", "b3", "b4"}
	if !reflect.DeepEqual(a.GetState(100).ControlLayout, permanent) {
		t.Fatal("universal controls are missing")
	}
	if err := a.SaveControlLayout([]string{}); err != nil || !reflect.DeepEqual(a.currentControlLayout(), permanent) {
		t.Fatal("permanent controls could be removed", err)
	}
	a.model.mock = nil
	a.model.opts.mode = "control"
	a.model.companion.Ship = shipOverview{Type: "krait_mkii", ID: 1, ModulesAt: testNow, Modules: []shipModule{{Item: "hpt_heatsinklauncher_turret_tiny"}}}
	p := a.prefs
	p.AutoAddShipControls = true
	p.ScanlineSpacing = 16
	if err := a.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.currentControlLayout(), append(append([]string{}, permanent...), "b5")) {
		t.Fatal("automatic add missed fitted launcher")
	}
	if len(a.model.requests) != 0 {
		t.Fatal("automatic layout sent flight input")
	}
	path := filepath.Join(filepath.Dir(a.settingsPath), "cockpit-layout.json")
	before, _ := os.ReadFile(path)
	a.syncAutomaticControls()
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("unchanged loadout rewrote layout")
	}
	a.model.companion.Ship = shipOverview{Type: "sidewinder", ID: 2, ModulesAt: testNow.Add(time.Minute), Modules: []shipModule{{Item: "hpt_chafflauncher_turret_tiny"}}}
	a.syncAutomaticControls()
	layout := a.currentControlLayout()
	if len(layout) != 10 || layout[8] != "b5" || layout[9] != "b6" {
		t.Fatal("ship swap failed to retain old and add new utilities", layout)
	}
	if err := a.SaveControlLayout(append(append([]string{}, permanent...), "b5")); err == nil {
		t.Fatal("automatic available control was silently removed")
	}
	loaded, err := loadDesktopPreferences(a.settingsPath)
	if err != nil || !loaded.AutoAddShipControls || loaded.ScanlineSpacing != 16 || loaded.Fullscreen {
		t.Fatal("new preferences did not persist safely", loaded, err)
	}
	for _, spacing := range []int{3, 5, 26} {
		bad := a.prefs
		bad.ScanlineSpacing = spacing
		if err := a.SavePreferences(bad); err == nil {
			t.Fatal("invalid scan-line pitch accepted", spacing)
		}
	}
}

func TestDesktopGraphicalPagesExposeStructuredSanitizedData(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock"}, testNow)
	m.companion = mockCompanionData(testNow)
	m.companion.Commander = "Commander\x1b]2;unsafe\a"
	for _, page := range []int{pageShip, pageNavigation, pageMissions, pageHardpoints, pageDiagnostics, pageHelp} {
		m.page = page
		view := m.desktopPageView()
		if len(view.Panels) == 0 {
			t.Fatal("graphical page is empty", page)
		}
		for _, panel := range view.Panels {
			for _, field := range panel.Fields {
				if strings.ContainsAny(field.Value, "\x1b\a") || strings.Contains(field.Value, "unsafe") {
					t.Fatal("terminal sequence escaped to a graphical field")
				}
			}
			for _, row := range panel.Rows {
				for _, cell := range row {
					if strings.ContainsAny(cell, "\x1b\a") {
						t.Fatal("unsanitized table data")
					}
				}
			}
		}
	}
	m.page = pageNavigation
	m.companion.Nav.Route = make([]routePoint, 300)
	for i := range m.companion.Nav.Route {
		m.companion.Nav.Route[i] = routePoint{StarSystem: "Example", StarClass: "G"}
	}
	if len(m.desktopPageView().Panels[1].Rows) != 100 {
		t.Fatal("route rendering is not bounded")
	}
	a := layoutTestApp(t)
	a.model.page = pageMissions
	state := a.GetState(100)
	if state.Text != "" || len(state.PageView.Panels) == 0 {
		t.Fatal("desktop still renders an ASCII menu")
	}
}

func TestDesktopFireGroupSelectorsOnlyEditValidatedReferences(t *testing.T) {
	a := layoutTestApp(t)
	m := hardpointTestModel(t, filepath.Join(t.TempDir(), "references.json"))
	m.opts.mode = "mock"
	a.model = m
	a.model.page = pageHardpoints
	view := a.GetState(100).PageView
	if len(view.Hardpoints) == 0 {
		t.Fatal("no structured hardpoint rows")
	}
	row := view.Hardpoints[0]
	if err := a.SetFireGroupReference(row.Key, 2, 1); err != nil {
		t.Fatal(err)
	}
	updated := a.GetState(100).PageView.Hardpoints[0]
	if updated.Groups[2] != 1 || row.Groups[2] != -1 {
		t.Fatal("reference did not update or aliased the old snapshot")
	}
	if err := a.SetFireGroupReference("unknown-module", 2, 1); err == nil {
		t.Fatal("unknown module accepted")
	}
	if err := a.SetFireGroupReference(row.Key, 8, 1); err == nil {
		t.Fatal("out-of-range group accepted")
	}
	if err := a.SetFireGroupReference(row.Key, 0, 3); err == nil {
		t.Fatal("invalid trigger accepted")
	}
	if len(a.model.requests) != 0 {
		t.Fatal("reference selector sent a flight action")
	}
	if _, err := os.Stat(m.opts.firegroups); !os.IsNotExist(err) {
		t.Fatal("mock reference saved to the real file")
	}
}
