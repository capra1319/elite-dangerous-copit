package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

type cockpitOptions struct {
	mode, controls, path, config string
	settings                     string
	journal, goals, firegroups   string
	interval, stale              time.Duration
	boot                         bool
}

const (
	pageCockpit = iota
	pageDiagnostics
	pageHelp
	pageSettings
	pageShip
	pageNavigation
	pageMissions
	pageEngineering
	pageHardpoints
	pageCount
)

type dashboardFrame struct {
	State                                                           *Snapshot
	Mode, Controls, Source, Quality, Action, ControlStatus, Pending string
	GameFocused                                                     bool
	Received                                                        time.Time
	PageStatus                                                      string
}
type clockMsg time.Time
type scanMsg struct{}
type pageStepMsg int
type gamePresenceMsg gamePresence
type backendDoneMsg struct{ err error }
type activity struct {
	time time.Time
	text string
}

type cockpitModel struct {
	opts                       cockpitOptions
	frame                      dashboardFrame
	mock                       *Mock
	now                        time.Time
	width, height, page        int
	command                    []rune
	held                       map[rune]bool
	history                    []activity
	requests                   chan controlRequest
	backend                    tea.Cmd
	work                       *sessionWork
	scenario                   int
	scroll                     int
	fatal                      error
	presence                   gamePresence
	connected                  bool
	scanFrame                  int
	scanTickPending            bool
	switchMode                 bool
	themeChoice                int
	savedTheme                 int
	settingsMessage            string
	companion                  companionData
	companionError             string
	journalReader              *journalReader
	goals                      map[string]int
	goalMessage                string
	materialIndex              int
	onlyPinned                 bool
	materialFilter             string
	materialSearching          bool
	goalEditing                bool
	goalReplace                bool
	goalKey                    string
	goalDigits                 []rune
	firegroups                 fireGroupFile
	fireGroupMessage           string
	fireGroupLoadError         bool
	hardpointIndex, groupIndex int
	bootActive                 bool
	bootStarted                time.Time
	bootElapsed                time.Duration
	stationSearchActive        bool
	stationSearchIndex         int
	stationSearchMessage       string
	outfitting                 outfittingState
}

func cloneSnapshot(s *Snapshot) *Snapshot {
	if s == nil {
		return nil
	}
	c := *s
	if s.Flags != nil {
		c.Flags = u64(*s.Flags)
	}
	if s.Flags2 != nil {
		c.Flags2 = u64(*s.Flags2)
	}
	if s.GuiFocus != nil {
		c.GuiFocus = integer(*s.GuiFocus)
	}
	if s.FireGroup != nil {
		c.FireGroup = integer(*s.FireGroup)
	}
	c.Pips = append([]int(nil), s.Pips...)
	if s.Fuel != nil {
		f := *s.Fuel
		c.Fuel = &f
	}
	if s.Cargo != nil {
		cargo := *s.Cargo
		c.Cargo = &cargo
	}
	return &c
}

func newCockpit(opts cockpitOptions, now time.Time) cockpitModel {
	if opts.interval <= 0 {
		opts.interval = defaultTelemetryInterval
	}
	if opts.stale <= 0 {
		opts.stale = time.Minute
	}
	m := cockpitModel{opts: opts, now: now, width: 110, height: 36, held: make(map[rune]bool)}
	m.bootActive, m.bootStarted = opts.boot, now
	selection, err := loadUISettings(opts.settings)
	m.themeChoice, m.savedTheme = selection, selection
	if err != nil {
		m.settingsMessage = "Could not load settings: " + err.Error()
	}
	m.frame = dashboardFrame{Mode: opts.mode, Controls: opts.controls, Source: opts.path, ControlStatus: "DISARMED", Quality: "Waiting for ship telemetry", Action: "Ready", Received: now}
	m.companion = newCompanionData()
	m.goals, err = loadMaterialGoals(opts.goals)
	if err != nil {
		m.goalMessage = "Could not load targets: " + err.Error()
	}
	m.firegroups, err = loadFireGroups(opts.firegroups)
	if err != nil {
		m.fireGroupMessage = "Could not load group reference: " + err.Error()
		m.fireGroupLoadError = true
	}
	if opts.mode != "mock" && opts.journal != "" {
		m.journalReader = newJournalReader(opts.journal)
	}
	if opts.mode == "mock" {
		m.companion = mockCompanionData(now)
		m.mock = newMock(now)
		m.syncMock("Simulator ready — try your controls")
	}
	m.scanTickPending = m.scanning()
	return m
}

func (m cockpitModel) tickCockpit() tea.Cmd {
	return m.work.command(time.Second, func() tea.Msg { return clockMsg(time.Now()) })
}
func (m cockpitModel) Init() tea.Cmd {
	commands := []tea.Cmd{m.tickCockpit(), m.backend, m.pollCompanion(false)}
	if m.bootActive {
		commands = append(commands, m.tickBoot())
	}
	if m.opts.mode != "mock" {
		commands = append(commands, m.work.command(0, func() tea.Msg { return gamePresenceMsg(probeGamePresence()) }))
	}
	if m.scanTickPending {
		commands = append(commands, m.tickScanner())
	}
	return tea.Batch(commands...)
}

func (m cockpitModel) pollCompanion(delay bool) tea.Cmd {
	if m.journalReader == nil {
		return nil
	}
	interval := time.Duration(0)
	if delay {
		interval = 250 * time.Millisecond
	}
	return m.work.command(interval, func() tea.Msg { return m.journalReader.poll() })
}

func (m cockpitModel) tickScanner() tea.Cmd {
	return m.work.command(100*time.Millisecond, func() tea.Msg { return scanMsg{} })
}

func (m *cockpitModel) continueScanning() tea.Cmd {
	if m.scanning() && !m.scanTickPending {
		m.scanTickPending = true
		return m.tickScanner()
	}
	return nil
}

func (m cockpitModel) usableSession() bool {
	s := m.frame.State
	return s != nil && s.Flags != nil && (s.has(mainShip|fighter|srv) || s.has2(7)) &&
		!strings.HasPrefix(m.frame.Quality, "UNAVAILABLE") && !strings.Contains(m.frame.Quality, "READ ERROR")
}

func (m *cockpitModel) updateConnection() {
	if !m.presence.Checked || (m.presence.Known && !m.presence.Running) || !m.usableSession() {
		m.connected = false
		return
	}
	age := m.now.Sub(m.frame.State.Timestamp)
	if age < -futureSnapshotTolerance {
		m.connected = false
	} else if age <= m.opts.stale {
		m.connected = true
	}
	// Once connected, an unchanged snapshot is not a lost-game heartbeat.
}

func (m cockpitModel) scanning() bool {
	return m.opts.mode != "mock" && m.fatal == nil && !m.connected
}

func (m *cockpitModel) record(text string) {
	if text == "" || (len(m.history) > 0 && m.history[0].text == text) {
		return
	}
	m.history = append([]activity{{time: m.now, text: text}}, m.history...)
	if len(m.history) > 30 {
		m.history = m.history[:30]
	}
}
func (m *cockpitModel) syncMock(action string) {
	m.frame = dashboardFrame{State: cloneSnapshot(&m.mock.State), Mode: "mock", Controls: m.opts.controls, Quality: "SIMULATED", Action: action, ControlStatus: "SIMULATION", Received: m.now}
	m.record(action)
}
func (m *cockpitModel) mockAction(command string) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(command)))
	if len(fields) == 0 {
		return
	}
	action, err := mockCommand(m.mock, fields, m.now)
	if err != nil {
		action = "Blocked: " + err.Error()
	}
	m.syncMock(action)
}

func (m cockpitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case outfittingMsg:
		m.applyOutfitting(msg)
		return m, nil
	case stationSearchOpenedMsg:
		m.stationSearchResult(msg)
		return m, nil
	case bootMsg:
		if !m.bootActive {
			return m, nil
		}
		m.bootElapsed = max(m.bootElapsed, time.Time(msg).Sub(m.bootStarted))
		if m.bootElapsed >= bootDuration {
			m.bootActive = false
			return m, nil
		}
		return m, m.tickBoot()
	case tea.WindowSizeMsg:
		if msg.Width > 0 && msg.Height > 0 {
			m.width, m.height = msg.Width, msg.Height
		}
	case clockMsg:
		// A delayed tick may arrive after a newer telemetry frame.
		if stamp := time.Time(msg); stamp.After(m.now) {
			m.now = stamp
		}
		m.updateConnection()
		scan := m.continueScanning()
		return m, tea.Batch(m.tickCockpit(), scan)
	case pageStepMsg:
		m.bootActive = false
		m.stepPage(int(msg))
		return m, nil
	case scanMsg:
		m.scanTickPending = false
		if m.scanning() {
			m.scanFrame = (m.scanFrame + 1) % 120
		}
		scan := m.continueScanning()
		return m, scan
	case gamePresenceMsg:
		m.presence = gamePresence(msg)
		m.updateConnection()
		var poll tea.Cmd
		if m.fatal == nil {
			poll = m.work.command(250*time.Millisecond, func() tea.Msg { return gamePresenceMsg(probeGamePresence()) })
		}
		scan := m.continueScanning()
		return m, tea.Batch(poll, scan)
	case dashboardFrame:
		m.now = time.Now()
		if msg.Action == "" {
			msg.Action = m.frame.Action
			if m.frame.State == nil && msg.State != nil {
				msg.Action = "Reading ship telemetry"
			}
		}
		m.frame = msg
		if msg.ControlStatus == "ARMED" {
			m.bootActive = false
		}
		m.record(msg.Action)
		m.updateConnection()
		scan := m.continueScanning()
		return m, scan
	case companionMsg:
		m.companion, m.companionError = msg.Data, msg.Error
		if m.journalReader != nil && m.journalReader.ctx.Err() == nil {
			return m, m.pollCompanion(true)
		}
		return m, nil
	case backendDoneMsg:
		if msg.err != nil {
			m.outfitting.cancelRequest()
			m.outfitting.Error = "Connection session stopped; restart before searching"
			if m.work != nil {
				m.work.cancel()
			}
			m.fatal = msg.err
			m.frame.ControlStatus = "DISARMED"
			m.frame.Action = msg.err.Error()
			m.frame.Quality = "Connection stopped"
			m.record(msg.err.Error())
		} else {
			return m, tea.Quit
		}
	case tea.BlurMsg:
		m.held = make(map[rune]bool)
	case tea.KeyReleaseMsg:
		key := msg.Key()
		delete(m.held, numpadCode(key))
	case tea.PasteMsg:
		if m.page == pageNavigation && m.outfitting.Active {
			text := safeText(msg.Content)
			if runes := []rune(text); len(runes) > 128 {
				text = string(runes[:128])
			}
			if m.outfitting.editSystem {
				m.outfitting.Input = text
				m.outfitting.replaceInput = false
			} else if m.outfitting.Stage == "modules" {
				m.outfitting.Filter = text
				m.outfitting.ModuleOffset = 0
			} else {
				m.outfitting.StationFilter = text
				m.outfitting.StationIndex = 0
			}
		} else if m.page == pageEngineering && m.goalEditing {
			digits := strings.TrimSpace(msg.Content)
			if len(digits) > 0 && len(digits) <= 6 && strings.Trim(digits, "0123456789") == "" {
				m.goalDigits, m.goalReplace = []rune(digits), false
			}
		} else if m.page == pageEngineering {
			m.materialSearching = true
			m.materialFilter = safeText(msg.Content)
			if runes := []rune(m.materialFilter); len(runes) > 80 {
				m.materialFilter = string(runes[:80])
			}
			m.materialIndex = 0
		} else if m.mock != nil && m.opts.controls == "commands" && m.page == pageCockpit {
			text := strings.ReplaceAll(strings.ReplaceAll(msg.Content, "\n", " "), "\r", " ")
			m.command = append(m.command, []rune(text)...)
			if len(m.command) > 240 {
				m.command = m.command[:240]
			}
		}
	case tea.KeyPressMsg:
		key := msg.Key()
		if m.bootActive && !key.IsRepeat {
			m.bootActive = false
			if key.Mod == 0 && (key.Code == ' ' || key.Code == tea.KeyEnter || key.Code == tea.KeyKpEnter) {
				return m, nil
			}
		}
		if delta := numpadPageStep(key); delta != 0 {
			// Control mode receives page keys through WM_HOTKEY;
			// do not also apply a terminal key event for the same physical press.
			if m.opts.mode == "control" && m.fatal == nil && !strings.HasPrefix(m.frame.PageStatus, "Global page keys unavailable") {
				return m, nil
			}
			if key.IsRepeat {
				return m, nil
			}
			code := numpadCode(key)
			if m.held[code] {
				return m, nil
			}
			m.held[code] = true
			m.stepPage(delta)
			return m, nil
		}
		if m.page == pageEngineering && m.engineeringKey(key) {
			return m, nil
		}
		if m.page == pageNavigation {
			if handled, cmd := m.outfittingKey(key); handled {
				return m, cmd
			}
			if handled, cmd := m.stationSearchKey(key); handled {
				return m, cmd
			}
		}
		if m.page == pageHardpoints && m.hardpointKey(key) {
			return m, nil
		}
		if m.page == pageSettings && key.Mod == 0 {
			switch key.Code {
			case tea.KeyUp:
				m.themeChoice = (m.themeChoice + len(tuiThemes) - 1) % len(tuiThemes)
				m.settingsMessage = "Preview only. Enter applies and saves."
				return m, nil
			case tea.KeyDown:
				m.themeChoice = (m.themeChoice + 1) % len(tuiThemes)
				m.settingsMessage = "Preview only. Enter applies and saves."
				return m, nil
			case tea.KeyEnter, tea.KeyKpEnter:
				if err := saveUISettings(m.opts.settings, m.themeChoice); err != nil {
					m.settingsMessage = "Could not save: " + err.Error()
				} else {
					m.savedTheme = m.themeChoice
					m.settingsMessage = "Saved: " + m.palette().Name
				}
				return m, nil
			}
		}
		switch key.Code {
		case tea.KeyEscape:
			if m.page != 0 {
				m.leaveSettings()
				m.page = 0
				return m, nil
			}
			return m, tea.Quit
		case tea.KeyTab:
			step := 1
			if key.Mod&tea.ModShift != 0 {
				step = -1
			}
			m.stepPage(step)
			return m, nil
		case tea.KeyF6, tea.KeyF7, tea.KeyF8, tea.KeyF9, tea.KeyF10:
			if key.Mod != 0 || key.IsRepeat {
				return m, nil
			}
			m.leaveSettings()
			m.page = pageShip + int(key.Code-tea.KeyF6)
			m.scroll = 0
			return m, nil
		case tea.KeyF1:
			m.leaveSettings()
			m.scroll = 0
			if m.page == 2 {
				m.page = 0
			} else {
				m.page = 2
			}
			return m, nil
		case tea.KeyF5:
			if key.IsRepeat || key.Mod != 0 {
				return m, nil
			}
			if m.page == pageSettings {
				m.leaveSettings()
				m.page = pageCockpit
			} else {
				m.page = pageSettings
			}
			m.scroll = 0
			return m, nil
		case tea.KeyF2:
			if m.mock != nil {
				scenarios := []string{"flight", "docked", "cruise", "danger", "srv", "foot", "unknown"}
				m.scenario = (m.scenario + 1) % len(scenarios)
				m.mockAction("scenario " + scenarios[m.scenario])
			}
			return m, nil
		case tea.KeyF3:
			if m.mock != nil {
				if m.opts.controls == "commands" {
					m.opts.controls = "numpad"
				} else {
					m.opts.controls = "commands"
				}
				m.command = nil
				m.held = make(map[rune]bool)
				m.syncMock("Input type: " + m.opts.controls)
			}
			return m, nil
		case tea.KeyF4:
			if key.IsRepeat || key.Mod != 0 {
				return m, nil
			}
			// The session runner stops and joins the backend before changing mode.
			m.switchMode = true
			return m, tea.Quit
		}
		if m.page != 0 && (key.Code == tea.KeyDown || key.Code == tea.KeyPgDown) {
			step := 1
			if key.Code == tea.KeyPgDown {
				step = 8
			}
			m.scroll += step
			return m, nil
		}
		if m.page != 0 && (key.Code == tea.KeyUp || key.Code == tea.KeyPgUp) {
			step := 1
			if key.Code == tea.KeyPgUp {
				step = 8
			}
			m.scroll = max(0, m.scroll-step)
			return m, nil
		}
		if key.Code == 'c' && key.Mod&tea.ModCtrl != 0 {
			return m, tea.Quit
		}
		if key.Code == 'q' && (m.page != pageCockpit || len(m.command) == 0) {
			return m, tea.Quit
		}
		if m.opts.mode == "control" && key.Code == 'd' {
			requestStop(m.requests)
			return m, nil
		}
		if m.mock == nil {
			return m, nil
		} // Real game input belongs only to the guarded global-hotkey backend.
		if m.page != 0 {
			return m, nil
		}
		if m.opts.controls == "numpad" {
			code := numpadCode(key)
			if code >= tea.KeyKp0 && code <= tea.KeyKp9 && key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModShift|tea.ModSuper) == 0 {
				if key.IsRepeat || m.held[code] {
					return m, nil
				}
				m.held[code] = true
				n := int(code - tea.KeyKp0)
				index := n - 1
				if n == 0 {
					index = 9
				}
				command := liveActions[index].ID
				if n >= 1 && n <= 4 {
					command += " toggle"
				}
				m.mockAction(command)
			} else if key.Code == tea.KeyEnter || key.Code == tea.KeyKpEnter {
				m.mockAction("close")
			}
			return m, nil
		}
		if key.Code == tea.KeyEnter || key.Code == tea.KeyKpEnter {
			command := string(m.command)
			m.command = nil
			if command == "help" {
				m.page = 2
			} else if command == "q" || command == "quit" || command == "exit" {
				return m, tea.Quit
			} else {
				m.mockAction(command)
			}
		} else if key.Code == tea.KeyBackspace {
			if len(m.command) > 0 {
				m.command = m.command[:len(m.command)-1]
			}
		} else if key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 && key.Text != "" {
			m.command = append(m.command, []rune(key.Text)...)
			if len(m.command) > 240 {
				m.command = m.command[:240]
			}
		}
	}
	return m, nil
}

func numpadCode(key tea.Key) rune {
	if key.BaseCode >= tea.KeyKp0 && key.BaseCode <= tea.KeyKp9 || key.BaseCode == tea.KeyKpMinus || key.BaseCode == tea.KeyKpPlus {
		return key.BaseCode
	}
	return key.Code
}

func runCockpit(ctx context.Context, opts cockpitOptions, input io.Reader, output io.Writer) error {
	return runCockpitSessions(ctx, opts, func(opts cockpitOptions) (cockpitModel, error) {
		return runCockpitSession(ctx, opts, input, output)
	})
}

// Each session owns its backend and waits for it to release all hotkeys before
// returning. No real reader or control engine survives entry into the simulator.
func runCockpitSessions(ctx context.Context, opts cockpitOptions, session func(cockpitOptions) (cockpitModel, error)) error {
	realOpts := opts
	realOpts.boot = false
	if realOpts.mode == "mock" {
		realOpts.mode, realOpts.controls = "control", "numpad"
	}
	for ctx.Err() == nil {
		final, err := session(opts)
		if err != nil {
			return err
		}
		if !final.switchMode {
			return nil
		}
		if opts.mode == "mock" {
			opts = realOpts
		} else {
			opts.mode, opts.controls = "mock", "numpad"
			opts.boot = false
		}
	}
	return nil
}

func runCockpitSession(ctx context.Context, opts cockpitOptions, input io.Reader, output io.Writer) (result cockpitModel, sessionErr error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	work := newSessionWork(child)
	defer func() {
		cancel()
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-work.stop():
		case <-timer.C:
			sessionErr = errors.Join(sessionErr, errors.New("session workers did not stop in time; refusing to start another session"))
		}
	}()
	requests := make(chan controlRequest, 4)
	m := newCockpit(opts, time.Now())
	m.work = work
	if m.journalReader != nil {
		m.journalReader.ctx = work.ctx
	}
	m.requests = requests
	var program *tea.Program
	if opts.mode != "mock" {
		m.backend = func() tea.Msg {
			d := display{out: io.Discard, controls: opts.controls, requests: requests, publish: func(frame dashboardFrame) { program.Send(frame) },
				navigate: func(delta int) { program.Send(pageStepMsg(delta)) }}
			if opts.mode == "control" {
				return backendDoneMsg{err: runControl(work.ctx, d, opts.path, opts.config, opts.interval, opts.stale, false)}
			}
			code := runLive(work.ctx, d, opts.path, opts.interval, opts.stale, false)
			if code != 0 {
				return backendDoneMsg{err: fmt.Errorf("telemetry reader stopped (%d)", code)}
			}
			return backendDoneMsg{}
		}
	}
	backend := m.backend
	m.backend = nil
	options := []tea.ProgramOption{tea.WithContext(child), tea.WithInput(input), tea.WithOutput(output), tea.WithFPS(30), tea.WithWindowSize(m.width, m.height)}
	if os.Getenv("NO_COLOR") == "" {
		options = append(options, tea.WithColorProfile(colorprofile.TrueColor))
	}
	program = tea.NewProgram(m, options...)
	if backend != nil {
		go work.command(0, func() tea.Msg { program.Send(safeBackend(backend)); return nil })()
	}
	final, err := program.Run()
	result, _ = final.(cockpitModel)
	if result.fatal != nil && !result.switchMode {
		return result, result.fatal
	}
	if errors.Is(err, tea.ErrInterrupted) || (ctx.Err() != nil && !errors.Is(err, tea.ErrProgramPanic)) {
		result.switchMode = false
		return result, nil
	}
	return result, err
}
