package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *cockpitModel) outfittingKey(key tea.Key) (bool, tea.Cmd) {
	o := &m.outfitting
	if !o.Active {
		if key.Mod == 0 && (key.Code == 'o' || key.Code == 'O') && !key.IsRepeat {
			m.showOutfitting()
			return true, nil
		}
		return false, nil
	}
	if key.Code == tea.KeyTab || key.Code == tea.KeyF1 || key.Code == tea.KeyF4 || key.Code >= tea.KeyF5 && key.Code <= tea.KeyF10 {
		m.closeOutfitting()
		return false, nil
	}
	if key.Mod == tea.ModCtrl && (key.Code == 'r' || key.Code == 'R') {
		if o.Stage == "modules" {
			return true, m.lookupModules(o.SelectedKey, true)
		}
		return true, m.lookupStations(o.Input, true)
	}
	if key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 {
		return false, nil
	}
	switch key.Code {
	case tea.KeyEscape:
		if o.Loading {
			o.cancelRequest()
			o.Message = "Search cancelled"
			return true, nil
		}
		if o.editSystem || o.Stage == "system" {
			m.closeOutfitting()
			return true, nil
		}
		if o.Stage == "modules" {
			o.Stage = "stations"
			o.Error = ""
			o.Filter = ""
			return true, nil
		}
		o.editSystem, o.replaceInput = true, true
		return true, nil
	case tea.KeyF2:
		o.cancelRequest()
		o.editSystem, o.replaceInput = true, true
		return true, nil
	case tea.KeyF3:
		if o.Stage == "modules" {
			index := 0
			for i, category := range outfitCategories {
				if category == o.Category {
					index = i
					break
				}
			}
			o.Category = outfitCategories[(index+1)%len(outfitCategories)]
			o.ModuleOffset = 0
		}
		return true, nil
	case tea.KeyEnter, tea.KeyKpEnter:
		if key.IsRepeat || o.Loading {
			return true, nil
		}
		if o.editSystem || o.Stage == "system" {
			return true, m.lookupStations(o.Input, false)
		}
		if o.Stage == "stations" {
			rows := o.stationRows()
			if len(rows) > 0 {
				return true, m.lookupModules(rows[min(o.StationIndex, len(rows)-1)].Key, false)
			}
		}
		return true, nil
	case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown:
		if o.editSystem {
			return true, nil
		}
		step := 1
		if key.Code == tea.KeyPgUp || key.Code == tea.KeyPgDown {
			step = 8
		}
		if key.Code == tea.KeyUp || key.Code == tea.KeyPgUp {
			step = -step
		}
		if o.Stage == "stations" {
			o.StationIndex = min(max(0, o.StationIndex+step), max(0, len(o.stationRows())-1))
		} else {
			o.ModuleOffset = min(max(0, o.ModuleOffset+step), max(0, len(o.moduleRows())-1))
		}
		return true, nil
	case tea.KeyBackspace:
		field := &o.StationFilter
		if o.editSystem {
			field = &o.Input
			o.replaceInput = false
		} else if o.Stage == "modules" {
			field = &o.Filter
		}
		if runes := []rune(*field); len(runes) > 0 {
			*field = string(runes[:len(runes)-1])
		}
		o.StationIndex, o.ModuleOffset = 0, 0
		return true, nil
	}
	if key.Text != "" {
		field := &o.StationFilter
		if o.editSystem {
			field = &o.Input
			if o.replaceInput {
				*field = ""
				o.replaceInput = false
			}
		} else if o.Stage == "modules" {
			field = &o.Filter
		}
		*field = safeText(*field + key.Text)
		if runes := []rune(*field); len(runes) > 128 {
			*field = string(runes[:128])
		}
		o.StationIndex, o.ModuleOffset = 0, 0
		return true, nil
	}
	return false, nil
}

func (m cockpitModel) outfittingPage(width int) string {
	o := m.outfitting
	label := "STATION OUTFITTING // EDSM"
	if o.Simulated || m.opts.mode == "mock" {
		label = "STATION OUTFITTING // SIMULATED"
	}
	input := o.Input
	if input == "" {
		input = "type system name"
	}
	if o.editSystem {
		input += "▏"
	}
	lines := []string{m.section(label), "SYSTEM > " + safeText(input)}
	if o.editSystem || o.Stage == "system" {
		lines = append(lines, "Enter loads stations · F2 edits the system", ink("Public lookup runs only when requested.", m.palette().Muted))
	}
	if o.Loading {
		lines = append(lines, bold("Loading from EDSM…  Esc cancels", m.palette().Accent))
	}
	if o.Error != "" {
		lines = append(lines, ink(o.Error, m.palette().Danger))
	}
	if !o.Loading && !o.editSystem && o.Stage == "stations" {
		rows := o.stationRows()
		lines = append(lines, "STATIONS > "+safeText(o.StationFilter)+"▏  (type to filter)")
		visible := max(1, m.height-17)
		start := max(0, o.StationIndex-visible+1)
		for i := start; i < min(len(rows), start+visible); i++ {
			station := rows[i]
			mark := "  "
			if i == o.StationIndex {
				mark = "> "
			}
			service := "outfitting not reported"
			if station.HaveOutfitting {
				service = "outfitting"
			}
			line := mark + station.Name + " · " + service
			if i == o.StationIndex {
				line = bold(line, m.palette().Accent)
			}
			lines = append(lines, ansi.Truncate(line, max(1, width-4), "…"))
		}
		if len(rows) == 0 {
			lines = append(lines, "No matching stations reported.")
		}
		lines = append(lines, fmt.Sprintf("%d stations · Enter opens selected outfitting", len(rows)))
	}
	if !o.Loading && !o.editSystem && o.Stage == "modules" {
		station, _ := o.selectedStation()
		lines = append(lines, safeText(station.Name), ink(outfitUpdateLabel(station.Updated, m.now), m.palette().Warning), "FILTER > "+safeText(o.Filter)+"▏  e.g. 5A drive")
		rows := o.moduleRows()
		lines = append(lines, fmt.Sprintf("%s · %d / %d modules", o.Category, len(rows), len(o.Modules)))
		visible := max(1, m.height-19)
		start := min(o.ModuleOffset, max(0, len(rows)-visible))
		for _, module := range rows[start:min(len(rows), start+visible)] {
			name := module.Name
			if module.Ship != "" {
				name += " · " + module.Ship
			}
			lines = append(lines, ansi.Truncate(name, max(1, width-4), "…"))
		}
		if len(rows) == 0 {
			lines = append(lines, "No matching module data reported.")
		}
	}
	return m.panel(width, m.palette().Border, lines...)
}

func (m cockpitModel) outfittingFooter(width int) string {
	o := m.outfitting
	status := "Last-reported availability · prices/quantities unavailable"
	if o.Error != "" {
		status = o.Error
	} else if o.Message != "" {
		status = o.Message
	}
	stop := "Ctrl+C exit"
	if m.opts.mode == "control" {
		stop = "Ctrl+Alt+F9 arm/disarm  Ctrl+Alt+F10 STOP"
	}
	keys := "Type system  Enter search  Esc back"
	if !o.editSystem && o.Stage == "stations" {
		keys = "Type filter  ↑↓ select  Enter modules  F2 system  Esc back"
	}
	if !o.editSystem && o.Stage == "modules" {
		keys = "Type filter  ↑↓ scroll  F3 type  Ctrl+R refresh  Esc back"
	}
	return strings.Join([]string{padLine(ink(status, m.palette().Muted), width), padLine(ink(stop, m.palette().Warning), width), padLine(ink(keys, m.palette().Text), width)}, "\n")
}
