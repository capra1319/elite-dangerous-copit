//go:build desktop

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopMaterialDetailsRequireExplicitSelectionAndExposeRowComments(t *testing.T) {
	a := layoutTestApp(t)
	a.SelectPage(pageEngineering)
	if view := a.GetState(100).Engineering; view.Selected != "" || view.Note != "" || len(view.Rows) == 0 {
		t.Fatal("details opened without selecting an item", view.Selected)
	}
	a.FilterMaterials("arsenic", false)
	if a.GetState(100).Engineering.Selected != "" {
		t.Fatal("search automatically selected a material")
	}
	a.SelectMaterial("raw/arsenic")
	if err := a.SaveMaterialNote("raw/arsenic", "Long saved comment\nAnother line", true); err != nil {
		t.Fatal(err)
	}
	view := a.GetState(100).Engineering
	if view.Selected != "raw/arsenic" || view.Rows[0].Note != view.Note || view.Rows[0].Pinned {
		t.Fatal("explicit selection or saved comment column failed", view)
	}
	a.SelectMaterial("")
	if a.GetState(100).Engineering.Selected != "" {
		t.Fatal("closing details reselected the first item")
	}
	a.SelectMaterial("raw/arsenic")
	a.FilterMaterials("iron", false)
	if a.GetState(100).Engineering.Selected != "" {
		t.Fatal("filter substituted a different material")
	}
	a.FilterMaterials("arsenic", false)
	a.SelectMaterial("raw/arsenic")
	a.SelectPage(pageShip)
	a.SelectPage(pageEngineering)
	if a.GetState(100).Engineering.Selected != "" {
		t.Fatal("page re-entry opened stale details")
	}
	if len(a.model.requests) != 0 {
		t.Fatal("material selection/comment sent game input")
	}
}

func TestDesktopMaterialPinsAreIndependentPersistedAndModeIsolated(t *testing.T) {
	a := layoutTestApp(t)
	a.model.opts.mode = "control"
	a.SelectPage(pageEngineering)
	key := "raw/arsenic"
	if err := a.SetMaterialPinned(key, true, false); err != nil {
		t.Fatal(err)
	}
	a.FilterMaterials("", true)
	view := a.GetState(100).Engineering
	if len(view.Rows) != 1 || !view.Rows[0].Pinned || view.Rows[0].Target != 0 || view.Selected != "" {
		t.Fatal("pin required a target or opened details", view)
	}
	a.SelectMaterial(key)
	a.model.opts.goals = filepath.Join(t.TempDir(), "goals.json")
	if err := a.SaveMaterialTarget(key, 42); err != nil {
		t.Fatal("pinned-without-target material could not save target", err)
	}
	if err := a.SaveMaterialNote(key, "Pinned reference", false); err != nil {
		t.Fatal("independent pinned row could not save comment", err)
	}
	reopened := newDesktopApp(defaultDesktopPreferences(), a.settingsPath)
	if !reopened.materialPins[key] {
		t.Fatal("pin did not survive relaunch")
	}
	path := filepath.Join(filepath.Dir(a.settingsPath), "engineering-pins.json")
	before, _ := os.ReadFile(path)
	a.model.opts.mode = "mock"
	if len(a.GetState(100).Engineering.Rows) != 0 {
		t.Fatal("real pin leaked into simulator")
	}
	if err := a.SetMaterialPinned(key, true, true); err != nil {
		t.Fatal(err)
	}
	if err := a.SetMaterialPinned(key, false, false); err == nil {
		t.Fatal("mode mismatch allowed real pin change")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("simulator changed saved pins")
	}
	a.model.opts.mode = "control"
	if err := a.SetMaterialPinned(key, false, false); err != nil {
		t.Fatal(err)
	}
	if len(a.GetState(100).Engineering.Rows) != 0 || a.model.goals[key] != 42 || a.materialNotes[key] != "Pinned reference" {
		t.Fatal("unpin removed target/comment or target kept the pin")
	}
	if pins, err := loadMaterialPins(path); err != nil || len(pins) != 0 {
		t.Fatal("unpin did not persist", err)
	}
	if len(a.model.requests) != 0 {
		t.Fatal("pin operation sent game input")
	}
}

func TestDesktopMaterialPinValidationAndFailedWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "engineering-pins.json")
	for _, content := range []string{"{", `{"version":2,"pins":[]}`, `{"version":1,"pins":null}`, `{"version":1,"pins":["../arsenic"]}`, `{"version":1,"pins":["raw/arsenic","raw/arsenic"]}`, strings.Repeat("x", (256<<10)+1)} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if pins, err := loadMaterialPins(path); err == nil || len(pins) != 0 {
			t.Fatal("invalid pin file accepted")
		}
	}
	corrupt, _ := os.ReadFile(path)
	a := newDesktopApp(defaultDesktopPreferences(), filepath.Join(dir, "window.json"))
	a.model = newCockpit(cockpitOptions{mode: "control"}, testNow)
	if a.materialPinsError == "" || a.SetMaterialPinned("raw/arsenic", true, false) == nil {
		t.Fatal("corrupt pins can be overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(corrupt) {
		t.Fatal("invalid pin file was changed")
	}
	a.materialPinsError = ""
	a.settingsPath = filepath.Join(path, "not-a-directory", "window.json")
	if err := a.SetMaterialPinned("raw/arsenic", true, false); err == nil || a.materialPins["raw/arsenic"] {
		t.Fatal("failed write changed pin state", err)
	}
	if err := a.SetMaterialPinned("raw/not_known", true, false); err == nil {
		t.Fatal("unknown material accepted")
	}
	a.closing = true
	if err := a.SetMaterialPinned("raw/arsenic", true, false); err == nil {
		t.Fatal("closing allowed pin writes")
	}
}
