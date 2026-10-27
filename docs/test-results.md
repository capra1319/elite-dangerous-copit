# Test verification

## Repository layout cleanup — 27 October 2026

Source and tests now live together under app/, with their embedded frontend/ and data/ directories. Guides, reviews, old terminal previews and planning material live under docs/. Build scripts and the preset installer are under tools/; optional terminal and compatibility launchers are under tools/launchers/. The root desktop launcher remains Launch Co-pit.cmd. Both executables and required runtime configuration are packaged in build/bin/.

Both Go test suites and both vet configurations passed after the move. Production Wails and terminal builds completed using the new paths. The layout change preserves the Go source contents and package relationships; every moved Go source/test blob matches its original. The 1,080-view browser matrix and outfitting/state checks pass with the relocated embedded frontend. Root and compatibility launcher checks pass in isolated folders, including spaces, first-build cancellation and errors. The rebuilt terminal app passes an explicit one-shot mock smoke check. Preset-source and packaged paths plus existing-file protection pass against a temporary fake profile with the normal Windows PowerShell module path; no real game settings were touched. Documentation links and image paths were validated.

## Overview, screenshots and root launcher — 27 October 2026

The README is an overview with current simulated Cockpit, Ship and Engineering screenshots; the detailed reference moved to docs/user-guide.md. All changed documentation links and screenshot targets were checked. Current screenshot capture used the embedded frontend with isolated fixtures, blocked external requests and no game-input calls; the landscape images were visually inspected. A portrait Ship image is included in the user guide.

The production Wails build completed in the main checkout. Launch Co-pit.cmd opens the built EXE with build/bin as its working directory and exits; tools/launchers/Start-Co-pit.cmd delegates to it. Isolated temporary-folder checks using a harmless local stub passed normal launch, spaces and ampersands in the folder path, the compatibility alias, declining the first-build prompt, missing-Go guidance and visible build-failure reporting. These checks did not launch the game companion or send game input. No backend or frontend behavior changed.

## Current merged baseline — 27 October 2026

Both applications and the final Balanced sizing update are available on `main`; merge commit 38a67b7 includes the terminal and Wails histories. After resolving the merge, `go test ./...`, `go test -tags desktop ./...` and `go vet -tags desktop ./...` passed. Both worktrees were clean before this documentation update.

The sizing update passed the existing 1,080-view browser matrix plus the outfitting/flight-state checks. The 1080p Cockpit destination remains visible at default scale; portrait Ship and 1080p Cockpit screenshots were inspected. The production Wails build completed as `build/bin/Co-pit.exe`. Default text is 18 px body, 16 px supporting labels, 22 px equipment names and 20 px cockpit control labels; internal spacing is compact while section gaps remain distinct.

Native physical-monitor readability and live gameplay checks remain manual. The sections below are historical test records; earlier text sizes, test counts, temporary build names and branch references describe their dated revisions. For current build and verification commands see [development](development.md) and [the sizing implementation](visual-readability-implementation.md).

## Build verification — 24 October 2026

Built on Windows amd64 with Go 1.26.4, Bubble Tea v2, Lip Gloss v2 and their pinned dependencies.

## Passed

- All 88 standard automated tests (`go test ./...`); native-console testing, mock-preview export, desktop control timing, global page keys and subprocess lifecycle checks are opt-in. The subprocess helper is invoked only by the lifecycle test. `go vet ./...` passes.
- Boot animation tests verify that connection updates are processed during animation, the handoff happens after 800 ms, skip/navigation/Quit remain responsive, backend arming/errors bypass the intro, mock switching does not replay it, and its timer cannot start after session shutdown. Animated frames fit all three themes at 60x20 through 140x42 while keeping the disarmed status and stop shortcut visible.
- The boot artwork now uses original text-only eagle-medal silhouettes with full, compact and miniature variants. The existing layout checks also verify the eagle head and medal caption remain visible with the skip hint and control status at every supported size and animation phase.
- An actual terminal run displayed the animated logo sweep and filling startup bar, automatically handed off to the cockpit, and exited with Q. The preview explicitly opted into mock mode and sent no game input.
- Shutdown review and fixes are detailed in [SAFETY-REVIEW.md](safety-review.md). Tests verify owned-work cancellation/joining, real TUI Quit, blocked mock input, backend-panic cleanup and error reporting, canceled action rejection, stale/previous-session hotkey rejection, bounded journal records, preserved key-release failures and redirected input-handle ownership. The opt-in Windows tests reclaimed shortcuts after graceful cancellation, panic, forced test-process termination and partial startup failure. The actual EXE quit cleanly in disarmed control mode; classic mock printed `Stopped.` after Ctrl+C during a partial line. No test copy remained running. The race detector is unavailable on this machine (CGO disabled; no C compiler).
- Connection styling tests verify the full-width connected/disarmed and connected/armed banners, window titles and emergency-stop visibility across all nine pages, three themes and terminal sizes from 60x20 through 140x42. Startup, cached telemetry, focus alone, read errors, unavailable data, setup errors, stale snapshots, process exit, missing state, backend failure and mock mode cannot activate the bright armed treatment. The banner replaces the header's blank row, preserving the page content height.
- Hardpoints page tests cover F10 versus the modified emergency-stop shortcut, all-nine-page navigation, mount ordering, utility inclusion, unknown/zero ammo, last-loadout timestamps, mothership/fighter isolation, reference persistence and deletion, commander/ship/module isolation, corrupt-file protection, save failures, mock edits staying off disk, and selected cells/details remaining visible in all themes at 60x20 through 140x42.
- A read-only EXE run against the actual local journal displayed all ten reported hardpoint/utility mounts via scrolling, selected group E, deployed/stowed state, and point-defence ammo of 12 / 10000 from the last loadout. F10 opened the page at 80x24; editing A=primary and B=secondary saved only to a separate test reference file. Q exited cleanly. No game inputs were sent or game files changed. The grid explicitly labels assignments as a manual reference because Elite exports only the selected group, not its trigger assignments.
- Clock tests cover subsecond, one-second and five-second future snapshots, the first value beyond the tolerance, a large future offset, equivalent timezone representations, and delayed clock ticks. UI age, connection scanning, data-quality labels and guarded control checks agree on the existing five-second allowance; only a larger offset produces a mismatch. Small future offsets display zero age rather than flashing a false warning.
- Numpad page tests cover physical Num+ / Num- identity, top-row/modifier exclusion, hold suppression, release/repress, wraparound, cancellation of unsaved theme/target edits, local fallback, and separation from Num0–9 flight controls. Control mode handles its registered page keys once through the global-hotkey path, avoiding duplicate terminal-event navigation.
- The opt-in Windows page-key check passed: the real disarmed backend registered its global page keys, received previous/next WM_HOTKEY messages posted only to its own thread, called the correct view-only navigation callbacks, then stopped and released Num+ and Num-. No keyboard input was injected into Elite or the desktop.
- Four companion pages are implemented with F6/F7/F8/F9 shortcuts: Ship Overview, Navigation, Mission Board and Engineering Checklist. Tests cover journal history replay, partial-record completion, exactly-once delta handling across polls, rotation, truncation, legacy/current filenames, commander isolation, immutable published inventory, bad completed records, cancellation, mission IDs/progress/redirects/completion, material snapshot/trade/crafting/reward arithmetic, fighter isolation, route recovery/clearing, and a cached clear not overwriting a newer target.
- New-page model tests cover direct shortcuts, forward/reverse page cycling, isolation from flight input, themed populated layouts from 60x20 to 140x42, terminal-control sanitisation, unknown/empty data labels, coordinate-derived distances, delivery progress and engineering search/edit/save/unpin flows. Material goals survive relaunch without altering theme settings; invalid targets and write failures remain visible. Missing inventory never produces an invented completed target.
- Interactive read-only integration against the actual local journal loaded the recorded commander/ship/loadout, current system/body, the authoritative empty active mission list and the material inventory. F6-F9 opened each new page at 80x24. Searching Arsenic and setting a target of 20 showed `0 / 20 / 20`; the goal was saved to a separate test preferences file. No game commands were armed or sent and no game files were changed.
- Theme/settings tests cover live previews, cancellation through Esc/Tab/F1/F5, saving and restoring all three themes across fresh real/mock sessions, isolated palettes per model, invalid/oversized settings, visible write failures and unchanged mock control state while using Settings. All three themes fit Cockpit, Diagnostics, Help and Settings at 60x20, 80x24, 110x36 and 140x42, both while scanning and after connecting; disarmed status and the stop shortcut remain visible.
- Interactive EXE verification used a separate temporary appearance file: F5 opened Settings, arrow keys previewed themes, Enter saved Retro Operations, and a fresh read-only launch restored its double-line borders. Previewing Flight Console changed to square borders; Esc returned to the scanner in the saved Retro theme. The compact Settings page fit at 80x24 without a clipped panel, and Q exited cleanly. No game controls were armed or sent. The rebuilt `eliteaux.exe` includes these themes.
- Responsiveness update: default telemetry polling is 100 ms (previously 500 ms), cockpit frames publish immediately after each real-control read, process checks run every 250 ms (previously one second), and foreground/input checks run every 10 ms (previously 25 ms). Missing status files return for the next poll immediately; partial-file retry waits are 10/20 ms. The 60 ms key pulse, 300 ms input cooldown and telemetry confirmation guards remain in place. Tests verify that an absent file does not spend time retrying.
- The opt-in desktop control timing check (`ELITEAUX_CONTROL_TIMING_TEST=1 go test -run '^TestControlTelemetryPublishLatency$' -v .`) published a changed temporary Status.json in **111 ms**. It stayed DISARMED, sent no keyboard input and released its hotkeys. It tests the real control backend's publication path, with a 750 ms deadline that catches the previous one-second redraw delay. It excludes Elite's own telemetry-write latency and terminal rendering time.
- Normal startup now selects real-game control with numpad input and starts disarmed. The default-mode preview test verifies actual telemetry, no simulator prompt/data, and no changes to the status file. Mock CLI tests now opt in explicitly.
- F4 mode-switch tests cover deliberate keypresses, compact/wide toggle visibility, restored real connection settings, discarded simulated state, disarmed rescanning, and refusal to enter another mode when backend shutdown fails.
- The rebuilt EXE was launched with no arguments in the normal Windows desktop session. It immediately connected to the running game and displayed real supercruise telemetry, lights, night vision, power and fuel while DISARMED. F4 entered simulation; F4 again restored current real telemetry and DISARMED control without shortcut conflicts. Q exited cleanly. No arming or game input was performed.
- A separate compiled read-only integration check against a missing test status file verified scanning, F4 into mock, F4 back to read-only scanning with simulated state discarded, and clean Q exit.
- Connection scanner tests cover startup, background-game readiness independent of focus, cached telemetry without a game, exit/restart, stale startup, quiet connected snapshots, read recovery, SRV/on-foot contexts, process-inspection fallback, clock mismatch, animation start/stop, terminal bounds, accessible diagnostics and fatal-error presentation.
- The cockpit groups controls by Flight & Utility, Visibility, Combat and Navigation at compact/wide sizes. Toggle states use ACTIVE/INACTIVE; missing feedback remains UNKNOWN and stale feedback stays LAST KNOWN. Night vision appears once under Visibility, Num5 still changes its simulated state, and pending feedback stays visible. The real-game bindings and original hardware mapping remain intact.
- The compiled scanner ran interactively in read-only mode against a missing test file: its activity bar updated through changed cells, Tab opened Diagnostics and Q exited cleanly. No repeating screen clears occurred after initial terminal setup.
- A normal hidden Windows session verified the coloured scanner and global-shortcut backend together. Windows reported the game absent; the waiting screen remained active with no standard-error output. A deliberately mismatching profile made arming impossible, and no game input was sent.
- Scanner wide/compact PNGs and a looping animated preview were rendered from the actual Bubble Tea View and inspected alongside the revised five-toggle cockpit layout.
- Bubble Tea model tests cover compact/wide bounds, persistent stop instructions, unknown/stale labels, physical numpad identity and held-key suppression, typed commands, scenario/input switching, scrolling help, immutable telemetry frames and isolation of real-game actions behind the guarded backend.
- Interactive Bubble Tea terminal check: typed `t3 on` changed the mock lamp, Tab opened Diagnostics, and Q exited cleanly. After initial screen setup, refreshes updated changed cells instead of blanking the dashboard.
- Wide/compact visual previews were rendered from the actual Bubble Tea View content with explicitly simulated data and inspected for spacing and legibility.
- Read-only Bubble Tea integration check read the local game status file, displayed stale/unknown data honestly and exited cleanly with Q.
- Normal Windows integration check passed with the new coloured Bubble Tea interface and global-shortcut backend running together. The test remained disarmed and used a deliberately mismatching profile so it could not arm or send input.
- Refresh rendering updates only changed rows in one terminal write. Regression checks verify indicator changes without full-screen clearing, unchanged refreshes producing no terminal output, shortened/removed messages disappearing, repaint after resizing, and important feedback fitting a small viewport.
- Compiled live-view terminal check: successive refreshes changed only the snapshot-age row; the dashboard and lamps were not cleared or repainted. The read-only test process was stopped afterward.
- Real-control tests cover disarmed startup, no deferred replay, pending-request blocking, telemetry-only acknowledgement, same-second changed snapshots, timeout without retry, focus/read/stale/clock failure, no automatic rearm, profile validation, context restrictions, map closing, delivery errors, consumable uncertainty and cooldown.
- Input-adapter fakes verify rejection of unrelated windows/held modifiers and release of app-owned keys after cancellation/focus loss. No unattended SendInput calls were made against the game or desktop.
- The prepared personalized preset validates all ten function-key bindings and avoids direct numpad conflicts. Active-profile checks reject a different selected ship preset.
- Native Windows console test passed separately in a dedicated hidden console: numpad gear press, held-key suppression, release/repress, night vision, Escape exit, and restoration of the previous console input mode. Synthetic records were written only to that test console; no desktop key injection was used. The tool's pseudoterminal denied writing synthetic console records, so this test used its own console.
- Alternate input selection and shortcut preview, all ten numpad mappings, top-row/modifier exclusion, Enter/F1/Esc, and continued operation of typed commands.
- Static checks (`go vet ./...`).
- Release compilation (`.\tools\Build-EliteAux.ps1`).
- Compiled mock app: gear on, external gear off, failed scoop request with unchanged lamp, on-foot unknown state, and quit.
- Interactive Windows terminal: mock prompt accepted `t1 on`, redrew L1 as on, and accepted `q`.
- Windows Saved Games discovery located the actual Elite Dangerous Status.json.
- Compiled live app read the existing local file. Its older timestamp and zero flags were correctly displayed as stale and unknown, with missing Flags2 called out. No game files were changed.
- The later real-control diagnostics also read fresh snapshots from the active game, correctly showing supercruise, lights and night vision. Diagnostics remain disarmed and register no hotkeys.
- Compiled live polling against a separate synthetic file: gear change displayed, malformed content retained last-known state with an error, file deletion reported unavailable, and replacement recovered with lights on.
- Interactive live display refreshed and responded to Ctrl+C with “Stopped.” The surrounding tool shell reported interruption; this was not a separate graceful-exit-code assertion.

## Not yet tested in actual gameplay

- Gear, scoop, lights, hardpoints, night vision and flight-assist changes while actively flying.
- Second-monitor readability and terminal sizing in the user's preferred setup.
- Physical numpad behaviour in the user's terminal host, including Num Lock handling; the native integration test uses console records rather than a physical keyboard.
- An armed real-game command is still untested; do the initial Num3 lights check after selecting the preset.
- Long-running play session, actual game exit/restart and real game file-write cadence. Exit/restart transitions are covered by model tests; Windows process enumeration was checked with the game absent.
- Real keyboard delivery/acknowledgement during an armed game session. The control feature is implemented but has not been demonstrated in actual gameplay.
- Physical hardware: not implemented.

## User check

### Normal Windows startup verification

The Codex restricted environment initially denied global shortcut registration. A subsequent normal-session test passed: the compiled controller registered its arm/stop hotkeys and remained running disarmed. This test used a deliberately mismatching preset, making arming impossible, and sent no game input. The process was stopped afterward and Windows released its hotkeys. The app reports the restricted-environment case with instructions to launch from File Explorer.

The new EliteAuxPanel.4.2 preset was installed alongside the original Custom profile. The active preset selection was not changed. Primary HOTAS bindings in the prepared profile were compared with the original and preserved. The original profile was not overwritten.

Launch tools/launchers/Start-Live.cmd while Elite is closed: the scanner should show the game waiting. Start Elite and load your commander; the cockpit should open after a recent usable snapshot arrives. Use existing game controls to change gear, scoop, lights, hardpoints and night vision. The matching indicator should change after Elite writes its next snapshot, normally within the app's 500 ms polling interval plus file-read time. Confirm both on and off transitions. Close the game and check that the scanner returns even when Status.json remains. A quiet connected snapshot should instead stay visible with a stale label.

For actual control, follow REAL-GAME-SETUP.md: select EliteAuxPanel under Ship Controls, run tools/launchers/Start-Game-Control.cmd, return to the unobstructed cockpit, arm with Ctrl+Alt+F9 and first test lights with Num3. Confirm the cockpit and the app agree; disarm with Ctrl+Alt+F10.

## Co-pit desktop — 25 October 2026

- `go test ./...` passes the existing 88 standard terminal tests.
- `go test -tags desktop ./...` passes those tests plus 3 desktop tests: validated display preference reload/fallback, fullscreen exit despite failed preference persistence, and joined mode/shutdown cleanup with rejection of a restart after closing.
- `go vet -tags desktop ./...` and `node --check frontend/app.js` pass.
- Wails v2.17.0 production build creates `build/bin/Co-pit.exe` with the Co-pit icon and embedded frontend; no Node build step is required.
- Native manual checks: F11 changed the window from 1280 × 850 to the 2560 × 1440 monitor dimensions with no OS frame/taskbar; Esc restored 1280 × 850. The tall-window preset produced 720 × 1100 with stacked panels and all nine page links at the bottom. Settings remained vertically scrollable.
- Closing the native test instance removed Co-pit and its direct child processes. Real controls stayed disarmed throughout; gameplay arming/action checks were not performed.
- Physical portrait monitor fullscreen, multi-monitor transitions, Windows display scaling, forced WebView2 termination, and fullscreen preference startup restoration remain manual integration checks. Companion pages currently reuse shared text readouts rather than a complete graphical redesign.


## Usability and focus resume — 25 October 2026

The flight-first page order, numbered terminal console tabs, type-to-search engineering editor, +/- target adjustment and simulation preference isolation are covered by regression tests. Focus tests cover same-session pause/resume without replay, Stop, game/app close, binding/read/stale/backwards-clock failures, PID/window/path/creation-time replacement, and delivery failure. A Windows process-identity test uses the test process only and sends no input.

Co-pit additionally tests guarded UI control requests, Stop replacing a pending click, and its searchable material/quantity form. Native mock checks verified Ship lights changing its green LED, searching Arsenic, saving a target of 20 against 12 owned (8 needed), and the portrait cockpit/bottom navigation. The visual test instance exited with no remaining Co-pit/direct WebView2 children. The Windows lifecycle integration test passed graceful exit, panic, forced termination and partial-registration cleanup. Real-game auto-resume/click delivery has not been exercised; no test sent flight input.


Current standard test counts: 94 for the terminal build and 99 with the desktop build tag. Opt-in Windows lifecycle and global page-key checks also pass.


## Station-search shortcut — 25 October 2026

Three additional standard tests validate fixed HTTPS search categories, terminal picker layout/flight-input isolation, and cancellation before browser launch. The picker is checked at 60×20, 80×24, 110×36 and 140×42 in all themes. Search tools are public Inara station, outfitting, commodity, material-trader and component pages; no mission offers or current-stock guarantees are invented. No automated test opens a browser.

Current standard tests: 97 terminal, 102 desktop.

## Module warnings, station viewer and screen styling — 25 October 2026

- Terminal `go test ./...` and `go vet ./...` pass. Co-pit `go test -tags desktop ./...` and `go vet -tags desktop ./...` pass. Both Windows executables rebuilt successfully.
- Module-warning tests cover healthy, damaged and zero-integrity key systems, ordinary modules, missing/out-of-range/NaN/infinite integrity, variant module identifiers, false-positive exclusions, and narrow terminal output. A desktop test verifies structured snapshot isolation, no flight requests, report timestamps and no duplicate module text.
- A regression verifies class-zero station utility mounts are not displayed as weapons. Existing on-demand station-client and desktop lifecycle tests pass, including cancellation, mode changes, shutdown and rejection of obsolete replies.
- Headless Edge checked the actual frontend assets with clearly simulated data at 1280x900, 720x1100 and 360x800 in all three themes. Module warning states, unknown values without meters, narrow-screen bounds, safe literal module names, loading labels, and pointer-transparent screen decorations passed. Screenshots were visually inspected. These checks did not launch the native companion, arm controls, send flight input, or verify live in-game damage telemetry.
- The new module display remains a last-Loadout journal snapshot. Optional scan lines are static and scoped to screen panels. The styling adds no background activity. The previous real EDSM availability check is recorded separately; no additional live request was needed for this styling update.

## Applied Bold Lens controls — 25 October 2026

- Applied the approved glass-button design to all ten Co-pit controls: component names above, 48 px local SVG icons and NUM key labels on the lens, no small LEDs or visible ON/OFF labels, and etched corner/stencil/chassis decals. The controls inherit all three theme palettes and adapt to portrait layout.
- Existing desktop tests (`go test -tags desktop ./...`) and vet (`go vet -tags desktop ./...`) pass. The production Windows executable rebuilt successfully with the icon licences packaged.
- Headless Edge verified the actual frontend at widths 1440, 1100, 720, 360 and 320 across all three themes. Labels, icon dimensions, key bounds, absent LED/state-label nodes and pointer-transparent decals passed. Landscape and portrait screenshots were visually inspected.
- An isolated mock bridge verified no startup flight requests; click and keyboard requests use the correct control ID; requests do not optimistically illuminate glass; subsequent reported ACTIVE does; disabled clicks are ignored while the reported active indication remains visible; unknown state stays explicitly marked; momentary actions have no persistent pressed state; polling preserves the button nodes. No external resource request was made. These checks did not interact with Elite or inject real game input.
- No control backend, arming/focus-resume behavior, telemetry polling, shutdown ownership or terminal app source was changed by this styling update. Icons are a locally embedded Lucide 1.8.0 subset with its ISC licence retained.


## Compact square controls and saved columns — 25 October 2026

- Desktop tests and vet pass. Two new preference tests cover Automatic and explicit column-count persistence, existing version-1 preference compatibility, no control-state/request changes, rejected negative/too-large values without modifying disk or memory, and rejection of fractional counts.
- Headless Edge checked the actual frontend in all three themes at 1440, 1100, 720, 360 and 320 px. Every button remains 112 x 112 px; every lens is square, the icons remain 48 px, and names and key labels fit. Landscape and portrait screenshots were visually inspected.
- Every Buttons per row option (Automatic and 1-10) was selected through the Settings control with an isolated mock bridge at 1920 px. The effective row count matched each choice; narrowing the window fit fewer columns, and widening restored the saved request. Preference changes did not dispatch flight actions. Existing guarded-click, reported-state illumination, disabled-button, unknown/action, no-external-resource and stable-node checks passed again. No live game input was sent.


## Ship-aware toggle and trigger layouts — 25 October 2026

- Desktop tests and vet pass. Three new layout tests verify empty defaults, restart persistence, copied input/snapshot arrays, duplicate/unknown/action-key rejection, schema/file bounds, failed-write and closing behavior, no control-state or queued-input changes, retained layout/file contents during ship swaps, accurate toggle/trigger kinds, known/unknown/absent launcher states, rejection of missing utilities and recovery when returning to a fitted ship.
- Headless Edge exercised the actual frontend with an isolated mock bridge: empty first panel; + checklist excludes the missing launcher; selected controls enter the correct group; changing ships updates the checklist and retains unavailable tiles; unavailable buttons are disabled and pulse dim red; returning ship data stops the pulse; reduced-motion mode uses a steady indication.
- Reassignment changes the displayed key and group, removal and Clear panel update the saved model, failed saves leave assignments unchanged and errors visible, repeated edits correctly unlock the editor, and Escape dismisses the picker without changing fullscreen. Checklist keys do not invoke cockpit/mock shortcuts. All three themes fit at 1440, 720 and 360 px; buttons remain 112 px squares. Checklist, landscape and portrait screenshots were visually inspected.
- No live Elite input or external asset requests were used for these checks. Availability is deliberately limited to supported actions and the last reported ship loadout; live ammunition, health and broader unimplemented game utilities are not inferred.

## Session-only fullscreen — 25 October 2026

- Desktop tests and vet pass. Updated preference tests confirm no fullscreen/borderless field is stored and leaving fullscreen succeeds with an unwritable preferences path.
- New regression tests load legacy fullscreen/borderless flags without restoring them or losing theme/layout/scanline/column preferences, verify the desktop constructor clears inherited fullscreen, and confirm toggling mode neither creates nor changes the settings file. Saving another appearance choice preserves the current in-memory mode while a reload starts windowed.
- The production build completed. With the user's permission, the running Co-pit was closed via its normal window-close path, the executable was replaced, and its SHA-256 matched the prepared build. No forced process termination was used. Native re-entry/F11 display checks were not repeated; the window-mode callback and reload behavior are covered by the automated tests.


## Graphical menus, permanent controls and display sizing — 25 October 2026

- Desktop tests and vet pass. New tests cover the eight permanent controls, automatic addition of supported fitted utilities, retained unavailable assignments after ship swaps, saved preferences, validated scan-line spacing, structured/sanitised page data, the 100-entry route bound and direct fire-group reference validation. Reference edits and preference/layout changes do not queue flight input. Legacy fullscreen fields remain excluded from persistence.
- Headless Edge exercised the actual frontend with isolated simulated backend fixtures. Permanent controls cannot be removed or reassigned; toggles and activatable buttons occupy separate groups. Automatic adding saves the preference and includes supported fitted utilities; available utilities cannot be removed until the option is disabled. The spacing slider updates the screen decoration and saved value. Hardpoint dropdowns save only the manual reference.
- All graphical pages fit at 1440, 720 and 360 px in all three themes without horizontal overflow. Ship, Navigation, Missions and Hardpoints screenshots were inspected. Engineering's material list expands by more than 450 px when increasing a landscape window from 1920x1100 to 1920x1600, with its panel reaching the available main-area height. Short/narrow layouts retain scrolling access.
- No live Elite input or external resource request was used. The production Windows/amd64 Wails build completed at build/bin/Co-pit.exe with embedded frontend assets, icon, configuration and licences. Co-pit was not running, so the executable was replaced directly. Native monitor/display-scaling checks remain manual.


## Engineering target draft focus fix — 25 October 2026

- Reproduced the reported bug in the actual frontend with headless Edge: entering 42 over a saved target of 12, blurring and refreshing reverted the field to 12. The same check passes with explicit unsaved-draft tracking.
- Focus changes and state refreshes preserve an unsaved quantity. Save clears only the draft it submitted after success; write failures and newer edits during an in-flight save retain the quantity. Selecting another material or switching real/mock mode resets the draft; page navigation alone preserves it. Blur does not save. Save and Remove target remain explicit operations.
- Headless checks cover these behaviors using a simulated bridge with successful, failed and delayed saves. No flight input was sent. Desktop tests and the graphical page/theme/layout checks pass again. The production Co-pit executable was rebuilt with the corrected embedded frontend.


## Comments-only material reference — 25 October 2026

- Three desktop regression tests verify comments across relaunches/ship swaps, independent simulator memory without real-file writes, mode-mismatch rejection, target/input isolation, explicit clearing, bounds/key/UTF-8/control-character validation, CRLF normalisation, failed-write/closing behavior, oversized/corrupt-file protection and retention of valid future-material entries. Desktop tests and vet pass.
- Headless Edge exercises the actual frontend with an isolated bridge: comment drafts survive blur, polling, material selection and page navigation; explicit save/clear, failed and delayed saves retain the correct draft; real/simulator contexts stay separate; HTML-like content remains text; file errors disable writes visibly. Raw/manufactured/encoded materials all offer the same editor. All three themes fit landscape, portrait and 360 px layouts; portrait was visually inspected. No network requests or game input occurred.
- The target-quantity draft regression and graphical page/theme/full-height checks pass again. The production Co-pit executable was rebuilt with comments-only item details. No blueprint data files, importer or additional source licences were added to the repository.


## Selectable Engineering details, independent pins and comment column — 25 October 2026

- Desktop tests and vet pass. Three new tests cover initially empty/explicit selection, closing and page re-entry, no implicit selection after filtering, saved row comments, independent pins with zero targets, pin persistence/reload, unpin without changing target/comment, real/simulator isolation, mode mismatch and closing rejection, invalid/oversized/duplicate pin files, corruption protection and failed atomic writes. Existing selection tests were updated for the requested opt-in details behavior. No flight input is queued.
- Headless Edge checks the actual frontend with an isolated bridge: details initially hidden; full-width list when closed; pin clicks do not open details; pin and Pinned materials filter work without quantities; unpin hides a filtered selection while retaining its target; comment saves update the existing row DOM; Enter opens details; write failures leave pin state unchanged; corrupt pins show a visible error and disable pin controls.
- Long spaced, unbroken and multiline saved comments remain one-line previews with CSS nowrap/hidden/ellipsis. All three themes fit open/closed layouts at 1920x1600, 1440x1100, 720x1400 and 360x800 without horizontal overflow or increased row height. Landscape and portrait list screenshots were visually inspected. Quantity-draft, comment-draft and graphical-page/full-height checks pass again. No network request or game input was used.
- The production Wails executable was rebuilt with the new Engineering list and details behavior.


## Responsive page-menu labels — 25 October 2026

- Menu labels, page numbers, sidebar spacing/width and page headings now share responsive sizing. Portrait page buttons use three columns at 520 px and below. No saved preferences or backend/input behavior changed.
- Headless Edge at 2x pixel density checked all nine labels across all three themes at 800x650, 1440x1100, 1920x1080, 3840x2160, 360x800, 720x1400, 1080x1920 and 2160x3840. Labels remain visible and fit without clipping/overflow. Landscape menu text grows from 18.36 px at 1440 to 26 px at 3840; portrait grows from 11 px at 360 to 24 px at 2160. Sidebar width and page headings grow with it. Clicking Ship still selects the correct page and active state. No game input or external requests occurred.
- The existing all-page/theme/layout checks pass again. A high-density landscape screenshot was visually inspected. The production EXE was rebuilt with embedded navigation styles. Native physical-monitor checks were not repeated.


## Ship health styles and module power — 25 October 2026

- Desktop tests, shared terminal tests and desktop vet pass. New tests cover legacy/default/saved/invalid health preferences and omission of fullscreen; valid zero/unknown/non-finite hull; copied view values; live shields up/down; stale/future/missing/read-error/non-main-ship/game-exit/backend-failure warning suppression; explicit simulator values; route final/next/jumps/direct-distance and labelled target fallback with no queued commands.
- Module tests cover matching draw, 0 MW, zero-based priority conversion to 1–5, known off vs unknown switch, absent supply, repeated reads, copied snapshot slices and closed file handles; malformed/partial/mismatched/negative/invalid-priority/duplicate/empty/oversized reports; cancellation; stale-session rejection and power reset across ship/loadout changes.
- Headless Edge tested the actual embedded frontend with isolated simulated bridge snapshots. Both health styles and saved selection, nil vs zero vs UP state, offline warning/reset across pages, reduced motion, Navigation-only route action, full-width Ship summary above modules, two columns (one on narrow windows), all three themes at 1440x1100, 900x1600, 650x1200 and 360x800 passed. Default Cockpit and portrait Ship screenshots were visually inspected. Existing page/theme/layout checks pass, including permanent/automatic controls, scan-line settings, fire-group reference menus and full-height Engineering. No external request or game input was sent.
- The production Wails build completed with the updated frontend and backend in build/bin/Co-pit.exe. No real-game telemetry session or physical-monitor scaling test was performed; actual shield percentage and module-overdraw diagnosis remain unavailable by design.


## Compact module cards - 25 October 2026

Module names now use bold 20 px text. Tighter padding, inline power/priority readings and reduced spacing shorten the cards while retaining condition warnings and integrity meters. Headless Edge passed all three themes at 1440x1100, 900x1600, 650x1200 and 360x800 without horizontal overflow. The portrait screenshot was visually inspected. A production replacement was built as Co-pit-next.exe because the running Co-pit process locks the canonical executable. Backend behavior is unchanged.

Compact-card replacement installed as build/bin/Co-pit.exe on 26 October 2026 after user approval. Co-pit was already closed at installation; the installed executable SHA-256 matches the prepared production build.


## Visual readability branch - 26 October 2026

Checkpoint fc44b06 preserves the previous design; implementation is isolated on visual-readability. Desktop and terminal tests plus desktop vet pass. New tests cover durable UI scale, legacy and invalid inputs, shutdown rejection, fullscreen isolation and shared equipment names. Portable browser checks cover 1,080 page/theme/size/scale combinations and verify default 1080p Cockpit destination visibility, no horizontal page overflow, scaled typography/controls, saved size/reset, explicit manual-reference editor/focus, reachable Engineering saves, picker Escape and reduced motion. Populated/loading/error/empty outfitting, flight visual states and all shared text palette colors against shared theme surfaces pass. Quantity drafts, comment drafts and material pin/list regression checks pass. No browser test sends game input or external requests. A separate production Co-pit-readability.exe is supplied for user testing; native monitor/DPI and physical readability checks remain manual. See docs/visual-readability-implementation.md for details and reproduction.
