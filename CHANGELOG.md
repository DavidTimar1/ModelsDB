# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Every commit bumps the version in `VERSION` and adds a matching entry here.

## [1.29.0] - 2026-07-18

### Added
- **A "Check for updates" button in the Info dialog.** It forces an immediate
  release check instead of waiting for the once-a-day background check, and reports
  the result inline - either that a newer version is available (with a download link
  and the banner) or that you are on the latest version. Backed by a new
  `?force=1` parameter on `/api/update-check` that hits GitHub synchronously and
  bypasses the daily throttle.

## [1.28.2] - 2026-07-18

### Fixed
- **ZDR (Zero Data Retention) derivation restored.** OpenRouter moved its provider
  data-policy endpoint from `/api/frontend/all-providers` (now 404) to
  `/api/frontend/v1/all-providers`. The old URL's 404 made every ZDR refresh fail
  soft, leaving stale flags. The endpoint moved but its shape is unchanged
  (`data[].dataPolicy.retainsPrompts` keyed by provider `slug`), so pointing at the
  `/v1/` path is the whole fix. Verified end to end against OpenRouter's own
  `zdr=true` model list.

## [1.28.1] - 2026-07-17

### Changed
- Refreshed the README screenshot to show the current table and card layouts.

## [1.28.0] - 2026-07-17

### Added
- **A live model counter, "N of M".** It shows how many models are currently shown
  out of the total, updating on every filter, search, and column change. It sits
  after the Clear button in the desktop filter bar, and before the Sort control in
  the mobile sort bar. The counts come from DataTables' own row info, so they match
  what the table reports.
- **A close (X) button at the top of the mobile filter drawer**, so the drawer can be
  dismissed from inside it as well as by tapping the backdrop. Card view only.

### Fixed
- **The filter drawer no longer slides across the screen when the layout crosses the
  table/card breakpoint.** The drawer's slide transition was attached to its resting
  (off-screen) state, so crossing the breakpoint animated it into and back out of
  view. The transition now lives only on the open state: entering card view places
  the drawer off-screen instantly, and opening it still animates. (Closing is
  instant - animating it both ways in pure CSS would bring the flash back, and the
  layout breakpoint is deliberately CSS-only.)

## [1.27.4] - 2026-07-17

### Changed
- **The desktop table rows are taller and their dividers stronger**, so one row
  separates from the next more clearly. Cell top/bottom padding grows, and the row
  divider is darkened from DataTables' faint `rgba(0,0,0,0.15)` to the theme border
  colour. Scoped above the card breakpoint, so the mobile cards are unaffected.

## [1.27.3] - 2026-07-17

### Changed
- **The mobile Sort control moves to the right, beside the direction button.** The
  "Sort" label, the dropdown and the up/down direction toggle now sit together as one
  group on the right of the sort bar, with the Filters button alone on the left,
  instead of the Sort control stranded in the middle.

## [1.27.2] - 2026-07-17

### Changed
- **The OpenRouter mark is now their real v2 brand glyph, in brand colour.** The
  old icon was a hand-drawn monochrome shape. The glyph is the official asset,
  filled with OpenRouter's brand colour keyed to the theme - purple (`#7624F4`) in
  light, lime (`#C8FF00`) in dark - via the `--or-fill` token. It is one shared
  icon, so it updates in both the desktop table and the mobile cards.
- **More breathing room between mobile card cells.** The card gap grew from `5px 8px`
  to `10px 14px`, and the Speed/Rating/OCR controls get extra separation (~30px
  apart) so the row of selects no longer reads as cramped. Desktop table spacing is
  unchanged.
- **The mobile Sort dropdown is sized to its content** (`width: max-content`, capped
  at the bar width) instead of stretching wide.

## [1.27.1] - 2026-07-17

### Changed
- **Mobile card polish (card view only; desktop table unchanged).**
  - The Filters button now sits a clear ~20px from the Sort control instead of
    crowding it.
  - The Name cell puts the icon row inline after the name and date, wrapping the
    icon cluster as a unit only when the line is too narrow, with the personal note
    on its own line below. (The desktop table keeps its stacked name / icons / note.)
  - The In $/Out $ cells now read like the other card cells: an uppercase `IN`/`OUT`
    caption over the value, followed inline by the abbreviated unit and the
    pricing-note icon.
  - The pricing-note affordance is the plain note glyph, visible at rest and tappable
    to open the existing note editor, replacing the boxed empty-state control.

## [1.27.0] - 2026-07-17

### Changed
- **The table folds five columns into two cells, and the layout is a plain
  breakpoint again.** HuggingFace, OpenRouter, Notes, ZDR and the pricing Unit no
  longer have columns of their own. The Name cell is now three lines - name and date;
  a row of marks (capability icons, the HF and OpenRouter links, copy-id, the note
  icon, and the favourite star); and the personal note when there is one - and the
  In $/Out $ cells carry the pricing unit as a short abbreviation (`tkn`, `chr`,
  `img`, ...) on a second line, with the pricing-note badge on both. Favorite also
  lost its column; its star moved into the Name cell's mark row.
- **Layout switches table vs cards at a single 1300px breakpoint.** The measured
  root-font fit from 1.26.0 is gone. With the columns folded in, the table's
  min-content is ~1147px, so it fits comfortably above 1300px and cards take over
  below. The breakpoint is one number in two places that are kept in step
  (`style.css` `@media`, `app.js` `matchMedia`).
- **Type is fixed `rem` on a percentage root**, not a `vw` clamp. `--fs-body` is
  `1rem` and the root is `87.5%` (14px on a default base), so the whole scale still
  tracks a reader's browser base-size preference. The root size is the single knob
  that sets the table's width.
- **Card cells have no fixed columns.** Each card is a wrapping flex flow: a cell
  grows to absorb its line's leftover space but never exceeds its own content width,
  and cells sharing a line share a height. This replaces the fixed-track grid, which
  stranded empty space as a card narrowed.
- Sorting is dropped for the fields that lost their columns (HuggingFace, OpenRouter,
  note, ZDR, pricing unit, favorite); each remains filterable.

### Fixed
- **A save with any column hidden could wipe personal fields.** `queueSave` read
  `favorite`, `speed`, `rating`, `ocr_quality` and the notes straight from their
  cells; a hidden column has no cell, so the read returned an empty default, and
  because `SaveCurated` applies a partial update that empty value overwrote the
  stored one. Editing one field with another's column hidden could blank a rating or
  clear a favourite. The save now includes a field only when its control is actually
  present, so a hidden column leaves its stored value untouched.

## [1.26.3] - 2026-07-16

### Fixed
- **Speed, OCR and Rating did not line up in a card.** Rating stood 11px taller than
  its two neighbours and its control sat lower and 22px narrower, in a row meant to
  read as three matching selects. Two causes: the cell padding reset was keyed on the
  `.col-narrow` class, which is a desktop width hint set on Speed and OCR but not
  Rating, so Rating alone kept the table's cell padding; and the three captions were
  inline, so a caption and a select with a min-width competed for one cell and
  whichever lost wrapped its control onto a second line. The captions now sit on their
  own line, each control fills its cell, and the padding reset is keyed on the field.
- **A longer price pushed the Out pill onto a second line**, making that cell taller
  than the In cell beside it. The In/Out captions and pills no longer wrap.

## [1.26.2] - 2026-07-16

### Changed
- **A dev server now shows its version number in the header** (e.g. `v1.26.1-dev`)
  instead of an anonymous `dev`. `start_dev.sh` and `start_dev_agent.sh` inject the
  repo-root `VERSION` with a `-dev` suffix, which names the version the build came
  from without claiming to be that release - so a dev and a production window sitting
  side by side can be told apart at a glance. `IsDevVersion()` treats the suffix as a
  development build, so the version gate, the update nag and the self-updater stay off
  exactly as before; `parseSemver` already ignores a `-suffix`, so comparisons are
  unaffected. A bare `go run` with no ldflags still reports `dev`.

## [1.26.1] - 2026-07-16

### Changed
- **Speed and Rating now sit just before OCR** in the table, after Size (GB).

### Fixed
- **The filter drawer swept across the screen on every filter or sort change in card
  view.** `fitTableToViewport()` takes `.view-cards` off to measure the table in table
  form and puts it straight back; the drawer transitions its `transform`, so the
  browser animated that round-trip - it started fully on-screen as a full-height
  overlay and slid out over 250ms, on every redraw. Transitions and animations are now
  suppressed while a measurement is in flight, and the result is committed with a
  forced reflow before they resume. Opening the drawer still animates.

## [1.26.0] - 2026-07-16

### Changed
- **The table fits by measurement, not by a breakpoint.** `fitTableToViewport()`
  measures what the table needs and solves for the root font size that makes it fill
  the available width, clamped to a legible 8-14px. Card view is now the *output* of
  that measurement - it engages only when the table cannot fit even at 8px - rather
  than a number someone typed. Columns can be shown and hidden at runtime, so the
  table's width is a runtime variable and no fixed breakpoint can be right for it.
  There are now **no media queries and no `matchMedia` calls** in the app's CSS or JS.
- **Type scale is a plain multiple of the root size.** `--fs-title`/`--fs-control`/
  `--fs-body` were `clamp()` expressions driven by `vw`, which pinned text to the
  window rather than to the measured fit. `--fs-body` is now exactly `1rem`, making
  the root size and the table's text size the same thing. The root size is applied as
  a percentage, so a raised browser base font size is still respected.
- **Dimensions that affect the table's width are `rem`.** Cell padding, column widths,
  the inline Speed/OCR/Rating selects and the price pills all scale with the root size.
  Borders stay `px`.
- **Paging follows the view.** `applyViewPaging()` pages 25/row in card view and shows
  all rows otherwise, reading the same class the fit sets instead of its own breakpoint.

### Fixed
- **The table's panel was never styled.** Nine rule locations targeted
  `.dataTables_wrapper`, a DataTables **1.x** class. The vendored library has been
  DataTables 2.0.8 since the first commit and emits `.dt-container`, so the panel's
  background, padding, radius and shadow had never applied - the table sat bare
  against the page while the filter bar had a panel. The table is now inset inside a
  real panel, with the inset scaling with the font.
- **The table overflowed the viewport.** `columnDefs` set fixed px widths
  (`230px` for Name, `60px` for Speed/OCR), which DataTables renders into a
  `<colgroup>`. Those never scaled, so ~350px of the table's width was constant and
  the fit solved against a table that did not exist. At 1024px the table painted
  16px past the viewport edge; it is now inset 10px inside its panel.
- **The fit ignored the panel's padding**, so the table overran the panel's right
  edge by exactly that padding at every width.
- **The pager theming never applied.** Four rules lost on specificity to
  `datatables.min.css`, which prefixes the same selectors with `div` - `!important`
  does not help, as it only ranks within a specificity tier. The accent "current page"
  pill never rendered, and in light theme hovering a pager button turned it near-black.
- **The notes textarea was not focused when opened**, costing a second click before
  typing: the focus call targeted the span that `replaceWith` had just detached.
- **The 2500px width cap applied to neither the filter bar nor the sort bar**, whose
  selectors (`#filters`, `.mobile-bar`) match no element. Past 2500px they would have
  spanned full width while the table capped.
- Removed dead `div.top` rules (a DataTables 1.x-era top row this app never renders)
  and a `width` option DataTables does not accept.

## [1.25.0] - 2026-07-16

### Added
- **`./start_dev_agent.sh` - an isolated sandbox server.** Pins config/data/cache to
  `./.temp-data` and runs on port 8125, with its own database seeded from the
  embedded catalog. Because the data dir differs, the PID file differs, so it runs
  alongside `./start_dev.sh` (port 8123) and a production install (port 8122)
  without stopping either. Lets one person restart the server freely while another
  keeps using theirs. `./.temp-data` is git-ignored and disposable, and holds no
  personal data.

### Changed
- **The PID file records identity, not just a number.** It is now JSON -
  `{"pid","port","exe"}` - because a bare PID cannot be validated.
- **The dev launchers print the machine's Tailscale URL as the primary link**,
  resolved at runtime via `tailscale ip -4`, and explain why to prefer it over
  `127.0.0.1` when connecting from another machine. `start_dev.sh` previously
  printed the literal placeholder `<this-machine-tailscale-ip>`, leaving the
  loopback URL as the only clickable link.

### Fixed
- **A stale PID file could kill an unrelated process.** Startup takeover signalled
  whatever PID the file named, with no check that it was ours. PIDs are recycled, so
  a file left by an exited server could name someone else's process. The takeover now
  kills only while the recorded port is still bound - a free port means the instance
  is gone - and clears the stale file instead.
- **The post-kill wait watched the wrong port.** It waited on this process's port
  rather than the port the killed instance held. When they differed the wait returned
  at once, and the new process could open `modelsdb.db` while the old one still had
  it. It now waits on the recorded port, and fails loudly if the port never frees.
- **`stop` and `status` could act on a server that was not theirs.** They read a PID
  without checking which port it belonged to; both now require the recorded port to
  match the port in question.

## [1.24.0] - 2026-07-15

### Added
- **Undo / redo of personal data edits.** Two buttons at the start of the filter
  row (and `Ctrl`/`Cmd`+`Z` / `Ctrl`/`Cmd`+`Shift`+`Z`) undo and redo edits to the
  six personal fields - favorite, notes, speed, rating, OCR quality, and pricing
  note. Undo restores the previous value in the cell and saves it through the same
  path a manual edit uses, so the database stays in sync; a fresh edit clears the
  redo trail. The history is session-only and held in memory: a page reload or a
  server restart clears it, and nothing about `/api/save` or what gets stored
  changes. Filters, sort, column visibility/order, colours and theme are not part
  of this history.

## [1.23.0] - 2026-07-15

### Changed
- **One-line filter bar.** All filter controls now flow on a single row that wraps
  only as an overflow fallback (there is no forced second row). The **Clear**
  button moved to the very end, after every filter and the Colors/Columns
  controls.
- **Input / Output / Year are fixed-size summary dropdowns.** These three
  multi-value filters no longer grow as you add selections. Each is a fixed-width
  button whose label summarises the state - the field name when nothing is picked,
  the single value when one is picked, or "Multiple selected" for two or more -
  over a dropdown of checkboxes. Filtering behaviour and the saved/restored filter
  state are unchanged, and the control works by touch in the mobile filter drawer.
- **Details modal close button.** The "x" that closes the model-details dialog is
  now a proper close button pinned to the modal's top-right corner, with a 32px
  hit area and a muted-to-solid hover, in both light and dark themes and on both
  desktop and mobile. The modal body scrolls beneath a fixed head, so the close
  stays in view.

### Removed
- **Select2 dependency.** The Input/Output/Year controls no longer use Select2;
  the bundled Select2 stylesheet and script are dropped from the UI.

## [1.22.0] - 2026-07-15

### Added
- **Column show/hide control.** A "Columns" dropdown in the filter bar lists a
  checkbox per column; unchecking one hides that column (via DataTables column
  visibility). All columns start visible, and the hidden set persists across
  reloads (stored server-side alongside the column order, so visibility and
  drag-reorder are independent and both survive a reload). Hiding several columns
  also narrows the desktop table, easing the wide-table horizontal scroll.
- **Copy-id icon beside the model name.** OpenRouter-listed models show a small
  copy icon that copies the model id (e.g. `anthropic/claude-haiku-4.5`) to the
  clipboard (with the non-secure-origin fallback), flashing a check on success.

### Changed
- **The model name opens the Details modal.** Clicking (or keyboard-activating)
  a model name opens the same details dialog the Details button used to open. The
  name no longer links to OpenRouter - that page link lives on the "O" icon.
- **Details column and button removed.** The dedicated Details column (desktop)
  and the card's Details button (mobile) are gone; the name opens the modal
  instead. On a mobile card, row 7 is now just the ZDR cell.
- **ZDR is coloured text in every view.** The desktop ZDR cell drops its tinted
  background fill and shows green "Yes" / red "No" text, matching the mobile card.
- **Desktop table cells are left-aligned.** Every header and body cell aligns
  left, overriding the automatic right-align of numeric columns and centring of
  icon columns.
- **Desktop Context shows rounded thousands.** The Context cell renders `128k`
  (like the mobile card) while still sorting on the raw token count; its header
  gains a "Context size in thousands" tooltip.
- **P / AP header tooltips name the unit.** The Parameters and Active-parameters
  headers now read "Parameters (in billions)" and "Active parameters (in
  billions)" on hover.
- **Responsive breakpoint raised to 1000px.** The card view now applies to any
  viewport under 1000px wide and the full table applies at 1000px and up (was
  768px), because the ~20-column table cannot fit below roughly 1000px.

## [1.21.0] - 2026-07-15

### Added
- **OpenRouter presence filter.** A new "OpenRouter: All / Yes / No" dropdown in
  the filter bar (mirroring the HF-link filter) keeps only models that do (or do
  not) carry an OpenRouter routing link. The choice persists across reloads.
- **Favorites-only filter.** A new "Favorites" dropdown in the flags row shows
  only starred models when set to "Favorites only". The choice persists across
  reloads.

### Changed
- **One typography system across the whole UI.** A single UI font family is used
  everywhere (monospace only for the details-modal JSON and preformatted blocks),
  and every text size is one of three fluid `clamp()` tokens - title, control,
  body - that scale smoothly with the viewport. Body copy caps at 16px and the
  title at 28px on wide screens; cards use an inverse body curve so text stays
  comfortably large on a phone. The stray serif on the info button is gone.
- **Mobile pager shows a compact sequence** - previous, page 1, page 2, an
  ellipsis, the last page, next - instead of a long run of page numbers.
- **Mobile pager row re-laid-out.** "Showing 1 to 25 of N" sits on its own line;
  below it the pager sits at the left and "25 / page" at the right.
- **Filters button moved into the mobile sort bar** (was in the top bar), sitting
  to the left of the sort field and direction toggle.
- **Desktop page-length control ("N / page") is right-aligned** in its row, with
  the "Showing ... entries" info line at the left.
- **Desktop data-table cells are vertically centered.** Every cell's content
  (including multi-line name cells and the inline select controls) centers within
  its row.
- **Mobile cards separate clearly from the page.** The page, top bar, and sort bar
  sit on the page background while each card sits on the card background, so cards
  read as raised tiles in both light and dark themes.
- **Nothing is sticky on a phone** - the top bar, sort bar, and update banner all
  scroll away with the page.
- **Mobile card row-1 icons (HF / OpenRouter / star) are vertically centered**
  against the model-name block.
- **The empty pricing-note badge on a card is a rounded-square note-with-plus
  icon** (a clear "add a note" affordance) instead of a bare plus; the has-note
  round orange badge is unchanged.

## [1.20.2] - 2026-07-15

### Added
- **`--help`/`-h` (and a bare `help`) print a full usage message** listing every
  command and global flag, then exit without starting a server.
- **`--version`/`-v` print the version**, then exit without starting a server.

### Fixed
- **An unrecognized command or flag is now rejected** with a clear error naming
  the offending argument and a "try --help" hint, exiting non-zero. Previously an
  unknown flag such as `--help` or `--version` was silently ignored, left no
  subcommand, and fell through to start a real server on the default data, cache,
  and config directories - whose single-instance takeover then stopped the
  running production server. A stray flag can no longer start a server.

## [1.20.1] - 2026-07-15

### Changed
- **The "O" column logo now links to the model's OpenRouter page** (opens
  `openrouter.ai/<slug>` in a new tab) instead of copying the model id. Applies to
  both the desktop table and the mobile cards.
- **Pricing-note badge uses a cleaner note-document icon** in place of the "!"
  glyph. All behaviour (hover tip, click-to-edit modal, mobile touch target) is
  unchanged.
- **Details-modal JSON wraps on desktop too** (previously only on mobile), so a
  long JSON line no longer forces a horizontal scroll.
- **Page-length control now reads "N / page"** (e.g. "25 / page") instead of
  "Show N entries".
- **Mobile card rows 1 and 2 refined.** The date now sits inline after the
  capability icons on the line below the name. Row 2 is exactly three
  label-over-value columns - CONTEXT, PARAMS (total and active together in one
  cell), and SIZE - each with its uppercase caption on its own line above the
  value.

## [1.20.0] - 2026-07-15

### Changed
- **Capability icons now sit on their own line directly below the model name**,
  in both the desktop table and the mobile cards, instead of trailing inline to
  the right of the name.
- **Rating is now a dropdown that matches the Speed and OCR selects**, in both
  the desktop table and the mobile cards, replacing the click-to-open colour
  swatch. Its menu keeps the 0-4 red-to-green option colours as a cue; it still
  sorts numerically and saves through the same debounced queue.
- **Mobile card layout refined.** The date moves off the name row to sit beside
  Context, params, and size; Context, params, In, and Out gain short captions;
  total and active parameters read together as one "35B / 3B" cell; and the
  pricing-note badge stays inline with the unit text instead of dropping to its
  own line.
- **Mobile cards read as separate tiles on a neutral page.** The space between
  cards no longer shows a tinted colour band - the page takes the same tone as
  the cards and each card is defined by its own border and shadow, in both light
  and dark themes.
- **Mobile ZDR shows as coloured text only** (green "Yes" / red "No") with no
  filled cell background.

## [1.19.1] - 2026-07-15

### Fixed
- **Dev-run scripts now start the server directly and are reachable from a
  phone.** `start_dev.sh` and `start_dev_binary.sh` invoke the `serve`
  subcommand, so they launch the web server immediately instead of dropping into
  the interactive menu, and they bind `--host tailscale` (this machine's
  Tailscale address) alongside loopback, so the dev UI can be opened on a phone
  on the tailnet for mobile testing.

## [1.19.0] - 2026-07-15

### Added
- **Mobile card layout redesigned into a dense, multi-column grid.** Each model
  renders as a compact card with a fixed seven-row layout (name and date with
  quick-action icons; context, parameters, and on-disk size; input/output
  modalities; input/output pricing with the unit and note badge; notes; speed,
  OCR, and rating; ZDR and details), replacing the one-field-per-line stack.
  Empty fields collapse so cards stay tight.
- **Mobile sort and pagination bar** pinned above the cards: a field selector
  (name, date, context, parameters, size, input/output price, rating, speed) with
  an ascending/descending toggle, plus the page controls moved to the top. It
  drives the same ordering the desktop column headers use.
- **Copy button and wrapped JSON in the Details modal**, which now fills the
  screen on mobile so long payloads stay readable without horizontal scrolling.

### Changed
- **Cards read as separate raised surfaces**, with a dedicated card background,
  spacing, and border in both themes; the light theme also gains a modest
  contrast increase.

### Fixed
- **Input/Output modality filters appeared to do nothing on mobile.** The Select2
  dropdown opened behind the filter drawer; it now layers above the drawer, so
  tapping opens the options and filters the list.

## [1.18.2] - 2026-07-15

### Changed
- **Dev-run scripts reworked for safe local testing.** `start.sh` is now
  `start_dev.sh`: it runs the server from source (`go run`) in the foreground on
  port 8123, isolated to `./data`, and no longer adopts a built binary's
  config or port. A new `start_dev_binary.sh` runs the built `./dist/modelsdb`
  on port 8124 with its config, data, and cache dirs all pinned to `./data`, so
  exercising the shipped binary never touches the production data dir or stops
  the installed instance. Both dev scripts share `./data`, so only one runs at a
  time.

### Removed
- **`stop.sh`.** The foreground dev scripts stop on Ctrl-C; the installed binary
  still has its own `modelsdb stop` subcommand.

## [1.18.1] - 2026-07-15

### Fixed
- **Console error when a note textarea loses focus.** Committing a note edit with
  the Enter key detached the focused textarea, which fired a second synchronous
  blur that re-ran the display swap on the now-orphaned node and threw a
  `replaceChild` DOM error. A re-entrancy guard makes the swap and save run once.
  Notes save exactly as before on both the Enter and click-away commit paths.

## [1.18.0] - 2026-07-15

### Added
- **Six HuggingFace-direct models added to the catalog.** `google: Gemma-4-12B`,
  `google: Gemma-4-31B`, `google: Gemma-4-E4B`, `google: Gemma-4-E2B`,
  `unsloth: Qwen3.6-27B-MTP-GGUF`, and `Qwen: Qwen3.6-35B-A3B-FP8`, each with
  parameters, MoE/active-parameter facts, context length, input/output
  modalities, and native-precision on-disk size read from the HuggingFace repo.

## [1.17.0] - 2026-07-15

### Added
- **Mobile-friendly layout.** Below 768px the table reflows into one card per
  model, as a pure-CSS skin over the same single DataTable (no second render
  path), enabled by a `width=device-width` viewport meta tag so the breakpoint
  fires on real phones. Each cell becomes a labeled line, sourced from a new per-cell
  `data-label` attribute that is inert to filtering and sorting. The filter bar
  becomes an off-canvas drawer opened by a narrow-viewport-only **Filters**
  button (repositioned by CSS only, so Select2 and filter persistence are
  untouched), low-signal cells are de-emphasised, the empty pricing-note badge
  becomes an always-visible >=44px touch target, the pricing-note modal closes on
  an outside tap, and a narrow viewport pages 25 rows while desktop still shows
  all. Desktop layout and behavior are unchanged.

## [1.16.2] - 2026-07-10

### Added
- **`Max Size (GB)` filter.** A numeric filter in the filter bar hides models
  whose on-disk size exceeds the entered value. Models with no recorded size are
  kept in the results and sort to the bottom of the Size column.

## [1.16.1] - 2026-07-10

### Changed
- The table's on-disk-size column is now headed **`Size (GB)`** and shows each
  value as a whole number rounded up (e.g. `55.56` -> `56`), with the `GB` unit
  moved out of the cells into the header. The stored `disk_size_gb` value stays
  a decimal and still sorts by its exact size.

## [1.16.0] - 2026-07-09

### Added
- **`disk_size_gb` curated column - native-precision on-disk model size.** A new
  objective, manually-researched column recording the total size in GB of a
  model's native-precision weight files (safetensors / pytorch `.bin`) on the
  main revision of its canonical HuggingFace repo, excluding quantized/GGUF
  mirrors and non-weight files. It is threaded everywhere `parameters` flows:
  the DB schema (`disk_size_gb REAL`), a new backed-up migration
  (`schemaVersion` 2 -> 3, `addDiskSizeColumn`, idempotent), the objective
  import/export (`ImportFullRecord`, `CuratedBytes` - emitted only when set),
  the curated update/save paths (`UpdateCurated`, `SaveCurated` preserves it
  when a UI save omits it), and a new read-only "Size" column in the table
  (rendered e.g. `16.1 GB`, sortable numerically). The OpenRouter refresh never
  clobbers it (absent from the refresh upsert's `ON CONFLICT SET`). The
  `update-database-manually` skill documents its definition, the HF field-map
  row, the example INSERT, and a "find rows missing disk size" query.
  `CuratedMinAppVersion` is unchanged: the column is optional, so an older build
  simply ignores the extra key and the catalog stays readable.
- **On-disk sizes for the whole HuggingFace catalog.** Every model with a
  HuggingFace repo now carries `disk_size_gb`, researched from the repo's
  weight-file listing (native-precision safetensors/`.bin`; GGUF-only repos use
  their full-precision quant). One gated repo (`microsoft/WizardLM-2-8x22B`)
  could not be measured and is left blank.
- **New HuggingFace-direct models.** Added a batch of HuggingFace-only models the
  OpenRouter API does not list - spanning chat/reasoning, vision, embedding,
  OCR, TTS, translation, image, video, and encoder models - each with
  parameters, context, modalities, and on-disk size.

### Changed
- The default table column order places the OpenRouter (`O`) column immediately
  after the HuggingFace (`HF`) column; the `Size` column sits after `AP`.

## [1.15.1] - 2026-07-07

### Fixed
- **Header version could show a stale number.** The version label (and the
  update-available check) read the cached update-check result, so after an
  in-place update or a version change while the daily check was still cached, the
  UI showed the previously cached version instead of the running one. `current`
  and `available` are now always computed from the running binary; only the
  latest-release lookup is cached.

## [1.15.0] - 2026-07-07

### Changed
- **The embedded catalog is now the single published catalog.** `data/curated.json`
  is no longer git-tracked (the whole `data/` dir is git-ignored); the public
  catalog ships only at `internal/seedcatalog/catalog.json`. `build.sh`
  auto-refreshes that file from the live DB before building when one is present.

### Removed
- **Remote catalog seed.** The first-run fetch of `curated.json` from the GitHub
  repo is gone (`SeedFromRemote`, `fetchRemoteCurated`, `curatedRawURL`). A fresh
  install seeds from the embedded catalog instead.
- **First-run setup modal.** With the embedded seed, a fresh install populates
  itself automatically, so the setup prompt (`/api/setup`, `NeedsSetup`, the
  modal UI) is removed. The `first_run_complete` config field is dropped.

## [1.14.0] - 2026-07-07

### Added
- **Embedded seed catalog - first launch works offline.** The published catalog
  is baked into the release binary (`internal/seedcatalog`, `//go:embed`). A
  fresh install with an empty database seeds itself from the embedded copy on
  first launch, with no network fetch and no setup prompt; the startup OpenRouter
  refresh then updates the live models. The remote/setup seed remains as a
  fallback for builds that ship without an embedded catalog.
- **`modelsdb export [path]` command.** Writes the objective catalog (personal
  data and unlisted models excluded) to a file. Publishing a new catalog is now
  `modelsdb export internal/seedcatalog/catalog.json` from the repo root, then
  commit and build the release. With no path it rewrites the data dir's
  `curated.json`.

## [1.13.0] - 2026-07-07

### Added
- **Unlisted models (private-from-export flag).** A new `unlisted` curated flag
  (schema v2) keeps a model in the local database but **excludes it from the
  public `curated.json` export**, so a maintainer's private or experimental
  hand-added models are not published to the repo. Unlisted rows are withheld by
  `CuratedBytes`; everything else exports as before. The flag survives OpenRouter
  refreshes and is fully reversible.

## [1.12.0] - 2026-07-07

### Added
- **"O" column - copy the OpenRouter model id.** A new narrow column shows the
  OpenRouter routing logo for every OpenRouter-listed model (source `openrouter`
  or `collection`). Clicking it copies that model's OpenRouter id (e.g.
  `anthropic/claude-haiku-4.5`) to the clipboard. HuggingFace-direct and manual
  models have no OpenRouter id, so the icon is absent for them. The copy works on
  a plain-http bind (Tailscale/LAN) too, via a clipboard fallback.
- The `/api/models` response now includes the `id` field (the OpenRouter model
  identifier), alongside the existing `slug`.

## [1.11.0] - 2026-07-07

### Changed
- **"Update models" button.** The catalog-refresh button (top bar) is renamed
  from "Update" to "Update models", and its tooltip states it refreshes the
  catalog from OpenRouter and does not update the app itself. The separate app
  self-update banner (with "Update now" / "Restart to apply") still appears only
  when a newer release is available, so the two actions are no longer confusable.
- **Paths info box clarifies personal-data storage.** The info dialog no longer
  lists a non-existent `personal.json`; it adds a note that personal data (notes,
  ratings, favorites, speed, OCR quality) is stored inside the database only.

### Removed
- Stale references to a `personal.json` file in code comments. Personal data has
  no separate file - it lives only in `modelsdb.db`.

## [1.10.1] - 2026-07-07

### Changed
- **Separator between menu actions.** After an interactive menu action finishes,
  a horizontal rule and blank lines are printed before the menu redraws, in both
  the main menu and the "Add to PATH" submenu, so fresh output is visually
  separated from what scrolled past instead of merging into one wall of text.

## [1.10.0] - 2026-07-07

### Added
- **`host: "tailscale"` auto-detect.** Setting `host` to `"tailscale"` (in
  `config.jsonc`, `--host`, or `MODELSDB_HOST`) makes the server find this
  machine's Tailscale IP (`100.64.0.0/10`) itself and bind it, so you no longer
  hardcode the `100.x` address. If no Tailscale address is present, it logs a
  warning and stays loopback-only.
- **Refuse public binds by default.** The server now refuses to bind `0.0.0.0`,
  `::`, or any public address, because it has no authentication. A private,
  loopback, or Tailscale address binds normally; a public/wildcard one requires
  an explicit `--allow-public` flag, `MODELSDB_ALLOW_PUBLIC=1`, or
  `"allow_public": true`. The flag is forwarded to the background server.

### Changed
- **Lowercase data directories.** The per-OS config/data/cache directory leaf is
  now lowercase `modelsdb` (was `ModelsDB`); the product is still styled
  **ModelsDB** in the UI and docs. On a case-sensitive filesystem an existing
  capitalized `ModelsDB` directory is migrated to the lowercase name once on
  startup, so no data is orphaned.

## [1.9.0] - 2026-07-07

### Added
- **"Add to PATH" menu action.** The process-control menu can make `modelsdb`
  runnable from any terminal. On Linux/macOS it offers two methods: a **symlink**
  into `~/.local/bin`, or adding the binary's **folder to PATH** via the shell
  rc file chosen from `$SHELL` (bash -> `~/.bashrc`, zsh -> `~/.zshrc`, else
  `~/.profile`). On Windows it adds the folder to the **per-user PATH** in the
  registry. The two Unix methods are **mutually exclusive** - choosing one
  removes the other - and only ever the artifacts the app created (a fenced,
  marked block in the rc file; the exact `~/.local/bin/modelsdb` symlink; the
  binary's own folder in the Windows user Path). An **Uninstall** choice removes
  them. It takes effect in new terminals (a process cannot change its parent
  shell's live environment), and the menu says so.

## [1.8.0] - 2026-07-07

### Added
- **Background control built into the binary.** Running `modelsdb` in a terminal
  with no subcommand now shows an interactive menu: start in background, stop the
  background instance, start in the foreground, status (with open-in-browser),
  update models, quit. The same actions are available as script-friendly
  subcommands: `modelsdb start` (background), `stop`, `status`, and `serve`
  (force the foreground server). Background start re-launches the binary detached
  (a new session on Unix; a detached, window-less process on Windows), so it
  keeps running after the launching shell or window closes. No external scripts
  are needed.

### Changed
- A no-subcommand launch shows the menu **only** when standard input is a
  terminal. Non-interactive launches (a pipe, a service, `nohup` with stdin
  redirected) start the server directly, exactly as before, so existing
  automation is unaffected. `build.sh` now relaunches with `serve` explicitly.

## [1.7.0] - 2026-07-06

### Added
- **Configurable bind address (`host`).** A new optional `host` field in
  `config.jsonc` (also `--host` / `MODELSDB_HOST`) makes the server reachable on
  an address beyond loopback - for example a Tailscale `100.x` IP or a LAN IP -
  so the UI can be opened from another device. The default stays `127.0.0.1`
  (same-machine only), so nothing changes unless you set it. The server always
  binds loopback in addition to any configured host, so local tools and the
  single-instance check keep working. A wildcard (`0.0.0.0` / `::`) binds every
  interface. The app has **no authentication**: any non-loopback bind exposes
  full read/write (including the data-update and self-update endpoints) to
  anyone who can route to that address - the startup log and the API guide now
  say so.

## [1.6.0] - 2026-07-05

### Added
- **In-app binary self-update.** When a newer release exists, the update banner
  now offers **Update now** on the platforms the release publishes (linux/amd64,
  windows/amd64). It downloads the new binary, verifies it, swaps it in place next
  to the running executable, and then offers **Restart to apply**; the restart
  re-launches the process (execve on unix; spawn-and-exit on Windows). On any
  other platform the banner keeps the plain download link (notify-only). New
  routes: `POST /api/app-update/apply`, `GET /api/app-update/status`,
  `POST /api/app-update/restart`. This is separate from `/api/update` (which
  refreshes model data).
- **Signed-checksum verification (fail-closed).** Self-update verifies an ed25519
  signature over the release `SHA256SUMS` file (against a public key embedded in
  the binary), then matches the downloaded asset's SHA-256 against its line in
  that file. A missing/invalid signature, a missing `SHA256SUMS(.sig)`, or a hash
  mismatch aborts the update and leaves the running binary untouched. There is no
  unsigned fallback.
- **Automatic download opt-in.** An "Automatically download updates" checkbox in
  the info dialog (persisted via `/api/settings` as `auto_update`) makes the daily
  update check stage a verified update in the background. Applying it still needs
  an explicit **Restart** click; the app never restarts unattended.
- **`cmd/sign`** maintainer/CI helper (not shipped): `keygen` prints a fresh
  ed25519 key pair; `sums <file>` writes a detached base64 signature using the
  `MODELSDB_SIGN_KEY` secret. The release workflow signs `SHA256SUMS` and uploads
  `SHA256SUMS.sig`.
- New `internal/selfupdate` package implementing the fail-closed, OS-split
  self-update model.

## [1.5.1] - 2026-06-29

### Removed
- The `.githooks/` pre-commit secret scanner. The publish step already secret-scans
  the exact ship-set before anything reaches the public repo (`scan_secrets` in the
  deploy flow), so a per-clone commit hook added complexity without covering the
  real exposure. Secret protection at publish time is unchanged.

## [1.5.0] - 2026-06-29

### Added
- A shared, version-controlled **pre-commit secret scanner** under `.githooks/`.
  When [gitleaks](https://github.com/gitleaks/gitleaks) is installed it scans the
  staged diff before each commit and blocks the commit if a secret-shaped string is
  found (the value is redacted, never printed). It fails open - if gitleaks is not
  installed the scan is skipped and the commit is allowed - so it adds no friction
  for contributors without the tool. Activate it once per clone with
  `git config core.hooksPath .githooks` (documented in `CONTRIBUTING.md`). The hook
  is included in the public mirror so forks get it too. Bypass a single commit with
  `SECRETSCAN_SKIP=1 git commit ...`.

## [1.4.0] - 2026-06-26

### Changed
- A development run (`go run`, e.g. via `./start.sh`) now honours an explicit
  path override instead of always forcing `./data`. The default is still `./data`,
  but a `--config-dir`/`--data-dir`/`--cache-dir` flag, a `MODELSDB_*_DIR` env var,
  or (when the config dir holds a `config.jsonc`) its `data_dir`/`cache_dir`/`port`
  now take effect. `./start.sh` uses this: when a built binary's `dist/config.jsonc`
  is present it points the dev server at `./dist`, so iterating from source and the
  shipped build share one data store; with no `dist/config.jsonc` it falls back to
  `./data`.
- `./stop.sh` now finds the PID file in both `./data` and `./data/.cache`, so it
  stops the server whether the cache dir is the dev default or the location
  `dist/config.jsonc` specifies (`cache_dir "../data/.cache"`).

## [1.3.0] - 2026-06-26

### Added
- A **light/dark theme toggle** in the header (a sun/moon button next to the
  GitHub icon). The whole UI switches between the existing light theme and a new
  dark theme. The choice is remembered per-device and, on a first visit, follows
  the operating system's light/dark preference. The theme is applied before the
  page paints, so there is no flash of the wrong colours on load.

### Changed
- The light theme's panel/table surfaces use a softer off-white (`#ecf0f5`)
  instead of pure white.
- Table cell styling moved out of inline `style` attributes into CSS classes, so
  both themes are driven entirely by the stylesheet (no functional change).

## [1.2.2] - 2026-06-26

### Changed
- The manual-update guide now describes the pricing-note UI correctly: a note is
  shown as a bold orange "!" badge on a model's Unit cell (hover to read, click to
  edit), replacing the outdated reference to a separate "P.Note" column.
- Agent guidance now requires checking the manual-update guide whenever a change
  touches the data model, curated fields, or the UI that surfaces them, so the
  guide cannot silently drift out of date.

## [1.2.1] - 2026-06-26

### Changed
- Shortened the introduction in `CONTRIBUTING.md`.
- Broadened the ignore rules for local-only developer files (editor workspace
  files and machine-local configuration) so they stay out of version control and
  are never carried into the public mirror.

## [1.2.0] - 2026-06-26

### Added
- Columns can be **reordered by dragging their headers**. The chosen order is
  remembered across reloads and restarts (saved with the other UI state).
- The first-run "Ask an AI if this is safe" **Claude / ChatGPT links** now also
  appear at the top of the info ("i") dialog in the header.

### Changed
- The wide **P.Note** column is gone. A model that has a pricing note now shows a
  bold orange `!` badge on its **Unit** cell, so noted rows stand out at a glance;
  a model without one shows a faint `+` only when you hover that cell, to add one.
  Hover the badge to read the note; click it to open a small editor that stays
  open until you click away (so you can select the text), saving any change on close.

### Fixed
- The embedded UI is now served with `Cache-Control: no-cache`, so a browser always
  picks up a new `app.js`/`style.css` after the app is updated. Embedded assets have
  no modification time and so sent no cache validator, which could leave a stale UI
  cached after an update.

## [1.1.2] - 2026-06-26

### Fixed
- The portable config file is now recognized as `config.jsonc` (the project's standard
  extension for config files), in addition to the legacy `config.json`. Previously only
  `config.json` was loaded, so a `config.jsonc` next to the binary was ignored and the
  app fell back to the OS-default folders. The app now prefers `config.jsonc` and writes
  that name on first run; an existing `config.json` still works.

## [1.1.1] - 2026-06-26

### Fixed
- Build the Year filter's dropdown options with safe DOM construction instead of
  string-concatenated HTML. This clears a CodeQL "DOM text reinterpreted as HTML"
  (js/xss-through-dom) warning. Every other dynamic value in the UI was already escaped.

## [1.1.0] - 2026-06-26

### Added
- A GitHub link icon in the app header that points to the project repo.
- An "Is this safe?" section in the README, and a matching prompt in the first-run
  setup dialog, each with one-click links to ask Claude or ChatGPT about the project.
- A `SECURITY.md` explaining how to report security issues.
- An app icon. Windows builds now embed it in `modelsdb.exe`, and it is also the
  browser-tab favicon.

### Changed
- Rewrote the README, ARCHITECTURE, DESIGN, CONTRIBUTING, and CLAUDE docs in a
  simpler, clearer style.
- The "README" reference in the info dialog is now a link to the README on GitHub.

### Removed
- Dropped `jsconfig.json` and `package.json` from the published repo. They were
  editor-only tooling, so they no longer ship.
- Removed the SVG favicon in favor of the new icon.

## [1.0.1] - 2026-06-25

### Changed
- CI: bumped the pinned GitHub Actions to their current releases - `actions/checkout`
  to v7.0.0, `actions/setup-go` to v6.5.0, and `softprops/action-gh-release` to v3.0.1
  (each pinned by commit SHA).

## [1.0.0] - 2026-06-25

### Added
- Initial public release. ModelsDB is a single self-contained Go binary (no cgo)
  that serves a local web UI for browsing, rating, and annotating AI models.
- Catalog from OpenRouter's public API, enriched by scraping the six collection
  pages that surface model types the API omits (image, video, speech-to-text,
  text-to-speech, embedding, rerank), with per-model pricing scraped for the
  models the API does not price.
- Personal layer: notes, ratings, speed/OCR tags, and favorites, stored only in
  the local SQLite database and never written to any shared file.
- Read-only HTTP query API on a fixed loopback port for other local apps and
  agents, plus an agent-oriented markdown usage guide at `/api`.
- Durability and operability: schema migrations with automatic pre-migration
  backups and integrity checks, single-instance/one-port startup, runtime-resolved
  per-OS config/data/cache directories with a portable mode, a configurable
  upstream repo (the fork switch), a notify-only update check, and file logging.
- Public objective catalog export (`curated.json`) that seeds a fresh install,
  MIT licensed, with CI (build/vet/test) and tag-triggered release binaries.
