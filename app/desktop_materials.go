//go:build desktop

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type DesktopMaterialRow struct {
	materialRow
	Note   string
	Pinned bool
}

type materialPinsFile struct {
	Version int      `json:"version"`
	Pins    []string `json:"pins"`
}

func loadMaterialPins(path string) (map[string]bool, error) {
	pins := map[string]bool{}
	data, err := readSmallFile(path, 256<<10)
	if os.IsNotExist(err) {
		return pins, nil
	}
	if err != nil {
		return pins, err
	}
	var stored materialPinsFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return pins, err
	}
	if stored.Version != 1 || stored.Pins == nil || len(stored.Pins) > 1000 {
		return pins, errors.New("unsupported material pins file")
	}
	for _, key := range stored.Pins {
		if !validGoal(key, 1) || pins[key] {
			return map[string]bool{}, errors.New("invalid or duplicate material pin")
		}
		pins[key] = true
	}
	return pins, nil
}

// Desktop pins are separate from target quantities and from the terminal's target filter.
// The caller owns a.mu. Snapshot rows contain copied values, never the stored maps.
func (a *DesktopApp) desktopMaterialRows() []DesktopMaterialRow {
	m := a.model
	m.onlyPinned = false
	pins := a.materialPins
	if m.opts.mode == "mock" {
		pins = a.mockMaterialPins
	}
	rows := []DesktopMaterialRow{}
	for _, row := range m.materialRows() {
		pinned := pins[row.Key]
		if a.model.onlyPinned && !pinned {
			continue
		}
		note, _ := a.materialNote(row.Key)
		rows = append(rows, DesktopMaterialRow{materialRow: row, Note: note, Pinned: pinned})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Pinned && !rows[j].Pinned })
	return rows
}

func (a *DesktopApp) SetMaterialPinned(key string, pinned, simulated bool) error {
	if !validGoal(key, 1) {
		return errors.New("invalid material")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return errors.New("application is closing")
	}
	if (a.model.opts.mode == "mock") != simulated {
		return errors.New("mode changed; select the material again")
	}
	if !simulated && a.materialPinsError != "" {
		return errors.New(a.materialPinsError)
	}
	m := a.model
	m.onlyPinned = false
	found := false
	for _, row := range m.materialRows() {
		if row.Key == key {
			found = true
			break
		}
	}
	if !found {
		return errors.New("select a material from the list")
	}
	current := a.materialPins
	if simulated {
		current = a.mockMaterialPins
	}
	next := make(map[string]bool, len(current)+1)
	for id := range current {
		next[id] = true
	}
	if pinned {
		next[key] = true
	} else {
		delete(next, key)
	}
	if len(next) > 1000 {
		return errors.New("too many pinned materials")
	}
	if simulated {
		a.mockMaterialPins = next
		return nil
	}
	if a.settingsPath == "" {
		return errors.New("material pin location is unavailable")
	}
	keys := make([]string, 0, len(next))
	for id := range next {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	if err := writeJSONAtomic(filepath.Join(filepath.Dir(a.settingsPath), "engineering-pins.json"), materialPinsFile{Version: 1, Pins: keys}); err != nil {
		return err
	}
	a.materialPins = next
	return nil
}
