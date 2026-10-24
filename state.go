package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Display, scanner and guarded input share the same small clock-skew allowance.
const futureSnapshotTolerance = 5 * time.Second

const (
	docked      uint64 = 1 << 0
	landed      uint64 = 1 << 1
	gear        uint64 = 1 << 2
	shields     uint64 = 1 << 3
	supercruise uint64 = 1 << 4
	assistOff   uint64 = 1 << 5
	hardpoints  uint64 = 1 << 6
	lights      uint64 = 1 << 8
	scoop       uint64 = 1 << 9
	silent      uint64 = 1 << 10
	massLocked  uint64 = 1 << 16
	charging    uint64 = 1 << 17
	cooldown    uint64 = 1 << 18
	lowFuel     uint64 = 1 << 19
	overheating uint64 = 1 << 20
	mainShip    uint64 = 1 << 24
	fighter     uint64 = 1 << 25
	srv         uint64 = 1 << 26
	nightVision uint64 = 1 << 28
	jumping     uint64 = 1 << 30
)

type Snapshot struct {
	Timestamp  time.Time  `json:"timestamp"`
	Event      string     `json:"event"`
	Flags      *uint64    `json:"Flags"`
	Flags2     *uint64    `json:"Flags2"`
	GuiFocus   *int       `json:"GuiFocus"`
	FireGroup  *int       `json:"FireGroup"`
	Pips       []int      `json:"Pips"`
	Fuel       *fuelState `json:"Fuel"`
	Cargo      *float64   `json:"Cargo"`
	LegalState string     `json:"LegalState"`
}

type fuelState struct {
	Main      float64 `json:"FuelMain"`
	Reservoir float64 `json:"FuelReservoir"`
}

func u64(v uint64) *uint64               { return &v }
func integer(v int) *int                 { return &v }
func (s Snapshot) has(mask uint64) bool  { return s.Flags != nil && *s.Flags&mask != 0 }
func (s Snapshot) has2(mask uint64) bool { return s.Flags2 != nil && *s.Flags2&mask != 0 }

func decode(data []byte) (Snapshot, error) {
	var s Snapshot
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&s); err != nil {
		return s, fmt.Errorf("invalid/incomplete JSON: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return s, errors.New("expected one JSON snapshot")
	}
	if s.Event != "Status" {
		return s, errors.New("not a Status event")
	}
	if s.Timestamp.IsZero() {
		return s, errors.New("missing timestamp")
	}
	if s.Flags == nil {
		return s, errors.New("missing Flags")
	}
	return s, nil
}

// Open afresh on every poll: Elite may replace rather than append to the file.
// A bounded read also protects the terminal from a wrongly selected large file.
func readStatus(path string) (Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer f.Close()
	const limit = 256 * 1024
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return Snapshot{}, err
	}
	if len(data) > limit {
		return Snapshot{}, errors.New("status file exceeds 256 KiB")
	}
	return decode(data)
}

func (s Snapshot) context() string {
	switch {
	case s.has2(1 << 0):
		return "ON FOOT"
	case s.has2(1 << 1):
		return "TAXI"
	case s.has2(1 << 2):
		return "MULTICREW"
	case s.has(srv):
		return "SRV"
	case s.has(fighter):
		return "FIGHTER"
	case !s.has(mainShip):
		return "UNKNOWN / NO SHIP SESSION"
	case s.has(jumping):
		return "MAIN SHIP / JUMPING"
	case s.has(supercruise):
		return "MAIN SHIP / SUPERCRUISE"
	case s.has(docked):
		return "MAIN SHIP / DOCKED"
	case s.has(landed):
		return "MAIN SHIP / LANDED"
	default:
		return "MAIN SHIP / NORMAL SPACE"
	}
}

func (s Snapshot) shipFeedbackKnown() bool {
	return s.Flags != nil && s.has(mainShip) && !s.has(srv|fighter) && !s.has2(7)
}

type Switch struct {
	ID, Name string
	Mask     uint64
}

var switches = []Switch{
	{"t1", "Landing gear", gear},
	{"t2", "Cargo scoop", scoop},
	{"t3", "Ship lights", lights},
	{"t4", "Hardpoints", hardpoints},
}

type Mock struct {
	State    Snapshot
	FailNext bool
}

func newMock(now time.Time) *Mock {
	return &Mock{State: Snapshot{Timestamp: now, Event: "Status", Flags: u64(mainShip | shields), Flags2: u64(0), GuiFocus: integer(0), FireGroup: integer(0), Pips: []int{4, 4, 4}}}
}

func (m *Mock) scenario(name string, now time.Time) error {
	flags, flags2 := mainShip|shields, uint64(0)
	switch name {
	case "flight":
	case "docked":
		flags |= docked | gear | massLocked
	case "cruise":
		flags |= supercruise
	case "danger":
		flags |= lowFuel | overheating
	case "srv":
		flags, flags2 = srv, 0
	case "foot":
		flags, flags2 = 0, 1
	case "unknown":
		flags = 0
	default:
		return errors.New("scenarios: flight, docked, cruise, danger, srv, foot, unknown")
	}
	m.State = Snapshot{Timestamp: now, Event: "Status", Flags: u64(flags), Flags2: u64(flags2), GuiFocus: integer(0), FireGroup: integer(0), Pips: []int{4, 4, 4}}
	m.FailNext = false
	return nil
}

func (m *Mock) setSwitch(id, desired string, external bool, now time.Time) (string, error) {
	var target *Switch
	for i := range switches {
		if switches[i].ID == id {
			target = &switches[i]
			break
		}
	}
	if target == nil {
		return "", errors.New("switch must be t1, t2, t3 or t4")
	}
	if desired != "on" && desired != "off" && desired != "toggle" {
		return "", errors.New("use on, off or toggle")
	}
	if !m.State.shipFeedbackKnown() {
		return "", errors.New("no main-ship state; choose scenario flight")
	}
	if !external {
		if m.State.GuiFocus == nil || *m.State.GuiFocus != 0 {
			return "", errors.New("a simulated panel is open; type close")
		}
		if target.Mask != lights && m.State.has(docked|landed|supercruise|charging|jumping) {
			return "", errors.New("this switch is blocked in the simulated flight context")
		}
	}
	actual := m.State.has(target.Mask)
	want := desired == "on" || (desired == "toggle" && !actual)
	if want == actual {
		return target.Name + ": already satisfied; no simulated pulse", nil
	}
	if !external && m.FailNext {
		m.FailNext = false
		return target.Name + ": simulated request failed; actual state unchanged; no retry", nil
	}
	if want {
		*m.State.Flags |= target.Mask
	} else {
		*m.State.Flags &^= target.Mask
	}
	m.State.Timestamp = now
	if external {
		return target.Name + ": external/HOTAS change simulated; LEDs follow telemetry", nil
	}
	return target.Name + ": one simulated pulse; matching mock state observed", nil
}

func (m *Mock) button(id string, now time.Time) (string, error) {
	if id != "b1" && id != "b2" && id != "b3" && id != "b4" && id != "b5" && id != "b6" {
		return "", errors.New("button must be b1 to b6")
	}
	if !m.State.shipFeedbackKnown() {
		return "", errors.New("no main-ship state; choose scenario flight")
	}
	if m.State.GuiFocus == nil || *m.State.GuiFocus != 0 {
		return "", errors.New("a simulated panel is open; type close")
	}
	if (id == "b5" || id == "b6") && m.State.has(docked|landed|supercruise|charging|jumping) {
		return "", errors.New("consumable blocked outside normal flight")
	}
	if m.FailNext {
		m.FailNext = false
		return "Simulated request failed; no retry", nil
	}
	m.State.Timestamp = now
	switch id {
	case "b1":
		*m.State.Flags ^= nightVision
		return "Night vision: simulated toggle", nil
	case "b2":
		m.State.GuiFocus = integer(6)
		return "Galaxy map opened in simulator; type close to return", nil
	case "b3":
		m.State.GuiFocus = integer(7)
		return "System map opened in simulator; type close to return", nil
	case "b4":
		m.State.FireGroup = integer((*m.State.FireGroup + 1) % 3)
		return "Next fire group: simulator cycles through three groups", nil
	case "b5":
		return "Heat sink: simulated pulse only; activation is unconfirmed", nil
	default:
		return "Chaff: simulated pulse only; activation is unconfirmed", nil
	}
}
