//go:build desktop

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterialCommentsPersistAndRemainIsolatedFromSimulator(t *testing.T) {
	a := layoutTestApp(t)
	a.model.opts.mode = "control"
	a.model.page = pageEngineering
	a.FilterMaterials("arsenic", false)
	a.SelectMaterial("raw/arsenic")
	key, note := "raw/arsenic", "G5 FSD reminder\nCollect more near home — α"
	target := a.GetState(100).Engineering.Rows[0].Target
	if err := a.SaveMaterialNote(key, note, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(a.settingsPath), "material-notes.json")
	before, _ := os.ReadFile(path)
	view := a.GetState(100).Engineering
	if view.Note != note || view.NotesError != "" || view.Rows[0].Target != target || len(a.model.requests) != 0 {
		t.Fatal("comment changed target/input state or did not display", view)
	}
	reopened := newDesktopApp(defaultDesktopPreferences(), a.settingsPath)
	if reopened.materialNotes[key] != note {
		t.Fatal("comment did not survive relaunch")
	}
	a.model.companion.Ship = shipOverview{Type: "sidewinder", ID: 99}
	if a.GetState(100).Engineering.Note != note {
		t.Fatal("ship swap lost material comment")
	}
	a.model.opts.mode = "mock"
	if a.GetState(100).Engineering.Note != "" {
		t.Fatal("simulator exposed real comments")
	}
	if err := a.SaveMaterialNote(key, "Simulation only", true); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveMaterialNote(key, "queued real save", false); err == nil {
		t.Fatal("mode change allowed a real save")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) || a.GetState(100).Engineering.Note != "Simulation only" {
		t.Fatal("simulator wrote real file or lost its in-memory comment")
	}
	a.model.opts.mode = "control"
	if a.GetState(100).Engineering.Note != note {
		t.Fatal("returning from simulator lost real comment")
	}
	if err := a.SaveMaterialNote(key, "", false); err != nil {
		t.Fatal(err)
	}
	stored, err := loadMaterialNotes(path)
	if err != nil || len(stored) != 0 {
		t.Fatal("clear comment did not persist", err)
	}
}

func TestMaterialCommentsRejectInvalidInputAndPreserveOnWriteFailure(t *testing.T) {
	a := layoutTestApp(t)
	a.model.opts.mode = "control"
	key := "raw/arsenic"
	if err := a.SaveMaterialNote(key, "Keep this\r\nSecond line", false); err != nil || a.materialNotes[key] != "Keep this\nSecond line" {
		t.Fatal("newline normalisation failed", err)
	}
	for _, bad := range []struct{ key, note string }{
		{key, strings.Repeat("a", 2001)}, {key, "\x1b[2J"}, {key, "bad\x00note"}, {key, string([]byte{0xff})}, {"raw/../arsenic", "bad"}, {"raw/not_a_known_material", "bad"},
	} {
		if err := a.SaveMaterialNote(bad.key, bad.note, false); err == nil {
			t.Fatal("invalid comment accepted", bad.key)
		}
	}
	blocker := filepath.Join(t.TempDir(), "file-not-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	a.settingsPath = filepath.Join(blocker, "window.json")
	if err := a.SaveMaterialNote(key, "Lost write", false); err == nil || a.materialNotes[key] != "Keep this\nSecond line" {
		t.Fatal("failed write replaced in-memory comment", err)
	}
	a.closing = true
	if err := a.SaveMaterialNote(key, "After close", false); err == nil {
		t.Fatal("closing accepted note write")
	}
}

func TestMaterialCommentFileValidationAndCorruptionProtection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "material-notes.json")
	for _, contents := range []string{
		"{", `{"version":2,"notes":{}}`, `{"version":1,"notes":null}`, `{"version":1,"notes":{"raw/arsenic":""}}`,
		`{"version":1,"notes":{"raw/arsenic":"bad\u001b"}}`, strings.Repeat("x", materialNotesFileLimit+1),
	} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if notes, err := loadMaterialNotes(path); err == nil || len(notes) != 0 {
			t.Fatal("invalid notes file accepted")
		}
	}
	contents := []byte("{broken saved notes")
	if err := os.WriteFile(path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	a := newDesktopApp(defaultDesktopPreferences(), filepath.Join(filepath.Dir(path), "window.json"))
	a.model = newCockpit(cockpitOptions{mode: "control"}, testNow)
	if a.materialNotesError == "" || a.SaveMaterialNote("raw/arsenic", "overwrite", false) == nil {
		t.Fatal("corrupt notes file can be overwritten silently")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(contents) {
		t.Fatal("corrupt source file was changed")
	}
	a.model.opts.mode = "mock"
	if err := a.SaveMaterialNote("raw/arsenic", "Independent simulator note", true); err != nil {
		t.Fatal("real-file error leaked into simulator", err)
	}
	// Preserve valid comments for future/unknown journal materials without inventing recipes.
	data, _ := json.Marshal(materialNotesFile{Version: 1, Notes: map[string]string{"raw/future_material": "User supplied"}})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if notes, err := loadMaterialNotes(path); err != nil || notes["raw/future_material"] != "User supplied" {
		t.Fatal("valid future-material note lost", err)
	}
}
