package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const outfitStationFixture = `{"name":"Sol","stations":[{"id":1,"marketId":123,"name":"Example Orbital","type":"Starport","distanceToArrival":425,"haveOutfitting":true,"updateTime":{"outfitting":"2026-10-20 12:00:00"}},{"id":2,"marketId":124,"name":"Other Port","type":"Outpost","haveOutfitting":false,"updateTime":{}}]}`
const outfitModuleFixture = `{"name":"Sol","sName":"Example Orbital","marketId":123,"outfitting":[{"id":"int_hyperdrive_size5_class5","name":"5A Frame Shift Drive"},{"id":"int_hyperdrive_size5_class5","name":"5A Frame Shift Drive"},{"id":"hpt_heatsinklauncher_tiny","name":"0I Heat Sink Launcher"},{"id":"hpt_pulselaser_fixed_large","name":"3E Pulse Laser (Fixed)"},{"id":"diamondback_armour_grade1","name":"1I Lightweight Alloy"}]}`

func testOutfitHTTP(t *testing.T, handler http.HandlerFunc) *outfittingClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := newOutfittingClient()
	client.base, client.gap = server.URL, 0
	client.http.Timeout = 2 * time.Second
	return client
}

func TestOutfittingAPIParsesIdentitiesModulesAndPreservesReportAge(t *testing.T) {
	var calls atomic.Int32
	client := testOutfitHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/api-system-v1/stations" {
			if len(r.URL.Query()) != 1 || r.URL.Query().Get("systemName") != "Sol" {
				t.Error("system query was not encoded correctly")
			}
			fmt.Fprint(w, outfitStationFixture)
		} else {
			if len(r.URL.Query()) != 1 || r.URL.Query().Get("marketId") != "123" {
				t.Error("module lookup did not use the exact market identity")
			}
			fmt.Fprint(w, outfitModuleFixture)
		}
	})
	stations, err := client.stations(context.Background(), "  Sol  ", false)
	if err != nil || len(stations.Stations) != 2 {
		t.Fatal(stations, err)
	}
	station := stations.Stations[0]
	expected := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	if !station.Updated.Equal(expected) || station.Updated.Location() != time.UTC {
		t.Fatal("reported UTC update time was lost")
	}
	cached, err := client.stations(context.Background(), "Sol", false)
	if err != nil || !cached.Cached || calls.Load() != 1 || !cached.Stations[0].Updated.Equal(expected) {
		t.Fatal("cache changed the report age or made another query", err)
	}
	modules, err := client.modules(context.Background(), station, false)
	if err != nil || len(modules.Modules) != 4 {
		t.Fatal("module deduplication/decoding failed", err, len(modules.Modules))
	}
	o := outfittingState{Active: true, Stage: "modules", Stations: stations.Stations, SelectedKey: station.Key, Modules: modules.Modules, Filter: "5A drive", Category: "Internal modules", Class: "5", Rating: "A", Retrieved: time.Now()}
	rows := o.moduleRows()
	if len(rows) != 1 || rows[0].Class == nil || *rows[0].Class != 5 || rows[0].Rating != "A" {
		t.Fatal("combined name/type/class/rating filter failed")
	}
	o.Filter, o.Category, o.Class, o.Rating = "", "All types", "0", "I"
	if rows = o.moduleRows(); len(rows) != 1 || rows[0].Category != "Utility mounts" {
		t.Fatal("class zero was treated as an unknown class")
	}
	m := newCockpit(cockpitOptions{mode: "live"}, expected.Add(8*24*time.Hour))
	m.outfitting = o
	view := m.outfitSnapshot()
	if !view.OldReport || !strings.Contains(view.UpdatedLabel, "8 days old") {
		t.Fatal("fetching now made an old report appear fresh", view.UpdatedLabel)
	}
	if _, err := client.stations(context.Background(), "Sol", true); err != nil || calls.Load() != 3 {
		t.Fatal("explicit refresh did not bypass the cache", err, calls.Load())
	}
	clock := time.Now().Add(11 * time.Minute)
	client.now = func() time.Time { return clock }
	if _, err := client.stations(context.Background(), "Sol", false); err != nil || calls.Load() != 4 {
		t.Fatal("expired cache was used indefinitely", err, calls.Load())
	}
}

func TestOutfittingRejectsChangedStationAndUnsafeInput(t *testing.T) {
	client := testOutfitHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api-system-v1/stations" {
			fmt.Fprint(w, outfitStationFixture)
			return
		}
		fmt.Fprint(w, strings.Replace(outfitModuleFixture, `"marketId":123`, `"marketId":999`, 1))
	})
	stations, err := client.stations(context.Background(), "Sol", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.modules(context.Background(), stations.Stations[0], false); err == nil {
		t.Fatal("different market identity accepted")
	}
	for _, input := range []string{"", "Sol\nother", strings.Repeat("x", 129)} {
		if _, err := outfitSystemName(input); err == nil {
			t.Fatal("invalid system name accepted")
		}
	}
	query := `Sol &marketId=999/../../other`
	encoded := testOutfitHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Query()) != 1 || r.URL.Query().Get("systemName") != query {
			t.Error("entered system escaped its query parameter")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": query, "stations": []any{}})
	})
	if _, err := encoded.stations(context.Background(), query, false); err != nil {
		t.Fatal(err)
	}
	module := classifyOutfitModule("int_fuel", "5A Fuel\x1b]2;injected-title\a\nScoop")
	if strings.Contains(module.Name, "\x1b") || strings.Contains(module.Name, "injected-title") || strings.Contains(module.Name, "\n") {
		t.Fatal("provider text escaped the terminal/DOM text boundary")
	}
}

func TestOutfittingHTTPFailuresBoundsAndInvalidDataAreNotCached(t *testing.T) {
	for _, kind := range []string{"rate-limit", "server-error", "redirect", "invalid-json", "missing-list", "trailing-json", "oversize-body", "too-many-stations", "too-many-modules"} {
		t.Run(kind, func(t *testing.T) {
			var calls atomic.Int32
			client := testOutfitHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch kind {
				case "rate-limit":
					w.WriteHeader(429)
				case "server-error":
					w.WriteHeader(503)
				case "redirect":
					http.Redirect(w, r, "https://example.com/not-edsm", 302)
				case "invalid-json":
					fmt.Fprint(w, "<html>provider unavailable</html>")
				case "missing-list":
					fmt.Fprint(w, `{"name":"Sol"}`)
				case "trailing-json":
					fmt.Fprint(w, outfitStationFixture+` {}`)
				case "oversize-body":
					fmt.Fprint(w, strings.Repeat(" ", 2<<20)+outfitStationFixture)
				case "too-many-stations":
					fmt.Fprintf(w, `{"name":"Sol","stations":[%s]}`, strings.TrimSuffix(strings.Repeat("{},", 4001), ","))
				case "too-many-modules":
					fmt.Fprintf(w, `{"name":"Sol","sName":"Example Orbital","marketId":123,"outfitting":[%s]}`, strings.TrimSuffix(strings.Repeat("{},", 6001), ","))
				}
			})
			for i := 0; i < 2; i++ {
				var err error
				if kind == "too-many-modules" {
					_, err = client.modules(context.Background(), outfitStation{System: "Sol", Name: "Example Orbital", MarketID: 123}, false)
				} else {
					_, err = client.stations(context.Background(), "Sol", false)
				}
				if err == nil {
					t.Fatal("invalid provider response was accepted")
				}
			}
			if calls.Load() != 2 || len(client.cache) != 0 {
				t.Fatal("invalid data was cached or redirect was followed")
			}
		})
	}
}

func TestOutfittingRequestsCancelAndJoinAtShutdown(t *testing.T) {
	entered, serverCancelled := make(chan struct{}), make(chan struct{})
	client := testOutfitHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, `{"name":"Sol","stations":[`)
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
		close(serverCancelled)
	})
	m := newCockpit(cockpitOptions{mode: "live"}, testNow)
	m.work = newSessionWork(context.Background())
	m.outfitting.client = client
	cmd := m.lookupStations("Sol", false)
	finished := make(chan tea.Msg, 1)
	go func() { finished <- cmd() }()
	<-entered
	select {
	case <-m.work.stop():
	case <-time.After(time.Second):
		t.Fatal("HTTP request blocked session shutdown")
	}
	select {
	case <-serverCancelled:
	case <-time.After(time.Second):
		t.Fatal("HTTP connection remained active after cancellation")
	}
	result := <-finished
	if result == nil {
		t.Fatal("running lookup did not report cancellation")
	}
	msg := result.(outfittingMsg)
	if msg.err == nil {
		t.Fatal("partial response was accepted after cancellation")
	}
	old := m.outfitting.request
	m.closeOutfitting()
	m.applyOutfitting(msg)
	if m.outfitting.Active || m.outfitting.request == old || len(m.outfitting.Stations) != 0 {
		t.Fatal("late result reopened or populated a closed viewer")
	}
}

func TestOutfittingQueuedAndDelayedRequestsRespectCancellation(t *testing.T) {
	var calls atomic.Int32
	client := testOutfitHTTP(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, outfitStationFixture) })
	<-client.gate
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := client.stations(ctx, "Sol", false); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued request ignored cancellation")
	}
	client.gate <- struct{}{}
	client.gap = time.Hour
	client.last = time.Now()
	ctx, cancel = context.WithCancel(context.Background())
	go func() { _, err := client.stations(ctx, "Sol", false); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("paced request ignored cancellation")
	}
	if calls.Load() != 0 {
		t.Fatal("cancelled request reached provider")
	}
}

func TestOutfittingSupersededResultsAndMockNeverSendFlightOrNetworkRequests(t *testing.T) {
	m := newCockpit(cockpitOptions{mode: "mock"}, testNow)
	m.requests = make(chan controlRequest, 4)
	first := m.lookupStations("First system", false)()
	second := m.lookupStations("Second system", false)()
	m.applyOutfitting(second.(outfittingMsg))
	m.applyOutfitting(first.(outfittingMsg))
	if m.outfitting.System != "Second system" || len(m.requests) != 0 {
		t.Fatal("older system result replaced the current lookup")
	}
	station := m.outfitting.Stations[0]
	m.applyOutfitting(m.lookupModules(station.Key, false)().(outfittingMsg))
	if len(m.outfitting.Modules) != 5 || !m.outfitSnapshot().Simulated {
		t.Fatal("mock viewer did not provide clearly simulated modules")
	}
	m.closeOutfitting()
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF7})
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	for _, letter := range "Shinrarta Dezhra" {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}
	// F2 replaces a previous query with the next typed system.
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyF2})
	for _, letter := range "Shinrarta Dezhra" {
		m = updateCockpit(t, m, tea.KeyPressMsg{Code: letter, Text: string(letter)})
	}
	if m.outfitting.Input != "Shinrarta Dezhra" {
		t.Fatal("system editing discarded spaces", m.outfitting.Input)
	}
	m = updateCockpit(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if m.outfitting.Active || m.page != pageMissions || len(m.requests) != 0 {
		t.Fatal("navigation retained viewer work or sent a control request")
	}
}

func TestOutfittingTerminalBoundsAndReportAgeOnEveryTheme(t *testing.T) {
	for theme := range tuiThemes {
		for _, size := range [][2]int{{60, 20}, {80, 24}, {110, 36}, {140, 42}} {
			m := newCockpit(cockpitOptions{mode: "control"}, time.Now())
			m.width, m.height, m.themeChoice, m.page = size[0], size[1], theme, pageNavigation
			m.showOutfitting()
			for _, stage := range []string{"system", "stations", "modules", "loading", "error"} {
				m.outfitting.Stage = stage
				m.outfitting.editSystem = stage == "system"
				m.outfitting.Stations = mockOutfitStations("Sol").Stations
				m.outfitting.SelectedKey = "simulated/1"
				m.outfitting.Modules = mockOutfitModules().Modules
				m.outfitting.Category = "All types"
				m.outfitting.Loading = stage == "loading"
				m.outfitting.Error = ""
				if stage == "error" {
					m.outfitting.Error = "EDSM is unavailable; try again later"
				}
				view := ansi.Strip(m.View().Content)
				rows := strings.Split(view, "\n")
				if len(rows) > size[1] {
					t.Fatal("outfitting viewer exceeded height", stage, size)
				}
				for _, row := range rows {
					if ansi.StringWidth(row) > size[0] {
						t.Fatal("outfitting viewer exceeded width", stage, size)
					}
				}
				if !strings.Contains(view, "Ctrl+Alt+F10") || !strings.Contains(view, "OUTFITTING") {
					t.Fatal("viewer hid stop instructions or source")
				}
			}
		}
	}
	if label := outfitUpdateLabel(time.Time{}, testNow); !strings.Contains(label, "unavailable") {
		t.Fatal("missing report time was invented")
	}
}

func TestLiveEDSMOutfittingLookup(t *testing.T) {
	if os.Getenv("ELITEAUX_EDSM_LIVE_TEST") != "1" {
		t.Skip("opt-in public EDSM integration check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client := newOutfittingClient()
	stations, err := client.stations(ctx, "Sol", false)
	if err != nil {
		t.Fatal(err)
	}
	var station outfitStation
	for _, candidate := range stations.Stations {
		if candidate.Name == "Abraham Lincoln" {
			station = candidate
			break
		}
	}
	if station.Key == "" {
		t.Fatal("expected public integration station not reported")
	}
	result, err := client.modules(ctx, station, false)
	if err != nil || len(result.Modules) == 0 {
		t.Fatal("live outfitting lookup returned no module data", err)
	}
	t.Logf("EDSM: %d stations in Sol; %d modules at %s; %s", len(stations.Stations), len(result.Modules), station.Name, outfitUpdateLabel(station.Updated, time.Now()))
}
