package main

import tea "charm.land/bubbletea/v2"

const (
	previousPageHotkey = 3
	nextPageHotkey     = 4
)

type pageDefinition struct {
	ID             int
	Name, Shortcut string
}

// One manifest controls terminal tabs, cycling and the desktop navigation.
var flightPages = []pageDefinition{
	{pageCockpit, "Cockpit", ""}, {pageShip, "Ship", "F6"}, {pageNavigation, "Navigation", "F7"},
	{pageMissions, "Missions", "F8"}, {pageHardpoints, "Hardpoints", "F10"}, {pageEngineering, "Engineering", "F9"},
	{pageSettings, "Settings", "F5"}, {pageDiagnostics, "Diagnostics", ""}, {pageHelp, "Help", "F1"},
}

func pagePosition(page int) int {
	for i, item := range flightPages {
		if item.ID == page {
			return i
		}
	}
	return 0
}

func numpadPageStep(key tea.Key) int {
	code := numpadCode(key)
	if key.Mod == 0 {
		if code == tea.KeyKpMinus {
			return -1
		}
		if code == tea.KeyKpPlus {
			return 1
		}
	}
	return 0
}

func (m *cockpitModel) stepPage(delta int) {
	m.closeOutfitting()
	m.leaveSettings()
	m.goalEditing, m.materialSearching = false, false
	m.stationSearchActive = false
	m.page = flightPages[((pagePosition(m.page)+delta)%len(flightPages)+len(flightPages))%len(flightPages)].ID
	m.scroll = 0
}

// Page shortcuts are view-only and never enter the flight-action dispatcher.
func (d display) handlePageHotkey(id int) bool {
	if d.navigate == nil {
		return false
	}
	switch id {
	case previousPageHotkey:
		d.navigate(-1)
		return true
	case nextPageHotkey:
		d.navigate(1)
		return true
	}
	return false
}
