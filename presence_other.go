//go:build !windows

package main

func probeGamePresence() gamePresence {
	return gamePresence{Checked: true, Error: "process detection requires Windows; using telemetry"}
}
