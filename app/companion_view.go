package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

func safeText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(text))
}

func reportedText(text string) string {
	if strings.TrimSpace(text) == "" {
		return "unavailable"
	}
	return safeText(text)
}

func symbolText(text string) string {
	text = strings.TrimSuffix(strings.TrimPrefix(safeText(text), "$"), ";")
	return strings.ReplaceAll(text, "_", " ")
}

func reportedAt(stamp time.Time) string {
	if stamp.IsZero() {
		return "not reported"
	}
	return stamp.Local().Format("02 Jan 15:04:05")
}

func (m cockpitModel) pageTabs(width int) string {
	position := pagePosition(m.page)
	start := 0
	if width < 110 {
		start = max(0, position-1)
	}
	var tabs []string
	if start > 0 {
		tabs = append(tabs, ink("‹", m.palette().Muted))
	}
	for i := start; i < len(flightPages); i++ {
		name := fmt.Sprintf("%02d/%s", i+1, strings.ToUpper(flightPages[i].Name))
		label := ink(" "+name+" ", m.palette().Muted)
		if i == position {
			label = m.chip(name, m.palette().Accent)
		}
		candidate := strings.Join(append(append([]string(nil), tabs...), label), " ")
		if ansi.StringWidth(candidate)+2 > width && i > position {
			tabs = append(tabs, ink("›", m.palette().Muted))
			break
		}
		tabs = append(tabs, label)
	}
	return padLine(strings.Join(tabs, " "), width)
}

func (m cockpitModel) journalStatus() string {
	if m.opts.mode == "mock" {
		return "SIMULATED / example journal data"
	}
	if m.companionError != "" {
		return safeText(m.companionError)
	}
	if m.companion.Warning != "" {
		return safeText(m.companion.Warning)
	}
	if m.companion.LastEvent.IsZero() {
		return "Waiting for Player Journal data"
	}
	prefix := "Last recorded"
	if m.presence.Known && !m.presence.Running {
		prefix = "Cached / game not running"
	}
	return prefix + " · " + reportedAt(m.companion.LastEvent)
}

func (m cockpitModel) shipOverviewSummary(width int) string {
	ship := m.companion.Ship
	name := ship.Name
	if name == "" {
		name = shipDisplayName(ship.Type)
	}
	hull := "unavailable"
	if ship.Hull != nil && *ship.Hull >= 0 && *ship.Hull <= 1 {
		hull = fmt.Sprintf("%.1f%%", *ship.Hull*100)
	}
	fuel, cargo := "unavailable in current context", "unavailable"
	if state := m.frame.State; state != nil && state.shipFeedbackKnown() {
		if state.Fuel != nil {
			fuel = fmt.Sprintf("%.1f t", state.Fuel.Main)
			if ship.Tank != nil {
				fuel += fmt.Sprintf(" / %.1f t", *ship.Tank)
			}
		}
		if state.Cargo != nil {
			cargo = fmt.Sprintf("%.0f t", *state.Cargo)
		}
	}
	if ship.CargoCapacity != nil {
		cargo += fmt.Sprintf(" / %d t capacity", *ship.CargoCapacity)
	}
	lines := []string{m.section("SHIP OVERVIEW"), ink(m.journalStatus(), m.palette().Muted), "",
		"Commander    " + reportedText(m.companion.Commander),
		"Ship         " + reportedText(name),
		"Type / ID    " + shipDisplayName(ship.Type) + " / " + reportedText(ship.Ident),
		"Game mode    " + reportedText(m.companion.GameMode),
		"Fuel         " + fuel, "Cargo        " + cargo,
		"Hull         " + hull + " · last reported " + reportedAt(ship.HullAt)}
	if ship.MaxJump != nil {
		lines = append(lines, fmt.Sprintf("Max jump     %.2f LY (unladen)", *ship.MaxJump))
	}
	if ship.Rebuy != nil {
		lines = append(lines, fmt.Sprintf("Rebuy        %.0f CR (last reported)", *ship.Rebuy))
	}
	lines = append(lines, "Loadout      "+reportedAt(ship.Updated))
	return m.panel(width, m.palette().Border, lines...)
}

func (m cockpitModel) shipOverviewPage(width int) string {
	ship := m.companion.Ship
	modules := []string{m.section(fmt.Sprintf("INSTALLED MODULES / %d", len(ship.Modules))),
		"Last loadout report: " + reportedAt(ship.ModulesAt),
		"Integrity is a snapshot, not a live damage feed.",
		"! KEY = damaged key system; CHECK = other damage."}
	if len(ship.Modules) == 0 {
		modules = append(modules, "Loadout details have not been reported yet.")
	}
	for _, row := range shipModuleRows(ship.Modules) {
		marker, colour := "    ", m.palette().Text
		if row.KeySystem {
			marker, colour = "KEY ", m.palette().Accent
		}
		switch row.Condition {
		case "critical":
			marker, colour = "! KEY ", m.palette().Danger
		case "damaged":
			marker, colour = "CHECK ", m.palette().Warning
		case "unknown":
			marker, colour = "? ", m.palette().Muted
		}
		modules = append(modules, ink(marker+row.Health+" · "+row.Power+" · "+row.Name, colour))
		modules = append(modules, ink("  "+row.Slot+" · "+row.Item, m.palette().Muted))
		if row.KeySystem {
			modules = append(modules, ink("  "+row.Consequence, colour))
		}
	}
	return m.shipOverviewSummary(width) + "\n" + m.panel(width, m.palette().Border, modules...)
}

func (nav navigationData) currentRouteIndex() int {
	if nav.System == "" {
		return -1
	}
	for i, point := range nav.Route {
		if strings.EqualFold(point.StarSystem, nav.System) {
			return i
		}
	}
	return -1
}

func (m cockpitModel) navigationPage(width int) string {
	if m.outfitting.Active {
		return m.outfittingPage(width)
	}
	if m.stationSearchActive {
		return m.stationSearchPanel(width)
	}
	nav := m.companion.Nav
	target, class := nav.Target, nav.TargetClass
	index := nav.currentRouteIndex()
	remaining := "unavailable"
	if nav.Remaining != nil {
		remaining = fmt.Sprint(*nav.Remaining)
	}
	if index >= 0 {
		remaining = fmt.Sprint(len(nav.Route) - index - 1)
		if target == "" && index+1 < len(nav.Route) {
			target, class = nav.Route[index+1].StarSystem, nav.Route[index+1].StarClass
		}
	}
	distance := "unavailable"
	for _, point := range nav.Route {
		if target != "" && strings.EqualFold(point.StarSystem, target) {
			if ly, ok := routeDistance(nav.Position, point.StarPos); ok {
				distance = fmt.Sprintf("%.2f LY", ly)
			}
		}
	}
	lines := []string{m.section("NAVIGATION"), ink(m.journalStatus(), m.palette().Muted), ink("O  Station outfitting    S  Browser search", m.palette().Accent),
		"Current system   " + reportedText(nav.System), "Body             " + reportedText(nav.Body), "Station          " + reportedText(nav.Station),
		"Next target      " + reportedText(target), "Star class       " + reportedText(class), "Distance         " + distance, "Remaining jumps  " + remaining}
	if target != "" && class != "" {
		scoopable := strings.Contains("KGBFOAM", strings.ToUpper(class)) && len(class) == 1
		if scoopable {
			lines = append(lines, "Target star      Scoopable class (requires a fuel scoop)")
		} else {
			lines = append(lines, "Target star      Scoopability not confirmed for this class")
		}
	}
	route := []string{m.section("PLOTTED ROUTE / LAST REPORTED"), "Route snapshot: " + reportedAt(nav.RouteAt)}
	if len(nav.Route) == 0 {
		route = append(route, "No plotted route reported. Plot one in the galaxy map.")
	} else {
		if index < 0 {
			route = append(route, "Current system is not on this cached route; replot in game.")
		}
		start := max(0, index)
		for i := start; i < len(nav.Route); i++ {
			point := nav.Route[i]
			marker := "○"
			if i == index {
				marker = "●"
			} else if i == len(nav.Route)-1 {
				marker = "◎"
			}
			leg := ""
			if i > 0 {
				if ly, ok := routeDistance(nav.Route[i-1].StarPos, point.StarPos); ok {
					leg = fmt.Sprintf(" · %.2f LY", ly)
				}
			}
			route = append(route, marker+" "+safeText(point.StarSystem)+" ["+reportedText(point.StarClass)+"]"+leg)
		}
	}
	return m.panel(width, m.palette().Border, lines...) + "\n" + m.panel(width, m.palette().Border, route...)
}

func (m cockpitModel) sortedMissions() []missionData {
	missions := make([]missionData, 0, len(m.companion.Missions))
	for _, mission := range m.companion.Missions {
		missions = append(missions, mission)
	}
	sort.Slice(missions, func(i, j int) bool {
		if missions[i].Expiry.Equal(missions[j].Expiry) {
			return missions[i].MissionID < missions[j].MissionID
		}
		if missions[i].Expiry.IsZero() {
			return false
		}
		if missions[j].Expiry.IsZero() {
			return true
		}
		return missions[i].Expiry.Before(missions[j].Expiry)
	})
	return missions
}

func missionDeadline(expiry, now time.Time) string {
	if expiry.IsZero() {
		return "unavailable"
	}
	left := expiry.Sub(now)
	if left <= 0 {
		return "elapsed · awaiting game mission update"
	}
	return fmt.Sprintf("%dd %02dh %02dm", int(left.Hours())/24, int(left.Hours())%24, int(left.Minutes())%60)
}

func (m cockpitModel) missionBoardPage(width int) string {
	missions := m.sortedMissions()
	lines := []string{m.section(fmt.Sprintf("MISSION BOARD / %d RECORDED ACTIVE", len(missions))), ink(m.journalStatus(), m.palette().Muted)}
	if !m.companion.MissionsKnown {
		lines = append(lines, "Startup mission summary not yet seen; list may be incomplete.")
	}
	if len(missions) == 0 {
		if m.companion.MissionsKnown {
			lines = append(lines, "", "No active missions reported by the game.")
		} else {
			lines = append(lines, "", "Enter the game to load your mission summary.")
		}
	}
	for _, mission := range missions {
		name := mission.LocalisedName
		if name == "" {
			name = symbolText(mission.Name)
		}
		lines = append(lines, "", bold(reportedText(name), m.palette().Accent),
			fmt.Sprintf("Mission ID   %d", mission.MissionID),
			"Destination  "+reportedText(mission.DestinationSystem)+" / "+reportedText(mission.DestinationStation),
			"Deadline     "+missionDeadline(mission.Expiry, m.now))
		if mission.Reward != nil {
			lines = append(lines, fmt.Sprintf("Reward       %d CR", *mission.Reward))
		}
		if mission.Count != nil {
			progress := "delivery progress not reported"
			if mission.Delivered != nil {
				progress = fmt.Sprintf("%d / %d delivered", *mission.Delivered, *mission.Count)
			}
			lines = append(lines, "Cargo        "+reportedText(mission.Commodity)+" · "+progress)
			if mission.Delivered != nil && *mission.Delivered >= *mission.Count {
				lines = append(lines, "Delivery recorded; check in-game mission hand-in.")
			}
		}
		if !mission.Details {
			lines = append(lines, "Acceptance details are outside the loaded journal history.")
		}
	}
	return m.panel(width, m.palette().Border, lines...)
}

func (m cockpitModel) engineeringPage(width int) string {
	rows := m.materialRows()
	index := min(max(0, m.materialIndex), max(0, len(rows)-1))
	view := "ALL MATERIALS / targets first"
	if m.onlyPinned {
		view = "PINNED TARGETS"
	}
	search := m.materialFilter
	if search == "" {
		search = "type a material name…"
	}
	lines := []string{m.section("ENGINEERING // MATERIAL TERMINAL"), "SEARCH > " + safeText(search) + "▏", view + " · F2 switches list",
		ink("Material                       Owned / target / need", m.palette().Muted)}
	visible := max(1, m.height-16)
	start := max(0, index-visible+1)
	for i := start; i < min(len(rows), start+visible); i++ {
		row := rows[i]
		marker, colour := "[ ]", m.palette().Text
		if row.Target == 0 {
			marker = " · "
		} else if row.Known && row.Count >= row.Target {
			marker, colour = "[✓]", m.palette().Success
		}
		cursor := " "
		if i == index {
			cursor, colour = ">", m.palette().Accent
		}
		owned := "?"
		if row.Known {
			owned = fmt.Sprint(row.Count)
		}
		value := owned + " owned"
		if row.Target > 0 {
			need := "?"
			if row.Known {
				need = fmt.Sprint(max(0, row.Target-row.Count))
			}
			value = fmt.Sprintf("%s / %d / %s", owned, row.Target, need)
		}
		labelWidth := max(1, width-5-ansi.StringWidth(value)-1)
		left := ansi.Truncate(cursor+marker+" "+safeText(row.Display), labelWidth, "…")
		lines = append(lines, ink(between(left, value, width-4), colour))
	}
	if len(rows) == 0 {
		lines = append(lines, "No matches. Esc clears search. F2 shows all/pinned.")
	}
	detail := "↑↓ select · Enter target · +/- adjust · Delete remove"
	if len(rows) > 0 {
		row := rows[index]
		detail = fmt.Sprintf("%d/%d · %s · Enter sets a material target", index+1, len(rows), row.Category)
	}
	lines = append(lines, ink(safeText(detail), m.palette().Muted))
	if m.goalEditing {
		lines = append(lines, bold("TARGET > "+string(m.goalDigits)+"▏  Enter save / Esc cancel", m.palette().Accent))
	}
	return m.panel(width, m.palette().Border, lines...)
}

func mockCompanionData(now time.Time) companionData {
	d := newCompanionData()
	d.Commander, d.GameMode, d.Source, d.LastEvent = "DEMO COMMANDER", "SIMULATED", "SIMULATED", now
	hull, tank, jump, rebuy := .94, 32.0, 27.4, 1200000.0
	healthy, plantHealth, shieldHealth := 1.0, .87, .76
	d.Ship = shipOverview{Type: "Krait Mk II", Name: "Wayfarer", Ident: "DEMO-01", Hull: &hull, Tank: &tank, MaxJump: &jump, Rebuy: &rebuy, Updated: now, HullAt: now}
	d.Ship.ModulesAt = now
	d.Ship.Modules = []shipModule{
		{Slot: "PowerPlant", Item: "int_powerplant_size6_class5", On: true, Health: &plantHealth},
		{Slot: "MainEngines", Item: "int_engine_size6_class5", On: true, Health: &healthy},
		{Slot: "FrameShiftDrive", Item: "int_hyperdrive_size5_class5", On: true, Health: &healthy},
		{Slot: "LifeSupport", Item: "int_lifesupport_size4_class1", On: true},
		{Slot: "Slot01_Size6", Item: "int_shieldgenerator_size6_class5", On: true, Health: &shieldHealth},
		{Slot: "LargeHardpoint1", Item: "hpt_pulselaser_fixed_large", On: true, Health: &hull},
		{Slot: "LargeHardpoint2", Item: "hpt_multicannon_gimbal_large", On: true, Health: &hull, AmmoInClip: integer(90), AmmoInHopper: integer(1800)},
		{Slot: "MediumHardpoint1", Item: "hpt_beamlaser_turret_medium", On: true, Health: &hull},
		{Slot: "TinyHardpoint1", Item: "hpt_heatsinklauncher_turret_tiny", On: true, AmmoInClip: integer(1), AmmoInHopper: integer(2)},
	}
	d.Nav = navigationData{System: "Example System", Target: "Example Destination", TargetClass: "K", Position: []float64{0, 0, 0},
		Route: []routePoint{{StarSystem: "Example System", StarClass: "G", StarPos: []float64{0, 0, 0}}, {StarSystem: "Example Destination", StarClass: "K", StarPos: []float64{18.6, 0, 0}}}, RouteAt: now}
	count, delivered := 36, 24
	d.Missions[1] = missionData{MissionID: 1, LocalisedName: "Deliver medical supplies (example)", DestinationSystem: "Example Destination", DestinationStation: "Example Station", Commodity: "Medical supplies", Count: &count, Delivered: &delivered, Expiry: now.Add(134 * time.Minute), Details: true}
	d.MissionsKnown = true
	for _, category := range []string{"raw", "manufactured", "encoded"} {
		d.MaterialCategories[category] = true
	}
	d.stockDelta("raw", "arsenic", "Arsenic", 12)
	d.stockDelta("manufactured", "chemicalmanipulators", "Chemical Manipulators", 8)
	d.stockDelta("encoded", "dataminedwake", "Datamined Wake Exceptions", 3)
	return d
}
