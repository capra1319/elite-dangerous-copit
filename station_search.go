package main

import (
	"context"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type stationSearchProvider struct{ Name, Description, URL string }

// Public search tools, opened only on a deliberate press. No journal data or
// commander identity is placed in the URL; the user chooses the site's filters.
var stationSearchProviders = []stationSearchProvider{
	{"Stations & services", "Station name, reference system, services and landing pads", "https://inara.cz/elite/nearest-stations/"},
	{"Outfitting & ships", "Ship modules, ships, suits and personal equipment", "https://inara.cz/elite/nearest-outfitting/"},
	{"Commodities", "Goods, buy/sell prices, supply and demand", "https://inara.cz/elite/commodities/"},
	{"Material traders", "Locate contacts that exchange ship engineering materials", "https://inara.cz/elite/nearest-stations/?formbrief=1&pa1%5B%5D=25&pi17=1&pi18=3&pi19=5000"},
	{"Odyssey components", "Reported fleet-carrier component listings and prices", "https://inara.cz/elite/market-materials/"},
}

func stationSearchURL(index int) (string, error) {
	if index < 0 || index >= len(stationSearchProviders) {
		return "", errors.New("choose a station search category")
	}
	return stationSearchProviders[index].URL, nil
}

type stationSearchOpenedMsg struct{ err error }

func (m cockpitModel) openStationSearch() tea.Cmd {
	address, err := stationSearchURL(m.stationSearchIndex)
	if err != nil {
		return func() tea.Msg { return stationSearchOpenedMsg{err: err} }
	}
	ctx := context.Background()
	if m.work != nil {
		ctx = m.work.ctx
	}
	return m.work.command(0, func() tea.Msg { return stationSearchOpenedMsg{err: openExternalSearch(ctx, address)} })
}

func (m *cockpitModel) stationSearchKey(key tea.Key) (bool, tea.Cmd) {
	if key.Mod != 0 {
		return false, nil
	}
	if key.Code == 's' || key.Code == 'S' {
		if key.IsRepeat {
			return true, nil
		}
		m.stationSearchActive = !m.stationSearchActive
		m.stationSearchMessage = ""
		m.scroll = 0
		return true, nil
	}
	if !m.stationSearchActive {
		return false, nil
	}
	if key.Code == tea.KeyF1 || key.Code == tea.KeyF5 || key.Code >= tea.KeyF6 && key.Code <= tea.KeyF10 {
		m.stationSearchActive = false
		return false, nil
	}
	switch key.Code {
	case tea.KeyEscape:
		m.stationSearchActive = false
		return true, nil
	case tea.KeyUp:
		m.stationSearchIndex = (m.stationSearchIndex + len(stationSearchProviders) - 1) % len(stationSearchProviders)
		return true, nil
	case tea.KeyDown:
		m.stationSearchIndex = (m.stationSearchIndex + 1) % len(stationSearchProviders)
		return true, nil
	case tea.KeyEnter, tea.KeyKpEnter:
		if key.IsRepeat {
			return true, nil
		}
		m.stationSearchMessage = "Opening browser…"
		return true, m.openStationSearch()
	}
	return false, nil
}

func (m cockpitModel) stationSearchPanel(width int) string {
	lines := []string{m.section("STATION SEARCH // INARA"), ink("Opens your browser · filters are on Inara", m.palette().Muted)}
	for i, provider := range stationSearchProviders {
		label := fmt.Sprintf("  %d  %s", i+1, provider.Name)
		if i == m.stationSearchIndex {
			label = ">" + label[1:]
			label = bold(label, m.palette().Accent)
		}
		lines = append(lines, label)
	}
	if m.height >= 28 {
		lines = append(lines, "", stationSearchProviders[m.stationSearchIndex].Description, "Use reference system: "+reportedText(m.companion.Nav.System))
	}
	lines = append(lines, ink("↑↓ choose   Enter opens browser   Esc back", m.palette().Muted))
	return m.panel(width, m.palette().Border, lines...)
}

func (m *cockpitModel) stationSearchResult(msg stationSearchOpenedMsg) {
	if msg.err != nil {
		m.stationSearchMessage = "Could not open browser: " + msg.err.Error()
		return
	}
	m.stationSearchMessage = "Search sent to browser · check listing update times"
}

func validStationSearchURL(address string) bool {
	for _, provider := range stationSearchProviders {
		if address == provider.URL {
			return true
		}
	}
	return false
}
