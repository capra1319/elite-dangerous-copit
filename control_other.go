//go:build !windows

package main

import (
	"context"
	"fmt"
	"time"
)

func defaultControlConfig() string { return "control.json" }
func runControl(ctx context.Context, d display, path, configPath string, interval, stale time.Duration, once bool) error {
	return fmt.Errorf("real-game control requires Windows amd64")
}
