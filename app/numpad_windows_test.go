//go:build windows

package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsInputRecordLayout(t *testing.T) {
	var record inputRecord
	if unsafe.Sizeof(record) != 20 || unsafe.Offsetof(record.KeyDown) != 4 || unsafe.Offsetof(record.ControlKeyState) != 16 {
		t.Fatal("INPUT_RECORD layout does not match Win32")
	}
}

type captureConsole struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (c *captureConsole) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buffer.Write(p)
}
func (c *captureConsole) text() string { c.mu.Lock(); defer c.mu.Unlock(); return c.buffer.String() }

// Opt-in only: run in a dedicated test console, never the user's normal terminal.
// WriteConsoleInput puts records in this console's queue; no global keys are sent.
func TestWindowsNumpadConsole(t *testing.T) {
	if os.Getenv("ELITEAUX_CONSOLE_TEST") != "1" {
		t.Skip("requires opt-in dedicated Windows test console")
	}
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	kernel := syscall.NewLazyDLL("kernel32.dll")
	var initial uint32
	getMode := kernel.NewProc("GetConsoleMode")
	ok, _, err := getMode.Call(input.Fd(), uintptr(unsafe.Pointer(&initial)))
	if ok == 0 {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var capture captureConsole
	finished := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		finished <- runNumpad(ctx, input, display{out: &capture, controls: "numpad"})
	}()
	defer func() { cancel(); <-done }()
	wait := func(text string, count int) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Count(capture.text(), text) >= count {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("console did not report %q (%d times): %s", text, count, capture.text())
	}
	write := func(records ...inputRecord) {
		t.Helper()
		var written uint32
		ok, _, err := kernel.NewProc("WriteConsoleInputW").Call(input.Fd(), uintptr(unsafe.Pointer(&records[0])), uintptr(len(records)), uintptr(unsafe.Pointer(&written)))
		if ok == 0 || written != uint32(len(records)) {
			t.Fatalf("WriteConsoleInput: %d records: %v", written, err)
		}
	}
	key := func(vk uint16, down bool) inputRecord {
		record := inputRecord{EventType: 1, RepeatCount: 1, VirtualKey: vk, ControlKeyState: 0x20}
		if down {
			record.KeyDown = 1
		}
		return record
	}
	wait("Ready for numpad presses", 1)
	write(key(0x61, true), key(0x61, true))
	wait("Last action: Landing gear:", 1)
	time.Sleep(60 * time.Millisecond)
	if count := strings.Count(capture.text(), "Last action: Landing gear:"); count != 1 {
		t.Fatalf("held key triggered %d actions", count)
	}
	write(key(0x61, false), key(0x61, true), key(0x61, false))
	wait("Last action: Landing gear:", 2)
	write(key(0x65, true), key(0x65, false))
	wait("Last action: Night vision:", 1)
	write(key(0x1b, true))
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Escape did not exit")
	}
	var restored uint32
	ok, _, err = getMode.Call(input.Fd(), uintptr(unsafe.Pointer(&restored)))
	if ok == 0 || initial != restored {
		t.Fatalf("console mode not restored: %x -> %x: %v", initial, restored, err)
	}
}
