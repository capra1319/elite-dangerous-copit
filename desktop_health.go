//go:build desktop

package main

import (
	"fmt"
	"math"
	"strings"
)

type desktopShipHealth struct {
	HullPercent, ShieldPercent *float64
	ShieldState, Source        string
	Offline                    bool
}

type desktopCockpitRoute struct {
	Available                                      bool
	Destination, Next, Remaining, Distance, Source string
}

func (m cockpitModel) desktopHealth() desktopShipHealth {
	result := desktopShipHealth{ShieldState: "UNKNOWN", Source: "Shield state unavailable. Hull is a last-reported journal snapshot: " + reportedAt(m.companion.Ship.HullAt)}
	if hull := m.companion.Ship.Hull; hull != nil && !math.IsNaN(*hull) && !math.IsInf(*hull, 0) && *hull >= 0 && *hull <= 1 {
		percent := *hull * 100
		result.HullPercent = &percent
	}
	link, _ := m.linkState()
	if m.known() && ((link == "TELEMETRY" && m.presence.Checked && m.presence.Known && m.presence.Running) || m.opts.mode == "mock") {
		result.ShieldState = "UP"
		result.Source = "Shields: current up/down state; percentage not reported. Hull snapshot: " + reportedAt(m.companion.Ship.HullAt)
		if !m.frame.State.has(shields) {
			zero := 0.0
			result.ShieldPercent, result.ShieldState, result.Offline = &zero, "OFFLINE", true
		} else if m.opts.mode == "mock" {
			sample := 82.0
			result.ShieldPercent = &sample
		}
	}
	if m.opts.mode == "mock" {
		result.Source = "SIMULATED shield and hull readings · no game telemetry"
	}
	return result
}

func (m cockpitModel) desktopRoute() desktopCockpitRoute {
	n := m.companion.Nav
	result := desktopCockpitRoute{Next: "Not reported", Remaining: "Not reported", Distance: "Not reported", Source: "No plotted destination reported."}
	index := n.currentRouteIndex()
	if index >= 0 && index < len(n.Route)-1 {
		result.Available = true
		result.Destination = reportedText(n.Route[len(n.Route)-1].StarSystem)
		result.Next = reportedText(n.Route[index+1].StarSystem)
		result.Remaining = fmt.Sprint(len(n.Route) - index - 1)
		if ly, ok := routeDistance(n.Position, n.Route[len(n.Route)-1].StarPos); ok && !math.IsNaN(ly) && !math.IsInf(ly, 0) {
			result.Distance = fmt.Sprintf("%.2f LY · direct", ly)
		}
		result.Source = "Route snapshot: " + reportedAt(n.RouteAt)
	} else if strings.TrimSpace(n.Target) != "" {
		result.Available, result.Destination, result.Next = true, reportedText(n.Target), reportedText(n.Target)
		if n.Remaining != nil && *n.Remaining >= 0 {
			result.Remaining = fmt.Sprint(*n.Remaining)
		}
		result.Source = "Reported FSD target; final route destination is not known."
	}
	return result
}
