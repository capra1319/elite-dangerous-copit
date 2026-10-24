package main

import (
	"path/filepath"
	"strings"
)

// Presence is a read-only UI hint. It never authorizes keyboard output.
type gamePresence struct {
	Checked, Known, Running bool
	Error                   string
}

func isGameExecutable(name string) bool {
	base := filepath.Base(name)
	return strings.EqualFold(base, "EliteDangerous64.exe") || strings.EqualFold(base, "EliteDangerous.exe")
}
