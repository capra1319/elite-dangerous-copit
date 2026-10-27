package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const bootDuration = 800 * time.Millisecond

type bootMsg time.Time

func (m cockpitModel) tickBoot() tea.Cmd {
	return m.work.command(40*time.Millisecond, func() tea.Msg { return bootMsg(time.Now()) })
}

func (m cockpitModel) bootScreen(width, height int) string {
	p := m.palette()
	phase := min(19, max(0, int(m.bootElapsed/(40*time.Millisecond))))
	inside := max(1, width-4)
	logo := eagleMedalLogo(inside, max(1, height-6))
	spare := height - len(logo) - 6
	lines := []string{m.section("FLIGHT SYSTEMS / STARTUP")}
	if spare > 0 {
		lines = append(lines, "")
		spare--
	}
	for row, text := range logo {
		var sweep strings.Builder
		beam := phase*(inside+8)/19 - 4
		for col, character := range []rune(text) {
			colour := p.Border
			if col <= beam {
				colour = p.Accent
			}
			if col >= beam-2 && col <= beam+2 {
				colour = p.Info
			}
			if row%2 == 1 && col > beam {
				colour = p.Muted
			}
			sweep.WriteString(bold(string(character), colour))
		}
		lines = append(lines, sweep.String())
	}
	barWidth := min(40, max(8, inside-2))
	filled := (phase + 1) * barWidth / 20
	bar := "[" + ink(strings.Repeat("█", filled), p.Accent) + ink(strings.Repeat("░", barWidth-filled), p.Border) + "]"
	if spare > 0 {
		lines = append(lines, "")
		spare--
	}
	lines = append(lines, strings.Repeat(" ", max(0, (inside-barWidth-2)/2))+bar)
	state := m.sessionIndicator()
	lines = append(lines, ink(state.label, state.colour))
	if spare > 0 {
		lines = append(lines, ink(fmt.Sprintf("%s · %s", p.Name, "terminal systems online"), p.Muted))
	}
	hint := "Game detection runs now · any key skips"
	if m.opts.mode == "mock" {
		hint = "Simulator active · any key skips"
	}
	lines = append(lines, ink(hint, p.Muted))
	return m.panel(width, m.palette().Border, lines...)
}
