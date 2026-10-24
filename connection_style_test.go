package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestConnectionIndicatorTracksBackendAndDoesNotHighlightInvalidArming(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "control"}, testNow)
	if state := m.sessionIndicator(); state.ready || state.armed || !strings.Contains(state.label, "SEARCHING | DISARMED") {
		t.Fatal("startup must show searching and disarmed", state)
	}
	m.frame.State = cloneSnapshot(&newMock(testNow).State)
	m.frame.GameFocused = true
	m.updateConnection()
	if m.sessionIndicator().ready {
		t.Fatal("cached telemetry and focus bypassed the process check")
	}
	m.presence = gamePresence{Checked: true, Known: true, Running: true}
	m.updateConnection()
	if state := m.sessionIndicator(); !state.ready || state.armed || state.label != "CONNECTED | DISARMED" {
		t.Fatal("connection must remain distinct from arming", state)
	}
	m.frame.ControlStatus = "ARMED"
	if state := m.sessionIndicator(); !state.ready || !state.armed || state.colour != m.palette().Success {
		t.Fatal("backend acknowledgement did not activate armed treatment", state)
	}
	for _, invalidate := range []func(*cockpitModel){
		func(m *cockpitModel) { m.frame.Quality = "READ ERROR: partial file" },
		func(m *cockpitModel) { m.frame.Quality = "UNAVAILABLE: missing file" },
		func(m *cockpitModel) { m.frame.Quality = "SETUP: invalid bindings" },
		func(m *cockpitModel) { m.now = testNow.Add(2 * time.Minute) },
		func(m *cockpitModel) { m.presence.Running = false; m.updateConnection() },
		func(m *cockpitModel) { m.frame.State = nil; m.updateConnection() },
		func(m *cockpitModel) { m.fatal = errors.New("backend stopped") },
	} {
		other := m
		invalidate(&other)
		if state := other.sessionIndicator(); state.armed || state.ready || strings.Contains(state.label, "CONNECTED") {
			t.Fatal("invalid connection retained the active armed treatment", state)
		}
	}
	m.frame.ControlStatus = "DISARMED"
	if m.sessionIndicator().armed || m.connectionBorder() == m.palette().Success {
		t.Fatal("disarming left armed styling behind")
	}
	m.opts.mode = "live"
	if m.sessionIndicator().label != "CONNECTED | READ ONLY" {
		t.Fatal("read-only mode shown as armed")
	}
	m.opts.mode = "mock"
	m.frame.ControlStatus = "ARMED"
	if state := m.sessionIndicator(); state.armed || state.ready || !strings.Contains(state.label, "MOCK DATA") {
		t.Fatal("simulation presented as a real connection")
	}
}

func TestConnectedAndArmedBannerIsVisibleOnEveryPageAndTheme(t *testing.T) {
	for theme := range tuiThemes {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
			m := newCockpit(cockpitOptions{mode: "control"}, testNow)
			m.themeChoice, m.savedTheme, m.width, m.height = theme, theme, size[0], size[1]
			m.frame.State = cloneSnapshot(&newMock(testNow).State)
			m.presence = gamePresence{Checked: true, Known: true, Running: true}
			m.updateConnection()
			m.companion = mockCompanionData(testNow)
			for _, armed := range []bool{false, true} {
				m.frame.ControlStatus = "DISARMED"
				label := "CONNECTED | DISARMED"
				if armed {
					m.frame.ControlStatus = "ARMED"
					label = ">>> CONNECTED | ARMED <<<"
				}
				for page := pageCockpit; page < pageCount; page++ {
					m.page = page
					view := m.View()
					plain := ansi.Strip(view.Content)
					rows := strings.Split(plain, "\n")
					if len(rows) > size[1] {
						t.Fatal("status banner increased terminal height")
					}
					for _, row := range rows {
						if ansi.StringWidth(row) > size[0] {
							t.Fatal("status banner caused wrapping")
						}
					}
					if len(rows) < 3 || !strings.Contains(rows[2], label) || !strings.Contains(plain, "Ctrl+Alt+F10") || !strings.Contains(view.WindowTitle, label) {
						t.Fatalf("theme %d, %v, page %d: missing status or stop key\n%s", theme, size, page, plain)
					}
					if width := ansi.StringWidth(m.connectionBanner(size[0] - 4)); width != size[0]-4 {
						t.Fatal("status strip does not span the terminal")
					}
				}
			}
		}
	}
}
