package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCockpitMockToggleRequiresDeliberateF4(t *testing.T) {
	for _, mode := range []string{"control", "live", "mock"} {
		m := newCockpit(cockpitOptions{mode: mode, controls: "numpad"}, testNow)
		for _, key := range []tea.KeyPressMsg{{Code: tea.KeyF4, IsRepeat: true}, {Code: tea.KeyF4, Mod: tea.ModCtrl}} {
			m = updateCockpit(t, m, key)
			if m.switchMode {
				t.Fatal("repeated or modified F4 must not change modes")
			}
		}
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyF4})
		if !next.(cockpitModel).switchMode || cmd == nil || cmd() != tea.Quit() {
			t.Fatal("F4 must end the session so its backend can stop")
		}
		if mode != "mock" && next.(cockpitModel).mock != nil {
			t.Fatal("mock started before the real backend was stopped")
		}
	}
}

func TestCockpitModeSessionsRestoreRealOptionsAndDiscardSimulation(t *testing.T) {
	for _, startMode := range []string{"control", "live", "mock"} {
		opts := cockpitOptions{mode: startMode, controls: "commands", path: "status-path", config: "config-path", interval: time.Second, stale: time.Minute}
		if startMode == "control" {
			opts.controls = "numpad"
		}
		var modes []string
		err := runCockpitSessions(context.Background(), opts, func(current cockpitOptions) (cockpitModel, error) {
			modes = append(modes, current.mode)
			if current.path != opts.path || current.config != opts.config || current.interval != opts.interval || current.stale != opts.stale {
				t.Fatal("mode switch lost connection settings")
			}
			m := newCockpit(current, testNow)
			if current.mode == "mock" {
				m.mockAction("t1 on")
				m.opts.controls = "commands" // F3 must not change real input options.
			} else if m.mock != nil || m.frame.State != nil || m.frame.ControlStatus != "DISARMED" || !m.scanning() {
				t.Fatal("return to real mode must discard mock state, disarm and scan afresh")
			}
			if current.mode == "control" && current.controls != "numpad" {
				t.Fatal("return to control must use numpad")
			}
			if current.mode == "live" && current.controls != opts.controls {
				t.Fatal("observation settings were not restored")
			}
			m.switchMode = len(modes) < 3
			return m, nil
		})
		want := startMode + ",mock," + startMode
		if startMode == "mock" {
			want = "mock,control,mock"
		}
		if err != nil || strings.Join(modes, ",") != want {
			t.Fatalf("session transitions: %v, %v", modes, err)
		}
	}
}

func TestCockpitModeSwitchStopsOnBackendShutdownFailure(t *testing.T) {
	want := errors.New("backend did not stop")
	calls := 0
	err := runCockpitSessions(context.Background(), cockpitOptions{mode: "control"}, func(cockpitOptions) (cockpitModel, error) {
		calls++
		return cockpitModel{switchMode: true}, want
	})
	if !errors.Is(err, want) || calls != 1 {
		t.Fatal("must not start mock while the real backend may still be running")
	}
}

func TestMockToggleVisibleWhileScanningAndSimulating(t *testing.T) {
	for _, mode := range []string{"control", "live", "mock"} {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}} {
			m := newCockpit(cockpitOptions{mode: mode}, testNow)
			m.width, m.height = size[0], size[1]
			label := "F4 mock ON"
			if mode == "mock" {
				label = "F4 mock OFF"
			}
			if !strings.Contains(ansi.Strip(m.View().Content), label) {
				t.Fatalf("%s %v: mock toggle hidden", mode, size)
			}
		}
	}
}

func updateCockpit(t *testing.T, m cockpitModel, msg tea.Msg) cockpitModel {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(cockpitModel)
}

func TestCockpitFitsTerminalAndKeepsControlFeedbackVisible(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {80, 24}, {100, 30}, {110, 36}, {140, 42}} {
		m := newCockpit(cockpitOptions{mode: "control", controls: "numpad"}, testNow)
		m.width, m.height = size[0], size[1]
		m.frame.State = cloneSnapshot(&newMock(testNow).State)
		m.presence = gamePresence{Checked: true, Known: true, Running: true}
		m.updateConnection()
		m.frame.Action = "Lights confirmed"
		view := m.View()
		rows := strings.Split(view.Content, "\n")
		if len(rows) > size[1] {
			t.Fatalf("%v: frame scrolls (%d rows)", size, len(rows))
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > size[0] {
				t.Fatalf("%v: row wraps: %q", size, ansi.Strip(row))
			}
		}
		plain := ansi.Strip(view.Content)
		for _, want := range []string{"DISARMED", "Landing gear", "Hardpoints", "NUM 1", "NUM 0", "Lights confirmed", "Ctrl+Alt+F9", "Ctrl+Alt+F10"} {
			if !strings.Contains(strings.ToLower(plain), strings.ToLower(want)) {
				t.Fatalf("%v: important control hidden: %s\n%s", size, want, plain)
			}
		}
		if !view.AltScreen {
			t.Fatal("dashboard should use its own terminal screen")
		}
	}
}

func TestCockpitUnknownAndStaleDataAreExplicit(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "live"}, testNow)
	m.width, m.height = 120, 40
	s := newMock(testNow).State
	s.Flags = u64(0)
	m.frame.State = &s
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "SCANNING") || !strings.Contains(view, "Load your commander") {
		t.Fatal("no-ship data should wait for a session rather than imply systems are off")
	}
	m.presence = gamePresence{Checked: true, Known: true, Running: true}
	m.frame.State = cloneSnapshot(&newMock(testNow).State)
	m.updateConnection()
	s = newMock(testNow.Add(-2 * time.Minute)).State
	m.frame.State = &s
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "STALE DATA") || !strings.Contains(view, "LAST KNOWN") {
		t.Fatal("old state should be clearly marked")
	}
}

func TestCockpitMockNumpadUsesPhysicalKeysAndIgnoresHolds(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock", controls: "numpad"}, testNow)
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: '1', Text: "1"})
	if m.mock.State.has(gear) {
		t.Fatal("top-row key triggered numpad action")
	}
	key := tea.KeyPressMsg{Code: '1', BaseCode: tea.KeyKp1, Text: "1"}
	m = updateCockpit(t, m, key)
	if !m.mock.State.has(gear) {
		t.Fatal("numpad key did not deploy gear")
	}
	m = updateCockpit(t, m, key)
	if !m.mock.State.has(gear) {
		t.Fatal("held key repeated")
	}
	m = updateCockpit(t, m, tea.KeyReleaseMsg{Code: '1', BaseCode: tea.KeyKp1})
	m = updateCockpit(t, m, key)
	if m.mock.State.has(gear) {
		t.Fatal("released/repressed key did not toggle")
	}
}

func TestCockpitNightVisionIsGroupedWithTelemetryToggles(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
		m := newCockpit(cockpitOptions{mode: "mock", controls: "numpad"}, testNow)
		m.width, m.height = size[0], size[1]
		view := strings.ToUpper(ansi.Strip(m.View().Content))
		at, visibility, combat := strings.Index(view, "NIGHT VISION"), strings.Index(view, "VISIBILITY"), strings.Index(view, "COMBAT")
		if at <= visibility || visibility < 0 || at >= combat || strings.Count(view, "NIGHT VISION") != 1 {
			t.Fatalf("%v: night vision should appear once in the visibility category", size)
		}
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyKp5})
		state, _ := m.primaryState(4)
		if !m.mock.State.has(nightVision) || state != "● ACTIVE" {
			t.Fatal("Num5 should toggle night vision with matching telemetry state")
		}
		m.frame.Pending = "b1"
		if !strings.Contains(ansi.Strip(m.toggleRow(4, 27, true)), "ACTIVE · WAIT") {
			t.Fatal("night vision toggle should show pending feedback")
		}
	}
}

func TestCockpitTypedCommandsScenarioAndInputSwitch(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock", controls: "commands"}, testNow)
	for _, r := range "t3 on" {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.mock.State.has(lights) || len(m.command) != 0 {
		t.Fatal("typed controls no longer work")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF3})
	if m.opts.controls != "numpad" {
		t.Fatal("alternate input type not selected")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF2})
	if !m.mock.State.has(docked) {
		t.Fatal("scenario key did not select docked")
	}
}

func TestCockpitRealGameInputIsOwnedByGuardedBackend(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "control", controls: "numpad"}, testNow)
	requests := make(chan string, 2)
	m.requests = requests
	m.frame.State = cloneSnapshot(&newMock(testNow).State)
	m.frame.ControlStatus = "ARMED"
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyKp1})
	if m.frame.State.has(gear) || len(requests) != 0 {
		t.Fatal("UI key directly changed/sent real-game state")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if <-requests != "disarm" || m.frame.ControlStatus != "ARMED" {
		t.Fatal("UI should request disarm and wait for backend confirmation")
	}
}

func TestCockpitHelpCanScrollAndReturnToFlight(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock"}, testNow)
	m.width, m.height = 80, 24
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF1})
	initial := m.View().Content
	for i := 0; i < 3; i++ {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	if m.View().Content == initial || !strings.Contains(ansi.Strip(m.View().Content), "consumables") {
		t.Fatal("help cannot reach its later instructions")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.page != 0 {
		t.Fatal("Escape did not return to cockpit")
	}
}

func TestCockpitFrameSnapshotsAreIndependent(t *testing.T) {
	s := newMock(testNow).State
	s.Fuel = &fuelState{Main: 12}
	frame := cloneSnapshot(&s)
	*s.Flags |= gear
	s.Pips[0] = 8
	s.Fuel.Main = 3
	if frame.has(gear) || frame.Pips[0] == 8 || frame.Fuel.Main != 12 {
		t.Fatal("published UI snapshot shares mutable game state")
	}
	c, _ := testEngine(t)
	fuel := newMock(testNow).State
	fuel.Fuel = &fuelState{Main: 23}
	c.update(&fuel, nil, testNow, testTarget())
	baseline := c.Sample
	fuel2 := newMock(testNow).State
	fuel2.Fuel = &fuelState{Main: 22}
	c.update(&fuel2, nil, testNow, testTarget())
	if c.State.Fuel.Main != 22 || c.Sample != baseline {
		t.Fatal("metadata must refresh without inventing a toggle acknowledgement")
	}
}

func TestExportCockpitPreview(t *testing.T) {
	dir := os.Getenv("ELITEAUX_EXPORT_VIEW")
	if dir == "" {
		t.Skip("optional visual preview export")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	m := newCockpit(cockpitOptions{mode: "mock", controls: "numpad"}, testNow)
	m.mock.State.Flags = u64(mainShip | shields | gear | lights | nightVision)
	m.mock.State.Pips = []int{4, 6, 2}
	m.mock.State.Fuel = &fuelState{Main: 26.4, Reservoir: 0.38}
	m.syncMock("Ship lights: matching simulated state observed")
	for _, preview := range []struct {
		name string
		w, h int
	}{{"cockpit-wide", 140, 40}, {"cockpit-compact", 80, 24}} {
		m.width, m.height = preview.w, preview.h
		if err := os.WriteFile(filepath.Join(dir, preview.name+".ansi"), []byte(m.View().Content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scan := newCockpit(cockpitOptions{mode: "control", controls: "numpad"}, testNow)
	scan.width, scan.height = 110, 36
	scan.presence = gamePresence{Checked: true, Known: true}
	scan.frame.Action = "Waiting for Elite Dangerous — no commands sent"
	for i := 0; i < 60; i++ {
		scan.scanFrame = i
		name := fmt.Sprintf("scanner-%02d.ansi", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(scan.View().Content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scan.width, scan.height = 80, 24
	if err := os.WriteFile(filepath.Join(dir, "scanner-compact.ansi"), []byte(scan.View().Content), 0600); err != nil {
		t.Fatal(err)
	}
}
