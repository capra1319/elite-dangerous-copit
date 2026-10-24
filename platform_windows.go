//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func enableANSI() bool {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	var mode uint32
	h := os.Stdout.Fd()
	ok, _, _ := kernel.NewProc("GetConsoleMode").Call(h, uintptr(unsafe.Pointer(&mode)))
	if ok == 0 {
		return false
	}
	ok, _, _ = kernel.NewProc("SetConsoleMode").Call(h, uintptr(mode|0x0004))
	return ok != 0
}

func terminalDimensions() (int, int) {
	var info struct {
		SizeX, SizeY, CursorX, CursorY int16
		Attributes                     uint16
		Left, Top, Right, Bottom       int16
		MaxX, MaxY                     int16
	}
	ok, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleScreenBufferInfo").Call(os.Stdout.Fd(), uintptr(unsafe.Pointer(&info)))
	if ok != 0 {
		return int(info.Right - info.Left + 1), int(info.Bottom - info.Top + 1)
	}
	return 100, 40
}

func savedGamesPath() string {
	// FOLDERID_SavedGames. Respect Windows folder redirection.
	id := syscall.GUID{Data1: 0x4C5C32FF, Data2: 0xBB9D, Data3: 0x43B0, Data4: [8]byte{0xB5, 0xB4, 0x2D, 0x72, 0xE5, 0x4E, 0xAA, 0xA4}}
	var path *uint16
	hr, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("SHGetKnownFolderPath").Call(uintptr(unsafe.Pointer(&id)), 0, 0, uintptr(unsafe.Pointer(&path)))
	if path == nil {
		return ""
	}
	defer syscall.NewLazyDLL("ole32.dll").NewProc("CoTaskMemFree").Call(uintptr(unsafe.Pointer(path)))
	if hr != 0 {
		return ""
	}
	units := make([]uint16, 0, 260)
	for offset := uintptr(0); offset < 32768; offset++ {
		unit := *(*uint16)(unsafe.Add(unsafe.Pointer(path), offset*2))
		if unit == 0 {
			return syscall.UTF16ToString(units)
		}
		units = append(units, unit)
	}
	return ""
}
