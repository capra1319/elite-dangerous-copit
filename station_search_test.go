package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestStationSearchOnlyUsesKnownPublicSearchTools(t *testing.T) {
	if len(stationSearchProviders) != 5 {
		t.Fatal("expected station, outfitting, market, trader and component tools")
	}
	for i, provider := range stationSearchProviders {
		address, err := stationSearchURL(i)
		parsed, parseErr := url.Parse(address)
		if err != nil || parseErr != nil || parsed.Scheme != "https" || parsed.Host != "inara.cz" || provider.Name == "" || provider.Description == "" || !validStationSearchURL(address) {
			t.Fatal("invalid station search provider", i, err, parseErr)
		}
	}
	for _, index := range []int{-1, len(stationSearchProviders)} {
		if _, err := stationSearchURL(index); err == nil {
			t.Fatal("invalid category accepted")
		}
	}
	for _, address := range []string{"file:///C:/test.exe", "https://example.com/", "https://inara.cz/elite/nearest-stations/?commander=private", " https://inara.cz/elite/nearest-stations/"} {
		if validStationSearchURL(address) {
			t.Fatal("unlisted external address accepted")
		}
	}
}

func TestStationSearchMenuFitsAndDoesNotSendGameInput(t *testing.T) {
	for theme := range tuiThemes {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
			m := newCockpit(cockpitOptions{mode: "control"}, testNow)
			m.requests = make(chan controlRequest, 4)
			m.width, m.height, m.themeChoice = size[0], size[1], theme
			m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF7})
			m = updateCockpit(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
			m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
			if !m.stationSearchActive || m.stationSearchIndex != 4 {
				t.Fatal("station search did not open/wrap")
			}
			next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(cockpitModel)
			if cmd == nil || len(m.requests) != 0 || m.frame.State != nil {
				t.Fatal("search changed flight state or did not prepare a browser command")
			}
			// Do not run the command: external browser actions require an actual user press.
			view := ansi.Strip(m.View().Content)
			if !strings.Contains(view, "STATION SEARCH") || !strings.Contains(view, "browser") || !strings.Contains(view, "Ctrl+Alt+F10") {
				t.Fatal("search hid its purpose or emergency stop")
			}
			rows := strings.Split(view, "\n")
			if len(rows) > size[1] {
				t.Fatal("station search exceeded terminal height")
			}
			for _, row := range rows {
				if ansi.StringWidth(row) > size[0] {
					t.Fatal("station search exceeded terminal width")
				}
			}
			m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.stationSearchActive || m.page != pageNavigation {
				t.Fatal("Escape did not return to navigation")
			}
		}
	}
}

func TestStationSearchCannotLaunchAfterSessionShutdown(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "control"}, testNow)
	m.work = newSessionWork(context.Background())
	command := m.openStationSearch()
	<-m.work.stop()
	if msg := command(); msg != nil {
		t.Fatal("browser command ran after shutdown")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := openExternalSearch(ctx, stationSearchProviders[0].URL); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled browser request ignored cancellation", err)
	}
	m.stationSearchResult(stationSearchOpenedMsg{err: errors.New("no browser")})
	if !strings.Contains(m.stationSearchMessage, "Could not open browser") {
		t.Fatal("browser failure was not reported")
	}
}
