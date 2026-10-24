package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeSink struct {
	keys []string
	err  error
}

func (s *fakeSink) Pulse(ctx context.Context, key string, target gameTarget) error {
	s.keys = append(s.keys, key)
	return s.err
}
func testTarget() gameTarget {
	return gameTarget{Window: 10, PID: 20, Path: filepath.Join(os.TempDir(), "EliteDangerous64.exe")}
}
func testConfig() controlConfig {
	c := controlConfig{Version: 1, Preset: "EliteAuxPanel", Keys: make(map[string]string)}
	for _, a := range liveActions {
		c.Keys[a.ID] = a.DefaultKey
	}
	return c
}
func testEngine(t *testing.T) (*controlEngine, *fakeSink) {
	t.Helper()
	sink := &fakeSink{}
	c := &controlEngine{Config: testConfig(), Sink: sink, StaleAfter: time.Minute}
	s := newMock(testNow).State
	c.update(&s, nil, testNow, testTarget())
	return c, sink
}
func armEngine(t *testing.T, c *controlEngine) {
	t.Helper()
	if err := c.arm(testTarget(), testNow, nil); err != nil {
		t.Fatal(err)
	}
}

func TestControlStartsDisarmedAndCannotReplay(t *testing.T) {
	c, sink := testEngine(t)
	if err := c.trigger(context.Background(), "t1", testNow, testTarget(), nil); err == nil || len(sink.keys) != 0 {
		t.Fatal("disarmed input was sent")
	}
	armEngine(t, c)
	if len(sink.keys) != 0 {
		t.Fatal("arming replayed a prior input")
	}
}

func TestControlTelemetryConfirmationAndPending(t *testing.T) {
	c, sink := testEngine(t)
	armEngine(t, c)
	if err := c.trigger(context.Background(), "t1", testNow, testTarget(), nil); err != nil {
		t.Fatal(err)
	}
	if c.State.has(gear) || c.Pending == nil || len(sink.keys) != 1 {
		t.Fatal("delivery must not invent actual state")
	}
	if err := c.trigger(context.Background(), "t1", testNow.Add(time.Second), testTarget(), nil); err == nil || len(sink.keys) != 1 {
		t.Fatal("duplicate/pending input was sent")
	}
	s := newMock(testNow).State
	c.update(&s, nil, testNow.Add(time.Second), testTarget())
	if c.Pending == nil {
		t.Fatal("identical old snapshot acknowledged a request")
	}
	s = newMock(testNow).State
	s.Flags = u64(*s.Flags | gear)
	c.update(&s, nil, testNow.Add(2*time.Second), testTarget())
	if c.Pending != nil || !c.State.has(gear) || !strings.Contains(c.Last, "confirmed") {
		t.Fatal("changed snapshot in same game timestamp second should acknowledge")
	}
	if len(sink.keys) != 1 {
		t.Fatal("telemetry triggered corrective input")
	}
}

func TestControlTimeoutExternalChangeAndNoRetry(t *testing.T) {
	c, sink := testEngine(t)
	armEngine(t, c)
	if err := c.trigger(context.Background(), "t2", testNow, testTarget(), nil); err != nil {
		t.Fatal(err)
	}
	s := newMock(testNow).State
	c.update(&s, nil, testNow.Add(6*time.Second), testTarget())
	if c.Pending != nil || len(sink.keys) != 1 || !strings.Contains(c.Last, "no retry") {
		t.Fatal("timeout should clear request without repeat")
	}
	s = newMock(testNow.Add(7 * time.Second)).State
	s.Flags = u64(*s.Flags | scoop)
	c.update(&s, nil, testNow.Add(7*time.Second), testTarget())
	if !c.State.has(scoop) || len(sink.keys) != 1 {
		t.Fatal("late/external change should only update observed state")
	}
}

func TestControlFocusLossReadFailureAndNoRearm(t *testing.T) {
	for _, kind := range []string{"focus", "read", "stale", "backwards"} {
		t.Run(kind, func(t *testing.T) {
			c, sink := testEngine(t)
			armEngine(t, c)
			s := newMock(testNow).State
			target := testTarget()
			now := testNow
			var err error
			switch kind {
			case "focus":
				target = gameTarget{}
			case "read":
				err = errors.New("partial file")
			case "stale":
				now = now.Add(2 * time.Minute)
			case "backwards":
				s.Timestamp = s.Timestamp.Add(-time.Second)
			}
			c.update(&s, err, now, target)
			if c.Armed {
				t.Fatal("unsafe source/focus did not disarm")
			}
			s = newMock(testNow.Add(3 * time.Minute)).State
			c.update(&s, nil, testNow.Add(3*time.Minute), testTarget())
			if c.Armed || len(sink.keys) != 0 {
				t.Fatal("recovery must not rearm or send anything")
			}
		})
	}
}

func TestControlArmRequiresGameAndKnownContext(t *testing.T) {
	for _, kind := range []string{"game", "profile", "context", "missingFlags2", "map", "stale"} {
		t.Run(kind, func(t *testing.T) {
			c, _ := testEngine(t)
			target := testTarget()
			now := testNow
			var profileErr error
			switch kind {
			case "game":
				target.Path = "notepad.exe"
			case "profile":
				profileErr = errors.New("wrong preset")
			case "context":
				c.State.Flags = u64(0)
			case "missingFlags2":
				c.State.Flags2 = nil
			case "map":
				c.State.GuiFocus = integer(6)
			case "stale":
				now = now.Add(2 * time.Minute)
			}
			if err := c.arm(target, now, profileErr); err == nil || c.Armed {
				t.Fatal("invalid arm accepted")
			}
		})
	}
}

func TestControlFlightGuardsAndMapClose(t *testing.T) {
	c, sink := testEngine(t)
	c.State.Flags = u64(mainShip | docked | gear)
	armEngine(t, c)
	if err := c.trigger(context.Background(), "t1", testNow, testTarget(), nil); err == nil {
		t.Fatal("gear should be blocked docked")
	}
	if err := c.trigger(context.Background(), "t3", testNow, testTarget(), nil); err != nil {
		t.Fatal("lights should work docked", err)
	}
	if len(sink.keys) != 1 || sink.keys[0] != "F3" {
		t.Fatal("wrong light output")
	}
	c, _ = testEngine(t)
	armEngine(t, c)
	c.State.GuiFocus = integer(6)
	if err := c.trigger(context.Background(), "b1", testNow, testTarget(), nil); err == nil {
		t.Fatal("night vision should be blocked in a map")
	}
	if err := c.trigger(context.Background(), "b2", testNow, testTarget(), nil); err != nil {
		t.Fatal("same map button must close map", err)
	}
}

func TestControlDeliveryFailureDisarmsAndConsumablesUnconfirmed(t *testing.T) {
	c, sink := testEngine(t)
	armEngine(t, c)
	sink.err = errors.New("injection failed")
	if err := c.trigger(context.Background(), "t1", testNow, testTarget(), nil); err == nil || c.Armed || c.Pending != nil {
		t.Fatal("failed delivery must disarm without pending state")
	}
	c, sink = testEngine(t)
	armEngine(t, c)
	before := *c.State.Flags
	if err := c.trigger(context.Background(), "b5", testNow, testTarget(), nil); err != nil {
		t.Fatal(err)
	}
	if *c.State.Flags != before || c.Pending != nil || !strings.Contains(c.Last, "unconfirmed") || len(sink.keys) != 1 {
		t.Fatal("consumable confirmation was invented")
	}
	if err := c.trigger(context.Background(), "b5", testNow.Add(100*time.Millisecond), testTarget(), nil); err == nil || len(sink.keys) != 1 {
		t.Fatal("cooldown did not block a quick repeat")
	}
}

func TestPreparedProfileAndActivePresetCheck(t *testing.T) {
	data, err := os.ReadFile("EliteAuxPanel.4.2.binds")
	if err != nil {
		t.Fatal(err)
	}
	config := testConfig()
	if err := validateProfile(data, config); err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(string(data), "Key_F3", "Key_F4", 1)
	if err := validateProfile([]byte(bad), config); err == nil {
		t.Fatal("output key conflict was accepted")
	}
	bad = strings.Replace(string(data), "Key_F3", "Key_Numpad_3", 1)
	if err := validateProfile([]byte(bad), config); err == nil {
		t.Fatal("direct game numpad binding was accepted")
	}
	dir := t.TempDir()
	config.BindingsFile = filepath.Join(dir, "EliteAuxPanel.4.2.binds")
	if err := os.WriteFile(config.BindingsFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(dir, "StartPreset.4.start")
	if err := os.WriteFile(selected, []byte("General\r\nCustom\r\nSRV\r\nFoot\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := profileReady(config); err == nil {
		t.Fatal("wrong selected ship preset accepted")
	}
	if err := os.WriteFile(selected, []byte("General\r\nEliteAuxPanel\r\nSRV\r\nFoot\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := profileReady(config); err != nil {
		t.Fatal(err)
	}
}

type fakePulseAPI struct {
	target           gameTarget
	held             bool
	scans            []bool
	waitErr, scanErr error
}

func (f *fakePulseAPI) Foreground() gameTarget { return f.target }
func (f *fakePulseAPI) Held(vk uint16) bool    { return f.held }
func (f *fakePulseAPI) Scan(scan uint16, up bool) error {
	f.scans = append(f.scans, up)
	return f.scanErr
}
func (f *fakePulseAPI) Wait(ctx context.Context, d time.Duration, target gameTarget) error {
	return f.waitErr
}

func TestInputPulsePairsReleaseOnFocusLossAndCancellation(t *testing.T) {
	for _, waitErr := range []error{nil, errors.New("focus changed"), context.Canceled} {
		api := &fakePulseAPI{target: testTarget(), waitErr: waitErr}
		err := deliverPulse(context.Background(), "F3", testTarget(), api)
		if (err != nil) != (waitErr != nil) || len(api.scans) != 2 || api.scans[0] || !api.scans[1] {
			t.Fatal("app-owned key was not correctly paired", err, api.scans)
		}
	}
}

func TestInputPulseRejectsUnrelatedWindowAndHeldModifiers(t *testing.T) {
	for _, api := range []*fakePulseAPI{{target: gameTarget{}}, {target: testTarget(), held: true}} {
		if err := deliverPulse(context.Background(), "F1", testTarget(), api); err == nil || len(api.scans) != 0 {
			t.Fatal("input sent despite unsafe focus/key state")
		}
	}
	api := &fakePulseAPI{target: testTarget(), scanErr: errors.New("SendInput returned zero")}
	if err := deliverPulse(context.Background(), "F1", testTarget(), api); err == nil || len(api.scans) != 1 {
		t.Fatal("must not release keys when no app-owned down was accepted")
	}
}
