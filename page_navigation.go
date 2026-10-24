package main

import tea "charm.land/bubbletea/v2"

const (
	previousPageHotkey = 3
	nextPageHotkey     = 4
)

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
	m.leaveSettings()
	m.goalEditing, m.materialSearching = false, false
	m.page = ((m.page+delta)%pageCount + pageCount) % pageCount
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
