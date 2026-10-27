//go:build desktop

package main

import (
	"strings"
	"testing"
)

func TestDesktopShipWarningSnapshotIsReadOnlyAndDoesNotDuplicateText(t *testing.T) {
	model := newCockpit(cockpitOptions{mode: "mock"}, testNow)
	model.companion = mockCompanionData(testNow)
	model.page = pageShip
	a := &DesktopApp{model: model, prefs: defaultDesktopPreferences()}
	state := a.GetState(100)
	if len(state.ShipModules) == 0 || state.ShipModulesAt == "not reported" {
		t.Fatal("missing structured module report")
	}
	if strings.Contains(state.Text, "INSTALLED MODULES") || strings.Contains(state.Text, "KEY SYSTEM DAMAGED") {
		t.Fatal("module report duplicated in plain readout")
	}
	if state.ShipModules[0].Condition != "critical" || state.ShipModules[1].Condition != "ok" || state.ShipModules[3].Condition != "unknown" {
		t.Fatalf("warning states were lost: %+v", state.ShipModules)
	}
	*state.ShipModules[0].HealthPercent = 0
	if *a.model.companion.Ship.Modules[0].Health != .87 || len(a.model.requests) != 0 {
		t.Fatal("read-only warning view affected telemetry or controls")
	}
	a.model.page = pageNavigation
	if len(a.GetState(100).ShipModules) != 0 {
		t.Fatal("hidden module view unnecessarily published on another page")
	}
}
