package main

import (
	"fmt"
	"io"
	"strings"
)

type terminalScreen struct {
	lines         []string
	width, height int
	initialized   bool
}

func (d display) invalidateScreen() {
	if d.screen != nil {
		d.screen.initialized = false
	}
}

// Write only changed rows, in a single batch. Do not blank the screen between
// frames: that produced a visible flash and repainted every lamp each second.
func (s *terminalScreen) render(out io.Writer, frame string, width, height int) {
	if width < 4 || height < 4 {
		return
	}
	lines := fitFrame(frame, width, height-2)
	full := !s.initialized || s.width != width || s.height != height
	var update strings.Builder
	for row, line := range lines {
		if !full && row < len(s.lines) && line == s.lines[row] {
			continue
		}
		fmt.Fprintf(&update, "\x1b[%d;1H%s\x1b[K", row+1, line)
	}
	// Erase vanished rows and shortened suffixes, not the whole dashboard.
	for row := len(lines); row < len(s.lines) && row < height-2; row++ {
		fmt.Fprintf(&update, "\x1b[%d;1H\x1b[K", row+1)
	}
	if update.Len() == 0 {
		return
	}
	if full {
		fmt.Fprintf(&update, "\x1b[%d;1H\x1b[J", len(lines)+1)
	}
	fmt.Fprintf(&update, "\x1b[%d;1H", len(lines)+1)
	// One terminal write prevents the user seeing a partly rendered frame.
	if _, err := io.WriteString(out, "\x1b[?25l"+update.String()+"\x1b[?25h"); err != nil {
		s.initialized = false
		return
	}
	s.lines = lines
	s.width, s.height = width, height
	s.initialized = true
}

func fitFrame(frame string, width, maxRows int) []string {
	lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
	// Compact small windows before sacrificing state or action rows.
	for _, prefix := range []string{"", "Timestamp:", "Source:", "LED legend:"} {
		if len(lines) <= maxRows {
			break
		}
		compacted := make([]string, 0, len(lines))
		for _, line := range lines {
			remove := line == ""
			if prefix != "" {
				remove = strings.HasPrefix(line, prefix)
			}
			if remove {
				// Each pass may drop optional rows, never an indicator or status row.
				continue
			}
			compacted = append(compacted, line)
		}
		lines = compacted
	}
	if len(lines) > maxRows {
		lines = lines[:maxRows]
		lines[maxRows-1] = "Enlarge terminal to show the remaining controls."
	}
	for i, line := range lines {
		runes := []rune(strings.ReplaceAll(strings.TrimSuffix(line, "\r"), "\t", "    "))
		// Keep away from the final column to prevent wrapping and viewport scroll.
		if len(runes) > width-1 {
			line = string(runes[:width-4]) + "..."
		} else {
			line = string(runes)
		}
		lines[i] = line
	}
	return lines
}
