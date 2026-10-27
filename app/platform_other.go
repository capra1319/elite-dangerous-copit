//go:build !windows

package main

func enableANSI() bool               { return true }
func savedGamesPath() string         { return "" }
func terminalDimensions() (int, int) { return 100, 40 }
