package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func powerReader(t *testing.T) *journalReader {
	t.Helper()
	r := newJournalReader(t.TempDir())
	r.data.SessionStart = testNow.Add(-time.Minute)
	applyRecord(t, &r.data, `{"event":"Loadout","Ship":"sidewinder","ShipID":1,"Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1","On":true,"Priority":0},{"Slot":"MediumHardpoint1","Item":"hpt_test","On":false,"Priority":4}]}`)
	return r
}
func powerReport(t *testing.T, r *journalReader, body string) {
	t.Helper()
	writeFixture(t, filepath.Join(r.dir, "ModulesInfo.json"), journalRecord(t, body))
}

func TestModulePowerReportMatchesFitAndDoesNotInventSupply(t *testing.T) {
	r := powerReader(t)
	powerReport(t, r, `{"event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1","Power":0,"Priority":0},{"Slot":"MediumHardpoint1","Item":"hpt_test","Power":0.52,"Priority":4}]}`)
	if err := r.readModulePower(); err != nil {
		t.Fatal(err)
	}
	rows := moduleRowsWithPower(r.data.Ship)
	if len(rows) != 2 || rows[0].Draw != "0.00 MW" || rows[0].Priority != "1" || rows[1].Priority != "5" || rows[1].Draw != "0.52 MW" || rows[1].ConfiguredPower != "OFF" || rows[0].ConfiguredPower != "ON" {
		t.Fatal(rows)
	}
	for _, row := range rows {
		if row.Supply != "Not reported" {
			t.Fatal("inferred supply or overdraw", row)
		}
	}
	// Repeated polls keep the immutable report without workers or file handles.
	if err := r.readModulePower(); err != nil {
		t.Fatal(err)
	}
	clone := r.data.clone()
	clone.Ship.ModulesPower[0].Item = "changed"
	if r.data.Ship.ModulesPower[0].Item == "changed" {
		t.Fatal("snapshot aliases slice")
	}
	if err := os.Remove(filepath.Join(r.dir, "ModulesInfo.json")); err != nil {
		t.Fatal("reader kept file open", err)
	}
	if err := r.readModulePower(); err != nil || !r.data.Ship.PowerInfoAt.IsZero() {
		t.Fatal("missing file retained data", err)
	}
}

func TestModulePowerRejectsBadFilesAndRetainsLastMatchingReport(t *testing.T) {
	r := powerReader(t)
	valid := `{"event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1","Power":0,"Priority":0}]}`
	powerReport(t, r, valid)
	if err := r.readModulePower(); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"event":`,
		`{"event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"wrong_ship","Power":1}]}`,
		`{"event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1","Power":-1}]}`,
		`{"event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1","Priority":5}]}`,
		`{"event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1"},{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1"}]}`,
		`{"event":"ModuleInfo","Modules":[]}`,
	} {
		if strings.HasSuffix(body, `:`) {
			writeFixture(t, filepath.Join(r.dir, "ModulesInfo.json"), body)
		} else {
			powerReport(t, r, body)
		}
		if err := r.readModulePower(); err == nil {
			t.Fatal("bad report accepted", body)
		}
		if len(r.data.Ship.ModulesPower) != 1 {
			t.Fatal("partial write replaced last matching report")
		}
	}
	writeFixture(t, filepath.Join(r.dir, "ModulesInfo.json"), strings.Repeat(" ", (1<<20)+1))
	if err := r.readModulePower(); err == nil {
		t.Fatal("oversized report accepted")
	}
	powerReport(t, r, valid)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.ctx = ctx
	if err := r.readModulePower(); err != context.Canceled {
		t.Fatal("cancellation ignored", err)
	}
}

func TestModulePowerRejectsOldSessionAndShipSwap(t *testing.T) {
	r := powerReader(t)
	powerReport(t, r, `{"timestamp":"`+testNow.Add(-time.Hour).Format(time.RFC3339)+`","event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1","Power":0,"Priority":0}]}`)
	if err := r.readModulePower(); err != nil || len(r.data.Ship.ModulesPower) != 0 {
		t.Fatal("old snapshot used", err)
	}
	powerReport(t, r, `{"event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1","Power":0,"Priority":0}]}`)
	if err := r.readModulePower(); err != nil {
		t.Fatal(err)
	}
	applyRecord(t, &r.data, `{"timestamp":"`+testNow.Add(time.Second).Format(time.RFC3339)+`","event":"Loadout","Ship":"cobra","ShipID":2,"Modules":[{"Slot":"PowerPlant","Item":"int_other","On":true}]}`)
	if len(r.data.Ship.ModulesPower) != 0 || !r.data.Ship.PowerInfoAt.IsZero() {
		t.Fatal("ship swap kept old draw")
	}
	if err := r.readModulePower(); err != nil || len(r.data.Ship.ModulesPower) != 0 {
		t.Fatal("old fit reattached", err)
	}
	var m shipModule
	if err := json.Unmarshal([]byte(`{"Slot":"x","Item":"y"}`), &m); err != nil || m.OnKnown {
		t.Fatal(m, err)
	}
	if row := moduleRowsWithPower(shipOverview{Modules: []shipModule{m}})[0]; row.ConfiguredPower != "Unknown" || row.Priority != "Unknown" || row.Draw != "Unknown" {
		t.Fatal("missing fields treated as zero", row)
	}
}

func TestModulePowerCacheRepopulatesAfterSameTimestampLoadout(t *testing.T) {
	r := powerReader(t)
	powerReport(t, r, `{"event":"ModuleInfo","Modules":[{"Slot":"PowerPlant","Item":"int_powerplant_size2_class1","Power":0,"Priority":0}]}`)
	if err := r.readModulePower(); err != nil {
		t.Fatal(err)
	}
	r.data.Ship.ModulesPower = nil
	r.data.Ship.PowerInfoAt = time.Time{}
	if err := r.readModulePower(); err != nil || len(r.data.Ship.ModulesPower) != 1 {
		t.Fatal("cached file not reapplied after repeated same-second Loadout", err)
	}
	applyRecord(t, &r.data, `{"timestamp":"`+testNow.Add(time.Minute).Format(time.RFC3339)+`","event":"LoadGame","Ship":"sidewinder","ShipID":1}`)
	if !r.data.Ship.PowerInfoAt.IsZero() || len(r.data.Ship.ModulesPower) != 0 {
		t.Fatal("session restart retained power report")
	}
	if err := r.readModulePower(); err != nil || len(r.data.Ship.ModulesPower) != 0 {
		t.Fatal("earlier-session power reattached", err)
	}
}
