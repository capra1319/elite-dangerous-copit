# Elite Aux — possible additions

These are feature ideas, not a claim that every feature is already implemented. The current app uses local Status.json telemetry and guarded keyboard input. Most richer displays can use the Player Journal and companion JSON files without an account, paid service or hardware.

**Now implemented:** Navigation (F7), Ship Overview (F6), Mission Board (F8) and Engineering Checklist (F9), including journal replay/tailing, route snapshots and saved material targets. All use the three selectable TUI themes. See README for data availability and controls. Engineering currently tracks your own material quantities; automatic blueprint recipes remain a possible later extension.

## Toggle controls

**Implemented:** landing gear (Num1), cargo scoop (Num2), ship lights (Num3), hardpoints (Num4) and night vision (Num5). All five display reported ACTIVE/INACTIVE state in their categories: Flight & Utility (gear/scoop), Visibility (lights/night vision) and Combat (hardpoints). Night vision retains its existing b1 command and F5 game binding.

| Possible extra toggle | Reported state | Implementation notes |
|---|---|---|
| Flight assist | Flags bit 5: flight assist OFF | Invert this bit for an intuitive ON indicator; add an optional validated game binding. |
| Silent running | Flags bit 10 | Show a prominent heat warning and retain manual arming/context checks. |
| Analysis / combat HUD mode | Flags bit 27: analysis mode | Display the actual mode rather than an arbitrary on/off label. |
| SRV handbrake | Flags bit 12 | A separate SRV page and context-specific bindings are needed. |
| SRV drive assist | Flags bit 15 | Keep SRV state and controls separate from main-ship controls. |

Adding a command requires extending the preset/configuration and guarded input mapping. A status flag alone is read-only feedback. Keep existing numpad shortcuts stable; introduce new bindings deliberately rather than silently reusing occupied keys. No new commands should automatically fire on connection or reconnection.

## Additional pages and integrations

| Feature | Data / approach | Useful behaviour |
|---|---|---|
| Navigation panel | Journal Location, FSDTarget, FSDJump and NavRoute.json | Current system, next destination, star class and remaining plotted route. |
| Audio alerts | Existing low-fuel/overheating flags and journal warnings | Optional sounds or spoken alerts on a new warning, with cooldown/mute controls. |
| Commander and ship identity | Journal LoadGame and Loadout | Commander, ship name/type, tank capacity and loadout summary. |
| Hull damage and repairs | Loadout, HullDamage and repair journal events | Timestamped last-reported health and damage alerts. HullDamage is threshold-based, not a continuous health feed. |
| Missions and cargo | Journal mission/cargo events and Cargo.json | Destinations, deadlines, delivery quantities and cargo manifest. Reconstruct state from startup records/history and distinguish missing data. |
| Exploration and materials | Scan/FSS, Materials and collection journal events | Discovery log, collected materials and session totals. |
| Flight log | Journal travel, docking and combat events | Searchable session history and trip statistics, beyond the current control activity log. |
| Control profiles | Local configuration and game-binding validation | Different layouts for exploration, trading and combat, with an explicit profile selection. |
| ESP32 panel | Later USB-serial transport | Physical switches send intents to the same guarded backend; LED updates reflect reported telemetry. The app remains usable without hardware. |

Navigation is now available. Optional audio alerts remain a useful next addition without changing the current input setup.

Sources: [Frontier Player Journal Manual, v32](https://hosting.zaonce.net/community/journal/v32/Journal_Manual-v32.pdf), especially Status (§14), NavRoute (§4.18), HullDamage (§5.10), startup records and mission events. The game may expose newer fields; inspect real samples before implementing them.
