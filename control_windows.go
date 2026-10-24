//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var controlUser32 = syscall.NewLazyDLL("user32.dll")
var controlKernel32 = syscall.NewLazyDLL("kernel32.dll")

func foregroundGame() gameTarget {
	hwnd, _, _ := controlUser32.NewProc("GetForegroundWindow").Call()
	if hwnd == 0 {
		return gameTarget{}
	}
	var pid uint32
	controlUser32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	h, _, _ := controlKernel32.NewProc("OpenProcess").Call(0x1000, 0, uintptr(pid))
	if h == 0 {
		return gameTarget{}
	}
	defer controlKernel32.NewProc("CloseHandle").Call(h)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	ok, _, _ := controlKernel32.NewProc("QueryFullProcessImageNameW").Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if ok == 0 {
		return gameTarget{}
	}
	t := gameTarget{Window: hwnd, PID: pid, Path: syscall.UTF16ToString(buf[:size])}
	if !t.valid() {
		return gameTarget{}
	}
	return t
}

// INPUT is 40 bytes on amd64; its union is aligned at offset 8.
type keyboardInput struct {
	VK, ScanCode uint16
	Flags, Time  uint32
	Extra        uintptr
}
type nativeInput struct {
	Type  uint32
	Union [4]uintptr
}

type windowsPulseAPI struct{}

func (windowsPulseAPI) Foreground() gameTarget { return foregroundGame() }
func (windowsPulseAPI) Held(vk uint16) bool {
	value, _, _ := controlUser32.NewProc("GetAsyncKeyState").Call(uintptr(vk))
	return value&0x8000 != 0
}
func (windowsPulseAPI) Scan(scan uint16, up bool) error {
	var input nativeInput
	input.Type = 1
	ki := (*keyboardInput)(unsafe.Pointer(&input.Union[0]))
	ki.ScanCode = scan
	ki.Flags = 0x0008
	if up {
		ki.Flags |= 0x0002
	}
	count, _, err := controlUser32.NewProc("SendInput").Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	if count != 1 {
		return fmt.Errorf("Windows accepted %d of 1 keyboard events (%v); run both programs normally at the same privilege level", count, err)
	}
	return nil
}
func (windowsPulseAPI) Wait(ctx context.Context, duration time.Duration, target gameTarget) error {
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			if foregroundGame() != target {
				return errors.New("focus changed during pulse")
			}
			return nil
		case <-ticker.C:
			if foregroundGame() != target {
				return errors.New("focus changed during pulse")
			}
		}
	}
}

type windowsSink struct{}

func (windowsSink) Pulse(ctx context.Context, key string, target gameTarget) error {
	return deliverPulse(ctx, key, target, windowsPulseAPI{})
}

type windowMessage struct {
	Window  uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

func defaultControlConfig() string {
	exe, err := os.Executable()
	if err != nil {
		return "control.json"
	}
	return filepath.Join(filepath.Dir(exe), "control.json")
}

func runControl(ctx context.Context, d display, path, configPath string, interval, stale time.Duration, once bool) error {
	if ctx.Err() != nil {
		return nil
	}
	if runtime.GOARCH != "amd64" {
		return errors.New("real-game control currently requires Windows amd64")
	}
	config, err := loadControlConfig(configPath)
	if err != nil {
		return err
	}
	c := &controlEngine{Config: config, Sink: windowsSink{}, StaleAfter: stale, Last: "DISARMED: focus the cockpit, close panels/chat, then Ctrl+Alt+F9"}
	refresh := func() error {
		s, readErr := readWithRetry(ctx, path)
		if readErr == nil {
			c.update(&s, nil, time.Now(), foregroundGame())
		} else {
			c.update(nil, readErr, time.Now(), foregroundGame())
		}
		profileErr := profileReady(config)
		if profileErr != nil && c.Armed {
			c.disarm(profileErr.Error())
		}
		return profileErr
	}
	profileErr := refresh()
	if ctx.Err() != nil {
		return nil
	}
	show := func() {
		status := "DISARMED"
		if c.Armed {
			status = "ARMED"
		}
		d.controlStatus = status
		d.pending = ""
		if c.Pending != nil {
			d.pending = c.Pending.Action.ID
		}
		d.gameFocused = foregroundGame().valid()
		quality := qualityFor(c.State, c.ReadError, time.Now(), stale)
		if profileErr != nil {
			quality += " | SETUP: " + profileErr.Error()
		}
		d.show(c.State, "control", path, quality, c.Last, time.Now())
	}
	show()
	if ctx.Err() != nil {
		return nil
	}
	if once {
		return nil
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	register := controlUser32.NewProc("RegisterHotKey")
	unregister := controlUser32.NewProc("UnregisterHotKey")
	peek := controlUser32.NewProc("PeekMessageW")
	// Claim one control session; arm/stop and page shortcuts persist until exit.
	registered := make(map[int]bool)
	add := func(id int, mods, vk uintptr) error {
		ok, _, err := register.Call(0, uintptr(id), mods|0x4000, vk)
		if ok == 0 {
			if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
				return errors.New("Windows denied global shortcuts in this environment; run Start-Game-Control.cmd directly from File Explorer in your normal desktop session")
			}
			return fmt.Errorf("hotkey %d unavailable (%v); close another control instance or conflicting shortcut app", id, err)
		}
		registered[id] = true
		return nil
	}
	removeActions := func() {
		for id := 100; id < 110; id++ {
			if registered[id] {
				unregister.Call(0, uintptr(id))
				delete(registered, id)
			}
		}
	}
	defer func() {
		for id := range registered {
			unregister.Call(0, uintptr(id))
		}
	}()
	if err := add(1, 0x0001|0x0002, 0x78); err != nil {
		return err
	} // Ctrl+Alt+F9
	if err := add(2, 0x0001|0x0002, 0x79); err != nil {
		return err
	} // Ctrl+Alt+F10
	if d.navigate != nil {
		d.pageStatus = "Global: Num- previous / Num+ next (works while Elite is focused)"
		for _, pageKey := range []struct {
			id int
			vk uintptr
		}{{previousPageHotkey, 0x6d}, {nextPageHotkey, 0x6b}} {
			if err := add(pageKey.id, 0, pageKey.vk); err != nil {
				for _, id := range []int{previousPageHotkey, nextPageHotkey} {
					if registered[id] {
						unregister.Call(0, uintptr(id))
						delete(registered, id)
					}
				}
				d.pageStatus = "Global page keys unavailable; use Num- or Num+ in the app. " + err.Error()
				break
			}
		}
	}
	addActions := func() error {
		for n := 0; n < 10; n++ {
			if err := add(100+n, 0, uintptr(0x60+n)); err != nil {
				removeActions()
				return err
			}
		}
		return nil
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	nextRead := time.Now().Add(interval)
	nextDraw := time.Now().Add(time.Second)
	requests := d.requests
	var armedSince uint32
	for {
		if ctx.Err() != nil {
			c.disarm("app closing")
			return nil
		}
		select {
		case <-ctx.Done():
			c.disarm("app closing")
			fmt.Fprintln(d.out, "\nStopped; hotkeys released.")
			return nil
		case request, open := <-requests:
			if !open {
				requests = nil
				continue
			}
			if request == "disarm" {
				c.disarm("user stop from dashboard")
				removeActions()
				show()
			}
			continue
		case <-ticker.C:
		}
		now := time.Now()
		if c.Armed && foregroundGame() != c.Target {
			c.disarm("game lost focus")
			removeActions()
			show()
		}
		if !now.Before(nextRead) {
			profileErr = refresh()
			if !c.Armed {
				removeActions()
			}
			nextRead = time.Now().Add(interval)
			// Publish new telemetry/confirmations as soon as they are read. The
			// one-second redraw remains only an idle heartbeat (and plain output).
			if d.publish != nil || d.ansi {
				show()
				nextDraw = time.Now().Add(time.Second)
			}
		}
		if !now.Before(nextDraw) {
			show()
			nextDraw = now.Add(time.Second)
		}
		for {
			if ctx.Err() != nil {
				c.disarm("app closing")
				return nil
			}
			var msg windowMessage
			available, _, _ := peek.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
			if available == 0 {
				break
			}
			if msg.Message != 0x0312 {
				continue
			}
			id := int(msg.WParam)
			if !registered[id] {
				continue
			}
			ticks, _, _ := controlKernel32.NewProc("GetTickCount").Call()
			if !acceptControlHotkey(id, msg.Time, uint32(ticks), armedSince) {
				continue
			}
			if d.handlePageHotkey(id) {
				continue
			}
			if id == 2 {
				c.disarm("emergency stop")
				removeActions()
				show()
				continue
			}
			if id == 1 {
				if c.Armed {
					c.disarm("user toggle")
					removeActions()
				} else {
					profileErr = refresh()
					if ctx.Err() != nil {
						c.disarm("app closing")
						return nil
					}
					if err := c.arm(foregroundGame(), time.Now(), profileErr); err != nil {
						c.Last = "Cannot arm: " + err.Error()
					} else if err := addActions(); err != nil {
						c.disarm(err.Error())
					} else {
						ticks, _, _ := controlKernel32.NewProc("GetTickCount").Call()
						armedSince = uint32(ticks)
					}
				}
				show()
				continue
			}
			if id < 100 || id >= 110 {
				continue
			}
			profileErr = refresh()
			n := id - 100
			actionIndex := n - 1
			if n == 0 {
				actionIndex = 9
			}
			if err := c.trigger(ctx, liveActions[actionIndex].ID, time.Now(), foregroundGame(), profileErr); err != nil {
				c.Last = "Blocked: " + err.Error()
			}
			if !c.Armed {
				removeActions()
			}
			show()
		}
	}
}
