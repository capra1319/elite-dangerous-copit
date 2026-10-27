//go:build desktop

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed frontend/*
var desktopAssets embed.FS

type DesktopPreferences struct {
	UIScale             int    `json:"uiScale"`
	Version             int    `json:"version"`
	Theme               string `json:"theme"`
	Layout              string `json:"layout"`
	Fullscreen          bool   `json:"fullscreen"`
	Scanlines           bool   `json:"scanlines"`
	ControlColumns      int    `json:"controlColumns"`
	AutoAddShipControls bool   `json:"autoAddShipControls"`
	ScanlineSpacing     int    `json:"scanlineSpacing"`
	HealthDisplay       string `json:"healthDisplay"`
}

func defaultDesktopPreferences() DesktopPreferences {
	return DesktopPreferences{UIScale: 100, Version: 1, Theme: "command-deck", Layout: "auto", ScanlineSpacing: 12, HealthDisplay: "ship"}
}
func validDesktopPreferences(p DesktopPreferences) bool {
	_, themeOK := themeIndex(p.Theme)
	return (p.UIScale == 0 || p.UIScale >= 100 && p.UIScale <= 200 && p.UIScale%25 == 0) && (p.HealthDisplay == "" || p.HealthDisplay == "ship" || p.HealthDisplay == "bars") && p.Version == 1 && themeOK && (p.Layout == "auto" || p.Layout == "portrait" || p.Layout == "landscape") && p.ControlColumns >= 0 && p.ControlColumns <= 10 && (p.ScanlineSpacing == 0 || p.ScanlineSpacing >= 4 && p.ScanlineSpacing <= 24 && p.ScanlineSpacing%2 == 0)
}
func loadDesktopPreferences(path string) (DesktopPreferences, error) {
	p := defaultDesktopPreferences()
	data, err := readSmallFile(path, 4096)
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err = decodeDesktopPreferences(data, &p); err != nil {
		return defaultDesktopPreferences(), err
	}
	return p, nil
}

// Separate from terminal appearance settings, and never persists mock/armed state.
type DesktopApp struct {
	sessionMu          sync.Mutex
	mu                 sync.Mutex
	ctx                context.Context
	fullscreenWindow   func(bool)
	work               *sessionWork
	closing            bool
	model              cockpitModel
	prefs              DesktopPreferences
	settingsPath       string
	message            string
	layout             desktopControlLayout
	layoutError        string
	autoLayoutAttempt  string
	materialNotes      map[string]string
	mockMaterialNotes  map[string]string
	materialNotesError string
	materialPins       map[string]bool
	mockMaterialPins   map[string]bool
	materialPinsError  string
	materialSelection  string
	opts               cockpitOptions
}

type DesktopControl struct {
	ID, Label, Key, State, Kind, Availability, Description string
	Enabled                                                bool
	Permanent                                              bool
}
type DesktopEngineering struct {
	Rows                      []DesktopMaterialRow
	Selected, Filter, Message string
	Pinned                    bool
	Note, NotesError          string
	PinsError                 string
}
type DesktopState struct {
	Page                                                                                                    int
	Pages                                                                                                   []pageDefinition
	StationSearch                                                                                           []stationSearchProvider
	Engineering                                                                                             DesktopEngineering
	ShipModules                                                                                             []shipModuleView
	Health                                                                                                  desktopShipHealth
	CockpitRoute                                                                                            desktopCockpitRoute
	ShipPowerAt, ShipPowerError                                                                             string
	ShipModulesAt                                                                                           string
	Outfitting                                                                                              outfitViewData
	Paused                                                                                                  bool
	Status, Detail, Mode, Theme, Commander, Ship, System, Context, Fuel, Pips, Warning, Action, Error, Text string
	Ready, Armed                                                                                            bool
	Controls                                                                                                []DesktopControl
	ControlLayout                                                                                           []string
	PageView                                                                                                desktopPageView
	Preferences                                                                                             DesktopPreferences
	Editor, PageKeys                                                                                        string
}

func newDesktopApp(p DesktopPreferences, settingsPath string) *DesktopApp {
	// Window mode belongs to this process, never a previous session.
	p.Fullscreen = false
	status := defaultStatusPath()
	userSettings := defaultSettingsPath()
	opts := cockpitOptions{mode: "control", controls: "numpad", path: status, config: defaultControlConfig(),
		settings: userSettings, journal: filepath.Dir(status), goals: filepath.Join(filepath.Dir(userSettings), "engineering-goals.json"),
		firegroups: filepath.Join(filepath.Dir(userSettings), "fire-groups.json"), interval: defaultTelemetryInterval, stale: time.Minute}
	layout, layoutErr := loadControlLayout(filepath.Join(filepath.Dir(settingsPath), "cockpit-layout.json"))
	a := &DesktopApp{prefs: p, settingsPath: settingsPath, opts: opts, layout: layout}
	if layoutErr != nil {
		a.layoutError = "Cockpit layout could not be read: " + layoutErr.Error() + ". Defaults are shown."
	}
	if settingsPath != "" {
		var err error
		a.materialPins, err = loadMaterialPins(filepath.Join(filepath.Dir(settingsPath), "engineering-pins.json"))
		if err != nil {
			a.materialPinsError = "Saved material pins could not be loaded: " + err.Error() + ". Fix the pins file and restart before saving."
		}
		a.materialNotes, err = loadMaterialNotes(filepath.Join(filepath.Dir(settingsPath), "material-notes.json"))
		if err != nil {
			a.materialNotesError = "Saved material comments could not be loaded: " + err.Error() + ". Fix the notes file and restart before saving."
		}
	}
	a.fullscreenWindow = func(enabled bool) {
		if enabled {
			wr.WindowFullscreen(a.ctx)
		} else {
			wr.WindowUnfullscreen(a.ctx)
		}
	}
	return a
}

func (a *DesktopApp) startup(ctx context.Context) {
	a.ctx = ctx
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	a.startSession(false)
}

// sessionMu serialises start/stop; callbacks only touch their own current session.
func (a *DesktopApp) startSession(mock bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return
	}
	opts := a.opts
	if mock {
		opts.mode = "mock"
	}
	work := newSessionWork(a.ctx)
	a.work = work
	a.model = newCockpit(opts, time.Now())
	a.materialSelection = ""
	selection, _ := themeIndex(a.prefs.Theme)
	a.model.themeChoice, a.model.savedTheme = selection, selection
	a.model.work = work
	requests := make(chan controlRequest, 4)
	a.model.requests = requests
	reader := a.model.journalReader
	if reader != nil {
		reader.ctx = work.ctx
	}
	if mock {
		return
	}
	publish := func(frame dashboardFrame) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.work != work || work.ctx.Err() != nil {
			return
		}
		a.model.now = time.Now()
		a.model.frame = frame
		a.model.record(frame.Action)
		a.model.updateConnection()
	}
	navigate := func(delta int) {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.work == work && work.ctx.Err() == nil {
			a.model.stepPage(delta)
			if a.model.page != pageEngineering {
				a.materialSelection = ""
			}
		}
	}
	go work.command(0, func() tea.Msg {
		result := safeBackend(func() tea.Msg {
			return backendDoneMsg{err: runControl(work.ctx, display{out: io.Discard, controls: opts.controls, requests: requests,
				publish: publish, navigate: navigate}, opts.path, opts.config, opts.interval, opts.stale, false)}
		}).(backendDoneMsg)
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.work == work && work.ctx.Err() == nil {
			if result.err == nil {
				result.err = errors.New("game controls stopped")
			}
			a.model.outfitting.cancelRequest()
			a.model.outfitting.Error = "Connection session stopped; restart before searching"
			a.model.fatal = result.err
			a.model.frame.ControlStatus = "DISARMED"
			work.cancel()
		}
		return nil
	})()
	go work.command(0, func() tea.Msg {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for work.ctx.Err() == nil {
			presence := probeGamePresence()
			var journal companionMsg
			if reader != nil {
				journal = reader.poll()
			}
			a.mu.Lock()
			if a.work == work && work.ctx.Err() == nil {
				a.model.now = time.Now()
				a.model.presence = presence
				if reader != nil {
					a.model.companion, a.model.companionError = journal.Data, journal.Error
					a.syncAutomaticControls()
				}
				a.model.updateConnection()
			}
			a.mu.Unlock()
			select {
			case <-work.ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
		return nil
	})()
}

func (a *DesktopApp) stopSession() error {
	a.mu.Lock()
	work := a.work
	a.mu.Unlock()
	if work == nil {
		return nil
	}
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-work.stop():
		a.mu.Lock()
		if a.work == work {
			a.work = nil
		}
		a.mu.Unlock()
		return nil
	case <-timer.C:
		return errors.New("session workers did not stop; refusing to start another session")
	}
}
func (a *DesktopApp) shutdown(context.Context) {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	a.mu.Lock()
	a.closing = true
	a.model.closeOutfitting()
	a.mu.Unlock()
	if err := a.stopSession(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
func (a *DesktopApp) SetMock(enabled bool) error {
	a.sessionMu.Lock()
	defer a.sessionMu.Unlock()
	a.mu.Lock()
	if a.closing {
		a.mu.Unlock()
		return errors.New("application is closing")
	}
	same := (a.model.opts.mode == "mock") == enabled && a.work != nil && a.work.ctx.Err() == nil
	a.mu.Unlock()
	if same {
		return nil
	}
	if err := a.stopSession(); err != nil {
		return err
	}
	a.startSession(enabled)
	return nil
}
func (a *DesktopApp) Disarm() {
	a.mu.Lock()
	defer a.mu.Unlock()
	requestStop(a.model.requests)
}

// Each press is checked again by the foreground-only guarded control backend.
func (a *DesktopApp) PressControl(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	var action *actionDef
	for i := range liveActions {
		if liveActions[i].ID == id {
			action = &liveActions[i]
			break
		}
	}
	if action == nil {
		return errors.New("unknown control")
	}
	if a.closing || a.work == nil || a.work.ctx.Err() != nil {
		return errors.New("control session is stopped")
	}
	if !shipControlAvailable(a.model.utilityAvailability(id)) {
		return errors.New("action availability is not confirmed for the last reported ship loadout")
	}
	if a.model.mock != nil {
		command := id
		if strings.HasPrefix(id, "t") {
			command += " toggle"
		}
		a.model.now = time.Now()
		a.model.mockAction(command)
		return nil
	}
	if a.model.frame.ControlStatus != "ARMED" && a.model.frame.ControlStatus != "PAUSED" {
		return errors.New("focus Elite and press Ctrl+Alt+F9 to arm first")
	}
	select {
	case a.model.requests <- controlRequest{Kind: "action", Action: id, Created: time.Now()}:
		return nil
	default:
		return errors.New("a control press is already waiting; press again deliberately")
	}
}

func (a *DesktopApp) FilterMaterials(query string, pinned bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	query = safeText(query)
	if runes := []rune(query); len(runes) > 80 {
		query = string(runes[:80])
	}
	a.model.materialFilter, a.model.onlyPinned, a.model.materialIndex = query, pinned, 0
	a.model.goalEditing, a.model.materialSearching, a.model.stationSearchActive = false, false, false
}
func (a *DesktopApp) SelectMaterial(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if key == "" {
		a.materialSelection = ""
		return
	}
	for _, row := range a.desktopMaterialRows() {
		if row.Key == key {
			a.materialSelection = key
			return
		}
	}
}
func (a *DesktopApp) SaveMaterialTarget(key string, target int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if target < 0 || target > 999999 || !validGoal(key, max(1, target)) {
		return errors.New("enter a target from 1 to 999999, or remove it")
	}
	found := false
	for _, row := range a.desktopMaterialRows() {
		if row.Key == key {
			found = true
			break
		}
	}
	if !found {
		return errors.New("select a material from the list")
	}
	a.model.saveTarget(key, target)
	if strings.HasPrefix(a.model.goalMessage, "Could not") {
		return errors.New(a.model.goalMessage)
	}
	return nil
}

// The session owns both the HTTP request and its completion callback. Late
// results cannot update a replacement backend or a closed viewer.
func (a *DesktopApp) runOutfittingCommand(cmd tea.Cmd, work *sessionWork) {
	if cmd == nil {
		return
	}
	go work.command(0, func() tea.Msg {
		msg := cmd()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.work == work && !a.closing && work.ctx.Err() == nil {
			if result, ok := msg.(outfittingMsg); ok {
				a.model.applyOutfitting(result)
			}
		}
		return nil
	})()
}
func (a *DesktopApp) ShowOutfitting(open bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return
	}
	if open {
		a.model.page = pageNavigation
		a.model.showOutfitting()
	} else {
		a.model.closeOutfitting()
	}
}
func (a *DesktopApp) SearchOutfittingStations(system string, refresh bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing || a.work == nil || a.work.ctx.Err() != nil {
		return errors.New("session has stopped; restart or switch mode before searching")
	}
	if _, err := outfitSystemName(system); err != nil {
		return err
	}
	a.model.showOutfitting()
	cmd := a.model.lookupStations(system, refresh)
	a.runOutfittingCommand(cmd, a.work)
	return nil
}
func (a *DesktopApp) SelectOutfittingStation(key string, refresh bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing || a.work == nil || a.work.ctx.Err() != nil {
		return errors.New("session has stopped")
	}
	if !a.model.outfitting.Active {
		return errors.New("open station outfitting first")
	}
	found := false
	for _, station := range a.model.outfitting.Stations {
		if station.Key == key {
			found = true
			break
		}
	}
	if !found {
		return errors.New("choose a station from the current system's list")
	}
	cmd := a.model.lookupModules(key, refresh)
	a.runOutfittingCommand(cmd, a.work)
	return nil
}
func (a *DesktopApp) FilterOutfitting(stations, query, category, class, rating string) error {
	validCategory := false
	for _, name := range outfitCategories {
		if name == category {
			validCategory = true
			break
		}
	}
	if !validCategory {
		return errors.New("unknown module category")
	}
	if class != "" {
		number, err := strconv.Atoi(class)
		if err != nil || number < 0 || number > 99 {
			return errors.New("invalid module class")
		}
	}
	if rating != "" && (len(rating) != 1 || rating[0] < 'A' || rating[0] > 'I') {
		return errors.New("invalid module rating")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.model.outfitting.Active {
		return nil
	}
	o := &a.model.outfitting
	o.StationFilter, o.Filter = outfitInputText(stations), outfitInputText(query)
	o.Category, o.Class, o.Rating = category, class, rating
	o.StationIndex, o.ModuleOffset = 0, 0
	return nil
}
func (a *DesktopApp) CancelOutfitting() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.model.outfitting.cancelRequest()
	a.model.outfitting.Message = "Search cancelled"
}

func (a *DesktopApp) OpenStationSearch(index int) error {
	address, err := stationSearchURL(index)
	if err != nil {
		return err
	}
	a.mu.Lock()
	ctx, closing := a.ctx, a.closing
	a.mu.Unlock()
	if closing || ctx == nil {
		return errors.New("application is not ready")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	wr.BrowserOpenURL(ctx, address)
	return nil
}

func (a *DesktopApp) SelectPage(page int) {
	if page < 0 || page >= pageCount {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.model.closeOutfitting()
	a.model.leaveSettings()
	a.model.goalEditing, a.model.materialSearching, a.model.stationSearchActive = false, false, false
	if page != pageEngineering {
		a.materialSelection = ""
	}
	a.model.page, a.model.scroll = page, 0
}
func (a *DesktopApp) PageKey(name string) {
	keys := map[string]rune{"ArrowUp": tea.KeyUp, "ArrowDown": tea.KeyDown, "ArrowLeft": tea.KeyLeft, "ArrowRight": tea.KeyRight,
		"Enter": tea.KeyEnter, "Escape": tea.KeyEscape, "Backspace": tea.KeyBackspace, "Delete": tea.KeyDelete, "PageUp": tea.KeyPgUp, "PageDown": tea.KeyPgDown}
	code, ok := keys[name]
	if !ok {
		if len([]rune(name)) != 1 {
			return
		}
		code = []rune(name)[0]
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	key := tea.Key{Code: code}
	if len([]rune(name)) == 1 {
		key.Text = name
	}
	// Only shared page editors receive these keys; no flight input or quit dispatch.
	if a.model.page == pageEngineering {
		a.model.engineeringKey(key)
	}
	if a.model.page == pageHardpoints {
		a.model.hardpointKey(key)
	}
}
func (a *DesktopApp) SavePreferences(p DesktopPreferences) error {
	if p.UIScale == 0 {
		p.UIScale = 100
	}
	if p.HealthDisplay == "" {
		p.HealthDisplay = "ship"
	}
	if !validDesktopPreferences(p) {
		return errors.New("invalid display preferences")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return errors.New("application is closing")
	}
	p.Fullscreen = a.prefs.Fullscreen
	if err := writeDesktopPreferences(a.settingsPath, p); err != nil {
		return err
	}
	a.prefs = p
	a.autoLayoutAttempt = ""
	a.syncAutomaticControls()
	selection, _ := themeIndex(p.Theme)
	a.model.themeChoice, a.model.savedTheme = selection, selection
	return nil
}
func (a *DesktopApp) SetFullscreen(enabled bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closing {
		return errors.New("application is closing")
	}
	// Runtime only: switching window mode never reads or writes preferences.
	a.fullscreenWindow(enabled)
	a.prefs.Fullscreen = enabled
	return nil
}
func (a *DesktopApp) PortraitWindow() error {
	if err := a.SetFullscreen(false); err != nil {
		return err
	}
	wr.WindowUnmaximise(a.ctx)
	wr.WindowSetSize(a.ctx, 720, 1100)
	wr.WindowCenter(a.ctx)
	return nil
}
func (a *DesktopApp) Minimise() { wr.WindowMinimise(a.ctx) }
func (a *DesktopApp) Close()    { wr.Quit(a.ctx) }

func (a *DesktopApp) GetState(width int) DesktopState {
	a.mu.Lock()
	defer a.mu.Unlock()
	m := &a.model
	if m.page != pageEngineering {
		a.materialSelection = ""
	}
	m.now = time.Now()
	m.width, m.height = min(160, max(40, width)), 80
	m.updateConnection()
	state := m.sessionIndicator()
	result := DesktopState{Outfitting: m.outfitSnapshot(), Page: m.page, Pages: flightPages, StationSearch: stationSearchProviders, Paused: m.frame.ControlStatus == "PAUSED", Status: state.label, Detail: state.detail, Ready: state.ready, Armed: state.armed,
		Mode: m.opts.mode, Theme: a.prefs.Theme, Preferences: a.prefs, Commander: m.companion.Commander,
		Ship: shipDisplayName(m.companion.Ship.Type), System: m.companion.Nav.System, Fuel: ansi.Strip(m.fuelLine()),
		Pips: ansi.Strip(m.pipsLine()), Warning: ansi.Strip(m.alertLine()), Action: m.frame.Action,
		Error: a.message, PageKeys: m.frame.PageStatus}
	if m.frame.State != nil {
		result.Context = m.frame.State.context()
	}
	result.ControlLayout = a.currentControlLayout()
	result.ShipModulesAt = reportedAt(m.companion.Ship.ModulesAt)
	if result.Error == "" {
		result.Error = a.layoutError
	}
	if m.fatal != nil {
		result.Error = m.fatal.Error()
	}
	for i, action := range liveActions {
		text := "READY"
		if action.Mask != 0 {
			text = "UNKNOWN"
			if m.known() {
				text = "INACTIVE"
				if m.frame.State.has(action.Mask) {
					text = "ACTIVE"
				}
			}
		}
		key := fmt.Sprint(i + 1)
		if i == 9 {
			key = "0"
		}
		kind := "trigger"
		if action.Mask != 0 {
			kind = "toggle"
		}
		availability := m.utilityAvailability(action.ID)
		result.Controls = append(result.Controls, DesktopControl{ID: action.ID, Label: action.Label, Key: "NUM " + key, State: text, Kind: kind, Permanent: permanentShipControl(action.ID), Availability: availability, Description: controlDescription(action.ID), Enabled: (m.mock != nil || state.ready && (m.frame.ControlStatus == "ARMED" || m.frame.ControlStatus == "PAUSED")) && shipControlAvailable(availability)})
	}
	result.Health, result.CockpitRoute = m.desktopHealth(), m.desktopRoute()
	result.PageView = m.desktopPageView()
	if m.page == pageShip {
		result.ShipModules = moduleRowsWithPower(m.companion.Ship)
		result.ShipPowerAt, result.ShipPowerError = reportedAt(m.companion.Ship.PowerInfoAt), m.companion.Ship.PowerInfoError
	}
	if m.page == pageEngineering {
		rows := a.desktopMaterialRows()
		selected := ""
		for _, row := range rows {
			if row.Key == a.materialSelection {
				selected = row.Key
				break
			}
		}
		a.materialSelection = selected
		note, notesError := a.materialNote(selected)
		result.Engineering = DesktopEngineering{Rows: rows, Selected: selected, Filter: m.materialFilter, Pinned: m.onlyPinned, Message: m.goalMessage, Note: note, NotesError: notesError}
		if m.opts.mode != "mock" {
			result.Engineering.PinsError = a.materialPinsError
		}
	}
	result.Text = ""
	if m.page == pageEngineering {
		result.Editor = m.goalMessage
		if m.goalEditing {
			result.Editor = "TARGET: " + string(m.goalDigits) + " | Enter saves; Esc cancels"
		}
		if m.materialSearching {
			result.Editor = "SEARCH: /" + m.materialFilter + " | Enter finishes; Esc clears"
		}
	}
	if m.page == pageHardpoints {
		result.Editor = m.fireGroupMessage
	}
	return result
}

func main() {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	path := filepath.Join(dir, "Co-pit", "window.json")
	prefs, settingsErr := loadDesktopPreferences(path)
	app := newDesktopApp(prefs, path)
	if settingsErr != nil {
		app.message = "Display preferences could not be read: " + settingsErr.Error()
	}
	assets, _ := fs.Sub(desktopAssets, "frontend")
	err = wails.Run(&options.App{Title: "Co-pit", Width: 1280, Height: 850, MinWidth: 360, MinHeight: 480,
		Frameless: true, WindowStartState: options.Normal, BackgroundColour: options.NewRGB(10, 12, 14),
		AssetServer: &assetserver.Options{Assets: assets}, OnStartup: app.startup, OnShutdown: app.shutdown,
		HideWindowOnClose: false, Bind: []interface{}{app}, Windows: &windows.Options{Theme: windows.Dark},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "3a4b3c57-d745-4b24-932f-c580ce143e36"}})
	if err != nil && !strings.Contains(err.Error(), "already running") {
		fmt.Fprintln(os.Stderr, err)
	}
}
