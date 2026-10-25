//go:build desktop

package main

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

type desktopField struct{ Label, Value string }
type desktopPanel struct {
	Title, Hint string
	Fields      []desktopField
	Columns     []string
	Rows        [][]string
	Lines       []string
	Collapsible bool
}
type desktopHardpoint struct {
	Key, Name, Slot, Power, Health, Ammo, Priority string
	Groups                                         []int
}
type desktopPageView struct {
	Source     string
	Panels     []desktopPanel
	Hardpoints []desktopHardpoint
}

func field(label, value string) desktopField { return desktopField{label, reportedText(value)} }
func optionalNumber(value *float64, format string) string {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return "unavailable"
	}
	return fmt.Sprintf(format, *value)
}
func (m cockpitModel) desktopPageView() desktopPageView {
	view := desktopPageView{Source: m.journalStatus(), Panels: []desktopPanel{}, Hardpoints: []desktopHardpoint{}}
	switch m.page {
	case pageShip:
		s := m.companion.Ship
		name := s.Name
		if name == "" {
			name = shipDisplayName(s.Type)
		}
		fuel, cargo := "unavailable in current context", "unavailable"
		if state := m.frame.State; state != nil && state.shipFeedbackKnown() {
			if state.Fuel != nil {
				fuel = fmt.Sprintf("%.1f t", state.Fuel.Main)
				if s.Tank != nil {
					fuel += fmt.Sprintf(" / %.1f t", *s.Tank)
				}
			}
			if state.Cargo != nil {
				cargo = fmt.Sprintf("%.0f t", *state.Cargo)
			}
		}
		if s.CargoCapacity != nil {
			cargo += fmt.Sprintf(" / %d t capacity", *s.CargoCapacity)
		}
		hull := "unavailable"
		if s.Hull != nil && *s.Hull >= 0 && *s.Hull <= 1 {
			hull = fmt.Sprintf("%.1f%%", *s.Hull*100)
		}
		view.Panels = append(view.Panels, desktopPanel{Title: "Ship overview", Hint: "Hull and loadout values are last reported journal data.", Fields: []desktopField{
			field("Commander", m.companion.Commander), field("Ship", name), field("Type", shipDisplayName(s.Type)), field("Registration", s.Ident),
			field("Game mode", m.companion.GameMode), field("Fuel", fuel), field("Cargo", cargo), field("Hull", hull),
			field("Hull report", reportedAt(s.HullAt)), field("Maximum jump", optionalNumber(s.MaxJump, "%.2f LY")),
			field("Rebuy", optionalNumber(s.Rebuy, "%.0f CR")), field("Loadout report", reportedAt(s.ModulesAt)),
		}})
	case pageNavigation:
		n := m.companion.Nav
		target, class := n.Target, n.TargetClass
		index := n.currentRouteIndex()
		remaining := "unavailable"
		if n.Remaining != nil {
			remaining = fmt.Sprint(*n.Remaining)
		}
		if index >= 0 {
			remaining = fmt.Sprint(len(n.Route) - index - 1)
			if target == "" && index+1 < len(n.Route) {
				target, class = n.Route[index+1].StarSystem, n.Route[index+1].StarClass
			}
		}
		distance := "unavailable"
		for _, point := range n.Route {
			if strings.EqualFold(point.StarSystem, target) && target != "" {
				if ly, ok := routeDistance(n.Position, point.StarPos); ok {
					distance = fmt.Sprintf("%.2f LY", ly)
				}
			}
		}
		view.Panels = append(view.Panels, desktopPanel{Title: "Navigation", Fields: []desktopField{
			field("Current system", n.System), field("Body", n.Body), field("Station", n.Station), field("Next target", target), field("Star class", class), field("Distance", distance), field("Remaining jumps", remaining),
		}})
		route := desktopPanel{Title: "Plotted route", Hint: "Last route report: " + reportedAt(n.RouteAt), Columns: []string{"System", "Star class", "Leg distance", "Position"}, Rows: [][]string{}}
		if len(n.Route) == 0 {
			route.Hint += " · No plotted route reported."
		} else if index < 0 {
			route.Hint += " · Current system is not on this cached route; replot in game."
		}
		start := max(0, index)
		for i := start; i < min(len(n.Route), start+100); i++ {
			point := n.Route[i]
			leg, status := "—", "Ahead"
			if i == index {
				status = "Current"
			} else if i == len(n.Route)-1 {
				status = "Destination"
			}
			if i > 0 {
				if ly, ok := routeDistance(n.Route[i-1].StarPos, point.StarPos); ok {
					leg = fmt.Sprintf("%.2f LY", ly)
				}
			}
			route.Rows = append(route.Rows, []string{reportedText(point.StarSystem), reportedText(point.StarClass), leg, status})
		}
		if len(n.Route)-start > 100 {
			route.Hint += " · Showing the next 100 route entries."
		}
		view.Panels = append(view.Panels, route)
	case pageMissions:
		missions := m.sortedMissions()
		overview := desktopPanel{Title: "Accepted missions", Hint: "Journal-recorded missions; this is not a live station mission board."}
		if !m.companion.MissionsKnown {
			overview.Hint += " The startup summary is not yet known, so this list may be incomplete."
		}
		if len(missions) == 0 {
			overview.Lines = []string{"No active missions reported."}
			if !m.companion.MissionsKnown {
				overview.Lines = []string{"Waiting for the game's mission summary."}
			}
		}
		view.Panels = append(view.Panels, overview)
		for _, mission := range missions {
			name := mission.LocalisedName
			if name == "" {
				name = symbolText(mission.Name)
			}
			card := desktopPanel{Title: reportedText(name), Fields: []desktopField{
				field("Mission ID", fmt.Sprint(mission.MissionID)), field("System", mission.DestinationSystem), field("Station", mission.DestinationStation),
				field("Time remaining", missionDeadline(mission.Expiry, m.now)),
			}}
			if mission.Reward != nil {
				card.Fields = append(card.Fields, field("Reward", fmt.Sprintf("%d CR", *mission.Reward)))
			}
			if mission.Count != nil {
				progress := "delivery progress not reported"
				if mission.Delivered != nil {
					progress = fmt.Sprintf("%d / %d delivered", *mission.Delivered, *mission.Count)
				}
				card.Fields = append(card.Fields, field("Cargo", mission.Commodity), field("Progress", progress))
			}
			if !mission.Details {
				card.Hint = "Acceptance details are outside the loaded journal history."
			}
			view.Panels = append(view.Panels, card)
		}
	case pageHardpoints:
		active := "Active fire group unavailable"
		if s := m.frame.State; s != nil && s.shipFeedbackKnown() && s.FireGroup != nil && *s.FireGroup >= 0 && *s.FireGroup < 26 {
			active = fmt.Sprintf("Last reported active group: %c", 'A'+*s.FireGroup)
		}
		view.Panels = append(view.Panels, desktopPanel{Title: "Hardpoints and fire groups", Hint: active, Lines: []string{
			"Group selectors save your manual reference only. They do not change game fire-group assignments.",
			"Loadout and ammunition snapshot: " + reportedAt(m.companion.Ship.ModulesAt),
		}})
		reference := m.firegroups.Ships[m.fireGroupShipKey()]
		for _, module := range m.hardpointModules() {
			row := desktopHardpoint{Key: hardpointModuleKey(module), Name: hardpointLabel(module), Slot: safeText(module.Slot), Power: "Off", Health: shipModuleRows([]shipModule{module})[0].Health, Ammo: "not reported", Priority: "unknown", Groups: make([]int, 8)}
			if module.On {
				row.Power = "On"
			}
			if module.Priority != nil {
				row.Priority = fmt.Sprint(*module.Priority + 1)
			}
			if module.AmmoInClip != nil || module.AmmoInHopper != nil {
				clip, reserve := "?", "?"
				if module.AmmoInClip != nil {
					clip = fmt.Sprint(*module.AmmoInClip)
				}
				if module.AmmoInHopper != nil {
					reserve = fmt.Sprint(*module.AmmoInHopper)
				}
				row.Ammo = clip + " / " + reserve
			}
			for g := 0; g < 8; g++ {
				row.Groups[g] = -1
				if value, ok := reference[row.Key][string(rune('A'+g))]; ok {
					row.Groups[g] = value
				}
			}
			view.Hardpoints = append(view.Hardpoints, row)
		}
	case pageDiagnostics:
		focus := "Another window"
		if m.frame.GameFocused {
			focus = "Elite Dangerous"
		}
		process := "Checking"
		if m.presence.Checked {
			process = "Not running"
			if !m.presence.Known {
				process = "Unknown; telemetry fallback"
			} else if m.presence.Running {
				process = "Running"
			}
		}
		view.Panels = append(view.Panels, desktopPanel{Title: "Connection", Fields: []desktopField{
			field("Mode", m.opts.mode), field("Control state", m.frame.ControlStatus), field("Focused window", focus), field("Game process", process),
			field("Telemetry age", m.age()), field("Source quality", m.frame.Quality), field("Page shortcuts", m.frame.PageStatus), field("Process check", m.presence.Error),
		}})
		view.Panels = append(view.Panels, desktopPanel{Title: "Data sources", Collapsible: true, Fields: []desktopField{
			field("Status file", m.opts.path), field("Journal folder", m.opts.journal), field("Journal source", m.companion.Source), field("Bindings configuration", m.opts.config), field("Engineering targets", m.opts.goals),
		}})
		if s := m.frame.State; s != nil {
			fields := []desktopField{field("Game timestamp", s.Timestamp.UTC().Format("2006-01-02 15:04:05 UTC")), field("GUI focus", derefInt(s.GuiFocus)), field("Flags2", derefU64(s.Flags2))}
			if s.Flags != nil {
				fields = append(fields, field("Flags", fmt.Sprintf("0x%08X", *s.Flags)))
			}
			view.Panels = append(view.Panels, desktopPanel{Title: "Telemetry snapshot", Fields: fields})
		}
	case pageHelp:
		view.Source = "Co-pit flight companion"
		view.Panels = []desktopPanel{
			{Title: "Getting started", Lines: []string{"Start Elite Dangerous and select the EliteAuxPanel control preset.", "Focus Elite and press Ctrl+Alt+F9 to arm. Ctrl+Alt+F10 or STOP cancels controls.", "Changing focus pauses input; returning to the same authorised game resumes when checks pass."}},
			{Title: "Your cockpit", Lines: []string{"Permanent ship controls are always present, in separate Toggles and Activatable buttons groups.", "Use + for supported fitted utilities, or enable automatic adding in Settings.", "A retained unavailable utility pulses dim red and cannot be clicked. Unknown availability stays unknown.", "Each tile shows its action's existing numpad key. Editing tiles does not rewrite game bindings."}},
			{Title: "Pages and shortcuts", Lines: []string{"Use the page buttons, Ctrl+Tab or numpad plus/minus to move between pages.", "Ship and module condition use journal snapshots, with report times shown.", "Navigation can load community-reported station outfitting on request.", "Engineering: search, select a material, enter a target and save.", "Fire-group selectors are a manual reference; configure actual fire groups in game."}},
			{Title: "Display", Lines: []string{"F11 enables borderless fullscreen for this session; Escape restores a window.", "Every launch starts windowed. Other display preferences remain saved.", "Settings includes row width, theme, portrait layout and adjustable scan-line spacing."}},
		}
	}
	return view
}

func (a *DesktopApp) SetFireGroupReference(key string, group, trigger int) error {
	if group < 0 || group >= 8 || trigger < -1 || trigger > 2 {
		return errors.New("invalid fire group or trigger")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return errors.New("application is closing")
	}
	if a.model.fireGroupLoadError || a.model.fireGroupShipKey() == "" {
		return errors.New("an identified, valid ship reference is required")
	}
	for index, module := range a.model.hardpointModules() {
		if hardpointModuleKey(module) != key {
			continue
		}
		a.model.hardpointIndex, a.model.groupIndex = index, group
		a.model.saveFireGroup(module, trigger)
		if strings.HasPrefix(a.model.fireGroupMessage, "Saved") || strings.HasPrefix(a.model.fireGroupMessage, "SIMULATED") {
			return nil
		}
		return errors.New(a.model.fireGroupMessage)
	}
	return errors.New("module is no longer in the reported loadout")
}
