package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
)

type mockInput struct {
	io.Reader
	cancel, close func()
}

func prepareMockReader(input io.Reader) (mockInput, error) {
	if closer, ok := input.(io.ReadCloser); ok {
		var once sync.Once
		close := func() { once.Do(func() { _ = closer.Close() }) }
		return mockInput{Reader: input, cancel: close, close: close}, nil
	}
	switch input.(type) {
	case *strings.Reader, *bytes.Reader, *bytes.Buffer:
		return mockInput{Reader: input, cancel: func() {}, close: func() {}}, nil
	}
	return mockInput{}, errors.New("mock text input must be a cancellable stream or an in-memory reader")
}
