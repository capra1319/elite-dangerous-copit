package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The software toggle group can grow independently of the four hardware LEDs.
// Night vision retains its existing b1 command and Num5 binding.
var cockpitToggles = []Switch{switches[0], switches[1], switches[2], switches[3], {"b1", "Night vision", nightVision}}

type cockpitCategory struct {
	name             string
	toggles, buttons []int
}

var cockpitCategories = []cockpitCategory{
	{"FLIGHT & UTILITY", []int{0, 1}, nil},
	{"VISIBILITY", []int{2, 4}, nil},
	{"COMBAT", []int{3}, []int{3, 4, 5}},
	{"NAVIGATION", nil, []int{1, 2}},
}

var cockpitButtons = []struct{ key, name string }{
	{"5", "Night vision"}, {"6", "Galaxy map"}, {"7", "System map"},
	{"8", "Fire group"}, {"9", "Heat sink"}, {"0", "Chaff"},
}

func ink(text string, c color.Color) string { return lipgloss.NewStyle().Foreground(c).Render(text) }
func bold(text string, c color.Color) string {
	return lipgloss.NewStyle().Bold(true).Foreground(c).Render(text)
}
func (m cockpitModel) chip(text string, c color.Color) string {
	return lipgloss.NewStyle().Foreground(m.palette().BG).Background(c).Bold(true).Padding(0, 1).Render(text)
}
func padLine(text string, width int) string {
	if width < 1 {
		return ""
	}
	s := ansi.Truncate(text, width, "…")
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}
func between(left, right string, width int) string {
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return padLine(left+"  "+right, width)
	}
	return left + strings.Repeat(" ", gap) + right
}
func (m cockpitModel) panel(width int, c color.Color, lines ...string) string {
	width = max(6, width)
	inside := width - 4
	frame := m.palette().Frame
	if c == m.palette().Border {
		c = m.connectionBorder()
	}
	rows := []string{ink(frame.topLeft+strings.Repeat(frame.horizontal, width-2)+frame.topRight, c)}
	for _, line := range lines {
		rows = append(rows, ink(frame.vertical, c)+" "+padLine(line, inside)+" "+ink(frame.vertical, c))
	}
	rows = append(rows, ink(frame.bottomLeft+strings.Repeat(frame.horizontal, width-2)+frame.bottomRight, c))
	return strings.Join(rows, "\n")
}
func (m cockpitModel) section(label string) string { return bold(label, m.palette().Muted) }
func (m cockpitModel) grid(cards []string, columns int) string {
	var rows []string
	for start := 0; start < len(cards); start += columns {
		end := min(start+columns, len(cards))
		joined := make([]string, 0, end-start)
		for _, card := range cards[start:end] {
			joined = append(joined, card)
		}
		gap := ink("  ", m.palette().BG)
		var parts []string
		for i, card := range joined {
			if i > 0 {
				parts = append(parts, gap)
			}
			parts = append(parts, card)
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, parts...))
	}
	return strings.Join(rows, "\n")
}

func (m cockpitModel) View() tea.View {
	w, h := max(20, m.width), max(8, m.height)
	content := m.render(w, h)
	v := tea.NewView(content)
	v.AltScreen = true
	v.ReportFocus = true
	v.WindowTitle = "Elite Aux • " + m.sessionIndicator().label
	if os.Getenv("NO_COLOR") == "" {
		v.BackgroundColor = m.palette().BG
		v.ForegroundColor = m.palette().Text
	}
	v.KeyboardEnhancements.ReportEventTypes = true
	return v
}

func (m cockpitModel) render(width, height int) string {
	w := max(16, width-4)
	mode, modeColour := "OBSERVE", m.palette().Info
	if m.opts.mode == "mock" {
		mode, modeColour = "SIMULATOR", m.palette().Simulation
	}
	if m.opts.mode == "control" {
		mode, modeColour = "LIVE CONTROL", m.palette().Accent
	}
	armed, armColour := "READ ONLY", m.palette().Info
	if m.opts.mode == "mock" {
		armed, armColour = "MOCK DATA", m.palette().Simulation
	} else if m.opts.mode == "control" {
		armed, armColour = "DISARMED", m.palette().Warning
		if m.frame.ControlStatus == "ARMED" {
			armed, armColour = "● ARMED", m.sessionIndicator().colour
		} else if m.frame.ControlStatus == "PAUSED" {
			armed = "PAUSED"
		}
	}
	link, linkColour := m.linkState()
	if m.opts.mode == "control" && strings.Contains(m.frame.Quality, "SETUP:") {
		link, linkColour = "SETUP NEEDED", m.palette().Warning
	}
	title := between(bold("ELITE // AUX", m.palette().Accent)+ink("  FLIGHT TERMINAL", m.palette().Muted), m.chip(mode, modeColour)+"  "+m.chip(armed, armColour), w)
	context := "Awaiting ship telemetry"
	if m.frame.State != nil {
		context = m.frame.State.context()
	}
	status := between(ink(context, m.palette().Muted), ink("● "+link, linkColour)+"  "+ink(m.age(), m.palette().Muted), w)
	header := strings.Join([]string{title, status, m.connectionBanner(w), m.pageTabs(w)}, "\n")
	footer := m.footer(w)
	budget := height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	var body string
	switch m.page {
	case 1:
		body = m.diagnostics(w)
	case 2:
		body = m.help(w)
	case pageSettings:
		body = m.appearanceSettings(w)
	case pageShip:
		body = m.shipOverviewPage(w)
	case pageNavigation:
		body = m.navigationPage(w)
	case pageMissions:
		body = m.missionBoardPage(w)
	case pageEngineering:
		body = m.engineeringPage(w)
	case pageHardpoints:
		body = m.hardpointsPage(w)
	default:
		if m.fatal != nil {
			body = m.panel(w, m.palette().Danger, bold("CONNECTION STOPPED", m.palette().Danger), m.fatal.Error(), "Close the app, resolve the error, then relaunch.", "Open Diagnostics from the tabs. F1 opens Help.")
		} else if m.bootActive {
			body = m.bootScreen(w, budget)
		} else if m.scanning() {
			body = m.connectionScanner(w, budget)
		} else if w >= 100 && height >= 34 {
			body = m.fullCockpit(w)
		} else {
			body = m.compactCockpit(w)
		}
	}
	if m.page != 0 {
		rows := strings.Split(body, "\n")
		offset := min(m.scroll, max(0, len(rows)-max(1, budget)))
		body = strings.Join(rows[offset:], "\n")
	}
	body = m.cropBlock(body, w, max(1, budget))
	content := header + "\n\n" + body
	remaining := height - lipgloss.Height(content) - lipgloss.Height(footer)
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	} else {
		content += "\n"
	}
	content += footer
	rows := strings.Split(content, "\n")
	if len(rows) > height {
		rows = rows[:height]
	}
	for i, row := range rows {
		rows[i] = "  " + padLine(row, w)
	}
	return strings.Join(rows, "\n")
}

func (m cockpitModel) cropBlock(content string, width, height int) string {
	rows := strings.Split(content, "\n")
	if len(rows) > height {
		rows = rows[:height]
		rows[len(rows)-1] = ink("↑ Enlarge this window for the full cockpit layout", m.palette().Muted)
		if m.page != pageCockpit {
			rows[len(rows)-1] = ink("↑↓ Scroll / PgUp PgDn for more", m.palette().Muted)
		}
	}
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, width, "…")
	}
	return strings.Join(rows, "\n")
}

func (m cockpitModel) linkState() (string, color.Color) {
	if m.opts.mode == "mock" {
		return "SIMULATED", m.palette().Simulation
	}
	if m.fatal != nil {
		return "OFFLINE", m.palette().Danger
	}
	if m.scanning() {
		return "SEARCHING", m.palette().Info
	}
	if m.frame.State == nil || strings.HasPrefix(m.frame.Quality, "UNAVAILABLE") || strings.Contains(m.frame.Quality, "READ ERROR") {
		return "NO SIGNAL", m.palette().Danger
	}
	age := m.now.Sub(m.frame.State.Timestamp)
	if age > m.opts.stale || age < -futureSnapshotTolerance {
		return "STALE DATA", m.palette().Warning
	}
	return "TELEMETRY", m.palette().Success
}
func (m cockpitModel) age() string {
	if m.frame.State == nil {
		return "--"
	}
	d := m.now.Sub(m.frame.State.Timestamp)
	if d < -futureSnapshotTolerance {
		return "clock mismatch"
	}
	if d < 0 {
		d = 0
	}
	return d.Round(time.Second).String() + " ago"
}
func (m cockpitModel) known() bool { return m.frame.State != nil && m.frame.State.shipFeedbackKnown() }

func (m cockpitModel) primaryState(index int) (string, color.Color) {
	if !m.known() {
		return "? UNKNOWN", m.palette().Muted
	}
	on := m.frame.State.has(cockpitToggles[index].Mask)
	value, c := "○ INACTIVE", m.palette().Muted
	if on {
		value, c = "● ACTIVE", m.palette().Success
	}
	if link, _ := m.linkState(); link == "STALE DATA" || link == "NO SIGNAL" || link == "OFFLINE" {
		c = m.palette().Warning
	}
	return value, c
}
func (m cockpitModel) buttonInfo(index int) (string, color.Color) {
	s := m.frame.State
	if index == 0 {
		return m.primaryState(4)
	}
	if index == 1 || index == 2 {
		focus := 6
		if index == 2 {
			focus = 7
		}
		if s != nil && s.GuiFocus != nil && *s.GuiFocus == focus {
			return "● MAP OPEN", m.palette().Info
		}
		return "OPEN / CLOSE", m.palette().Muted
	}
	if index == 3 {
		if s != nil && s.FireGroup != nil && *s.FireGroup >= 0 && *s.FireGroup < 26 {
			return "GROUP " + string(rune('A'+*s.FireGroup)), m.palette().Info
		}
		return "NEXT GROUP", m.palette().Muted
	}
	return "CONSUMABLE · PULSE", m.palette().Warning
}
func (m cockpitModel) toggleRow(index, width int, compact bool) string {
	sw := cockpitToggles[index]
	value, c := m.primaryState(index)
	if compact {
		value = strings.TrimLeft(value, "○●? ")
	}
	if m.frame.Pending == sw.ID {
		value += " · WAIT"
		c = m.palette().Warning
	}
	left := ink(fmt.Sprintf("NUM %d", index+1), m.palette().Accent) + " " + sw.Name
	left = ansi.Truncate(left, max(1, width-ansi.StringWidth(value)-1), "…")
	return between(left, bold(value, c), width)
}

func (m cockpitModel) actionRow(index, width int, compact bool) string {
	button := cockpitButtons[index]
	info, c := m.buttonInfo(index)
	if compact {
		info = strings.ReplaceAll(info, "OPEN / CLOSE", "OPEN/CLOSE")
		info = strings.ReplaceAll(info, "● MAP OPEN", "MAP OPEN")
	}
	if index >= 4 {
		info = "PULSE"
	}
	return between(ink("NUM "+button.key, m.palette().Accent)+" "+button.name, ink(info, c), width)
}

func (m cockpitModel) controlCategories(width int, compact bool) string {
	columns := 2
	if compact && width < 54 {
		columns = 1
	}
	cardWidth := (width - 2*(columns-1)) / columns
	var cards []string
	for start := 0; start < len(cockpitCategories); start += columns {
		end := min(start+columns, len(cockpitCategories))
		rowCount := 0
		for _, category := range cockpitCategories[start:end] {
			rowCount = max(rowCount, len(category.toggles)+len(category.buttons))
		}
		for _, category := range cockpitCategories[start:end] {
			inside := cardWidth
			if !compact {
				inside -= 4
			}
			lines := []string{m.section(category.name)}
			for _, index := range category.toggles {
				lines = append(lines, m.toggleRow(index, inside, compact))
			}
			for _, index := range category.buttons {
				lines = append(lines, m.actionRow(index, inside, compact))
			}
			for len(lines) < rowCount+1 {
				lines = append(lines, "")
			}
			card := m.panel(cardWidth, m.palette().Border, lines...)
			if compact {
				card = padBlock(strings.Join(lines, "\n"), cardWidth)
			}
			cards = append(cards, card)
		}
	}
	feedback := "SHIP CONTROLS  /  TELEMETRY"
	if !m.known() {
		feedback = "SHIP CONTROLS  /  UNKNOWN"
	} else if link, _ := m.linkState(); link == "STALE DATA" || link == "NO SIGNAL" || link == "OFFLINE" {
		feedback = "SHIP CONTROLS  /  LAST KNOWN"
	}
	return m.section(feedback) + "\n" + m.grid(cards, columns)
}

func (m cockpitModel) fullCockpit(width int) string {
	mainWidth := width
	sidebar := width >= 120
	if sidebar {
		mainWidth = width - 33
	}
	controls := m.controlCategories(mainWidth, false)
	var body string
	if sidebar {
		body = lipgloss.JoinHorizontal(lipgloss.Top, padBlock(controls, mainWidth), "  ", m.shipRail(31))
	} else {
		body = controls + "\n" + m.shipStrip(width)
	}
	return body + "\n" + m.activityPanel(width, 2)
}
func padBlock(content string, width int) string {
	rows := strings.Split(content, "\n")
	for i, row := range rows {
		rows[i] = padLine(row, width)
	}
	return strings.Join(rows, "\n")
}

func (m cockpitModel) compactCockpit(width int) string {
	controls := m.controlCategories(width, true)
	if m.height < 32 {
		status := m.shipStrip(width)
		if m.height < 26 {
			shield, sc := m.shipValue(shields, false)
			assist, ac := m.shipValue(assistOff, true)
			status = ink("SHIELDS ", m.palette().Muted) + ink(shield, sc) + ink("   ASSIST ", m.palette().Muted) + ink(assist, ac) + ink("   FSD ", m.palette().Muted) + ink(m.fsd(), m.palette().Info) + "\n" + m.pipsLine() + "   " + m.fuelLine() + "\n" + m.alertLine()
			if m.height < 24 {
				status = ink("FSD ", m.palette().Muted) + ink(m.fsd(), m.palette().Info) + "   " + m.pipsLine() + "\n" + m.alertLine()
			}
		}
		return controls + "\n" + status
	}
	return controls + "\n" + m.shipStrip(width) + "\n" + m.activityPanel(width, 1)
}

func (m cockpitModel) shipValue(mask uint64, invert bool) (string, color.Color) {
	if !m.known() {
		return "UNKNOWN", m.palette().Muted
	}
	on := m.frame.State.has(mask)
	if invert {
		on = !on
	}
	if on {
		return "ON", m.palette().Success
	}
	return "OFF", m.palette().Muted
}
func (m cockpitModel) shipStrip(width int) string {
	shield, sc := m.shipValue(shields, false)
	assist, ac := m.shipValue(assistOff, true)
	line := ink("SHIELDS ", m.palette().Muted) + bold(shield, sc) + "    " + ink("FLIGHT ASSIST ", m.palette().Muted) + bold(assist, ac) + "    " + ink("FSD ", m.palette().Muted) + ink(m.fsd(), m.palette().Info)
	return m.panel(width, m.palette().Border, line, m.pipsLine()+"    "+m.fuelLine(), m.alertLine())
}

func (m cockpitModel) alertLine() string {
	if !m.known() {
		return ink("ALERTS  unavailable", m.palette().Muted)
	}
	var alerts []string
	for _, a := range []struct {
		mask  uint64
		label string
	}{{lowFuel, "LOW FUEL"}, {overheating, "OVERHEATING"}, {silent, "SILENT RUNNING"}} {
		if m.frame.State.has(a.mask) {
			alerts = append(alerts, a.label)
		}
	}
	if len(alerts) == 0 {
		return ink("ALERTS  no reported warnings", m.palette().Muted)
	}
	return bold("! "+strings.Join(alerts, "  /  "), m.palette().Danger)
}
func (m cockpitModel) fsd() string {
	if !m.known() {
		return "UNKNOWN"
	}
	s := m.frame.State
	if s.has(jumping) {
		return "JUMPING"
	}
	if s.has(charging) {
		return "CHARGING"
	}
	if s.has(cooldown) {
		return "COOLDOWN"
	}
	if s.has(massLocked) {
		return "MASS LOCKED"
	}
	if s.has(supercruise) {
		return "SUPERCRUISE"
	}
	return "IDLE"
}
func (m cockpitModel) fuelLine() string {
	if m.frame.State == nil || m.frame.State.Fuel == nil {
		return ink("FUEL —", m.palette().Muted)
	}
	return ink("FUEL ", m.palette().Muted) + ink(fmt.Sprintf("%.1f t", m.frame.State.Fuel.Main), m.palette().Info)
}
func (m cockpitModel) pipsLine() string {
	s := m.frame.State
	if s == nil || len(s.Pips) != 3 {
		return ink("SYS —   ENG —   WEP —", m.palette().Muted)
	}
	var parts []string
	for i, label := range []string{"SYS", "ENG", "WEP"} {
		value := s.Pips[i]
		if value < 0 || value > 8 {
			parts = append(parts, ink(label+" ?", m.palette().Muted))
			continue
		}
		parts = append(parts, ink(label+" ", m.palette().Muted)+ink(fmt.Sprintf("%.1f", float64(value)/2), m.palette().Info))
	}
	return strings.Join(parts, "   ")
}
func (m cockpitModel) shipRail(width int) string {
	shield, sc := m.shipValue(shields, false)
	assist, ac := m.shipValue(assistOff, true)
	status := m.panel(width, m.palette().Border, m.section("SHIP CONDITION"), between("Shields", bold(shield, sc), width-4), between("Flight assist", bold(assist, ac), width-4), between("FSD", ink(m.fsd(), m.palette().Info), width-4), m.fuelLine())
	var pipRows []string
	pipRows = append(pipRows, m.section("POWER DISTRIBUTION"))
	for i, label := range []string{"SYS", "ENG", "WEP"} {
		value := -1
		if m.frame.State != nil && len(m.frame.State.Pips) == 3 {
			value = m.frame.State.Pips[i]
		}
		bar := ink("········", m.palette().Border)
		if value >= 0 && value <= 8 {
			bar = ink(strings.Repeat("▪", value), m.palette().Info) + ink(strings.Repeat("·", 8-value), m.palette().Border)
		}
		pipRows = append(pipRows, ink(label+"  ", m.palette().Muted)+bar)
	}
	warnings := []string{m.section("FLIGHT ALERTS")}
	if m.known() {
		for _, a := range []struct {
			mask uint64
			text string
		}{{lowFuel, "LOW FUEL"}, {overheating, "OVERHEATING"}, {silent, "SILENT RUNNING"}} {
			if m.frame.State.has(a.mask) {
				warnings = append(warnings, bold("! "+a.text, m.palette().Danger))
			}
		}
	}
	if len(warnings) == 1 {
		if m.known() {
			warnings = append(warnings, ink("No reported warnings", m.palette().Muted))
		} else {
			warnings = append(warnings, ink("Ship state unavailable", m.palette().Muted))
		}
	}
	return status + "\n" + m.panel(width, m.palette().Border, pipRows...) + "\n" + m.panel(width, m.palette().Border, warnings...)
}

func (m cockpitModel) activityPanel(width, count int) string {
	lines := []string{m.section("ACTIVITY")}
	for i := 0; i < count; i++ {
		if i >= len(m.history) {
			lines = append(lines, ink("Waiting for an action or game update", m.palette().Muted))
			continue
		}
		a := m.history[i]
		c := m.palette().Text
		if strings.Contains(strings.ToLower(a.text), "blocked") || strings.Contains(strings.ToLower(a.text), "fail") {
			c = m.palette().Warning
		}
		lines = append(lines, ink(a.time.Format("15:04:05")+"  ", m.palette().Muted)+ink(a.text, c))
	}
	return m.panel(width, m.palette().Border, lines...)
}

func (m cockpitModel) footer(width int) string {
	if m.page == pageNavigation && m.outfitting.Active {
		return m.outfittingFooter(width)
	}
	if m.page == pageNavigation && m.stationSearchActive {
		message := m.stationSearchMessage
		if message == "" {
			message = "Check update age. No live mission-board preview."
		}
		stop := "Tab pages  Q exit"
		if m.opts.mode == "control" {
			stop = "Ctrl+Alt+F9 arm/disarm  Ctrl+Alt+F10 STOP"
		}
		return padLine(ink(message, m.palette().Accent), width) + "\n" + padLine(ink(stop, m.palette().Muted), width) + "\n" + padLine(ink("↑↓ category  Enter opens browser  Esc back", m.palette().Text), width)
	}
	last := padLine(ink("LAST  ", m.palette().Muted)+ink(m.frame.Action, m.palette().Text), width) + "\n"
	if m.page == pageHardpoints {
		status := m.fireGroupMessage
		if status == "" {
			status = "Reference only · match assignments to your in-game panel"
		}
		control := "Tab / Num+/- pages  Q exit"
		if m.opts.mode == "control" {
			control = "Ctrl+Alt+F9 arm/disarm  Ctrl+Alt+F10 STOP"
		}
		return padLine(ink(status, m.palette().Accent), width) + "\n" + padLine(ink(control, m.palette().Muted), width) + "\n" +
			padLine(ink("↑↓ module  ←→ group  1 primary  2 secondary  0 none  Del ?", m.palette().Text), width)
	}
	if m.page == pageEngineering {
		status := m.goalMessage
		if status == "" {
			status = "Engineering targets · saved separately from game controls"
		}
		if m.materialSearching {
			status = "SEARCH > " + safeText(m.materialFilter) + "▏  Enter target / Esc clear"
		}
		if m.goalEditing {
			status = "TARGET > " + string(m.goalDigits) + "▏  Enter save / Esc cancel"
		}
		control := "Tab page  F5 settings  Q exit"
		if m.opts.mode == "control" {
			control = "Ctrl+Alt+F9 arm/disarm  Ctrl+Alt+F10 STOP"
		}
		return padLine(ink(status, m.palette().Accent), width) + "\n" + padLine(ink(control, m.palette().Muted), width) + "\n" +
			padLine(ink("Type search  ↑↓ select  Enter target  F2 all/pinned  +/-", m.palette().Text), width)
	}
	if m.page == pageSettings {
		status := "THEME  " + m.palette().Name
		if m.themeChoice != m.savedTheme {
			status += " (preview)"
		}
		if m.settingsMessage != "" {
			status = m.settingsMessage
		}
		stop := ""
		if m.opts.mode == "control" {
			stop = "  Ctrl+Alt+F10 STOP"
		}
		return padLine(ink(status, m.palette().Accent), width) + "\n" +
			padLine(ink("Up/Down preview  Enter save  Esc cancel"+stop, m.palette().Text), width) + "\n" +
			padLine(ink("F5 settings  Tab page  Num+/- pages  Q exit", m.palette().Muted), width)
	}
	if m.opts.mode == "mock" {
		input := ink("NUMPAD READY", m.palette().Simulation) + ink("  Focus this window · Num Lock on · Enter closes map", m.palette().Muted)
		if m.opts.controls == "commands" {
			input = ink("COMMAND › ", m.palette().Accent) + ink(string(m.command), m.palette().Text) + ink("▏", m.palette().Accent)
		}
		return last + padLine(input, width) + "\n" + padLine(ink("F4 mock OFF  F5 settings  Tab page  Num+/- pages  Q exit", m.palette().Muted), width)
	}
	if m.opts.mode == "control" {
		return last + padLine(bold("Ctrl+Alt+F9", m.palette().Accent)+" arm / disarm    "+bold("Ctrl+Alt+F10", m.palette().Danger)+" STOP", width) + "\n" + padLine(ink("F4 mock ON  F5 settings  Tab page  Num+/- pages  Q exit", m.palette().Muted), width)
	}
	return last + padLine(ink("OBSERVATION ONLY · use your usual game controls", m.palette().Info), width) + "\n" + padLine(ink("F4 mock ON  F5 settings  Tab page  Num+/- pages  Q exit", m.palette().Muted), width)
}

func (m cockpitModel) diagnostics(width int) string {
	status := m.frame.ControlStatus
	if m.opts.mode == "mock" {
		status = "SIMULATION"
	}
	focus := "not detected"
	if m.frame.GameFocused {
		focus = "Elite Dangerous is foreground"
	}
	lines := []string{m.section("CONNECTION & CONTROL"), "Mode           " + m.opts.mode, "Input          " + m.opts.controls, "Control state  " + status, "Game focus     " + focus, "Telemetry age  " + m.age(), "", m.section("SOURCE HEALTH"), m.frame.Quality, "", m.section("STATUS FILE"), m.opts.path, "", m.section("CONTROL CONFIGURATION"), m.opts.config}
	lines = append(lines, "", m.section("PLAYER JOURNAL"), safeText(m.opts.journal), m.journalStatus(), "Source: "+safeText(m.companion.Source), "", m.section("ENGINEERING TARGETS"), safeText(m.opts.goals), safeText(m.goalMessage))
	lines = append(lines, "", m.section("PAGE SHORTCUTS"), "Numpad minus: previous page. Numpad plus: next page.", reportedText(m.frame.PageStatus))
	if m.frame.State != nil {
		skew := m.now.Sub(m.frame.State.Timestamp)
		lines = append(lines, fmt.Sprintf("Snapshot age: %.3f seconds (negative = ahead)", skew.Seconds()), "Small offsets up to 5 seconds are tolerated; timestamps use UTC.")
	}
	process := "checking"
	if m.presence.Checked {
		process = "not running"
		if !m.presence.Known {
			process = "unavailable; telemetry fallback"
		} else if m.presence.Running {
			process = "running (focus independent)"
		}
	}
	if m.opts.mode != "mock" {
		lines = append(lines, "", "Game process   "+process)
		if m.presence.Error != "" {
			lines = append(lines, "Process check  "+m.presence.Error)
		}
	}
	if m.frame.State != nil && m.frame.State.Flags != nil {
		lines = append(lines, "", fmt.Sprintf("Flags          0x%08X", *m.frame.State.Flags), "Flags2         "+derefU64(m.frame.State.Flags2), "GUI focus      "+derefInt(m.frame.State.GuiFocus), "Game timestamp "+m.frame.State.Timestamp.UTC().Format(time.RFC3339))
	}
	return m.panel(width, m.palette().Border, lines...) + "\n" + m.activityPanel(width, 4)
}
func (m cockpitModel) help(width int) string {
	lines := []string{bold("YOUR AUXILIARY FLIGHT PANEL", m.palette().Accent), "", m.section("CONTROLS BY CATEGORY"), "Flight & Utility:  1 Landing gear, 2 Cargo scoop", "Visibility:        3 Ship lights, 5 Night vision", "Combat:            4 Hardpoints, 8 Fire group, 9 Heat sink, 0 Chaff", "Navigation:        6 Galaxy map, 7 System map", "", m.section("REAL GAME CONTROL"), "Ctrl+Alt+F9   Arm/disarm while Elite is focused", "Ctrl+Alt+F10  Stop sending commands", "Focus loss pauses; returning to the same game resumes. STOP cancels.", "Select EliteAuxPanel under Ship Controls before arming.", "The matching map key closes its map. Disarm before chat/menus.", "", m.section("MOCK / TRAINING (OPT-IN)"), "F4 toggles mock mode on/off while this app is focused.", "Real controls stop before simulation. Returning starts disarmed.", "F2 cycles flight scenarios. F3 switches typed commands / numpad.", "Typed examples: t1 on, t3 toggle, b1, external t1 off, fail.", "No mock action sends input to the game.", "", m.section("FEEDBACK"), "Toggles report ACTIVE / INACTIVE; missing data stays UNKNOWN.", "Active, inactive, waiting and warnings use each theme's colours.", "LEDs follow telemetry. Failed requests are not retried.", "Heat sink / chaff are consumables in real-game mode.", "", m.section("APPEARANCE"), "F5 opens Settings. Up/Down previews the three themes.", "Enter saves. Esc cancels an unsaved preview.", "", m.section("COMPANION PAGES"), "F6 Ship   F7 Navigation   F8 Missions", "F9 Engineering   F10 Hardpoints / fire groups", "Hardpoints: arrows select module/group; 0/1/2 set reference.", "Delete clears a reference. Game assignments are not exported.", "Tab / Shift+Tab cycles every page. Up/Down or PgUp/PgDn scroll.", "Engineering: type to search, F2 switches all/pinned.", "Enter edits a target; type quantity, Enter saves, Esc cancels.", "+/- adjusts a target; Delete removes it. Esc clears search.", "Hull and module health are last reported, not continuous.", "Mission details depend on available acceptance records.", "", m.section("NUMPAD PAGE KEYS"), "Num+ moves to the next page. Num- moves to the previous page.", "In real-control mode they also work while Elite is focused.", "Page keys never arm controls or send flight commands."}
	return m.panel(width, m.palette().Border, lines...)
}
