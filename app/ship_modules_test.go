package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func number(value float64) *float64 { return &value }

func TestShipModuleWarningsUseReportedDamageAndKeepUnknownUnknown(t *testing.T) {
	for _, test := range []struct {
		name      string
		health    *float64
		key       bool
		condition string
	}{
		{"healthy key system", number(1), true, "ok"},
		{"minor key damage", number(.9999), true, "critical"},
		{"destroyed key system", number(0), true, "critical"},
		{"healthy ordinary module", number(1), false, "ok"},
		{"damaged ordinary module", number(.72), false, "damaged"},
		{"missing", nil, true, "unknown"},
		{"negative", number(-.1), true, "unknown"},
		{"above one", number(1.1), true, "unknown"},
		{"NaN", number(math.NaN()), true, "unknown"},
		{"infinity", number(math.Inf(1)), true, "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			module := shipModule{Slot: "PowerPlant", Item: "int_powerplant_size4_class5", Health: test.health}
			if !test.key {
				module.Slot, module.Item = "Slot01_Size4", "int_cargorack_size4_class1"
			}
			row := shipModuleRows([]shipModule{module})[0]
			if row.Condition != test.condition || row.KeySystem != test.key || row.Power != "OFF" {
				t.Fatalf("unexpected warning: %+v", row)
			}
			if test.condition == "unknown" && (row.HealthPercent != nil || row.Health != "Unknown") {
				t.Fatalf("invalid integrity must stay unknown: %+v", row)
			}
			if test.name == "minor key damage" && row.Health != "<100%" {
				t.Fatal(row.Health)
			}
			if module.Health != test.health || module.On {
				t.Fatal("presentation changed source telemetry")
			}
		})
	}
}

func TestShipModuleRolesMatchCoreSlotsAndVariantItemsOnly(t *testing.T) {
	for _, module := range []shipModule{
		{Slot: "PowerPlant"}, {Slot: "MainEngines"}, {Slot: "FrameShiftDrive"}, {Slot: "LifeSupport"},
		{Slot: "Cockpit"}, {Slot: "PowerDistributor"}, {Slot: "Radar"},
		{Item: "int_hyperdrive_overcharge_size5_class5"}, {Item: "int_shieldgeneratorfast_size5_class5"},
		{Item: "int_powerplant_guardian_size6_class5"},
	} {
		row := shipModuleRows([]shipModule{module})[0]
		if !row.KeySystem || row.Consequence == "" {
			t.Fatalf("missing role: %+v", module)
		}
	}
	for _, item := range []string{"int_shieldreinforcement_size1_class1", "int_hyperdriveinterdictor_size2_class1", "hpt_shieldbooster_size0_class5"} {
		if shipModuleRows([]shipModule{{Item: item}})[0].KeySystem {
			t.Fatal("over-broad module match", item)
		}
	}
}

func TestUtilitySizeZeroOutfittingRecordIsNotAWeapon(t *testing.T) {
	row := classifyOutfitModule("hpt_shieldbooster_size0_class5", "0A Shield Booster")
	if row.Category != "Utility mounts" || row.Class == nil || *row.Class != 0 {
		t.Fatalf("class-zero utility was listed as a weapon: %+v", row)
	}
}

func TestShipModuleWarningsRemainVisibleAtNarrowTerminalWidths(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock", stale: time.Minute}, time.Now())
	m.companion.Ship.ModulesAt = time.Now()
	m.companion.Ship.Modules = []shipModule{{Slot: "PowerPlant", Item: "int_powerplant_size4_class5", Health: number(.87)}, {Slot: "LifeSupport"}}
	for _, width := range []int{40, 60, 80, 140} {
		view := ansi.Strip(m.shipOverviewPage(width))
		if !strings.Contains(view, "! KEY 87.0%") || !strings.Contains(view, "Unknown") || !strings.Contains(view, "snapshot") {
			t.Fatal("missing warning", view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatalf("view exceeds %d", width)
			}
		}
	}
}
