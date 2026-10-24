//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Only this test binary starts a child. The production app never starts one.
// The helper stays disarmed and does not call SendInput.
func TestLifecycleHelperProcess(t *testing.T) {
	mode := os.Getenv("ELITEAUX_LIFECYCLE_HELPER")
	if mode == "" {
		t.Skip("subprocess-only lifecycle helper")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := make(chan string)
	close(requests) // A closed UI request channel must not starve cancellation/polling.
	ready := false
	msg := safeBackend(func() tea.Msg {
		return backendDoneMsg{err: runControl(ctx, display{out: io.Discard, requests: requests, navigate: func(int) {}, publish: func(frame dashboardFrame) {
			if frame.ControlStatus != "DISARMED" {
				panic("lifecycle test armed unexpectedly")
			}
			if !ready && strings.HasPrefix(frame.PageStatus, "Global:") {
				ready = true
				fmt.Println("READY")
				switch mode {
				case "graceful":
					cancel()
				case "panic":
					panic("injected lifecycle failure")
				case "forced":
					select {} // Parent kills only this helper process.
				}
			}
		}}, os.Getenv("ELITEAUX_LIFECYCLE_STATUS"), "control.json", defaultTelemetryInterval, time.Minute, false)}
	})
	done := msg.(backendDoneMsg)
	if mode == "panic" {
		if done.err == nil || !strings.Contains(done.err.Error(), "injected lifecycle failure") {
			t.Fatal("panic was swallowed", done.err)
		}
	} else if done.err != nil {
		t.Fatal(done.err)
	}
	if !ready {
		t.Fatal("helper did not register page keys")
	}
}

func assertLifecycleShortcutsReleased(t *testing.T) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for i, key := range []struct{ mods, vk uintptr }{{0x0003, 0x78}, {0x0003, 0x79}, {0, 0x6b}, {0, 0x6d}} {
		id := uintptr(12000 + i)
		ok, _, err := controlUser32.NewProc("RegisterHotKey").Call(0, id, key.mods|0x4000, key.vk)
		if ok == 0 {
			t.Fatal("shortcut survived helper exit", key, err)
		}
		controlUser32.NewProc("UnregisterHotKey").Call(0, id)
	}
}

func TestWindowsMockPipeCancellationOwnsOnlyItsDuplicate(t *testing.T) {
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, err := prepareMockInput(ctx, input, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, reader); close(done) }()
	cancel()
	reader.cancel()
	reader.close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("duplicate input read did not unblock")
	}
	if _, err := input.Stat(); err != nil {
		t.Fatal("cancellation closed the caller's input handle", err)
	}
}

func TestWindowsControlCleanupAfterExitPanicAndForcedTermination(t *testing.T) {
	if os.Getenv("ELITEAUX_LIFECYCLE_TEST") != "1" {
		t.Skip("opt-in desktop lifecycle test; close the controller first")
	}
	path := filepath.Join(t.TempDir(), "Status.json")
	writeFixture(t, path, fmt.Sprintf(`{"timestamp":%q,"event":"Status","Flags":16777224,"Flags2":0,"GuiFocus":0}`, time.Now().UTC().Format(time.RFC3339Nano)))
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"graceful", "panic", "forced"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestLifecycleHelperProcess$", "-test.timeout=4s")
			command.Env = append(os.Environ(), "ELITEAUX_LIFECYCLE_HELPER="+mode, "ELITEAUX_LIFECYCLE_STATUS="+path)
			var stderr bytes.Buffer
			command.Stderr = &stderr
			stdout, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = command.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if command.ProcessState == nil {
					_ = command.Process.Kill()
					_ = command.Wait()
				}
			}()
			line, err := bufio.NewReader(stdout).ReadString('\n')
			if err != nil || strings.TrimSpace(line) != "READY" {
				t.Fatal("helper not ready", line, err, stderr.String())
			}
			if mode == "forced" {
				if err := command.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			}
			err = command.Wait()
			if mode != "forced" && err != nil {
				t.Fatal("helper exit failed", err, stderr.String())
			}
			if ctx.Err() != nil {
				t.Fatal("helper shutdown timed out")
			}
			assertLifecycleShortcutsReleased(t)
		})
	}
	t.Run("partial-registration-failure", func(t *testing.T) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		register := controlUser32.NewProc("RegisterHotKey")
		unregister := controlUser32.NewProc("UnregisterHotKey")
		ok, _, err := register.Call(0, 15001, 0x4003, 0x79)
		if ok == 0 {
			t.Fatal("cannot reserve stop shortcut for the failure test", err)
		}
		defer unregister.Call(0, 15001)
		if err := runControl(context.Background(), display{out: io.Discard}, path, "control.json", defaultTelemetryInterval, time.Minute, false); err == nil {
			t.Fatal("hotkey conflict was ignored")
		}
		ok, _, err = register.Call(0, 15002, 0x4003, 0x78)
		if ok == 0 {
			t.Fatal("partial startup leaked the arm shortcut", err)
		}
		unregister.Call(0, 15002)
	})
}
