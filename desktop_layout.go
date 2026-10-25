//go:build desktop

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type desktopControlLayout struct {
	Version int      `json:"version"`
	Actions []string `json:"actions"`
}

func defaultControlLayout() desktopControlLayout {
	result := desktopControlLayout{Version: 1, Actions: []string{}}
	return result
}

func validateControlLayout(layout desktopControlLayout) error {
	if layout.Version != 1 || layout.Actions == nil || len(layout.Actions) > len(liveActions) {
		return errors.New("unsupported cockpit layout")
	}
	seen := map[string]bool{}
	for _, id := range layout.Actions {
		if seen[id] {
			return errors.New("an action may only appear once")
		}
		known := false
		for _, action := range liveActions {
			if action.ID == id {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unsupported cockpit action: %q", safeText(id))
		}
		seen[id] = true
	}
	return nil
}

func loadControlLayout(path string) (desktopControlLayout, error) {
	defaults := defaultControlLayout()
	data, err := readSmallFile(path, 4096)
	if err != nil {
		if os.IsNotExist(err) {
			return defaults, nil
		}
		return defaults, err
	}
	var result desktopControlLayout
	if err := json.Unmarshal(data, &result); err != nil {
		return defaults, err
	}
	if err := validateControlLayout(result); err != nil {
		return defaults, err
	}
	return result, nil
}

func (a *DesktopApp) currentControlLayout() []string {
	if a.layout.Version != 1 {
		return permanentControlLayout(nil)
	}
	return permanentControlLayout(a.layout.Actions)
}

// Saving a display layout cannot alter game bindings or queue flight input.
func (a *DesktopApp) SaveControlLayout(actions []string) error {
	layout := desktopControlLayout{Version: 1, Actions: append([]string{}, actions...)}
	if err := validateControlLayout(layout); err != nil {
		return err
	}
	layout.Actions = permanentControlLayout(layout.Actions)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return errors.New("application is closing")
	}
	previous := map[string]bool{}
	for _, id := range a.currentControlLayout() {
		previous[id] = true
	}
	for _, id := range layout.Actions {
		if !previous[id] && !shipControlAvailable(a.model.utilityAvailability(id)) {
			return errors.New("a selected action is not available in the current reported ship loadout")
		}
	}
	if a.prefs.AutoAddShipControls {
		kept := map[string]bool{}
		for _, id := range layout.Actions {
			kept[id] = true
		}
		for _, id := range a.currentControlLayout() {
			if !permanentShipControl(id) && shipControlAvailable(a.model.utilityAvailability(id)) && !kept[id] {
				return errors.New("turn off automatic ship controls before removing an available utility")
			}
		}
	}
	if a.settingsPath == "" {
		return errors.New("cockpit layout location is unavailable")
	}
	if err := writeJSONAtomic(filepath.Join(filepath.Dir(a.settingsPath), "cockpit-layout.json"), layout); err != nil {
		return err
	}
	a.layout, a.layoutError = layout, ""
	return nil
}

func (m cockpitModel) utilityAvailability(id string) string {
	if m.mock != nil {
		return "SIMULATED"
	}
	if m.companion.Ship.Type == "" {
		return "SHIP UNKNOWN"
	}
	var module string
	switch id {
	case "b5":
		module = "hpt_heatsinklauncher_"
	case "b6":
		module = "hpt_chafflauncher_"
	default:
		return "BUILT IN"
	}
	if m.companion.Ship.ModulesAt.IsZero() {
		return "LOADOUT UNKNOWN"
	}
	for _, fitted := range m.companion.Ship.Modules {
		if strings.HasPrefix(strings.ToLower(fitted.Item), module) {
			return "IN LAST LOADOUT"
		}
	}
	return "NOT IN LAST LOADOUT"
}

func shipControlAvailable(availability string) bool {
	return availability == "BUILT IN" || availability == "IN LAST LOADOUT" || availability == "SIMULATED"
}

func controlDescription(id string) string {
	switch id {
	case "t1":
		return "Deploy or retract landing gear."
	case "t2":
		return "Deploy or retract the cargo scoop."
	case "t3":
		return "Switch ship lights."
	case "t4":
		return "Deploy or retract hardpoints."
	case "b1":
		return "Switch night vision."
	case "b2":
		return "Open or close the galaxy map."
	case "b3":
		return "Open or close the system map."
	case "b4":
		return "Cycle to the next fire group."
	case "b5":
		return "Request one heat sink deployment; requires a fitted launcher."
	case "b6":
		return "Request one chaff deployment; requires a fitted launcher."
	}
	return ""
}

func permanentShipControl(id string) bool {
	switch id {
	case "t1", "t2", "t3", "t4", "b1", "b2", "b3", "b4":
		return true
	}
	return false
}
func permanentControlLayout(extra []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, action := range liveActions {
		if permanentShipControl(action.ID) {
			result = append(result, action.ID)
			seen[action.ID] = true
		}
	}
	for _, id := range extra {
		if !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	return result
}

// Called under the existing model lock, only for journal updates or an explicit preference save.
func (a *DesktopApp) syncAutomaticControls() {
	if !a.prefs.AutoAddShipControls || a.closing || a.model.mock != nil {
		return
	}
	current := a.currentControlLayout()
	next := append([]string{}, current...)
	seen := map[string]bool{}
	for _, id := range current {
		seen[id] = true
	}
	for _, action := range liveActions {
		if !permanentShipControl(action.ID) && !seen[action.ID] && shipControlAvailable(a.model.utilityAvailability(action.ID)) {
			next = append(next, action.ID)
			seen[action.ID] = true
		}
	}
	if len(next) == len(current) {
		return
	}
	attempt := fmt.Sprintf("%s/%d/%d/%s", a.model.companion.Ship.Type, a.model.companion.Ship.ID, a.model.companion.Ship.ModulesAt.UnixNano(), strings.Join(next, ","))
	if a.autoLayoutAttempt == attempt {
		return
	}
	a.autoLayoutAttempt = attempt
	if a.settingsPath == "" {
		a.layoutError = "Cockpit layout location is unavailable"
		return
	}
	layout := desktopControlLayout{Version: 1, Actions: next}
	if err := writeJSONAtomic(filepath.Join(filepath.Dir(a.settingsPath), "cockpit-layout.json"), layout); err != nil {
		a.layoutError = "Could not automatically add ship controls: " + err.Error()
		return
	}
	a.layout, a.layoutError = layout, ""
}
