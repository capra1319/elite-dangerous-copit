//go:build desktop

package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func layoutTestApp(t *testing.T) *DesktopApp {
	t.Helper()
	a := newDesktopApp(defaultDesktopPreferences(), filepath.Join(t.TempDir(), "window.json"))
	a.ctx = context.Background()
	a.opts.settings, a.opts.goals, a.opts.firegroups = "", "", ""
	a.startSession(true)
	t.Cleanup(func() { a.shutdown(context.Background()) })
	return a
}

func TestDesktopLayoutKeepsPermanentControlsAndRejectsInvalidAssignments(t *testing.T) {
	a := layoutTestApp(t)
	if len(a.GetState(100).ControlLayout) != 8 || len(a.GetState(100).Controls) != len(liveActions) {
		t.Fatal("new panel must keep universal controls and the supported action catalogue")
	}
	ids := []string{"t3", "b1", "b5"}
	initial := a.model.frame.ControlStatus
	if err := a.SaveControlLayout(ids); err != nil {
		t.Fatal(err)
	}
	ids[0] = "t4"
	want := permanentControlLayout([]string{"b5"})
	snapshot := a.GetState(100)
	snapshot.ControlLayout[0] = "t2"
	if !reflect.DeepEqual(a.currentControlLayout(), want) {
		t.Fatal("layout aliases caller data")
	}
	restarted := newDesktopApp(defaultDesktopPreferences(), a.settingsPath)
	if !reflect.DeepEqual(restarted.currentControlLayout(), want) {
		t.Fatal("layout was lost on restart")
	}
	path := filepath.Join(filepath.Dir(a.settingsPath), "cockpit-layout.json")
	before, _ := os.ReadFile(path)
	for _, bad := range [][]string{{"t3", "t3"}, {"not-an-action"}, {"F12"}, {"t1", "t2", "t3", "t4", "b1", "b2", "b3", "b4", "b5", "b6", "extra"}} {
		if err := a.SaveControlLayout(bad); err == nil {
			t.Fatal("unsafe/duplicate assignment accepted", bad)
		}
		after, _ := os.ReadFile(path)
		if string(after) != string(before) || !reflect.DeepEqual(a.currentControlLayout(), want) {
			t.Fatal("rejected layout changed saved state")
		}
	}
	if a.model.frame.ControlStatus != initial || len(a.model.requests) != 0 {
		t.Fatal("layout editing affected controls")
	}
	if err := a.SaveControlLayout([]string{}); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadControlLayout(path)
	if err != nil || loaded.Actions == nil || !reflect.DeepEqual(loaded.Actions, permanentControlLayout(nil)) {
		t.Fatal("optional reset lost permanent controls", loaded, err)
	}
}

func TestDesktopLayoutSurvivesShipSwapAndUnavailableActionsCannotRun(t *testing.T) {
	a := layoutTestApp(t)
	a.model.mock = nil
	a.model.opts.mode = "control"
	a.model.frame.ControlStatus = "PAUSED"
	a.model.companion.Ship = shipOverview{Type: "krait_mkii", ModulesAt: testNow, Modules: []shipModule{{Item: "hpt_heatsinklauncher_turret_tiny"}}}
	if err := a.SaveControlLayout([]string{"b5", "t3"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(a.settingsPath), "cockpit-layout.json")
	before, _ := os.ReadFile(path)
	a.model.companion.Ship = shipOverview{Type: "sidewinder", ModulesAt: testNow.Add(time.Minute), Modules: []shipModule{{Item: "hpt_chafflauncher_turret_tiny"}}}
	view := a.GetState(100)
	if !reflect.DeepEqual(view.ControlLayout, permanentControlLayout([]string{"b5"})) {
		t.Fatal("ship swap removed an assigned action")
	}
	for _, control := range view.Controls {
		if control.ID == "b5" && (control.Availability != "NOT IN LAST LOADOUT" || control.Enabled) {
			t.Fatal("missing launcher is still available", control)
		}
		if control.ID == "b1" && control.Kind != "toggle" {
			t.Fatal("night vision was classified as a trigger")
		}
		if control.ID == "b6" && (control.Kind != "trigger" || control.Availability != "IN LAST LOADOUT") {
			t.Fatal("fitted utility was not recognised")
		}
	}
	if err := a.PressControl("b5"); err == nil || len(a.model.requests) != 0 {
		t.Fatal("unavailable utility queued an action")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("ship swap silently rewrote the saved panel")
	}
	if err := a.SaveControlLayout([]string{"t3", "b5", "b6"}); err != nil {
		t.Fatal("could not keep unavailable controls while adding a fitted one", err)
	}
	if err := a.SaveControlLayout([]string{"t3", "b6"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveControlLayout([]string{"t3", "b6", "b5"}); err == nil {
		t.Fatal("checklist could add a launcher absent from the current ship")
	}
	a.model.companion.Ship = shipOverview{Type: "krait_mkii", ModulesAt: testNow.Add(2 * time.Minute), Modules: []shipModule{{Item: "hpt_heatsinklauncher_turret_tiny"}}}
	if !shipControlAvailable(a.model.utilityAvailability("b5")) {
		t.Fatal("launcher did not become available after returning to its ship")
	}
	a.model.companion.Ship = shipOverview{Type: "krait_mkii"}
	if a.model.utilityAvailability("b5") != "LOADOUT UNKNOWN" {
		t.Fatal("unknown loadout was presented as absent")
	}
	a.model.companion.Ship = shipOverview{}
	if a.model.utilityAvailability("t3") != "SHIP UNKNOWN" {
		t.Fatal("missing ship was presented as known")
	}
}

func TestDesktopLayoutCorruptionWriteFailureAndCloseKeepSafeState(t *testing.T) {
	a := layoutTestApp(t)
	path := filepath.Join(filepath.Dir(a.settingsPath), "cockpit-layout.json")
	for _, bad := range []string{"{}", "{\"version\":1,\"actions\":null}", "{\"version\":2,\"actions\":[]}", "{\"version\":1,\"actions\":[\"b5\",\"b5\"]}", "{\"version\":1,\"actions\":[\"arbitrary-key\"]}", "[] trailing", strings.Repeat("x", 4097)} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err := loadControlLayout(path)
		if err == nil || len(loaded.Actions) != 0 {
			t.Fatal("corrupt layout must fail visibly to an empty panel")
		}
	}
	restarted := newDesktopApp(defaultDesktopPreferences(), a.settingsPath)
	if restarted.layoutError == "" || len(restarted.currentControlLayout()) != 8 {
		t.Fatal("corrupt-file error hidden")
	}
	if err := a.SaveControlLayout([]string{"t3"}); err != nil {
		t.Fatal(err)
	}
	before := a.currentControlLayout()
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	a.settingsPath = filepath.Join(blocker, "window.json")
	if err := a.SaveControlLayout([]string{}); err == nil || !reflect.DeepEqual(a.currentControlLayout(), before) {
		t.Fatal("write failure changed layout")
	}
	a.closing = true
	if err := a.SaveControlLayout([]string{}); err == nil || !reflect.DeepEqual(a.currentControlLayout(), before) {
		t.Fatal("closed app changed layout")
	}
}
