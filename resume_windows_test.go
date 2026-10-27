//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"os"
	"testing"
)

func TestFocusResumeProcessIdentityRejectsReusedPID(t *testing.T) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	target := gameTarget{Window: 1, PID: uint32(os.Getpid()), Path: "EliteDangerous64.exe", Created: uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)}
	if !gameSessionAlive(target) {
		t.Fatal("running process session was not identified")
	}
	target.Created++
	if gameSessionAlive(target) {
		t.Fatal("same PID with different creation time was treated as the same session")
	}
}
