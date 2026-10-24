# Connect the panel to Elite Dangerous

The app now has a real-game control mode. It reads Elite's local Status.json for feedback and uses Windows keyboard input to request actions through Elite's bindings. The journal/status interface supplies telemetry; it is not a writable command endpoint. A virtual joystick is another possible input route, but would add a device driver to this prototype.

## One-time setup

1. Extract the full package into a new folder. Do not run only the EXE from inside the ZIP.
2. Run **Install-Preset.cmd**. This adds **EliteAuxPanel.4.2.binds** to Elite's bindings folder. If it has already been installed unchanged, the installer does nothing. It refuses to replace a different existing EliteAuxPanel preset.
3. Restart Elite if it was already open so it can discover the new preset.
4. In **Options > Controls > Ship Controls**, choose **EliteAuxPanel** in the preset list and apply it. Leave the general, SRV and on-foot preset selections as they are.
5. Open **eliteaux.exe** or **Start.cmd** (the existing **Start-Game-Control.cmd** also works). The app immediately scans for Elite and connects when your commander session is ready. Return to your ship's cockpit and close the internal/external panels, chat and maps.
6. Turn **Num Lock on**. With Elite focused, press **Ctrl+Alt+F9** to arm. The panel on monitor two will say **ARMED**.
7. First try **Num3** to toggle lights. The lamp changes only after the game writes the matching state. Press Num3 again to turn them back off.

The supplied preset was cloned from this user's Custom.4.2 HOTAS profile. Its primary HOTAS bindings were preserved; only ten previously empty secondary slots were given function-key bindings. Do not use this personalized preset to replace somebody else's controls. The original Custom preset and StartPreset selection files are not overwritten by the installer.

## Real-game keys

| Numpad trigger | Function | Output binding added to the new preset |
|---|---|---|
| Num1 | Toggle landing gear | F1 |
| Num2 | Toggle cargo scoop | F2 |
| Num3 | Toggle ship lights | F3 |
| Num4 | Toggle hardpoints | F4 |
| Num5 | Toggle night vision | F5 |
| Num6 | Open/close galaxy map | F6 |
| Num7 | Open/close system map | F7 |
| Num8 | Next fire group | F8 |
| Num9 | Heat sink | F9 |
| Num0 | Chaff | F10 |
| Num+ | Next app page (even while Elite is focused) | No game input |
| Num- | Previous app page (even while Elite is focused) | No game input |

There is no need to press Enter after these keys. **Ctrl+Alt+F9** toggles arming; **Ctrl+Alt+F10** disarms immediately. To exit, disarm, return to the app terminal and press Ctrl+C. Escape in the game retains its normal game function; it does not close the controller app.

Switching away from Elite disarms the app and releases the ten flight-action numpad hotkeys. Return to the cockpit and press Ctrl+Alt+F9 again. The two modified arm/stop shortcuts are registered while the controller runs; a second controller instance cannot claim them. Num+ and Num- remain available for paging while disarmed and while Elite is focused; keep those two keys unbound in Elite. Optional page-key conflicts are reported in Diagnostics and do not disable flight control. Numpad flight-control conflicts are reported instead of silently proceeding.

## What confirmation means

Gear, scoop, lights, hardpoints and night vision wait for a changed game snapshot that matches the requested state. An input pulse does not change any LED by itself. While waiting, further requests are rejected; they are not queued. After five seconds without matching telemetry, the app reports a timeout and never automatically repeats a toggle.

Maps, fire-group selection and consumables display input delivery as unconfirmed. Heat sinks/chaff still require the relevant modules and ammunition; sending a key does not prove a module fired. Those two buttons consume ammunition when the game accepts them.

The app blocks controls for unknown state, on-foot/SRV/fighter/taxi context and incompatible flight conditions. Gear/scoop/hardpoints and consumables are blocked while docked, landed, charging or in supercruise. Lights can be tested while docked. The matching map shortcut can close a map; other actions are blocked when a GUI panel is selected.

**Disarm before typing chat or navigating menus.** GuiFocus cannot identify every chat/menu state. The app also checks the foreground game window immediately before sending input, but Windows keyboard injection is global and has a small focus-change race. Do not treat this prototype as a primary flight controller.

## Troubleshooting

- **SETUP: select EliteAuxPanel** — select that preset specifically in Ship Controls and apply it. The app verifies the second line of StartPreset.4.start and checks all ten output bindings before arming.
- **Main-ship telemetry not available** — enter the ship and wait for Status.json to update. Missing fields remain unknown.
- **Telemetry stale** — status is older than the default 60 seconds. Use the normal game controls to cause a fresh status update, then rearm. Quiet flight can leave an unchanged snapshot; this age check is deliberately conservative. `-stale-after 2m` changes the threshold if needed.
- **Close panels/maps** — return to the unobstructed cockpit before arming. A status file with GuiFocus 1 means the ship's right-hand panel is selected.
- **Keyboard delivery failed** — run the game and panel normally at the same privilege level. Windows can block input into an elevated process. The app will not elevate itself.
- **Key conflict** — remove duplicate game numpad bindings, or close the other hotkey application. The prepared profile leaves the numpad free. Raw game-controller input can still see physical keys, so the app validates that the game profile does not bind numpad triggers directly.
- **Preset disappeared after changing controls** — Elite may save the edited profile as Custom. The controller intentionally refuses to run against a different active profile. Re-select the supplied preset or update the named bindings/config together.
- **No game action despite successful input delivery** — test the corresponding F-key directly with the panel disarmed. If that also fails, check the selected game preset. Real in-game acceptance and third-party keyboard tools vary with the user's setup; the automated checks do not prove gameplay success.

For a nonstandard setup, `-config PATH` selects a JSON config. `bindingsFile` can be an explicit full path; the default `auto` resolves the named preset in the game's normal bindings folder. Output bindings support unique single function keys F1-F12. The app never changes the game profile during play.

## Other modes remain available

Normal startup uses real-game control and starts disarmed. With the app focused, **F4** explicitly enables mock mode and stops the real-control backend. Press **F4** again to discard simulated state and resume real detection, disarmed. From the read-only launcher, F4 returns to read-only mode. A normal relaunch always starts with mock off.

- **Start-Mock.cmd** — typed simulator commands.
- **Start-Mock-Numpad.cmd** — focused-terminal numpad simulator.
- **Start-Live.cmd** — read-only game state viewer.
- **Start-Game-Control.cmd** — numpad operates the real game after arming.

The mock's Enter/F1/Esc shortcuts belong to the simulator. In real-game mode only the listed numpad triggers and modified arm/stop shortcuts are registered.

## References

[Frontier's Journal Manual, section 14](https://hosting.zaonce.net/community/journal/v32/Journal_Manual-v32.pdf) defines the telemetry snapshot. The [Elite Stream Deck plugin's binding documentation](https://github.com/mhwlng/streamdeck-elite) describes the same need for keyboard bindings alongside joystick controls. Windows input uses [SendInput](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput); shortcuts use [RegisterHotKey](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-registerhotkey).
