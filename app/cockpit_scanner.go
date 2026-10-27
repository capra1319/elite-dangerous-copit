package main

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m cockpitModel) connectionScanner(width, height int) string {
	process, file, session := ink("Checking Windows processes", m.palette().Info), ink("Waiting for Status.json", m.palette().Muted), ink("Waiting for a commander session", m.palette().Muted)
	if m.presence.Checked {
		if !m.presence.Known {
			process = ink("Unavailable — using telemetry", m.palette().Warning)
		} else if m.presence.Running {
			process = ink("Elite Dangerous detected", m.palette().Success)
		} else {
			process = ink("Waiting for the game to start", m.palette().Info)
		}
	}
	s := m.frame.State
	if s != nil {
		file = ink("Status snapshot found", m.palette().Success)
		if !m.usableSession() {
			session = ink("Load your commander into the game", m.palette().Warning)
		} else if age := m.now.Sub(s.Timestamp); age < -futureSnapshotTolerance {
			session = ink("Snapshot clock mismatch", m.palette().Warning)
		} else if age > m.opts.stale {
			session = ink("Cached snapshot — waiting for an update", m.palette().Warning)
		} else {
			session = ink("Session data ready", m.palette().Success)
		}
	}
	if strings.HasPrefix(m.frame.Quality, "UNAVAILABLE") {
		file = ink("Status file unavailable — retrying", m.palette().Warning)
	} else if strings.Contains(m.frame.Quality, "READ ERROR") {
		file = ink("Waiting for a complete snapshot", m.palette().Warning)
	}
	steps := []string{
		m.section("01 / GAME PROCESS"), process, "",
		m.section("02 / LOCAL STATUS FILE"), file, "",
		m.section("03 / COMMANDER SESSION"), session,
	}
	title := bold("SCANNING FOR ELITE DANGEROUS", m.palette().Info)
	bar := m.scanBuffer(min(40, max(8, width-8)))
	if width >= 96 && height >= 21 {
		radar := m.scanRadar()
		body := lipgloss.JoinHorizontal(lipgloss.Top, radar, "    ", strings.Join(steps, "\n"))
		lines := []string{title, ink("ACQUIRING LOCAL TELEMETRY", m.palette().Muted), ""}
		lines = append(lines, strings.Split(body, "\n")...)
		lines = append(lines, "", bar+ink("  LISTENING", m.palette().Info), "",
			ink("Start Elite and load your commander. The cockpit opens automatically.", m.palette().Muted),
			ink("Tab: connection details   •   Controls stay disarmed until you arm them.", m.palette().Muted))
		return m.panel(width, m.palette().Border, lines...)
	}
	return m.panel(width, m.palette().Border, title, "",
		ink("GAME     ", m.palette().Muted)+process, ink("STATUS   ", m.palette().Muted)+file, ink("SESSION  ", m.palette().Muted)+session,
		"", bar+ink("  LISTENING", m.palette().Info), "",
		ink("Start Elite and load your commander.", m.palette().Muted), ink("The cockpit opens automatically. Tab: details.", m.palette().Muted))
}

// This is an activity indicator, not an estimated completion percentage.
func (m cockpitModel) scanBuffer(width int) string {
	width = max(8, width)
	span := width - 5
	position := m.scanFrame % (span * 2)
	if position > span {
		position = span*2 - position
	}
	return ink(strings.Repeat("░", position), m.palette().Border) + ink("▒▓█▓▒", m.palette().Info) + ink(strings.Repeat("░", width-position-5), m.palette().Border)
}

func (m cockpitModel) scanRadar() string {
	const columns, rows = 31, 11
	angle := float64(m.scanFrame%60) * 2 * math.Pi / 60
	var lines []string
	for row := 0; row < rows; row++ {
		var line strings.Builder
		for col := 0; col < columns; col++ {
			x, y := float64(col-columns/2)/1.5, float64(row-rows/2)*2
			r := math.Hypot(x, y)
			cell, colour := " ", m.palette().Border
			if r <= 10.4 {
				if math.Abs(r-10) < 0.8 || math.Abs(r-6) < 0.6 || col == columns/2 || row == rows/2 {
					cell = "·"
				}
				delta := math.Mod(angle-math.Atan2(y, x)+2*math.Pi, 2*math.Pi)
				if delta < 0.24 {
					cell, colour = "•", m.palette().Info
				} else if delta < 0.65 && cell != " " {
					colour = m.palette().Muted
				}
			}
			if row == rows/2 && col == columns/2 {
				cell, colour = "+", m.palette().Accent
			}
			line.WriteString(ink(cell, colour))
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}
