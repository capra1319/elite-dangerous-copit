package main

// GetTickCount and MSG.Time are wrapping 32-bit millisecond counters. Stop
// remains available even when delayed; arming and actions must be recent.
func acceptControlHotkey(id int, posted, now, armedSince uint32) bool {
	if id == 2 {
		return true
	}
	if now-posted > 300 {
		return false
	}
	if id >= 100 && id < 110 && int32(posted-armedSince) < 0 {
		return false
	}
	return id == 1 || id == previousPageHotkey || id == nextPageHotkey || (id >= 100 && id < 110)
}
