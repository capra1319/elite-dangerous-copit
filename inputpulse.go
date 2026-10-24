package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type pulseAPI interface {
	Foreground() gameTarget
	Held(uint16) bool
	Scan(uint16, bool) error
	Wait(context.Context, time.Duration, gameTarget) error
}

func deliverPulse(ctx context.Context, key string, target gameTarget, api pulseAPI) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	scan, ok := functionKeyScan(key)
	if !ok {
		return errors.New("unsupported keyboard binding")
	}
	if !target.valid() || api.Foreground() != target {
		return errors.New("Elite Dangerous is no longer foreground")
	}
	for _, vk := range []uint16{0x10, 0x11, 0x12, 0x5b, 0x5c, functionKeyVK(key)} {
		if api.Held(vk) {
			return errors.New("release held modifiers and output keys before using the panel")
		}
	}
	if api.Foreground() != target {
		return errors.New("focus changed before input")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err = api.Scan(scan, false); err != nil {
		return fmt.Errorf("key down: %w", err)
	}
	defer func() {
		// Pair our own down even on cancellation/focus loss. Never release user keys.
		if releaseErr := api.Scan(scan, true); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("key release: %w", releaseErr))
		}
	}()
	return api.Wait(ctx, 60*time.Millisecond, target)
}

func functionKeyVK(key string) uint16 {
	scan, _ := functionKeyScan(key)
	if scan >= 0x3b && scan <= 0x44 {
		return 0x70 + scan - 0x3b
	}
	if scan == 0x57 {
		return 0x7a
	}
	if scan == 0x58 {
		return 0x7b
	}
	return 0
}
