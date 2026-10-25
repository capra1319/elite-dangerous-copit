//go:build windows

package main

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"
)

func openExternalSearch(ctx context.Context, address string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validStationSearchURL(address) {
		return fmt.Errorf("unsupported station search address")
	}
	verb, _ := syscall.UTF16PtrFromString("open")
	location, _ := syscall.UTF16PtrFromString(address)
	result, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(location)), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("Windows could not open the default browser (code %d)", result)
	}
	return nil
}
