package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestBootDoesNotDelayConnectionAndEndsOnTime(t *testing.T) {
	now := time.Now()
	m := newCockpit(cockpitOptions{mode: "control", boot: true}, now)
	m = updateCockpit(t, m, gamePresenceMsg{Checked: true, Known: true, Running: true})
	m = updateCockpit(t, m, dashboardFrame{State: cloneSnapshot(&newMock(now).State), ControlStatus: "DISARMED"})
	if !m.connected || !m.bootActive || !strings.Contains(ansi.Strip(m.View().Content), "FLIGHT SYSTEMS / STARTUP") {
		t.Fatal("boot blocked connection or did not render")
	}
	m = updateCockpit(t, m, bootMsg(now.Add(400*time.Millisecond)))
	if !m.bootActive || m.bootElapsed != 400*time.Millisecond {
		t.Fatal("animation did not advance")
	}
	next, cmd := m.Update(bootMsg(now.Add(bootDuration)))
	m = next.(cockpitModel)
	if m.bootActive || cmd != nil || !m.connected || strings.Contains(ansi.Strip(m.View().Content), "FLIGHT SYSTEMS / STARTUP") {
		t.Fatal("boot failed to hand off to the already-connected cockpit")
	}
	if m.frame.ControlStatus != "DISARMED" {
		t.Fatal("boot changed the backend arming state")
	}
}

func TestBootSkipNavigationQuitAndBackendStateRemainResponsive(t *testing.T) {
	for _, test := range []struct {
		key  tea.KeyPressMsg
		page int
		quit bool
	}{
		{tea.KeyPressMsg{Code: ' '}, pageCockpit, false},
		{tea.KeyPressMsg{Code: tea.KeyEnter}, pageCockpit, false},
		{tea.KeyPressMsg{Code: tea.KeyF10}, pageHardpoints, false},
		{tea.KeyPressMsg{Code: 'q'}, pageCockpit, true},
	} {
		m := newCockpit(cockpitOptions{mode: "control", boot: true}, testNow)
		next, cmd := m.Update(test.key)
		m = next.(cockpitModel)
		if m.bootActive || m.page != test.page {
			t.Fatal("boot swallowed navigation or did not skip")
		}
		if test.quit && (cmd == nil || cmd() != tea.Quit()) {
			t.Fatal("boot swallowed Quit")
		}
		_, cmd = m.Update(bootMsg(testNow.Add(40 * time.Millisecond)))
		if cmd != nil {
			t.Fatal("skipped boot restarted its timer")
		}
	}
	m := newCockpit(cockpitOptions{mode: "control", boot: true}, testNow)
	m = updateCockpit(t, m, pageStepMsg(1))
	if m.bootActive || m.page != pageShip {
		t.Fatal("global page navigation did not bypass boot")
	}
	m = newCockpit(cockpitOptions{mode: "control", boot: true}, testNow)
	m = updateCockpit(t, m, dashboardFrame{ControlStatus: "ARMED"})
	if m.bootActive {
		t.Fatal("boot obscured the newly armed cockpit")
	}
	m = newCockpit(cockpitOptions{mode: "control", boot: true}, testNow)
	m = updateCockpit(t, m, backendDoneMsg{err: errors.New("startup failure")})
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "CONNECTION STOPPED") || strings.Contains(view, "FLIGHT SYSTEMS / STARTUP") {
		t.Fatal("boot obscured backend failure")
	}
}

func TestBootFitsAllThemesAndKeepsSafetyStatusVisible(t *testing.T) {
	for theme := range tuiThemes {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
			for _, elapsed := range []time.Duration{0, 320 * time.Millisecond, 760 * time.Millisecond} {
				m := newCockpit(cockpitOptions{mode: "control", boot: true}, testNow)
				m.themeChoice, m.savedTheme, m.width, m.height, m.bootElapsed = theme, theme, size[0], size[1], elapsed
				view := ansi.Strip(m.View().Content)
				rows := strings.Split(view, "\n")
				if len(rows) > size[1] {
					t.Fatal("boot exceeded terminal height")
				}
				for _, row := range rows {
					if ansi.StringWidth(row) > size[0] {
						t.Fatal("boot wrapped a terminal row")
					}
				}
				for _, label := range []string{"STARTUP", "DISARMED", "Ctrl+Alt+F10", "any key skips"} {
					if !strings.Contains(view, label) {
						t.Fatalf("boot hid %q at %v", label, size)
					}
				}
				if !strings.Contains(view, "/o") || (!strings.Contains(view, "ELITE // AUX") && !strings.Contains(view, "ELITE AUX")) {
					t.Fatalf("eagle head or medal caption clipped at %v", size)
				}
			}
		}
	}
}

func TestBootRunsOncePerLaunchAndItsTimerIsSessionOwned(t *testing.T) {
	for _, mode := range []string{"control", "live", "mock"} {
		calls := 0
		err := runCockpitSessions(context.Background(), cockpitOptions{mode: mode, boot: true}, func(opts cockpitOptions) (cockpitModel, error) {
			if opts.boot != (calls == 0) {
				t.Fatal("mode switching replayed the boot animation")
			}
			calls++
			return cockpitModel{switchMode: calls < 3}, nil
		})
		if err != nil || calls != 3 {
			t.Fatal("mode transition failed", err, calls)
		}
	}
	m := newCockpit(cockpitOptions{mode: "live", boot: true}, testNow)
	m.work = newSessionWork(context.Background())
	<-m.work.stop()
	if msg := m.tickBoot()(); msg != nil {
		t.Fatal("boot timer ran after session shutdown")
	}
}
