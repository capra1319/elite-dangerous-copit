package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestNumpadBindingsAndRelease(t *testing.T) {
	want := []string{"b6", "t1 toggle", "t2 toggle", "t3 toggle", "t4 toggle", "b1", "b2", "b3", "b4", "b5"}
	var latch keyLatch
	for i, command := range want {
		key := consoleKey{Down: true, VirtualKey: uint16(0x60 + i), Controls: 0x20}
		if got := latch.command(key); got != command {
			t.Fatalf("Num%d: got %q, want %q", i, got, command)
		}
		for repeat := 0; repeat < 5; repeat++ {
			if got := latch.command(key); got != "" {
				t.Fatalf("held Num%d repeated: %q", i, got)
			}
		}
		key.Down = false
		if got := latch.command(key); got != "" {
			t.Fatal("release triggered an action")
		}
		key.Down = true
		if got := latch.command(key); got != command {
			t.Fatal("new press after release was ignored")
		}
	}
}

func TestNumpadDoesNotCaptureTopRowOrModifiers(t *testing.T) {
	for _, key := range []consoleKey{
		{Down: true, VirtualKey: 0x31},                 // top-row 1
		{Down: true, VirtualKey: 0x30},                 // top-row 0
		{Down: true, VirtualKey: 0x23},                 // End, including Num Lock off
		{Down: true, VirtualKey: 0x61, Controls: 0x01}, // Alt
		{Down: true, VirtualKey: 0x61, Controls: 0x08}, // Ctrl
		{Down: true, VirtualKey: 0x61, Controls: 0x10}, // Shift
	} {
		var latch keyLatch
		if got := latch.command(key); got != "" {
			t.Fatalf("unexpected shortcut: %+v -> %s", key, got)
		}
	}
}

func TestNumpadSpecialKeys(t *testing.T) {
	for key, want := range map[uint16]string{0x0d: "close", 0x70: "help", 0x1b: "quit"} {
		var latch keyLatch
		if got := latch.command(consoleKey{Down: true, VirtualKey: key}); got != want {
			t.Fatalf("key %x: got %q", key, got)
		}
	}
}

func TestNumpadPreviewAndControlSelection(t *testing.T) {
	var out, errs bytes.Buffer
	if code := run([]string{"-mode", "mock", "-controls", "numpad", "-once", "-plain"}, strings.NewReader(""), &out, &errs); code != 0 {
		t.Fatal(code, errs.String())
	}
	for _, want := range []string{"CONTROLS: NUMPAD", "Num1 Gear", "Num0 Chaff", "Hold = one action"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing numpad hint %q", want)
		}
	}
	for _, args := range [][]string{{"-controls", "surprise"}, {"-mode", "live", "-controls", "numpad"}, {"-mode", "mock", "-controls", "numpad"}} {
		if code := run(args, strings.NewReader(""), &out, &errs); code != 2 {
			t.Fatalf("accepted unsupported input combination: %v", args)
		}
	}
	out.Reset()
	if code := run([]string{"-mode", "mock", "-controls", "commands", "-plain"}, strings.NewReader("t3 on\nq\n"), &out, &errs); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(out.String(), "Ship lights: one simulated pulse") || !strings.Contains(out.String(), "TYPED COMMANDS") {
		t.Fatal("typed controls must still work")
	}
}
