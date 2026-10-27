# Development

Both Co-pit desktop and Elite Aux terminal live on `main`. The former
`wails-conversion` and `visual-readability` branches were merged in commit
38a67b7 and retired. Use a new feature branch from `main` for future work.

## Repository layout

```text
Launch Co-pit.cmd       Open the desktop app
app/                   Shared Go source and tests
  frontend/            Embedded desktop HTML, CSS and JavaScript
  data/                Embedded material catalogues and terminal artwork
  wails.json           Wails project configuration
tools/                 Desktop/terminal build scripts and preset installer
  launchers/           Optional terminal and compatibility launchers
build/                 App icons/manifests and ignored bin/ outputs
docs/                  Guides, screenshots, review and test records
assets/                Original branding assets
licenses/              Third-party notices
```

`go.mod` and `go.sum` stay at the repository root. Go sources remain in
one package under `app/`, so shared behavior and tests stay together.
The desktop build runs Wails from `app/` and uses `build:dir` to place
outputs in the root `build/` folder. Both executables and their runtime
configuration are packaged together in `build/bin/`.

## Build on Windows

Install Go 1.26 or newer. Dependencies are pinned in `go.mod` and `go.sum`;
the first build needs internet access to download them. Run commands from
the repository root.

```powershell
git switch main
git pull --ff-only
.\tools\Build-Co-pit.ps1
```

The script invokes the pinned Wails CLI and creates `build/bin/Co-pit.exe`,
with configuration, the binding preset and licence files in `build/bin`.
Launch by double-clicking `Launch Co-pit.cmd` in the repository root.
`tools/launchers/Start-Co-pit.cmd` is a compatibility alias for the same launcher. If the
EXE is absent, the launcher checks for Go and offers a first build; it shows
build failures instead of silently closing. It launches the app with
`build/bin` as its working directory and then exits. Existing builds open
without rebuilding. The app embeds its
frontend assets; changes to HTML, CSS or JavaScript require rebuilding the EXE.
Microsoft WebView2 Runtime is required to run the desktop app.

Build and run the terminal app separately:

```powershell
.\tools\Build-EliteAux.ps1
.\tools\launchers\Start.cmd
```

The default build uses `app/main_terminal.go`; the `desktop` build tag selects
the Wails entry point. Both applications share the Go control and telemetry
code. Run one app at a time to avoid global shortcut conflicts. Normal
launches scan for the game and start disarmed; simulation requires explicit
selection. See [game setup](game-setup.md) for the binding preset.

Build outputs are ignored by Git. Use `build/bin/Co-pit.exe` for the current
desktop build; older local `Co-pit-readability.exe` and `Co-pit-next.exe`
files are superseded test copies. Builds in another checkout are independent.

## Verify changes

```powershell
go test ./...
go test -tags desktop ./...
go vet ./...
go vet -tags desktop ./...
```

For frontend layout checks, Node.js must be able to resolve Playwright,
and Microsoft Edge must be installed. These are development dependencies,
not runtime dependencies of the packaged app. The opt-in fixture exporter
uses simulated data and writes only to the explicitly selected path:

```powershell
$env:COPIT_FRONTEND_FIXTURE = Join-Path $env:TEMP 'copit-readability-fixtures.json'
go test -tags desktop -run TestExportDesktopUpgradeFixtures ./...
node tools/check-readability.cjs
node tools/check-readability-states.cjs
```

Screenshots and reports default to ignored `tmp/readability`. Set
`COPIT_VISUAL_OUTPUT` to choose another output directory. The browser checks
block external requests and do not send game input. They cover 1080p
landscape, 1440×2560 portrait, approximate Windows scaling, all themes and
interface sizes, Engineering editors, the control picker and station views.
Physical display readability and real-game integration still need manual
verification. See [test evidence](test-results.md).

## Frontend sizing

Shared typography and spacing tokens are in `app/frontend/style.css`. Component
styles are in `controls.css`, `health.css`, `ship.css`, `pages.css`,
`navigation.css` and `outfitting.css`; `screen.css` supplies decoration.

At 100% interface size, body text is 18 px, supporting labels 16 px,
equipment names 22 px, page headings 26 px, and cockpit control names/keys
20 px. Square controls remain 112 px with 48 px icons. Keep the root
16 px scale separate from body text so changing readability does not also
enlarge every control. Saved interface scaling applies to both dimensions
and typography. General section gaps are 24 px; the dense Cockpit uses
16 px between panels and keeps helper lines close to their related content.

The [implementation notes](visual-readability-implementation.md) describe
responsive layouts and the [original plan](visual-readability-plan.md)
records the historical review.
