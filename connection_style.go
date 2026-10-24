package main

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type sessionIndicator struct {
	label, detail string
	colour        color.Color
	ready, armed  bool
}

// The bright armed treatment requires both a live connection and the backend's
// acknowledgement. Local keys and cached snapshots cannot activate it.
func (m cockpitModel) sessionIndicator() sessionIndicator {
	p := m.palette()
	if m.opts.mode == "mock" {
		return sessionIndicator{label: "SIMULATOR | MOCK DATA", detail: "Training mode", colour: p.Simulation}
	}
	control := "READ ONLY"
	if m.opts.mode == "control" {
		control = "DISARMED"
		if m.frame.ControlStatus == "ARMED" {
			control = "ARMED"
		}
	}
	if m.fatal != nil {
		return sessionIndicator{label: "OFFLINE | " + control, detail: "Connection stopped", colour: p.Danger}
	}
	if m.opts.mode == "control" && strings.Contains(m.frame.Quality, "SETUP:") {
		return sessionIndicator{label: "SETUP NEEDED | " + control, detail: "See Diagnostics", colour: p.Warning}
	}
	if strings.HasPrefix(m.frame.Quality, "UNAVAILABLE") || strings.Contains(m.frame.Quality, "READ ERROR") {
		return sessionIndicator{label: "SIGNAL LOST | " + control, detail: "Waiting for telemetry", colour: p.Danger}
	}
	if m.scanning() {
		return sessionIndicator{label: "SEARCHING | " + control, detail: "Waiting for game session", colour: p.Muted}
	}
	link, _ := m.linkState()
	if link != "TELEMETRY" {
		return sessionIndicator{label: link + " | " + control, detail: "Check telemetry", colour: p.Warning}
	}
	if m.opts.mode == "control" && m.frame.ControlStatus == "ARMED" {
		return sessionIndicator{label: ">>> CONNECTED | ARMED <<<", detail: "GAME CONTROLS ACTIVE", colour: p.Success, ready: true, armed: true}
	}
	if m.opts.mode == "control" {
		return sessionIndicator{label: "CONNECTED | DISARMED", detail: "Controls locked", colour: p.Warning, ready: true}
	}
	return sessionIndicator{label: "CONNECTED | READ ONLY", detail: "Observing game", colour: p.Info, ready: true}
}

func (m cockpitModel) connectionBanner(width int) string {
	state := m.sessionIndicator()
	text := " " + state.label
	if width >= 70 {
		text = between(text, state.detail+" ", width)
	}
	return lipgloss.NewStyle().Bold(true).Foreground(m.palette().BG).Background(state.colour).Render(padLine(text, width))
}

func (m cockpitModel) connectionBorder() color.Color {
	state := m.sessionIndicator()
	if state.armed {
		return m.palette().Success
	}
	if state.ready {
		return m.palette().Info
	}
	return m.palette().Border
}
