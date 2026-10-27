//go:build windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// Sends WM_HOTKEY only to this test's backend thread; no keyboard input is
// injected into Windows or Elite. Requires the controller to be closed.
func TestControlGlobalNumpadPageKeys(t *testing.T) {
	if os.Getenv("ELITEAUX_PAGE_KEYS_TEST") != "1" {
		t.Skip("requires opt-in desktop page-hotkey session; stays disarmed")
	}
	path := filepath.Join(t.TempDir(), "Status.json")
	writeFixture(t, path, fmt.Sprintf(`{"timestamp":%q,"event":"Status","Flags":16777224,"Flags2":0,"GuiFocus":0}`, time.Now().UTC().Format(time.RFC3339Nano)))
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan uint32, 1)
	deltas := make(chan int, 4)
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- runControl(ctx, display{out: io.Discard, controls: "numpad", navigate: func(delta int) { deltas <- delta }, publish: func(frame dashboardFrame) {
			if frame.ControlStatus != "DISARMED" {
				return
			}
			if strings.HasPrefix(frame.PageStatus, "Global:") {
				thread, _, _ := controlKernel32.NewProc("GetCurrentThreadId").Call()
				select {
				case ready <- uint32(thread):
				default:
				}
			}
		}}, path, "control.json", defaultTelemetryInterval, time.Minute, false)
	}()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("backend did not release shortcuts")
		}
	}()
	var thread uint32
	select {
	case thread = <-ready:
	case err := <-done:
		t.Fatalf("backend stopped: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("global page keys were not registered")
	}
	post := controlUser32.NewProc("PostThreadMessageW")
	for _, item := range []struct{ id, delta int }{{previousPageHotkey, -1}, {nextPageHotkey, 1}} {
		ok, _, err := post.Call(uintptr(thread), 0x0312, uintptr(item.id), 0)
		if ok == 0 {
			t.Fatal("could not post page message", err)
		}
		select {
		case delta := <-deltas:
			if delta != item.delta {
				t.Fatal("wrong page direction")
			}
		case <-time.After(time.Second):
			t.Fatal("page message did not reach UI callback")
		}
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("backend did not stop")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for i, vk := range []uintptr{0x6b, 0x6d} {
		id := uintptr(9000 + i)
		ok, _, err := controlUser32.NewProc("RegisterHotKey").Call(0, id, 0x4000, vk)
		if ok == 0 {
			t.Fatal("page shortcut was not released", err)
		}
		controlUser32.NewProc("UnregisterHotKey").Call(0, id)
	}
}

// This uses real global arm/stop shortcuts, but never arms or sends input.
// Opt in in a desktop session with no other controller instance running.
func TestControlTelemetryPublishLatency(t *testing.T) {
	if os.Getenv("ELITEAUX_CONTROL_TIMING_TEST") != "1" {
		t.Skip("requires opt-in desktop control session; stays disarmed")
	}
	path := filepath.Join(t.TempDir(), "Status.json")
	writeStatus := func(flags uint64) {
		t.Helper()
		data := fmt.Sprintf(`{"timestamp":%q,"event":"Status","Flags":%d,"Flags2":0,"GuiFocus":0}`, time.Now().UTC().Format(time.RFC3339Nano), flags)
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeStatus(mainShip | shields)
	ctx, cancel := context.WithCancel(context.Background())
	frames := make(chan dashboardFrame, 16)
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- runControl(ctx, display{out: io.Discard, controls: "numpad", publish: func(frame dashboardFrame) {
			select {
			case frames <- frame:
			case <-ctx.Done():
			}
		}}, path, "control.json", defaultTelemetryInterval, time.Minute, false)
	}()
	defer func() {
		cancel()
		select {
		case <-finished:
		case <-time.After(2 * time.Second):
			t.Error("control backend did not release its hotkeys")
		}
	}()
	select {
	case <-frames:
	case err := <-done:
		t.Fatalf("backend stopped: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("no initial frame")
	}
	start := time.Now()
	writeStatus(mainShip | shields | gear)
	deadline := time.NewTimer(750 * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case frame := <-frames:
			if frame.ControlStatus != "DISARMED" {
				t.Fatal("timing check must remain disarmed")
			}
			if frame.State != nil && frame.State.has(gear) {
				t.Logf("changed status published in %s", time.Since(start).Round(time.Millisecond))
				return
			}
		case err := <-done:
			t.Fatalf("backend stopped: %v", err)
		case <-deadline.C:
			t.Fatal("changed telemetry waited for the old one-second redraw")
		}
	}
}

func TestWindowsSendInputLayout(t *testing.T) {
	var input nativeInput
	var keyboard keyboardInput
	var msg windowMessage
	if unsafe.Sizeof(input) != 40 || unsafe.Offsetof(input.Union) != 8 || unsafe.Sizeof(keyboard) != 24 || unsafe.Offsetof(keyboard.Extra) != 16 || unsafe.Sizeof(msg) != 48 || unsafe.Offsetof(msg.WParam) != 16 {
		t.Fatal("native input/message layout does not match Windows amd64")
	}
}

func TestControlPreviewDoesNotArmOrModifyTelemetry(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Status.json")
	data := []byte(`{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":16777224,"Flags2":0,"GuiFocus":0}`)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	// No mode/input flags: opening the app must select the real backend.
	if code := run([]string{"-once", "-plain", "-status", p, "-config", "control.json"}, strings.NewReader(""), &out, &errs); code != 0 {
		t.Fatal(code, errs.String())
	}
	if !strings.Contains(out.String(), "REAL GAME CONTROL") || !strings.Contains(out.String(), "CONTROL: DISARMED") || !strings.Contains(out.String(), "Num1 Gear") {
		t.Fatal("control preview missing mode/shortcuts")
	}
	if strings.Contains(out.String(), "SIMULAT") || strings.Contains(out.String(), "mock>") {
		t.Fatal("default startup exposed simulated state")
	}
	after, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("preview changed telemetry")
	}
}
