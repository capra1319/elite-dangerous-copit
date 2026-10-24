//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func prepareMockInput(ctx context.Context, input io.Reader, output io.Writer) (mockInput, error) {
	file, ok := input.(*os.File)
	if !ok {
		return prepareMockReader(input)
	}
	handle := windows.Handle(file.Fd())
	var original uint32
	if windows.GetConsoleMode(handle, &original) == nil {
		mode := (original | windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_EXTENDED_FLAGS) &^ uint32(windows.ENABLE_LINE_INPUT|windows.ENABLE_ECHO_INPUT|windows.ENABLE_QUICK_EDIT_MODE|windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
		if err := windows.SetConsoleMode(handle, mode); err != nil {
			return mockInput{}, err
		}
		reader := &mockConsoleReader{ctx: ctx, handle: handle, output: output}
		return mockInput{Reader: reader, cancel: func() {}, close: func() { _ = windows.SetConsoleMode(handle, original) }}, nil
	}
	// Own a duplicate so cancellation never closes the caller's stdin handle.
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), handle, windows.CurrentProcess(), &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return mockInput{}, err
	}
	return prepareMockReader(os.NewFile(uintptr(duplicate), file.Name()))
}

// Read console events only after checking availability. A normal line-mode
// ReadFile can block indefinitely on a partial line when Ctrl+C cancels context.
type mockConsoleReader struct {
	ctx     context.Context
	handle  windows.Handle
	output  io.Writer
	line    []uint16
	pending []byte
}

func (r *mockConsoleReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	for len(r.pending) == 0 {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		var count uint32
		ok, _, err := controlKernel32.NewProc("GetNumberOfConsoleInputEvents").Call(uintptr(r.handle), uintptr(unsafe.Pointer(&count)))
		if ok == 0 {
			return 0, fmt.Errorf("read console event count: %v", err)
		}
		if count == 0 {
			select {
			case <-r.ctx.Done():
				return 0, r.ctx.Err()
			case <-ticker.C:
			}
			continue
		}
		var record inputRecord
		var got uint32
		ok, _, err = controlKernel32.NewProc("ReadConsoleInputW").Call(uintptr(r.handle), uintptr(unsafe.Pointer(&record)), 1, uintptr(unsafe.Pointer(&got)))
		if ok == 0 {
			return 0, fmt.Errorf("read console input: %v", err)
		}
		if got == 0 || record.EventType != 1 || record.KeyDown == 0 || record.Char == 0 {
			continue
		}
		for i := 0; i < min(max(1, int(record.RepeatCount)), 1000); i++ {
			switch record.Char {
			case 13:
				r.pending = append(r.pending, []byte(string(utf16.Decode(r.line))+"\n")...)
				r.line = nil
				fmt.Fprint(r.output, "\r\n")
			case 8:
				if n := len(r.line); n > 0 {
					r.line = r.line[:n-1]
					if n > 1 && utf16.IsSurrogate(rune(r.line[n-2])) {
						r.line = r.line[:n-2]
					}
					fmt.Fprint(r.output, "\b \b")
				}
			case 26:
				if len(r.line) == 0 {
					return 0, io.EOF
				}
			default:
				if record.Char < 32 {
					continue
				}
				if len(r.line) >= 32768 {
					return 0, errors.New("mock command exceeds 32 Ki characters")
				}
				r.line = append(r.line, record.Char)
				if !utf16.IsSurrogate(rune(record.Char)) {
					fmt.Fprint(r.output, string(rune(record.Char)))
				}
			}
		}
	}
	n := copy(buffer, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
