package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// These are user-maintained references: the journal does not export trigger assignments.
type fireGroupFile struct {
	Version int                                  `json:"version"`
	Ships   map[string]map[string]map[string]int `json:"ships"`
}

func emptyFireGroups() fireGroupFile {
	return fireGroupFile{Version: 1, Ships: make(map[string]map[string]map[string]int)}
}

func loadFireGroups(path string) (fireGroupFile, error) {
	result := emptyFireGroups()
	if path == "" {
		return result, nil
	}
	data, err := readSmallFile(path, 1<<20)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return emptyFireGroups(), err
	}
	if err = validateFireGroups(result); err != nil {
		return emptyFireGroups(), err
	}
	if result.Ships == nil {
		result.Ships = make(map[string]map[string]map[string]int)
	}
	return result, nil
}

func validateFireGroups(data fireGroupFile) error {
	if data.Version != 1 || len(data.Ships) > 256 {
		return errors.New("unsupported or oversized fire-group reference")
	}
	for ship, modules := range data.Ships {
		if len(ship) == 0 || len(ship) > 512 || len(modules) > 128 {
			return errors.New("invalid ship reference")
		}
		for module, groups := range modules {
			if len(module) == 0 || len(module) > 512 || len(groups) > 8 {
				return errors.New("invalid module reference")
			}
			for group, trigger := range groups {
				if len(group) != 1 || group[0] < 'A' || group[0] > 'H' || trigger < 0 || trigger > 2 {
					return errors.New("invalid group or trigger")
				}
			}
		}
	}
	return nil
}

func (m cockpitModel) fireGroupShipKey() string {
	identity := m.companion.FID
	if identity == "" {
		identity = m.companion.Commander
	}
	ship := m.companion.Ship
	if identity == "" || ship.Type == "" || ship.ModulesAt.IsZero() {
		return ""
	}
	// Account and ship identity prevent another commander or ship inheriting a reference.
	key, _ := json.Marshal([]string{identity, strings.ToLower(ship.Type), fmt.Sprint(ship.ID)})
	return string(key)
}

func hardpointModuleKey(module shipModule) string {
	return strings.ToLower(module.Slot) + "/" + strings.ToLower(module.Item)
}

func hardpointSize(slot string) int {
	slot = strings.ToLower(slot)
	for i, prefix := range []string{"huge", "large", "medium", "small", "tiny"} {
		if strings.HasPrefix(slot, prefix+"hardpoint") {
			return i
		}
	}
	return 5
}

func (m cockpitModel) hardpointModules() []shipModule {
	var modules []shipModule
	for _, module := range m.companion.Ship.Modules {
		if strings.Contains(strings.ToLower(module.Slot), "hardpoint") {
			modules = append(modules, module)
		}
	}
	sort.SliceStable(modules, func(i, j int) bool {
		a, b := hardpointSize(modules[i].Slot), hardpointSize(modules[j].Slot)
		if a != b {
			return a < b
		}
		return modules[i].Slot < modules[j].Slot
	})
	return modules
}

func hardpointLabel(module shipModule) string {
	text := strings.ToLower(module.Item)
	text = strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(text, "$"), ";"), "hpt_")
	replacer := strings.NewReplacer(
		"basicmissilerack", "Missile rack", "pulselaser", "Pulse laser", "burstlaser", "Burst laser", "beamlaser", "Beam laser",
		"multicannon", "Multi-cannon", "plasmaaccelerator", "Plasma accelerator", "railgun", "Rail gun",
		"heatsinklauncher", "Heat sink", "chafflauncher", "Chaff", "shieldbooster", "Shield booster",
		"plasmapointdefence", "Point defence", "crimescanner", "Kill warrant scanner", "mininglaser", "Mining laser",
		"gimbal", "Gimballed", "turret", "Turreted", "fixed", "Fixed", "_", " ")
	return safeText(replacer.Replace(text))
}

func (m *cockpitModel) hardpointKey(key tea.Key) bool {
	if key.Mod != 0 {
		return false
	}
	modules := m.hardpointModules()
	switch key.Code {
	case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown:
		step := 1
		if key.Code == tea.KeyPgUp || key.Code == tea.KeyPgDown {
			step = 8
		}
		if key.Code == tea.KeyUp || key.Code == tea.KeyPgUp {
			step = -step
		}
		m.hardpointIndex = min(max(0, m.hardpointIndex+step), max(0, len(modules)-1))
		m.scroll = 0
		return true
	case tea.KeyLeft:
		m.groupIndex = (m.groupIndex + 7) % 8
		return true
	case tea.KeyRight:
		m.groupIndex = (m.groupIndex + 1) % 8
		return true
	case '0', '1', '2', tea.KeyDelete, tea.KeyBackspace:
		if key.IsRepeat {
			return true
		}
		if len(modules) == 0 || m.fireGroupShipKey() == "" {
			m.fireGroupMessage = "Waiting for an identified ship loadout."
			return true
		}
		if m.fireGroupLoadError {
			m.fireGroupMessage = "Reference file could not be loaded; fix it before saving."
			return true
		}
		trigger := -1
		if key.Code >= '0' && key.Code <= '2' {
			trigger = int(key.Code - '0')
		}
		module := modules[min(max(0, m.hardpointIndex), len(modules)-1)]
		m.saveFireGroup(module, trigger)
		return true
	}
	return false
}

func (m *cockpitModel) saveFireGroup(module shipModule, trigger int) {
	// Copy before editing, so a failed write never changes the displayed saved reference.
	data, err := json.Marshal(m.firegroups)
	if err != nil {
		m.fireGroupMessage = "Could not save: " + err.Error()
		return
	}
	var next fireGroupFile
	if err = json.Unmarshal(data, &next); err != nil {
		m.fireGroupMessage = "Could not save: " + err.Error()
		return
	}
	shipKey, moduleKey, group := m.fireGroupShipKey(), hardpointModuleKey(module), string(rune('A'+m.groupIndex))
	if next.Ships[shipKey] == nil {
		next.Ships[shipKey] = make(map[string]map[string]int)
	}
	if next.Ships[shipKey][moduleKey] == nil {
		next.Ships[shipKey][moduleKey] = make(map[string]int)
	}
	if trigger < 0 {
		delete(next.Ships[shipKey][moduleKey], group)
	} else {
		next.Ships[shipKey][moduleKey][group] = trigger
	}
	if err = validateFireGroups(next); err == nil && m.opts.mode != "mock" {
		err = writeJSONAtomic(m.opts.firegroups, next)
	}
	if err != nil {
		m.fireGroupMessage = "Could not save: " + err.Error()
		return
	}
	m.firegroups = next
	m.fireGroupMessage = "Saved reference · game assignments unchanged"
	if m.opts.mode == "mock" {
		m.fireGroupMessage = "SIMULATED reference · not saved to disk"
	}
}

func (m cockpitModel) hardpointsPage(width int) string {
	modules := m.hardpointModules()
	active := "Selected group / deployment: unavailable"
	if state := m.frame.State; state != nil && state.shipFeedbackKnown() && state.FireGroup != nil && *state.FireGroup >= 0 && *state.FireGroup < 26 {
		deployment := "STOWED"
		if state.has(hardpoints) {
			deployment = "DEPLOYED"
		}
		active = fmt.Sprintf("Last reported group %c · hardpoints %s", 'A'+*state.FireGroup, deployment)
	}
	lines := []string{m.section("HARDPOINTS / FIRE GROUPS"), ink(active, m.palette().Info),
		"Manual reference: ? unknown  - none  1 primary  2 secondary"}
	labelWidth := max(1, width-4-27)
	header := padLine("Installed module", labelWidth)
	for group := 0; group < 8; group++ {
		label := fmt.Sprintf(" %c ", 'A'+group)
		if group == m.groupIndex {
			label = "[" + string(rune('A'+group)) + "]"
		}
		header += label
	}
	lines = append(lines, ink(header, m.palette().Muted))
	if len(modules) == 0 {
		message := "Waiting for a reported ship loadout."
		if !m.companion.Ship.ModulesAt.IsZero() {
			message = "No installed hardpoints or utilities in the reported loadout."
		}
		lines = append(lines, message, ink(m.journalStatus(), m.palette().Muted))
		return m.panel(width, m.palette().Border, lines...)
	}
	index := min(max(0, m.hardpointIndex), len(modules)-1)
	visible := max(1, m.height-19)
	start := max(0, index-visible+1)
	reference := m.firegroups.Ships[m.fireGroupShipKey()]
	for i := start; i < min(len(modules), start+visible); i++ {
		module := modules[i]
		cursor, colour := " ", m.palette().Text
		if i == index {
			cursor, colour = ">", m.palette().Accent
		}
		row := padLine(ansi.Truncate(cursor+hardpointLabel(module), labelWidth, "…"), labelWidth)
		for group := 0; group < 8; group++ {
			value := "?"
			if trigger, ok := reference[hardpointModuleKey(module)][string(rune('A'+group))]; ok {
				value = fmt.Sprint(trigger)
				if trigger == 0 {
					value = "-"
				}
			}
			if i == index && group == m.groupIndex {
				row += "[" + value + "]"
			} else {
				row += " " + value + " "
			}
		}
		lines = append(lines, ink(row, colour))
	}
	module := modules[index]
	state, health, ammo, priority := "OFF", "?", "not reported", "?"
	if module.On {
		state = "ON"
	}
	if module.Health != nil {
		health = fmt.Sprintf("%.0f%%", *module.Health*100)
	}
	if module.AmmoInClip != nil || module.AmmoInHopper != nil {
		clip, reserve := "?", "?"
		if module.AmmoInClip != nil {
			clip = fmt.Sprint(*module.AmmoInClip)
		}
		if module.AmmoInHopper != nil {
			reserve = fmt.Sprint(*module.AmmoInHopper)
		}
		ammo = clip + " / " + reserve
	}
	if module.Priority != nil {
		priority = fmt.Sprint(*module.Priority + 1)
	}
	lines = append(lines,
		fmt.Sprintf("%d/%d · %s · %s", index+1, len(modules), safeText(module.Slot), hardpointLabel(module)),
		fmt.Sprintf("%s · health %s · priority %s · ammo %s", state, health, priority, ammo),
		"Loadout / ammo last reported: "+reportedAt(m.companion.Ship.ModulesAt),
		ink("Groups are your reference; ammo is not a live counter.", m.palette().Muted))
	return m.panel(width, m.palette().Border, lines...)
}
