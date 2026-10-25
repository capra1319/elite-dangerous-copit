package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestFocusPauseResumesOnlySameManuallyArmedSessionWithoutReplay(t *testing.T) {
	c, sink := testEngine(t)
	if c.resume(testTarget(), testNow, nil) {
		t.Fatal("fresh application auto-armed")
	}
	armEngine(t, c)
	if err := c.trigger(context.Background(), "t1", testNow, testTarget(), nil); err != nil {
		t.Fatal(err)
	}
	state := newMock(testNow).State
	c.update(&state, nil, testNow.Add(time.Millisecond), gameTarget{})
	if c.Armed || !c.ResumeTarget.valid() || c.Pending != nil || c.Target.valid() {
		t.Fatal("focus loss failed to pause and discard pending input")
	}
	if c.resume(gameTarget{}, testNow.Add(time.Second), nil) {
		t.Fatal("resumed outside game")
	}
	if !c.resume(testTarget(), testNow.Add(time.Second), nil) || !c.Armed || len(sink.keys) != 1 {
		t.Fatal("same-session focus return failed or replayed input")
	}
	if c.ResumeTarget.valid() {
		t.Fatal("resume token remained after arming")
	}
}

func TestStopFaultRestartAndSessionReplacementCancelFocusResume(t *testing.T) {
	for _, kind := range []string{"stop", "app-close", "game-close", "profile", "unreadable", "stale", "backwards", "new-pid", "reused-pid", "new-window", "new-path", "delivery"} {
		t.Run(kind, func(t *testing.T) {
			c, sink := testEngine(t)
			armEngine(t, c)
			c.observeFocus(gameTarget{})
			target := testTarget()
			now := testNow.Add(time.Second)
			switch kind {
			case "stop", "app-close", "game-close":
				c.disarm(kind)
			case "profile":
				c.resume(target, now, errors.New("bindings changed"))
			case "unreadable":
				c.update(nil, errors.New("partial"), now, gameTarget{})
			case "stale":
				c.resume(target, testNow.Add(2*time.Minute), nil)
			case "backwards":
				s := newMock(testNow.Add(-time.Second)).State
				c.update(&s, nil, now, gameTarget{})
			case "new-pid":
				target.PID++
			case "reused-pid":
				target.Created++
			case "new-window":
				target.Window++
			case "new-path":
				target.Path = filepath.Join(os.TempDir(), "other", "EliteDangerous64.exe")
			case "delivery":
				c.resume(target, now, nil)
				sink.err = errors.New("release failed")
				_ = c.trigger(context.Background(), "t3", now, target, nil)
			}
			if c.resume(target, now, nil) || c.Armed || c.ResumeTarget.valid() {
				t.Fatal("non-focus interruption retained auto-resume", kind)
			}
			if kind != "delivery" && len(sink.keys) != 0 {
				t.Fatal("recovery sent game input")
			}
		})
	}
}

func TestFocusResumeWaitsForClosedPanelsAndStopOverridesWaitingClick(t *testing.T) {
	c, _ := testEngine(t)
	armEngine(t, c)
	c.observeFocus(gameTarget{})
	c.State.GuiFocus = integer(6)
	if c.resume(testTarget(), testNow, nil) || !c.ResumeTarget.valid() {
		t.Fatal("panel must defer focus resume")
	}
	c.State.GuiFocus = integer(0)
	if !c.resume(testTarget(), testNow, nil) {
		t.Fatal("closed cockpit did not allow resume")
	}
	queue := make(chan controlRequest, 4)
	queue <- controlRequest{Kind: "action", Action: "t3", Created: time.Now()}
	requestStop(queue)
	if len(queue) != 1 || (<-queue).Kind != "stop" {
		t.Fatal("Stop did not replace a waiting click")
	}
}

func TestFlightPageOrderAndEngineeringTypeSearch(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock"}, testNow)
	expected := []int{pageCockpit, pageShip, pageNavigation, pageMissions, pageHardpoints, pageEngineering, pageSettings, pageDiagnostics, pageHelp}
	for _, page := range expected {
		if m.page != page {
			t.Fatal("unexpected page order", m.page, page)
		}
		m.stepPage(1)
	}
	if m.page != pageCockpit {
		t.Fatal("page order did not wrap")
	}
	m.page = pageEngineering
	for _, letter := range "phosphorus" {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}
	if !strings.Contains(m.materialFilter, "phosphorus") || m.onlyPinned {
		t.Fatal("typing P did not search normally")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.goalEditing {
		t.Fatal("search required an extra confirmation before editing")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: '2', Text: "20"})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.goals["raw/phosphorus"] != 20 {
		t.Fatal("simple target edit failed")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.materialFilter != "" || m.page != pageEngineering {
		t.Fatal("Escape did not clear the search")
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF2})
	if !m.onlyPinned {
		t.Fatal("all/pinned switch failed")
	}
}

func TestMockEngineeringEditsStayInMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goals.json")
	if err := saveMaterialGoals(path, map[string]int{"raw/arsenic": 5}); err != nil {
		t.Fatal(err)
	}
	m := newCockpit(cockpitOptions{mode: "mock", goals: path}, testNow)
	m.saveTarget("raw/arsenic", 20)
	saved, err := loadMaterialGoals(path)
	if err != nil || saved["raw/arsenic"] != 5 || m.goals["raw/arsenic"] != 20 {
		t.Fatal("simulator changed saved real targets")
	}
}
