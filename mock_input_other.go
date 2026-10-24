//go:build !windows

package main

import (
	"context"
	"io"
)

func prepareMockInput(_ context.Context, input io.Reader, _ io.Writer) (mockInput, error) {
	return prepareMockReader(input)
}
