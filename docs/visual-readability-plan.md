# Co-pit visual review and improvement plan

26 October 2026 · Historical review and implementation plan.

Implemented and merged into `main` on 27 October 2026. The measurements and proposed sizes below describe the original review, before the final Balanced sizing adjustments. See [the current implementation](visual-readability-implementation.md) for shipped sizes, spacing and verification.

The main problem is inconsistent sizing and information hierarchy. The navigation has responsive typography, while most actual information remains fixed at 10–14 CSS pixels. Panels expand into available space without keeping related information together. Decorative framing and scan lines receive more space and emphasis than some important readings.

The design should remain a sci-fi cockpit with glass square controls, three themes, radar-style health graphics and technical accents. The recommended change is a shared readable design system across every page.

## Supported displays and review method

**Primary: 1920×1080 landscape. Secondary, fully supported: 1440×2560 portrait.** High-resolution landscape and portrait are additional checks, not the basis for oversized layouts.

Reviewed the current embedded HTML/CSS/JS using isolated sample bridge data, across all nine main pages and three themes. The initial sweep covered 1080p, 4K and portrait viewports; focused sweeps covered both target monitors at approximate Windows 100%, 125% and 150% scaling. Representative screenshots and computed sizes were inspected, including Engineering with details open. All sampled main pages fit horizontally; that does not establish readability, adequate vertical layout or complete accessibility.

The 1080p scaling profiles used 1920×1080, 1536×864 and 1280×720 CSS viewports with corresponding device scale factors. Portrait used 1440×2560, 1152×2048 and approximately 960×1707. This checks layout and pixel density; it does not reproduce every native WebView2/Windows DPI behavior or the physical viewing distance. The user's Windows scaling setting is still unspecified.

Outfitting and the add-control dialog received a source/style review; their full populated, error and keyboard states should be included in implementation validation. Simulated data was used throughout; there were no game inputs or external requests from the reviewed frontend.

## Findings

| Priority | Finding and evidence | User impact | Recommended correction |
|---|---|---|---|
| High | At 1080p the navigation is about 21.5 px, but Engineering rows are 13 px, generic values 14 px, module field labels 11 px and footer text 9 px. At 4K the menu reaches 26 px while these data sizes stay unchanged. | The large menu promises a readable interface, but the information being used is much smaller. | One shared type scale, applied to content, controls, status and navigation. Add saved UI scaling. |
| High | The default Cockpit extends below the 1080p viewport: activatable controls and destination require scrolling. Large heading gaps, technical strips, framing and panel spacing consume height. | Important flight information is hard to reach at the primary resolution. | Compact the surrounding structure before reducing any text or button dimensions; design the default layout to fit its main controls and bottom destination at 1080p. |
| High | The 1440 px portrait menu uses 23.04 px labels, but at the 960 CSS-pixel viewport used to approximate 150% scaling they become 15.36 px. | Viewport-based font sizing counteracts intended magnification. | Use stable relative typography with explicit user scaling; let available width control reflow, not smaller text. |
| High | Scan lines are a panel pseudo-element above content (`z-index: 2`). Grids, gradients, double borders and corner brackets recur on almost every surface. | Texture crosses small glyphs and competes with tables and forms. | Move decorative layers behind content; keep stronger scanning effects inside gauges, with calm solid surfaces under text. |
| Medium | At wide sizes the health readings sit far from the ship icon, fixed 112 px control squares spread across very wide fractional columns, and module values are pushed to distant card edges. | Users must scan large distances to associate labels, values and controls. | Bound internal group widths and use stable label/value grids. Keep square controls together with sensible gaps. |
| Medium | Ship shows raw-looking names such as “hpt pulselaser fixed large”, while Hardpoints shows “Pulse laser Fixed large”. Similar headings have different casing, weights and sizes. Generic panel heading rules also affect material-detail headings. | The same equipment feels like unrelated data on different pages. | One display-name formatter and explicit heading/card/table components; keep raw identifiers available only as secondary technical detail. |
| Medium | Engineering uses a fixed 82 px stock column and 32 px pin button. With details open at smaller effective widths, material names truncate heavily; portrait splits list/details into separately scrolling regions. | Labels, quantities and the editor compete for limited space. | Size columns from their content and text scale; use a clear list/detail split with predictable scrolling and visible editor actions. Preserve one-line comment ellipses. |
| Medium | Ship has several lines of explanation above its modules. Navigation puts browser search and explanatory text before the current route. Settings has long, stretched forms. | Routine actions and data lose priority to repeated instructions. | Keep freshness, unknown-state meaning and essential warnings visible; move longer explanations into accessible Details sections. Put the page's main information first. |
| Medium | Status vocabulary varies: ON/On, Unknown/unknown, unavailable and not reported. Warnings, selected states and decorative accents use different local treatments. | People must relearn visual meaning on each screen. | Shared status presentation and wording, preserving the distinction between state unknown, data not reported, stale snapshots and actual alerts. |
| Maintenance | Later styles repeatedly override earlier rules across `style.css`, `screen.css`, `pages.css`, `navigation.css` and `health.css`; ship styles also live in the health stylesheet. Control column calculation separately hardcodes 112 px. | A local readability fix can produce a new inconsistency elsewhere. | Central tokens and component ownership; remove superseded rules as components migrate. Derive layout calculations from the same dimensions used by CSS. |

## Proposed visual rules

Initial sizes below are **CSS pixels at 100% app scale**, to be checked in the 1080p preview. Windows scaling remains respected; devicePixelRatio must not be multiplied into sizing again.

| Role | Proposed default |
|---|---|
| Body, data tables, form input and button text | 16 px |
| Secondary labels and report timestamps | 14 px |
| Primary connection/arming status | 16 px, semibold, clearly separated states |
| Navigation | 18 px, stable across pages and supported layouts |
| Card/module names | 20 px, bold |
| Section headings | 18–20 px, consistent weight/casing |
| Page headings | 28–32 px |
| Primary health values | 28–32 px |
| Nonessential footer/technical decoration | 12 px; never the only place essential information appears |

Use Segoe UI/system sans-serif for names, instructions, forms and tables. Keep Cascadia/Consolas for numeric readings, key labels and short technical accents. Use tabular numbers for quantities, power, distance and percentages. Avoid paragraphs or long names in spaced uppercase text.

Add a saved **UI scale** control, initially 100%, with 125%, 150%, 175% and 200% options and an obvious reset. It must enlarge all relevant text and interaction areas together. Density is achieved by spacing and arrangement, not tiny fonts. Preserve the compact module treatment. Scaling should work independently of orientation and theme, with sensible reflow and scrolling at the largest settings.

Use a common spacing rhythm, consistent panel edges and a clear division between primary, secondary and destructive actions. Ordinary buttons should share readable labels and a consistent hit area; compact icon actions need an adequate hit area and accessible names. Keep glass lenses for cockpit ship controls, where their illuminated state has meaning.

Use consistent semantic colors for normal state, selection, caution, unavailable/unknown and warning. Text and symbols must carry important status in addition to color. Target at least 4.5:1 contrast for ordinary text and 3:1 for large text, including against the final decorated surfaces; this is a future verification target, not a claim that the current app was contrast-certified. See [W3C contrast guidance](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html). Support text enlargement through 200% without losing content or functions, following [W3C resize-text guidance](https://www.w3.org/WAI/WCAG22/Understanding/resize-text.html).

## Page-by-page layout work

**Cockpit:** tighten the title/header area and decorative strips. Group the ship, shield and hull readings within a coherent compact header, rather than placing labels at opposite window edges. Keep toggles and momentary triggers as clearly separated banks. Retain square glass buttons, names above and key labels on the lens, plus the saved maximum columns setting. The default 1080p arrangement should expose the standard supported controls and a compact destination summary at the bottom without routine scrolling. At larger app scales or a deliberately low column count, allow controlled scrolling; never shrink text to force that layout. Give real disconnected, armed, paused and simulator states the same visual hierarchy.

**Ship:** preserve the full-width summary above two modules per row. Group related summary fields. Keep bold module names, compact cards, integrity meters and key-system damage warnings. Put power draw and priority in aligned adjacent fields; avoid long gaps between a label and its value. Use shared friendly equipment names. Keep short report-age/status information visible and make detailed telemetry limitations expandable. Two columns remain the normal layout; stack only when enlarged text or narrow width makes a pair unreadable.

**Engineering:** preserve the full-height list, explicit item selection, independent pins, targets and saved comment column with ellipsis. Increase row/column header text. Keep stock values close to material names, align numeric data, and provide the full truncated text on focus/selection. Landscape uses list plus details when both have useful widths. Portrait retains list access above a clearly bounded details area, with an obvious close control and predictable scrolling to target/comment actions. Avoid a long list consuming the useful editing area after selection.

**Navigation/outfitting:** show the current target/route before secondary search tools. Make internal outfitting browsing and external Inara links visually distinct and clearly named. Use the same readable filters, table headings, empty/loading/error states and report-age treatment as other pages. The previously proposed system/port dossier and expanded plotting/bookmarking features are a separate functional project; this review does not assume those mockups already exist in the app.

**Hardpoints and Missions:** apply the shared equipment/card/table typography. Compact the repeated fire-group editor with a readable summary and explicit edit mode, while continuing to label it as a manual reference. Use consistent data rows, readable mission titles and restrained empty-state panels. Preserve existing functions and all data limitations.

**Settings, Diagnostics and Help:** group settings under straightforward labels such as Display, Appearance and Controls. Bound form widths and keep labels near controls. Put UI scale near the top with a live preview. Use readable summaries in Diagnostics, with detailed technical content in expandable sections. Keep help and external links consistent with navigation/actions elsewhere.

## Delivery sequence

1. **Approve the shared visual specification through previews.** Show Cockpit, Ship and Engineering at 1920×1080 landscape and 1440×2560 portrait, including a larger-text setting. Use current app markup/data in local previews. These three pages establish the component rules before applying them across the app.
2. **Implement the foundation.** Add shared type/spacing/surface/status tokens and saved scale validation. Replace the conflicting generic rules; connect control-fit calculations to actual rendered dimensions. Separate decoration from text layers.
3. **Apply the high-priority pages.** Cockpit first for 1080p vertical fit, then Ship and Engineering. Confirm explicit selection, notes/target drafts and controls remain usable as layout changes.
4. **Bring remaining screens and states onto the same components.** Navigation/outfitting, Hardpoints, Missions, Settings, dialogs, Diagnostics and Help. Consolidate names, timestamps, status wording and action styles.
5. **Validate and package.** Run focused frontend checks and relevant preference tests, compare screenshots, then perform a native Wails pass before building the replacement EXE.

## Acceptance checks

- Primary screenshots: 1080p landscape at 100%, 125% and 150% Windows-equivalent scaling. Secondary: 1440×2560 portrait at those same settings. Also check windowed use and 4K; moving the native app between monitors needs a real DPI test.
- Test app scale separately through 200%. Typography, names, form labels and status indicators must enlarge together; no viewport rule should counteract it. Scrolling at larger scales is acceptable; loss of content or actions is not.
- Default 1080p Cockpit shows the primary health, supported default control arrangement and bottom destination. User-selected low column counts and larger scaling remain accessible through predictable scrolling.
- No horizontal page overflow; wide data tables may have an explicit local scroll area where needed. Long equipment/station names, large quantities, long notes and unavailable readings must remain understandable.
- All themes have the same geometry and hierarchy. Check decoration enabled/disabled, reduced motion, active/off/unknown/unavailable controls, shield offline, disconnected, stale and error states. Verify actual composited contrast.
- Engineering names/values stay readable; comment previews remain single-line with ellipsis. Opening/closing details and editing drafts must preserve data and keyboard focus. Save actions remain reachable at shorter heights.
- Test the add-control dialog and outfitting viewer with populated, empty, loading and error states. Check native selects, focus rings, keyboard navigation and all close/back actions.
- UI changes must not change the game's controls, arming/focus-resume rules, data sourcing, session cleanup or fullscreen persistence behavior.

## Evidence

The original review screenshots and measurements were saved in the local Codex visualization workspace. The checkpoint commit preserves the original source. Current implementation screenshots and verification instructions are in [Visual readability implementation](visual-readability-implementation.md).
