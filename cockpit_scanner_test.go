package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestScannerNeedsGameAndCurrentSessionButNotFocus(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "control"}, time.Now())
	if !m.scanning() || !m.scanTickPending {
		t.Fatal("startup should scan")
	}
	s := newMock(time.Now()).State
	m = updateCockpit(t, m, dashboardFrame{State: &s})
	if !m.scanning() {
		t.Fatal("cached file should not bypass the initial game-process check")
	}
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Known: true})
	if !m.scanning() {
		t.Fatal("fresh cached file should not connect when game is absent")
	}
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Known: true, Running: true})
	if m.scanning() || m.frame.GameFocused {
		t.Fatal("game in background with usable telemetry should open cockpit")
	}
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Known: true})
	if !m.scanning() {
		t.Fatal("game exit should restore scanner even with a cached snapshot")
	}
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Known: true, Running: true})
	if m.scanning() {
		t.Fatal("game restart should recover automatically")
	}
}

func TestScannerRejectsStaleStartupButAllowsQuietConnectedSession(t *testing.T) {
	now := time.Now()
	m := newCockpit(cockpitOptions{mode: "live"}, now)
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Known: true, Running: true})
	s := newMock(now.Add(-2 * time.Minute)).State
	m = updateCockpit(t, m, dashboardFrame{State: &s})
	if !m.scanning() || !strings.Contains(ansi.Strip(m.View().Content), "Cached snapshot") {
		t.Fatal("old initial file must wait for a new snapshot")
	}
	s = newMock(time.Now()).State
	m = updateCockpit(t, m, dashboardFrame{State: &s})
	m = updateCockpit(t, m, clockMsg(time.Now().Add(2*time.Minute)))
	if m.scanning() || !strings.Contains(ansi.Strip(m.View().Content), "STALE DATA") {
		t.Fatal("quiet unchanged data should be labelled stale, not treated as a lost process")
	}
}

func TestScannerHandlesReadRecoveryUnknownContextsAndClockMismatch(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "live"}, time.Now())
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Error: "process inspection unavailable"})
	for _, context := range []string{"flight", "srv", "foot"} {
		mock := newMock(time.Now())
		if err := mock.scenario(context, time.Now()); err != nil {
			t.Fatal(err)
		}
		m = updateCockpit(t, m, dashboardFrame{State: cloneSnapshot(&mock.State)})
		if m.scanning() {
			t.Fatalf("%s should connect through telemetry fallback", context)
		}
		m = updateCockpit(t, m, dashboardFrame{State: cloneSnapshot(&mock.State), Quality: "READ ERROR: partial JSON"})
		if !m.scanning() {
			t.Fatal("read error should wait for recovery")
		}
	}
	s := newMock(time.Now().Add(time.Minute)).State
	m = updateCockpit(t, m, dashboardFrame{State: &s})
	if !m.scanning() || !strings.Contains(ansi.Strip(m.View().Content), "clock mismatch") {
		t.Fatal("future telemetry must not open cockpit")
	}
	s = newMock(time.Now()).State
	s.Flags = u64(0)
	m = updateCockpit(t, m, dashboardFrame{State: &s})
	if !m.scanning() {
		t.Fatal("main menu / unknown session should scan")
	}
}

func TestScannerAnimationStopsInCockpitAndRestartsOnDisconnect(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "live"}, time.Now())
	before := m.View().Content
	next, cmd := m.Update(scanMsg{})
	m = next.(cockpitModel)
	if cmd == nil || !m.scanTickPending || m.View().Content == before {
		t.Fatal("waiting screen should animate and schedule exactly one next tick")
	}
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Known: true, Running: true})
	s := newMock(time.Now()).State
	m = updateCockpit(t, m, dashboardFrame{State: &s})
	next, cmd = m.Update(scanMsg{})
	m = next.(cockpitModel)
	if cmd != nil || m.scanTickPending {
		t.Fatal("animation should stop once connected")
	}
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Known: true})
	if !m.scanTickPending {
		t.Fatal("disconnect should schedule a new animation tick")
	}
	m = updateCockpit(t, m, backendDoneMsg{err: errors.New("test startup error")})
	next, cmd = m.Update(scanMsg{})
	m = next.(cockpitModel)
	if cmd != nil || m.scanning() || !strings.Contains(ansi.Strip(m.View().Content), "CONNECTION STOPPED") {
		t.Fatal("fatal error should show an actionable error, not spin forever")
	}
	if newCockpit(cockpitOptions{mode: "mock"}, time.Now()).scanning() {
		t.Fatal("mock mode must not wait for the real game")
	}
}

func TestScannerFitsTerminalAndPreservesNavigationAndStop(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
		m := newCockpit(cockpitOptions{mode: "control"}, time.Now())
		m.width, m.height = size[0], size[1]
		view := m.View().Content
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("scanner exceeds terminal height")
		}
		for _, row := range strings.Split(view, "\n") {
			if ansi.StringWidth(row) > size[0] {
				t.Fatal("scanner wraps terminal rows")
			}
		}
		for _, label := range []string{"SCANNING", "Ctrl+Alt+F10", "Tab page"} {
			if !strings.Contains(ansi.Strip(view), label) {
				t.Fatalf("%v: hidden instruction %q", size, label)
			}
		}
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.page != pageShip || strings.Contains(ansi.Strip(m.View().Content), "SCANNING FOR") {
			t.Fatal("flight pages must remain accessible while scanning")
		}
	}
}

func TestGamePresenceMatchesOnlyGameExecutables(t *testing.T) {
	for _, name := range []string{"EliteDangerous64.exe", "elitedangerous.EXE"} {
		if !isGameExecutable(name) {
			t.Fatal("game executable was rejected")
		}
	}
	for _, name := range []string{"EDLaunch.exe", "notEliteDangerous64.exe", "EliteDangerous64.exe.bak", "eliteaux.exe"} {
		if isGameExecutable(name) {
			t.Fatalf("unrelated executable matched: %s", name)
		}
	}
}
