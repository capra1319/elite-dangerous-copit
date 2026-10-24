//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// INPUT_RECORD contains a 16-byte event union after the type and alignment.
// The fields below expose KEY_EVENT_RECORD; only read them for KEY_EVENT (1).
type inputRecord struct {
	EventType       uint16
	Padding         uint16
	KeyDown         int32
	RepeatCount     uint16
	VirtualKey      uint16
	ScanCode        uint16
	Char            uint16
	ControlKeyState uint32
}

func runNumpad(ctx context.Context, input *os.File, d display) error {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel.NewProc("GetConsoleMode")
	setMode := kernel.NewProc("SetConsoleMode")
	getCount := kernel.NewProc("GetNumberOfConsoleInputEvents")
	read := kernel.NewProc("ReadConsoleInputW")
	h := input.Fd()
	var original uint32
	ok, _, _ := getMode.Call(h, uintptr(unsafe.Pointer(&original)))
	if ok == 0 {
		return fmt.Errorf("numpad input requires an interactive Windows console; open Start-Mock-Numpad.cmd, or use -controls commands")
	}
	// Disable line input, echo, quick edit and VT input; preserve Ctrl+C handling.
	mode := (original | 0x0080 | 0x0001) &^ uint32(0x0002|0x0004|0x0040|0x0200)
	ok, _, err := setMode.Call(h, uintptr(mode))
	if ok == 0 {
		return fmt.Errorf("cannot enable numpad console input: %v", err)
	}
	defer setMode.Call(h, uintptr(original))
	m := newMock(time.Now())
	d.show(&m.State, "mock", "", "SIMULATED -- no game or hardware needed", "Ready for numpad presses", time.Now())
	var latch keyLatch
	tick := time.NewTicker(15 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(d.out, "\nStopped.")
			return nil
		case <-tick.C:
		}
		var count uint32
		ok, _, err = getCount.Call(h, uintptr(unsafe.Pointer(&count)))
		if ok == 0 {
			return fmt.Errorf("cannot read numpad event count: %v", err)
		}
		if count == 0 {
			continue
		}
		var records [32]inputRecord
		if count > uint32(len(records)) {
			count = uint32(len(records))
		}
		var got uint32
		ok, _, err = read.Call(h, uintptr(unsafe.Pointer(&records[0])), uintptr(count), uintptr(unsafe.Pointer(&got)))
		if ok == 0 {
			return fmt.Errorf("cannot read numpad events: %v", err)
		}
		for i := uint32(0); i < got; i++ {
			record := records[i]
			if record.EventType == 0x0010 { // FOCUS_EVENT: don't keep stale held keys.
				latch = keyLatch{}
				continue
			}
			if record.EventType != 1 {
				continue
			}
			command := latch.command(consoleKey{Down: record.KeyDown != 0, VirtualKey: record.VirtualKey, Controls: record.ControlKeyState})
			if command == "" {
				continue
			}
			if command == "quit" {
				fmt.Fprintln(d.out, "Goodbye.")
				return nil
			}
			if command == "help" {
				fmt.Fprintln(d.out, numpadHelp)
				d.invalidateScreen()
				continue
			}
			action, commandErr := mockCommand(m, strings.Fields(command), time.Now())
			if commandErr != nil {
				action = "Blocked: " + commandErr.Error()
			}
			d.show(&m.State, "mock", "", "SIMULATED -- no game or hardware needed", action, time.Now())
		}
	}
}
