package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestClockSkewToleranceIsConsistentAcrossDisplayScannerAndControls(t *testing.T) {
	for _, ahead := range []time.Duration{0, time.Millisecond, 500 * time.Millisecond, time.Second, 5 * time.Second, 5*time.Second + time.Millisecond, time.Minute} {
		t.Run(ahead.String(), func(t *testing.T) {
			m := newCockpit(cockpitOptions{mode: "control"}, testNow)
			s := newMock(testNow.Add(ahead)).State
			m.frame.State = &s
			m.presence = gamePresence{Checked: true, Known: true, Running: true}
			m.updateConnection()
			engine, _ := testEngine(t)
			engine.State = cloneSnapshot(&s)
			tooFar := ahead > futureSnapshotTolerance
			if (m.age() == "clock mismatch") != tooFar || m.scanning() != tooFar || (engine.telemetryUsable(testNow) != nil) != tooFar || strings.HasPrefix(qualityFor(&s, nil, testNow, time.Minute), "CLOCK MISMATCH") != tooFar {
				t.Fatal("display, connection and input guards disagree about clock skew")
			}
			if !tooFar && m.age() != "0s ago" {
				t.Fatal("small future offset should display zero age")
			}
		})
	}
	m := newCockpit(cockpitOptions{mode: "live"}, testNow)
	s := newMock(testNow).State
	s.Timestamp = s.Timestamp.In(time.FixedZone("US example", -7*60*60))
	m.frame.State = &s
	if m.age() != "0s ago" {
		t.Fatal("timezone representation changed the age of the same instant")
	}
}

func TestDelayedClockTickCannotMoveDisplayBehindNewerTelemetry(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "live"}, testNow)
	m.now = testNow.Add(time.Second)
	s := newMock(testNow.Add(900 * time.Millisecond)).State
	m.frame.State = &s
	m = updateCockpit(t, m, clockMsg(testNow))
	if !m.now.Equal(testNow.Add(time.Second)) || m.age() != "0s ago" {
		t.Fatal("queued timer moved the model clock backwards")
	}
}

func TestNumpadPageNavigationUsesPhysicalKeysAndPreservesFlightKeys(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock", controls: "numpad"}, testNow)
	for _, key := range []tea.KeyPressMsg{{Code: '-', Text: "-"}, {Code: '+', Text: "+"}, {Code: tea.KeyKpPlus, Mod: tea.ModShift}, {Code: tea.KeyKp4, Mod: tea.ModCtrl | tea.ModAlt}} {
		m = updateCockpit(t, m, key)
		if m.page != pageCockpit {
			t.Fatal("top-row or modified operator changed pages")
		}
	}
	key := tea.KeyPressMsg{Code: '+', BaseCode: tea.KeyKpPlus, Text: "+"}
	m = updateCockpit(t, m, key)
	if m.page != pageShip || m.mock.State.has(gear) {
		t.Fatal("physical Num+ failed to navigate independently")
	}
	m = updateCockpit(t, m, key)
	if m.page != pageShip {
		t.Fatal("held page key repeated")
	}
	m = updateCockpit(t, m, tea.KeyReleaseMsg{Code: '+', BaseCode: tea.KeyKpPlus})
	m = updateCockpit(t, m, key)
	if m.page != pageNavigation {
		t.Fatal("released/repressed page key did not navigate")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyKpMinus})
	if m.page != pageShip {
		t.Fatal("Num- did not navigate backwards")
	}
	for _, mode := range []string{"live", "mock"} {
		model := newCockpit(cockpitOptions{mode: mode}, testNow)
		model = updateCockpit(t, model, tea.KeyPressMsg{Code: '-', BaseCode: tea.KeyKpMinus, Text: "-"})
		if model.page != pageHelp {
			t.Fatal("physical Num- did not wrap backwards")
		}
	}
	control := newCockpit(cockpitOptions{mode: "control"}, testNow)
	control = updateCockpit(t, control, tea.KeyPressMsg{Code: tea.KeyKpPlus})
	if control.page != pageCockpit {
		t.Fatal("control-mode page key must wait for its global hotkey message to avoid duplicate navigation")
	}
	control = updateCockpit(t, control, pageStepMsg(1))
	if control.page != pageShip {
		t.Fatal("global page message was not applied")
	}
	control.frame.PageStatus = "Global page keys unavailable"
	control = updateCockpit(t, control, tea.KeyPressMsg{Code: tea.KeyKpMinus})
	if control.page != pageCockpit {
		t.Fatal("local page keys must work when global registration is unavailable")
	}
}

func TestGlobalPageDispatcherIsViewOnlyAndCancelsUnsavedEdits(t *testing.T) {
	var deltas []int
	d := display{navigate: func(delta int) { deltas = append(deltas, delta) }}
	if !d.handlePageHotkey(previousPageHotkey) || !d.handlePageHotkey(nextPageHotkey) || d.handlePageHotkey(100) || d.handlePageHotkey(1) || len(deltas) != 2 || deltas[0] != -1 || deltas[1] != 1 {
		t.Fatal("page and flight hotkey dispatch overlapped")
	}
	if (display{}).handlePageHotkey(previousPageHotkey) {
		t.Fatal("missing navigation callback must not consume other input")
	}
	m := newCockpit(cockpitOptions{mode: "control"}, testNow)
	m.page, m.themeChoice, m.savedTheme = pageSettings, 1, 0
	m = updateCockpit(t, m, pageStepMsg(1))
	if m.page != pageDiagnostics || m.themeChoice != 0 {
		t.Fatal("global page change did not cancel appearance preview")
	}
	m.page, m.goalEditing, m.materialSearching = pageEngineering, true, true
	m.goals["raw/arsenic"] = 20
	m = updateCockpit(t, m, pageStepMsg(1))
	if m.page != pageSettings || m.goalEditing || m.materialSearching || m.goals["raw/arsenic"] != 20 {
		t.Fatal("page change wrote or retained an unfinished editor")
	}
}
