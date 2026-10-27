//go:build !windows

package main

import (
	"context"
	"fmt"
	"os"
)

func runNumpad(ctx context.Context, input *os.File, d display) error {
	return fmt.Errorf("numpad controls currently require Windows; use -controls commands")
}
