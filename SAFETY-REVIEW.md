## Co-pit desktop addition — 25 October 2026

The review below describes the terminal baseline. The Wails build adds a WebView2 desktop frontend and therefore has runtime-owned browser helper processes. The terminal statement about no child processes does not apply to those WebView2 helpers. No standalone browser, HTTP development server, background service or automatic restart is added to the packaged app.

The desktop backend reuses the guarded Windows controls and journal reader. Each session owns its cancellation context and joins both the control worker and process/journal worker before mode replacement. Callbacks reject stale sessions. Backend failure cancels companion polling and marks controls disarmed. Wails shutdown cancels and joins the current session; close exits instead of hiding to a tray. Single-instance locking prevents two Co-pit windows from competing for shortcuts. The terminal app remains a separate executable and can still conflict if run concurrently.

Display preferences never save mock mode or armed state. An unwritable preference file cannot prevent leaving fullscreen. All game and journal text is assigned with DOM `textContent`; no game-provided HTML or scripts are executed. The frontend uses embedded local assets and restricts resource loading with a content security policy.

Native checks verified F11 fullscreen across the current monitor, Esc restoration to the preceding window size, automatic portrait reflow at 720 × 1100, and ordinary close with no remaining Co-pit process or direct WebView2 children. Forced termination of the new WebView2 runtime has not been verified; the terminal baseline's forced-termination checks only verify its own backend and hotkeys. WebView2 helper lifetime after a crash remains an integration check.

# Shutdown and input safety review — 24 October 2026

Reviewed the Windows amd64 application, terminal session runner, real-game control backend, keyboard pulse delivery, local file readers, preference writes and application launchers. This is a code review plus targeted tests, not a guarantee against every possible OS or hardware failure.

## Findings fixed

| Finding | Change |
|---|---|
| Bubble Tea deliberately does not wait for command goroutines; the app waited only for its main backend. | Session-owned work now cancels timers and joins journal reads, process probes and the backend before returning or switching modes. Commands that start after shutdown are rejected. A three-second cleanup failure is reported and prevents another session starting. |
| An unexpected backend panic could terminate the app; broad handling of `ErrProgramKilled` could hide a UI panic or other UI failure. | Backend panics unwind the existing resource/key-release defers and become visible errors. Backend failure cancels companion polling too. Unexpected UI errors now propagate to the caller instead of being treated as an ordinary quit. |
| Only Ctrl+C was subscribed; Windows console-close notifications use SIGTERM when delivered to Go. | The main context now subscribes to both interrupt and SIGTERM. OS termination still has limits; see below. |
| Classic mock text input could remain blocked in a read, or leave its scanner sending after Quit. | Mock input has a session context, cancellation and a joined scanner. Windows console input polls available events so a partial line cannot prevent cancellation. Redirected file/pipe input uses an owned duplicate; cancelling does not close the caller's stdin handle. |
| Shutdown could be delayed while draining queued hotkeys, and a delayed arm shortcut was not age-checked. | Cancellation is checked inside the queue loop and before arming. Arm/page/action messages older than 300 ms are discarded. Flight messages predating the current arming session are discarded. Emergency stop remains accepted even when delayed. |
| A closed UI request channel could cause a busy loop. | A closed request channel is disabled while the backend continues observing cancellation and telemetry. |
| Journal record size was checked after allocating the whole line; route/profile files could grow beyond their size check during the read. | Journal records are bounded during accumulation, oversized records are discarded without retaining their contents, and cancellation is checked per chunk. Route, binding and active-preset reads are bounded. |
| A panic during a preference write could leave the temporary file open. | Temporary-file cleanup defers closing before removing, including during panic unwinding. Existing saved preferences are replaced only after encoding, syncing and closing the temporary file succeeds. |
| Every application launcher waited at `pause` after the EXE exited. | Application launchers now return when the EXE ends. An existing user terminal stays open normally; double-click launchers no longer wait for another keypress after exit. |

## Existing protections retained

- The terminal core creates no owned child processes, network listener, background service, scheduled task or auto-restart. Default game data comes from local files. Explicit browser actions and optional EDSM HTTPS lookups are described below; Wails additionally owns WebView2 helpers. The subprocesses in the lifecycle tests belong only to the test harness and are explicitly waited for or killed.
- Each normal control session starts disarmed. Windows hotkeys are thread-owned and explicitly unregistered on return, error and panic unwind, including partial registration failure. Tickers and process/file handles have cleanup paths.
- Flight input requires an armed backend, the expected foreground game process/window, current main-ship telemetry, valid game bindings and a permitted game context. Temporary focus loss pauses input for the authorised game session. Explicit Stop, faults, session replacement and shutdown cancel automatic resume; see the update below.
- Each accepted synthetic key-down is paired with a deferred key-up on normal completion, cancellation, focus loss and panic unwinding. Output keys and modifiers already held by the user block delivery. Key-release failures are preserved even when another error occurred first. No automatic action retry was added.
- Page browsing, saved reference grids and mock controls do not send flight input. Theme, material-goal and fire-group preferences are separate from game telemetry and game control files.

## Verification

- All **88 standard tests** pass with `go test ./...`; `go vet ./...` passes. This includes four boot-animation tests added after the original shutdown review; the boot timer uses the same session-owned cancellation/joining mechanism.
- Added tests cover task cancellation/joining, rejection of late work, panic cleanup/error reporting, real TUI session Quit, blocked mock input, cancellation before file/input actions, stale hotkeys and counter wraparound, key-release error reporting, bounded journal records and Windows pipe-handle ownership.
- The concurrent worker/input shutdown tests also passed 20 repetitions.
- The opt-in desktop tests passed for **normal cancellation, an injected backend panic, forced termination of a disarmed test subprocess, and partial hotkey-registration failure**. Arm/stop/page hotkeys could be registered again after process exit. The existing global page-key test passed too. These tests never armed the controller or called SendInput.
- The rebuilt EXE opened in real control mode, remained disarmed and exited with Q. Classic mock mode accepted a command and printed `Stopped.` after Ctrl+C during an unfinished line. No test copy remained running afterward.
- The race detector was unavailable: this environment has `CGO_ENABLED=0` and no C compiler. Passing these tests does not substitute for a race-detector run.

Desktop checks can be repeated with the controller closed:

```powershell
$env:ELITEAUX_LIFECYCLE_TEST = '1'
$env:ELITEAUX_PAGE_KEYS_TEST = '1'
go test -run '^(TestWindowsControlCleanupAfterExitPanicAndForcedTermination|TestControlGlobalNumpadPageKeys)$' -v .
```

## Remaining limits

Windows terminates the process's threads and releases its handles/resources after a forced process termination. This app has no production child processes to outlive it. Microsoft documents that forced termination does **not** allow cleanup code to run: [Terminating a Process](https://learn.microsoft.com/en-us/windows/win32/procthread/terminating-a-process).

The synthetic key pulse lasts 60 ms to let Elite detect it reliably. A forced kill, power loss or unrecoverable runtime failure during that interval can prevent the matching key-up from being sent. Ordinary cancellation and focus loss execute that release; abrupt termination cannot be made equivalent without changing the delivery design. The app also cannot undo an action the game already accepted before shutdown.

Console-close/logoff/shutdown cleanup notifications are best effort and subject to Windows behavior and time limits. A forced exit can leave a small, incomplete temporary preference file, but it does not represent a running process or connection. Paths explicitly configured to point to network shares are managed by Windows' filesystem layer; the optional, user-requested EDSM network path is described below.


## Focus resume and usability update — 25 October 2026

Both applications share the flight-first page manifest and focus-pause engine. A temporary focus change releases all flight-action hotkeys and removes pending toggle confirmation; only the manually armed game identity is kept in memory. Process creation time plus PID/path/window identity prevents a restarted or reused PID inheriting permission. Background process liveness is checked while paused. No arming permission is persisted, so closing/crashing either app or the game requires a fresh manual arm.

Stop and manual disarm clear active and paused permission. Telemetry read errors, stale/backward clocks, unavailable ship context, transitions, binding changes, delivery errors, and shortcut-registration errors also cancel permission. Automatic resume waits for released numpad/output/modifier keys, re-registers shortcuts, and resets the hotkey epoch to reject older messages. All output still requires Elite to be foreground at delivery.

Co-pit has clickable control buttons. A click requires existing manual arming/paused permission, is accepted only for a known action, expires after 300 ms (checked again after focusing), restores only the authorised live game window, and passes through the same telemetry/profile/context/cooldown/delivery checks. Stop replaces waiting UI clicks. Failed/expired presses are never retried. Glass indicators follow telemetry. Simulation buttons and engineering edits never affect the real game or saved real targets.

Unit tests cover pause/resume without replay, explicit stops and all fault/restart cases, process creation identity, held-queue replacement by Stop, tab traversal, ordinary engineering search/editing, and mock preference isolation. Windows graceful/panic/forced termination and partial shortcut-registration cleanup checks passed again. Native visual checks used only explicit simulation and confirmed button LEDs and the engineering form. Live gameplay focus-resume and click-to-game input remain integration checks, not claimed as verified gameplay.

## Optional station-search browser action — 25 October 2026

The user-invoked Navigation shortcut opens a fixed, allowlisted HTTPS Inara search tool in the default browser. It does not scrape the site, run an HTTP client/server, poll online data, send journal/commander information, or create a hidden owned helper. Windows uses ShellExecuteW with a literal URL rather than a command string. Co-pit uses Wails' browser-open runtime API. The browser is a separate visible application deliberately opened by the user, so it remains open when the companion exits.

The terminal launch command is session-owned and cannot begin after cancellation/shutdown. Unsupported addresses and categories are rejected. Tests validate the category URL allowlist, menu bounds/stop visibility in all themes, absence of flight input when browsing the picker, and cancellation before launch. Tests never open an external browser. Exact live mission offers are not represented as available station data; prices/stock are explicitly labelled community-reported and potentially old.

## On-demand outfitting and module-warning update — 25 October 2026

The built-in outfitting viewer adds a Go HTTP client for the fixed EDSM HTTPS host. It sends only an explicitly entered/selected system or station identifier, not commander credentials, journal contents or flight controls. Requests are serialised and paced, have bounded dial/TLS/header/overall timeouts, reject redirects, and cap response size and record counts. Idle keep-alive reuse is disabled. Bodies are closed; cache data is validated, memory-only, bounded and expires after ten minutes. No retry, listener or periodic online poll is added.

Request generations reject obsolete replies. View cancellation, page departure, mode replacement, fatal backend errors and shutdown cancel active or waiting requests; desktop work is joined through the session owner. Tests cover cancellation, close/mode/shutdown handling, cache/error bounds, response identity and no late UI reopening. The opt-in real EDSM check previously loaded Sol stations and Abraham Lincoln modules without arming controls. Abrupt OS process termination closes process-owned sockets; this is distinct from best-effort graceful UI cleanup.

Module warnings use last-reported journal snapshots and never queue controls, modify telemetry or infer current malfunction. Invalid/missing integrity remains unknown, even on a key system. Key-system damage is highlighted at any damage level as a conservative presentation choice; no unsupported malfunction threshold is asserted. Decorative screen frames and optional static scan lines ignore pointer input and add no timers, network calls or child processes.


## Bold Lens controls — 25 October 2026

The approved glass controls change only frontend presentation. Component labels remain above the buttons, bindings stay on the glass, and native accessible button states remain available. Illumination comes only from the existing backend's ACTIVE state. Clicks do not optimistically illuminate glass. UNKNOWN has a visible label and dashed lens; momentary map/fire-group/consumable controls remain actions without a persistent pressed state. Disabled controls still display their last reported state but cannot dispatch clicks.

Icons are a ten-icon Lucide 1.8.0 subset embedded as local SVG node definitions, with the ISC licence retained in source and the packaged licences folder. No remote icon loader, network request, timer, new process or stored arming state is introduced. Decals and etched brackets are static, pointer-transparent decoration. Existing input gating, focus resume, stop handling and shutdown ownership are unchanged.


## Compact square layout preference — 25 October 2026

ControlColumns is a display-only integer from 0 through 10 (0 = Automatic). Invalid counts are rejected before writing settings; older version-1 settings without it remain compatible. Changing this value neither queues control requests nor alters arming. A frontend ResizeObserver adjusts the visible column count to available panel width, without overwriting the saved choice; it disconnects on pagehide. Buttons and lenses remain fixed 112 px and square respectively. Labels, accessibility state, explicit unknown/action notes and telemetry-driven illumination are retained.


## Ship-aware custom cockpit — 25 October 2026

Layouts always include the eight permanent built-in controls and persist only versioned, allowlisted action IDs in cockpit-layout.json. Duplicate/unknown actions, invalid schema, oversized files and failed atomic writes do not replace valid in-memory assignments. Read failures show an error and fall back to the permanent controls. Caller/snapshot slices are copied. Closing rejects saves. No action keys, command strings, macros, game bindings, telemetry or arming permissions are saved in this file.

The Add checklist filters the supported catalogue using the reported ship and launcher membership in its last Loadout. Save rechecks newly introduced actions under the model lock, so a ship change between selection and saving cannot add an unavailable utility. Existing assignments are retained across swaps. Confirmed missing launchers disable UI clicks and are also rejected by PressControl; unknown ship/module availability stays unknown. Built-in actions still require the guarded flight backend to accept any request. The fixed numpad action map is unchanged.

Unavailable tiles pulse dim red via CSS only; reduced-motion preference disables the animation. State polling updates catalogue availability without rewriting layout. The optional automatic-add preference saves newly available supported utilities only on journal updates or an explicit preference save, retaining existing assignments and suppressing repeated attempts after a write failure. Native dialog keyboard handling prevents cockpit shortcuts firing while picking controls, and the dialog keeps STOP available. The checklist and layout editor dispatch only saved-layout operations. No additional network calls, timers, owned processes, auto retries or input loops were introduced.

## Session-only window mode — 25 October 2026

Fullscreen/borderless fullscreen is now runtime-only. Loading older preferences ignores their fullscreen value, new desktop instances clear inherited runtime mode, and Wails always starts in Normal window state. The current fullscreen flag still reaches the frontend so F11, Esc and Settings remain accurate within a session.

Appearance saves use a dedicated stored structure containing only version, theme, layout, scan lines/spacing, control-column count and automatic ship-control preference. Fullscreen/borderless flags are omitted. Toggling fullscreen performs no preference I/O, so an unwritable profile cannot prevent returning to a window. The existing custom window chrome remains the same. Ship-control assignments are stored separately and are unaffected.


## Graphical pages and permanent controls — 25 October 2026

Automatic adding only stores supported action IDs after a validated ship loadout; it never arms the app or dispatches flight input. Permanent controls remain subject to the same telemetry/focus/input gates. No worker, timer, network poll or child process was added. Existing unavailable utility tiles remain disabled and pulse dim red.

Desktop pages use structured data rendered through textContent and native form controls. Displayed journal strings remain sanitised; route tables are capped at 100 entries. Fire-group dropdowns validate the current ship, module, group and assignment under the existing mutex, reject closing/load-error states, and only save the manual local reference. They cannot change the game's bindings or trigger weapons. Scan-line spacing is validated before persistence; decoration is static and pointer-transparent. Full-height layouts only change scrolling and sizing.


## Per-material comments — 25 October 2026

Engineering uses user-saved comments consistently for every material; no blueprint dataset or online recipe lookup was added. Comments are plain text, bounded to 2,000 Unicode characters and a versioned 1 MiB local file with at most 1,000 material keys. The backend validates canonical key syntax, selected-list membership, UTF-8 and control characters, normalises CRLF, and rejects mode-mismatched or closing saves. Saves clone the current map and atomically replace the file before updating memory. Corrupt/oversized files show an error and block real saves rather than silently replacing the original.

Simulator notes live in a separate in-memory map. The bridge exposes the selected material's comment and saved comments for visible list rows, as requested by the Comment column; stored maps are never exposed by reference. Frontend textareas/textContent prevent HTML execution; drafts are keyed by material and real/simulator context, survive refresh/focus/navigation, and clear only after the matching successful save. In-flight newer edits and failed saves retain their drafts. No arming, bindings, targets, flight commands, telemetry, network calls, timers or child processes are changed by note operations. Unsaved drafts are session-only.


## Explicit material details, pins and comment column — 25 October 2026

Desktop material selection uses a session-only key and begins empty. Filtering cannot substitute another material for an invisible selection, and page departure/mode replacement clears selection. Desktop row values include copied saved comment strings and pin flags. The comment preview uses textContent plus CSS nowrap/hidden/ellipsis; full notes remain plain text in the editor. Separate native select/pin buttons avoid nested controls; pin clicks do not open details. Keyed rows preserve focus during refreshes and edits.

Independent desktop pins use a versioned allowlisted-key file capped at 256 KiB and 1,000 unique keys. Only the real mode atomically writes engineering-pins.json; simulator pins use a separate in-memory map. Saves validate current catalogue membership, mode and closing state. Errors leave prior pins intact, and corrupt files visibly disable pin writes. Pinning/unpinning never changes target quantities, comments, game bindings, arming or queued flight input. The terminal target filter is unchanged.


## Cockpit health and module power — 25 October 2026

The health DTO copies finite, bounded hull values and only shows shield state for fresh main-ship telemetry with a known running game process, or explicitly labelled simulation. It uses the player's Status flags, never ShipTargeted values. Offline red edges/banner clear for stale/future data, read errors, missing/main-ship-invalid state, process exit and backend failure. Warning animation is CSS-only, pointer-transparent and respects reduced motion. The setting is allowlisted to ship/bars and defaults old preferences to ship; durable preferences still omit fullscreen.

ModulesInfo.json is read in the existing cancellable journal poll, with scoped closed file handles and no new worker, network, process or timer. Reads are bounded to 1 MiB and 512 entries, validate event/timestamp, finite nonnegative draw, priority 0–4, bounded unique slot/item identities and current fitted membership. Reports older than the current loadout/session are ignored; Loadout resets previous power data. File metadata and fit identity cache unchanged reports. Malformed/partial/mismatched reports preserve only the last accepted snapshot and expose the read error. Missing files clear power data. Built-in Cockpit/CargoHatch/DataLinkScanner entries may be ignored when absent from the fit. Published slices are copied and scalar pointers remain event-owned/immutable.

Power draw is never treated as plant capacity, and zero draw/configured-off is never used to invent an overdraw shutdown. Actual supply is explicitly not reported. Draw/priority report time and loadout time remain separate. Module card rendering uses textContent and existing sanitisation. The destination display reads cached route/target values, labels direct distance and unknown final-route fallbacks, and its button only selects Navigation. No arming/focus/binding behavior changed.


## Visual readability implementation - 26 October 2026

Saved UI size is allowlisted to 100/125/150/175/200; zero legacy values normalize to 100. It is stored with appearance choices and never fullscreen or arming state. Appearance writes reject closing sessions. The frontend scales relative units without introducing a worker, service or network request. Control-fit calculations use the rendered square dimension; no binding or input rule changes. Scan-line pseudo-elements stay behind content, and warnings remain pointer-transparent with reduced-motion handling. Manual fire-group editors preserve local open/focus state across DTO updates and still invoke only the existing validated reference save. Shared module display names retain raw identifiers as secondary detail. Engineering row reuse, explicit selection, per-item drafts and persistence gates remain intact.
