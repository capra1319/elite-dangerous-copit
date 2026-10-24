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

- The production application contains no child-process creation, network client/listener, background service, scheduled task or auto-restart code. Default game data comes from local files. The subprocesses in the lifecycle tests belong only to the test harness and are explicitly waited for or killed.
- Each normal control session starts disarmed. Windows hotkeys are thread-owned and explicitly unregistered on return, error and panic unwind, including partial registration failure. Tickers and process/file handles have cleanup paths.
- Flight input requires an armed backend, the expected foreground game process/window, current main-ship telemetry, valid game bindings and a permitted game context. Focus loss, invalid telemetry, changed bindings or delivery failure disarm it.
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

Console-close/logoff/shutdown cleanup notifications are best effort and subject to Windows behavior and time limits. A forced exit can leave a small, incomplete temporary preference file, but it does not represent a running process or connection. Paths explicitly configured to point to network shares are managed by Windows' filesystem layer; the app itself has no networking implementation.
