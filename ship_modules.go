package main

import (
	"fmt"
	"math"
	"strings"
)

// shipModuleView is presentation only. Reported damage never issues a control request.
type shipModuleView struct {
	Slot, Item, Name, Power, Health, Condition, Label, Consequence string
	KeySystem                                                      bool
	HealthPercent                                                  *float64
}

func moduleRole(module shipModule) (name, consequence string) {
	slot := strings.ToLower(module.Slot)
	item := strings.ToLower(module.Item)
	switch {
	case slot == "powerplant" || strings.HasPrefix(item, "int_powerplant_"):
		return "Power plant", "Failure can reduce the power available to ship systems."
	case slot == "mainengines" || strings.HasPrefix(item, "int_engine_"):
		return "Thrusters", "Failure can compromise thrust and manoeuvring."
	case slot == "frameshiftdrive" || strings.HasPrefix(item, "int_hyperdrive_"):
		return "Frame shift drive", "Failure can prevent supercruise or hyperspace escape."
	case slot == "lifesupport" || strings.HasPrefix(item, "int_lifesupport_"):
		return "Life support", "Failure can compromise the cockpit's breathable atmosphere."
	case slot == "cockpit" || slot == "canopy" || strings.HasPrefix(item, "int_cockpit_"):
		return "Canopy", "A breach can expose the cockpit and require emergency oxygen."
	case slot == "powerdistributor" || strings.HasPrefix(item, "int_powerdistributor_"):
		return "Power distributor", "Failure can compromise energy distribution."
	case slot == "radar" || slot == "sensors" || strings.HasPrefix(item, "int_sensors_"):
		return "Sensors", "Failure can compromise contacts and target acquisition."
	case strings.HasPrefix(item, "int_shieldgenerator_") || strings.HasPrefix(item, "int_shieldgeneratorfast_"):
		return "Shield generator", "Failure can leave the hull without shield protection."
	}
	return "", ""
}

func shipModuleRows(modules []shipModule) []shipModuleView {
	rows := make([]shipModuleView, 0, len(modules))
	for _, module := range modules {
		name, consequence := moduleRole(module)
		row := shipModuleView{Slot: safeText(module.Slot), Item: symbolText(module.Item),
			Name: name, Consequence: consequence, KeySystem: name != "", Power: "OFF",
			Health: "Unknown", Condition: "unknown", Label: "INTEGRITY UNKNOWN"}
		if row.KeySystem {
			row.Label = "KEY SYSTEM / INTEGRITY UNKNOWN"
		}
		if row.Name == "" {
			row.Name = row.Item
			if row.Name == "" {
				row.Name = reportedText(row.Slot)
			}
		}
		if module.On {
			row.Power = "ON"
		}
		if module.Health != nil && !math.IsNaN(*module.Health) && !math.IsInf(*module.Health, 0) && *module.Health >= 0 && *module.Health <= 1 {
			health := *module.Health
			percent := health * 100
			row.HealthPercent = &percent
			row.Health = fmt.Sprintf("%.1f%%", percent)
			if health > .999 && health < 1 {
				row.Health = "<100%"
			} else if health > 0 && health < .001 {
				row.Health = "<0.1%"
			}
			row.Condition, row.Label = "ok", "NO DAMAGE REPORTED"
			if row.KeySystem {
				row.Label = "KEY SYSTEM"
			}
			if health < 1 {
				row.Condition, row.Label = "damaged", "DAMAGE REPORTED"
				if row.KeySystem {
					row.Condition, row.Label = "critical", "KEY SYSTEM DAMAGED"
				}
			}
			if health == 0 {
				row.Label = "ZERO INTEGRITY REPORTED"
			}
		}
		rows = append(rows, row)
	}
	return rows
}
