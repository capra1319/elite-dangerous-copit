//go:build windows

package main

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

func probeGamePresence() gamePresence {
	result := gamePresence{Checked: true}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		if isGameExecutable(windows.UTF16ToString(entry.ExeFile[:])) {
			result.Known, result.Running = true, true
			return result
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		result.Known = true
	} else {
		result.Error = err.Error()
	}
	return result
}
