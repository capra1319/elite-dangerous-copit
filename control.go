package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type actionDef struct {
	ID, Label, GameAction, DefaultKey string
	Mask                              uint64
}

var liveActions = []actionDef{
	{"t1", "Landing gear", "LandingGearToggle", "F1", gear},
	{"t2", "Cargo scoop", "ToggleCargoScoop", "F2", scoop},
	{"t3", "Ship lights", "ShipSpotLightToggle", "F3", lights},
	{"t4", "Hardpoints", "DeployHardpointToggle", "F4", hardpoints},
	{"b1", "Night vision", "NightVisionToggle", "F5", nightVision},
	{"b2", "Galaxy map", "GalaxyMapOpen", "F6", 0},
	{"b3", "System map", "SystemMapOpen", "F7", 0},
	{"b4", "Next fire group", "CycleFireGroupNext", "F8", 0},
	{"b5", "Heat sink", "DeployHeatSink", "F9", 0},
	{"b6", "Chaff", "FireChaffLauncher", "F10", 0},
}

type controlConfig struct {
	Version      int               `json:"version"`
	Preset       string            `json:"preset"`
	BindingsFile string            `json:"bindingsFile"`
	Keys         map[string]string `json:"keys"`
}

func loadControlConfig(path string) (controlConfig, error) {
	var c controlConfig
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("open control config: %w", err)
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 65536))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("invalid control config: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return c, errors.New("expected one config object")
	}
	if c.Version != 1 || c.Preset == "" || strings.ContainsAny(c.Preset, "/\\\r\n") {
		return c, errors.New("config needs version 1 and a plain preset name")
	}
	seen := make(map[string]bool)
	if len(c.Keys) != len(liveActions) {
		return c, errors.New("config must name all ten panel actions")
	}
	for _, a := range liveActions {
		key := strings.ToUpper(c.Keys[a.ID])
		if _, ok := functionKeyScan(key); !ok {
			return c, fmt.Errorf("%s needs a single function key F1-F12", a.ID)
		}
		if seen[key] {
			return c, fmt.Errorf("duplicate output key %s", key)
		}
		seen[key] = true
		c.Keys[a.ID] = key
	}
	return c, nil
}

func functionKeyScan(key string) (uint16, bool) {
	if !strings.HasPrefix(key, "F") {
		return 0, false
	}
	n, err := strconv.Atoi(key[1:])
	if err != nil {
		return 0, false
	}
	if n >= 1 && n <= 10 {
		return uint16(0x3a + n), true
	}
	if n == 11 {
		return 0x57, true
	}
	if n == 12 {
		return 0x58, true
	}
	return 0, false
}

type xmlBinding struct {
	Device    string       `xml:"Device,attr"`
	Key       string       `xml:"Key,attr"`
	Modifiers []xmlBinding `xml:"Modifier"`
}
type xmlAction struct {
	XMLName   xml.Name
	Primary   xmlBinding `xml:"Primary"`
	Secondary xmlBinding `xml:"Secondary"`
	Binding   xmlBinding `xml:"Binding"`
	Toggle    struct {
		Value string `xml:"Value,attr"`
	} `xml:"ToggleOn"`
}
type xmlProfile struct {
	XMLName xml.Name    `xml:"Root"`
	Preset  string      `xml:"PresetName,attr"`
	Actions []xmlAction `xml:",any"`
}

func validateProfile(data []byte, config controlConfig) error {
	var profile xmlProfile
	if err := xml.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("invalid game bindings: %w", err)
	}
	if profile.Preset != config.Preset {
		return errors.New("game preset name does not match config")
	}
	expected := make(map[string]string)
	for _, a := range liveActions {
		expected[a.GameAction] = "Key_" + config.Keys[a.ID]
	}
	found := make(map[string]bool)
	for _, a := range profile.Actions {
		name := a.XMLName.Local
		for _, b := range []xmlBinding{a.Primary, a.Secondary, a.Binding} {
			if b.Device != "Keyboard" {
				continue
			}
			if strings.Contains(strings.ToLower(b.Key), "numpad") || strings.Contains(strings.ToLower(b.Key), "num_") {
				return fmt.Errorf("%s uses a numpad key in-game; remove that binding to avoid two actions", name)
			}
			for gameAction, key := range expected {
				if b.Key == key {
					if name != gameAction || len(b.Modifiers) != 0 {
						return fmt.Errorf("%s conflicts with panel output %s", name, key)
					}
					if found[name] {
						return fmt.Errorf("duplicate keyboard binding for %s", name)
					}
					found[name] = true
				}
			}
			for _, modifier := range b.Modifiers {
				for _, key := range expected {
					if modifier.Key == key {
						return fmt.Errorf("panel key %s is used as a modifier", key)
					}
				}
			}
		}
		if name == "ToggleCargoScoop" && a.Toggle.Value != "1" {
			return errors.New("cargo scoop must use toggle mode")
		}
	}
	for _, a := range liveActions {
		if !found[a.GameAction] {
			return fmt.Errorf("missing %s keyboard binding (%s)", a.Label, config.Keys[a.ID])
		}
	}
	return nil
}

func bindingsDir() string {
	home, _ := os.UserHomeDir()
	candidates := []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Frontier Developments", "Elite Dangerous", "Options", "Bindings"), filepath.Join(home, "AppData", "Local", "Frontier Developments", "Elite Dangerous", "Options", "Bindings")}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return candidates[0]
}

func profileReady(c controlConfig) error {
	path := c.BindingsFile
	if path == "auto" || path == "" {
		path = filepath.Join(bindingsDir(), c.Preset+".4.2.binds")
	}
	data, err := readSmallFile(path, 4<<20)
	if err != nil {
		return fmt.Errorf("install %s.4.2.binds first: %w", c.Preset, err)
	}
	if err := validateProfile(data, c); err != nil {
		return err
	}
	selected, err := readSmallFile(filepath.Join(filepath.Dir(path), "StartPreset.4.start"), 64<<10)
	if err != nil {
		return errors.New("cannot read active ship preset; choose EliteAuxPanel in Ship Controls")
	}
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(strings.TrimPrefix(string(selected), "\ufeff")), "\r", ""), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[1]) != c.Preset {
		return fmt.Errorf("select %s under Options > Controls > Ship Controls", c.Preset)
	}
	return nil
}

type gameTarget struct {
	Window uintptr
	PID    uint32
	Path   string
}

func (t gameTarget) valid() bool {
	name := strings.ToLower(filepath.Base(t.Path))
	return t.Window != 0 && t.PID != 0 && (name == "elitedangerous64.exe" || name == "elitedangerous.exe")
}

type pulseSink interface {
	Pulse(context.Context, string, gameTarget) error
}
type pendingRequest struct {
	Action   actionDef
	Desired  bool
	Baseline uint64
	Started  time.Time
}

type controlEngine struct {
	Config     controlConfig
	Sink       pulseSink
	State      *Snapshot
	ReadError  error
	Sample     uint64
	Armed      bool
	Target     gameTarget
	Pending    *pendingRequest
	Last       string
	lastSent   time.Time
	StaleAfter time.Duration
}

func (c *controlEngine) disarm(reason string) {
	c.Armed = false
	c.Pending = nil
	c.Target = gameTarget{}
	c.Last = "DISARMED: " + reason
}

func (c *controlEngine) update(s *Snapshot, err error, now time.Time, target gameTarget) {
	c.ReadError = err
	if err == nil && s != nil {
		if c.State != nil && s.Timestamp.Before(c.State.Timestamp) {
			c.ReadError = errors.New("telemetry timestamp moved backwards")
			c.disarm("telemetry changed session; wait for a current snapshot")
			return
		}
		if c.State == nil || !sameControlSample(*c.State, *s) {
			c.Sample++
		}
		c.State = cloneSnapshot(s)
	}
	if c.Armed {
		if target != c.Target {
			c.disarm("game lost focus or restarted")
			return
		}
		if err := c.telemetryUsable(now); err != nil {
			c.disarm(err.Error())
			return
		}
	}
	if p := c.Pending; p != nil {
		if c.Sample > p.Baseline && c.State != nil && c.State.has(p.Action.Mask) == p.Desired {
			c.Last = p.Action.Label + ": requested state confirmed by game telemetry"
			c.Pending = nil
		} else if now.Sub(p.Started) >= 5*time.Second {
			c.Last = p.Action.Label + ": no matching state observed; no retry"
			c.Pending = nil
		}
	}
}

func sameControlSample(a, b Snapshot) bool {
	return a.Timestamp.Equal(b.Timestamp) && derefU64(a.Flags) == derefU64(b.Flags) && derefU64(a.Flags2) == derefU64(b.Flags2) && derefInt(a.GuiFocus) == derefInt(b.GuiFocus) && derefInt(a.FireGroup) == derefInt(b.FireGroup)
}

func (c *controlEngine) telemetryUsable(now time.Time) error {
	if c.ReadError != nil {
		return errors.New("telemetry is unreadable")
	}
	if c.State == nil || !c.State.shipFeedbackKnown() || c.State.Flags2 == nil || c.State.GuiFocus == nil {
		return errors.New("main-ship telemetry is not available")
	}
	limit := c.StaleAfter
	if limit <= 0 {
		limit = time.Minute
	}
	if age := now.Sub(c.State.Timestamp); age > limit || age < -futureSnapshotTolerance {
		return errors.New("telemetry is stale or the clock differs")
	}
	if c.State.has(jumping) || c.State.has2(1<<12) {
		return errors.New("ship is in a transition")
	}
	return nil
}

func (c *controlEngine) arm(target gameTarget, now time.Time, profileErr error) error {
	if !target.valid() {
		return errors.New("focus the Elite Dangerous game, then press Ctrl+Alt+F9")
	}
	if profileErr != nil {
		return profileErr
	}
	if err := c.telemetryUsable(now); err != nil {
		return err
	}
	if *c.State.GuiFocus != 0 {
		return errors.New("close ship panels/maps and chat before arming")
	}
	c.Target = target
	c.Armed = true
	c.Pending = nil
	c.Last = "ARMED: numpad controls operate Elite Dangerous while this game stays focused"
	return nil
}

func (c *controlEngine) trigger(ctx context.Context, id string, now time.Time, target gameTarget, profileErr error) error {
	if err := ctx.Err(); err != nil {
		c.disarm("app closing")
		return err
	}
	if !c.Armed {
		return errors.New("disarmed; focus the cockpit and press Ctrl+Alt+F9")
	}
	if target != c.Target || !target.valid() {
		c.disarm("game is not foreground")
		return errors.New(c.Last)
	}
	if profileErr != nil {
		c.disarm(profileErr.Error())
		return profileErr
	}
	if err := c.telemetryUsable(now); err != nil {
		c.disarm(err.Error())
		return err
	}
	if c.Pending != nil {
		return errors.New("waiting for game telemetry; request was not queued")
	}
	if now.Sub(c.lastSent) < 300*time.Millisecond {
		return errors.New("input cooldown; press again deliberately")
	}
	var a *actionDef
	for i := range liveActions {
		if liveActions[i].ID == id {
			a = &liveActions[i]
			break
		}
	}
	if a == nil {
		return errors.New("unknown panel action")
	}
	focus := *c.State.GuiFocus
	if focus != 0 && !(id == "b2" && focus == 6) && !(id == "b3" && focus == 7) {
		return errors.New("close the current game panel or chat first")
	}
	if (id == "t1" || id == "t2" || id == "t4" || id == "b5" || id == "b6") && c.State.has(docked|landed|supercruise|charging|jumping) {
		return errors.New("action is blocked in this flight context")
	}
	key := c.Config.Keys[id]
	if _, ok := functionKeyScan(key); !ok {
		return errors.New("output binding is not configured")
	}
	var pending *pendingRequest
	if a.Mask != 0 {
		pending = &pendingRequest{Action: *a, Desired: !c.State.has(a.Mask), Baseline: c.Sample, Started: now}
	}
	if err := c.Sink.Pulse(ctx, key, c.Target); err != nil {
		c.disarm("keyboard delivery failed: " + err.Error())
		return err
	}
	c.lastSent = now
	c.Pending = pending
	if pending != nil {
		c.Last = a.Label + ": sent; waiting for actual game state"
	} else {
		c.Last = a.Label + ": key pulse sent; activation unconfirmed"
	}
	return nil
}
