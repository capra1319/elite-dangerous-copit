package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type display struct {
	out           io.Writer
	ansi          bool
	controls      string
	controlStatus string
	screen        *terminalScreen
	publish       func(dashboardFrame)
	requests      <-chan controlRequest
	pending       string
	gameFocused   bool
	navigate      func(int)
	pageStatus    string
}

const defaultTelemetryInterval = 100 * time.Millisecond

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, input io.Reader, output, errout io.Writer) int {
	fs := flag.NewFlagSet("eliteaux", flag.ContinueOnError)
	fs.SetOutput(errout)
	mode := fs.String("mode", "control", "control (default; real game, starts disarmed), live (read-only), or mock (opt-in simulator)")
	controls := fs.String("controls", "", "input type: commands or numpad (default: numpad for control, commands otherwise)")
	config := fs.String("config", defaultControlConfig(), "real-game control configuration file")
	settings := fs.String("settings", defaultSettingsPath(), "appearance settings file")
	journal := fs.String("journal", "", "Player Journal folder (default: Status.json folder)")
	goals := fs.String("goals", "", "engineering targets file (default: engineering-goals.json beside appearance settings)")
	firegroups := fs.String("fire-groups", "", "manual fire-group reference file (default: fire-groups.json beside appearance settings)")
	ui := fs.String("ui", "tui", "interface: tui (Bubble Tea cockpit) or classic")
	boot := fs.Bool("boot", true, "show the brief terminal boot animation (use -boot=false to skip)")
	path := fs.String("status", defaultStatusPath(), "path to Elite Dangerous Status.json")
	once := fs.Bool("once", false, "print one snapshot and exit")
	plain := fs.Bool("plain", false, "print snapshots without clearing the terminal")
	interval := fs.Duration("interval", defaultTelemetryInterval, "live polling interval (minimum 100ms)")
	stale := fs.Duration("stale-after", 60*time.Second, "mark snapshots older than this as stale; not a game heartbeat")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(errout, "Unexpected argument; use -help for options.")
		return 2
	}
	if *ui != "tui" && *ui != "classic" {
		fmt.Fprintln(errout, "Interface must be tui or classic.")
		return 2
	}
	if *mode != "mock" && *mode != "live" && *mode != "control" {
		fmt.Fprintln(errout, "Mode must be mock, live or control.")
		return 2
	}
	if *controls == "" {
		*controls = "commands"
		if *mode == "control" {
			*controls = "numpad"
		}
	}
	if *controls != "commands" && *controls != "numpad" {
		fmt.Fprintln(errout, "Controls must be commands or numpad.")
		return 2
	}
	if *mode == "live" && *controls == "numpad" {
		fmt.Fprintln(errout, "Live mode is read-only. Use -mode control -controls numpad for real-game control.")
		return 2
	}
	if *mode == "control" && *controls != "numpad" {
		fmt.Fprintln(errout, "Real-game control requires -controls numpad. Mock typed commands remain available.")
		return 2
	}
	if *interval < 100*time.Millisecond || *stale <= 0 {
		fmt.Fprintln(errout, "Use interval >= 100ms and stale-after > 0.")
		return 2
	}
	if *journal == "" {
		*journal = filepath.Dir(*path)
	}
	if *goals == "" {
		*goals = filepath.Join(filepath.Dir(*settings), "engineering-goals.json")
	}
	if *firegroups == "" {
		*firegroups = filepath.Join(filepath.Dir(*settings), "fire-groups.json")
	}
	d := display{out: output, controls: *controls}
	if !*plain && output == os.Stdout {
		if stat, err := os.Stdout.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
			d.ansi = enableANSI()
		}
	}
	if d.ansi {
		d.screen = &terminalScreen{}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *ui == "tui" && !*plain && !*once && input == os.Stdin && output == os.Stdout {
		if err := runCockpit(ctx, cockpitOptions{mode: *mode, controls: *controls, path: *path, config: *config, settings: *settings, journal: *journal, goals: *goals, firegroups: *firegroups, interval: *interval, stale: *stale, boot: *boot}, input, output); err != nil {
			fmt.Fprintln(errout, err)
			return 1
		}
		return 0
	}
	if *mode == "control" {
		if err := runControl(ctx, d, *path, *config, *interval, *stale, *once); err != nil {
			fmt.Fprintln(errout, err)
			return 1
		}
		return 0
	}
	if *mode == "mock" {
		if *controls == "numpad" {
			if *once {
				m := newMock(time.Now())
				d.show(&m.State, "mock", "", "SIMULATED -- no game or hardware needed", "Ready", time.Now())
				return 0
			}
			file, ok := input.(*os.File)
			if !ok {
				fmt.Fprintln(errout, "Numpad input requires an interactive Windows console. Use -controls commands for text input.")
				return 2
			}
			if err := runNumpad(ctx, file, d); err != nil {
				fmt.Fprintln(errout, err)
				return 1
			}
			return 0
		}
		return runMock(ctx, input, d, *once)
	}
	return runLive(ctx, d, *path, *interval, *stale, *once)
}

func defaultStatusPath() string {
	if dir := savedGamesPath(); dir != "" {
		return filepath.Join(dir, "Frontier Developments", "Elite Dangerous", "Status.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Saved Games", "Frontier Developments", "Elite Dangerous", "Status.json")
}

func (d display) show(s *Snapshot, mode, source, quality, action string, now time.Time) {
	if d.publish != nil {
		d.publish(dashboardFrame{State: cloneSnapshot(s), Mode: mode, Controls: d.controls, Source: source, Quality: quality, Action: action, ControlStatus: d.controlStatus, Pending: d.pending, GameFocused: d.gameFocused, Received: now, PageStatus: d.pageStatus})
		return
	}
	output := d.out
	var frame strings.Builder
	d.out = &frame
	fmt.Fprintln(d.out, "ELITE AUX PANEL | Ship Systems")
	fmt.Fprintln(d.out, strings.Repeat("=", 72))
	if mode == "mock" {
		fmt.Fprintln(d.out, "MODE: MOCK SIMULATOR -- all controls affect simulated state only")
		if d.controls == "numpad" {
			fmt.Fprintln(d.out, "CONTROLS: NUMPAD -- immediate key presses; keep this window focused")
		} else {
			fmt.Fprintln(d.out, "CONTROLS: TYPED COMMANDS -- type a command, then press Enter")
		}
	} else if mode == "control" {
		fmt.Fprintln(d.out, "MODE: REAL GAME CONTROL -- keyboard output + actual Status.json feedback")
		fmt.Fprintln(d.out, "CONTROL:", d.controlStatus, "| Ctrl+Alt+F9: arm/disarm | Ctrl+Alt+F10: stop")
		fmt.Fprintln(d.out, "Source:", source)
	} else {
		fmt.Fprintln(d.out, "MODE: LIVE FILE / READ ONLY -- no game input is sent")
		fmt.Fprintln(d.out, "Source:", source)
	}
	fmt.Fprintln(d.out, "Data:", quality)
	if s != nil {
		age := now.Sub(s.Timestamp).Round(time.Second)
		fmt.Fprintf(d.out, "Context: %s | Snapshot age: %s\n", s.context(), age)
		fmt.Fprintln(d.out, "Timestamp:", s.Timestamp.UTC().Format(time.RFC3339))
	} else {
		fmt.Fprintln(d.out, "Context: UNKNOWN | Waiting for a valid snapshot")
	}
	fmt.Fprintln(d.out)
	fmt.Fprintln(d.out, "     CONTROL              ACTUAL       FEEDBACK LED")
	for i, sw := range switches {
		state, led := "UNKNOWN", "[?]"
		if s != nil && s.shipFeedbackKnown() {
			if s.has(sw.Mask) {
				state, led = "ON", "[*]"
			} else {
				state, led = "OFF", "[ ]"
			}
		}
		fmt.Fprintf(d.out, "  %s %-20s %-12s L%d %s\n", strings.ToUpper(sw.ID), sw.Name, state, i+1, led)
	}
	fmt.Fprintln(d.out)
	fmt.Fprintln(d.out, "  B1 Night vision    B2 Galaxy map     B3 System map")
	fmt.Fprintln(d.out, "  B4 Next fire group B5 Heat sink      B6 Chaff")
	fmt.Fprintln(d.out)
	if s != nil && s.shipFeedbackKnown() {
		fmt.Fprintf(d.out, "Shields: %-3s  Flight assist: %-3s  Night vision: %-3s\n", onoff(s.has(shields)), onoff(!s.has(assistOff)), onoff(s.has(nightVision)))
		fmt.Fprintf(d.out, "FSD: mass lock %s | charging %s | cooldown %s\n", onoff(s.has(massLocked)), onoff(s.has(charging)), onoff(s.has(cooldown)))
		fmt.Fprintf(d.out, "Warnings: low fuel %s | overheating %s | silent running %s\n", onoff(s.has(lowFuel)), onoff(s.has(overheating)), onoff(s.has(silent)))
	} else {
		fmt.Fprintln(d.out, "Ship indicators: UNKNOWN / unavailable in this context")
	}
	if s != nil {
		focus, group := "unknown", "unknown"
		if s.GuiFocus != nil {
			focus = fmt.Sprint(*s.GuiFocus)
		}
		if s.FireGroup != nil {
			group = fmt.Sprint(*s.FireGroup + 1)
		}
		fmt.Fprintf(d.out, "GUI focus: %s (0 = no panel) | Fire group: %s\n", focus, group)
		if s.Flags2 == nil {
			fmt.Fprintln(d.out, "Flags2 missing: on-foot/taxi/multicrew context not fully available.")
		}
	}
	fmt.Fprintln(d.out, "LED legend: [*] on  [ ] off  [?] unknown; stale values are LAST KNOWN.")
	if action != "" {
		fmt.Fprintln(d.out, "Last action:", action)
	}
	if mode == "mock" {
		if d.controls == "numpad" {
			fmt.Fprintln(d.out, "Num1 Gear | Num2 Scoop | Num3 Lights | Num4 Hardpoints | Num5 Night vision")
			fmt.Fprintln(d.out, "Num6 Galaxy map | Num7 System map | Num8 Fire group | Num9 Heat sink | Num0 Chaff")
			fmt.Fprintln(d.out, "Num Lock ON. Enter: close map | F1: help | Esc: quit. Hold = one action.")
		} else {
			fmt.Fprintln(d.out, "Try: t1 on | t2 toggle | b1 | scenario docked | external t1 off | help | q")
		}
	} else if mode == "control" {
		fmt.Fprintln(d.out, "Num1 Gear | Num2 Scoop | Num3 Lights | Num4 Hardpoints | Num5 Night vision")
		fmt.Fprintln(d.out, "Num6 Galaxy map | Num7 System map | Num8 Fire group | Num9 Heat sink | Num0 Chaff")
		fmt.Fprintln(d.out, "Keep Elite focused. Num Lock ON. Same map key closes it. Ctrl+C quits app.")
	} else {
		fmt.Fprintln(d.out, "Ctrl+C to exit. Snapshot age does not prove whether the game is running.")
	}
	if d.ansi {
		screen := d.screen
		if screen == nil {
			screen = &terminalScreen{}
		}
		width, height := terminalDimensions()
		screen.render(output, frame.String(), width, height)
	} else {
		io.WriteString(output, frame.String())
	}
}

func onoff(v bool) string {
	if v {
		return "ON"
	}
	return "OFF"
}

func qualityFor(s *Snapshot, readErr error, now time.Time, stale time.Duration) string {
	if readErr != nil {
		if s != nil {
			return "READ ERROR -- showing LAST KNOWN state: " + readErr.Error()
		}
		return "UNAVAILABLE: " + readErr.Error()
	}
	if s == nil {
		return "UNKNOWN"
	}
	if now.Sub(s.Timestamp) < -futureSnapshotTolerance {
		return "CLOCK MISMATCH -- future snapshot; check system time"
	}
	if now.Sub(s.Timestamp) > stale {
		return "STALE -- LAST KNOWN snapshot; may be unchanged or game may be closed"
	}
	return "READABLE / RECENT SNAPSHOT -- game session not independently verified"
}

func readWithRetry(ctx context.Context, path string) (Snapshot, error) {
	var s Snapshot
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		s, err = readStatus(path)
		if err == nil {
			return s, nil
		}
		// A missing file needs the next poll, not retries that block hotkeys.
		if os.IsNotExist(err) {
			return s, err
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return Snapshot{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return s, err
}

func runLive(ctx context.Context, d display, path string, interval, stale time.Duration, once bool) int {
	var last *Snapshot
	lastPrinted := ""
	for {
		s, err := readWithRetry(ctx, path)
		if ctx.Err() != nil {
			fmt.Fprintln(d.out, "\nStopped.")
			return 0
		}
		if err == nil {
			last = &s
		}
		now := time.Now()
		quality := qualityFor(last, err, now, stale)
		// Plain output stays readable: print only snapshot or quality changes.
		key := quality
		if last != nil {
			key += fmt.Sprintf("|%s|%d|%v|%v|%v|%v", last.Timestamp, *last.Flags, derefU64(last.Flags2), derefInt(last.GuiFocus), derefInt(last.FireGroup), last.Pips)
		}
		if d.ansi || key != lastPrinted {
			d.show(last, "live", path, quality, "", now)
			lastPrinted = key
		}
		if once {
			if err != nil {
				return 1
			}
			return 0
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			fmt.Fprintln(d.out, "\nStopped.")
			return 0
		case <-timer.C:
		}
	}
}

func derefU64(p *uint64) string {
	if p == nil {
		return "missing"
	}
	return fmt.Sprint(*p)
}
func derefInt(p *int) string {
	if p == nil {
		return "missing"
	}
	return fmt.Sprint(*p)
}

func runMock(ctx context.Context, input io.Reader, d display, once bool) int {
	m := newMock(time.Now())
	d.show(&m.State, "mock", "", "SIMULATED -- no game or hardware needed", "Ready", time.Now())
	if once {
		return 0
	}
	child, cancel := context.WithCancel(ctx)
	reader, err := prepareMockInput(child, input, d.out)
	if err != nil {
		cancel()
		fmt.Fprintln(d.out, err)
		return 1
	}
	done := make(chan struct{})
	stopCancel := context.AfterFunc(child, reader.cancel)
	defer func() {
		cancel()
		stopCancel()
		reader.cancel()
		<-done
		reader.close()
	}()
	type scanResult struct {
		line string
		err  error
		end  bool
	}
	lines := make(chan scanResult)
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			select {
			case lines <- scanResult{line: scanner.Text()}:
			case <-child.Done():
				return
			}
		}
		select {
		case lines <- scanResult{err: scanner.Err(), end: true}:
		case <-child.Done():
		}
	}()
	fmt.Fprint(d.out, "mock> ")
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(d.out, "\nStopped.")
			return 0
		case result := <-lines:
			if ctx.Err() != nil {
				fmt.Fprintln(d.out, "\nStopped.")
				return 0
			}
			if result.end {
				if result.err != nil {
					fmt.Fprintln(d.out, result.err)
					return 1
				}
				return 0
			}
			fields := strings.Fields(strings.ToLower(result.line))
			if len(fields) == 0 {
				fmt.Fprint(d.out, "mock> ")
				continue
			}
			if fields[0] == "q" || fields[0] == "quit" || fields[0] == "exit" {
				fmt.Fprintln(d.out, "Goodbye.")
				return 0
			}
			if fields[0] == "help" {
				fmt.Fprintln(d.out, mockHelp)
				d.invalidateScreen()
				fmt.Fprint(d.out, "mock> ")
				continue
			}
			action, err := mockCommand(m, fields, time.Now())
			if err != nil {
				action = "Blocked: " + err.Error()
			}
			d.invalidateScreen()
			d.show(&m.State, "mock", "", "SIMULATED -- no game or hardware needed", action, time.Now())
			fmt.Fprint(d.out, "mock> ")
		}
	}
}

func mockCommand(m *Mock, fields []string, now time.Time) (string, error) {
	switch {
	case len(fields) == 2 && strings.HasPrefix(fields[0], "t"):
		return m.setSwitch(fields[0], fields[1], false, now)
	case len(fields) == 1 && strings.HasPrefix(fields[0], "b"):
		return m.button(fields[0], now)
	case len(fields) == 3 && fields[0] == "external":
		return m.setSwitch(fields[1], fields[2], true, now)
	case len(fields) == 2 && fields[0] == "scenario":
		if err := m.scenario(fields[1], now); err != nil {
			return "", err
		}
		return "Scenario loaded: " + fields[1], nil
	case len(fields) == 1 && fields[0] == "close":
		m.State.GuiFocus = integer(0)
		m.State.Timestamp = now
		return "Simulated panel closed", nil
	case len(fields) == 1 && fields[0] == "fail":
		m.FailNext = true
		return "Next eligible simulated request will fail without changing actual state", nil
	case len(fields) == 1 && fields[0] == "show":
		return "Current simulated state", nil
	default:
		return "", fmt.Errorf("unknown command; type help")
	}
}

const mockHelp = `Commands (press Enter after each):
  t1 on/off/toggle    Landing gear
  t2 on/off/toggle    Cargo scoop
  t3 on/off/toggle    Ship lights
  t4 on/off/toggle    Hardpoints
  b1                 Night vision
  b2 / b3            Galaxy / system map (simulated GUI focus)
  b4                 Next of three simulated fire groups
  b5 / b6            Heat sink / chaff (simulated unconfirmed pulse)
  close              Close simulated map/panel
  external t1 off     Simulate a HOTAS/keyboard state change (also t2/t3/t4)
  scenario NAME      flight, docked, cruise, danger, srv, foot, unknown
  fail               Fail next eligible request; no retry
  show               Display the current state
  help               Show these commands
  q                  Exit
These mock commands never send keys to Elite Dangerous. Real-game control is a separate mode.`
