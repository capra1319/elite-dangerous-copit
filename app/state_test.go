package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMissingStatusDoesNotWaitThroughRetries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := readWithRetry(ctx, filepath.Join(t.TempDir(), "missing.json"))
	if !os.IsNotExist(err) {
		t.Fatalf("missing file should return immediately for the next poll, got %v", err)
	}
}

var testNow = time.Date(2026, 10, 23, 7, 0, 0, 0, time.UTC)

func TestDecodeUnsignedAndOptionalFields(t *testing.T) {
	s, err := decode([]byte(`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":2147483652,"FutureField":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if !s.has(gear) || !s.has(1<<31) {
		t.Fatal("unsigned high bit or gear lost")
	}
	if s.Flags2 != nil || s.GuiFocus != nil {
		t.Fatal("missing optional fields must remain unknown")
	}
}

func TestRejectInvalidSnapshots(t *testing.T) {
	for _, data := range []string{
		``, `{`, `null`,
		`{"timestamp":"2026-10-23T07:00:00Z","event":"Status"}`,
		`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":null}`,
		`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":-1}`,
		`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":1.5}`,
		`{"event":"Status","Flags":0}`,
		`{"timestamp":"bad","event":"Status","Flags":0}`,
		`{"timestamp":"2026-10-23T07:00:00Z","event":"LoadGame","Flags":0}`,
		`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":0} {}`,
	} {
		if _, err := decode([]byte(data)); err == nil {
			t.Errorf("accepted invalid snapshot: %q", data)
		}
	}
}

func TestMockSwitchAndExternalFeedback(t *testing.T) {
	m := newMock(testNow)
	if _, err := m.setSwitch("t1", "on", false, testNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !m.State.has(gear) {
		t.Fatal("gear should be on")
	}
	msg, err := m.setSwitch("t1", "on", false, testNow.Add(2*time.Second))
	if err != nil || !strings.Contains(msg, "no simulated pulse") {
		t.Fatal("already satisfied must not toggle")
	}
	if _, err := m.setSwitch("t1", "off", true, testNow.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if m.State.has(gear) {
		t.Fatal("external change must become actual state")
	}
}

func TestFailedRequestLeavesActualStateUnchanged(t *testing.T) {
	m := newMock(testNow)
	m.FailNext = true
	msg, err := m.setSwitch("t2", "on", false, testNow)
	if err != nil || !strings.Contains(msg, "failed") || m.State.has(scoop) || m.FailNext {
		t.Fatal("failure must not change state or retry")
	}
	if m.State.Timestamp != testNow {
		t.Fatal("failure must not invent new telemetry")
	}
}

func TestContextGuardsAndLightsException(t *testing.T) {
	m := newMock(testNow)
	if err := m.scenario("docked", testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := m.setSwitch("t4", "on", false, testNow); err == nil {
		t.Fatal("hardpoints should be blocked docked")
	}
	if _, err := m.button("b5", testNow); err == nil {
		t.Fatal("consumables should be blocked docked")
	}
	if _, err := m.setSwitch("t3", "on", false, testNow); err != nil {
		t.Fatal("lights should work docked", err)
	}
	if err := m.scenario("foot", testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := m.button("b1", testNow); err == nil {
		t.Fatal("ship buttons should be blocked on foot")
	}
	if m.State.shipFeedbackKnown() {
		t.Fatal("on foot must not show known ship lamps")
	}
}

func TestMapAndConsumableSemantics(t *testing.T) {
	m := newMock(testNow)
	if _, err := m.button("b2", testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := m.setSwitch("t1", "on", false, testNow); err == nil {
		t.Fatal("map must block ship switch")
	}
	if _, err := mockCommand(m, []string{"close"}, testNow); err != nil {
		t.Fatal(err)
	}
	before := *m.State.Flags
	msg, err := m.button("b6", testNow)
	if err != nil || !strings.Contains(msg, "unconfirmed") || *m.State.Flags != before {
		t.Fatal("consumable must not invent confirmation")
	}
}

func TestUnknownStateAndInvertedFlightAssist(t *testing.T) {
	s, err := decode([]byte(`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":0}`))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	display{out: &out}.show(&s, "live", "test", "STALE", "", testNow)
	if strings.Count(out.String(), "L1 [?]") != 1 || strings.Contains(out.String(), "Flight assist: ON") {
		t.Fatal("empty context must show unknown")
	}
	out.Reset()
	s.Flags = u64(mainShip | assistOff)
	display{out: &out}.show(&s, "live", "test", "RECENT", "", testNow)
	if !strings.Contains(out.String(), "Flight assist: OFF") {
		t.Fatal("assist-off bit must invert display")
	}
}

func TestQualitySeparatesReadTimeFromSnapshotAge(t *testing.T) {
	s := newMock(testNow).State
	if !strings.HasPrefix(qualityFor(&s, nil, testNow.Add(2*time.Minute), time.Minute), "STALE") {
		t.Fatal("rereading old data must not make it recent")
	}
	if !strings.HasPrefix(qualityFor(&s, os.ErrNotExist, testNow, time.Minute), "READ ERROR -- showing LAST KNOWN") {
		t.Fatal("read failure should label retained state")
	}
	if !strings.HasPrefix(qualityFor(&s, nil, testNow.Add(-time.Minute), time.Minute), "CLOCK MISMATCH") {
		t.Fatal("future data should be flagged")
	}
}

func TestFileReplacementPartialAndRecovery(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Status.json")
	write := func(data string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":16777224}`)
	if _, err := readStatus(p); err != nil {
		t.Fatal(err)
	}
	write(`{"timestamp":`)
	if _, err := readStatus(p); err == nil {
		t.Fatal("partial file must be rejected")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	write(`{"timestamp":"2026-10-23T07:00:01Z","event":"Status","Flags":16777228}`)
	s, err := readStatus(p)
	if err != nil || !s.has(gear) {
		t.Fatal("replacement should recover", err)
	}
	write(strings.Repeat(" ", 256*1024+1))
	if _, err := readStatus(p); err == nil {
		t.Fatal("oversized file must be rejected")
	}
}

func TestCLIInteractiveMock(t *testing.T) {
	var out, errs bytes.Buffer
	input := strings.NewReader("t1 on\nexternal t1 off\nfail\nt2 on\nq\n")
	if code := run([]string{"-mode", "mock", "-plain"}, input, &out, &errs); code != 0 {
		t.Fatal(code, errs.String())
	}
	for _, want := range []string{"MOCK SIMULATOR", "matching mock state observed", "external/HOTAS", "request failed", "Goodbye"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestCLIReadOnlyAndInvalidArguments(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Status.json")
	data := []byte(`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":16777228}`)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := run([]string{"-mode", "live", "-status", p, "-once", "-plain"}, strings.NewReader("t1 off\n"), &out, &errs); code != 0 {
		t.Fatal(code, errs.String())
	}
	after, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("live mode must not modify status")
	}
	if !strings.Contains(out.String(), "READ ONLY") || !strings.Contains(out.String(), "L1 [*]") {
		t.Fatal("live output missing state")
	}
	for _, args := range [][]string{{"-mode", "control", "-controls", "commands"}, {"-interval", "1ms"}, {"-stale-after", "0s"}, {"surprise"}} {
		if code := run(args, strings.NewReader(""), &out, &errs); code != 2 {
			t.Fatalf("invalid args accepted: %v", args)
		}
	}
}
