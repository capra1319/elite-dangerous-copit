package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type modulePowerInfo struct {
	Slot, Item string
	Power      *float64
	Priority   *int
}

func modulePowerKey(slot, item string) string {
	return strings.ToLower(strings.TrimSpace(slot)) + "/" + strings.ToLower(strings.TrimSpace(item))
}

// ModulesInfo has no ship ID. Require a report from this session and loadout,
// and reject fitted-module mismatches instead of attaching another ship's data.
func (r *journalReader) readModulePower() error {
	s := &r.data.Ship
	if s.ModulesAt.IsZero() || len(s.Modules) == 0 {
		return nil
	}
	fit := fmt.Sprintf("%s/%d/%d/%d", s.Type, s.ID, s.ModulesAt.UnixNano(), r.data.SessionStart.UnixNano())
	path := filepath.Join(r.dir, "ModulesInfo.json")
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		s.ModulesPower, s.PowerInfoAt = nil, time.Time{}
		r.powerInfo = nil
		return nil
	}
	if err != nil {
		return err
	}
	if !s.PowerInfoAt.IsZero() && r.powerInfo != nil && r.powerFit == fit && info.Size() == r.powerInfo.Size() && info.ModTime() == r.powerInfo.ModTime() {
		return nil
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	data, err := readSmallFile(path, 1<<20)
	if err != nil {
		return err
	}
	var event struct {
		Timestamp time.Time
		Event     string
		Modules   []modulePowerInfo
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("module power report incomplete: %w", err)
	}
	if event.Event != "ModuleInfo" || event.Timestamp.IsZero() || len(event.Modules) == 0 || len(event.Modules) > 512 || event.Timestamp.After(time.Now().Add(futureSnapshotTolerance)) {
		return errors.New("invalid module power report")
	}
	if event.Timestamp.Before(s.ModulesAt) || event.Timestamp.Before(r.data.SessionStart) {
		r.powerInfo, r.powerFit = info, fit
		return nil
	}
	fitted := map[string]bool{}
	for _, module := range s.Modules {
		fitted[modulePowerKey(module.Slot, module.Item)] = true
	}
	seen := map[string]bool{}
	rows := []modulePowerInfo{}
	for _, module := range event.Modules {
		key := modulePowerKey(module.Slot, module.Item)
		if module.Slot == "" || module.Item == "" || len(module.Slot) > 128 || len(module.Item) > 256 || seen[key] {
			return errors.New("invalid module identity in power report")
		}
		seen[key] = true
		if module.Power != nil && (math.IsNaN(*module.Power) || math.IsInf(*module.Power, 0) || *module.Power < 0 || *module.Power > 1e6) || module.Priority != nil && (*module.Priority < 0 || *module.Priority > 4) {
			return errors.New("invalid module draw or priority")
		}
		if !fitted[key] {
			// The auxiliary file can also contain these implicit built-in modules.
			if strings.EqualFold(module.Slot, "ShipCockpit") || strings.EqualFold(module.Slot, "CargoHatch") || strings.EqualFold(module.Slot, "DataLinkScanner") {
				continue
			}
			return errors.New("module power report does not match this loadout")
		}
		rows = append(rows, module)
	}
	if len(rows) == 0 {
		return errors.New("no matching modules in power report")
	}
	s.ModulesPower, s.PowerInfoAt = rows, event.Timestamp
	r.powerInfo, r.powerFit = info, fit
	return nil
}

func modulePriority(priority *int) string {
	if priority == nil || *priority < 0 || *priority > 4 {
		return "Unknown"
	}
	return fmt.Sprint(*priority + 1)
}

func moduleRowsWithPower(ship shipOverview) []shipModuleView {
	rows := shipModuleRows(ship.Modules)
	for i, module := range ship.Modules {
		rows[i].Priority = modulePriority(module.Priority)
		rows[i].Draw, rows[i].Supply, rows[i].ConfiguredPower = "Unknown", "Not reported", "Unknown"
		if module.On || module.OnKnown {
			rows[i].ConfiguredPower = "OFF"
			if module.On {
				rows[i].ConfiguredPower = "ON"
			}
		}
		if ship.PowerInfoAt.IsZero() || ship.PowerInfoAt.Before(ship.ModulesAt) {
			continue
		}
		for _, power := range ship.ModulesPower {
			if modulePowerKey(module.Slot, module.Item) != modulePowerKey(power.Slot, power.Item) {
				continue
			}
			if power.Power != nil && !math.IsNaN(*power.Power) && !math.IsInf(*power.Power, 0) && *power.Power >= 0 && *power.Power <= 1e6 {
				rows[i].Draw = fmt.Sprintf("%.2f MW", *power.Power)
			}
			rows[i].Priority = modulePriority(power.Priority)
			break
		}
	}
	return rows
}
