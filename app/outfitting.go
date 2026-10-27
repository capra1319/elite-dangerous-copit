package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type outfittingState struct {
	Active, Loading, Cached, Simulated                    bool
	Input, System, Stage, Filter, Category, Class, Rating string
	StationFilter, SelectedKey, Message, Error            string
	Stations                                              []outfitStation
	Modules                                               []outfitModule
	Retrieved                                             time.Time
	StationIndex, ModuleOffset                            int
	editSystem, replaceInput                              bool
	request                                               uint64
	cancel                                                context.CancelFunc
	client                                                *outfittingClient
}

type outfittingMsg struct {
	request  uint64
	kind     string
	stations outfitStationsResult
	modules  outfitModulesResult
	err      error
}

func (o *outfittingState) cancelRequest() {
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
	o.request++
	o.Loading = false
}

func (m *cockpitModel) closeOutfitting() { m.outfitting.cancelRequest(); m.outfitting.Active = false }

func (m *cockpitModel) showOutfitting() {
	m.stationSearchActive = false
	o := &m.outfitting
	o.Active = true
	if o.Stage == "" {
		o.Stage = "system"
		o.Input = m.companion.Nav.System
		o.editSystem = true
		o.replaceInput = true
		o.Category = "All types"
	}
	if o.client == nil {
		o.client = newOutfittingClient()
	}
	m.scroll = 0
}

func (o outfittingState) selectedStation() (outfitStation, bool) {
	for _, station := range o.Stations {
		if station.Key == o.SelectedKey {
			return station, true
		}
	}
	return outfitStation{}, false
}

func (o outfittingState) stationRows() []outfitStation {
	var rows []outfitStation
	for _, station := range o.Stations {
		if strings.Contains(strings.ToLower(station.Name+" "+station.Type), strings.ToLower(o.StationFilter)) {
			rows = append(rows, station)
		}
	}
	return rows
}

func (o outfittingState) moduleRows() []outfitModule {
	rows := make([]outfitModule, 0)
	for _, module := range o.Modules {
		if o.Category != "" && o.Category != "All types" && module.Category != o.Category {
			continue
		}
		if o.Rating != "" && module.Rating != o.Rating {
			continue
		}
		if o.Class != "" && (module.Class == nil || fmt.Sprint(*module.Class) != o.Class) {
			continue
		}
		text := strings.ToLower(module.Name + " " + module.Category + " " + module.Ship + " " + module.ID)
		matched := true
		for _, term := range strings.Fields(strings.ToLower(o.Filter)) {
			if !strings.Contains(text, term) {
				matched = false
				break
			}
		}
		if matched {
			rows = append(rows, module)
		}
	}
	return rows
}

func (m *cockpitModel) lookupStations(input string, refresh bool) tea.Cmd {
	o := &m.outfitting
	o.cancelRequest()
	o.Active = true
	o.Error, o.Message = "", ""
	system, err := outfitSystemName(input)
	if err != nil {
		o.Error = err.Error()
		return nil
	}
	o.Input, o.Stage, o.Loading = system, "stations", true
	o.System, o.StationFilter, o.SelectedKey = "", "", ""
	o.Stations, o.Modules = nil, nil
	o.StationIndex, o.ModuleOffset = 0, 0
	o.editSystem, o.replaceInput = false, false
	o.Simulated = m.opts.mode == "mock"
	if o.client == nil {
		o.client = newOutfittingClient()
	}
	parent := context.Background()
	if m.work != nil {
		parent = m.work.ctx
	}
	ctx, cancel := context.WithCancel(parent)
	o.cancel = cancel
	id, client, simulated := o.request, o.client, o.Simulated
	return m.work.command(0, func() tea.Msg {
		defer cancel()
		if simulated {
			return outfittingMsg{request: id, kind: "stations", stations: mockOutfitStations(system)}
		}
		result, err := client.stations(ctx, system, refresh)
		return outfittingMsg{request: id, kind: "stations", stations: result, err: err}
	})
}

func (m *cockpitModel) lookupModules(key string, refresh bool) tea.Cmd {
	o := &m.outfitting
	var selected outfitStation
	for _, station := range o.Stations {
		if station.Key == key {
			selected = station
			break
		}
	}
	if selected.Key == "" {
		o.Error = "Choose a station from the current system's list"
		return nil
	}
	o.cancelRequest()
	o.SelectedKey, o.Stage, o.Loading = key, "modules", true
	o.Error, o.Message, o.Filter, o.Class, o.Rating = "", "", "", "", ""
	o.Category = "All types"
	o.Modules = nil
	o.ModuleOffset = 0
	o.editSystem = false
	parent := context.Background()
	if m.work != nil {
		parent = m.work.ctx
	}
	ctx, cancel := context.WithCancel(parent)
	o.cancel = cancel
	id, client, simulated := o.request, o.client, o.Simulated
	if client == nil {
		client = newOutfittingClient()
		o.client = client
	}
	return m.work.command(0, func() tea.Msg {
		defer cancel()
		if simulated {
			return outfittingMsg{request: id, kind: "modules", modules: mockOutfitModules()}
		}
		result, err := client.modules(ctx, selected, refresh)
		return outfittingMsg{request: id, kind: "modules", modules: result, err: err}
	})
}

func (m *cockpitModel) applyOutfitting(msg outfittingMsg) {
	o := &m.outfitting
	if msg.request != o.request || !o.Active {
		return
	}
	o.Loading = false
	if o.cancel != nil {
		o.cancel()
		o.cancel = nil
	}
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			o.Error = "Search cancelled"
		} else {
			o.Error = outfitText(msg.err.Error(), 220)
		}
		return
	}
	if msg.kind == "stations" {
		o.System, o.Stations, o.Retrieved, o.Cached = msg.stations.System, msg.stations.Stations, msg.stations.Retrieved, msg.stations.Cached
		o.Message = fmt.Sprintf("%d stations reported · choose a station", len(o.Stations))
		if len(o.Stations) == 0 {
			o.Message = "No stations reported for this system"
		}
	} else {
		o.Modules, o.Retrieved, o.Cached = msg.modules.Modules, msg.modules.Retrieved, msg.modules.Cached
		o.Message = fmt.Sprintf("%d modules reported · prices and quantities are not supplied", len(o.Modules))
		if len(o.Modules) == 0 {
			o.Message = "No outfitting data reported; this does not prove the station has no modules"
		}
	}
}

func outfitUpdateLabel(stamp, now time.Time) string {
	if stamp.IsZero() {
		return "Outfitting update time unavailable"
	}
	age := now.Sub(stamp)
	when := stamp.UTC().Format("02 Jan 2006 15:04 UTC")
	if age < -5*time.Minute {
		return when + " · timestamp ahead of local clock"
	}
	age = max(time.Duration(0), age)
	if age >= 24*time.Hour {
		return fmt.Sprintf("%s · %d days old", when, int(age.Hours()/24))
	}
	if age >= time.Hour {
		return fmt.Sprintf("%s · %d hours old", when, int(age.Hours()))
	}
	return fmt.Sprintf("%s · %d minutes old", when, int(age.Minutes()))
}

func mockOutfitStations(system string) outfitStationsResult {
	now := time.Now()
	distance := 425.0
	return outfitStationsResult{System: system, Stations: []outfitStation{
		{Key: "simulated/1", System: system, Name: "Example Orbital (simulated)", Type: "Starport", Distance: &distance, HaveOutfitting: true, Updated: now.Add(-3 * time.Hour)},
		{Key: "simulated/2", System: system, Name: "Example Port (simulated)", Type: "Planetary Port", HaveOutfitting: true},
	}, Retrieved: now}
}

func mockOutfitModules() outfitModulesResult {
	return outfitModulesResult{Retrieved: time.Now(), Modules: []outfitModule{
		classifyOutfitModule("int_hyperdrive_size5_class5", "5A Frame Shift Drive"),
		classifyOutfitModule("int_hyperdrive_size4_class5", "4A Frame Shift Drive"),
		classifyOutfitModule("int_fuelscoop_size6_class5", "6A Fuel Scoop"),
		classifyOutfitModule("hpt_pulselaser_fixed_large", "3E Pulse Laser (Fixed)"),
		classifyOutfitModule("hpt_heatsinklauncher_tiny", "0I Heat Sink Launcher"),
	}}
}

type outfitViewData struct {
	Active, Loading, Cached, Simulated, OldReport                                                                                   bool
	Input, System, Stage, Filter, StationFilter, Category, Class, Rating, SelectedKey, Error, Message, UpdatedLabel, RetrievedLabel string
	Stations                                                                                                                        []outfitStation
	Modules                                                                                                                         []outfitModule
	Station                                                                                                                         *outfitStation
	Categories, Classes, Ratings                                                                                                    []string
	TotalModules                                                                                                                    int
}

func (m cockpitModel) outfitSnapshot() outfitViewData {
	o := m.outfitting
	if !o.Active {
		return outfitViewData{Active: false}
	}
	view := outfitViewData{Active: o.Active, Loading: o.Loading, Cached: o.Cached, Simulated: o.Simulated || m.opts.mode == "mock", Input: o.Input, System: o.System, Stage: o.Stage,
		Filter: o.Filter, StationFilter: o.StationFilter, Category: o.Category, Class: o.Class, Rating: o.Rating, SelectedKey: o.SelectedKey, Error: o.Error, Message: o.Message,
		Stations: o.stationRows(), Modules: o.moduleRows(), Categories: outfitCategories, TotalModules: len(o.Modules), Classes: []string{}, Ratings: []string{}}
	if station, ok := o.selectedStation(); ok {
		view.Station = &station
		view.UpdatedLabel = outfitUpdateLabel(station.Updated, m.now)
		view.OldReport = !station.Updated.IsZero() && m.now.Sub(station.Updated) > 7*24*time.Hour
	}
	classes, ratings := map[int]bool{}, map[string]bool{}
	for _, module := range o.Modules {
		if module.Class != nil {
			classes[*module.Class] = true
		}
		ratings[module.Rating] = true
	}
	for class := 0; class <= 99; class++ {
		if classes[class] {
			view.Classes = append(view.Classes, fmt.Sprint(class))
		}
	}
	for _, rating := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"} {
		if ratings[rating] {
			view.Ratings = append(view.Ratings, rating)
		}
	}
	if !o.Retrieved.IsZero() {
		view.RetrievedLabel = "Retrieved " + o.Retrieved.UTC().Format("02 Jan 15:04 UTC")
	}
	return view
}
