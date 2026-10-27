# Elite Dangerous Virtual Auxiliary Button Box

**Codex-ready project specification · Version 1.0 · 23 October 2026**

## 1. Goal and starting point

Build a Windows desktop auxiliary control panel for a second monitor. Prove the layout, game feedback and input reliability in software before building a physical box with **4 maintained toggles, 6 momentary buttons and 4 LEDs**. The MVP must run without an ESP32, Raspberry Pi, serial device or game installation in mock mode.

The earlier conversation establishes a standard ESP32 and a stock Raspberry Pi 2 as available equipment. Its design replies were unavailable as readable text when preparing this plan. Consequently, the counts above are retained from the current request, while the assignments below are **proposed defaults**, not a reconstruction of the original wiring or labels. Make them configurable. Do not invent GPIO assignments or assume a particular ESP32 variant.

Success means the user can open the panel on monitor two, operate selected ship functions deliberately, and see telemetry-driven indicators that also follow changes made with the keyboard or HOTAS. A click is a request; telemetry is evidence of state. This is an auxiliary panel, not a primary flight controller.

### Scope

- Local, single-user Windows app; normal user privileges; mouse first.
- Four virtual maintained controls, six momentary controls, four prominent status lamps.
- Mock, live observation and live control modes, visibly distinguished.
- Read local game telemetry; send configured keyboard bindings only after explicit arming.
- Portable release folder, settings, diagnostic log and manual test guide.

Defer touchscreen certification, web/mobile clients, networking, cloud accounts, macros, voice, joystick emulation, firmware, enclosure design and Raspberry Pi services. No memory reading, game hooks, packet inspection or edits to game telemetry. Do not automatically edit game binding files.

## 2. Recommended implementation

Use **C# with .NET 10 LTS, WPF and XAML**, targeting `net10.0-windows`, initially `win-x64`. Use the current .NET 10 servicing SDK when implementation starts and record it in `global.json`. WPF provides a native Windows UI and direct Win32 interop without a browser server or embedded Chromium. This is lightweight in architecture; self-contained packaging will still include the .NET runtime. [WPF overview](https://learn.microsoft.com/en-us/dotnet/desktop/wpf/overview/), [.NET support](https://learn.microsoft.com/en-us/dotnet/core/releases-and-support).

Use `System.Text.Json`, `FileSystemWatcher` plus polling, a small explicit view-model layer, P/Invoke for Windows input/window functions, and xUnit for the core logic. Avoid a dependency-injection framework, database and large UI toolkit. Add `System.IO.Ports` only during the later hardware phase. Prefer plain styles and built-in controls over elaborate cockpit graphics.

### Project layout

```text
EliteAuxPanel.sln
global.json
README.md
src/
  EliteAuxPanel.Core/         # normalized state, decoding, command policy, simulator
  EliteAuxPanel.Windows/      # status reader, journal session guard, Win32 adapters
  EliteAuxPanel.App/          # WPF views, view models, composition, settings
tests/
  EliteAuxPanel.Core.Tests/   # decoding, policy, reconciliation, simulator
  EliteAuxPanel.Windows.Tests/# file replacement and adapter contract checks
fixtures/                    # synthetic status snapshots and journal excerpts
docs/
  BINDINGS.md
  MANUAL-TESTS.md
  HARDWARE-MIGRATION.md
```

Keep the core independent of WPF and Win32. Use the same command policy for UI and future serial events.

## 3. Panel layout and proposed assignments

Design for 1280×720 and 1920×1080, with readable scaling and large targets. A top strip displays mode, session/context, data quality, game focus and armed state. Below it: four toggles, six buttons in two rows of three, and four lamps. Include Settings, Disarm, and a compact event history outside the 4/6/4 hardware layout.

| ID | Proposed control | Semantics | Feedback |
|---|---|---|---|
| T1 | Landing gear | Desired deployed/retracted state | Gear flag |
| T2 | Cargo scoop | Desired deployed/retracted state | Scoop flag |
| T3 | Ship lights | Desired on/off state | Lights flag |
| T4 | Hardpoints | Desired deployed/retracted state | Hardpoints flag |
| B1 | Night vision | One toggle pulse | Night-vision flag |
| B2 | Galaxy map | One pulse | Delivery result only in MVP |
| B3 | System map | One pulse | Delivery result only in MVP |
| B4 | Next fire group | One pulse | Delivery result only in MVP |
| B5 | Heat sink | One pulse, visibly marked consumable | No guaranteed activation confirmation |
| B6 | Chaff | One pulse, visibly marked consumable | No guaranteed activation confirmation |
| L1 | Landing gear | Lit when deployed | Actual state only |
| L2 | Cargo scoop | Lit when deployed | Actual state only |
| L3 | Ship lights | Lit when on | Actual state only |
| L4 | Hardpoints | Lit when deployed | Actual state only |

These four lamps deliberately validate the four maintained controls. Optional future profiles can replace lamps with shields, mass lock, fuel or heat warnings. Do not expand the physical count to accommodate those; use small text badges in the software UI.

Every control has an editable label and action mapping. Leave real key bindings **unset** initially. Do not assume default keyboard shortcuts. The user assigns spare bindings in Elite's Controls menu and enters matching bindings in the app. Game actions that can be configured as hold or toggle must be verified as **toggle** for this MVP.

Show `ACTUAL: ON/OFF/UNKNOWN` and a separate pending request where appropriate. Pending amber means a request is unresolved, not that the corresponding LED is on. Use text/icons as well as colour. For unsupported context or unavailable telemetry show grey/unknown, not a reassuring off state. A button pulse displays `sent—unconfirmed`; input insertion alone is not game acknowledgement.

## 4. Architecture and contracts

```text
Status.json -> LiveTelemetrySource ---+
Mock scenarios -> MockTelemetrySource+--> StateStore --> panel + LED projection
Journal session guard --------------+

Panel intents ----------------------+--> CommandCoordinator --> InputSink
Future serial intents --------------+         ^                   |
                                               |          dry-run or Win32
                                     state + focus/session policy

StateStore -> future SerialLedSink -> ESP32 -> four LEDs
```

Suggested small interfaces, implemented asynchronously with cancellation:

```csharp
interface ITelemetrySource { IAsyncEnumerable<TelemetrySample> ReadAsync(CancellationToken ct); }
interface IInputSink { Task<InputDelivery> PulseAsync(KeyBinding binding, CancellationToken ct); }
interface IGameWindowService { GameWindowState Inspect(); }
interface IClock { DateTimeOffset UtcNow { get; } }
```

`CommandCoordinator` accepts `SetDesired(action, bool)` or `Pulse(action)` with an intent ID and source. It returns an explicit outcome: blocked, already satisfied, pending, observed satisfied, sent unconfirmed, failed or timed out. Serialize decisions and delivery; never let UI code call the Win32 adapter directly.

State includes nullable observed values, raw unsigned masks, optional GUI focus, source timestamp, local read time, local change sequence, source quality, context, session generation and pending commands. Keep `ObservedState`, `DesiredRequest` and input-delivery result separate. A match means the requested state was observed; it cannot prove which input caused it.

## 5. Game telemetry

Frontier documents `Status.json` as a replaced snapshot alongside the incremental journals, normally under the Windows Saved Games directory at `Frontier Developments\Elite Dangerous`. Resolve the Saved Games known folder rather than constructing a path from the username; provide a folder picker for redirected or unusual installations. Read files only. [Frontier Journal Manual v32, location and §14](https://hosting.zaonce.net/community/journal/v32/Journal_Manual-v32.pdf).

### Relevant masks

This subset is verified against the official v32 manual and cross-checked against the maintained [EDMarketConnector flag constants](https://github.com/EDCD/EDMarketConnector/blob/main/edmc_data.py#L390-L453). It is a baseline, not a claim that the 2021 manual describes every current game field. Decode `Flags` and `Flags2` independently as unsigned masks. Preserve unknown bits and tolerate added fields.

| Field | Bit | Decimal mask | Meaning |
|---|---:|---:|---|
| Flags | 0 | 1 | Docked |
| Flags | 1 | 2 | Landed |
| Flags | 2 | 4 | Gear down |
| Flags | 3 | 8 | Shields up |
| Flags | 4 | 16 | Supercruise |
| Flags | 5 | 32 | Flight assist **off** |
| Flags | 6 | 64 | Hardpoints deployed |
| Flags | 8 | 256 | Lights on |
| Flags | 9 | 512 | Scoop deployed |
| Flags | 10 | 1024 | Silent running |
| Flags | 16 | 65536 | FSD mass locked |
| Flags | 17 | 131072 | FSD charging |
| Flags | 18 | 262144 | FSD cooldown |
| Flags | 19 | 524288 | Low fuel |
| Flags | 20 | 1048576 | Overheating |
| Flags | 24 | 16777216 | Main ship |
| Flags | 25 | 33554432 | Fighter |
| Flags | 26 | 67108864 | SRV |
| Flags | 28 | 268435456 | Night vision |
| Flags | 30 | 1073741824 | FSD jump |
| Flags2 | 0 | 1 | On foot |
| Flags2 | 1 | 2 | Taxi |
| Flags2 | 2 | 4 | Multicrew |
| Flags2 | 12 | 4096 | Glide |

`GuiFocus == 0` means no selected GUI panel; it does **not** prove chat, pause or every menu is closed. Flags are telemetry, not a comprehensive permission system. Missing required fields mean unknown. Missing `Flags2` does not mean on-foot is false; allow a documented older-game profile only after verifying its capabilities. MVP live control targets the user's current main-ship client with the required fields available.

Use `ulong` and `(flags & mask) != 0`; do not sign-extend a 32-bit mask. Optional future flight-assist display must invert bit 5 when labelled “assist on.” Do not derive hardpoint or scoop position from the last click. Do not add newer SCO/SCA masks from memory: verify a current authoritative definition and a real fixture before using them.

### Reader behaviour

1. Read an initial snapshot, watch the **directory** for creation/change/rename and also poll every 500 ms. Coalesce bursts. The watcher is a hint, not the sole source of updates.
2. Open with sharing compatible with the writer, replacement and deletion. Read the entire small file; validate `event == "Status"`, timestamp and required numeric fields. Cap file size, e.g. 256 KiB.
3. On empty, partial, locked or malformed content, retry after 50/100/200 ms. Keep the last good display briefly, mark degraded quality, and block live commands until a valid read succeeds. Never convert a parse failure to all flags false.
4. Reject older timestamps within a session; treat a backward clock/session change as a resynchronization requiring disarm. Duplicate content is not a new acknowledgement.
5. Keep file/read health separate from snapshot age. Polling an unchanged old file does not refresh the timestamp. Status can remain unchanged during quiet play, so age alone cannot prove the game has stopped.
6. Adopt a conservative configurable snapshot-age control limit of 60 seconds initially. Older data remains visible with its age but control is blocked. Do not promise a game heartbeat or auto-clear stale state just because the process exists. Tune the limit using real play observations; false blocks are preferable to silent stale-state toggles.
7. Maintain a minimal journal session guard for `LoadGame`, `Shutdown` and death/session transitions. Follow the newest journal, handle rotation and partial final lines. Bootstrap from bounded recent records; do not replay the entire history as live events. A historical `LoadGame` alone cannot establish a current session. Process exit, clean shutdown, ambiguous session or context change cancels pending requests and requires rearming.

The user confirms they are in the live cockpit when arming. Combine that with process/window identity, acceptable telemetry and journal session information. If these cannot establish a usable session, remain in observation mode. This is best-effort protection, not a guarantee that all game UI states are detectable.

## 6. Input safety and synchronization

### Modes and hard gates

- **Mock:** simulated telemetry, simulated effects, no OS input adapter instantiated.
- **Observe:** real telemetry, dry-run command sink, no keyboard injection.
- **Control:** real telemetry and Win32 sink; starts disarmed on every launch and mode change.

Switching sources clears pending commands and disarms. Never replay clicks made while disarmed, unfocused, disconnected or in another mode. Settings and binding capture automatically disarm.

Allow a live command only if armed, the selected game process/window is foreground, the session is usable, required state is known and sufficiently recent, no conflicting command is pending, and the action is permitted in the current context. Identify the game by validated executable path/process ID and HWND; do not rely on a matching title or the launcher. Refresh identity after process restart.

Initial policy: controls work in main ship, outside GUI panels, taxi, multicrew, on-foot, SRV, fighter and transition contexts. Block gear/scoop/hardpoints while docked, landed, in supercruise, glide, charging or jumping. Lights/night vision may be permitted while docked or landed when all other gates pass. Gate consumables to undocked/unlanded normal flight. Maps/fire-group buttons use the same conservative cockpit policy initially. Write this as an action-policy table in code and docs rather than scattering conditions across handlers.

### Maintained controls backed by toggle bindings

Use this algorithm for T1–T4; there is no generic “set gear ON” game API here:

1. An explicit user movement specifies a desired state. Read the newest usable observed state.
2. If already equal, send nothing and return already satisfied.
3. If unknown, stale, blocked or pending, reject immediately with a reason. Do not queue it for later.
4. Otherwise capture baseline sample/session identifiers, create one pending request and send **one** configured toggle pulse.
5. Show pending separately from the unchanged observed LED. Ignore further requests for that action until resolved; disable its control visibly.
6. Resolve as observed satisfied only after a genuinely newer valid snapshot in the same session reports the desired state. Fresh reads of the old snapshot do not count.
7. Default timeout: 5 seconds, configurable after testing. On timeout, clear pending, retain the actual display, and show “No matching state observed.” **No automatic retry.** A late update may refresh the display but must not revive the request or cause another input.
8. Keyboard/HOTAS changes update observed positions immediately. Do not run a continuous loop that fights external changes or restores a saved virtual position.

Startup, reconnect, rescan and rearming are observational: they never send toggles. “Resync” means reread current state and clear pending requests. For a future physical maintained switch that disagrees with the game, show mismatch and require a new deliberate switch transition; do not enforce its boot position automatically.

### Keyboard delivery

Use Win32 `SendInput` with scan-code key events and correct extended-key flags, explicit down/up pairs and reverse-order modifier release. Validate the returned input count. Record insertion as delivery, never proof of game execution. Windows restricts injection by integrity level and existing held keys can affect synthetic input. Run game and panel as ordinary user processes; surface privilege mismatch rather than defaulting to administrator. [Microsoft SendInput](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-sendinput).

Begin with single non-modifier bindings; support at most one modifier after validation. Reject Windows/Alt/system shortcuts and duplicate mappings. Before sending, detect physically held conflicting modifiers or the mapped key and reject; never release keys owned by the user. Track only app-generated key downs. Use a short configurable press interval, initially 60 ms, verified in-game. Always attempt paired releases in `finally`, on disarm and orderly shutdown. Cancellation prevents new downs but must still allow cleanup of already-injected keys.

Recheck foreground immediately before injection and between stages. Focus loss cancels pending commands and disarms. Attempt cleanup for app-owned keys, log failures and show a clear error. A process crash cannot run cleanup; prefer single keys and keep pulse intervals short. Global keyboard input has an unavoidable focus-change race; never claim perfect targeting. Do not use background `PostMessage` key events as a fallback.

Serialize pulses globally, reject while busy rather than build a backlog, and apply a 250 ms per-action cooldown. Each click means at most one pulse; held mouse buttons and keyboard repeat do not repeat commands. Consumables have no automatic retry or persistent on state. Keep them unmapped until the user deliberately configures them. Exclude self-destruct, jettison-all and automated sequences from the MVP.

## 7. Second-monitor and Windows concerns

**Focus handling is an early feasibility gate.** A normal app window on monitor two can take keyboard focus away from the game when clicked. A browser-only prototype does not solve this native input problem.

Implement two distinct windows/modes:

- A normal configuration window that can accept keyboard input, always disarmed.
- A Run panel using `WS_EX_NOACTIVATE`, `ShowActivated=false`, and appropriate `WM_MOUSEACTIVATE` handling (`MA_NOACTIVATE`, not click-eating). Prevent controls from activating the window while preserving pointer events. Verify actual HWND behaviour; the WPF setting alone is insufficient. Microsoft documents the non-activating window style. [Extended window styles](https://learn.microsoft.com/en-us/windows/win32/winmsg/extended-window-styles).

Do not forcibly grab game focus. `SetForegroundWindow` has restrictions and is not guaranteed to succeed. If the run panel steals focus, block input and tell the user to return to the game; an intentional request must be made again. [SetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setforegroundwindow).

Test borderless/windowed Elite first. Exclusive fullscreen and pointer confinement may make the second monitor unusable without leaving the game; record this limitation. Touch may activate differently and remains experimental until tested. A mouse click can also expose cursor/camera problems even with correct foreground handling: test those before polishing the panel.

Other requirements:

- Per-monitor DPI awareness; test mixed 100%/150% scaling, negative monitor coordinates and monitor unplugging. Restore saved bounds onto a visible work area.
- Optional always-on-top, off by default; app must not cover critical game UI unexpectedly.
- Keep disk IO, polling and pulse timing off the UI thread. Marshal state changes to the dispatcher.
- Use validated, versioned JSON settings under `%LOCALAPPDATA%\EliteAuxPanel`; atomic saves, backup and reset. Save bindings/window position, never armed state or pending requests.
- Maintain a bounded rolling log of app decisions, source health and input counts. Do not record arbitrary keyboard input, chat or full commander journals by default.
- Single instance prevents double injection. Disarm on session lock/sleep and require manual rearm after resume.
- Provide a visible Disarm control plus an optional registered global disarm shortcut; report registration conflicts. Its purpose is to stop new input and cancel pending work, not toggle game systems off.
- Framework-dependent development build; self-contained portable release folder for the user. No service, driver or installer required. Document unsigned-app prompts without asking users to disable Windows protections.

## 8. Mock mode and reproducible fixtures

The first executable must launch directly into Mock mode with a visible banner. A deterministic simulator owns its own state and clock. The fake input sink reports accepted/rejected delivery; scheduled mock snapshots provide feedback. It must never touch the real game folder or Win32 injection path.

Provide scenarios for normal flight, docked, supercruise, gear/scoop/hardpoints changes, low fuel/heat badges, external HOTAS changes, delayed acknowledgement, no acknowledgement, late acknowledgement, focus loss, stale data, malformed/partial files, source disconnect, restart and unknown context. Allow choosing acknowledgement delay and failure. The UI must distinguish simulated telemetry from real telemetry at all times.

Minimal synthetic fixtures (substitute a current timestamp at runtime):

```json
{"timestamp":"2026-10-23T07:00:00Z","event":"Status","Flags":16777224,"Flags2":0,"GuiFocus":0}
```

This represents main-ship normal flight with shields up. Add 4 for gear, 512 for scoop, 256 for lights and 64 for hardpoints. For a separate fixture, set `Flags` to `2147483648` to catch signed-mask errors; that snapshot is not valid main-ship flight. Cover absent fields and unknown high bits independently.

## 9. Phased roadmap and Codex prompts

Implement one phase at a time. After each phase, update the README, summarize files changed, report automated checks and manual checks separately, and identify any untested real-game claims. Do not mark a live acceptance check passed using only a mock. Stop to report an actual blocker; otherwise continue within the requested phase.

### Phase 0 — Establish the repo and focus feasibility

**Prompt to Codex:**

> Read this project plan as the implementation specification. Inspect the workspace and any applicable repository instructions. Create the .NET 10 WPF solution and independent core project. Build a minimal ordinary settings window plus a non-activating run panel with a mock button and foreground diagnostics. Default to Mock; do not implement real keyboard injection yet. Verify build/test commands and document how to launch. Prioritize proving that mouse clicks reach the panel without changing the foreground HWND. Record a real-game borderless-mode test as pending if Elite is unavailable. Do not introduce hardware dependencies.

**Acceptance:** solution builds; mock click is logged once; settings disarms; foreground HWND is visible in diagnostics; non-activation works against a benign foreground test window. Real Elite click/focus/cursor behaviour is an explicit user test gate before Phase 3.

### Phase 1 — Complete the mock panel and command core

**Prompt to Codex:**

> Implement the configurable 4-toggle/6-button/4-lamp panel, normalized state, action-policy table, deterministic mock source, dry-run sink and serialized command coordinator. Add the desired-versus-observed reconciliation algorithm, 5-second timeout, no retries, no backlog and explicit delivery outcomes. Provide scenario controls for delayed, failed and external state changes. Test meaningful policy and race cases with an injected clock. Do not instantiate a native input sink in Mock.

**Acceptance:** exact 4/6/4 layout; all mappings visible; actual lamps follow simulator only; already-satisfied requests produce zero pulses; rapid clicks do not duplicate; one unresolved request blocks another for that action; timeout never retries; external changes never trigger corrective input; mode switch clears requests; 30-minute mock session stays responsive.

### Phase 2 — Live telemetry, read-only

**Prompt to Codex:**

> Add the read-only Status.json source, Saved Games discovery and folder selection, directory watching with polling fallback, bounded retries and unsigned flag decoding. Add the minimal journal session guard with rotation handling. Expose source quality, snapshot age and context. Keep Observe mode incapable of OS input. Add integration checks using temporary fixture directories, replacement writes, partial writes, deletion and backward timestamps. Document the binding setup without modifying any game files.

**Acceptance:** game gear/scoop/lights/hardpoints changes appear within 1 second of a successfully written readable snapshot under normal conditions; this is app latency, not a promise about game write frequency. Invalid data does not flash false lamps; unknown fields do not crash; stale data blocks control eligibility; file recovery sends nothing; a leftover file after game exit cannot establish an active control session. Observe mode has zero native input calls.

### Phase 3 — Guarded real keyboard control

**Prompt to Codex:**

> Implement the isolated scan-code SendInput adapter, validated binding editor, exact game-window identity checks, arm/disarm state and all command gates. Keep mappings unset by default. First validate insertion and cleanup with a dedicated benign test receiver through a test-only adapter configuration; production must allow only the selected game process. Test focus loss, held modifiers, partial SendInput results, shutdown and session changes using fakes. Never inject into an unrelated foreground window. Do not auto-focus the game or retry failed toggles. Document the required in-game manual tests; do not claim they ran unless they did.

**Acceptance:** disarmed/startup/reconnect/mode switch sends nothing; clicking a normal settings window blocks input; verified run-panel clicks retain Elite focus; one permitted click generates one complete pulse; HOTAS changes update lamps; pending actual state remains honest; timeout and loss of focus do not retry; input failure is visible. Demonstrate gear/scoop/lights/hardpoints individually in a safe game setting. Validate consumables only after deliberate user mapping and an appropriate flight test.

### Phase 4 — Package and evaluate the software MVP

**Prompt to Codex:**

> Complete validated settings persistence, monitor restoration, DPI behaviour, single-instance protection, bounded logs, lock/resume disarming and portable publishing. Write README, BINDINGS.md and a concise manual-test checklist. Run build/tests and launch the release build on this Windows machine if possible. Report actual results and remaining user tests. Do not implement serial or firmware yet.

**Acceptance:** portable app launches without development tools when packaged self-contained; no hardware or game required for Mock; settings survive restart but arming does not; unplugging monitor two leaves an accessible window; second instance cannot send input; lock/resume requires rearm. Complete at least one 30-minute real-game session with no duplicate pulses, stuck app-owned keys or unexplained toggles. Any failure of focus retention or feedback honesty blocks the “ready for hardware” decision.

### Phase 5 — Optional ESP32 bridge after MVP approval

**Prompt to Codex:**

> Implement the hardware migration described below only after the software MVP is accepted and the exact board is identified. Add a USB-serial transport behind the existing input and LED contracts, protocol tests and a no-device simulator first. Preserve all app gates and startup synchronization behaviour. Then create firmware for the confirmed board and pin map. Do not add native USB HID, Raspberry Pi services or alternative input paths by assumption.

**Acceptance:** serial simulator exercises reconnects, duplicate events and malformed frames; no startup switch snapshot causes a game action; physical actions pass the same policy as UI; LEDs track game telemetry even when the HOTAS changes it; USB disconnect never replays presses; firmware fails visibly to unknown/link-lost state when the host disappears.

## 10. Later ESP32 / USB-serial migration

### Roles

Keep the Windows app as the game bridge: it reads telemetry, decides whether a request is allowed, sends keyboard input and transmits LED state. The ESP32 scans switches/buttons, debounces them and drives LEDs. A standard ESP32 can use a board's USB-to-UART connection; identify the actual bridge chip and driver when the board is confirmed. [Espressif serial connection guide](https://docs.espressif.com/projects/esp-idf/en/stable/esp32/get-started/establish-serial-connection.html).

Do not assume an ordinary ESP32 USB connector can enumerate as a native HID keyboard. USB device capabilities depend on the chip; Espressif documents HID-capable USB device stacks for variants such as ESP32-S3. This design does not require that upgrade. [Espressif USB device stack](https://docs.espressif.com/projects/esp-idf/en/release-v5.4/esp32s3/api-reference/peripherals/usb_device.html).

**The ESP32 cannot independently know the game's actual LED states through its buttons alone.** It needs telemetry returned by the Windows host. USB-serial supplies both directions. The Raspberry Pi 2 adds no required role for this design and stays out of the data path.

### Proposed protocol v1

Use newline-delimited UTF-8 JSON over 115200 baud, 8N1, maximum 1 KiB per frame. Negotiate version and capabilities. Example frames are proposals, not existing firmware compatibility:

```json
{"v":1,"type":"hello","bootId":"abc123","toggles":4,"buttons":6,"leds":4}
{"v":1,"type":"snapshot","bootId":"abc123","seq":1,"toggles":[false,true,false,false]}
{"v":1,"type":"input","bootId":"abc123","seq":2,"kind":"toggle","id":"T1","value":true}
{"v":1,"type":"input","bootId":"abc123","seq":3,"kind":"button","id":"B1","edge":"press"}
{"v":1,"type":"ledState","session":"host42","seq":7,"quality":"valid","values":[true,false,true,false]}
```

- Device sends an initial switch snapshot for display/mismatch detection only. Subsequent debounced transitions become intents. Button release does not send another command. Start with 25 ms debounce and tune using the physical switches.
- Deduplicate events by boot ID and sequence. A new boot ID starts a fresh device session. Reject invalid IDs, wrong versions, out-of-range values and oversized/unfinished frames.
- Do not retry input events across reconnects. A receipt ACK, if added, means transport receipt, not game success. LED snapshots are idempotent and may be resent.
- Host sends full LED state when telemetry changes and periodically, e.g. every second. Include valid/unknown/link quality; do not encode unknown as ordinary off.
- Implement a host/device heartbeat and firmware timeout, e.g. 3 seconds. On loss of host, use a documented distinct blink pattern for unavailable feedback; this is a link indicator, not a game-state claim. Offer brightness limits.
- USB disconnect cancels hardware-origin pending requests and requires rearm after handshake and observation. Never flush buffered presses into the game. Opening a COM port can reset some boards through control lines; tolerate reboot and never turn its initial snapshot into actions.
- Store optional COM selection and device identity; reconnect with bounded backoff. Do not send protocol messages to every available COM device. The user selects the device initially.
- Physical maintained positions can differ from game states. Show mismatch in the app; require a deliberate transition or an explicit per-control request. A generic “sync” button must not blindly toggle four systems.

Before wiring, confirm exact board/module, bridge, switch topology and LED type. Select safe GPIOs after checking boot-strapping/input-only restrictions and board schematics. Use suitable LED resistors/drivers and 3.3 V logic. Do not power arbitrary panel lamps directly from GPIO. These electrical details are a separate implementation gate, not a requirement for the software MVP.

## 11. Verification matrix

| Case | Required outcome |
|---|---|
| Game absent | Mock runs; Observe reports unavailable; Control cannot arm |
| Desired state already observed | Zero pulses |
| Gear click, delayed update | One pulse, pending request, lamp changes only on telemetry |
| No matching update | Timeout message; no repeat |
| Late matching update | Display updates; no resurrected command |
| HOTAS changes a toggle | Panel follows actual state; zero corrective pulses |
| Old identical snapshot reread | No acknowledgement; age does not reset |
| Partial or replaced status file | No crash, no false off display, commands blocked while degraded |
| Missing Flags2 or GUI focus | Unknown context; conservative live block |
| On foot/SRV/fighter/taxi/menu | Ship controls blocked with reason |
| Focus lost during pulse | Stop new downs, attempt app-owned releases, disarm |
| Alt/Ctrl already physically held | Reject conflicting injection; do not release user keys |
| Repeated/held click | At most one pulse per intentional accepted activation |
| Settings opened / source changed | Disarm, cancel pending, no deferred replay |
| Sleep/resume/process restart | Disarm; re-establish identity and telemetry |
| High bit set | Unsigned decoding remains correct |
| ESP32 reconnect (later) | Observe initial positions; no keyboard action |

Use unit tests for decoding, state-machine decisions and timing; adapter fakes for failures and focus changes; temporary-directory integration tests for file behaviour; manual checks for real Win32 focus and Elite input consumption. Never make game input injection part of unattended tests on the user's desktop.

## 12. Definition of done and handoff

The software-first proof of concept is complete when Phases 0–4 pass, the release is usable on the second monitor, all controls are configurable, telemetry drives the lamps, the documented focus limitations are understood, and the user has tested the panel in their actual game/display configuration. Hardware work starts only after that evaluation.

Required implementation deliverables: source, portable release folder, README with prerequisites/run/build/publish steps, game-binding setup guide, tests and synthetic fixtures, manual test results with unrun cases labelled, known limitations, and the proposed serial protocol document. Hardware firmware is not an MVP deliverable.

### First message to paste into Codex

```text
Use the attached Elite-Dangerous-Button-Box-Project-Plan.md as the project specification.
Implement Phase 0 first, then report its acceptance results and any real-game tests
that need me to perform them. Use C#/.NET 10 WPF and preserve the hardware-independent
Mock mode. Keep the 4-toggle/6-button/4-LED layout and proposed configurable mappings.
Do not introduce real keyboard injection before the guarded-input phase, and do not
start ESP32 firmware or serial integration until the software MVP is accepted.
Inspect existing project instructions and files before creating or changing code.
Make the implementation runnable, keep documentation current, and distinguish
automated evidence from tests that require my game and monitor setup.
```

### Research limits

Sources were checked on 23 October 2026. The official Journal Manual located for this plan is version 32, covering July 2021. Its longstanding masks suffice for the proposed panel, but new fields and actual game behaviour must be checked during implementation. Focus retention, synthetic key acceptance, status update cadence, context coverage and touchscreen behaviour require real-machine testing. This plan does not claim that those integration tests have already passed.
