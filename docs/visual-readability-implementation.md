# Visual readability implementation

Updated 27 October 2026 · Available on `main` · Design baseline: fc44b06

The readability work and Balanced sizing are merged into `main` with the terminal companion updates. Merge commit 38a67b7 preserves the combined history. The former `wails-conversion` and `visual-readability` development branches have been retired; use `main` for both applications. The original review remains in [the design plan](visual-readability-plan.md), while this document describes the current implementation.

## Result

- Shared typography: 18 px body/data/input text, 16 px secondary text, 18 px navigation, 22 px equipment names, 26 px page headings and 30 px health readings at 100% interface size. Numeric readings use tabular figures. The same rem-based scale sizes spacing, buttons, icons, inputs and navigation, without multiplying Windows DPI.
- Settings > Display > Interface size saves 100%, 125%, 150%, 175% or 200%, with a live preview and Reset size button. Missing legacy values default to 100%. Invalid values and saves during shutdown are rejected. Fullscreen stays session-only.
- The default 1920x1080 Cockpit displays health, both control banks and the bottom destination summary without page scrolling. Wide screens arrange the two banks side by side; narrower layouts stack them. Square lenses remain 112 px at standard size and scale with the interface. Their fitting calculation reads rendered dimensions rather than a separate hardcoded pixel assumption.
- Shield/hull readings are grouped around the radar graphic. Real shields show UP, with the percentage limitation in the source note; a confirmed shields-down report still uses 0% and the existing warning. Unknown and stale states retain their prior backend semantics.
- Ship retains a full-width summary and paired compact module cards with large names. Draw/priority fields use aligned readable rows, report ages stay visible and longer limitations are expandable. Ship and Hardpoints now use matching friendly weapon/utility names.
- Engineering retains explicit details, independent pins, saved comments and targets. Larger rows and content-sized stock columns improve reading. Comments stay on one line with ellipses. In portrait, selecting an item gives more space to the editor while retaining a bounded list. Short windows allow page scrolling and the editor remains independently scrollable.
- Current navigation/route data appears before outfitting and browser tools. External Inara search is an explicit expandable section. Outfitting uses the same form/table sizes and error/loading treatment.
- Hardpoint cards show a compact manual-reference summary, with the reference editor opened explicitly. Saving keeps the editor open and preserves selector focus. These assignments still only update the local reference.
- Settings, Help and Diagnostics share headings/forms/cards. Detailed telemetry and data-source information is expandable. Quiet surfaces and scan-line decoration behind content replace overlapping texture and duplicated border effects. Glass lens decoration and gauge scan lines are retained.
- At large interface sizes in small windows, navigation moves to a scrollable compact bottom layout instead of squeezing page content. Portrait uses the same readable text sizes, with column counts based on available space.

## Validation

Go desktop and terminal suites pass; desktop vet passes. New backend tests cover legacy scale defaults, persistence, all supported values, invalid integer/type rejection, failed/closing saves, fullscreen isolation, no queued input and shared equipment display names.

The portable browser check renders 1,080 combinations: nine pages, three themes, five UI scales, and eight display profiles. Profiles include primary 1080p landscape and secondary 1440x2560 portrait with 100/125/150 percent Windows-equivalent viewport sizes, 4K and 800x650 windowed use. It verifies no horizontal page overflow, consistent text/control scaling, default 1080p destination visibility, saving/resetting UI size, explicit fire-group editing with focus retention, Engineering save reachability, picker Escape, reduced motion, and decoration below text.

Additional checks cover outfitting populated/loading/error/empty states at five sizes and three UI scales, and connected/armed/paused/unknown/unavailable/shields-offline displays. Shared foreground palette colors exceed 4.5:1 against each shared theme surface. Existing Engineering quantity/comment draft, pin/list, focus, ellipsis and error-recovery checks also pass. Browser tests use isolated sample bridge data and block external requests; they send no game input.

The browser profiles approximate DPI layout; native WebView2 rendering, moving between monitors and physical viewing distance remain manual checks. The user will test the packaged Wails binary. The review does not claim a full accessibility certification, or add the previously proposed advanced navigation dossier/bookmark/plot features.

## Reproducing the visual checks

Requires Go, a local Node runtime with Playwright resolvable, and Microsoft Edge. Set COPIT_FRONTEND_FIXTURE to an absolute temporary JSON path, then run:

~~~powershell
go test -tags desktop -run TestExportDesktopUpgradeFixtures ./...
node tools/check-readability.cjs
node tools/check-readability-states.cjs
~~~

With no explicit fixture path, the browser tools read copit-readability-fixtures.json in the OS temporary directory; the Go exporter requires the environment variable explicitly to avoid writing files during normal tests. COPIT_VISUAL_OUTPUT can choose the screenshot/report directory; otherwise outputs go under ignored tmp/readability. These are optional development checks, not runtime app dependencies.

## User test build

The current production binary is build/bin/Co-pit.exe. The older build/bin/Co-pit-readability.exe is the initial readability test build and does not contain the latest sizing adjustments. Close any existing Co-pit copy before opening the current build so only one copy owns global shortcuts.

Suggested checks:

1. On 1080p landscape, inspect the complete default Cockpit and destination without scrolling. Try your preferred button-column count.
2. In Settings > Display, choose a larger interface size, reset it, and verify the saved choice after restarting.
3. On the portrait monitor, check the bottom navigation, paired Ship modules and Engineering with details both closed and open.
4. Edit a quantity and comment, click away, navigate back and confirm drafts remain; save them deliberately. Confirm pins and comment ellipses behave as before.
5. Open/close the outfitting viewer and control picker; check filters, long names and keyboard focus.
6. Confirm the manual fire-group reference opens explicitly and remains open after saving. Check native text appearance at your actual Windows display scale.

## Balanced sizing follow-up

Checkpoint commit: 95fc076, recorded before the sizing changes. The selected Balanced preview uses 18 px body text, 16 px supporting labels, 22 px equipment names, and consistent 20 px bold cockpit control names and keys at 100% interface size. The root scale stays at 16 px so square buttons remain 112 px and UI scaling continues to work normally.

Panels use 12 px internal padding; table and material rows use 8 px vertical padding while controls keep their minimum hit areas. General section grids retain 24 px gaps, and the dense Cockpit keeps 16 px between panels with helper lines closer to their associated content. The existing theme, borders, glass controls and scan lines are retained. No new section banners were introduced. Two-line control labels reserve equal height so the lenses align.

Both existing browser checks pass after this change, including 1,080 page/theme/scale/profile combinations, default 1080p destination visibility, portrait layouts, outfitting states, control picker and Engineering editor access. Screenshots of the 1080p Cockpit and portrait Ship page were inspected. The changes are limited to frontend CSS; game input, arming, process ownership and backend persistence are unchanged.
