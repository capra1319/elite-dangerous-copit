//go:build desktop

package main

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHealthPreferencePersistsAndDefaultsToShip(t *testing.T) {
	if defaultDesktopPreferences().HealthDisplay != "ship" {
		t.Fatal("wrong default")
	}
	var p DesktopPreferences
	if err := decodeDesktopPreferences([]byte(`{"version":1,"theme":"command-deck","layout":"auto","fullscreen":true}`), &p); err != nil || p.HealthDisplay != "ship" || p.Fullscreen {
		t.Fatal(p, err)
	}
	path := filepath.Join(t.TempDir(), "prefs.json")
	p.HealthDisplay = "bars"
	p.Fullscreen = true
	if err := writeDesktopPreferences(path, p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "fullscreen") {
		t.Fatal("session window state persisted")
	}
	loaded, err := loadDesktopPreferences(path)
	if err != nil || loaded.HealthDisplay != "bars" {
		t.Fatal(loaded, err)
	}
	p.HealthDisplay = "invalid"
	if validDesktopPreferences(p) {
		t.Fatal("invalid display accepted")
	}
}

func TestHealthRequiresFreshMainShipTelemetryAndKeepsMissingUnknown(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "control", stale: time.Minute}, testNow)
	m.presence = gamePresence{Checked: true, Known: true, Running: true}
	m.frame.State = cloneSnapshot(&newMock(testNow).State)
	m.frame.State.Flags = u64(mainShip)
	m.updateConnection()
	hull := .61
	m.companion.Ship.Hull = &hull
	result := m.desktopHealth()
	if !result.Offline || result.ShieldPercent == nil || *result.ShieldPercent != 0 || *result.HullPercent != 61 {
		t.Fatal(result)
	}
	*result.HullPercent = 99
	if hull != .61 {
		t.Fatal("view aliases source")
	}
	for _, invalidate := range []func(*cockpitModel){
		func(m *cockpitModel) { m.now = testNow.Add(2 * time.Minute) },
		func(m *cockpitModel) { m.frame.State.Timestamp = testNow.Add(time.Hour) },
		func(m *cockpitModel) { m.frame.Quality = "READ ERROR" },
		func(m *cockpitModel) { m.frame.State = nil },
		func(m *cockpitModel) { m.frame.State.Flags = u64(fighter) },
		func(m *cockpitModel) { m.presence.Running = false },
		func(m *cockpitModel) { m.fatal = errors.New("closed") },
	} {
		other := m
		other.frame.State = cloneSnapshot(m.frame.State)
		invalidate(&other)
		result := other.desktopHealth()
		if result.Offline || result.ShieldPercent != nil || result.ShieldState != "UNKNOWN" {
			t.Fatal("invalid telemetry retained shield warning", result)
		}
	}
	m.frame.State.Flags = u64(mainShip | shields)
	result = m.desktopHealth()
	if result.Offline || result.ShieldPercent != nil || result.ShieldState != "UP" {
		t.Fatal("invented percentage", result)
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), -1, 1.01} {
		m.companion.Ship.Hull = &value
		if m.desktopHealth().HullPercent != nil {
			t.Fatal("invalid hull accepted")
		}
	}
	zero := 0.
	m.companion.Ship.Hull = &zero
	if result = m.desktopHealth(); result.HullPercent == nil || *result.HullPercent != 0 {
		t.Fatal("zero hull lost")
	}
	m.opts.mode = "mock"
	if result = m.desktopHealth(); result.ShieldPercent == nil || *result.ShieldPercent != 82 || !strings.Contains(result.Source, "SIMULATED") {
		t.Fatal("mock not marked", result)
	}
}

func TestCockpitRouteUsesReportedRouteOrLabelsTargetFallback(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock"}, testNow)
	m.companion.Nav = navigationData{System: "Origin", Position: []float64{0, 0, 0}, RouteAt: testNow, Route: []routePoint{{StarSystem: "Origin", StarPos: []float64{0, 0, 0}}, {StarSystem: "Next", StarPos: []float64{1, 1, 1}}, {StarSystem: "Final", StarPos: []float64{3, 4, 0}}}}
	route := m.desktopRoute()
	if !route.Available || route.Destination != "Final" || route.Next != "Next" || route.Remaining != "2" || route.Distance != "5.00 LY · direct" {
		t.Fatal(route)
	}
	m.companion.Nav.Position = nil
	if m.desktopRoute().Distance != "Not reported" {
		t.Fatal("missing coordinates used")
	}
	m.companion.Nav.Route = nil
	m.companion.Nav.Target = "Target"
	route = m.desktopRoute()
	if route.Destination != "Target" || !strings.Contains(route.Source, "final route destination is not known") {
		t.Fatal(route)
	}
	m.companion.Nav.Target = ""
	if m.desktopRoute().Available {
		t.Fatal("invented route")
	}
	if len(m.requests) != 0 {
		t.Fatal("view sent commands")
	}
	// The DTO remains valid JSON with all unknown measurements.
	if _, err := json.Marshal(m.desktopHealth()); err != nil {
		t.Fatal(err)
	}
}
