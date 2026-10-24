package main

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Interpret the emitted cursor/erase operations to verify what stays visible,
// including suffixes that must disappear when an error message gets shorter.
func applyScreen(t *testing.T, rows []string, output string) []string {
	t.Helper()
	row := 0
	textWritten := false
	escape := regexp.MustCompile(`\x1b\[([?0-9;]*)([A-Za-z])`)
	for len(output) > 0 {
		if strings.HasPrefix(output, "\x1b[") {
			match := escape.FindStringSubmatchIndex(output)
			if match == nil || match[0] != 0 {
				t.Fatalf("unknown terminal output: %q", output)
			}
			args, command := output[match[2]:match[3]], output[match[4]:match[5]]
			switch command {
			case "H":
				n, err := strconv.Atoi(strings.Split(args, ";")[0])
				if err != nil {
					t.Fatal(err)
				}
				row = n - 1
				textWritten = false
			case "K": // Text has replaced the row from column one already.
				if !textWritten && row < len(rows) {
					rows[row] = ""
				}
			case "J":
				if args == "2" {
					t.Fatal("renderer blanked the whole screen")
				}
				for i := row; i < len(rows); i++ {
					rows[i] = ""
				}
			case "l", "h":
			default:
				t.Fatalf("unexpected terminal command %q", command)
			}
			output = output[match[1]:]
			continue
		}
		end := strings.IndexByte(output, 0x1b)
		if end < 0 {
			end = len(output)
		}
		if row < 0 || row >= len(rows) {
			t.Fatal("renderer wrote outside the viewport")
		}
		rows[row] = output[:end]
		textWritten = true
		output = output[end:]
	}
	return rows
}

func TestScreenChangesIndicatorsWithoutBlankingOrRepaintingStaticRows(t *testing.T) {
	var screen terminalScreen
	var output bytes.Buffer
	screen.render(&output, "HEADER\nLights: OFF\nAction: waiting for telemetry\n", 80, 25)
	rows := applyScreen(t, make([]string, 25), output.String())
	output.Reset()
	screen.render(&output, "HEADER\nLights: ON\nAction: ok\n", 80, 25)
	if strings.Contains(output.String(), "HEADER") {
		t.Fatal("unchanged header was repainted")
	}
	rows = applyScreen(t, rows, output.String())
	if rows[0] != "HEADER" || rows[1] != "Lights: ON" || rows[2] != "Action: ok" {
		t.Fatalf("incorrect final screen: %q", rows[:3])
	}
	output.Reset()
	screen.render(&output, "HEADER\nLights: ON\nAction: ok\n", 80, 25)
	if output.Len() != 0 {
		t.Fatal("unchanged refresh must leave the terminal untouched")
	}
}

func TestScreenRemovesOldRowsAndRepaintsAfterResize(t *testing.T) {
	var screen terminalScreen
	var output bytes.Buffer
	screen.render(&output, "HEADER\nState: ON\nOld error\n", 80, 25)
	rows := applyScreen(t, make([]string, 25), output.String())
	output.Reset()
	screen.render(&output, "HEADER\nState: ON\n", 80, 25)
	rows = applyScreen(t, rows, output.String())
	if rows[2] != "" {
		t.Fatal("removed error is still visible")
	}
	output.Reset()
	screen.render(&output, "HEADER\nState: ON\n", 40, 20)
	if !strings.Contains(output.String(), "HEADER") {
		t.Fatal("resize did not redraw the frame")
	}
}

func TestSmallTerminalFitsIndicatorsAndPreservesActionFeedback(t *testing.T) {
	var frame bytes.Buffer
	s := newMock(testNow).State
	display{out: &frame, controlStatus: "DISARMED"}.show(&s, "control", strings.Repeat("path", 40), "RECENT", "Lights confirmed", testNow)
	lines := fitFrame(frame.String(), 80, 23)
	if len(lines) > 23 {
		t.Fatal("frame will scroll the terminal")
	}
	for _, line := range lines {
		if len([]rune(line)) >= 80 {
			t.Fatal("row can wrap and scroll")
		}
	}
	visible := strings.Join(lines, "\n")
	for _, want := range []string{"CONTROL: DISARMED", "L1 [ ]", "L4 [ ]", "Last action: Lights confirmed", "Num1 Gear"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("small frame hid important feedback: %s", want)
		}
	}
}
