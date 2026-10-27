//go:build desktop

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const materialNoteLimit = 2000
const materialNotesFileLimit = 1 << 20

type materialNotesFile struct {
	Version int               `json:"version"`
	Notes   map[string]string `json:"notes"`
}

func validMaterialNote(key, note string) bool {
	if !validGoal(key, 1) || !utf8.ValidString(note) || utf8.RuneCountInString(note) > materialNoteLimit {
		return false
	}
	for _, r := range note {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}

func loadMaterialNotes(path string) (map[string]string, error) {
	empty := map[string]string{}
	data, err := readSmallFile(path, materialNotesFileLimit)
	if os.IsNotExist(err) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	var stored materialNotesFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return empty, err
	}
	if stored.Version != 1 || stored.Notes == nil || len(stored.Notes) > 1000 {
		return empty, errors.New("unsupported material notes file")
	}
	for key, note := range stored.Notes {
		if !validMaterialNote(key, note) || strings.TrimSpace(note) == "" {
			return empty, errors.New("invalid saved material comment")
		}
	}
	return stored.Notes, nil
}

// The caller owns a.mu; simulator notes never share the real notes map.
func (a *DesktopApp) materialNote(key string) (string, string) {
	if a.model.opts.mode == "mock" {
		return a.mockMaterialNotes[key], ""
	}
	return a.materialNotes[key], a.materialNotesError
}

// Saves plain text only. No game, journal, binding or target quantity is changed.
func (a *DesktopApp) SaveMaterialNote(key, note string, simulated bool) error {
	note = strings.ReplaceAll(note, "\r\n", "\n")
	if !validMaterialNote(key, note) {
		return errors.New("enter a plain-text comment of up to 2000 characters")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return errors.New("application is closing")
	}
	if (a.model.opts.mode == "mock") != simulated {
		return errors.New("mode changed; review the comment and save again")
	}
	if !simulated && a.materialNotesError != "" {
		return errors.New(a.materialNotesError)
	}
	found := false
	for _, row := range a.desktopMaterialRows() {
		if row.Key == key {
			found = true
			break
		}
	}
	if !found {
		return errors.New("select a material from the list")
	}
	current := a.materialNotes
	if simulated {
		current = a.mockMaterialNotes
	}
	next := make(map[string]string, len(current)+1)
	for id, text := range current {
		next[id] = text
	}
	if strings.TrimSpace(note) == "" {
		delete(next, key)
	} else {
		next[key] = note
	}
	stored := materialNotesFile{Version: 1, Notes: next}
	data, err := json.Marshal(stored)
	if err != nil || len(next) > 1000 || len(data) > materialNotesFileLimit {
		return errors.New("material notes exceed the local file limit")
	}
	if simulated {
		a.mockMaterialNotes = next
		return nil
	}
	if a.settingsPath == "" {
		return errors.New("material notes location is unavailable")
	}
	if err := writeJSONAtomic(filepath.Join(filepath.Dir(a.settingsPath), "material-notes.json"), stored); err != nil {
		return err
	}
	a.materialNotes = next
	return nil
}
