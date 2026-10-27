//go:build !windows

package main

import (
	"context"
	"errors"
)

func openExternalSearch(ctx context.Context, address string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.New("automatic browser launch is available on Windows; visit inara.cz/elite/nearest-stations/")
}
