# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Every commit bumps the version in `VERSION` and adds a matching entry here.

## [10.25.7] - 2026-10-19

### Fixed

- **On a phone the sort bar no longer leaves the sort direction button alone on a second line at the left.** The bar now has two rows at every text size: Filters and the model counter on the first, the Sort label, the sort field list and the direction button on the second. The field list fills the space between the label and the button, and a long field name is cut with an ellipsis and shows in full when the list is open. The desktop layout and the table view do not change.
- **On a phone the closed filters drawer no longer draws a dark strip at the left edge of the page, and the keyboard no longer lands on it.** The drawer's shadow now shows only while it is open. While it is closed it is hidden from the Tab key and from screen readers, so Tab after the sort direction button goes to the first card control. Closing the drawer with its X or the dimmed backdrop puts focus back on the Filters button.

## [10.25.6] - 2026-10-19

### Fixed

- **Starting ModelsDB in the background now tells you when it did not start.** On a database this version refuses (for example one from ModelsDB 1.x), `modelsdb start` and the menu entry "Start in background" used to print "Started ModelsDB in the background" and exit with success, while the server stopped a second later and the reason appeared only in the log file. They now say the server did not start, print the same explanation the normal start prints, and `modelsdb start` exits with an error. The database is not changed. A start that works still returns with its success line after about three seconds, and does not wait for the catalog update.

## [10.25.5] - 2026-10-18

### Fixed

- **Moving through the column headings with the keyboard no longer sometimes leaves one hidden behind the Name column.** Pressing Tab or Shift+Tab several times in quick succession could, about once in sixteen stops, leave a heading underneath the Name column and the pinned headings out of line. The scroll bar above the table sent back an old position after the table had already moved on; it now ignores that echo. Dragging the bar, the mouse wheel and the scroll bar below behave as before.
- **The message shown when copying a model id is refused is shorter.** It now reads "Could not copy the id." so it fits on a phone without covering the top bar.

## [10.25.4] - 2026-10-18

### Fixed

- **The copy icon beside a model name now works from the keyboard.** Enter and Space on it copy the model id and show the same tick as a click; Space no longer scrolls the page. When the browser refuses the copy, a red message now says so, where before nothing happened. Click and tap are unchanged.

## [10.25.3] - 2026-10-18

### Fixed

- **Moving through the table with the keyboard no longer leaves a column heading hidden behind the Name column.** On a wide screen the Name column stays in place while the rest scrolls, and Shift+Tab to a heading could leave it wholly or partly underneath. With the page scrolled, so the headings are pinned at the top, the pinned headings could also drift out of line with their columns. After a keyboard move the table now scrolls just far enough to show the heading, and the pinned headings stay in line. A mouse click, the mouse wheel, the scroll bar, the phone cards and touch are unchanged.

## [10.25.2] - 2026-10-18

### Changed

- **The daily model update now goes easy on OpenRouter.** It sends its requests one at a time with about a second between them, so a run takes around 25 minutes instead of two. Nobody waits on it, because it shows nothing while it runs. A button press during it still joins the run in progress.
- **Every update, the daily one, the button and the one at start, now slows down when OpenRouter asks.** On a "too many requests" or "unavailable" answer it waits as long as OpenRouter says (at most a minute), or 2 seconds doubling each time, and tries again up to five times. A request that is still refused counts as failed, and a part with too many failures is named in Info as before. The button and the start-up update keep their speed otherwise.

## [10.25.1] - 2026-10-18

### Fixed

- **On a phone card the icons now follow the date one by one.** They were one block that sat beside the date only when the whole block fitted, and otherwise dropped to a line of its own, which looked random from card to card. Each icon now follows the date after a normal space and wraps by itself where the line ends, flush with the left edge of the name. A card can end with one icon alone on its last line. A mouse on a desktop and the table view are unchanged.

## [10.25.0] - 2026-10-18

### Added

- **The model catalog now updates itself once a day while the app runs.** The switch is in Info, under "Daily model update", and it is on until you untick it. It runs the same update as Update models and shows nothing while it runs. Afterwards a page that is open shows a bar saying the models were updated, with a Refresh button; it never reloads on its own. A run that fails shows its reason in the bar and in Info, and is tried again within the hour. Every update counts for the day, including the one at start and the button, so a run is never started right after another. If part of an update fails, such as the zero data retention or throughput figures, the bar and Info name the part.
- **Info shows when the models were last updated and how it went.**

### Changed

- **Bars at the top of the page no longer lie on top of each other when two show together and the page is scrolled.** They stack, and on a wide screen they stay in view directly below the top bar instead of sliding under it. On a phone they scroll away with the page.
- **The tabs in Info on a phone no longer cut words in half.** A tab that does not fit moves to a second row.

## [10.24.7] - 2026-10-18

### Changed

- **The default text on a phone card is smaller.** Names and values were 20 pixels and are now 18; captions, marks, units, chips and the buttons were 18 and are now 16. The Aa button's larger and largest steps are still 1.15 and 1.3 times the normal size, so they read about 20.7 and 23.4 pixels for names and values. The filters drawer, the views panel and the dialogs follow by the same ratio. A mouse on a desktop and the table view are unchanged.

## [10.24.6] - 2026-10-18

### Fixed

- **The HuggingFace link, the OpenRouter link and the copy button on a phone card are now finger-sized.** They were 15 to 31 pixels wide next to a pencil and an eye of 44 pixels, so a tap often hit the wrong one. Each now has the same 44 pixel box as its neighbours; the pictures keep their size. On the narrowest phones a card with all five buttons may take a second line. A mouse on a desktop is unchanged.
- **The OpenRouter link, the copy button and the eye button now show a clear outline when you reach them with the keyboard.** They showed only a pale tint, which is hard to see; they now carry the same blue outline as the pencil. Hover and mouse clicks look the same as before.

## [10.24.5] - 2026-10-18

### Fixed

- **A model name on a phone card no longer breaks early.** The last word of the name was stuck to the date in brackets, so when both did not fit, the word dropped to the next line with the date and left a gap on the line above. Now the name fills each line, and only the date moves down when it does not fit. The table on a wide screen is unchanged.

## [10.24.4] - 2026-10-17

### Fixed

- **The page buttons and the "N / page" menu are now centred on a phone.** The page buttons sat in the left half and "N / page" sat against the right edge of the screen, so the row looked shifted right and the word "page" touched the edge. The page buttons now take one centred row and "N / page" a centred row under them, above and below the cards, on phones and tablets. The table view on a wide screen is unchanged.

## [10.24.3] - 2026-10-17

### Changed

- **Cards no longer say how many empty fields they hide.** With Hide empty on, a card still leaves out its empty fields, and the line "1 empty column is not shown. Use the pencil to fill it in." at the foot of every card is gone. With Hide empty off, nothing changes.

## [10.24.2] - 2026-10-15

### Fixed

- **The Columns manager fits a touch tablet in card view.** On a 1024px tablet the manager showed its wide table, which at the bigger phone text needs more than 1000px, so it scrolled sideways and cut off the action buttons. On every touch screen showing cards the manager now uses its narrow two-line rows, the form it already used on phones and on narrower tablets. A mouse and a table view keep the wide table.
- **A live jump from a very wide layout to the screen's own width puts the text size back.** Going from Chrome's Desktop site width (or a zoom far under 100%) straight to the normal width could leave the page about 4 percent too large until the next resize, because the width was measured while the enlarged layout was still in place. The width is now measured with that enlargement taken off first, so the result equals a fresh load.
- **A change of the layout width keeps you on the same card.** When the browser changes the page width (Desktop site on or off, a zoom change), the card at the top of the screen is put back at the top. Before, the scroll offset in pixels landed hundreds of cards away. The address bar showing and hiding while you scroll changes only the height and now does no extra layout work.
- **At the bigger text, the Info tabs, the Info tables' headings, the code examples in the search help and the agent brief wrap inside their box.** The tab strip was cut on a narrow phone, the search help ran a few pixels past its box on the narrowest phones, and the tables and the brief scrolled sideways on tablets.

## [10.24.1] - 2026-10-15

### Fixed

- **The Aa button and the bigger text now appear whenever a touch screen shows cards.** In 10.24.0 they appeared only when the browser laid the page out 700px wide or less, so a phone that lays the page out wider (Chrome's "Desktop site", or a zoom under 100%) showed cards with small text and no Aa button. The width limit is gone. A touch tablet that shows cards gets them too. A mouse, and a view set to show the table, are unchanged.
- **Cards read at the same size on the glass however wide the browser lays the page out.** When the browser lays the page out wider than the screen, the page measures how much wider and grows its text, buttons and gaps by that much, so values read about 20px and captions about 18px on the screen itself at the normal step. The small icons (copy, edit, show and hide), the top bar and the sort bar keep the size and the line breaks they have on a phone laid out at its own width. Where the layout and the screen agree, as on a tablet, nothing is scaled.
- **The filters drawer, the views panel, the menu and every dialog fit the screen when the browser lays the page out wider than it.** The drawer was a fixed 340px (a third of the screen at the Desktop site width) and cut its contents off, the Copy agent brief button sat under the close mark of the details dialog, and the Columns dialog scrolled sideways. Their fixed sizes now grow with the page, and the width breakpoints (the phone rules, the Columns dialog's table at 961px) are measured against the screen instead of the layout, so a phone in Desktop site mode gets the same narrow forms as one laid out at its own width.

## [10.24.0] - 2026-10-15

### Added

- **On a phone, the cards use much bigger text.** On a touch screen up to 700px wide, in card view, names and values are about 20px and captions, marks, units, chips and every control (the top bar, the sort bar, the pager) about 18px, up from about 16px and 15px. The filters drawer, the views panel and every dialog grow by the same ratio.
- **An "Aa" button in the top bar steps the text size.** Each press goes normal, larger, largest, then back to normal. The step is remembered in the browser and applied before the page first draws. It appears on a phone in card view only.

### Changed

- **Card fields squeeze as the screen narrows.** A field used to be as wide as its whole caption ("CONTEXT LENGTH"), so few fit on a line. A field is now as wide as its value or its caption's longest word, whichever is wider, and the caption wraps between words inside it. More fields share a line and cards come out shorter. This applies on a phone only; a tablet and a mouse look as before.

### Fixed

- **On a phone, the column Colours dialog and the model details dialog fit the screen.** In the Colours dialog the "Below" figure box ran past the edge at the bigger text sizes, and a long column name could widen the dialog; the rows now wrap and long names break. In the details dialog the "Copy JSON" button sat on top of the close mark on a 360px phone and ran off the screen on a 320px one; the buttons now wrap onto a second line. A mouse and a tablet wider than 700px are not changed.

### Removed

- **The pinch re-fit of 10.23.0 is gone.** Chrome's own pinch and double-tap magnify work as they did before 10.23.0. The remembered pinch zoom and the switched-off double-tap are removed. On a touch screen, cards the screen is not showing are still not laid out, which keeps the first load and the Aa presses fast with every row on one page; the scroll bar's length stays an estimate until the cards have been drawn, as in 10.23.0. The Columns dialog fixes of 10.23.1 and 10.23.2 stay.

## [10.23.2] - 2026-10-15

### Fixed

- **The Columns dialog no longer scrolls sideways when nothing is open.** The explanation pop-ups behind each column's kind icon were invisible but still 300px wide, so they made the dialog a little wider than its box on a phone at any zoom and in a narrow window. They now take no room until shown. A pop-up that is shown is also fitted inside the dialog's width, so on a pinched-in phone it no longer runs past either edge. It also stays inside the screen vertically: it opens above its icon when there is room, below when there is not, and where neither side is tall enough it takes the larger side and scrolls inside itself.

## [10.23.1] - 2026-10-14

### Fixed

- **The Columns dialog works at the top of the phone zoom range.** Pinched in to about 1.6 times or more (the page about 260px wide), the strip with Shown, Undo, Redo and Reset order wrapped into a column taller than the dialog, so after a long press on a column no other row could be reached and the drag could not start. In that range the strip now scrolls away with the list instead of sticking at the top, so rows stay under your finger and a drag reorders. The search box, the Sort and Answers selects, the kind menu, the column title and the Add column button also shrink to the dialog's width there, so nothing runs past the right edge. A phone at normal size, a phone pinched less, and a mouse at any width look and behave as before.

## [10.23.0] - 2026-10-14

### Added

- **On a phone, a pinch re-fits the page the way zoom does on a computer.** While the fingers are down the page is still Chrome's magnifier, so a card can run off the right edge for that moment. When the fingers lift, the page lays itself out again at the new size: text larger, cards taller, everything wrapped inside the screen. Pinching back out returns to the normal size. The zoom stops where a card name would no longer fit (about 2.8 times on a 412px phone), is remembered in the browser between visits, and a pinch that starts inside a dialog or the filters drawer is left alone. A mouse is not affected at any width.
- Double-tap magnify is switched off on touch screens, so the pinch is the one zoom.

### Changed

- On a touch screen, cards the screen is not showing are no longer laid out. With every row on one page this makes a re-fit take about a tenth of a second instead of about a second. The scroll bar's length is an estimate until the cards have been drawn.

## [10.22.4] - 2026-10-14

### Fixed

- **The filters drawer, the views panel and every dialog are readable on a phone.** On a touch screen their labels, inputs, buttons, notes and count pills were 11 to 13px; none is below 14px now, and they still grow with the browser's text size. The count badge on the card screen's Filters button (about 11px) is raised to the same floor. The desktop table, a mouse on a narrow window and the cards keep their sizes.

## [10.22.3] - 2026-10-14

### Fixed

- **The small print on the phone cards is readable.** On a touch screen the field captions, the source marks, the price units, the "N of M" count, the Filters and sort controls, the pager and the page-size menu were 11 to 13px; none is below 14px now, and they still grow with the browser's text size. Dialogs, the filters drawer, the desktop table and a mouse on a narrow window keep their sizes.

## [10.22.2] - 2026-10-14

### Added

- **The Info dialog has a Searching tab.** It explains how to write a search for the boxes that read the search language: plain words, quoted phrases, `!` to leave out, `AND` and `OR`, the two rules about list columns and missing values, and how the API reads the same text.

## [10.22.1] - 2026-10-14

### Fixed

- **A long value list keeps asking the server while it is open.** In a text column with more than 200 values, a search that narrowed the list to 200 or fewer made the panel filter only that short list afterwards, so changing the text (for example from `a large language model` to `vision OR audio`) showed nothing until the panel was reopened.
- The Columns dialog search box now hints at the search syntax like the other boxes.

## [10.22.0] - 2026-10-14

### Added

- **The search boxes understand a small search language.** The name search, a
  column's text box, the value-list search, the Columns dialog search and the
  "file under an existing model" search all read the same thing: every plain
  word must appear, in any order; `"a phrase"` must appear as written;
  `!word` leaves a word out; `AND` and `OR` in capitals combine terms (AND binds
  tighter). Half-typed text is never refused. The API takes it too, through
  `name=`, `contains:` and `/api/columns/values?contains=`, and the served guide
  describes it.

### Changed

- **Plain words are no longer one exact piece of text.** `openai 4o` now finds
  the OpenAI GPT-4o models; before, it found nothing because the words were
  not next to each other. Put the words in quotes to keep the old meaning.
- **A `!` term matches a model with nothing in the column**, like `not:`, so
  `contains:!draft` includes models with an empty cell.

## [10.21.2] - 2026-10-13

### Fixed

- **A page opened with a filter in its address no longer starts at "43 of 43".**
  The count's total, the size of the whole catalog, now arrives with the first
  rows instead of a moment after them, so the first draw already reads
  "43 of 664" in the card and the table layout.

## [10.21.1] - 2026-10-13

### Changed

- **The "N of M" model count sits above the table.** In the table layout it
  moved out of the filters pane to the row above the table, beside the page-size
  menu, so it shows whether the filters pane is open or shut. The card and phone
  layouts keep their count in the bar with Filters and Sort.

## [10.21.0] - 2026-10-13

### Added

- **Every filter that offers choices can now pick the models that have nothing.**
  The first row of every value list is **Empty**, with the number of models that
  hold no value in that column (it was an uncounted "(no value)" row). Yes/No
  filters gain an Empty option beside All, Yes and No. Empty survives a reload,
  the address bar and a saved view, and Clear all resets it.
- **A Links filter.** One filter, in the "Where it lives" group, with two rows,
  OpenRouter and HuggingFace, each cycling off, include and exclude. Including
  both keeps a model with either link; excluding one keeps the models without it.
  It is the new `links` query parameter (`links=any:huggingface,openrouter`,
  `links=not:openrouter`), evaluated from the OpenRouter id and the repository
  at query time, so nothing new is stored. Any other value answers 400 saying what
  it takes. Because the word is now read by the query, no column can be called
  `links`.
- `GET /api/columns/values` also answers `empty`: how many models hold no value
  in the column, over the whole catalog.

## [10.20.0] - 2026-10-13

### Added

- **An EU column marks the models OpenRouter can route through its EU address.**
  Every Update models, and the refresh at start, reads OpenRouter's EU model list
  once and marks each OpenRouter model yes or no. The column is visible by default,
  next to ZDR. A yes means an endpoint for the model is onboarded in the EU; using
  the EU address needs OpenRouter's Business or Enterprise plan. If the request
  fails, or the list looks wrong (empty, sharing nothing with the feed, or as large
  as the whole feed), nothing is written and the stored values stay.

## [10.19.0] - 2026-10-13

### Changed

- **How a model is built is now one Architecture line.**
  The fifteen HuggingFace columns that held the layers, heads, cache size, weight
  type, file format and quantization are folded into that model's Architecture
  column (schema 52). A model with no Architecture entry gets a line made from
  what was measured; an entry that only said "unknown" is replaced by one; an
  entry with real text keeps every word and gains only the weight facts it lacked.
  A model that uses sliding-window attention, or a Qwen3-Next style model, gets
  its layer counts, hidden size and attention heads and no cache figure, because
  the stored cache figures for those over-stated the cache. Bits per weight are
  written only when below 16. Architecture is now a column the app ships, kept
  under its old key, and an assistant fills it from a repository's config.json
  as the update procedure says. An import no longer records any of it.
  The migration checks its own counts and restores the pre-migration snapshot if
  they differ. A saved view that still names a removed column loads as before.

### Removed

- **Eleven columns that lost nothing are gone:** the six State columns, Vision
  projector, GGUF total (legacy) after its five parameter counts moved into
  Parameters, Weights size (bytes) (Weights size (GB) stays), Build repository
  (the same as Repository) and Build label (part of Build format).

## [10.18.0] - 2026-10-12

### Removed

- **The Pricing note column is gone.**
  "Priced per" says what a price is measured in, and a caveat about a price is a
  line in the model's Note. A shipped column cannot be deleted from the page, so
  this build removes it from every catalog (schema 51). Nothing you typed in it
  is lost: each pricing note with text in it is first added to that model's Note
  (after the existing note and a new line, or as the whole Note when there was
  none, and not twice when the Note already says the same). The migration
  checks its own counts and restores the pre-migration snapshot if they differ.
  A saved view that still names the column loads as before.

## [10.17.3] - 2026-10-12

### Changed

- **Each price filter has its own Hide 0 tick, and it hides the 0 prices.**
  The tick under In and under Out used to be one shared "Hide free" filter that
  ticked both and hid only OpenRouter's free variants, so models showing 0 stayed
  in the table. A tick now belongs to its column: it hides every model whose price
  in that column is 0, plus the free variants as before, and combines with the
  column's own Min or Max. A model with no price in the column is kept. In the
  query API, `hide_free` takes a comma-separated list of columns
  (`hide_free=price_prompt,price_completion`); `hide_free=1`, which older links
  and saved views carry, still means both prices, so they keep working.
- **Pricing lines the In and Out columns cannot hold go in Note.**
  The served guide and the manual update procedure now say `notes` carries them,
  and that `pricing_note` is no longer used. The column itself stays.

## [10.17.2] - 2026-10-12

### Changed

- **The Columns dialog remembers its filters.**
  Search, Sort, Answers, Only shown, Subject and Written by come back as you
  left them when the dialog reopens, after a page reload and after a restart.
  A Clear filters button appears while any of them is set and puts them all
  back to their defaults. The selection and the help panel still start closed
  and empty.
- **The Columns dialog's close button stays in view.**
  The X sticks to the top right while the list scrolls, so the dialog can be
  closed from anywhere in it.

## [10.17.1] - 2026-10-12

### Changed

- **The filters pane puts each control beside its label.**
  Every filter is now one row: a smaller label on the left, its control on the
  right, so the pane is shorter. Text and number filters gain a visible label
  and a short hint ("Contains...", "Min...", "Max...") in place of the column
  name. A label too long for its column is cut with an ellipsis and shows its
  full text on hover. The list of values a list filter opens still spans the
  pane's full width below its row.

## [10.17.0] - 2026-10-11

### Fixed

- **A catalog with a column of your own named like a shipped column now upgrades.**
  A database at schema 33 to 39 whose own column shares a name with a shipped one
  ("Parameters (B)", "Weights size (GB)", "Author") and holds a typed value the
  shipped column cannot take, such as text where the shipped column is a number,
  stopped the upgrade at v40 and was restored to schema 36 with no way forward.
  That column is now kept whole, with every value, as a column of its own whose
  label ends in " (mine)", and the upgrade completes. An own column that holds
  no value is still absorbed as before.

### Changed

- **The way out for a v1.29.0 or v1.30.1 database is a published conversion
  version.** The refusal message and `docs/migration.md` name the pre-release
  `bridge-8.11.4` and give the steps: stop ModelsDB, copy the three database
  files, run the conversion version's `backup` command once, start this version.
  They say the old view settings are not carried over and that cells of retired
  columns are dropped while typed values are kept. The page no longer names a
  commit that is not in the public repository. The conversion version keeps a
  custom column of yours named like a shipped one beside the moved figures
  instead of overwriting its text, and `docs/migration.md` says a figure typed
  into an old fixed field comes back under that field's fetched source.

## [10.16.0] - 2026-10-11

### Changed

- **An OpenRouter rename updates the row you already have.** When OpenRouter
  renames a model, the refresh finds your row by its OpenRouter id and gives it
  the new name, keeping every note, rating and answer, instead of adding a second
  row and leaving the first frozen. The old name stops answering at
  `/api/models/{name}`. Where the new name already has a row holding the same id,
  the older row is folded into it: your answers move over, and a pair holding two
  different answers of yours for one column is left alone and named in the log.
  Only rows OpenRouter created are ever renamed or merged. The first rename or
  merge of a refresh writes a snapshot into the data folder's `.backups`; a
  refresh with nothing to follow writes none.

## [10.15.0] - 2026-10-10

### Added

- **Throughput column**, in tokens per second, shown in the table by default. Each
  refresh reads OpenRouter's per-model stats feed (the one behind its own model
  page) and stores the median speed of the fastest provider it measures for the
  model. A model no provider is measured for stays empty, and a model that stops
  being measured is cleared on the next refresh. The feed is not documented by
  OpenRouter; if it changes, only this column stops filling. The existing min and
  max filter and the column sort work on it.

## [10.14.0] - 2026-10-10

### Added

- **Hide free** tick in the filter of In $ and Out $. It drops OpenRouter's free
  variants (an id ending `:free` or a name ending `(free)`), shows the same state
  on both columns, and travels as `hide_free=1` in the address, the saved view and
  the filter count. A price of 0 alone does not count as free.
- **A From and To date picker** on Listed on, Added, Expiration date and
  Knowledge cutoff, in place of the value list. Both ends are inclusive days; the
  server takes `from:` and `to:` on those columns (`"date":true` in
  `GET /api/columns`) and refuses any day not written YYYY-MM-DD.
- **Table and Filter ticks** on every row of the Columns dialog. A column can
  leave the table and keep its filter; turning Filter off removes the control and
  clears its condition. Stored per view as `col_filter_off`; a view without it
  starts with each Filter tick equal to its Table tick.

### Fixed

- **The X in the Columns dialog closes it.** A click on the icon landed on the
  drawing inside the button and was ignored.
- **A reload on a bare address applies the saved filters** instead of showing
  them in the controls over an unfiltered table.
- **A link's filters no longer outlive the first filter change**, which used to
  put them back into the controls when a column was shown or hidden.

## [10.13.3] - 2026-10-10

### Fixed

- **A database at schema 33 to 36 can upgrade again.** Migration v40 refused to
  merge `mine.parameters` into `parameters` (and nine more such pairs) and the
  upgrade was rolled back. Those files carry the reader's own twin of a
  huggingface column with a blank unit where the huggingface column says
  billions, GB, bytes or bits, and the twin holds no value. The guard that stops
  two unlike figures sharing one column now skips a twin of the reader's that
  holds no cell. A twin that does hold a value in another unit still stops the
  upgrade, so nothing the reader typed is ever moved under a different unit.
  Every backup at schema 33 to 39 now upgrades to 50 with its models, hidden
  rows and own values unchanged.
- **Two backups taken in the same second no longer overwrite each other.** A
  timestamped backup, a pre-migration snapshot and the safety copy made by
  `restore` take the next free name (`-2`, `-3`) instead of replacing a file
  that already has theirs. The name is claimed with a hard link, so there is no
  gap between checking and writing. The single-file overwriting backup still
  replaces its own file, as designed.

## [10.13.2] - 2026-10-10

### Fixed

- **`modelsdb backup` no longer fails with `database is locked` while a server
  is refreshing the catalog.** The command opened the database the way a server
  does and so also ran the column seeding and source declarations at start-up.
  Those writes sit in a transaction that reads first, and SQLite refuses such a
  transaction at once, ignoring the 5 second busy timeout, when another writer
  holds the lock, which a server does during its start-up refresh. A backup now
  opens the database without those writes, so it only reads.
- **Three retired tables no longer come back, and the schema moves to v50.**
  `ignored_models`, `derived_columns` and `derived_inputs` are dropped by earlier
  migrations, but every open re-applied the floor schema and created them again,
  empty. The floor schema is now applied only to a file that still has migrations
  to run, and migration v50 drops the empty copies from existing databases (it
  refuses a table that holds rows). The schema migration takes the usual
  pre-migration backup.

## [10.13.1] - 2026-10-09

### Fixed

- **`restore` and `update` now refuse beside a server on any port.** They only
  looked at the port the command itself resolved, so a server started with a
  different `--port` on the same data folder was not seen and `restore` could
  overwrite the database under it. They now also read the data folder's own
  instance record, and the refusal names the port the server really holds. A
  record whose process is gone, or whose port is held by something else, does
  not block. `backup` keeps its rule (refuse only when a migration would run
  under a server) with the same detection.
- **A server is recognised when the data folder is given as a relative path.**
  The comparison of data folders now makes both absolute, so `--data-dir ./x`
  matches the absolute folder the server reports.
- **"Press Enter to close this window..." only appears on a real terminal.**
  Input redirected from `/dev/null` counted as a terminal, so scripts and
  pipes that hit a fatal error were made to wait for a key. The prompt now needs
  both stdin and stderr to be terminals.

## [10.13.0] - 2026-10-09

The fixes from the pre-release review: the server and its commands, and the page and documents.

### Security

- **A request must name this server.** Every request, reads included, is refused
  with 403 when its `Host` header is not `localhost`, a loopback address, an
  address a listener really bound, or the host of an `allowed_origins` entry. A
  web page that rebinds its own name to this machine can no longer read or write
  here. Behind a reverse proxy, list the public address under `allowed_origins`
  as before. A page opened by a machine name or a DHCP address that is not bound
  needs the same entry.
- **Request bodies are capped at 16 MiB** (413 beyond it), and the server has
  header, read, write and idle timeouts, so a slow or oversized request cannot
  hold it for ever.
- **No page can frame the app** (`X-Frame-Options`, `frame-ancestors 'none'`,
  `X-Content-Type-Options: nosniff`).
- **Back up now writes only into the saved destination folder.** A request naming
  another folder is refused.
- **Add to PATH can no longer delete the running program.** It never replaces a
  regular file, nor a link to anything but a modelsdb binary, and swaps its link
  into place in one step.
- **`restore` and `update` refuse while a server runs** on the data folder, and
  `backup` refuses when a migration would have to run under the server.
- **Takeover stops only the process the port answers for.** A recorded process id
  that has been reused by another program is no longer killed.
- **One plain browser User-Agent on every outbound request**, and bounded reads of
  every remote answer. A repository address is reduced to `owner/name` before it
  reaches an API path, a paging link may not leave the HuggingFace host, and an
  OpenRouter model id is escaped in the address built from it.
- The address of a private network in the config template and the tests is a
  documentation address.

### Fixed

- **Every OpenRouter row is retyped after a refresh**, including the ones the feed
  no longer lists under their stored name, so no row keeps a type derived before
  its kind was recognised.
- **The decision-model wording test matches only a statement of the model's own
  output** ("a probability for every option", "one logit per allowed option"),
  not a benchmark, a classifier or a reward model.
- **NaN, infinities and hexadecimal numbers are refused** in a number cell.
- The pre-migration snapshot is made by SQLite, so it holds writes still in the
  write-ahead log; snapshots and restores are readable only by their owner and
  replace a file in one step; a failed model count stops a migration before it
  starts; the database pragmas apply to every connection.
- The brief's example placeholders follow the column's kind of value, and its
  steps are numbered without a gap.
- A column cannot be called `name`, `include_hidden`, `fields`, `source`,
  `hidden`, `variant_of` or `_raw`, and one refresh makes at most two hundred columns
  from the feed.
- `kept` in an import answer lists only what was stored. A refused feed value
  costs only its own model. Two requests adding one model at once, or a build
  named like an ordinary model, are answered 409 instead of changing the other
  row.
- The research brief, the served guide and the import field list describe what the
  server really sends (`held`, no `shipped`, no `chosen`).
- Log lines carry quoted names and no longer carry search text; the log writer
  recovers after a failed rotation; config, cache and PID files are written in one
  step.
- **Escape while renaming a column no longer closes the Columns dialog**, and
  Escape with the Colours dialog open closes only that dialog.
- **A card shows its date once.** The rule that hid the duplicate date cell named
  a column key that no longer exists.
- **The in-cell text editor takes its intended height and wrapping.** Its style
  rule named a class the editor does not carry.
- **An undo step is recorded only for an edit that was saved.**
- **A late answer no longer overwrites a newer one** in the filter value list and
  in the variation picker.
- **A failed re-read of the rows, or of a row opened for editing, now says so**
  instead of failing silently.
- **Editing the backup folder posts one check, not two.**
- **The views limit message follows the limit.**
- **A coloured yes or no reads in its pill's colour.**

### Changed

- Comments, dead code and dead styles removed from the page; the page source is
  ASCII only.
- README, the documents under `docs/` and the served manual agree with the code:
  no files that do not ship, no counts or one-machine measurements, the decisions
  type, OpenRouter's embedding, rerank, speech and transcription types and the
  shorter research brief.

### Removed

- The `chosen` member of `GET /api/import-fields`, which was always true.
- Code nothing reached: the old-name value translation, the third `cells` writer
  and the helpers that only tests used.

## [10.12.0] - 2026-10-09

### Added

- **A model type for decision models.** A model whose output is a score or
  probability for each allowed option, not text, is typed `decisions`. OpenRouter's
  decisions models are re-typed by the next Update models, and a HuggingFace
  import recognises one by its `decision-model`, `typed-decisions` or
  `calibrated-probabilities` tags or by a model card that describes the output
  that way. A model that also answers in text gives out `decisions` and `text`.
- **The research brief offers only fields that can apply to the row.** No
  OpenRouter listing fields on a repository-only row, nothing foreign to the
  model's type, no build, GGUF or recurrent-state fields on a row without them.
  It shows what the row already holds, describes every field and its exact form
  (dates as YYYY-MM-DD), and says when a base model's card counts and that a
  wrong stored value is reported, not written. The feed-made columns now carry
  descriptions.

### Fixed

- **OpenRouter's embedding, reranking, speech and transcription models are no
  longer typed chat.** They become `embedding`, `rerank`, `tts` and `stt` at the
  next Update models.
- **A HuggingFace import keeps what the config states under an unrecognised
  task**, uses the transformers task when the pipeline tag is unknown, and adds
  audio and video inputs that the config or tags state.
- **A GGUF repository's import records its parameter count and architecture**,
  and every import records the base model the card or tags name.
- **An embedding model's context length is its `max_seq_length`** from
  `sentence_bert_config.json`, not the position table of the model beneath it.
- **The import answer's `kept` list names column keys.**
- **The brief no longer sends a reader to a re-import that cannot fill a field.**
  A GGUF repository is offered no weight size or file format (no file is "the"
  weights), and recurrent-state figures are offered only on a row whose config the
  importer read them from.
- **The `gguf_total_bytes` column is no longer written.** It held a parameter
  count under a name that said bytes; the Parameters column holds that fact.
  Stored cells are left as they are, and the column is relabelled "GGUF total
  (legacy)" and described truthfully. **The schema moves from 48 to 49**: the
  migration relabels that one column and clears its unit on a database carried
  forward, only while the label is still the shipped one, and touches no cell.
  A rollback needs the pre-migration backup restored over the database, not just
  the old binary.
- **The brief offers a context window and a mixture-of-experts answer only on
  language models**, and `supported_resolutions` only on video models.
- The served guide and manual agree on modality writes, the context window and
  GGUF repositories.

## [10.11.2] - 2026-10-09

### Fixed

- **The close X in the Columns dialog sits in the middle of its button.** It
  was drawn against the left edge, 11 pixels off centre. One shared rule now
  centres the contents of every dialog's close button.
- **The Filters badge always points at something marked in the panel.** Hidden,
  Edited by me and Variations are remembered with the view, so one could be set
  on a bare address and count in the badge with nothing in the panel marked.
  Each of the three now carries the same count pill as a column filter while it
  holds a value.

## [10.11.1] - 2026-10-09

### Fixed

- **Reading the catalog no longer stalls while Update models runs.** The
  database has one connection, and the read of every model's values asked for it
  once more for every list-valued cell, each time queueing behind the refresh's
  writes. A page load or a name search waited 10 to 16 seconds, and up to 55
  seconds on a fresh catalog. The lists are now read in one query, and the same
  request answers in about a second during a refresh.
- **A filter that matches nothing no longer shows the first-install panel.** The
  "No models yet" panel appears only when the catalog holds no models. A filter
  with no match shows the empty table and the did-you-mean suggestions.
- **The model counter's second number is the size of the catalog.** It read
  "311 of 311" under a price filter; it now reads "311 of 668". The catalog
  size comes from a new `top_level` figure in `GET /api/columns/answers`.
- **Opening `/api` in a browser no longer logs a 404 for `/favicon.ico`.** The
  server answers that address with the page's icon.
- **The HuggingFace import dialog clears its "Reading ..." line** when the
  import has finished.
- **The published source no longer points at maintainer task folders.**
  Comments and documents say what is true without the pointer, and the minified
  page assets folder is excluded from the public deploy.

## [10.11.0] - 2026-10-08

### Changed

- **Releases are now signed on the maintainer's own machine, with a new key.**
  The release workflow builds a draft release and no longer holds any signing
  key; `go run ./cmd/sign publish` checks the draft's binaries, signs and
  publishes it. The signed checksums file now names the release version, and
  the in-app update refuses a version that is not newer than the running one or
  that differs from the version the update check announced, so an old signed
  build can no longer be replayed under a newer tag. The update verifies the
  signed checksums before it downloads the binary, and caps every download's
  size. `sign keygen` no longer prints the private key: it writes it to a file
  (mode 0600, outside any git work tree) and prints only the public key.
- **Installs older than 10.11.0 must be downloaded by hand once.** They carry
  the previous public key, so they can no longer verify an update and will show
  "Update failed" until replaced by a download of this version.
- **Signing a release now proves the binaries come from the tagged source.**
  `sign publish` fetches the tag's commit, requires the signing key to be the
  public key that commit embeds, rebuilds both binaries from it and refuses
  unless their hashes equal the draft's, and re-checks the draft just before it
  is made public. The release binaries are now built with `-trimpath` and
  without cgo so the same commit and Go toolchain give identical bytes; the
  toolchain is pinned in `go.mod`.
- **Signing a release is safe against hostile code in the tagged commit.**
  `sign publish` now needs `-commit <hash>` and refuses unless the tag points
  there, rebuilds inside a bubblewrap sandbox that cannot reach the key, the
  network or the home folder, treats `VERSION` and `pubkey.go` in the commit as
  untrusted files, and builds with `-buildvcs=false` and the running toolchain
  so the bytes do not depend on where the source sits. Everything it runs from the commit has a
  time limit and a size limit on its work folder, and the output it shows from
  those commands is stripped of control characters and cut short.
- **The staged update file is created exclusively**, so a symlink planted
  beside the program is never followed. `sign` no longer refuses a key file on
  Windows, where file modes carry no meaning; it warns instead.

## [10.10.4] - 2026-10-05

### Fixed

- **A build whose repository states no task is typed like the model it was
  made from, not chat.** Reading a build's repository again wrote `chat` when
  that repository carries no task, or a task this app does not map, replacing
  the type the build had been given from its parent. A build filed under a
  model now takes that model's type in that case. A build whose repository
  does state a task keeps what the task says, and a row filed under nothing
  still reads chat. A row already wrong is put right by reading its repository
  again (Read the repository again, in the model's details).

## [10.10.3] - 2026-10-05

### Fixed

- **A HuggingFace music model is now typed music, not speech.** Importing a
  repository that HuggingFace files under text-to-audio always stored it as
  `tts`, so a music generator showed Type tts and a researched `music` answer
  was outranked by it. The importer now reads the opening of the repository's
  model card for an audio-out model and files it as `music` when the card
  describes music generation, using the same test that separates music from
  speech for OpenRouter models. A speech model stays `tts`, and so does one
  whose card cannot be read. A row imported before this change keeps `tts`
  until its repository is read again (Read the repository again, in the
  model's details).

## [10.10.2] - 2026-10-05

### Added

- **Shift with a Columns-dialog move button sends the column to the top or
  the bottom.** Holding Shift while pressing a row's up or down button, with
  the mouse or with Enter or Space, moves that column, or the whole selection
  with its own internal order kept, to the front or the end of the order in
  one step, instead of one place. It is one Undo step, it is announced like
  any other move, the selection stays selected, and a press that cannot move
  anything says why (already first, already last, or not in Table order). The
  button's tooltip and the dialog's help list both mention Shift.

## [10.10.1] - 2026-10-05

### Changed

- **Active parameters is now a researched column only.** It used to list
  HuggingFace as a source, but nothing ever read that figure from a
  repository, so every stored value was researched. The column now says so,
  which means a researched value can no longer be outranked by a source that
  never states it. A build still receives its parent's active count; it is
  now recorded as research instead of as a repository's answer.

## [10.10.0] - 2026-10-05

### Added

- **The views can now be put in order.** Every view in the Views panel has a
  Move up and a Move down button, reachable with the keyboard. The new order
  is stored and survives a reload and a second tab. The first move stores a
  place for every view at once, under a new `view.<id>.position` setting
  listed in the served API guide; until then the list keeps its oldest-first
  order. A view made later arrives at the end, and a deleted view that is
  brought back by Undo returns to the place it had. Moving the first view up,
  the last view down, or any view while another is being renamed is refused
  with a sentence in the panel rather than by a disabled button, and each
  move is announced to a screen reader.

## [10.9.5] - 2026-10-05

### Added

- **A failing daily backup now shows on the main page.** When the daily
  backup is switched on and its last attempt was refused or failed, a bar at
  the top says so, gives the reason the backup reported, and has an Open Info
  button that goes to where the destination is set. Before, the reason was
  only visible inside the Info window, so a wrong folder could fail every
  night unseen. The bar has no dismiss control: it goes away on the next
  load after an attempt succeeds, and it never appears for a backup that is
  off, has never run, or last finished ok or with nothing changed. No new
  route and no new setting.

## [10.9.4] - 2026-10-05

### Changed

- **The manual procedure now teaches the reference-image columns.** It names
  `reference_images`, `max_reference_images` and `frame_images`, says that
  OpenRouter states the first two for an image model but not for a video
  model, and says that only an explicit statement on the model's own page or
  card counts: a pricing key named `reference_images` is not one. The served
  copy and the skill copy changed together.

## [10.9.3] - 2026-10-05

### Fixed

- **The model details window and the Columns window now behave as modal
  windows for a keyboard or a screen reader.** Both are marked as modal, so
  a screen reader no longer reads the page behind them, and Tab and
  Shift+Tab now stay inside the window instead of walking out into the
  table. Closing the details window already returned focus to the control
  that opened it, and still does.

## [10.9.2] - 2026-10-05

### Changed

- **The stylesheet no longer carries rules for controls the page stopped
  drawing.** The pricing-note badge and its two floating cards, the old
  note and favourite marks, the capability glyph classes and the modality
  checkbox panel had no element left to style. Nothing on the page changes:
  every element's computed style and size measured the same before and
  after, in light and dark.

## [10.9.1] - 2026-09-26

### Fixed

- **"Did you mean" now appears on the first load, not only after a filter
  change.** Opening the page on an address whose `name` matched no model left
  the table empty with no suggestions, because the suggestion panel was only
  ever asked for after a filter change went through `applyFilters`; the first
  load reads its rows a different way and never reached that call. The first
  load now asks the same, unchanged suggestion function with its own query
  and rows once its own setup has run.

## [10.9.0] - 2026-09-24

### Added

- **A model can now be found by a name that is close but not exact.** A new
  read-only `GET /api/models/similar?name=<...>` route ranks the catalog by
  how close each name is to the one given - the same matcher the app already
  used to suggest a column key for a misspelt query parameter, extended to
  fold a name into parts first, so `Qwen3-32B` and `Qwen-3-32B` are
  recognised as the same name written two ways. The search box now offers
  "Did you mean" suggestions when a search finds nothing, and
  `POST /api/models/add` and `POST /api/models/import` carry a `similar`
  advisory on every successful write, naming rows already here that look
  close to the one just created - a hint the exact duplicate check (unchanged)
  cannot give, since it refuses only a matching repository, OpenRouter id or
  identical name.

## [10.8.1] - 2026-09-22

### Fixed

- **The Build format cell no longer doubles the format word on a non-GGUF build.**
  `FP8 8-bit` and `AWQ 4-bit` are stored as such again, where they had read
  `FP8 FP8 8-bit`: a non-GGUF build's label already carries the format word, and
  the code was prefixing it a second time. A GGUF build's Build format, which
  reads `GGUF Q4_K_M`, is unaffected.

## [10.8.0] - 2026-09-21

### Added

- **The refresh reads OpenRouter's video and image listings, beside the general one**, and joins
  each record onto the row the general listing already made, by id. Every field either listing
  states becomes a column, the way every general-listing field already does, with the video
  listing's `pricing_skus` treated as one price-list column rather than one column per SKU key.
- **Which frame images a video model takes** (first frame, last frame, or both), and, for an
  image model, **how many reference images it takes at most and whether it takes any** - the one
  fact OpenRouter states structurally today. A video model's own reference answer, where
  OpenRouter states none, waits on a manual pass.

## [10.7.1] - 2026-09-19

### Changed

- **The red disagreement mark never appears on Description or Listed on.** Each source writes
  its own description and dates its own listing, so a difference there said nothing. It stays
  everywhere else, as he ruled.

## [10.7.0] - 2026-09-19

### Added

- **A Cards below control in the filter panel**, stored with the rest of the view. He picks how
  narrow the window has to get before the table turns into cards - 1642 (the old fixed width),
  1400, 1200, 1000, 800, or never - and the page switches at that width and nowhere else, live,
  with no reload.

## [10.6.3] - 2026-09-19

### Fixed

- **Who said it names the source on every filled cell**, not only where two sources could have
  answered; ticking it used to show little but the dots on his own columns.
- **No second edge line beside the arrow on a variation card.** The table's first-cell edge had
  started outranking the card's reset.

## [10.6.2] - 2026-09-18

### Fixed

- **Add model takes every field again.** 10.4.0's "the page never types into a fetched cell"
  read a brand-new row's blank history as "nothing here can ever be typed", so every fetched,
  measured or worked-out field on the custom-model form drew as a read-only line and only the
  name and his own columns could be typed. Add model now has its own case: every field but
  `added` (stamped by the app, never typeable) is an input again, landing as research exactly
  as before. The Edit dialog and the table keep last week's rule - a cell already on file is
  read-only there, filled or empty.

## [10.6.1] - 2026-09-18

### Fixed

- **In the dark theme the parent of a group is darker than its variations**, as in the light
  theme. 10.3.3 had brightened it and swapped the card tints the wrong way round there.

## [10.6.0] - 2026-09-18

### Changed

- **The refresh now reads every output kind OpenRouter lists, not text alone.** OpenRouter's
  `GET /models` route answers filtered to `output_modalities=text` by default, which hid every
  embeddings, image, video, transcription, speech, rerank and decisions model from the catalog.
  The refresh now reads `?output_modalities=all` on the same route, in the identical record shape,
  so those models arrive too: about 145 more rows on a dry run against a copy of his database,
  visible and priced (an embeddings model's prompt price lands in `In $` the same way a chat
  model's does).

## [10.5.1] - 2026-09-18

### Fixed

- **The Columns dialog's close button reads as a control now, not a bare glyph.** It had no
  border and no fill at rest, so it floated in the corner as an unbounded X (his rejection,
  2026-09-18: "How could you have created the big X close button on the Columns modal like
  this?"). It keeps the 44x44 floor and its position, and gains a 1px `var(--text-muted)` border,
  an 8px radius and a `var(--surface)` fill matching the dialog, with its icon down from 22px to
  20px and its stroke from 2.4 to 2 so it no longer overfills the box.

## [10.5.0] - 2026-09-18

### Changed

- **A list of price windows or leaderboard rows in the OpenRouter feed is now one list column,
  one member per window or row, in place of one column per key inside it.** The old walk took
  only the first window that stated a given key, so a cell could pair one window's price with a
  different window's hours; `pricing_overrides` and `benchmarks_design_arena` now hold every
  window or row whole, each its own `key=value` join in the feed's own order. The schema moves to
  48: the eleven old `pricing_overrides_*` columns and any `benchmarks_design_arena_*` column are
  deleted with their cells.

## [10.4.0] - 2026-09-18

### Changed

- **The page never types into a fetched, measured, derived or stamped cell, filled or empty.**
  His answer B to D3: an empty one used to open as a researched value; now it stays read-only,
  in the table and in the Edit model dialog, until a source states it. A cell already holding a
  researched value is unchanged - it stays editable until a source overtakes it. **Add model**
  is the one exception: every field on a brand-new row still lands as research, per his standing
  rule that adding a row means all fields. The server's `POST /api/save` is untouched - an
  assistant over the API still fills an empty fetched cell in as research.

## [10.3.5] - 2026-09-18

### Fixed

- **The Columns dialog keeps every column on one line until the window is under 960px.** The
  two-line row used to start at the table's card breakpoint (1642px), far wider than nine cells
  need, so his own window saw a name with the details stacked under it.

## [10.3.4] - 2026-09-18

### Changed

- **The Columns dialog's Measured icon is the HF mark** the Name cell already uses, as he asked,
  in place of the ruler.

## [10.3.3] - 2026-09-18

### Changed

- **The parent of a group is the darker row, its variations the lighter.** The two tints are
  swapped in the table and on the cards, in both themes, as he asked.

## [10.3.2] - 2026-09-18

### Changed

- **The variation arrow is a tree connector.** A line comes down from the parent row and turns
  into a head pointing at the variation's name, the direction he asked for.

## [10.3.1] - 2026-09-18

### Fixed

- **Three defects the tester found in the Columns dialog redesign (10.3.0).** A row on wide was
  73px, double its 36px target, and the first row sat 301px below the dialog's top: the order and
  actions cells set `display: flex` on the `<td>` itself, which drops a table cell out of table
  layout; the flex now lives on a span inside each cell instead. Closing the rest of the 41px gap
  in the first row's position (still over the 260px cap after that fix) meant tightening the
  pre-existing "Make a column of your own" form on wide screens: its two fields now sit on one
  line instead of two. Ctrl-clicking a row's title to select it left focus on `<body>` (the title
  span is `tabindex="-1"`, which a click never focuses) with Escape bound only to the dialog
  element, so neither the selection nor the dialog could be dismissed by keyboard; every route
  into a selection now focuses the row's own tick, and Escape is bound to the document for as
  long as the dialog is open. A phone-width title's `min-width: 160px` lost a specificity fight to
  the narrow block's own `min-width: 0`; the phone rule now matches that selector's specificity.

## [10.3.0] - 2026-09-18

### Changed

- **The Columns dialog, rebuilt to his specification.** A leaner toolbar (search, Sort, Answers,
  a Subject dropdown, a Written by dropdown, an Only-shown eye toggle, a (?) help toggle) replaces
  the old controls strip; the help toggle opens a list holding everything that used to be prose
  above the table, the view line and the five kinds each as an icon, a phrase and a sentence. The
  Subject and Written-by filters are checkbox dropdowns, closed by default, reading "Subject: All",
  a name or two, or "N of M" on the button. The list is one row shape at every width - nine cells,
  tick through Actions - that reflows into a two-line card below 1642px instead of a second row
  builder; the answers cell is now a bare number with the scope on its hover, and the written-by
  cell is a 16px icon (the OpenRouter mark, a ruler, fx, a padlock, a person) whose tip carries the
  full sentence. Rename, colours and delete are icon buttons, behind a More disclosure only at
  700px and under.
- **Moving a column animates and drags.** Up and down still work, now with a FLIP slide and a
  fading highlight on the row that moved; a row can also be dragged by its own surface (holding the
  mouse button down, or a 500ms press on touch, which selects the row so a second finger's tap adds
  to it before either one is dragged). Ctrl+click and Shift+click select and extend a range on
  desktop; ArrowUp/ArrowDown on a row's tick walk the list. The Select button, the grip and the
  pick-up-and-place route are gone - a click starts a drag directly, or Ctrl-click selects, and
  reordering is refused (with a spoken reason) under any sort but Table order.
- **The Columns dialog's close button** is 44x44 at every width, with a 22px X in the text colour
  rather than the muted grey the smaller dialogs use.

## [10.2.0] - 2026-09-18

### Added

- **The filter panel redesign.** A section head now reads as a heading (weight, capitals, an
  accent bar) instead of a smaller font than the control titles under it, and a `--filters-divider`
  token draws a visible line between the action row, the fixed controls and each section, clearing
  3:1 contrast in both themes. A small pill under the top toast says what a filter change just did
  to the row count - what changed, then the count - and fades out after 3.2 seconds; the same
  sentence reaches a screen reader through the existing polite region. A new "Full text" tick,
  stored with the view like Hide empty, turns off the 20-character cut on Notes, Pricing note and
  every text column of your own: the value renders in full, capped only by the width of the cell
  (28rem) so it wraps rather than widening the table.

## [10.1.0] - 2026-09-18

### Added

- **All and None in a value-list filter.** Between the legend and the list, they include or clear
  every value the panel is currently showing (a search narrows what they act on), the same as
  pressing every row by hand.

### Fixed

- **Compact controls apply only on a touch screen.** Below the card breakpoint the filter panel's
  inputs, selects and buttons, the top bar's buttons and the pager's buttons were all sized for a
  44px touch target the moment the window narrowed, whoever was holding it. They now grow only
  under `(pointer: coarse)`; a mouse on a narrow window keeps the table's own smaller controls.
- **A variation row's accent edge mark now wins over the table's own stripe, hover and
  sorted-column shading** in every state and both themes, and an L-shaped arrow rises from a
  variation's name to the parent it is filed under, in the table and on the card. The count in the
  Var column is drawn larger and bold.

## [10.0.4] - 2026-09-18

### Fixed

- **The page size you pick in the pager holds in every layout.** Below the card breakpoint it
  was pinned to 25, so Clear, a filter or a refresh put it back. The pager alone changes it now.
- **The red mark where OpenRouter and HuggingFace disagree** is drawn at the text's own size,
  filled red, centred on the line and set off from the value, and it stays on screen when the
  source marks are hidden: it is a warning about the value, not a note on who said it.
- **No separator line before a row's delete control.** The gap alone sets it apart.

## [10.0.3] - 2026-09-18

### Changed

- **The small marks beside a value that say who stated it are OFF by default.** With two automatic
  sources on most of the visible columns they sat on most cells. The **Who said it** tick in the filter
  panel turns them on, per view, as before.

## [10.0.2] - 2026-09-18

### Changed

- **Migration v47 drops a scraped value that duplicates the automatic answer.** A `collection`
  cell on a surviving column becomes research only where no OpenRouter or HuggingFace cell
  stands on that column and model, or where the automatic winner's text differs; a scrape
  holding the very text the automatic winner shows is dropped and counted as a duplicate, so the
  edit form does not offer to clear a value that adds nothing on hundreds of models. The
  migration's exact-count refusal counts the duplicates in the expected drop.
- **The start-up rewrite reapplies every shipped column's description** from the code, as it
  reapplies its sources, so a database seeded before a description changed reads the current
  one; headings, colours and visibility stay the reader's.

### Fixed

- The preamble served ahead of the manual-update procedure said a written value outranks every
  source; it now says what an assistant writes lands as research into an empty fetched cell,
  is replaced when OpenRouter or the repository states the fact, and is refused where an
  automatic value stands. A test holds it there.
- The guide's variations passage called the build columns custom columns; they are sealed and
  measured.

## [10.0.1] - 2026-09-18

### Fixed

- **A researched value an automatic source has overtaken can be cleared.** An empty value on a
  column research may fill is always accepted, automatic answer or not: it deletes the research
  cell and nothing else, so what the answers panel shows as no longer shown can be tidied away.
  A non-empty value there stays refused. `GET /api/models` names such columns in a new
  `overtaken` map, and the Edit form's read-only line for one carries a **Clear it** control.
- **A union column research alone fills is editable in the page.** `answered_by` says `merged`
  only where an automatic source contributed to the merge; research alone there answers
  `research`, so the page offers the editor rather than a read-only cell.

## [10.0.0] - 2026-09-18

### Changed

- **Fetched data is immutable; only your own columns are yours to edit.** The rule the data
  model rested on has changed. There are two kinds of column, and the line between them is who
  writes. A column OpenRouter states, a HuggingFace repository measures, or the app works out
  from one of those is maintained by an update and is never typed over: a save onto a cell an
  automatic source has stated is refused with 400 naming the source and its value. Where such
  a cell is EMPTY, what you type in the page or send through `POST /api/save` lands as a new
  source, `research`, ranked below every automatic source on every column, so it shows only
  until OpenRouter or the repository states the fact, after which theirs shows and yours sits
  beneath it, dated, in the answers panel. Your own columns (`notes`, `favorite`,
  `pricing_note`, every column you made) are written by nobody but you and no update touches
  them. `added` takes nothing typed at all. The order between two automatic sources is fixed
  in the code (OpenRouter first where OpenRouter serves the fact, the repository first where
  it is read for it) and reapplied to every database at every start, so no route changes it.
- **`GET /api/columns` says what every column is.** Each column carries `kind` (`fetched`,
  `measured`, `derived`, `stamped`, `yours`), `sources` in rank order, and `derived_from` on a
  worked-out column. `GET /api/models/{name}/answers/{column}` carries `written_at` per answer.
  `by:` takes any source the column declares, or `not_` followed by one, with `you` an alias of
  `hand`. `disputed` means OpenRouter and HuggingFace disagree, and nothing else.
- **The Columns dialog says what each column is and who writes it**, on every row and in a
  legend that is also a filter, beside a filter by subject. The Edit form draws a fetched cell
  holding an automatic value as a read-only line with the reason, an empty one as an input that
  saves as research, and your columns as inputs; the Add form offers every field. A click on a
  read-only cell says which kind it is. The answers panel lists every source's answer with its
  date and has no control but Close. A `Research` mark sits on every researched value, every
  mark now clears the contrast floor, and a **Who said it** tick in the filter panel hides them
  all, stored with the view like Hide empty. The delete dialog names your values and the
  researched ones separately.
- **Migration v46 re-stamps every hand cell on a fetched column as research**, dated as it was,
  with the cell count level; **migration v47 retires the `collection` source**: its cells on
  surviving columns become research unless you had typed over them, the three columns only it
  filled (`collections`, `price_display`, `pricing_url`) are deleted with their cells, and the
  migration refuses any other drop. The schema version is 47; rolling back needs the
  pre-migration snapshot as well as the old binary.
- **The served guide and the manual-update procedure describe the rule**: an assistant's write
  lands as research, cannot reach a cell an automatic source holds, and never creates or fills
  your own columns.

### Removed

- `GET, PUT /api/columns/{key}/sources`, the Order and Sources buttons and the source-order
  dialog: there is no precedence to manage.
- The `created` field of the save and add answers: a key naming no column is refused rather
  than minted, because making columns is yours alone.
- The `collection` source, the three columns only it filled, and `set_by_hand`.

## [9.20.0] - 2026-09-17

### Changed

- **Every field the OpenRouter feed carries is a column, made by the refresh.** Every refresh
  (`POST /api/update`, `modelsdb update`, the startup fetch) makes a column for every leaf path
  the feed carries that no shipped column reads, and fills it in that same refresh from the feed
  already in memory. The column is keyed by the whole path (`top_provider.context_length` is
  `top_provider_context_length`), headed by those words, made HIDDEN so no view changes until it
  is ticked, and SEALED like a shipped column, because the next refresh would make it again. A
  path a shipped column reads, the top-level `name`, a path with no value on any model (null or
  an empty array everywhere), and a path that is both a leaf and a branch of other paths make
  nothing. A path every model quotes as a numeric string, which is how the feed sends every
  price, is made as a number column and the quoted figures are accepted into it. In the Columns
  dialog a made column is found by its heading, its key or the feed's own path, and grouped by
  subject from that path: `pricing.` under Price, `architecture.`, `benchmarks.` and
  `reasoning.` under What it can do, `top_provider.` and `links.` under Where it lives.

### Removed

- **The "Import from OpenRouter..." dialog, its button, its Remove control, `GET /api/feed-fields`
  and `POST /api/feed-fields/import`.** Nothing is imported by hand any more. A column an earlier
  release let a reader import this way keeps its key, label and type, and is sealed on the next
  refresh; the new-and-vanished-field warnings after a refresh stay.

### Fixed

- **A feed path whose leaf is itself an array was stored as an array holding that array.**
  `reasoning.supported_efforts` would have been stored as `[["max","high","low"]]`, which reads
  back as one member. A list column now holds the array's members.

## [9.19.3] - 2026-09-17

### Fixed

- **The model details overlay could not be closed from the keyboard.** It never answered Escape,
  and its close mark was plain text that Tab could not reach. It now closes on Escape like every
  other dialog, the close mark is a real button with an accessible name that Enter and Space
  press, focus moves onto the overlay's title when it opens, and closing puts focus back on the
  model name it was opened from.

## [9.19.2] - 2026-09-17

### Fixed

- **Ctrl+Z on a button inside the model details overlay or the views panel undid a cell edit
  behind it.** 9.19.1 covered the native dialogs and the Columns overlay by name; these two are
  neither. The page's shortcuts now stand down while anything carrying the dialog role is
  displayed, as well as while a native dialog is open, and the details overlay carries that role
  as it should have, so the next overlay is covered by carrying the role rather than by a line of
  its own.

## [9.19.1] - 2026-09-17

### Fixed

- **Ctrl+Z on a button inside any dialog undid a cell edit behind it.** 9.19.0 stopped the leak
  for the Columns dialog only; the native dialogs (the add and edit model forms, the delete
  question, the answers panel, the source order, the colours editor, the feed fields list) leaked
  the same way, because a key pressed inside one still reaches the page. While any dialog is open
  the cell-edit shortcuts do nothing.

## [9.19.0] - 2026-09-17

### Added

- **Undo and Redo in the Columns dialog cover every layout change, over as many steps as were
  made.** A row's tick, the Shown tick over the whole list, a grip placement, Move up and Move
  down, Reset order and a heading drag on the table are each one step, and Undo walks back through
  them while Redo walks forward. Each button names what its next press puts back ("Undo: hid 12
  columns") and is dimmed when there is nothing. The steps belong to the view they were made in:
  a view switch or a reload empties them. Up to 50 steps are kept.
- **The Columns dialog's Sort gains Subject**, which groups the rows under the six headings the
  edit form and the filter panel use, in table order within each group. It is a way to find a
  column, so the move controls are refused under it as under every sort but Table order, and the
  Shown tick still acts on every listed row.

### Fixed

- **Ctrl+Z on a button inside the Columns dialog undid a cell edit behind it**, because the dialog
  is an overlay and the page's shortcut ignored only text fields. While the dialog is open the
  cell-edit shortcuts do nothing.
- **Import from OpenRouter left the page's column list one entry longer than the table until
  something rebuilt it**, so a tick after the new column's place reached the wrong column. The page
  now rebuilds after an import exactly as it does after Make a column of your own, and both read
  the table's sort before the order is reapplied rather than after.
- A tick in the Columns dialog saved the view twice: the redraw it makes fired the table's sort
  event, whose handler saved again. It saves once, and a save the server refuses now puts the tick
  back and says so, as a refused move already did.

## [9.18.7] - 2026-09-17

### Fixed

- **Pressing Import on a second field in the Import from OpenRouter dialog made every import form
  vanish.** The one form that travels under whichever row opened it was being destroyed when the
  list was rebuilt, together with its buttons, so the second row had nothing to draw. The form is
  now lifted out before any rebuild and moves to the row that was pressed, with Cancel and Import
  still working after any number of moves or searches.

## [9.18.6] - 2026-09-13

### Changed

- **The documents moved under `docs/` and were cut to overviews.** `docs/architecture.md`,
  `docs/design.md`, `docs/migration.md`, `docs/security.md` and `docs/contributing.md` replace the
  same files at the repository root. The architecture and design documents no longer restate what the
  code does; they say what the data looks like, who may write what, and where each part lives. The
  README is a user manual again rather than a feature tour. The message a below-floor database
  prints now names `docs/migration.md`.

## [9.18.5] - 2026-09-12

### Fixed

- **A column can be found in the Columns dialog by the feed's own name for it.** The Feed fields
  dialog told the reader that `hugging_face_id` already had a column, but that column is headed
  Repository, so searching for "hug" in the Columns dialog found nothing. Each column now carries the
  feed paths that fill it, the search matches on them, and the row shows the path beside the key so
  the match explains itself.

## [9.18.4] - 2026-09-12

### Changed

- **The description that appears when you hover a model name opens downwards when there is no room
  above it.** It used to open upwards always, so on a row near the top of the window its first lines
  ran under the top bar. It now measures the room above the name, down to the top bar or the heading
  that follows you down the page, whichever is lower, and opens below the name instead when it would
  not fit.

## [9.18.3] - 2026-09-10

### Fixed

- **The description that appears when you hover a model name was drawn behind the table heading.**
  Above the width where the Name column freezes in place, that column's cells each become a layer of
  their own, and the popup could not rise out of the cell holding it however high it was set. It now
  lifts its own cell while it is open, so it clears both the heading row and the floating copy of it
  that follows you down the page.

## [9.18.2] - 2026-09-10

### Fixed

- **The refusal to delete a model OpenRouter lists is now pinned by a test.** Every other test of that
  route sends the confirmation field, so the refusal could quietly move back behind the request for
  one and they would all still pass. It had already done exactly that once.
- **A note beside the Name cell's icon row stated one catalog's column widths as though they were
  every catalog's.** The reader sets their own column layout, so the figure that holds is the margin
  left over, not the widths.

## [9.18.1] - 2026-09-10

### Fixed

- **A build can no longer be recorded under a model that is itself a build.** The table draws a
  variation as a row under its parent, and such a row has no rows of its own, so the second build
  sat in the database and was drawn nowhere at all. Both ways of filing now refuse it and say what
  to do; filing a model under a variation was already refused. A row already in that state is not
  stranded: the mark on its own row still takes it back out.

## [9.18.0] - 2026-09-10

### Added

- **A model filed under another one can be taken back out**, from a mark beside its name on its
  own row. Filing was one press and nothing undid it, so a row filed under the wrong model was a
  hidden child of that model for good: deleting it used to be the way out, and since 9.17.0 a model
  OpenRouter lists cannot be deleted at all. The row keeps everything on it, the date it was added
  included, and it stays hidden until you press its eye, because whether a row is on your list is a
  separate decision. `POST /api/models/variations/unfile` is the route behind it.

### Fixed

- After an update that both added models and reported a change in the OpenRouter feed, the page
  said "reload to see the change" about rows it had already re-read in front of you.
- Asking to delete a model OpenRouter lists, without the confirmation field, answered by asking for
  the confirmation and only then refused the model. No confirmation can make such a row deletable,
  so the refusal comes first. This is an API caller's path; the page never sent that shape.

## [9.17.0] - 2026-09-10

### Changed

- **An update now stores every model OpenRouter lists.** A model the catalog does not hold is added
  as a row of its own, on the list rather than hidden, by the same write that refreshes a model
  already held. The catalog used to grow only when you picked a model by hand, so a new model was
  something you had to go looking for. A fresh install now fills itself on its first update.
- **The page says how many models an update added**, in a bar with one button that draws the
  **Added** column and sorts the table by it, newest first - the column ships hidden, so a number
  with no way to reach the rows would be no use. A count of zero says nothing at all, and the page
  no longer reloads itself when there is a count to show: the reload used to wipe the message a
  second after it appeared.
- **Every row carries a hide control beside its name**, visible without hovering, because putting
  models aside is now done in handfuls and a touch screen has no hover. A hidden row keeps the
  crossed-out eye that puts it back; exactly one of the two is drawn.
- **A model OpenRouter lists can no longer be deleted**, and the waste basket is not drawn on its
  row. The next update would store it again, so a delete would destroy your note, your star and
  every answer you typed and hand the row back minutes later. Hiding is what takes such a model out
  of the list, and it keeps everything. Every other row stays deletable, whatever its source.

### Removed

- **The "Add from OpenRouter" picker**, whole, with `GET /api/openrouter/models` and
  `POST /api/models/pick`. An update now stores every model the feed carries, so the picker had
  nothing left to offer.
- **The bar counting what OpenRouter offers and this catalog does not hold**, added in 9.16.0. That
  count is now always zero. The settings row it was kept in stays in the database, unread: removing
  a feature does not delete anybody's data.

### Fixed

- A failed read of the names already stored no longer passes silently. It used to leave the refresh
  with an empty list of what the catalog held, which would now report the whole feed as newly
  added.

## [9.16.0] - 2026-09-09

### Added

- The page now says when OpenRouter is offering models your library does not hold, and offers one
  click through to the **Add from OpenRouter** picker to decide about them. The number was only ever
  written to the server log before, so the only way to learn of a new model was to open the picker
  on the off-chance. It is measured every time the app reads the OpenRouter feed - at startup, on
  **Update models**, on opening the picker and after adding from it - and kept in the database, so
  the bar is drawn the moment the page loads without asking OpenRouter anything. A refresh still
  adds no model by itself: what enters the catalog is still chosen in the picker.
- The dismiss cross closes the bar for as long as the page is open. Nothing is stored, so it comes
  back on the next load with a fresh count: the app has nowhere yet to record a model you have
  ruled out.

### Fixed

- `start_dev_agent.sh` and `start_dev_binary.sh` were committed without their executable bit, so
  running either as `./start_dev_agent.sh` failed in a fresh clone.

## [9.15.4] - 2026-09-09

### Fixed

- On a narrow screen the top-right menu opened behind the page: its lower half was covered by the
  page-length control and the model list, so **Update models**, **Info** and **GitHub** could not
  be reached. The bar the menu hangs from is not sticky at that width, which left the menu with no
  standing of its own against the rest of the page.

## [9.15.3] - 2026-09-09

### Fixed

- The **Add a variation** dialog's "file a model you already have" list drew "Your catalog could
  not be read" instead of the catalog. It asked the server for six fields under the dotted
  namespace names the pivot abolished, and the server refuses an unknown field with 400.
- Ten more readings in the page asked for a cell key that no longer exists, and each fell back to
  nothing in silence: the model-name tooltip's description, id, slug and listing date, the
  repository link and the **Re-import from HuggingFace** button, the unit a price is drawn in, and
  the size a model's builds sort by.
- On a narrow screen, showing or hiding a column left the sort dropdown offering the set it was
  built from, so a column just switched on was the one thing that could not be sorted by. Renaming
  a column left its old name in that list. Both now rebuild it.

## [9.15.2] - 2026-09-08

### Changed

- `README.md`, `ARCHITECTURE.md` and `DESIGN.md` rewritten to describe the app as it is. They
  still described a startup fetch that fills the catalog, an Ignore list, a three-pass refresh,
  the `collections` and `pricing` CLI verbs, and a per-column "answered by" filter, none of which
  the app has.
- The **Update models** tooltip said the refresh pulls "catalog, collections, pricing". It now
  says it refreshes what OpenRouter states about the models already held, and adds none.
- The served manual-update guide said a row's `source` "decides how a row is REMOVED". Delete is
  the one removal verb, so `source` decides nothing. Both copies changed together.
- `CLAUDE.md` no longer points at `internal/catalog/collections.go`, which was removed in 9.15.0,
  no longer describes an automatic startup fetch, and writes the variation columns as the bare
  keys they are rather than as dotted ones.

## [9.15.1] - 2026-09-07

### Changed

- The agent rules say the catalog comes from OpenRouter's API alone, matching the removal of the two scrapes.

## [9.15.0] - 2026-09-07

### Removed

- The collection scrape and the model-page pricing scrape are gone. `POST
  /api/update` now runs the OpenRouter catalog fetch alone; the `collections`
  and `pricing` CLI subcommands, the `progressCollections`/`progressPricing`
  refresh phases, and `internal/catalog/collections.go` and
  `internal/catalog/pricing.go` are deleted outright, with no flag and no
  dormant path left behind. `store.SourceCollection` stays a declared source
  so every `collection` cell stored before this change keeps ranking and
  showing exactly as before; nothing rewrites or deletes stored data, and
  `schemaVersion` is unchanged. The served API reference (`apiGuide` in
  `internal/app/query.go`) and the two manual-update guide files
  (`internal/app/update-database-manually.md`,
  `.claude/skills/update-database-manually/SKILL.md`) are rewritten to drop
  every claim that a scrape runs, adds a row, or records a reading; the
  manual pricing workflow for API-absent models is unchanged.

## [9.14.0] - 2026-09-07

### Added

- Every refresh now records every leaf path the OpenRouter feed carries, not
  only the ones a shipped column reads (`internal/catalog/feedfields.go`,
  `internal/store/feedfields.go`, migration to schema version 45). The Columns
  dialog's new "Import from OpenRouter..." button opens a Feed fields dialog
  (`GET /api/feed-fields`) listing every recorded path with its kind, how many
  models carry it, a sample value, and whether it is already imported or has
  vanished from the feed. Importing a path (`POST /api/feed-fields/import`)
  creates an ordinary `openrouter` column, filled immediately from a live
  fetch and kept current by every later refresh; removing it goes through the
  ordinary column-delete route.
- The Update models result now warns when the feed gains or drops a field
  path since the last refresh (`GET /api/update/status`'s `warnings` field,
  surfaced as an amber status message rather than the success toast).
- `.claude/skills/update-database-manually/SKILL.md` (and its served twin,
  `internal/app/update-database-manually.md`) gained a short section pointing
  at the Feed fields dialog for a fact the feed carries but no column reads.
- The Feed fields dialog now shows "Column: <label>" for a path a shipped
  column already reads (`internal/catalog/feedcolumnmapping.go`), with no
  Import button on such a row, and the Update models warning lines are each
  prefixed "Warning: " so a new or vanished field is never mistaken for the
  success message.

## [9.13.1] - 2026-09-07

### Fixed

- The pricing scrape (`internal/catalog/pricing.go`, `RunPricing`) paces every
  page request one second apart, win or lose, instead of only after a
  success - the 2026-09-07 incident tripped openrouter.ai's volume block in
  under a minute at the old 150ms-after-success-only pace.
- A 403 or 429 on a page is retried up to three times (10s, 30s, 60s
  backoff, replaced by a `Retry-After` header under 120s), and once a page
  exhausts its retries the scrape stops rather than hammering every remaining
  page - they are reported as one "N page(s) not attempted after
  openrouter.ai refused requests" line instead of one 403 per page.
- `httpGetBrowser` (`internal/catalog/collections.go`) now returns the status,
  the first 200 bytes of the response body (whitespace-collapsed), and the
  `cf-ray`/`retry-after`/`x-ratelimit-*` headers on a non-200, instead of only
  the status - both the pricing scrape and the collection scrape now log what
  a block actually said.
- A collection listing whose model page OpenRouter serves as a 200 "404: Not
  Found" shell (`deepgram/flux-tts`, `fish-audio/s2.1-pro-free`) is now
  reported as "model page does not exist on openrouter.ai" instead of the
  generic "no pricing block found".

## [9.13.0] - 2026-09-06

### Changed

- The Columns dialog (`#col-manage-modal`) is near full screen width above a
  1642px breakpoint (`internal/app/static/style.css`), and above that width
  its row list is a real table with one short row per column instead of the
  tall block layout, which stays below the breakpoint. Resizing across the
  breakpoint with the dialog open redraws the list in the other shape without
  losing a tick, a selection, or a held column.
- The view line and the long grip/Move/Select hint above the rows collapse to
  one line each; the hint moves behind a `(?)` toggle (`#cmg-note-toggle`),
  closed by default. The held-back-by-Hide-empty note is now a warning glyph
  in the Visible cell, carrying the same sentence in its `title` and
  `aria-describedby`, so it survives without colour.
- Sorting the column list (`#cmg-sort`, and now a click on any sortable
  desktop column header) gained Key, Type and Answers alongside Table order
  and Alphabetical. Sorting is view only: it never writes `col_order` and
  never touches the real table's column order, and the grip, Move up/down and
  block-move controls disable themselves with a reason outside Table order.
- Adding a column (`.col-manage-new`, `internal/app/static/app.js`) moves to
  the top of the dialog and no longer reloads the page: the new column is
  spliced into `colConfig`, the dialog and the real table redraw in place,
  and every tick, selection and held column survives. Focus lands on the new
  column's label.
- The colour editor (`openColorEditor`/`closeColorEditor`) is now a second,
  stacked `<dialog id="col-color-modal">` instead of a panel at the bottom of
  the column dialog. Opening it focuses its heading; closing it by any of
  Save, No colours, Cancel, Escape or a backdrop click returns focus to the
  Colours button of the row that opened it. `saveColorRules` and the
  `PATCH /api/columns` payload are unchanged.
- Renaming a column in the desktop table is now inline: its pencil turns the
  Label cell into a text input, Enter saves through the same `PATCH
  /api/columns` path the narrow layout's prompt already used (now the shared
  `submitRename`), Escape cancels, and focus returns to the pencil either
  way. The narrow layout's prompt-based rename is unchanged.

## [9.12.1] - 2026-09-06

### Fixed

- `catalog.FetchFeed` (`internal/catalog/update.go`) now bounds the OpenRouter
  feed request to a 25-second timeout and its body to 32 MB, mirroring the
  HuggingFace fetch in `internal/catalog/huggingface.go`. A feed larger than
  the limit is refused with an error rather than silently truncated. Every
  caller - the startup fetch, `modelsdb update`, `POST /api/update`,
  `GET /api/openrouter/models` and `POST /api/models/pick` - inherits this.
- `POST /api/models/pick` (`internal/app/picker.go`) no longer overwrites a
  hand-added, huggingface, or variation row when its name matches a picked
  OpenRouter name: `store.ModelExists` is checked before
  `store.UpsertAPIModel`, and a matching name is skipped and reported in a
  new `"existing"` reply field instead of having its source and cells
  overwritten. `apiGuide` (`internal/app/query.go`) documents the new field.
- Removed dead residue left by the Ignore feature's removal in 9.12.0: an
  unreachable branch and stale error text in `internal/app/variations.go`,
  and stale comments in `internal/app/static/app.js`, `internal/app/index.html`
  and `internal/store/store.go`. Removed the dead `#ignored-table` and
  `.row-ignore` CSS from `internal/app/static/style.css`.

### Removed

- The `IGNORED_MODELS` entity from the on-demand database reference diagram
  (`internal/store/archdocs_gen_test.go`); the table was dropped in 9.12.0.

## [9.12.0] - 2026-09-06

### Changed

- **A refresh ("Update models") no longer adds a row.** `catalog.FetchModels`
  and `catalog.RunCollections` now update the OpenRouter and collection facts
  of models already in the library and add nothing; the only way a model
  joins the library is the new picker below.
- **The Ignore feature is gone**: the ignored-models panel, its two endpoints,
  its column, its table (`ignored_models`), and the migration that dropped
  `ignore.go` and `ignore_test.go`. Delete is now offered on every model in
  the table, with no ignore option in its dialog. `internal/dbcore/migrate.go`
  bumps `schemaVersion` to 44 to drop the table.

### Added

- **"Add from OpenRouter" picker**: a dialog, reached from the menu or from
  the empty-catalog panel on a fresh install, listing every OpenRouter model
  not already a row, with search, per-row and select-all/select-none
  checkboxes, and a live selected count. Submitting posts the checked names
  to the one route that now creates OpenRouter-sourced rows.
  `GET /api/openrouter/models` (`internal/app/picker.go`) answers the
  available list plus the current library size, and `POST
  /api/models/pick` adds the checked names. On a fresh install (an empty
  library) the picker opens with every row pre-checked.
  `internal/app/index.html` (`#pick-modal`), `internal/app/static/style.css`,
  `internal/app/static/app.js`.
- A model deleted after being picked from OpenRouter reappears in the picker
  and can be picked again, exactly as if it had never been added
  (`internal/app/picker_test.go`:
  `TestADeletedOpenRouterModelReappearsInThePickerAndCanBeRePicked`).

## [9.11.1] - 2026-09-06

### Fixed

- A hand-typed answer on a shipped column (the `.cell-hand` class) showed
  `--text-hand` in the desktop table but fell back to plain `--text` in card
  view below the 1642px breakpoint, because `#models tbody td`'s id-plus-tag
  selector there outranked the plain `.cell-hand` class. `internal/app/static/style.css`:
  added `#models tbody td.cell-hand` alongside the existing rule so the hand
  colour wins in both layouts.

## [9.11.0] - 2026-09-06

### Changed

- The per-column "Answered by you / Not answered by you" dropdown is gone from
  every column's own filter. One dropdown, "Edited by me", sits beside
  "Hidden" at the top of the filter panel instead, and shows only models with
  at least one hand-answered cell on a column the app ships (a reader's own
  custom columns don't count, because every cell there is theirs already). It
  filters client-side like Hidden does, persists the same way (address
  parameter `edited`, saved views), and an old saved view or link's leftover
  `by:` condition is dropped silently rather than applied with no control left
  to show it. `internal/app/static/app.js`: `isHandOnShipped`,
  `hasEditedByMe`, `#f_edited_by_me`'s `ext.search` predicate,
  `VIEW_TABLE_KEYS`. `internal/app/index.html`: the new `#f_edited_by_me`
  select.
- Every such hand-answered cell is drawn in a new colour, in both the table
  and the mobile card view (same markup): CSS token `--text-hand` in
  `internal/app/static/style.css`, class `cell-hand` set in `markCell` and
  `renderCell`. The `.prov-hand` dot beside it is unchanged.

## [9.10.4] - 2026-09-05

### Changed

- `GET /` and `HEAD /` answer a small self-contained HTML "starting" page
  while the server is still starting up, instead of the JSON `writeStarting`
  reply every other path still gets. The page polls `GET /api/health` about
  once a second and reloads itself once the server answers; after roughly 300
  tries it stops and asks for a manual reload. `internal/app/server.go`:
  `startingPageHTML`, `writeStartingPage`, `gatedHandler`.

## [9.10.3] - 2026-09-04

### Changed

- `.claude/skills/update-database-manually/SKILL.md` and its served twin
  `internal/app/update-database-manually.md` are cut from 694 lines to 499,
  under the 500-line documentation ceiling. Every route, column key and
  instruction is kept; the HuggingFace hand-reading section loses its worked
  examples and the rest loses repetition. The two files stay byte-identical.

## [9.10.2] - 2026-09-04

### Added

- `TestByReadsTheWinningSourceNotTheColumnsOwner` in `internal/app/query_test.go`
  exercises the `by:` query filter beyond `notes` and `added`: a shipped column
  OpenRouter answers that a hand answer sits over, a column of the reader's own,
  a flag column, and two `by:` tests ANDed on one request. No behaviour changes.

## [9.10.1] - 2026-08-28

### Fixed

- `start_dev_agent.sh` refuses a bare word as its first argument. It only ever
  runs `serve`, so `./start_dev_agent.sh stop` used to pass `stop` through as an
  ignored positional argument and start a SECOND sandbox server while the caller
  believed it had stopped one.

## [9.10.0] - 2026-08-28

### Changed

- Every document now describes the catalog as it is after the pivot: one column
  per fact under a plain key, with the source recorded on the cell and
  `column_sources` deciding which answer a read shows. `SKILL.md` and its served
  twin, `ARCHITECTURE.md`, `README.md`, `DESIGN.md` and `CLAUDE.md` are rewritten
  on that model, and the served preamble in `internal/app/skillguide.go` with
  them.
- The schema-document generator (`internal/store/archdocs_gen_test.go`) describes
  `sources`, `column_sources`, `cells.source` and `columns.resolve`, and no
  longer carries descriptions for the deleted derived-column tables.

## [9.9.0] - 2026-08-28

### Added

- The "answered by you" per-column filter now survives a reload. It is a real
  query parameter (`by:you` / `by:not_you`, on any column a hand answer may
  state) written into the address bar and the saved view exactly like every
  other filter, instead of a client-side pass over already-fetched rows.

## [9.8.0] - 2026-08-28

### Changed

- Schema migration v42 deletes every cell the pre-pivot derived-column
  subsystem left stamped `source='rule'`. Those cells stopped being read
  once the answers view (v41) picked the winning cell by source rank, and
  `rule` names no rank at all - they served an answer only when they were
  the sole cell on record for their column and model, which the migration
  checks first and refuses to run if it finds one.
- Schema migration v43 rewrites the reader's saved view settings
  (`col_hidden`, `col_known`, `col_order`, `order` in `app_settings`), which
  had been left naming columns by their pre-pivot dotted key since v41
  stripped that prefix from every column. A dotted entry naming a column
  that no longer exists is dropped instead of rewritten.

## [9.7.3] - 2026-08-28

### Fixed

- Two migration comments and one changelog line are written in ASCII, as the
  project requires of source code.

## [9.7.2] - 2026-08-28

### Fixed

- Migration v40 (the namespace pivot's collapse) no longer fails with
  `UNIQUE constraint failed: cells.column_id, cells.model_name, cells.source`
  when a pre-pivot binary wrote cells after v39 ran: it restamps every cell
  still carrying an empty `source` with the source its own column's
  namespace implies (the same mapping v39 used) before it widens `cells`'
  primary key and merges columns onto their survivors, so two such cells
  belonging to different members of one merge group no longer collide on
  the empty string they shared.

## [9.7.1] - 2026-08-28

### Fixed

- The order dialog (`#order-modal`) now draws "You", pinned at the top and
  never movable, carrying "Your own answer always wins. This app never
  replaces something you typed." A union column (`collections`,
  `input_modalities`, `output_modalities`) gets a "Sources" button in its
  place of "Order" in the column manager, opening the same dialog in a
  read-only, informational mode with no move buttons.
- The column manager's sort control (`#cmg-sort`) is down to two flat
  sorts, Table order and Alphabetical; the three "Source, then ..." options
  and the domain-sectioned accordion they drove are removed from both
  `index.html` and `app.js`, along with their now-dead CSS.
- The per-model disagreement panel (`renderAnswersDialog`) states, in these
  words, that a hand answer to a union column replaces every source rather
  than joining it: "You have set this yourself. OpenRouter and HuggingFace's
  own answers are no longer added in - clear your answer to bring them
  back." Its lead sentence for a non-union column with no hand answer now
  says "only one source below has answered so far" when that is true,
  rather than "the sources below disagree" when there is nothing to
  disagree about.
- Every column's filter now offers "answered by you" / "not answered by
  you" alongside its own value filter, read off the `answered_by` map
  already on every row (`GET /api/models`) rather than a new server
  parameter - this replaces the old habit of grouping the reader's own
  columns first.

## [9.7.0] - 2026-08-28

### Fixed

- The browser is repaired for the namespace pivot's server-side change (stage D):
  every read of the removed `column.namespace` and `column.routes_to` fields is
  gone from `app.js`, which had been printing literal "undefined" into the
  read-only reason under a locked cell and into the sort-column screen-reader
  label. Who can write a column is now read from
  `GET /api/columns/{key}/sources` (`sourcesOf`/`handDeclared`/`writeTargetFor`
  in `app.js`); what a column is about, for the filter panel, the column
  manager and the add/edit form, is a separate classification (`domainOf`)
  into six groups - The model, What it can do, Size, Price, Where it lives,
  Yours alone - replacing the five namespace sections.
- The add-model form showed only `mine`-namespace fields; every shipped field
  now appears, matching every other place the app lets a reader state what
  they know.

### Added

- A provenance mark beside any cell two or more sources could answer: a small
  dot for your own answer, the source's name where only one has, and a red
  warning where two or more fetched sources disagree and nobody has picked an
  answer. Clicking one opens a panel (reading
  `GET /api/models/{name}/answers/{column}`) listing every source's answer,
  with "Use this"/"Use this instead" and "Clear your answer" wired to the
  existing save path.
- An "Order" control in the column manager's per-row disclosure, for any
  column two or more fetched sources could answer: up/down buttons over
  `GET`/`PUT /api/columns/{key}/sources`, simpler than the column manager's
  own drag-and-keyboard reorder because this list never holds more than a
  handful of sources.

## [9.6.1] - 2026-08-28

### Added

- Restored two regression tests dropped during the stage D rewrite:
  `TestADeletedColumnStaysDeletedAfterARename` (a rename does not let a refresh
  recreate a column the reader deleted) and the old-parameter cut, now split as
  `TestTheOldNamedFiltersAreGone` and `TestZDRFiltersByItsOwnBareKey` because the
  pivot gave `zdr` a column under that bare name, so `?zdr=` is a real filter again.

## [9.6.0] - 2026-08-28

### Added

- **`GET /api/models/{name}/answers/{column}`** answers one model's disagreement
  panel for one column: every source the column declares, in rank order,
  whether or not it has answered, and which one wins.
- **`GET, PUT /api/columns/{key}/sources`** reads a column's declared sources in
  rank order and lets a reader reorder them. `hand` can never leave rank 0 or be
  dropped from the order; a union-resolve column (`collections`,
  `input_modalities`, `output_modalities`) has no order to set at all, and both
  routes say so rather than leaving it to be inferred.
- `apiGuide` (`internal/app/query.go`) is rewritten end to end for the
  namespace pivot: bare column keys, `resolve: first_stated|union`, hand as a
  source able to answer nearly every shipped column, and the two routes above.

### Fixed

- **`store.SetColumnSourceOrder` (`internal/store/provenance.go`) refused a
  plain swap of two already-declared ranks with a raw SQLite `UNIQUE
  constraint failed` error**, because it wrote the final ranks one row at a
  time inside the same transaction and `column_sources` carries `UNIQUE
  (column_id, rank)`. A first pass now parks every declared source at a rank
  no final rank ever reaches, so the second pass can never collide. Found
  exercising `PUT /api/columns/{key}/sources` against a live dev server.

## [9.5.1] - 2026-08-28

### Fixed

- **Migration v40 (`internal/dbcore/pivotcollapse.go`) is amended in place to
  fix two defects `vibe-tester` found in it, verified against the owner's own
  catalog: never on a database that matters, since [OBS] his live install is
  at schema 38.** Neither defect touches `internal/store`, `internal/catalog`
  or `internal/app`.
  - Every column now ends v40 with a `hand` row at `column_sources` rank 0,
    not only the eight of fifteen merge groups that happened to contain a
    `mine.*` member and not only at whatever rank that member's own position
    gave it. Without this, v41's `cells_source_is_declared` trigger refused
    every hand write to `zdr`, `repository`, `description`, `created_at`,
    `input_modalities`, `output_modalities`, `price_image_output` and
    `price_unit` on a database that had actually run the migration. `added`
    is the one column that takes no hand row, per criterion C16.
  - `collapseMergedColumns` now refuses to merge two columns stating one
    attribute if they disagree on value type or unit, rather than silently
    combining them - the fault that once made "author" mean the vendor on one
    row and the repository's republisher on the next. This replaces
    `TestEveryComputedColumnMergesOneFactInOneUnit`
    (`internal/store/shipped_names_test.go`), deleted in 9.5.0 with no
    successor; its replacement,
    `TestCollapseRefusesToMergeColumnsWhoseUnitOrTypeDisagree` and
    `TestCollapseAcceptsAllFifteenRealMergeGroups`, lives in
    `internal/dbcore/pivotcollapse_test.go` next to the migration it guards.

## [9.5.0] - 2026-08-28

### Changed

- **`internal/store` reads and writes cells by `Source` instead of by a
  namespace prefix baked into the column key, completing Stage B2 of the
  namespace pivot.** `WriteCells` now takes a `Source` (`SourceOpenRouter`,
  `SourceHuggingFace`, `SourceCollection`, `SourceHand`, `SourceApp`) rather
  than a namespace string; the write is refused unless that source is
  declared for the column in `column_sources` (checked in Go before the two
  SQL triggers `cells_source_is_declared` and `cells_source_never_moves`,
  added by v41, get a chance to refuse it a second way), and unless the
  primary key `(column_id, model_name, source)` accepts it. `mintColumn`
  replaces `mintMineColumn`: it creates a column and its rank -1
  `column_sources` row for `SourceHand` in one transaction, and refuses any
  source whose `Kind()` is not `hand`. Clearing a personal answer is now a
  DELETE of the `(column_id, model_name, 'hand')` row - never an empty save -
  so the next-ranked source's answer reappears on its own, with nothing
  rewritten. Reading goes through the new `answers` SQL view (lowest
  `column_sources.rank` wins); `ModelCells` additionally merges every
  source's list, in Go, for the three `resolve='union'` columns
  (`input_modalities`, `output_modalities`, `collections`) - unless the
  winning answer is `hand`, which stands whole rather than being merged in.
  `derived.go` and `transforms.go`, and every symbol they defined
  (`RoutingTargetOf`, `DefineDerivedColumn`, `RebuildDerivedColumn`,
  `RebuildEveryDerivedColumn`, `DerivedDefinitionOf`,
  `ErrColumnIsAnInput`, `ErrNoSuchTransform`, and the derived-column branches
  in `cells.go`'s `WriteCells`/`DeleteColumn`), are deleted: nothing in the
  new schema computes a column from others any more. Every dotted literal
  (`"mine.rating"`, `"openrouter.context_length"`, ...) is gone from
  `store`'s own logic; `fromcells.go` is kept working as the network-shape
  bridge for `catalog`/`app` (Stage C's to delete), translating at its edge
  instead, and its `firstStated(cells, "mine.price_image",
  "collection.price_image_output", "openrouter.price_image")` bug - which
  read `price_image_output` into `price_image` - is fixed by reading each
  price under its own bare key. `columnByLabel`'s case-insensitive
  uniqueness check now looks across every column, not only the reader's own,
  matching the unique index the database already enforces.
  `columnsStatingTheSameFact` (`gaps.go`) and `custom_columns.go`'s ownership
  check are rewritten for a column having several sources rather than
  several columns having one namespace each. `internal/store`'s and
  `internal/dbcore`'s test suites pass in full
  (`go test ./internal/store/... ./internal/dbcore/... -count=1`); `go build
  ./...` and `go vet ./internal/store/...` are clean.
  `internal/app/column_identity_test.go` does not compile against the new
  `store.Column` (it still reads a removed `Namespace` field) - its premise,
  one namespace owning one column, is obsolete under the pivot, and fixing
  it is a `store`-consuming app-layer redesign left for Stage C/D rather
  than a mechanical rename. Verified against a copy of the owner's live
  catalog, migrated through v39-v41 and read back through the new
  `store.ModelCells`: 685 models, 20,906 cells and 86 columns, all matching
  design-schema.md's targets exactly.

## [9.4.0] - 2026-08-28

### Changed

- **Schema migrations v40 and v41 collapse and contract the namespace pivot's
  schema, following merge-table.md's fifteen merge groups.** v40
  (`internal/dbcore/pivotcollapse.go`) widens `cells`' primary key to
  `(column_id, model_name, source)`, so two sources can answer the same
  attribute for the same model at once; collapses 116 columns to 86 by keeping
  each attribute's lowest-position column, moving every other column's cells
  onto it, and dropping the rest; fills `column_sources` for every column the
  collapse leaves untouched (a column with none would admit no writes at all
  once v41's trigger is in place); makes six previously-hidden merge-target
  columns (`price_unit`, `input_modalities`, `output_modalities`,
  `description`, `created_at`, `repository`) visible; moves the 108
  `active_parameters` cells from the `huggingface` source to `hand`, so a
  refresh can never overwrite a reader's own correction; stamps `added`'s
  cells with the `app` source; and sets `resolve='union'` on
  `input_modalities`, `output_modalities` and `collections`. v41
  (`internal/dbcore/pivotcontract.go`) rebuilds `columns` without `namespace`
  and its two old CHECKs, stripping every column's namespace prefix in one
  pass and replacing the CHECKs with `CHECK (instr(column_key,'.') = 0)` and a
  unique index on `label COLLATE NOCASE`; adds the `cells_source_is_declared`
  and `cells_source_never_moves` triggers, so the database itself refuses a
  write whose source is undeclared for its column or that would move an
  already-written cell; adds an `answers` view returning the lowest-ranked
  source's row per column per model; drops `derived_columns` and
  `derived_inputs`, superseded by v40's `column_sources` rows; and renames
  `source_records.namespace` to `source`. `internal/dbcore/db.go`'s floor
  schema no longer creates `idx_columns_namespace`: that statement ran
  unconditionally on every database open (not only a fresh install) and would
  otherwise fail every restart of a database once it reached v41, where
  `columns.namespace` no longer exists. Verified against a copy of the live
  catalog: 20,906 cells and 685 `mine.added` rows both unchanged before and
  after, 116 columns become 86, zero duplicate case-insensitive labels, zero
  dotted column keys, `PRAGMA foreign_key_check` and `integrity_check` both
  clean, both new triggers refuse the write they exist to refuse, and a second
  `OpenDB` against the migrated database is a clean no-op. This is Stage B1 of
  the pivot; `internal/store`'s write paths (`store.WriteCells` and friends)
  are unchanged and still address the pre-pivot shape, which is Stage B2.

## [9.3.0] - 2026-08-28

### Added

- **Schema migration v39 lays the groundwork for the namespace pivot, purely
  additively.** `internal/dbcore/pivotexpand.go` adds a `sources` table (seeded
  with `openrouter`, `huggingface`, `collection`, `hand`, `app`), a
  `column_sources` table, `columns.resolve` (defaulting to `first_stated`), and
  `cells.source` - backfilled from each cell's existing column namespace
  (`mine` to `hand`, `derived` to `rule`, the other three unchanged) and, for a
  computed column that still has its original `derived_inputs` recipe, from
  that recipe's own order. Nothing reads any of this yet: `store`, `catalog`
  and `app` are unchanged, and every column key still carries its namespace
  prefix. `internal/dbcore/migrate.go` also gains the framework a later
  migration needs to rebuild a table under `PRAGMA foreign_keys=OFF` (the
  `disableForeignKeys` migration flag, checked afterward with
  `PRAGMA foreign_key_check` - `PRAGMA integrity_check` does not see a row
  orphaned while the constraint was off) and moves the stored-cell-count guard
  to run around each migration rather than once around the whole chain (the
  `cellsMayFall` flag), so a later migration's deliberate drop cannot mask a
  different one's accident. Verified against a copy of the live catalog: 685
  models and 20,906 cells both unchanged before and after, every cell ends
  with a non-empty source, and `PRAGMA foreign_key_check` returns nothing.

## [9.2.4] - 2026-08-27

### Fixed

- **A closed output pipe during startup could kill the takeover after the previous
  instance was already dead, leaving nothing serving.** `KillPreviousInstance()`
  SIGKILLs any running instance before the new one binds the port, then logs it;
  Go's default action for SIGPIPE is to terminate the process, so a downstream
  reader that closed early - `| head`, `less` quit early, a restarting log
  collector - killed the new process on that very next log write. `Run()`
  (`internal/app/cli.go`) now ignores SIGPIPE, once, before any entry path can
  write to stdout or stderr (`shared.IgnoreBrokenPipeSignal()`,
  `internal/shared/sigpipe_unix.go` / `sigpipe_windows.go`), so a write to a
  closed pipe returns `EPIPE` - which `log.Printf` already discards - instead of
  ending the process. Neither the bare-invocation server start nor the
  unconditional single-instance takeover changed; both are deliberate.

## [9.2.3] - 2026-08-27

### Fixed

- **A throw out of the table redraw could silence the scroll strip for the life of the page.**
  `fillTable()` (`internal/app/static/app.js`) sets `drawingFromFillTable` around its own
  `table.draw(false)` so the `draw.dt` handler skips its geometry read for that draw. The flag is
  now cleared in a `finally`: cleared after the call, a throw out of `draw()` left it set, and the
  handler then skipped the read for every later sort and page-length change.

## [9.2.2] - 2026-08-27

### Fixed

- **A filter keystroke forced the browser to lay out the table twice, and now forces it once.**
  `fillTable()` (`internal/app/static/app.js`) redraws through `table.draw(false)`, which fires
  DataTables' own `draw.dt` handler synchronously, before `fillTable` has run
  `applyColumnVisibility`, `applyViewPaging` and `applyFixedHeader`. That handler used to call
  `updateTopScrollWidth()` immediately, forcing a layout read (`scrollWidth`/`clientWidth`) against
  a DOM `fillTable` was still mid-way through updating; `fillTable` then called it again, correctly,
  once every other layout-affecting call had finished. A new flag, `drawingFromFillTable`, is set
  around `fillTable`'s own `table.draw(false)` so the `draw.dt` handler skips its read for that
  draw only, leaving the handler doing its own read exactly as before for a sort or a page-length
  change driven through DataTables' own UI, which never goes through `fillTable`. The double read is
  proved gone by instrumenting the `scrollWidth` getter and counting the calls. No wall-clock
  saving is demonstrable: an independent measurement of 12 unprofiled trials on each side put the
  fixed code at a mean of 1272ms and the unfixed code at 927ms, with ranges of 398-2110ms and
  275-1453ms, so the trial spread on this shared machine is wider than any difference between the
  two.

## [9.2.1] - 2026-08-27

### Fixed

- **The Columns dialog's answer counts, and the filter panel's "no values" marks, no longer go
  stale within a page life.** Both read `columnHasValues`, filled by `refreshCatalogEmptiness()`
  (`internal/app/static/app.js`), which used to run only at page start, after a rows fetch, or after
  a view switch. Six routes change a cell without a rows fetch behind them and now call it on
  success: the inline cell editor (`writeCell`), the row editor (`#edit-form` submit), hide and
  unhide (`setHidden`), ignore, delete, and re-import. The four routes that already end in
  `reloadRows()` (both add flows, both variation flows) are unchanged, so no route fetches the
  counts twice for one action. The fetch never blocks the write's own status message, and a failed
  fetch leaves the previous counts and marks alone.

## [9.2.0] - 2026-08-26

### Fixed

- **The MoE column now reads what OpenRouter states too, not only your own answer and the
  repository's.** `derived.moe`'s recipe used to read `mine.moe` and `huggingface.moe` only, so a
  model OpenRouter's own listing describes as a mixture of experts (`openrouter.moe`) but no
  repository build confirms showed a blank MoE cell. The recipe now reads, in order, `mine.moe`,
  `huggingface.moe`, `openrouter.moe` - your own answer still wins over both sources, and the
  repository's measurement still stands ahead of the listing's wording. This was held back until a
  count against the maintainer's real catalog on 2026-08-26 showed the OpenRouter substring test
  (`moe` or `mixture` in the description) never misfires there: every one of 36 `mixture`-only
  matches reads "mixture-of-experts" outright, and the two sources agree everywhere they overlap.
  Schema v38 (`internal/dbcore/moereads.go`) repairs the stored recipe on an existing database and
  recomputes the column for every model, but only where the stored recipe is still the untouched
  original `[mine.moe, huggingface.moe]` in that order - any other stored recipe is the reader's own
  and is left alone.

## [9.1.1] - 2026-08-26

### Changed

- **Agent configuration only. No shipped file changed, so this build behaves exactly as 9.1.0.**
  `.claude/tool-gate-approved.jsonc` now records the command patterns the maintainer has approved:
  the six-command live upgrade, and deleting one of his own unsealed `mine.*` columns from his
  running server. `.claude/vibe-hooks.jsonc` and `.sembleignore` are added, the first holding this
  project's hook settings and the second keeping secrets and working folders out of the code
  search index.

## [9.1.0] - 2026-08-24

### Fixed

- **The Type column now reads every source that states a model's type, not only your own answer
  and a collection page's.** `derived.model_type`'s recipe used to read `mine.model_type` and
  `collection.model_type` only, so a model on no OpenRouter collection page - which is most chat
  models - showed a blank Type cell, and a music-generating model always showed as `tts` because
  the `music` answer OpenRouter's own listing implies (`openrouter.model_type`) was never read at
  all. The recipe now reads, in order, `mine.model_type`, `collection.model_type`,
  `openrouter.model_type`, `huggingface.model_type` - your own answer still wins over every source.
  Schema v37 (`internal/dbcore/typereads.go`) repairs the stored recipe on an existing database and
  recomputes the column for every model, but only where the stored recipe is still the untouched
  original: a recipe you changed yourself is left exactly as you left it.

## [9.0.0] - 2026-08-24

### Changed

- **The migration chain has a floor at schema version 33.** Migrations 1 to 33 and the code that
  existed only for them are gone: `internal/dbcore/carrycells.go`, `dropcustomcolumns.go`,
  `dropfactcolumns.go`, `mergerecipes.go`, `columncolors.go` and about 1,700 lines of
  `migrate.go`/`migrate_test.go`. `schemaSQL` is regenerated from a real database at version 33, so
  it stops re-creating four tables a migration used to drop on every start.
- **A database below the floor is refused before it is opened for writing.** `dbcore.OpenDB` reads
  `PRAGMA user_version` through a second, read-only connection first; a below-floor database is
  left byte-for-byte unchanged and the refusal points at the new `MIGRATION.md`.
- **`modelsdb backups` and `modelsdb restore` now run with no database open at all**, so they still
  work on an install a floor refusal has stopped. `RestoreBackup` also refuses a below-floor backup
  file before copying it over the live database.
- **A fresh install no longer creates the "Rating" and "OCR" demo custom columns.** Those were
  created by migrations below the new floor and replayed on every brand-new database; they carried
  no product decision behind them and are gone along with the rest of the below-floor chain.

## [8.12.0] - 2026-08-24

### Added

- **`allowed_origins` in `config.jsonc` names extra addresses this install is legitimately reached at.** Every non-GET request was already refused unless its `Origin` header equaled its own `Host` header, which breaks silently behind an HTTPS reverse proxy: the browser sends `Origin: https://name` while the proxy forwards `Host: name` with no port, so the two stop matching. Listing `"https://name"` under `allowed_origins` gives that address a second, explicit way to pass. Matching is exact on the full lowercased origin (scheme, host, and an explicit default port when the entry omits one) - never a prefix, suffix, or wildcard - so a listed `models.example.com` cannot be used to admit `evil-models.example.com` or a different scheme or port on the same host. The list is read once at startup from `config.jsonc` only: it is never accepted over HTTP, so a caller who can already reach `/api/settings` cannot add their own origin. A malformed entry (no scheme, a path, a query, a fragment, userinfo, or anything unparseable) is dropped rather than failing startup, logged with the reason, and counted in `GET /api/health`'s new `allowed_origins` (the normalized list in effect) and `allowed_origins_dropped` (how many entries were dropped) fields. The research brief's address list and the 403 refusal body both recognize a listed address the same way.

## [8.11.6] - 2026-08-24

### Fixed

- **A filter carried in the URL now applies on the very first paint.** `?variations=yes` and `?hidden=only` set the Hidden and Variations controls but drew every row until the reader touched the control again: the table's first draw ran before the controls were restored from the address bar, and nothing after the restore forced a second one. `restoreFilters()` now ends with a redraw, so any control it sets is applied at once - not only these two, so a future control with the same live-read shape needs no fix of its own. An unknown value for either control (`?variations=xyz`) now falls back to the control's default and warns, instead of leaving the `<select>` with nothing selected.

## [8.11.5] - 2026-08-24

### Fixed

- **Six wrong or incomplete statements in the served API reference.** `derived.model_type` was described as splitting text-to-speech from music by the model's description; that reading is stored in `openrouter.model_type`, which `derived.model_type` never reads. Re-typing one of your own columns was described as something you can do; no route offers it. The add-model ignore refusal matches the OpenRouter identifier and the HuggingFace repository as well as the name. The three measured figures are not columns the app ships, so the first write to one creates it as free text. A save with no cells still stamps the row, and two items naming one model are two separate writes. The duplicate check compares a repository or an identifier however any source stated it.

## [8.11.4] - 2026-08-24

### Changed

- **The maintainer's install is kept current without being asked.** The documented upgrade runs at the end of any piece of work that changed what the app does.

## [8.11.3] - 2026-08-24

### Changed

- **`app.js` has a syntax gate.** No JavaScript linter is installed in this project, so `node --check internal/app/static/app.js` is what runs before a commit. It needs nothing installed and it catches a syntax error before the server serves a page that cannot run.

## [8.11.2] - 2026-08-24

### Added

- **A test over the answer counts.** `GET /api/columns/answers` and the counting behind it had no test naming either. Two now seed a small catalog and pin the arithmetic the Columns dialog and the filter controls print: a count per column, zero for a column nothing has answered, the model count the same reply carries, and the fact that clearing a value takes its column's count down.

## [8.11.1] - 2026-08-24

### Fixed

- **A value typed into a column the table was not showing is now shown.** With **Hide empty** ticked, a column no model has answered is held back. Filling one in from the row editor brought the column back, but its cell still counted as empty: on a wide screen it read the new value in the faint style of an empty cell, and on a narrow screen the card left the value out altogether. The cell of a column that is not drawn is built and kept aside, out of the row, so the row's own marks never reached it. The marks are now written to every cell of the row, drawn or not.

## [8.11.0] - 2026-08-24

### Changed

- **The table builds only the page it shows.** It used to build every row of the catalog as real table markup before drawing any of it: 626 rows of 146 columns is over ninety thousand cells, and the browser laid out every one of them on load and again on every filter change. The table is now handed the rows themselves and builds the cells for the twenty-five it draws, one page at a time. Nothing on screen changes. On a catalog of 626 models the longest single block on load falls from 20,767 ms to 3,484 ms, clearing a search back to 626 rows from 10,440 ms to 2,680 ms, and searching for "gpt" from 2,083 ms to 1,227 ms.

## [8.10.0] - 2026-08-24

### Added

- **`GET /api/columns/answers` reports how many models have answered each column.** It takes no parameters, is counted over the whole catalog with hidden rows and builds included, and gives every column an entry - zero where nothing has answered it. It also reports how many models the catalog holds, so an unfilled catalog and an empty one read differently.

### Changed

- **Changing a filter no longer fetches the whole catalog a second time.** The figures the Columns dialog prints on every row, and the "no values" note on a filter control, are facts about the catalog and not about the page, so the page used to fetch all 674 models again - 1.4 MB - every time a filter narrowed the table. It now asks `GET /api/columns/answers` for the counts alone. On a search for "gpt" the extra traffic falls from 1,429,470 bytes to 4,467 bytes.

## [8.9.0] - 2026-08-24

### Added

- **Several columns can be moved at once in the Columns dialog.** Hold **Ctrl** and press a row's grip to select that column, hold **Shift** and press another to take everything between them, or press **Select** in the bar and then press grips, which is the whole of selecting for a reader with no keyboard. From the keyboard, Ctrl and the space bar select the row you are on and Shift with an arrow reaches further. The bar says how many columns are selected and offers **Clear**, **Move up** and **Move down**, which step the whole block one place. Pressing the grip of a selected row picks up all of them: they land next to each other in the table's own order, however far apart they started, and the whole move is one step for **Undo**. **The modifier is Ctrl on every platform**, including a Mac, and the Command key is not read at all. A selected row is tinted and carries a bar down its leading edge, and its grip says out loud that the column is selected. **A selection never survives closing the dialog**: it is thrown away when the dialog closes, and also by a search, the answers pick, the drawn-only tick, a change of sort, Escape, or a completed move. Selecting works only while the list is in **Table order**, where the list's order is the table's, and every control says so when it is refused.

## [8.8.1] - 2026-08-24

### Fixed

- **The Answers picker no longer stays visible while the catalog is empty.** Its own layout rule outranked the browser's rule for a hidden element, so the control was drawn and still operable on a page with nothing to count.

## [8.8.0] - 2026-08-24

### Added

- **The Columns dialog groups its rows under the five sources.** Yours, Computed, OpenRouter, HuggingFace and Collections, each a collapsible heading carrying how many columns it holds, sticky beneath the Shown bar so the source of the row under the pointer is always readable. The dialog still opens in Table order every time, and the sections are reached from the sort select, whose modes now read Table order (one flat list), Source then table order, Source then heading A to Z, and Source then key A to Z.
- **Every row says how much of the catalog has answered that column.** `536 answers` on the metadata line beside the key, or `Nothing has answered this`, which under Yours adds where to fill it in. The figure counts every model and build in the catalog, hidden ones included, and the count line at the top carries the total. A new Answers select narrows the list to the columns something has answered, or to the columns nothing has.

### Changed

- **The namespace toggle buttons above the list are gone.** The sections and the search do that job, and the tick in the Shown heading now acts on the rows the open sections reach and is refused when they reach none.
- **The dialog's opening paragraph is four sentences.** It explains what a heading and a row cannot say for themselves, and no longer enumerates every control below it.
- **The counting half of the dialog disappears while the catalog is empty.** No row carries a count, the Answers select is not drawn, and the count line points at Update models. While the catalog read has not landed or has failed, the select is refused with the reason and the count line says the answers are not counted.

## [8.7.4] - 2026-08-24

### Changed

- **The variation child rows say why they are built from the shown columns alone.** A child row is put into the page as it is and never enters the table's own list of rows, so nothing maps its cells onto columns: it lines up only because it carries a cell for exactly the columns the heading row still has. That is the opposite of a main row, which is handed to the table over every column, and the comment at the call now says so.

## [8.7.3] - 2026-08-24

### Fixed

- **Saving a colour rule no longer raises a DataTables warning alert.** Every rebuild of the table after the first one built its rows from the shown columns alone, while the table itself still had a column for every entry in the catalog. The table reads a row's cells by position, so the values landed against the wrong columns and the rest had no value at all, which it reported in a native alert naming a row and column number. Rows are now built with a cell for every column and the table takes the hidden ones out of the page itself. The alert also stole the keyboard, so Escape still failed to close the Columns dialog after a colour save; it now closes.

## [8.7.2] - 2026-08-24

### Fixed

- **Escape closes the Columns dialog after a colour rule is saved.** Saving colours hid the colour editor, which took the Save button away from the keyboard and left it on the page behind the dialog. The dialog only hears Escape while the keyboard is inside it, so the key did nothing and the dialog stayed open. The keyboard now goes back to the row whose colours were saved.

## [8.7.1] - 2026-08-23

### Changed

- **Agents exercise frontend work in a browser again.** The testing rule written earlier today banned browser checks outright, which left the drag, keyboard and dialog code with no verification at all, since no Go test reaches a pointer or a keypress. Frontend work is now run against the disposable development server and the agent says what it clicked and what happened.

## [8.7.0] - 2026-08-23

### Added

- **A column moves by dragging its heading in the table.** Press a heading, slide it over another heading and let go: the column takes the place that heading held, and the order is saved like any other move. The heading being carried dims, the heading it will land on is marked with a line down its left edge, and holding the pointer near either edge of the table scrolls it sideways so a column can be taken past the edge of the screen. Escape cancels, and so does letting go anywhere that is not a heading. A plain click still sorts, because a drag only starts after the pointer has moved five pixels, and the click that ends a drag is swallowed so a drop never also sorts. **The table only has a heading row on a screen wider than 1642px; below that the page draws cards, so there is nothing to drag and the Columns dialog is the route.** The dialog's grip, its Move up and Move down buttons and its keyboard all still move a column, so dragging is never the only way to do anything.

### Changed

- **The Var column can be moved.** It was pinned to the right-hand end of the table and no route could shift it. Name is still pinned first and nothing can be put before it; everything else, Var included, goes where you put it.

### Fixed

- **Escape closes the Columns dialog after the list is rebuilt.** Renaming a column, saving its colours or a change that alters which columns are drawn rebuilds the list of rows, which threw away the control the keyboard was on and dropped focus out of the dialog. Escape then did nothing, because the dialog only hears the key while the keyboard is inside it. Focus goes back to the row it was on, or to the search box.

## [8.6.2] - 2026-08-23

### Changed

- **Agents working on this project now test the code they write.** The rule said never to test anything unless the maintainer asked in that request, so every change was delivered unrun. An agent now builds its own change, runs the tests covering it and lints it without asking, reports exactly what it ran and what came back, and still never touches the live install or the real catalog.

## [8.6.1] - 2026-08-23

### Changed

- **Tab leaves the Columns dialog.** The dialog held Tab inside itself and wrapped from the last control back to the first, so the keyboard could not reach the page behind it without closing the dialog first. Tab and Shift+Tab now walk out of it like any other part of the page. Escape still closes it.

## [8.6.0] - 2026-08-23

### Changed

- **A write sent from a web page on another address is refused with 403.** Any other site you have open in the same browser could previously drive every write route of this server, because your browser can reach it and the server asks for no password. Every request that is not a GET or a HEAD is now checked before it reaches a route: when it carries an `Origin` header, that address must be the same address the request was sent to, port included. The app's own page always matches, on `127.0.0.1` and over Tailscale alike, and there is nothing to configure. A caller that is not a browser sends no `Origin` header at all, so `curl`, scripts and assistants are untouched. The refusal is plain text, it never repeats the address it refused, and the request body is never read.
- **The research brief now ignores an address this server never answered on.** `GET /api/models/brief` takes the address the caller reached the server at and writes it into every instruction in the brief, including the `curl -X POST .../api/save` line that sends the findings back. Any address at all was accepted, so a link could put someone else's server in a brief that a reader then pastes into an assistant. Only an address this process really bound is used now, and anything else is dropped without a word. The browser passes its own address, which always qualifies.
- **The served API guide states the 403 and counts the routes right.** It said that a route answers 405 for any method it does not take and nothing else; it now also states when a write is refused with 403 and how to avoid it. It also said there were 29 routes where the server registers 30.

## [8.5.2] - 2026-08-23

### Changed

- **The Columns dialog can now be used from the keyboard, and is announced as a dialog.** It carries a dialog role named by its **Columns** heading, so a screen reader says what has opened instead of reading on into the page. Its close control was a cross no keyboard could reach; it is a real button now, named `Close`, and looks the same as the one on the Add and Edit forms. Opening the dialog puts you in its search box, and closing it puts you back on the button you opened it from. **Escape** closes it, except while a column is picked up, where Escape still puts that column down and leaves the dialog standing. Tab stays inside the dialog while it is open. Nothing the dialog does has changed.

## [8.5.1] - 2026-08-23

### Changed

- **Copying a view now says a copy was made, and what it came from.** Pressing **Duplicate** and finishing with the name box used to be read out as an ordinary move between two views you already had, which never said the copy existed. A screen reader now hears `"Pricing copy" created from "Pricing". It shows 12 columns.`, with the names as they stand: the copy's name is cut to fit the forty characters a view name allows, so it is not one you could have guessed. Choosing a view by pressing its row in the list is unchanged and still reads out as a move.
- **A Hidden or Variations choice that could not be stored says how long it lasts, and stays on screen until you do something else.** The line now reads `That choice was not saved (<reason>), so it stands until you reload.` It used to say the same thing twice and leave out the one fact you cannot see for yourself. It also used to be wiped off the status bar the moment the view you were moving to finished loading, so it flashed past on screen while a screen reader always heard it; it now stands in the bar until your next action paints over it.
- **One press, one line.** A change the server refused to store was announced twice, in two different sets of words, the first cut off mid-sentence by the second. Where the app has its own words for a refusal - the Hidden and Variations dropdowns, a column moved, and a load that cannot store your arrangement - those are now the only words said.

## [8.5.0] - 2026-08-23

### Added

- **Duplicate, in the view panel, between New and Rename.** It makes a copy of the view you are in - its columns and their order, its filters, its sort and its page size - names it `Pricing copy`, then `Pricing copy 2` and so on where that name is taken, and moves you into the copy with its name selected so you can type your own over it. A name too long for the forty characters a view name allows is cut to fit. The copy is dated today, so it arrives at the end of the list, and nothing about the view it was copied from changes. Twenty views is still the most this keeps, and at twenty both New and Duplicate are unavailable.

### Changed

- **A filter set while a view is still loading is held until it has.** The table belongs to the view you are leaving until its rows arrive, while the filter controls and the columns are already the view you are moving to, so a filter run in that moment mixed the two. It now waits for the rows, and what runs when they arrive is your filter, over the view you set it in. The boxes themselves are redrawn on the way in, so it is the values you chose that are kept, not what is left sitting in them. A load that fails drops it, because the controls have gone back to the view you are still in and the line saying what went wrong is the last thing that should be painted over.
- **Hidden and Variations say when your choice was not stored.** Those two dropdowns rearrange the rows already on screen instead of reading the server, so they answer while a view is loading, and nothing can be stored in that moment. The choice stands on screen and the status bar says it was not stored. A screen reader is told either way.
- **A failed load and a refused undo say more.** A view that could not be read names the view you are still in, or, where you had just deleted one, says that the rows on screen are the deleted view's. An undo refused because twenty views are already there, or because the name has been taken since the delete, now names the view that cannot come back and the one thing you can do about it, instead of repeating the line already standing in the panel.

## [8.4.0] - 2026-08-23

### Added

- **Views: as many arrangements of the table as you want, and a button in the top bar to move between them.** The button says which view you are in; pressing it opens the list, and each view's row says what it holds - `12 columns, 3 filters, sorted by Price` - so you can tell two apart weeks later. Choosing one applies its columns and their order, its filters, its sort and its page size, and rewrites the address to match, without reloading the page. The change happens only once the models have been read: if that read fails, the table, the address and the button all stay on the view you were in, the one-step undo of the column order is still there, and the status bar says so whatever else has happened on the page in the meantime. The exception is the view you have just deleted, which is not gone back to: the button and the panel stay on the view the delete moved you to, the table keeps the rows it was showing, and the status bar says that too.
- **A view holds the filters, the sort and the page size as well as the columns.** Every change to any of them is saved into the view you are in the moment you make it: there is no Save button and nothing to remember. Which view a device is on is that device's own, and each tab may differ, so two tabs can sit on two arrangements at once. A deleted view can be undone until the page is reloaded, after which only a whole-database backup brings it back. A view deleted in one tab while a second tab is still in it is written back whole by that tab's next change, so nobody loses an arrangement to the other's delete. A change made while a view is still loading is not stored, and a column moved in that moment says so and goes back where it was rather than looking saved.
- **New, Rename and Delete, in the panel.** A new view starts with only the model name shown, no filters, and the columns in the order the app ships them; a line above the table offers **Choose columns** for as long as a view shows that one column. Columns left out because nothing on show has a value are not part of that count, so the line and the view's own row in the panel never say two different things. Renaming happens in the list itself, and a name another view already has, ignoring capitals, is refused with a line saying which one. Delete asks first, moves you to a neighbouring view, and leaves an **Undo** in the panel for the rest of the page's life. The last view cannot be deleted, and twenty views is the most this keeps - and **Undo** keeps to both of those rules, so it refuses when twenty views are already there, or when the name has been taken since the delete.
- **The Columns dialog names the view it is arranging**, because which columns are shown and their left-to-right order belong to that view. A column's name, its colours, and a column of your own are the same in every view.

### Changed

- **On a narrow screen the top bar drops the funnel and hides the ModelsDB title.** Both openings of the filter panel were drawn at once - the funnel in the bar and the **Filters** button in the sort bar below it - so the funnel goes at that width and the Filters button takes over the badge counting the filters you have set. The title is still there for a screen reader; it gives its width to the view name instead, so the view you are in is readable on a 390px phone.
- **The view panel is a bottom sheet on a narrow screen**, with a close button under the thumb, and a dropdown under the button on a wide one.

## [8.3.0] - 2026-08-23

### Changed

- **Your arrangement of the table is now a saved view, stored under its own keys.** The filters, the column order, which columns are shown, the page size, the hide-empty box and the sort no longer sit at the top level of the settings: they are stored as `view.<id>.<key>` rows, alongside the view's `name` and the date it was made. Nothing about this is on screen yet - the page looks and behaves exactly as it did - but the arrangement you already had is adopted, once, as a first view named **Main**, in a single write that removes the keys it came from in the same transaction. There is no half-moved state: either the view is there and the old keys are gone, or nothing changed at all.
- **An install rolled back to 8.1.x draws the shipped columns, and loses nothing.** That version does not read the new keys, so it sees no stored arrangement and starts from the defaults. Your arrangement is not deleted - it is still in the database under the view keys - and that version's flat saves are refused by the settings route rather than overwriting anything, so coming forward to 8.3.0 again restores it exactly as it was.
- **An install rolled back to 8.2.0 draws the shipped columns too, and anything you rearrange while you are there is discarded when you come forward.** Unlike 8.1.x, that version's saves are accepted rather than refused: it writes the table's settings back at the top level, where 8.3.0 no longer looks for them. Your stored view is untouched by any of it and comes back exactly as it was, but a day spent rearranging the table on 8.2.0 is swept away, unread, on the first load of 8.3.0. It is the version your upgrade keeps beside the new one, so it is the one a rollback lands on.
- **A link with a query string still overrides the filters on screen, and now enters the view only when you change a filter.** Arriving on somebody's link shows you their rows, as before; what is different is that hiding a column, changing the page size or clicking a header no longer writes that link's query into your saved arrangement. Only a filter act does.
- **A filter on a column you then hide now stays in your arrangement.** Until now, hiding a column quietly wrote that column's filter out of your saved filters on the next save, because the filters were rebuilt from the controls you could still see. The view keeps the filters as they stood when you last changed one, so the filter is back in its box when you show the column again, and narrows the rows the next time you change a filter. Clearing it is a filter act like any other.
- **Which view is on screen is remembered by the browser, not by the database**, per tab and per device, the way the theme already is. One device can never move another, and two tabs can sit on two arrangements at once.
- **A save that fails says so in the status bar**, naming the reason, rather than failing silently. If it is the one write the page makes as it loads that fails - the write that turns your existing arrangement into the first view - a bar across the top of the page says so and stays there, and the page stops saving until you reload, rather than filling the database with rows that have no view to belong to.
- **The served API guide describes the view keys**, and which settings stay global: the docked filter panel, the automatic-update preference, and the daily backup schedule.

## [8.2.0] - 2026-08-23

### Changed

- **`POST /api/settings` names what it stores and what it removes, and leaves everything else alone.** The body is now `{"set": {...}, "delete": [...]}`: everything in `set` is stored, everything in `delete` is removed, and any other stored key is untouched. Both members are optional. Until now the route replaced the whole set, so a key the body left out was deleted - which meant a page that had been open since before a preference existed could destroy that preference on its next save, without saying anything. Deleting is now the only way to remove a key: a `set` value of `null` stores `null`.
- **A caller posting the old flat object gets a 400 and writes nothing.** There is no fallback: a flat body accepted quietly would store nothing and look as though it had worked. The browser app ships inside the binary, so it always posts the new shape.
- **A malformed `backup_enabled`, `backup_dir` or `backup_overwrite` answers 400 rather than 500.** A value of the wrong type is the caller's mistake, and a 500 sent the reader looking in the logs for a server fault that was not there.
- **The settings body is capped at 4 MiB, and a larger one answers 400.** This route writes to disk, it takes no credentials, and the server answers on a LAN or Tailscale address when it is configured with one. A whole arrangement is a few kilobytes.
- **The served API guide describes the new shape**, including every reason the route answers 400.

## [8.1.2] - 2026-08-23

### Fixed

- **Putting a column down puts the focus back, but only when your own press put it down.** A press on the held grip, **Cancel**, `Escape`, the sort box, **Undo**, **Reset order** or a placement that cannot be made now hands focus to the grip of the column that was put down, or to the search box when the list is not showing that column. Focus is only moved when it was actually lost, so a control that survived the rebuild keeps it. A put-down nobody asked for - a model form opening, or the dialog closing - never moves focus.
- **Closing the Columns dialog puts the focus back on the Columns button.** It used to be left on a control inside the hidden dialog, which starts the next Tab at the top of the page.

### Changed

- **A model form opening with a column held says what happened**, rather than saying the move was cancelled: *"A model form opened, so <column> was put down. Nothing moved."*
- **A put-down of a column the list is not showing says so**, in the same announcement: *"... <column> is not in the list you are looking at now."*

## [8.1.1] - 2026-08-23

### Changed

- **The bar is the only voice when a column is picked up.** The second, hidden announcement that repeated it in different words is gone, and with it the guess at whether the press came from a keyboard. The bar also has a second line for when a search, a namespace button or **Only the columns the table shows** has left the held column out of the list: *"<column> picked up - not shown in this list, press a grip to place it"*. The place targets are unaffected, because a placement is worked out from the table's full order.
- **How to stop a move rides on the control that stops it.** **Cancel** is announced as *"Cancel the move (Escape)"*; its visible text is still **Cancel**.
- **Changing the sort with a column held says what happened**, rather than saying the move was cancelled: *"The sort changed, so <column> was put down. Nothing moved."*
- **A grip that is not holding anything no longer reports itself as an unpressed toggle.** While a column is held, every other grip is a control that commits a whole move, so `aria-pressed` is taken off it rather than set to false.

### Fixed

- **A placement no longer leaves the focus nowhere.** When the moved column has no row in the list the reader is looking at, focus goes to the grip of the row they pressed, then to the last row's grip, then to the search box. The announcement gains one clause in that case: *"<column> is not in the list you are looking at now"*, and focus moves before the announcement is made.
- **Pressing `All` in the Columns dialog's namespace row no longer throws the focus to the top of the page.** The button is rebuilt by its own press, and the focus is put back on it.

## [8.1.0] - 2026-08-23

### Added

- **A column can be picked up, the list scrolled, and placed with a second press, and that is the way to move one a long way.** Every row of the Columns dialog gains a grip at its start. Press it and the column is picked up; the sticky bar names what is held and offers **Cancel**; every other grip becomes *Place <column> before this one*, and a target under the last row places it at the end. Scrolling, ticking a column and searching all go on working while a column is held. Pressing the same grip again, **Cancel** or `Escape` puts it down and writes nothing. The whole route works from the keyboard: `Space` or `Enter` picks up and places, `Escape` puts down, and the up and down arrows walk the grips.
- **`Only the columns the table shows`**, a tick beside the search box that keeps only the columns the table is drawing right now. It composes with the search and the namespace buttons and is off every time the dialog opens. **A placement made in a narrowed list can put columns you cannot see between the two rows you can**, because a move is made in the table's full order; the line above the rows says so, and the announcement names the full-order neighbour rather than the one on screen.
- **Undo**, in the sticky bar beside **Reset order**. It puts back the order as it stood before the last change of this page's life, whatever made it - a placement, a move button or **Reset order** itself. One step, held in memory only, gone when the page is reloaded. It is not `Ctrl+Z`, which stays the cell-edit undo.

### Changed

- **Reset order** now says that Undo puts the arrangement back until you leave the page, rather than that it cannot be undone.

### Fixed

- **A move no longer throws the list back to the top.** The Columns dialog's own scroll position is captured with the rest of the table state and put back after the rebuild, so a move made far down a list of 110 columns leaves the list, and the focus, where they were.
- **Two quick moves can no longer put each other back.** Only the newest write of the column order may restore the previous one when it fails; an older write failing late is ignored.

## [8.0.1] - 2026-08-23

### Fixed

- **A restarted install and a fresh install now end at the same schema.** The three tables a personal column used to live in - `custom_columns`, `custom_column_options` and `custom_values` - were declared in the block that is applied on EVERY open, so schema version 31 dropped them once and the next start put them straight back, empty. They are declared for the migrations that read them now, and only for a database that has not reached version 31 yet.

### Added

- Schema version 36 removes those three tables from a database that still carries them. It refuses the migration, rather than dropping anything, if any of them is found holding a row.

## [8.0.0] - 2026-08-22

### Changed

- **The reader's view and the app's preferences now live in the database.** Every value the page saves - filters, column layout, page size, the *Automatically download updates* tick - is a row of the `app_settings` table instead of a `settings.json` file, so one file holds everything an install remembers and a backup carries it.
- **The automatic backup schedule now lives in `config.jsonc`**, beside `data_dir`, as `backup_enabled`, `backup_dir` and `backup_overwrite`. They can be set by hand; saving them in the Info dialog rewrites the file, which drops comments written into it.
- The scheduled daily backup now decides whether the database changed by digesting its contents, so the preference writes that now land in the database do not make an idle install produce a backup every day.
- The page saves nothing until it has read its settings back successfully, and says so with a bar offering **Reload** when that read fails, so a failed read can no longer overwrite a good stored view with an empty one.

### Added

- Schema version 35 recreates the `app_settings` table and, on a database that already held models, records a one-time notice.
- A dismissable bar telling an upgrading reader that their view preferences and backup schedule were reset. `POST /api/settings/ack-notice` clears it for good.
- Keys beginning `server.` are the server's own: `GET /api/settings` returns them and `POST /api/settings` refuses a blob carrying one with 400.

### Removed

- **`settings.json` is gone.** Nothing reads it, nothing converts it, and there is no fallback: upgrading resets the view preferences and the backup schedule, which are set again in the app.

## [7.16.4] - 2026-08-22

### Changed

- Agent rules: finished work is committed immediately, without asking.

## [7.16.3] - 2026-08-22

### Changed

- Agent rules: the procedure for updating the maintainer's live install is now
  six commands in `CLAUDE.md` instead of a completed task record.

## [7.16.2] - 2026-08-22

### Changed

- Agent rules: no test, lint, build or browser check runs unless the maintainer
  asks for it in that request, and no agent may claim anything works while that
  is so.

## [7.16.1] - 2026-08-22

### Fixed
- **A move with nothing left to cross is refused instead of jumping the column
  a hundred places.** When the column being moved is drawn but has no other
  drawn column left to cross in that direction, **Move up** and **Move down**
  now say so in their own name - *Move ZDR down (already last)* - and a press
  does nothing. Before, the press sent the column to the far end of the full
  order of a hundred and ten while the table did not change at all, so the
  reader was given a control that appeared to do nothing and an arrangement he
  had not asked for. The refused button is still there and still reachable by
  keyboard, as every refused move control in that dialog is. Showing or hiding
  a column now updates those buttons at once, because which columns are drawn
  is what a move steps over.

## [7.16.0] - 2026-08-22

### Added
- **The reader sets the left-to-right order of the columns.** Every row of the
  **Columns** dialog carries a **Move up** and a **Move down** button, and a
  move steps over the next column the screen is actually drawing rather than
  one place in a list of a hundred and ten, so a press always moves something
  the reader can see. It is ONE order, shared by the table and the cards: the
  cards are the same table under different CSS, so a column moved on a laptop
  is in the same place on a phone. The filter panel and the mobile sort list
  follow it too. The **Add model** and **Edit model** forms do not: arranging a
  table is a judgement about which columns to read side by side, and a form is
  a sequence to type through, so those keep the order the app ships. The order
  is saved with the rest of the layout and survives a reload and a restart, and
  **Reset order** puts it back the way the app ships after asking first.
- The move buttons are never taken away. One that cannot act says why in its
  own name, so a reader hearing it read out is told the reason rather than
  finding nothing there: a column already at the end, a list sorted A to Z
  instead of in table order, or a model form open over the top.

### Changed
- **Every row of the Columns dialog is one block, at every screen width.** The
  four-column table it replaces needed 866 pixels of width inside a box that
  can never exceed 720, so there was no screen it fitted on once the move
  buttons arrived. A row is now a label line carrying the tick, the move
  buttons and a **More** disclosure, a line naming the column's key and what it
  holds, and the **Rename**, **Colours** and **Delete** buttons behind that
  disclosure. The label wraps instead of truncating and the key is never cut,
  because rows nobody can tell apart was the complaint the dialog answers. On a
  screen 1642 pixels wide or narrower the dialog fills it as a sheet and every
  control is finger-sized.

### Fixed
- **A hand-edited layout file can no longer invent a column.** The saved order
  was looked up in a plain object, so an entry named `__proto__` matched
  something JavaScript puts on every object and pushed it into the table as a
  column. Only names the catalog actually holds are accepted now, each one
  once, and anything the file leaves out lands back beside the columns it ships
  next to.
- **A layout that fails to save no longer leaves the screen disagreeing with
  the file.** Saving checked that the request went out but never that the
  server accepted it. A refused reorder now puts the columns back where they
  were and says so.

## [7.15.7] - 2026-08-22

### Fixed
- **The Columns dialog no longer opens underneath the filters drawer.** On a
  screen 1642px wide or narrower the filter panel is an overlay drawer, and the
  **Columns** button lives inside it, so pressing the button opened the dialog
  behind the very panel it was pressed in: all the reader could see of it was
  its close cross. Pressing **Columns** now closes the drawer, and re-reads the
  top bar's funnel so its `aria-expanded` stops announcing a panel that is shut.
  On a wide screen the docked panel is untouched and stays remembered across a
  reload.

## [7.15.6] - 2026-08-22

### Fixed
- **The database-schema document says where its own command must be run from.**
  The paragraph `internal/store/archdocs_gen_test.go` prints around the `bash`
  block warned only that `MODELSDB_ARCHDOCS_DIR` must be absolute. The other
  half of the command, `go test ./internal/store`, is a relative package path,
  so a reader who followed the document's advice to work in a scratch folder
  outside the repository got `go: go.mod file not found` and no document. The
  text now pairs the two: run the command from the root of the repository, with
  the output going to an absolute path outside it.

## [7.15.5] - 2026-08-22

### Changed
- **The database-schema document is produced on demand and stored nowhere.**
  `internal/store/archdocs_gen_test.go` no longer tells its reader to write the
  document into a folder of the repository: the file comment, the doc comment on
  `TestGenerateArchitectureDocs`, and the `bash` block the generated document
  itself carries now name a scratch folder outside the repository
  (`/tmp/archdocs`), to be read and thrown away. The stored copy had drifted,
  saying schema version 33 while the schema was at 34, which is what a copy
  nobody regenerates does. The generator, and everything it reads and prints
  about the schema, is unchanged.

## [7.15.4] - 2026-08-22

### Fixed
- **`ARCHITECTURE.md` says where the generated database reference really comes
  from.** It named a `04_database.md` inside a maintainer task folder "in this
  repository", which was wrong twice over: that folder moved, and the task
  folders are not part of the repository, so no such file ships. It now names the generator that produces the reference on
  demand.

## [7.15.3] - 2026-08-22

### Fixed
- **The regeneration command in `internal/store/archdocs_gen_test.go` names the
  folder the architecture documents are actually in**, a completed task's
  `architecture` folder, in the file comment and in the instructions the generated `04_database.md` carries. Copying
  either one now writes the document where it lives.

## [7.15.2] - 2026-08-20

### Changed
- **The DataTable's `columnDefs` are worked out only where they are read**, in
  the call that creates the table on first load. `buildDtCols(entries)` now
  stands in that one argument rather than in a variable above the branch, so a
  filter rebuild - which keeps the existing table and never looks at the
  definitions - no longer maps over every drawn column to produce a list it
  throws away. Nothing a reader can see is different.

## [7.15.1] - 2026-08-20

### Changed
- **The DataTable's `columnDefs` are built by a function, `buildDtCols(entries)`,
  called from `fillTable`.** The definitions were worked out once when the script
  ran, from the column list as it then stood; they are now worked out from the
  entry list `fillTable` already has in hand. Nothing a reader can see is
  different: the definitions are only ever read when the table is created, and at
  that moment the entry list is the full `colConfig`, which is exactly what the
  old list was built from.

## [7.15.0] - 2026-08-20

### Changed
- **The DataTable no longer rebuilds on every filter change.** The instance is kept across filter updates, rows are cleared and refilled, and only the drawn columns are rebuilt on subsequent updates. The initial load still uses the full `colConfig` so DataTables gets a stable column set. `updateTopScrollWidth()` is now coalesced to run once per rebuild at the end of `fillTable` instead of three or four times, removing the repeated forced layouts. Together these cut the typing block from ~2.7 s to under a second.

## [7.14.2] - 2026-08-17

### Fixed
- **A build's format is read off the files the repository holds, and no longer
  defaults to `safetensors`.** When a repository's tags named no format, the
  importer wrote `safetensors` - a guess, and on some repositories a false one.
  `coqui/XTTS-v2` publishes four `.pth` files and not one safetensors, and
  `IndexTeam/IndexTTS-2` publishes `.pth` and `.pt` files with a single nested
  safetensors, yet both were recorded as safetensors builds while the Build
  files column beside them listed the real names. One table now pairs each
  weight extension with the format that extension is, and it answers both
  questions asked of a filename - is this a weight file, and what format is it -
  so a file that counts towards a build always names that build's format. A
  repository of `.pth` files is a **PyTorch** build; one holding more than one
  kind names all of them (`safetensors + PyTorch`), because the build is every
  one of those files. The tags are still read first, because they name a
  quantization scheme - AWQ, GPTQ, FP8 - which is stored inside safetensors
  files and which no filename reveals.

  Rows already recorded under the old default are NOT rewritten: the wrong word
  is in the row's name, which is the primary key other rows point at, as well as
  in its cells. Delete such a variation and record it again to get the corrected
  label.

## [7.14.1] - 2026-08-16

### Changed
- **Refiltering the table is up to sixteen times faster, and looks exactly the
  same.** Changing a filter rebuilds the table, and the rebuild ended by setting
  the visibility of every column one at a time. Each of those fires DataTables'
  `column-visibility` event, which the pinned-heading extension answers by
  measuring the table - and measuring it forces the browser to lay out every row
  and every column first. On a catalog of 652 models and 146 columns, one typed
  character cost 159 full-table layouts and blocked the page for 17.6 seconds.
  The columns that already are as they want to be are now left alone, the rest
  are set in one call per answer rather than one per column, and the extension is
  stood down for the batch and put back once at the end. The top bar's height,
  which the pinned heading is offset by, was a second forced layout of the whole
  table on every rebuild; it is measured once and read again only when the window
  changes, which is the only thing that can move it.

  Typing a character in a filter box now blocks for 1.8 seconds instead of 17.6,
  unticking Hide empty for 0.7 instead of 11.6, ticking a value in a value-list
  filter for 1.3 instead of 7.4, and Clear for 6.7 instead of 26.4. Sorting and
  paging never went through this path and are unchanged. Nothing a reader can see
  is different: the same columns are drawn, the same rows, the same cards, and
  screenshots before and after are identical to the byte.

## [7.14.0] - 2026-08-16

### Added
- **An Added column, holding the date a model first entered your catalog.** It
  is a sealed column of your own, `mine.added`, hidden on a fresh install, and
  it answers a question no source can: OpenRouter's "Listed on" is when the
  listing was published and a repository's is when the repository was made,
  while this one is about YOUR table. Every path that creates a row stamps it
  once - the OpenRouter refresh, a HuggingFace import, a model added by hand and
  a recorded build - and nothing ever rewrites it, so a refresh cannot quietly
  turn it into "the last time anything touched this row". It sorts and filters
  like any other column. It is sealed because a Delete click would permanently
  destroy dates that no source, refresh or import can ever state again.
- **A tick in the Columns dialog's Shown heading changes every column the list
  is showing.** It acts on the rows the search, the namespace buttons and the
  sort have left on screen and never on the whole catalog, shows a part-way
  state when only some of them are shown, carries an accessible label of its
  own, and saves once rather than once per column.

### Fixed
- **A ticked column the table was not drawing now says why, on its own row.**
  Hiding what is empty stays absolute - a tick does not overrule it - so ticking
  a column that no model on screen has a value in drew nothing and looked like a
  broken control. The dialog now writes one line on that row the moment the tick
  is made, announces it, and totals the held-back columns on the count line
  above the list. The line is written in place, so the keyboard keeps its
  position. It can only arise on the wide layout, where hide-empty takes out
  columns; a card hides its own empty cells instead.
- **A column added after you arranged the table arrives as the app ships it.** A
  saved layout now records the columns it knew about, so a column that did not
  exist when it was saved is decided by the server's own visible flag rather
  than defaulting to shown. Before this, every new column was drawn in every
  arranged table whatever the app shipped it as - the opposite of what the code
  claimed. A layout saved before this change keeps its own meaning, so nothing
  somebody arranged is rearranged by the upgrade; the first save afterwards
  records the column list, and the fix applies from then on.

### Removed
- **The Columns dialog's Computed dropdown is gone.** THIS LOSES A ONE-CLICK
  SHORTCUT and is not pure cleanup: "Computed only" was one press, and the
  namespace buttons reach the same list only by turning the other four off. It
  goes because two controls were cutting one list and had to be kept from
  contradicting each other - a reconciliation that existed solely to paper over
  having both - and the namespace buttons already say everything it said. The
  extra clicks are the accepted price.

### Database
- **Schema v34**: the Added column, and one date on every model already in the
  catalog. The true arrival history was never recorded anywhere, so every
  existing row is recorded as having arrived on 2026-08-16, the day the column
  shipped, rather than a plausible date invented per row. Purely additive: it
  creates one column and writes one value per model that has none, reads no
  other column, rewrites nothing, and a second pass writes nothing at all.

## [7.13.1] - 2026-08-16

### Changed
- **`README.md` describes the page as it is now.** It still told the reader to
  press buttons in the header that live in the top-right menu, so it now
  describes the bar across the top of the page (funnel and set-filter badge at
  the left, the title in the middle, connection dot, light/dark switch and menu
  at the right) and points at the menu for Add model, Update models, Info and
  GitHub. The filter section gains the value lists with their three states, the
  "no values" marking, the Hide empty checkbox, the add button on an empty cell,
  and the pager above the table as well as below it.

## [7.13.0] - 2026-08-16

### Added
- **The pager and the per-page menu now sit above the table as well as below
  it.** With 25 rows on screen the foot of the table is a screen away from its
  head. Both copies stay in sync in both directions, and "N of M" stays at the
  foot alone. In card view the two rows are told apart by the table's own
  `.dt-layout-table` row, so the copies land in their own grid areas instead of
  stacking on each other.
- **Hide empty is back**, one checkbox in the filter panel, on by default. On a
  wide screen it takes out the columns no row passing the other filters has a
  value in, judged over every filtered row and the variations under them and
  worked out again on each draw; on a card it takes out that card's own empty
  cells and keeps the footnote counting them. It adds to hand-hiding rather
  than overriding it, so the Columns manager still lists such a column as
  shown and unticking the box restores exactly the hand-made state. Kept in
  `settings.json` as `hide_empty`, left out of the address bar, untouched by
  Clear.
- **A filter whose column holds no value anywhere in the catalog is dimmed and
  says "no values"**, so a control that can only answer "no models" says so
  before it is used. It still works, and the answer is worked out in the
  browser over the whole catalog.

### Changed
- **Every namespace mark now carries its namespace's own colour**, through one
  per-theme custom-property pair each, wherever the mark appears: table
  headings, filter section headers, the column manager, form legends and the
  catalog stats table. OpenRouter and HuggingFace take their brand colours
  (HuggingFace darkened on light, where its gold measures 1.27:1); mine,
  derived and collection take green, indigo and rose. Every colour clears
  WCAG 1.4.11's 3:1 against the default heading background - measured at 5.51,
  4.71, 4.38, 6.91 and 5.28 on light, and 12.37, 10.09, 9.51, 7.34 and 8.07 on
  dark. Colour is never the only signal: the glyph shapes stay distinct and the
  namespace is still named in words.
- **An empty cell no longer carries placeholder text.** "Click to add..." and
  the bare "-" are gone everywhere, including the wide table: a placeholder is
  text, and text is what stops a cell being empty. A displayed empty editable
  cell carries a small add button instead, naming its column and model, opening
  the very editor a filled cell opens, and reachable and operable from the
  keyboard because it is a real button.

### Removed
- **The duplicate-heading namespace badge.** Every heading already leads with
  its namespace mark, so the badge printed the same glyph twice in one cell.
  Two visible columns sharing a heading are now told apart by the coloured
  leading marks, the tooltip and the visually hidden namespace name.

### Fixed
- **The top bar's status dot no longer prints over the title on a narrow
  screen.** The right-hand zone was given an equal share of the bar's leftover
  width, which is less than its three finger-sized controls measure; packed to
  its end, the shortfall pushed the first control out of the zone's start edge
  and across the title.

## [7.12.0] - 2026-08-15

### Changed
- **The top bar was rearranged.** It now holds exactly five things: a funnel
  button at the far left that toggles the filter panel at every width and
  carries the set-filters count badge; the ModelsDB title, visually centered;
  and at the far right the database dot, the theme toggle and a hamburger
  menu. The menu lists Add model, Update models, Info and GitHub - the first
  three trigger exactly the handlers their buttons always had, Update models
  is disabled and says so while a refresh runs, and GitHub opens the repo in
  a new tab. The menu follows the ARIA menu pattern: Enter/Space open it onto
  the first entry, the arrow keys move and wrap, Home/End jump to the edges,
  Escape closes and returns focus to the button, and a click outside or Tab
  closes it.
- **The 44px closed-filters strip is gone.** Its job and its badge moved onto
  the top bar's funnel, so a closed panel gives the table the whole width.
  The docked/overlay behaviour of the panel, the `filters_open` memory and
  the sort bar's `Filters` button on a narrow screen are all unchanged.
- **The version and the GitHub link left the top bar.** Both now live on one
  line at the top of the Info dialog's "Files & updates" tab (GitHub is also
  a menu entry). On a 390px screen the bar keeps one line: the title
  truncates and the three right-hand controls stay.

## [7.11.0] - 2026-08-15

### Added
- **The Info dialog carries the catalog's numbers**, on a third tab, "This
  catalog": how many models (and of them variations, hidden, and shown by the
  filters right now), how many rows each source created, what each namespace
  knows now (columns, models holding at least one value, values held), and
  the columns per namespace with how many are shown. Counted afresh from the
  server each time the tab is selected, so the totals hold whatever the table
  is filtered to; no new route was added. The tables scroll inside themselves
  on a narrow screen.
- **The Info dialog's tabs answer the arrow keys**: Left/Right move between
  the tabs and select as they go, wrapping at the ends, with Home and End for
  the edges - which the markup had promised and the page had never wired.

## [7.10.0] - 2026-08-15

### Added
- **Every filter on a choice, list or short text column is a value list with
  three states.** A button naming what is chosen opens a panel listing the
  values the column really holds (from `GET /api/columns/values`, over the
  whole catalog), each with its model count and a `(no value)` row first.
  Clicking, Space or Enter cycles a value off, include (+), exclude (-);
  the states read in grey-scale (box glyph, sign, strikethrough), each row's
  accessible name states its state, and every change is announced. A search
  box inside the panel narrows the list in hand, or asks the server with
  `contains=` on a column past 200 distinct values - those keep their letters
  box, with a "Show the N values anyway" link. What is chosen rides in the
  address as `any:`/`not:` parameters, a reload restores it, and an old
  link's bare exact value on a choice or list column restores as an include.
  Numbers keep their figure box and flags keep All/Yes/No.

### Fixed
- **Two quick filter changes can no longer draw the older answer.** Cycling a
  value through include to exclude puts two requests in flight, and the first
  could return last and overwrite the table with a filter nobody had set;
  only the newest query may draw now.
- **"Show the N values anyway" no longer closes the panel it opens.** The
  click removed its own button, so the document's outside-click handler
  judged the click outside the control and shut the panel on the spot.

## [7.9.0] - 2026-08-15

### Added
- **`GET /api/columns/values`**: the distinct values one column really holds,
  with how many models hold each, computed over the whole catalog. A list
  column is split into its members; a choice column answers its declared
  options in order, zero counts included; everything else comes most-held
  first. `contains=` narrows, `limit=` bounds (default cap 200 with
  `truncated: true`), and `distinct_total` always tells the whole count.
- **Two query operators, `any:` and `not:`**, on every kind of column but a
  number: a comma-separated value list to match (union) or to exclude
  (complement), with `\,` for a literal comma and an empty member meaning
  "no value at all".

### Fixed
- **A repeated query parameter is no longer silently dropped.** Naming the
  same column twice now adds a second condition ANDed with the first - it
  used to filter on the first value only and answer a wider set than it
  appeared to.

## [7.8.0] - 2026-08-15

### Added
- **The Columns dialog can be searched, filtered and sorted.** A control row
  above the list holds: a search box over heading or key; five namespace
  toggle buttons with their icons (plus All), each announcing its pressed
  state; a three-way Computed cut that agrees with the namespace toggles
  ("Computed only" turns the computed namespace on alone, and pressing any
  namespace button resets it to All); a sort dropdown (table order, heading,
  key, namespace then heading); and a "N of M columns" count. Sorting the
  list never reorders the table - `col_order` in `settings.json` is untouched,
  and the dialog says so beside the sort control. The state is not saved: it
  resets on every open.

## [7.7.0] - 2026-08-15

### Added
- **The filters live in a docked panel on the left, at every width.** On a
  wide screen the panel docks at 320px and the table shrinks beside it; closed,
  it leaves a 44px strip with the funnel icon and a badge counting the filters
  currently set. Open or closed is remembered per computer (`filters_open` in
  `settings.json`). On a narrow screen the overlay drawer is unchanged and
  still always starts closed.
- **Five collapsible namespace sections inside the panel** (`mine`, `derived`,
  `openrouter`, `huggingface`, `collection`), each header carrying the
  namespace icon, its name, a count of set filters and a disclosure arrow
  (a real button with `aria-expanded`, not a `<details>`). A control whose
  filter is SET stays visible while its section is collapsed, and clearing it
  hides it on the spot; the controls that name no column sit above the
  sections.

## [7.6.0] - 2026-08-15

### Added
- **A loading skeleton instead of a blank first paint.** The page starts with
  a heading row and eight grey shimmer rows written into `index.html` itself,
  under a line of words that moves as the load moves ("Reading the column
  list...", "Reading your view...", "Reading the models..."), each line
  mirrored to the polite screen-reader region. The skeleton is removed the
  moment the real rows are drawn; a failed first request replaces it with
  the failure text, and `prefers-reduced-motion` turns the shimmer off while
  keeping the grey rows and the words.

## [7.5.0] - 2026-08-15

### Added
- **A mark per namespace on every column heading.** Five inline SVG icons, one
  per namespace (the OpenRouter glyph, an HF monogram, a stack of pages for a
  collection, a person for your own columns, converging arrows for a computed
  one), drawn in `currentColor` so they take the theme. They appear before
  every table heading, on every row of the column manager, on each namespace
  legend in the add and edit forms, and in the duplicate-heading badge, which
  now shows the mark plus the word. Each icon is `aria-hidden`; a screen
  reader hears the namespace named in words after the heading text instead.
- **A computed column's heading is tinted** (`--heading-derived`: `#4338ca`
  light, `#a5b4fc` dark), so a value worked out from the others never reads
  as a stored fact. Measured contrast on the heading background: 6.91:1
  light and 7.34:1 dark, against a 4.5:1 floor; the icons ride the heading
  text colour at 12.78:1 light and 11.87:1 dark, against a 3:1 floor.

## [7.4.0] - 2026-08-15

### Added
- **The heading row stays on screen while you scroll.** The DataTables
  FixedHeader extension (4.0.1, matching the vendored DataTables 2.0.8) is
  vendored into the binary and pins the heading just under the top bar, with
  the page's own glue keeping the pinned copy aligned with the table's
  sideways scroll. Sorting by clicking the pinned heading works, and a focused
  row control is scrolled clear of it.
- **The Name column is frozen on a wide screen.** Scrolling sideways slides
  every other column under it, over the row's own opaque background in both
  themes, with a shadow on its edge while scrolled. Card view is untouched.
- **A second horizontal scrollbar above the table**, moving with the one at
  its foot, shown only while the table overflows and never entering the tab
  order.

### Fixed
- **A variation row now follows column show/hide.** Showing or hiding a
  column rebuilds the variation child rows, which previously kept their old
  cell set and drew every cell under the wrong heading until the next redraw.

## [7.3.0] - 2026-08-15

### Changed
- **The table draws 25 rows at a time by default, and remembers the size you
  pick.** A wide screen used to draw every row at once; it now starts at 25
  with the pager underneath, and choosing 50, 100 or All in the page-size menu
  is saved per computer (`page_length` in `settings.json`) and restored on the
  next load. Card view still always pages at 25.

## [7.2.0] - 2026-08-15

### Fixed
- **A card names every value again.** The card view's captions targeted cell
  selectors the page no longer emits, so a card was a column of bare values
  with nothing saying which column each one belonged to. One generic rule now
  captions every cell from its own `data-label`, so a column invented in the
  browser is captioned with no CSS change. The Name cell and the variations
  cell stay uncaptioned: one is the card's title, the other a pair of controls.

### Changed
- **An empty cell is no longer drawn on a card.** No more "Click to add..."
  and no bare "-" placeholders on a phone: a cell holding no value is left out
  of the card, and one line at the card's foot says how many empty columns were
  hidden and that the pencil fills one in. The wide table keeps its
  placeholders, which are the click targets there.
- The pencil that opens a row's form now answers Enter and Space from the
  keyboard, like every other row control.

### Removed
- Dead stylesheet rules from controls the rebuilt page no longer renders: the
  old fixed-size summary multiselect (`.cdd-*`), the old floating column
  show/hide panel (`.col-toggle`, `.col-toggle-panel`), the more/less clip
  toggle, the per-column card caption rules, and the per-card "hide empty
  data" class. The page is unchanged by their removal.

## [7.1.2] - 2026-08-14

### Fixed
- **The build script no longer stops programs it was not asked to stop.** It
  already stopped the one process recorded in its own PID file, which is the
  only one a build may replace. It then swept a second time by NAME, matching
  every process whose command line contained `modelsdb`: a developer instance
  on another port, a throwaway sandbox, and the server somebody was using at
  that moment. That sweep is gone.
- On Linux nothing has to be stopped to build at all, because a new file
  replaces the old one. On Windows a running program's file cannot be
  overwritten, and the PID file names which one to stop.

## [7.1.1] - 2026-08-14

### Changed
- **`README.md`, `ARCHITECTURE.md` and `DESIGN.md` are rewritten whole**, against
  the shape the app really has: one cell store, a column per namespace, and a
  computed column that resolves by the order of its inputs. Everything they said
  about fixed columns on the models table, custom columns in side tables, which
  columns a refresh phase owns on which model, a merge rule that kept the higher
  figure, a stored record of what was corrected by hand, and the ten named query
  filters is gone, because none of it exists.
- **`README.md`** now explains namespaces and cells in plain words, the nine
  columns a fresh install draws out of the 111 it holds, the Columns dialog
  (show, rename, colour, create, delete), the filter bar that builds itself from
  the columns on show, and a query-API tour whose every request was run against a
  throwaway server before it was written down.
- **`ARCHITECTURE.md`** documents the eight tables and their cascades, the one
  cell writer and what it refuses, the eleven transformations and why there is no
  expression language, colour rules as a column property, the package chain, the
  four refresh phases, the HuggingFace import and its duplicate rule, the write
  and read paths, the directory cascade and startup order, schema migration and
  the two kinds of backup.
- **`DESIGN.md`** documents a page that holds no list of fields: the table, the
  cell editor and where a typed value is routed, the filter bar, the address bar
  as the view, the Columns dialog, every dialog and why each is a native
  `<dialog>`, the two live regions behind the status toast, and where the
  interface stops.
- **`SECURITY.md`** now names every place the app reaches: OpenRouter, a
  HuggingFace repository when you ask it to read one, and GitHub for the update
  check.

## [7.1.0] - 2026-08-13

### Added
- **Any column can be coloured, by what it holds.** A colour rule is a property
  of the COLUMN, stored beside its heading and whether it is shown, so a column
  you invented shades exactly as one this app ships. The shape follows the kind
  of value: a NUMBER column takes ordered thresholds, each with a colour, and
  one colour for everything above the last of them; a YES-OR-NO column takes one
  colour for each answer; a ONE-OF-A-LIST column takes a colour per label. A
  threshold is read against the figure the cell SHOWS, which for a price is the
  price per million tokens.
- **The Columns dialog manages them**, beside showing, hiding and renaming: a
  "Colours" button per column, an editor whose fields follow what the column
  holds, and "No colours" to clear them.
- **`PATCH /api/columns` writes them**: send `"color_rules"` with a heading, or
  on its own, and an empty object clears them. `GET /api/columns` answers each
  column's rule. The served API guide documents the shape per kind of value.
- **Reading a model's repository again is back on its details.** The Details
  dialog offers "Re-import from HuggingFace" for a model that has a repository,
  and it NAMES what it will overwrite - the repository's own columns that hold a
  value now, and how many more it could fill - before anything is sent. It
  rewrites what the importer measures and nothing else: your note, your
  favourite star and every column of your own are untouched, because a source
  can only write its own columns.

### Changed
- **The price colouring is an ordinary instance of the new rule.** The two price
  columns ship with it - under a dollar green, under two yellow, above that red -
  so a fresh install looks as it did, and the rule can be changed or cleared like
  any other column's.
- **Re-typing a column of your own clears its colours**, because a rule written
  for one kind of value cannot be read as another.
- **A `PATCH /api/columns` naming neither a heading nor colours answers 400**
  rather than doing nothing.

### Removed
- **The Colors button and its per-pricing-unit threshold panel**, and the
  `color_ranges` key in `settings.json` that stored them. Colouring lives on the
  column now, where a column you made can carry one too.

### Database
- **Schema v33**: `columns` carries `color_rules`, and the two price columns are
  given the rule the table has always painted a price with. It reads and writes
  no value: a colour says how a value is drawn, never what it is. A rule already
  written by hand is left exactly as it is.

## [7.0.0] - 2026-08-13

### Changed
- **The browser page draws the catalog from cells, and holds no list of fields
  of its own.** It reads `GET /api/columns` and builds the table, the filter
  bar, the editors and the two model forms from it, so a column created,
  renamed or hidden at any moment changes all four. Nine columns are drawn on a
  fresh install - the ones the table has always shown, all of them computed -
  and every other column exists behind the column chooser.
- **Typing into a computed cell routes the value to the column that cell reads
  first**, which is the column named in `routes_to`, and the computed value is
  read back and redrawn. A computed column with no such column is READ ONLY: its
  cell has no editor, and pressing it says which column to write instead of
  swallowing the keystroke. A source column says the same.
- **Two visible columns sharing a heading each carry their namespace beside it**,
  so `derived.context_length` and `mine.context_length` can be told apart on
  screen.
- **The filters build a query in column keys and the server answers it.** The
  address bar carries the same parameters `GET /api/models` takes, so a link is
  a query that can be read before it is sent.
- **A column is named by its KEY in a saved layout and in a link.** A parameter
  naming no column - an `f_custom_<id>` from an older build - is dropped, named
  in the console, and the rest of the link still applies.
- **The column chooser manages columns through `/api/columns`**: show, hide,
  rename, create, and delete one of your own, which warns what it destroys
  before it sends the confirmation the server requires.
- **The variation picker's caption reads the account that published the
  repository** (`huggingface.repo_owner`), and its list of models to file is read
  from the whole catalog rather than from the rows the filters left on screen.

### Removed
- **The controls the new shape leaves nothing behind.** The pricing-note badge
  and its floating editor (the note is an ordinary column of yours now, edited
  where every other value is), the two "which facts to keep as columns" choosers
  (neither import route takes a choice), the Details dialog's re-import panel,
  and the fixed filters for year, type, modality, unit, links and capabilities,
  which are now one generated control per visible column.

## [6.0.1] - 2026-08-13

### Changed
- **The manual-research procedure is rewritten to the shape the server really
  speaks**, in both byte-identical copies (`internal/app/update-database-manually.md`
  and `.claude/skills/update-database-manually/SKILL.md`), and is 86,160 bytes no
  longer: it is 50,289. Every worklist is one `GET /api/models` filter naming a
  column key, every write is a `cells` object of `mine.` keys, and every `curl`
  printed in it was run against a throwaway server before it was written down.
  Gone with the shape they described: the `sqlite3` queries, the `custom_values`
  and custom-column mechanics, the per-source refusal tables, the 24-field table
  and the merge and hand-set rules. Kept whole: how to read a HuggingFace
  repository accurately, the pricing units, the duplicate-row rule and the
  notes-against-pricing-note convention.
- **The preamble served ahead of that procedure** names the five requests a
  research pass is built from and the two things HTTP cannot do, instead of a
  table mapping SQL steps onto endpoints.
- **The served API guide's remaining stale passages are corrected.** Its examples
  were still written in the flat pre-cell shape; it pointed at
  `GET`/`DELETE /api/custom-columns`, which no route serves; it described a
  `fields` list on `POST /api/models/variations` that the handler does not read;
  and it said a read's shape was not yet what a write takes, which it now is.

## [6.0.0] - 2026-08-13

### Changed
- **`POST /api/save` and `POST /api/models/add` take cells, keyed by column.**
  A request is now `[{"name":"...","cells":{"mine.notes":"..."}}]`, and every key
  is the same column key a read answers under, so a value read out of
  `GET /api/models` can be posted straight back with nothing looked up in
  between. Every value is sent as TEXT, and the column's own kind is what a
  figure, a flag, a list or a choice is checked against. The 24 flat field names,
  the `custom_values` object and the column NUMBERS they used are gone from the
  network entirely.
- **A write names your own namespace and nothing else.** A key belonging to a
  source (`openrouter.*`, `huggingface.*`, `collection.*`) or to a computed
  column (`derived.*`) answers 400 and names the `mine.` column to write instead,
  because a value written there would be replaced the next time that source is
  read or that column is worked out. A key with no namespace is refused the same
  way, naming the `mine.` key it looks like.
- **A save batch is honest, item by item.** Each item is applied on its own, in
  one transaction, and the answer carries a `results` array saying which items
  were written, which were refused and why, and which columns a write had to
  create. Every item written answers 200; some written and some refused answers
  207; nothing written answers the first item's own refusal, 404 for a model that
  is not here and 400 for a key or a value the write will not take. The old
  behaviour - stopping at the first bad item with a 404, having already written
  the ones before it - is gone.
- **One rule for a computed column: the first input that has a value.**
  `derived.context_length`, `derived.supports_tools`, `derived.supports_reasoning`
  and `derived.moe` took the largest figure, or counted a capability seen
  anywhere; each now takes the first of its inputs that states anything, and
  every one of them lists your own column first. So a correction DOWNWARDS is
  what the table shows: write 8192 where OpenRouter says 128000 and the merged
  column reads 8192, while what OpenRouter said stays beside it untouched.
- **The columns are managed at `/api/columns`.** `POST` creates a column of your
  own, `PATCH` changes its heading and nothing else, and `DELETE` removes it and
  every model's value for it - and requires `{"confirm":true}` in the BODY,
  exactly as `POST /api/models/delete` does. A column the app ships, and a column
  a computed column reads, are refused. `GET /api/custom-columns` is deleted with
  the numbering it spoke.
- **The research brief asks for cells.** `GET /api/models/brief` names each gap by
  its column key, says what heading it is shown under and whether the app ships
  it, and its worked example is the `cells` shape a save takes. A fact any source
  states is not a gap, so nothing asks for a length OpenRouter has already given.

### Removed
- The `highest` and `any_yes` transformations, which no shipped column uses any
  more. The other eleven are unchanged.
- `internal/store/tocells.go`, the write half of the translation between the old
  network shape and the cell store. What was real infrastructure inside it - how
  each source's reading is written into that source's own columns - moved to
  `internal/store/sourcecells.go` unchanged.

### Migration
- **Schema 32** rewrites the four computed columns' stored recipes from `highest`
  and `any_yes` to the first-that-has-a-value merge, and works those columns out
  again from the cells. A seeded recipe is never rewritten by the seed, so a
  catalog carried forward would otherwise keep the old rule for ever.

## [5.0.0] - 2026-08-13

### Changed
- **`GET /api/models` answers a model's identity and its cells, keyed by column.**
  A row is now `{name, source, hidden, variant_of, upstream, cells, _raw}`, and
  every fact about the model lives in `cells` under the key of the column that
  holds it - `openrouter.context_length`, `huggingface.parameters`, `mine.notes`,
  `derived.price_prompt`. Each value is the TEXT the database stores, so what a
  read gives back is what the database holds, character for character. A column a
  model has no value in is absent rather than empty. The flat object of fixed
  field names, and the `custom_values`, `set_by_hand` and `refresh_owned` fields
  that rode with it, are gone.
- **A query filters and projects by column key.** `?<key>=<value>` matches
  exactly ignoring capitals, `min:` and `max:` compare figures on a number
  column, and `contains:` searches inside a text or list column. On a list column
  an exact match finds one member; on a flag column yes, true and 1 are one word.
  An empty value matches the models with no value in that column. `fields=` names
  identity fields and column keys in one list, and the rows come back carrying a
  `cells` object holding only the columns asked for. Every column is filterable,
  including one the reader invented, with no code change.
- **A parameter this query does not take answers 400 and names it**, instead of
  being ignored - which used to answer with the whole catalog while looking like
  a narrow answer. The refusal offers the columns whose KEY comes closest and the
  columns whose LABEL comes closest, both, and a name that merely shares a word is
  offered too, so `min_context` is pointed at `derived.context_length`.
- **The merged columns are read as they are defined.** `derived.context_length`
  takes the highest figure any column states and `derived.supports_tools` says
  yes where any of them says yes, the reader's own column included rather than
  ahead of the others. A correction downwards is therefore answered in
  `mine.context_length`, which no refresh can reach, while the merged column
  beside it goes on stating the highest figure. Both are in every answer, under
  keys of their own.
- **The served API guide describes the shape the server speaks**, in its whole
  read half: the row, the column list, the grammar, the projection, the refusal,
  and a table taking each deleted parameter to the column key that replaces it.

### Added
- **`GET /api/columns`** answers every column this catalog holds - key,
  namespace, label, kind of value, unit, position, whether it is shown, whether
  the app ships it, and a choice column's options. On a COMPUTED column it also
  answers `routes_to`: the column of the reader's own that a value typed into it
  is stored in, empty where that column is read only. It carries no ids, because
  nothing over the network speaks in ids.

### Removed
- **The ten named query filters**, with no aliases: `type`, `input`, `output`,
  `tool`, `reasoning`, `zdr`, `min_context`, `max_in`, `max_out` and `all=1`.
  Each of them now answers 400 naming the parameter. `name` and `include_hidden`
  stay, because they filter a model's identity rather than a column, and
  `?include_hidden=0` is what asks for the whole catalog as JSON.

## [4.1.1] - 2026-08-13

### Fixed
- **The served API guide no longer describes machinery that was deleted.**
  `GET /api` is what this server SENDS to callers, so every sentence in it has to
  be true of the server that sent it. Three passages had outlived the one-cell
  redesign and were corrected:
  - *"Deleting a column a source fills also unticks that fact."* There is no tick
    and no fact chooser. The guide now says what really happens: no refresh,
    import or scrape ever creates a column of yours, so a column you delete stays
    deleted, and creating it again brings it back empty.
  - *"Every cell records who put the value there."* A value records no author. A
    column belongs to whoever writes it: a refresh writing as OpenRouter cannot
    name a column of yours at all, which is why nothing you write is ever
    replaced.
  - *"chosen"* in `GET /api/import-fields` is now documented as always true, with
    the reason - every source records every fact it states, on every read - and
    the field is kept only so a caller written against an older answer does not
    fail on a missing key.
- **Every other statement the redesign had falsified**, swept out of the guide
  and out of both copies of the manual-research procedure: the refresh-phase
  ownership rules and the table of which columns are refused on which models
  (nothing is owned, and all 24 fields go over HTTP on every model); the
  `"fields"` list on the import and variation routes, which no longer exists; the
  claim that a price on a collection model needs a shell and `sqlite3`; the
  `set_by_hand` column, which is worked out on every read rather than stored; the
  `refresh_owned` map, which is always empty; the SQL examples, which named
  tables and columns that have been dropped; the three renamed HuggingFace
  columns; and the endpoint count, which was ten routes out of date.
- **The manual-research procedure's rounding claim.** It said the three measured
  figures are stored at ONE decimal place and gave `32.8` as the stored form of
  `32.762123264`. They have been stored at TWO decimal places since 3.13.0, and
  the figure is `32.76`.
- **Every SQL query in the procedure now runs.** They read `models`, `columns`
  and `cells`, and each was executed against a copy of a real catalog before
  being written down.

## [4.1.0] - 2026-08-12

### Removed
- **The three tables a personal column used to live in.** `custom_columns`,
  `custom_column_options` and `custom_values` are gone. A column you own is a row
  of `columns` like every other column, its dropdown labels are `column_options`
  and its values are cells - one shape, one write path, one read path. Migration
  v31 moves each definition across, proves every stored value can still be read
  back out of the cells, and only then drops the tables.
- **The record of who wrote each value** (`custom_values.written_by`). It existed
  so a source and a person could share one column. They never share one now:
  what a source states is in that source's own column and what you write is in a
  column of yours beside it, so there is nothing to arbitrate.
- **A column's second identity** (`custom_columns.fact_key`) and the table that
  paired a column NAME with a fact. A column's key is what it is, from the moment
  it is created, and renaming one changes only the heading.
- **The fact chooser.** There is no ticking facts on or off any more, and no
  remembered choice: `app_settings` is dropped with the two keys it held. A
  source column is not a choice - it holds what the source said - so every fact a
  source states is recorded, and you never lose one because you had not ticked
  it. What you choose is which columns your table draws. A refresh therefore
  creates none of your columns, which is also why one you delete stays deleted.

### Changed
- **A personal column's number on the network is a new number.** It used to be
  its row in the old side table and it is its row in `columns` now, raised by a
  fixed base so a small number saved against the old table can never name a
  different column by accident. A filter your browser saved under an old number
  matches nothing and is dropped, rather than quietly filtering by whichever
  column now sits there. Everything else `GET /api/models`,
  `GET /api/custom-columns` and `GET /api/import-fields` answer is unchanged,
  byte for byte, on a real 642-model catalog.
- **`POST /api/models/import` and `POST /api/models/variations` no longer take a
  `fields` list.** Everything the repository states is recorded either way.

## [4.0.0] - 2026-08-12

### Removed
- **The twenty-seven fact columns leave `models`.** A model row now holds its
  identity and nothing else: its name, the route that created it, when it was
  last written, whether it is hidden, and which model it is filed under. Every
  fact about a model - what it costs, how much context it takes, what it takes in
  and gives out, whether it calls tools, the note somebody wrote about it - is a
  cell, in a column belonging to whoever stated it. Migration v30 drops the
  columns, and it refuses itself on a catalog whose values have not been carried
  into cells, so it can never drop the only copy of anything.
- **The whole write-permission machine.** A column used to be writable on one
  model and refused on another, and answering "which automatic pass would
  overwrite this column on this row" took a list of the columns a person could
  write, a rule per phase, a table of retired column names, and a record on every
  row of which values had been fixed by hand. None of it exists. Two sources
  never reach one value now, so the question is never asked: what a source states
  is in that source's own column, and an answer you write goes in a column of
  yours beside it.
- **`models.raw_json` and `models.set_by_hand`.** What each source last sent is
  kept per source in `source_records`, so a repository reading no longer
  overwrites an OpenRouter one. Which facts you have answered yourself is worked
  out on every read, from which of your own columns hold a value.

### Changed
- **A value you empty is you saying nothing, and a source may fill it again.**
  There is no record of a value having once been emptied on purpose and nothing
  behaves as though there were.
- **A source that reads a model again is believed.** A capability the OpenRouter
  listing stops reporting now disappears instead of being kept for ever. A
  capability seen in a DIFFERENT source still counts as present, and your own
  answer still stands over every source.
- **No write is refused for belonging to a refresh phase.** Every field
  `POST /api/save` and `POST /api/models/add` take is accepted on every model,
  and the research brief asks about every empty one.
- **The two token prices you write must read as a number.** They arrive as text,
  as they always have, and they land in a column that holds a figure.

### Fixed
- A variation recorded from a repository now carries what it shares with the
  model it was made from - its kind, its context length and its modality lists -
  where the app reads them, so a build no longer draws as a blank line.

## [3.26.0] - 2026-08-12

### Changed
- **Every answer this server gives is now built from the cell store.** A model's
  row carries only its identity - its name, the route that created it, whether
  it is hidden, and which model it is filed under - and every other fact about
  it is read out of the cells. The old columns are still written and nothing
  reads them any more.
- **Nothing on the network moved.** `GET /api/models`, `POST /api/save`,
  `GET /api/custom-columns` and every other route answer exactly the bytes they
  answered before, produced from cells by one translation that puts each value
  back under the name its old column had. That translation is deleted in the
  change that makes the network speak column keys.
- **Zero Data Retention is a reading where there is a listing to read, and a
  rule where there is not.** What OpenRouter's providers say is stored; a model
  no OpenRouter listing covers has nobody serving it, so the answer is yes
  without anything having to state it, whether or not a pass has ever run.

### Added
- **Five more columns for what a source's record implies rather than states**:
  the kind of model and the pricing unit an OpenRouter listing describes, the
  mixture-of-experts answer its description describes, and the kind of model, the
  description and the listing date a repository publishes. Each was worked out at
  the moment a record arrived and had nowhere of its own to live.

### Fixed
- **A measured figure is held at two decimal places in the cell store too**, so
  the parameter counts and the weights size read back as the figures they are
  stored as rather than at full precision.

## [3.25.0] - 2026-08-12

### Added
- **Every writer now records what it wrote as cells as well**, beside the
  columns it has always written. The OpenRouter pull and the ZDR pass fill the
  `openrouter.*` columns, the collection and pricing scrapes fill
  `collection.*`, the repository import and the variation importer fill
  `huggingface.*`, and every value typed by hand fills `mine.*`; each source's
  verbatim record goes to `source_records` under that source's name. Nothing
  reads a cell and no answer this server gives changes: the two are written side
  by side so the cells can be proved right before anything depends on them.
- **The columns this app ships are put in at startup**, after the migrations,
  and putting them in again changes nothing - so a column hidden, relabelled or
  given different options stays exactly as it was left.

### Notes
- The double write is TEMPORARY and lives for one release. The change after this
  one removes the old columns, and the double write goes with them.
- A value the cell store will not take is logged and passed over rather than
  failing the write that carried it, because the old columns are still what
  every answer is built from.
- Against a copy of a real catalog of 647 models: one startup fetch and one full
  refresh wrote 17,388 cells with no refusal, and holding 8,934 values of the
  old shape against the cells found 31 differences, every one of them a model
  the live listing no longer carries, a price page that could not be read that
  run, or a merged column keeping a figure upstream has since reduced.

## [3.24.0] - 2026-08-12

### Added
- **Everything a database already holds is carried into the cell store**
  (schema v29). Every model's fixed fact columns, every custom column with its
  option list and every stored value, and the verbatim upstream record all
  arrive as cells, each in the column that states that fact: what OpenRouter
  sent under `openrouter.*`, what a repository stated under `huggingface.*`,
  what a collection or price page stated under `collection.*`, and the note, the
  star, the pricing caveat and every column he invented under `mine.*`. Against
  a copy of a real catalog of 642 models: 1,300 stored values, 167 notes and 12
  favourites all reached a cell, and 588 upstream records reached
  `source_records`.
- **`huggingface.active_parameters`, "Active parameters (B)"**, beside
  `huggingface.parameters`. A mixture-of-experts model runs only some of its
  parameters per token, so that figure gets a source column of its own rather
  than being ambiguous inside the total.

### Changed
- **`openrouter.supported_parameters` holds a LIST**, not text, so one member
  can be searched for and the column does not sort by its own first letters. A
  column the app ships can never be re-typed, so it is decided now.
- **`huggingface.weight_bits` becomes `huggingface.declared_weight_bits`**,
  labelled **Declared weight bits**: it is what the repository's own
  configuration claims, as against `huggingface.build_bits_per_weight`, which is
  the same quantity measured off a downloadable build's files.
- **`huggingface.weight_storage` becomes `huggingface.weight_file_format`**,
  labelled **Weight file format**: it names a file format, safetensors or a
  pytorch `.bin`, and "storage" read as where or how much.
- **The size pair carries its units in both labels**, "Weights size (GB)" and
  "Weights size (bytes)", so a reader meeting either one alone can tell which
  figure it is.

### Note
- The migration is ADDITIVE: no old table, column or value is changed, so the
  database holds everything twice when it finishes and the app goes on reading
  and writing the old shape. Nothing reads a cell yet.

## [3.23.0] - 2026-08-12

### Changed
- **The two kinds of author are two columns, and nothing merges them.**
  `openrouter.author` becomes `openrouter.vendor`, labelled **Vendor**: who
  OpenRouter attributes the model to, read off the part of the id before the
  slash (`google`, `mistralai`, `nvidia`). `huggingface.author` becomes
  `huggingface.repo_owner`, labelled **Repo owner**: the account that published
  that repository, which is often somebody who republished a quantized build
  rather than the model's maker (`unsloth`, `bartowski`). Both columns sort and
  filter on their own.
- **Two prices of his own ship**, `mine.price_prompt` and `mine.price_completion`,
  taking the shipped personal set from eight columns to ten. Each is the FIRST
  input of the computed price column beside it, so a price typed into the table
  lands there - which is what lets the price of a model added by hand, or one
  that arrived from a collection page, still be corrected.
- **`mine.tool` and `mine.reasoning` are renamed** `mine.supports_tools` and
  `mine.supports_reasoning`, so a personal column carries the same word as the
  source columns stating the same fact.
- **A scraped token price says it is a token price.** `collection.price_prompt`
  and `collection.price_completion` are in USD per token, not "per unit": the
  scrape fills them only where the page prices the model per token.
  `collection.measurement` becomes `collection.price_unit`, "Priced per", and
  `collection.price_image` becomes `collection.price_image_output`, "Image output
  $", because it holds the price of a PRODUCED image - a different fact from
  `openrouter.price_image`, the price of an image sent in.
- **`huggingface.bits_per_weight` becomes `huggingface.build_bits_per_weight`**,
  "Build bits per weight", so it cannot be read as `huggingface.weight_bits`
  beside it: one is measured off a downloadable build, the other is what the
  repository's config declares.
- The ZDR column now says what it means: at least one company serving the model
  keeps no copy of what you send it.

### Removed
- **`derived.author`.** Merging a model's maker with the account that republished
  a copy of it produced a column whose meaning changed from row to row.

### Added
- **A computed column may only merge ONE fact, in ONE unit.**
  `TestEveryComputedColumnMergesOneFactInOneUnit` compares the word after the
  namespace, the unit and the kind of value of every input against the computed
  column that reads it, so merging unlike things now fails the build.

## [3.22.0] - 2026-08-12

### Added
- **Computed columns: a column whose value is worked out from other columns.** A
  recipe is one named transformation plus the columns it reads, IN ORDER, stored
  in `derived_columns` and `derived_inputs` rather than written in Go, so it can
  be read and changed. Merge ships first - take the first input that has a value
  - and twelve more join it: `highest`, `lowest`, `any_yes`, `to_number`,
  `multiply`, `divide`, `bytes_to_gigabytes`, `round`, `first_of_list`,
  `union_lists`, `join_text` and `map_values`. There is no expression language
  and no stored formula, because this server has no login: a case none of the
  thirteen covers is refused with the file and the shape of the entry to add.
- **A computed value is worked out the moment an input changes, for that model
  alone**, inside the same transaction as the write, and a computed column may
  read another so a chain of small steps is three columns rather than a formula.
  Changing a recipe rebuilds its own column for every model and stamps
  `rebuilt_at`; `store.RebuildDerivedColumn` and `store.RebuildEveryDerivedColumn`
  are the repair verbs.
- **A recipe that could never work is refused at the moment it is written**: a
  transformation nobody ships, a missing parameter, a column that does not exist,
  a kind of value the transformation cannot read, too many inputs, and a
  definition that could reach itself - which is refused naming the loop.
- **Deleting a column a computed column reads is refused**, naming every computed
  column that reads it, because removing it silently would change what a figure
  on screen means with nothing to say so.
- **The column set the app ships, seeded into a catalog that does not have it.**
  101 columns: 33 `openrouter`, 41 `huggingface`, 10 `collection`, exactly 8
  `mine` and 9 `derived`. Nine are visible - type, context length, both prices,
  tool, reasoning, ZDR, note and favourite - and every other column exists, holds
  its values and sits behind the column chooser. Seeding is idempotent and
  additive: it creates nothing twice and never touches a label, a position, a
  visibility, a kind or an option list somebody has changed.
- **Where a typed value lands, per computed column.** `store.RoutingTargetOf`
  answers the first `mine` column a computed column reads, and answers nothing at
  all for one that reads none - which means it is read only and must say so
  rather than swallowing the keystroke.

## [3.21.0] - 2026-08-12

### Added
- **The cell store: one fact about one model, stored as one row.** Six new
  tables - `columns`, `column_options`, `cells`, `derived_columns`,
  `derived_inputs` and `source_records` - hold a column definition, its options,
  a model's value in it, a computed column's definition and inputs, and the
  verbatim record each source last sent. A column carries a permanent key
  (`openrouter.context_length`) whose first word is the namespace that owns it,
  and a label that is shown, editable and free to repeat, so two sources stating
  the same thing are two columns and nothing has to decide which of them wins.
- **One writer for every cell, which is what holds a value and its sort key in
  step.** `store.WriteCells` reads the column's declared kind and stores the text
  as it was written plus the number the column sorts by: the figure for a number,
  1 or 0 for a flag, and the option's POSITION for a choice, so `fast, avg, slow`
  stays a scale instead of being reordered into an alphabet. Text and lists sort
  by their own letters and carry no number. A value the kind will not take is a
  refused write, never a stored value with no sort key, because a value that
  fails quietly would clear the cell instead. The whole call is one transaction.
- **A column the maintainer owns can be re-typed, and every one of its cells is
  worked out again in the same transaction.** One cell the new kind will not take
  refuses the whole change and names the model that blocks it. A column the app
  ships cannot be re-typed at all.
- **A source's column can be hidden and never deleted; the maintainer's own can
  be deleted freely.** OpenRouter, a HuggingFace repository and the collection
  pages state their columns again on every read, so a deletion of one is refused
  with the offer to hide it instead, which keeps every value.
- **A `mine.` key nothing holds yet mints its column on first write**, and the
  write answers with what it created, so a mistyped key is visible in the same
  reply that made it.

### Changed
- **Schema version 28** adds the six tables above and nothing else. It writes no
  row and touches no existing table, column, index or value, so every model,
  custom column, option, stored value and ignore entry comes through it exactly
  as it went in.

## [3.20.0] - 2026-08-11

### Added
- **A custom column now has an identity of its own, so it can be renamed and can
  stay deleted.** Every column carries `custom_columns.fact_key`, the catalogue
  fact it holds; a column the user invented holds none. A source finds its column
  by that fact first and by the column name second, so a renamed column keeps
  receiving its values, and a deletion is recorded against the fact rather than
  against a name the user is free to change - which is what stops the next
  unattended refresh re-creating a deleted column under a new id.
- **`PATCH /api/custom-columns?id=<id>` with `{"name":"..."}` renames a column.**
  It changes the heading and nothing else: every stored value stays, because a
  value names its column by id. A name another column already holds is refused
  with 400, matched ignoring capitals.
- **`GET /api/custom-columns` says whether anything can fill each column.** Each
  definition carries `fact_key` and `filled_by_a_source`, so a column no source
  writes any more can be seen and removed by choice instead of sitting there
  unexplained.

### Changed
- **A Speed column that holds values is the user's own from now on.** Schema v27
  pairs a column with a fact only when its name matches the catalogue exactly,
  and Speed is no fact this app records, so such a column and every one of its
  values come through untouched and end up with no fact key. Nothing is deleted
  and nothing is renamed. Rating and OCR are unchanged.

### Database
- **Schema v27** adds `custom_columns.fact_key` (nullable, unique) and fills it
  in by matching existing column names against the fact catalogue, exactly and
  ignoring capitals. A name no fact fills is left with no key, which is the safe
  answer: that column behaves exactly like one the user invented. One fact is
  claimed by one column only, so a database holding two columns whose names
  differ by capitals alone keeps both. Purely additive - no value moves, no
  column is removed, and every pairing it makes is logged with its reason.

## [3.19.0] - 2026-08-11

### Added
- **`GET /api/models/brief?name=<model>` answers what one model still has no
  answer for, and the instructions for filling it in.** The answer carries
  `gaps`, one object per field, each naming exactly the key `POST /api/save`
  takes on that row, where the value goes (the top level of the object, or
  `custom_values`), what the row holds now, what that column accepts, and
  whether the column is one a HuggingFace repository is READ for rather than
  typed. `brief` is the same answer written out as text to hand to an assistant,
  addressed to the address the caller reached this server at (`?origin=`) with
  every address this instance serves on listed beside it.

### Changed
- **The research brief is written by the server, from the rules the write path
  enforces.** It used to be composed in the browser's JavaScript from a
  hand-written list of field names in Go, and the two had drifted from the write
  path: three of the names it handed out - `active_parameters`, `parameters` and
  `disk_size_gb` - are refused by `POST /api/save`, which names the custom column
  to use instead. The list is now computed per row from the writable columns,
  the phases that own a column on that row, and the custom columns this database
  really has, so a field the brief asks for is a field the API takes.
- **The brief says what the row holds now**, per field, rather than announcing
  that everything it names is empty: a flag reads as `false`, `moe` reads as
  `unknown`, and a column with a value is not asked about at all.
- **The brief names the values each field accepts** - `tool` as "yes" or "no",
  `moe` as "yes", "no" or "unknown", a flag as true or false, a dropdown column
  by its own options - and asks for the sources that exist, saying plainly that
  a model with no paper and no provider page is normal and a source that cannot
  be found is not a failure.
- **The brief's worked example carries placeholders that cannot parse.** It used
  to print a plausible value for the model in question, so running it as printed
  recorded a fact nobody had established; every value in it is now an unquoted
  `<...>`, which is refused with 400 and writes nothing.
- **The brief carries the rule for a repository publishing more than one copy or
  format of its weights**, extending the served procedure's shard-index rule:
  count only the files `model.safetensors.index.json` names, and never a
  consolidated copy, an `original/` or `metal/` folder, or a quantized mirror.
- **`POST /api/models/import`'s `"missing"` is read back off the row** it just
  wrote, in that same per-field shape, instead of being a fixed list of names.
  Its `"kept"` now lists the facts that really became columns on that import,
  which is what the guide always said it was.

## [3.18.0] - 2026-08-11

### Fixed
- **A refresh phase that could not do its work now says so.** The collection
  scrape and the pricing scrape used to log every failure and then report success
  regardless, so `GET /api/update/status` could never carry a `collections:` or a
  `pricing:` entry however many pages had failed - a run where every collection
  page was unreachable presented itself as a clean finish. Both phases now answer
  with an error carrying the exact number of failures and the first five of them
  by name.
- **A failing page still does not discard what the others produced.** Each phase
  goes on past a failure exactly as before, so one unreachable collection page
  leaves the other five applied and one unreadable model page leaves every other
  model priced. The report is what is new, not the abandoning of work.
- **The database itself refuses a model with no name.** `models.name` carries a
  `NOT NULL` constraint (schema v26). It is the handle every route addresses a
  model by and the key every personal value is stored under, and SQLite otherwise
  permits a missing value in a `TEXT PRIMARY KEY` column, so such a row could sit
  in the table permanently out of reach. A row that somehow holds no name is
  given one ("Unnamed model") rather than deleted; a blank name is left as it is.
  No row is rewritten and no table rebuilt.

### Changed
- **The `app` package's refresh-phase type is called `progressPhase`**, with the
  constants `progressIdle`, `progressCatalog`, `progressCollections` and
  `progressPricing`. It shared a name with an unrelated type in the `store`
  package, where a "phase" is which pass OWNS a column rather than which pass is
  running. The values the status endpoint sends - `idle`, `catalog`,
  `collections`, `pricing` - are unchanged.
- **The served API guide states that a partial failure is reported**, so
  `GET /api/update/status` documents what an entry in `errors` now means.

### Removed
- **`collectionDef.setTool` and the branch it guarded.** No collection set it, so
  nothing could ever run that code.

## [3.17.0] - 2026-08-11

### Changed
- **A model is recognised by its repository and its upstream id, not only by its
  display name.** Adding or importing a model now asks one question first: does
  this catalog already hold it? The answer is looked for by the HuggingFace
  repository (`hf_slug`, ignoring capitals), then by the OpenRouter `id`, then by
  the name - in that order, stopping at the first row found. `POST
  /api/models/import` and `POST /api/models/add` both ask it.
- **Importing a repository this catalog already holds answers 409 and creates
  nothing**, naming the row that holds it and which identifier matched. It used
  to create a second row whenever the name it invents from the repository address
  differed from the name the row is stored under - which is nearly always, since
  an OpenRouter listing carries its own name for the model. Two rows for one
  model split a note, a favourite star and every column value across the pair
  with nothing saying so.
- **`"reimport": true` writes onto the row that already holds the model**,
  whatever that row is called, and the answer's `name` says which row was
  written. What it touches is unchanged: the facts the repository states, the
  repository, the description of a row that has none, the modality lists and the
  context length under the merge rule. Notes, the favourite star, the hidden flag
  and every value written by a person stay exactly as they are.
- **The ignore list is asked before anything else on an import.** A withheld
  repository is refused as withheld whether or not a re-read was asked for.
- The served API guide and both copies of the manual research procedure describe
  the new refusal and the re-import.

## [3.16.0] - 2026-08-11

### Added
- **The catalog refresh now fills your own columns, not only the app's.** Every
  fact OpenRouter states that no built-in column holds is recorded as a custom
  column, one per fact, created the moment its first value arrives: the author,
  the tokenizer family, the prompt template, the request parameters the model
  accepts, the longest answer it will produce, whether the provider moderates
  it, the knowledge cutoff, the expiry date, whether reasoning is mandatory,
  which model a listing is an alias of, and the cache, image-output,
  audio-output and web-search prices. A first refresh on an empty install
  created 17 columns and wrote 3,030 values across 405 models.
- **One catalogue of facts for every source.** The list that used to describe
  what a HuggingFace import can read now describes every fact this app can
  record, and each entry names the sources that can state it. A source can only
  ever write the columns for the facts it is listed against.
- `GET /api/import-fields` carries a `sources` field per fact, and covers the
  facts the OpenRouter refresh reads as well as the ones an import reads. It is
  still the one list the import dialog draws and an agent reads.

- **Every value in one of your own columns now records who wrote it** (schema
  v25), and that is what decides whether a refresh may change it: an empty cell
  it fills, a value an earlier refresh wrote it may correct when the source
  revises the figure, and a value you wrote it never touches. Every value your
  database already held is treated as yours, so nothing you have entered can be
  overwritten by this change. Clearing a cell forgets who wrote it, which is how
  you ask for the source's answer back.

### Changed
- **Deleting a custom column that holds a catalogue fact also unticks that
  fact**, in the same request, so a column you remove is not created again by
  the next unattended refresh. Tick it again in `GET /api/import-fields`, or
  name it in a `fields` list, to start recording it once more.
- **A fact this app has learned to record since you last chose starts ticked.**
  Your saved choice now records which facts were on offer when you made it, so a
  fact added afterwards counts as a question nobody asked you rather than one you
  said no to. A fact you unticked stays unticked, and one you kept stays kept.

## [3.15.1] - 2026-08-11

### Added
- A generator that reads the database shape and the write rules out of the
  running code and writes them out as documents, so a description of either can
  no longer drift from what the code does. It reports every table, column,
  foreign key and index, and for each column the app maintains, which part of a
  refresh claims it on which kind of row.

## [3.15.0] - 2026-08-11

### Changed
- **A new install is no longer given a "Speed" column it never asked for.** The
  app ships only the columns it maintains itself, and Speed was neither of
  those: it was created on every database, empty, whether or not anybody wanted
  a place to record how fast a model answers.
  - A fresh install now starts with two personal columns, **Rating** and
    **OCR**, instead of three.
  - An upgrade from a version old enough to still hold the original fixed
    `speed` field keeps every value in it, in a Speed column, exactly as before.
    A value is never left without a column to arrive in.

### Removed
- **An empty Speed column is removed from an existing install** (schema
  version 24). It is removed ONLY where it holds no value at all. A Speed column
  somebody has used is left completely alone - the column, its dropdown options
  and every stored value - because deleting a personal column permanently
  deletes every model's value for it, and your database is the only copy of
  those values.

## [3.14.0] - 2026-08-11

### Added
- **A model already in your catalog can be read from HuggingFace again.**
  Importing a model that is already here used to fail outright, which left one
  kind of data impossible to fill in at all: the shape figures a sizing tool
  reads - what one token of context costs, the layer and head counts behind it,
  how a weight is stored - are MEASUREMENTS, and the update procedure forbids
  anybody to type one, so they can come from an import and from nowhere else. A
  model added before those columns existed could therefore never get them.
  - In the **Add model** dialog, an import that lands on a name already here now
    offers to read the repository again.
  - In every model's **Details** dialog, **Re-import from HuggingFace** does the
    same thing where a model you already have is normally met. It appears only on
    a model with a HuggingFace repository recorded.
  - Over HTTP, `POST /api/models/import` takes `"reimport": true` and answers
    `200` with `"reimported": true`, so the procedure an AI assistant follows can
    do it without the browser.
- **Both surfaces say what will be overwritten before anything happens**, naming
  the columns of yours it rewrites and counting the ones it may create, because
  none of it can be undone.

### Changed
- **A re-import touches nothing personal.** Your note, your favourite star,
  whether the row is hidden, every column you made yourself, and any value you
  typed into one of the importer's columns that the repository does not state are
  all left exactly as they are. It rewrites only what the repository states.
- **Importing a name that is already here still fails without that flag**, so a
  repeat import cannot quietly overwrite a row, and the refusal now names the way
  past it. The ignore list still refuses either way: a re-import is not a way past
  a model you have withheld.

## [3.13.0] - 2026-08-11

### Changed
- **`Parameters (B)`, `Active parameters (B)` and `Weights size (GB)` are stored
  at two decimal places, not one.** The second digit is the difference between a
  figure and no figure at all at the small end of the catalog: a
  350-million-parameter model is `0.35` billion, and a single decimal place
  cannot hold that - it says `0.4`, a number nothing states about the model.
  `Qwen/Qwen3-32B` now stores `32.76` where it stored `32.8`, and its weights
  size `65.52`. A whole number still stays whole (`2.00` reads `2`), a trailing
  zero still goes (`33.10` reads `33.1`), and anything that does not read as a
  number is still left exactly as it arrived. The browser rounds a cell the same
  way the moment an edit commits, so the screen and the database agree.
- **Figures already stored at one decimal are not rewritten, and no migration
  pretends otherwise.** Rounding is one-way: `32.8` cannot become `32.76` again
  by arithmetic, because those digits are gone from the database and exist only
  in whatever source they were read from. Any such row is corrected by re-reading
  that source, which is a research pass rather than a schema step.

## [3.12.0] - 2026-08-11

### Changed
- **When two sources disagree about a model, ModelsDB keeps the fuller answer.**
  OpenRouter describes the endpoint it serves; a model's own HuggingFace
  repository describes the model. They do not always say the same thing, and
  whichever was written last used to win. Now the **higher figure wins and a
  capability listed in either source counts as listed**, across four columns:
  the context length, thinking, tool use and mixture of experts. [OBS] A
  LiquidAI card states 32,768 context while its config states 128,000, and the
  stored value is 128,000; OpenRouter reports that the endpoint for
  `LiquidAI: LFM2-24B-A2B` exposes no tools while its card documents native
  function calling, and the stored answer is "yes". The modality and collection
  lists already worked this way and are unchanged.
- **Your own edit beats that rule, and can bring a value down.** "The higher
  wins" with no exception would mean a wrong high figure could never be
  corrected: every refresh would put it straight back. So writing one of those
  four columns - in a cell, in the row's form, or over HTTP - records that you
  have fixed it, and no refresh touches it again until you change it. The record
  is a new `set_by_hand` field on every row of a `GET /api/models` answer, so
  you can see which values a refresh may still raise.
- **The context length and the thinking flag are writable on an OpenRouter model
  now.** Both were refused there because the catalog fetch used to assign them
  outright; it merges them instead, so the refusal would have made a wrong value
  permanent. The catalog fetch assigns ten columns outright rather than twelve,
  and the identifiers, description, creation date and the five raw prices are
  refused exactly as before.
- **Prices are not merged**, deliberately. A price is what the model costs
  today, and the newest reading is the true one; keeping the higher of two would
  quote a price nobody charges. Nor are the columns you own: each is measured by
  one source at one moment.

### Added
- **`models.set_by_hand`** (schema v23), the record of which merged columns a
  person has fixed on a model. Purely additive: every existing row starts empty,
  which means every merged column on it is still open to a refresh.

## [3.11.0] - 2026-08-11

### Changed
- **A refresh stores every model OpenRouter returns, free listings included.**
  OpenRouter lists a model's free endpoint as a model of its own, named
  `... (free)`, and the refresh threw every one of them away before anything
  looked at it. That was this app deciding which real models you were allowed to
  keep, on nothing but a word in the name. It stores all of them now, each with
  its own name, id and price. The **ignore list is untouched** and still holds:
  withholding a model is your instruction about your own catalog, which is a
  different thing entirely.
- **A collection still enriches the base model, not its free listing.** A free
  listing carries the paid model's canonical slug, so both rows answer to it.
  When a collection lookup falls through to the slug it now takes the row whose
  id carries no `:free` suffix, and takes the same one every run.

## [3.10.0] - 2026-08-11

### Fixed
- **A model in your database can no longer be missing from your database.** The
  read path dropped two shapes of row before anybody could see them: a row whose
  name carried `(free)`, and a row with no name. A dropped row is not merely
  absent from a list - every write route looks a model up by the name a read
  returned, so those rows could not be listed, could not be saved to (`POST
  /api/save` answered 404), could not be hidden and could not be deleted. They
  existed and no verb in this app could reach them, with nothing on the screen
  to say so: a shorter list looks exactly like a smaller catalog. [OBS] In a
  live 626-model catalog, 22 rows were in that state. Every stored row is now
  returned, and the hidden ones come back with `include_hidden=1` as they always
  did.
- **A record with no name is refused where a row would be created, instead of
  being dropped where rows are read.** A name is the only handle this app has on
  a model, so a row stored under an empty name could never be read, written,
  hidden or deleted. All four paths that create a row now answer with a refusal
  naming the reason, and the catalog refresh reports how many nameless records
  arrived rather than passing over them in silence.

## [3.9.0] - 2026-08-10

### Added
- **The brief for an AI assistant now names every address this ModelsDB answers
  on, and says which one is for what.** The brief carried a single address: the
  one typed into the browser that produced it. Read the app at a machine's
  network address - a Tailscale address, a LAN address - and every brief handed
  an assistant that address, including one running on the very machine ModelsDB
  runs on, where `http://127.0.0.1:8122` is what it should use. The server
  listens on both, but nothing told the page so. It does now: `GET /api/health`
  returns an `addresses` list, and the brief prints it above the steps with a
  plain reason for each line - the loopback address only from a process on the
  same machine, the network address from another machine that can reach it. The
  address the reader is on leads the list and is what every URL in the brief is
  written with, from the guide link to the closing `curl`, and one sentence says
  any listed address can take its place. Nothing is guessed: an install serving
  loopback alone says so in one line and invents no network address, and an
  address is listed only because its listener really bound - an extra address
  that fails to bind is never offered.
- **`GET /api/health` reports the addresses this instance is serving on.**
  `addresses` holds the base URLs of the listeners that bound, loopback first. A
  wildcard bind (`0.0.0.0`) is expanded into the addresses it really answers on -
  loopback plus this machine's own interface addresses - because nobody can send
  a request to `0.0.0.0`.

## [3.8.0] - 2026-08-09

### Fixed
- **The PID file can no longer name a server that is not there.** The file that
  records which process is running ModelsDB was written well before that process
  actually took its port. A start that then failed - the port
  already busy, the database unopenable - left the file naming a process that
  never served, on top of the record of the healthy server that was still
  running. From then on `modelsdb stop` refused to stop that server, because the
  record it consulted described a process that no longer existed. The order is
  now fixed the other way round: the port is taken first, and the record is
  written only once the port belongs to this process. A start that cannot bind
  leaves the existing record exactly as it found it, and a start that dies after
  binding removes its own record on the way out. The file names a process that is
  listening, or it names nothing.
- **`modelsdb stop` stops a server that is serving.** Stop now asks the PORT who
  is serving there, and only falls back to the PID file when nothing answers, so
  a stale or missing record can no longer leave you with a running server and no
  way to stop it. When the port is held by something it may not stop - a process
  that is not ModelsDB, or another install's server on the same port - it says
  precisely what it found and stops nothing. `status` and the menu report from
  the same source. Stopping with the port already free clears a leftover record.

### Changed
- **The port is taken before the database is opened.** Whether a data dir is
  already taken is judged by whether its port is bound, so a process that held
  the database while binding nothing could be missed entirely by a second start,
  which would then open the same database. Holding the port for the whole time
  the database is open closes that window. While the rest of startup finishes,
  the bound port answers every request with HTTP 503 and `"status":"starting"`
  rather than leaving it waiting, so nothing hangs and anything polling for the
  server to come up correctly reads it as not up yet.
- **`GET /api/health` names the process answering it.** The reply now carries
  `app`, `pid` and `exe` alongside the port, data dir and version, which is what
  lets a stop act on the server itself rather than on a file that may be wrong.

## [3.7.0] - 2026-08-08

### Fixed
- **"Copy agent brief" now copies.** In the panel shown after a model is added,
  the button reported *Copied* and left the clipboard exactly as it was. Two
  things had to be true at once for it to happen, which is why it worked on one
  machine and not another. A browser hands the clipboard to a page only in a
  secure context - `https` or `localhost` - so reading ModelsDB over plain `http`
  at a machine's network address leaves the page without it and falls back to an
  older way of copying, which works by putting the text in a hidden field and
  selecting it. That field was placed in the page behind the dialog, and a dialog
  makes everything behind it unreachable, so the field could never be selected
  and nothing was copied. The browser then answered that the copy had succeeded,
  which is why the button said so. The hidden field is now placed inside the open
  dialog, and success is judged by whether the browser really copied from it
  rather than by that unreliable answer.

### Added
- **The brief is now on the screen, not only behind the button.** The panel after
  a model is added, and each model's Details dialog, both show the whole brief in
  a read-only box you can read, scroll and select by hand. Copying is still one
  press of the button, and the button copies exactly the text in the box. If a
  copy cannot be made, the panel says so and points at the box instead of only
  flashing *Copy failed*.

## [3.6.1] - 2026-08-03

### Documentation
- **What "Has variations" answers with, once and for all.** Together with any
  setting of the **Hidden** dropdown it draws the models that have builds and
  those models' own builds underneath them, and no other row - not another hidden
  model, and not a build whose model is missing from the catalog. That was
  already how it behaved; nothing said so, and the two controls read as though
  they must fight, since one narrows the list and the other widens it. The
  reasoning now sits beside the code that decides it and in DESIGN.md.

## [3.6.0] - 2026-08-03

### Changed
- **The Hidden dropdown now starts on "Show all hidden".** A fresh install, and
  every press of **Clear**, leaves the table drawing hidden models alongside the
  rest. The reason is variations: one quantized build recorded under a model is
  stored as a hidden row, so a table that began by leaving hidden rows out drew a
  build you had just recorded nowhere at all, and nothing on the page named the
  control that would show it. All three choices are unchanged; only which one the
  control starts on has moved. **Choosing "Hide all hidden" is remembered**, is
  restored on the next visit, and travels in a link, so nobody who wants their
  hidden models out of the way has to choose it twice.
- **Hiding a model now marks it rather than removing it from the list**, since
  hidden rows are drawn by default: the row stays where it is, carrying the
  crossed-out-eye mark that undoes it. The delete dialog and the message after
  hiding say so instead of promising a disappearance.

## [3.5.1] - 2026-08-03

### Fixed
- **The table header no longer covers the top of the first row.** On the desktop
  table the first model was drawn with its top half painted over, so only a slice
  of it could be read. The panel the table sits on scrolls sideways, which makes
  it a scrolling box in both directions, and a header pinned inside such a box is
  measured against the panel rather than against the window: the offset that was
  meant to park the header under the top bar pushed it 56 pixels DOWN from the
  panel's own top edge instead, straight over the first row. It did that at every
  window width, at every height of the filter bar, and it carried the header off
  the screen when the page scrolled rather than pinning it. The header now carries
  no offset at all - it is an ordinary header at the top of the table, so it
  cannot cover a row.

## [3.5.0] - 2026-08-03

### Added
- **A Save path button beside the backup destination**, in the Info dialog's
  *Files & updates* panel. It keeps the folder, checks it can really be written
  to, and says "Saved." - which nothing on the screen did before, so the only way
  to find out whether a typed path had been kept was to close the dialog and open
  it again. Leaving the field still saves quietly, so a path is never lost by
  tabbing away.

### Fixed
- **"Never attempted yet." no longer follows a backup you just took.** Only the
  daily schedule recorded an attempt; the **Back up now** button recorded an
  outcome with no time against it, and the status line reads the time. Every
  route that really tries now records an attempt, so the line reports the backup
  it just wrote. The schedule keeps a reference point of its own, so taking a copy
  by hand never postpones the next automatic one.

## [3.4.0] - 2026-08-03

### Fixed
- **Ticking a column in the Columns panel brings it back at once.** With **Hide
  empty data** on it did nothing until the page was reloaded. A hidden column has
  no cells anywhere, and emptiness was judged from those cells, so the column read
  as empty on every row and was hidden again the instant it was ticked back on.
  Where there is no cell to read, the row as the page loaded it now answers
  instead; a cell still wins wherever there is one, so every edit made since is
  counted.
- **Hiding a priced or context column no longer empties the table.** With **Max
  In $**, **Max Out $** or **Min Context** set, hiding **In $**, **Out $** or
  **Cont** left the filter reading a cell that was no longer there - which came
  back as zero for the minimum and as a huge figure for the maximums, so every row
  failed and the table went blank with nothing saying why. A filter with no cell
  to read now tests nothing.

### Changed
- **A filter is on the bar only while its column is.** The filters that read a
  column's cells - **Min Context**, **Max In $**, **Max Out $** and every
  custom-column filter - appear and disappear with their column, in the filter bar
  and the phone drawer alike, so no control sits there looking live with nothing
  to read. Each keeps its value while its column is away and starts filtering
  again when it comes back. The filters that read no cell (Type, Thinking, Tool,
  MoE, ZDR, Notes, Favorites, HF Link, OpenRouter, Unit, Hidden, Variations, Year,
  Input, Output and the search box) are unaffected.

## [3.3.1] - 2026-08-03

### Changed
- **Columns and Clear now lead the filter bar**, directly after Undo and Redo,
  instead of sitting at its end. Both act on the whole bar - Columns decides what
  there is to filter at all, and Clear undoes every filter below it - and at the
  end of a bar that wraps over several lines, or a scroll down the phone drawer,
  neither could be found when it was wanted. **Colors** and **Custom columns**
  stay where they were, at the end: each opens a panel and changes nothing by
  itself. The drawer's close button is untouched.

## [3.3.0] - 2026-08-03

### Fixed
- **A version filed under a model that is no longer here is reachable again.** A
  row is grouped under another only when that other row is really in the catalog
  and is an ordinary model. One naming a model that is missing - or naming a
  version, which cannot hold versions of its own - used to be in no list at all:
  never a row of the table, and never drawn under anything either, so no filter
  reached it and the note, star and column values on it were stranded. Such a row
  is now drawn as an ordinary row, and the **Hidden** dropdown governs it like any
  other hidden model.

### Changed
- **Has variations now shows the versions themselves, not only the models that
  have them.** Choosing it draws each matching model's versions underneath it
  without also changing the **Hidden** dropdown, and lists a model that is itself
  hidden - which "Hide all hidden" previously dropped, so asking which models have
  versions could answer with an empty table. **Show only hidden** is unchanged.

## [3.2.1] - 2026-08-03

### Fixed
- **The card list no longer sits on a slab.** On a phone the table element still
  carried the desktop panel's background colour, which painted a filled block
  behind and between the cards. The table is now transparent at card widths, so
  every card sits on the page itself. The cards keep their own colours: a plain
  model, a model with variations, and a variation each look exactly as before,
  and the desktop table is unchanged.

## [3.2.0] - 2026-08-03

### Changed
- **The two parameter counts and the weights size are now stored at one decimal
  place.** `Parameters (B)`, `Active parameters (B)` and `Weights size (GB)`
  carried more precision than anybody reads - a repository's own 32.762123264
  billion sat in a cell next to a `27` somebody typed. Every path that writes one
  of them now rounds it as it is stored: a manual save, adding a model, an in-cell
  edit, a HuggingFace import, and the facts a variation inherits from the model it
  was built from. A whole number reads as a whole number (`27`, never `27.0`), and
  a figure below one keeps its decimal (`0.6` stays `0.6`, never `0`).
- **Every value already stored is rewritten to match**, by a schema migration
  (v22) that runs once the first time this version opens a database. It rewrites
  values in place, and a value that is not a number - a note typed into one of
  those columns - is left exactly as it is. As with every migration, a full backup
  of the database is taken before it runs.
- **A build's exact byte count is untouched.** `Build size (bytes)` is a
  measurement the variation importer read off a repository and it stays exact;
  only the GB figure the browser prints from it is shortened, which it already
  was.
- **The update procedure and the API guide say so**, in both the served copy and
  the repository's own, so an agent filling in a model is told what the column
  really holds.

## [3.1.1] - 2026-08-03

### Fixed
- **"Hide empty data" now reaches a variation's card.** A variation is drawn as
  a row attached to the model it belongs to, and the pass that decides what is
  empty never looked at those rows - so on a phone a variation's card kept every
  blank field while every other card dropped them. Its parent's card was always
  right; what looked wrong on the parent was the variation card underneath it.
- **A column only a variation fills in no longer disappears.** With the box
  ticked, a column that every model leaves blank but a variation carries a value
  for was judged empty everywhere and hidden, taking that value off the screen
  with it. Variations are now counted when the question is asked.
- **A variation's cells stay under the right headers.** When a column is hidden -
  by the box above or through **Columns** - the variation's row now drops the
  same column, instead of keeping a cell its neighbours had lost and shifting
  everything after it one column across.

### Changed
- **A card is tinted as one tile.** On a phone, the colour that groups a model
  with its variations was painted on each cell, which drew a patchwork of grey
  rectangles with the card showing through the gaps between them. The whole card
  now carries the colour: a model with variations one shade, its variations
  another, and the accent mark runs down the full height of a variation's card
  instead of stopping beside its first cell.
- **A long value is cut at twenty characters** rather than thirty, and the
  **more** control that opens the rest now sits at the end of the text instead
  of dropping onto a line of its own. On a phone it stays a finger-sized target
  while remaining part of the sentence.

## [3.1.0] - 2026-08-03

### Added
- **"Hide empty data" checkbox**, right after Redo. On the desktop table it
  hides any column with no value in a row currently passing the other
  filters; on a phone it hides just the empty field on each card instead,
  since emptiness is a per-model question there. A column you hid yourself
  through **Columns** stays hidden regardless of this box, and is unaffected
  by it either way.
- **"Has variations" filter**, next to **Hidden**: show only models with (or
  without) at least one recorded variation.

### Changed
- **The filter bar is reordered.** Every fixed filter now comes before the
  custom-column filters (`#cc-filters`), grouped search, then range inputs,
  then categorical dropdowns, then yes/no toggles, then status filters
  (Hidden, Variations) - so a personal column never sits between two of the
  app's own controls. **Hidden** moved out of its old spot after the custom
  columns to join this group.
- A phone's filter drawer no longer breaks a filter's icon and title onto two
  lines.

## [3.0.2] - 2026-08-02

### Fixed
- **A model whose name ends in a space can be hidden, removed and filed as a
  variation again.** OpenRouter publishes a few names with a space on the end -
  "Google: Gemma 4 26B A4B " is one - and the catalog stores every upstream name
  exactly as it arrives, so that space is part of the name the row is keyed on.
  Four requests tidied the name away before looking the row up, and then reported
  that no model had that name: hiding a model, ignoring one, deleting one, and
  filing one under another as a variation. All four now use the name exactly as
  they were given it, so those rows answer like any other. Tidying a name is still
  what happens when a row is CREATED by hand or from a HuggingFace address, which
  is the only place it ever belonged.

## [3.0.1] - 2026-08-02

### Fixed
- **A phone no longer scrolls sideways.** The sort bar above the cards holds a
  field selector as wide as its widest choice, and that list now names every
  column you made yourself - so a name like "Active parameters (B)" pushed the
  sort-direction button clean off the right edge of a 390px or 320px screen and
  gave the whole page a horizontal scrollbar. The bar wraps onto a second line
  instead, every control keeps its full size, and a wider screen still fits it
  all on one.

### Changed
- The design notes and the code comments they came from describe the table as it
  is now, rather than the fixed Params and Size columns that are yours to define.

## [3.0.0] - 2026-08-02

### Changed
- **The model table now ships only the columns ModelsDB maintains itself.**
  Everything else it can tell you about a model is a column YOU own: the author,
  both parameter counts, the size the weights take on disk, a build's bit width
  and format, and every architecture figure a GPU-sizing tool reads. Each of them
  can be hidden, renamed in your head, or deleted outright - and a fact you never
  wanted is a column that never appears. `models` went from 38 columns to 31, and
  `POST /api/save` from 28 writable columns to 24 plus your own.
- **A HuggingFace import asks which facts to keep.** The Add-model dialog and the
  Add-variation dialog both carry a list of everything the repository can tell
  them, with the column each one would fill. Tick what you want; only the ticked
  facts become columns, and the choice is remembered as the default for next time.
  The first time, everything is ticked, so pressing Import without opening the
  list records what was read.
- **A `custom_values` key may be a column's NAME as well as its id**, so a write
  needs no lookup: `{"custom_values": {"Parameters (B)": "32.762123264"}}`. A key
  that reads as a whole number is still an id.
- **`POST /api/custom-columns` on a name that already exists answers with that
  column** instead of failing, so "make sure this column exists" is one call that
  can be repeated.
- The manual-research procedure, served at
  `GET /api/guide/update-database-manually`, is rewritten throughout for the new
  write path, including how to create a column the user has not made yet.

### Added
- `GET /api/import-fields` lists every fact a HuggingFace import can read, the
  custom column each one fills, and whether it starts ticked.
- The import's 201 answer now carries `read` (everything the repository states)
  and `kept` (which of those became columns), which are no longer the same thing.

### Removed
- `models.author`, `models.parameters`, `models.active_parameters`,
  `models.disk_size_gb`, `models.bit` and `models.format`, each carried into the
  custom column of the same meaning first.
- `models.model_details`. Every fact that JSON blob held is a column of its own
  now, so each can be sorted by, filtered on and read in a cell - which a figure
  inside a blob never could. The keys that described the READING rather than the
  model are gone: an undeterminable figure is an empty cell.
- The fixed **P**, **AP**, **Size (GB)**, **Version**, **Format** and **Bits**
  table columns, and the Min Params / Min Active / Max Size filters. The same
  figures are number columns you own, which sort as quantities and filter by a
  minimum through the per-column controls.

### Migrations
- **v14 to v19** move one column each into a custom column: read every value,
  create the column only when some row has one, write the values, COUNT them back,
  and only then drop the old column. A count short of what left rolls the whole
  migration back and restores the pre-migration backup.
- **v20** takes `model_details` apart the same way, one custom column per fact.
- **v21** adds `app_settings`, a server-owned key/value table holding which facts
  an import keeps.
- Rehearsed against a copy of a real 569-model install: `PRAGMA integrity_check`
  ok, `user_version` 21, models 569, favourites 12, notes 167 all unchanged, and
  the stored custom values 21 -> 750 as the 729 carried values arrived. A
  parameter count came through digit for digit (`32.762123264`).

## [2.15.0] - 2026-08-02

### Added
- **Importing a model from HuggingFace now records the architecture a GPU-sizing
  tool cannot work without.** Deciding whether a model fits on a machine takes two
  numbers that are not the parameter count: what the weights take, which the
  on-disk size already gave, and what each token of CONTEXT adds, which nothing in
  this catalog held. The import now reads the repository's own `config.json` and
  records the per-token key-value cache cost, the attention layer count, key-value
  head count and head dimension it is computed from, a Mamba-style model's
  recurrent layer count and per-layer state (which does not grow with the
  context), how wide one weight is, how it was quantized, the exact byte count and
  the format. They ride in `model_details`, which `GET /api/models` returns as a
  real object, and the same object now rides on a recorded variation, so one
  question is answered in one shape either way.
- Every figure carries the formula or the config field it came from, and a figure
  that could not be derived is written down as a named reason rather than as a
  zero somebody would read as a measurement. The field names are HuggingFace's
  own, each cited against the transformers documentation that defines it.

## [2.14.0] - 2026-08-02

### Added
- **A model already in your catalog can be filed under another one as a
  variation.** Until now a variation could only be created by reading a
  HuggingFace repository, so a version somebody had already added as a row of its
  own - by hand, or from its HuggingFace address - could not be put where it
  belongs. The **Add a variation** dialog now has a second half: type part of a
  model's name, pick it, and it is filed under this model. Nothing is downloaded
  and nothing is looked up, so nothing about the row changes except that one
  fact - the note, the favourite star, every column value and every figure it
  holds ride along with it. It becomes hidden, like every variation, and the
  dialog says so and names the way to see it again.
  `POST /api/models/variations/existing` is the same thing over HTTP.
- Four relations are refused, each of which would leave the table unable to draw
  itself: a model under itself, under one of its own variations, under a row that
  is already a variation, and a model that has variations of its own. A row filed
  under the wrong model is corrected by filing it under the right one.

### Changed
- **The Var column's control is now on every row except a variation.** It used to
  appear only on a model with a HuggingFace repository, because looking up
  published versions was all it did. It can now also file a model you already
  have, which works on any row - and a model with no repository opens the dialog
  on a line saying there is nothing to look up on HuggingFace and pointing at the
  half that does work.

## [2.13.1] - 2026-08-02

### Fixed
- **A card no longer hides the columns a model has no value for.** Every cell with
  nothing in it was dropped from the mobile card, so a model with no parameter
  count, no recorded size or no price simply had no Params, Size, In or Out slot -
  and with no header row on a card, the caption is the only thing that names the
  column, so the card said nothing at all where the desktop table shows an empty
  cell under a header that is still there. A captioned cell now keeps its place
  and shows a faint em-dash. Only the cells that carry no caption in card view
  still collapse: the Var control on a model with nothing to open, and the three
  variation columns, which are blank on every ordinary row.

## [2.13.0] - 2026-08-02

### Added
- **A long note or personal-column value is drawn short, with the whole of it one
  press away.** Past thirty characters a cell now paints the first thirty, an
  ellipsis, and a **more** control; pressing it opens the whole value in place,
  line breaks and all, and pressing it again closes it. A value of thirty
  characters or fewer gets no control at all. The whole value never leaves the
  cell - it rides in `data-full` - so the search, every filter, the sort and the
  save all still read all of it, exactly as a Context cell renders `128k` while
  sorting on `128000`. The control is keyboard-operable, says which state the
  cell is in, and takes a finger-sized tap target on a card.
- **A text personal column is written in a textarea, in the cell and in the
  form.** It holds whatever a person writes, including several lines, and a
  single-line input cannot be given a line break at all. Enter commits and
  Shift+Enter starts a new line, exactly as the personal note already answered
  the keyboard.

## [2.12.0] - 2026-08-02

### Added
- **A personal column can now hold a number, and the table sorts it as one.** The
  custom-column system offered a text field and two kinds of single-select
  dropdown; the fourth kind, **Number**, holds one figure you type into the cell.
  It is a different thing from a dropdown of number labels, and the difference is
  what each sorts by: a number column sorts as a quantity, so 9 comes below 10
  rather than after it, while a dropdown sorts by its label's position in the list
  you wrote. A number column's filter is a **minimum**, matching Min Params, since
  a substring match on digits answers nothing anybody asks of a quantity. Anything
  that does not read as a number is refused rather than stored, and the digits are
  kept exactly as sent, so a researched figure never loses precision. Schema v13
  widens the allowed type names; the migration carries every dropdown option and
  every model's stored value across the table rebuild that needs.

### Changed
- **A custom-column value the column will not hold now answers 400, not 500.** A
  label outside a dropdown's option list, or a word sent to a number column, is
  the caller's to correct, exactly like every other refusal `POST /api/save` and
  `POST /api/models/add` make - and a 500 said the server had broken instead.
  The reply names the column and why. Nothing is stored either way.

## [2.11.2] - 2026-08-02

### Fixed
- **The table now switches to the card layout at the width where it actually stops
  fitting.** The single breakpoint that decides table vs cards (`style.css`'s
  `@media` queries and `app.js`'s `cardsQuery`) was calibrated against the table's
  minimum content width, but that width grew when the Version, Format and Bits
  columns became part of the table, and the breakpoint itself was never
  recalculated. The result was a common desktop window width (roughly
  1300-1600px) where the app rendered the desktop table but the table did not fit,
  forcing it to scroll sideways to see the rightmost columns - a cramped,
  cut-off experience that read as broken. The breakpoint is now measured against
  the table's current minimum content width (~1536px) with the same margin the
  original number kept, raising it from 1299px to 1642px, so every width above
  the breakpoint shows the full table with no forced scrolling and every width
  at or below it cleanly gets the card layout instead.

## [2.11.1] - 2026-08-02

### Fixed
- **The table no longer pushes the page wider than the viewport.** `overflow-x:auto`
  on the DataTables wrapper gives it a horizontal scrollbar when columns exceed the
  panel width, covering the gap above the card breakpoint where `table-layout:auto`
  lets columns grow past the viewport on a narrow desktop or tablet window.

## [2.11.0] - 2026-08-02

### Changed
- **A variation is now a row of the table, not a panel under one.** It was drawn
  as a list inside a box, with its own headings, its own controls and a paragraph
  explaining what it was - none of which a variation needs, because a variation
  is a model row that names a parent. It now carries the same cells and the same
  row controls as every other model, and two things mark it out: a tint, and
  sitting directly under the model it was built from. The model above it is
  tinted too, so a group reads as one block.
- **Three columns describe it**: **Version** (the quantization label), **Format**
  (GGUF, AWQ) and **Bits**. They are ordinary columns, filled from what the
  importer measured off the repository, and blank on a model that is not a
  variation.
- **It stays under its parent through every sort and filter.** Sorting by price
  moves the model and takes its variations with it; a filter that removes the
  model removes them too.
- The **Var** column keeps the control that adds a variation, and a count of the
  ones recorded. The arrow that opened the old list is gone - there is nothing to
  open when the rows are already there - and with it the panel's order control,
  its per-row copies of the remove and serve controls, and the remembered
  open/closed state (`variants_open` in the settings file).

### Note
- A variation still carries the `hidden` flag it is recorded with, so it appears
  only while the **Hidden** dropdown is drawing hidden rows, and `GET /api/models`
  still withholds it unless the caller asks. **Show only hidden** draws no
  variations at all: a variation is drawn beneath its parent, and that parent is
  not itself hidden, so it is filtered out and its variations go with it.

## [2.10.1] - 2026-08-02

### Documentation
- **The research procedure now covers a model that states no input or output
  modalities.** A blank modality list makes a model invisible to every filter for
  what it takes in or gives out, while the row still appears everywhere else, so
  nobody notices. The procedure gains a query that finds them, the reason they
  arrive that way (`collectionDefs` deliberately states no INPUT for the image
  and video collections, because a collection page does not say whether a model
  takes a prompt, an image or both), and where to read the real answer: the
  OpenRouter API first, then the model's own OpenRouter page, then the vendor's
  documentation - never a guess from the model's type.
- **It also says to address a model by NAME, never by `id`.** Two rows can hold
  the same `id` - one the collection scrape added, one the API refresh added
  under a different display name - so a write keyed on `id` picks one of them
  silently, and it may not be the row that needed writing.

## [2.10.0] - 2026-08-02

### Added
- **A Type filter, for the question the modality filters cannot answer.** Music
  generation, speech synthesis and transcription all deal in audio and carry
  identical modality lists, so asking the Output filter for `audio` returns every
  music AND speech model with no way to separate them - on a live catalog, all 36
  audio-output models. The filter bar now has a **Type** select offering the eight
  kinds a model can be: chat, image, video, embedding, rerank, tts, stt, music.
  It goes into the address and the saved view like every other filter, and Clear
  resets it.
- **`embedding` and `ranking` in the Output filter.** Both are values the catalog
  already stores and produces - 21 embedding models and 6 rerank models here -
  and neither could be asked for, because the dropdown offered only text, image,
  audio, video and file.

## [2.9.1] - 2026-08-01

### Added
- **Both steps of the add-a-variation picker can now be ordered.** 2.9.0 put an
  order control on the list of variations already recorded, which is not where
  the choosing happens. The picker itself had none, and its first step needs one
  most: it can list well over a hundred repositories - 127 for one model here -
  in whatever order HuggingFace returned them, which is no help when the question
  is which publisher to trust.

  Repositories now offer most downloaded (the new default), most liked, name A to
  Z, and "as HuggingFace listed them" for the order the step used to show. Files
  within a repository offer size and bit width either way round, starting at
  smallest first, which is the order the server already returned them in.

  A vision projector still sorts last in every order, because it is loaded
  alongside a build rather than being one. Each control stays hidden until its
  list holds more than one entry, and re-ordering redraws from what was already
  fetched rather than asking HuggingFace again.

## [2.9.0] - 2026-08-01

### Added
- **The variations list can be ordered, and the size cell hovers to the exact
  byte count.** A model's recorded variations were drawn in whatever order they
  arrived in, which is no help when the question you opened the list to answer is
  which of them fits the machine you have. The list's header line now carries an
  **Order** control - size or bit width, either way round - starting on size,
  smallest first. Hovering a size shows the figure it was read at
  (`216,715,360,960 bytes, read from the repository`) rather than the rounded GB
  in the cell; that number was already stored and simply had nowhere to appear.

  The order applies to every open list at once and is not saved between visits: a
  list is opened to answer a question and closed again, so the state worth coming
  back to is the default. A variation with no recorded size or bit width sorts
  last whichever direction is chosen, because "not known" is not a small number
  and would otherwise lead the smallest-first list. The control is left out
  entirely on a model with a single variation, where ordering can change nothing.

## [2.8.2] - 2026-08-01

### Documentation
- **One name for a quantized copy of a model: a variation.** The same thing was
  called three things - "downloadable version", "downloadable build" and
  "variation" - across 23 files, and the two prose names disagreed with the code,
  which has always said variation: `POST /api/models/variations`,
  `store.ImportVariation`, the `variant_of` column, the **Var** table column.
  "Downloadable" also picked out nothing, since every HuggingFace model is
  downloadable; what makes these rows different is being one quantized build OF
  another model. Renamed throughout the README, ARCHITECTURE, DESIGN, the `GET
  /api` guide, the served research procedure and its skill mirror, code comments
  and the one tooltip that said it on screen. No identifier, column, endpoint or
  stored value changed, so nothing an API caller depends on moved.

## [2.8.1] - 2026-08-01

### Changed
- **Clear now resets the Hidden dropdown too**, back to "Hide all hidden", along
  with every other control in the filter bar. It was being skipped on the
  grounds that it says what the table is a list OF rather than narrowing that
  list - which is a distinction the button does not make on screen: pressing
  Clear and finding the table still drawing only hidden models reads as Clear
  not having worked.

## [2.8.0] - 2026-08-01

### Changed
- **The "Show hidden" checkbox is now a "Hidden" dropdown with three choices.**
  A checkbox could only fold the hidden models back in among everything else,
  which meant the models you had put aside could never be looked at as a list of
  their own - the count in the label was the only evidence they still existed.
  The dropdown offers **Hide all hidden** (the default, and what the unticked box
  did), **Show all hidden** (what the ticked box did) and **Show only hidden**,
  which draws nothing else. Clear still leaves the control alone, the count is
  still in the label, and the address still carries the choice so a link
  reproduces the view.

  A saved setting or a shared link written by an earlier version carried a
  true/false there and is not one of the three values, so it opens at **Hide all
  hidden**; setting the dropdown once saves the new value. Note that hidden is
  not the same as ignored: a hidden model keeps its row and everything you wrote
  about it, while an ignored model has no row at all.

## [2.7.4] - 2026-08-01

### Fixed
- **The line telling you what just happened is no longer silent to a screen
  reader.** Everything the app says back after an action - that a save worked,
  which phase a refresh is on, why something failed - appeared in one fixed strip
  of coloured text that nothing announced. A screen reader reads a change only
  inside a live region, and the page had none for it, so all of that feedback
  reached sighted users only. The toast now has a voice: two hidden regions
  beside it, polite for progress and success, assertive for a failure, which has
  to interrupt where a progress note must not.
- **A failing save says it once instead of two dozen times.** A save against an
  unreachable server retries twelve times, and announcing each attempt would be
  24 interruptions reporting that nothing has been decided yet. The screen still
  counts every attempt; the spoken version says saving is not going through, then
  stays quiet until it succeeds or gives up. Repeated progress text is never
  re-announced for the same reason, while a repeated OUTCOME is - saving two
  cells says "Saved!" twice, because it happened twice.

## [2.7.3] - 2026-08-01

### Fixed
- **A custom column's value can no longer be erased by a request that meant to
  set it.** A custom column stores TEXT, whatever it shows on screen: the
  "Rating" dropdown's options are the labels "1" to "4", not the numbers. Sending
  the number - `{"custom_values":{"7":4}}` - did not fail. The value was read as
  an empty string, an empty string is how a custom value is CLEARED, and the
  request answered 200 having deleted the rating. Custom-column values are
  personal, they are in `modelsdb.db` and no other file, and nothing upstream
  re-creates them. `POST /api/save` and `POST /api/models/add` now answer 400 and
  write nothing at all - not the custom value, and not the other fields in the
  same object - naming the column and saying to send `"4"` rather than `4`.
  Clearing a value on purpose is unchanged: an empty string and `null` both still
  do it. This is the check 2.2.0 added for the columns on the models table,
  which custom values were outside of because they are written to their own
  table rather than through `store.UpdateCurated`.

## [2.7.2] - 2026-08-01

### Documentation
- **Contributing says that the page is not covered by any test.** The Go tests
  cover the server, and a change to `internal/app/index.html` or
  `internal/app/static/` is checked by nobody unless a person opens a browser
  and looks - this project ships no JavaScript test runner. A contributor had no
  way to know that from the instructions, which named `go build ./...` and
  `go test ./...` and stopped there. Contributing now asks for a browser check
  of any page change, and for the pull request to say what was clicked through.

## [2.7.1] - 2026-08-01

### Fixed
- **A note is displayed the way it was written.** A note typed across several
  lines read back as one run-on paragraph, in the Name cell and on the mobile
  card alike. The line breaks were never lost - the database keeps them,
  `GET /api/models` returns them and the page holds them - but HTML turns a line
  break in a block of text into a single space unless it is told otherwise, and
  the note was not. It now keeps every break and every run of spaces, and still
  wraps a line too long for the column. An empty note still collapses its line,
  and editing one keeps every break through the round trip.
- **A text-type custom column is displayed the same way.** It shares the note's
  display element, so a value that reaches it carrying line breaks now shows
  them rather than flattening them.

## [2.7.0] - 2026-08-01

### Fixed
- **Parameter counts keep their decimals.** `parameters` and `active_parameters`
  count BILLIONS, and that figure is often not a whole number - a 30B-A3B
  Mixture-of-Experts model has 3.3 billion active parameters, `Qwen/Qwen3-32B`
  counts 32.762123264 billion off its own repository, and a small embedding
  model is under one billion. Every one of those was rounded down on the way in
  - 3.3 to 3, and 0.6 to **0** - while the write answered `{"ok":true}`. Both
  columns are now `REAL`, like `disk_size_gb`, and every writer binds them as
  decimals: the OpenRouter refresh, `POST /api/save`, `POST /api/models/add`,
  the HuggingFace import, the downloadable-version importer and the in-cell
  editor. What you send is what is stored.
- **The table, the sort and the two parameter filters agree about a decimal.**
  A cell prints as few digits as the value needs - `7`, `32.8`, `0.6`, `0.023` -
  while sorting, the min-params filter and the min-active filter read the exact
  stored figure, so a row showing `32.8` still filters as 32.762123264. The
  mobile card's combined `398B / 3.3B` suffix is written the same way.
- **A small model imported from HuggingFace now gets its count.** The importer
  discarded any figure below one billion, because a whole-number column could
  only store it as 0. It discards a count for one reason now: a repository whose
  `safetensors` block counts packed low-bit elements, where the total is not a
  parameter count at all.
- **`tool` no longer claims "no" where nobody has answered.** A model added by
  hand, imported from HuggingFace or added by the collection scrape was stored
  and served as "no tool support" - a finding nobody had made, on the very rows
  whose import answer names `tool` among what is still to research. That column
  now reads empty on those rows, in the database and in `GET /api/models`, and
  empty means unestablished. Only the OpenRouter catalog fetch fills it, from
  what the API reports about the model's endpoint. Nothing that reads the field
  changes behaviour: everything tests it for "yes".
- **The served guide describes `POST /api/models/ignore` correctly.** It said a
  row nothing re-creates answers 400; the refusal is narrower than that - it is
  for a row offering NO KEY AT ALL, so a `source=huggingface` row carries its
  repository slug and succeeds.
- **The served guide describes a hand-added row correctly.** It said no refresh
  phase reaches such a row by identifier. The collection scrape does, by an `id`
  or `slug` written onto the row, and enriches it - it adds to the modality and
  collection lists and replaces `model_type` only while it reads empty or
  "chat", so nothing written there is taken away, which is what the guide now
  says.

### Changed
- **Schema v12** converts `parameters` and `active_parameters` from `INTEGER` to
  `REAL`, carrying every stored value across unchanged and leaving an empty
  column empty. It runs inside the same transactional, integrity-checked,
  auto-backed-up path as every other schema change. A count that was already
  rounded down stays rounded down - a migration carries what is stored, and
  cannot recover digits discarded before the write; re-importing the model from
  its HuggingFace address is what restores the real figure.

## [2.6.0] - 2026-08-01

### Added
- **An "Add model" control in the header**, opening one dialog with two paths: a
  HuggingFace address the server reads for itself, or a custom model typed field
  by field. The dialog swaps between choosing, filling in and reporting the
  result in place, and the new row is added to the table without reloading the
  page - so the scroll position, any open editor and the current page survive it.
  When a filter is hiding the new row, the dialog says so and names **Clear**.
- **A "Copy agent brief" button**, on the import result and in each model's
  Details dialog. It copies a short set of instructions naming one model, listing
  the fields that are empty, and giving the address of this instance so an AI
  assistant can read the research procedure, look the facts up, and post them
  back. The address comes from the page's own origin, so it always carries the
  port this server really runs on. Anyone not using an assistant can ignore it -
  the same fields are typed into the row by hand.
- **In-cell editors for the nine columns that have a cell**: context length, both
  parameter counts, size on disk, both prices, the date, and the two modality
  lists. Click the value, or press Enter on it, and it becomes a control; blur or
  Enter commits, Escape cancels, and `Ctrl`+`Z` undoes it. Prices are typed in
  the unit the column shows and stored as the per-unit figure, in plain digits.
  The modality lists open a panel of checkboxes built from the app's own
  vocabulary plus whatever unusual value that row already holds.
- **A pencil beside each model's name**, opening a form over all 28 fields a
  person or an agent may fill - including the sixteen that have no column
  anywhere: the author, the type, tool use, mixture of experts, the pricing unit,
  three more prices, the price page, the collections and the identifiers. It
  reads the row fresh from the server on open, sends only what changed, and
  redraws the row from the server's answer afterwards.
- **`refresh_owned` on every model in `GET /api/models`**: the columns a refresh
  phase would overwrite on THAT model, each mapped to a sentence naming the phase.
  It is computed by the same function `POST /api/save` builds its 400 from, so the
  fields the page shows disabled and the writes the server refuses cannot drift
  apart. `TestRefreshOwnedNamesExactlyWhatTheSaveRefuses` pins that both ways.
- **`author` in `GET /api/models`.** It was writable but never returned, so the
  page could not show what it held.

### Fixed
- **A stored value carrying a double quote no longer breaks out of the
  `data-dt-order` attribute** it is written into. A text-type custom column feeds
  that attribute its raw text, so a value like `say "boom"` ended the attribute
  early and put the rest of it into the row's markup.

## [2.5.0] - 2026-07-31

### Added
- **`GET /api/guide/update-database-manually` serves the manual-research
  procedure** - how the data the OpenRouter API never returns (parameter counts,
  the prices of the models it does not price, the models that exist only on
  HuggingFace) is looked up and written into this catalog by hand. It is the
  same document the project keeps as a Claude Code skill, served so an agent on
  another machine can read the steps it is being asked to follow. It is a route
  of its own rather than part of `GET /api`, because the text is around 60 KB,
  twice the API guide.
- **A preamble the app writes, ahead of that text**, mapping step by step what a
  reader with only HTTP can do and what needs a shell on the machine the server
  runs on. Since 2.4.0 nearly all of it is HTTP work, because `POST /api/save`
  takes 28 of a row's 38 columns. **One research step still needs a shell:**
  setting `price_prompt`, `price_completion`, `price_image`, `price_display` or
  `pricing_url` on a `collection` model that carries an `id`, which the API
  refuses because the pricing scrape assigns all five there.
- **A test holds the served copy byte-identical to the skill file.**
  `TestServedProcedureMatchesTheSkillFile` compares
  `internal/app/update-database-manually.md` to
  `.claude/skills/update-database-manually/SKILL.md` and names the `cp` that
  fixes a difference. Two copies are unavoidable - `//go:embed` cannot reach
  outside its own package directory - so the build goes red instead of the two
  drifting apart.

### Changed
- **The skill gained a section on custom rows and what to do when one looks like
  a model already in the catalog.** A hand-added row is legitimate and is never
  force-merged: an agent that spots a probable duplicate says what it found,
  recommends one action, and leaves the decision to the person, because nothing
  in this app merges two rows and removing either one destroys its notes, its
  favorite flag and its custom-column values. The only collision the database
  refuses is two models sharing a display name; two rows differing only by CASE
  are legal and will both exist, so spotting that they are the same model is the
  agent's job and the section says so.
- **The skill now states, per procedure, whether it needs a shell or works over
  HTTP.** Steps it still showed as `sqlite3` work that 2.4.0 made possible over
  HTTP now name the endpoint: the Phase-2 worklist and every other query
  (`GET /api/models?all=1&include_hidden=1`), finding a hidden model
  (`include_hidden=1`), reading and clearing the ignore list (`GET` and
  `DELETE /api/ignored`), reading the personal column definitions
  (`GET /api/custom-columns`), taking a backup (`POST /api/backup/run`), and
  running a refresh (`POST /api/update`).

## [2.4.0] - 2026-07-31

### Added
- **`POST /api/save` and `POST /api/models/add` now accept every column a person
  or an agent may fill - 28 of the 38 a model row carries.** Nineteen columns
  join the nine they already took: `id`, `slug`, `hf_slug`, `author`,
  `description`, `created_at`, `model_type`, `context_length`,
  `input_modalities`, `output_modalities`, `collections`, `supports_reasoning`,
  `price_prompt`, `price_completion`, `price_image`, `price_audio`,
  `price_internal_reasoning`, `price_display` and `pricing_url`. An agent that
  researched a model can now post back everything it found, and a model added by
  hand can be filled in completely in the request that creates it.
- **Which of the 28 a given model takes depends on that model.** The rule is per
  column AND per source, because the refresh phases write different columns on
  different rows. On a model whose source is `openrouter` the twelve columns the
  OpenRouter catalog fetch assigns outright are refused; on a `collection` model
  carrying an id the five the pricing scrape re-reads are refused; on a model
  imported from HuggingFace or added by hand, all 28 are writable. A refusal is a
  **400 that names both the column and the phase that owns it**, and writes
  nothing.
- A list column (`input_modalities`, `output_modalities`, `collections`) takes an
  array of quoted words, `["text","image"]`, and also takes one comma-separated
  string, `"text, image"` - the form `GET /api/models` returns the modality lists
  in, so a model can be read, changed and sent back. `[]` and `null` both empty
  one.

### Changed
- **A key these endpoints do not write is now REFUSED rather than ignored.** The
  ten columns nothing writes by hand (`name`, `source`, `zdr`, `hidden`,
  `variant_of`, `bit`, `format`, `model_details`, `raw_json`, `updated_at`), a
  price sent under the name a read returns it under (`prompt` rather than
  `price_prompt`), and a misspelled column each answer **400** naming what to
  correct. Silently dropping them was the same failure as a discarded value, in a
  quieter form.
- A save is a partial write **by construction**: the `UPDATE` statement is built
  from the keys the request carries, so a key left out is not in the statement at
  all. The read-modify-write merge that achieved that before is gone, along with
  `store.SaveCurated` - `store.UpdateCurated` is the one funnel every writer ends
  in.
- **`store.ImportFullRecord` can no longer replace a stored value with an empty
  one.** Its conflict branch previously cleared `hf_slug`, `author`,
  `context_length` and `supports_reasoning`, and that branch is reached when a
  collection scrape lands on an existing row by display name - so a model added
  by hand could lose four researched values to a scrape that knows nothing about
  them. All four now carry the guard the price columns already had.

### Documentation
- The served guide (`GET /api`) rewrites "The writable fields" as the full
  28-column table with the phase that overwrites each one, and replaces "What you
  cannot write here" with "Some columns belong to a refresh phase" and "The ten
  columns nothing writes here".
- `README.md` gains "Which columns you can write, and on which models".
- `ARCHITECTURE.md` gains "Which columns a write may fill".
- The `update-database-manually` skill now writes through HTTP wherever HTTP
  accepts the column, and keeps `sqlite3` only for the one case that still needs
  it: hand-pricing a `collection` model, whose five price columns the pricing
  scrape owns.

## [2.3.0] - 2026-07-31

### Added
- **`POST /api/models/import` adds a model from a HuggingFace address, and the
  server reads every value it stores.** The body carries the address and nothing
  else - `{"url":"https://huggingface.co/Qwen/Qwen3-32B"}` - so no measurement in
  the created row is somebody's recollection. The full page URL, that URL with a
  trailing `/tree/main` or a query string, the address without its scheme, and
  the bare `Qwen/Qwen3-32B` are all accepted.
- It fills `name` (built from the repository id as `Owner: Model`), `hf_slug`,
  `author`, `description`, `created_at`, `context_length`, `model_type`, both
  modality lists, `moe`, `parameters` and `disk_size_gb`, and answers **201**
  with a `missing` list naming the fields the repository does not state - which
  is what is left to research and write back with `POST /api/save`.
- **`disk_size_gb` is the weight files the shard index names, never the sum of
  every weight file in the repository.** A repository ships copies of its own
  weights, so summing them all gives two or three times the real figure while
  looking entirely plausible in a column this app presents as a measurement:
  `mistralai/Mistral-7B-Instruct-v0.3` sums to 28,992,159,440 bytes against the
  index's 14,496,080,928, and `openai/gpt-oss-20b` to 41,273,504,288 against
  13,761,316,904. A repository whose index cannot be read leaves the size unset
  rather than guessed.
- `parameters` is read from the repository's `safetensors` block only when every
  storage type in it is full precision. A packed low-bit repository puts several
  weights into one stored element, so its count is not a parameter count; a count
  below one billion is left unset too, since the column holds whole billions.
- Reading a repository as a model is `internal/catalog/hfmodel.go`, beside the
  `huggingface.go` that reads one for its downloadable builds.

### Changed
- A name created by `POST /api/models/add` or `POST /api/models/import` is
  trimmed and has runs of whitespace collapsed to one space. No name already
  stored is ever rewritten: renaming a row would return its notes, favorite flag
  and custom-column values empty and silently, so there is no rename anywhere in
  this app.
- The refusals match the contract the other write endpoints already keep: a name
  already in the catalog and a repository on the ignore list both answer **409**
  with nothing written, an address the route cannot use answers **400**, a
  **gated** repository answers **400** exactly as `POST /api/models/variations`
  refuses one, and an unreachable HuggingFace answers **502**.
- The served `GET /api` guide, `README.md`, `ARCHITECTURE.md` and the
  `update-database-manually` skill all document the new route, including which
  fields it fills and the two it deliberately leaves empty.

## [2.2.0] - 2026-07-31

### Fixed
- **A field sent as the wrong kind of value is refused instead of emptying the
  column.** `POST /api/save` and `POST /api/models/add` now answer **400** and
  write nothing at all for a value the column cannot store - a count sent as
  `"27"` rather than `27`, a number where text belongs, a word like `"yes"` for
  the `favorite` flag. The message names the field and the type it takes, so it
  can be corrected and sent again.
- **This is data loss on a save, which is why it matters more there than on an
  add.** A save is an overlay onto what the row already holds, and a value the
  statement could not read arrived at the column as empty - so
  `{"name":"X","parameters":"27"}` answered 200 while CLEARING a parameter count
  that had been looked up by hand and is held in this database and nowhere else.
  On an add the row is new, so the same value cost nothing.
- A refused add creates no row. The row and its curated values are two
  statements, so the types are read before the first of them rather than between
  them.

### Changed
- The check lives in `store.UpdateCurated`, which `store.AddModel` and
  `store.SaveCurated` both end in, so both writers are covered by construction
  and any writer added later inherits it. It answers a sentinel of its own,
  `store.ErrFieldType`, alongside `ErrNoSuchModel` and `ErrModelExists`, which is
  what lets both handlers answer 400 rather than 500.
- **Emptying a field on purpose is unchanged**, and is what an over-strict check
  would have broken: `null` empties any field, `""` empties `notes`,
  `measurement` and `pricing_note`, and `""` is still IGNORED by `tool` and
  `moe`. The `favorite` flag still reads a number (0 or 1) and the four texts
  `"true"`, `"1"`, `"false"`, `"0"`; any other value is now refused rather than
  read as false.
- The served `GET /api` guide gains a section on the refusal and no longer
  teaches the silent clearing as a rule to work around. `README.md`,
  `ARCHITECTURE.md` and the `update-database-manually` skill say the same.
- Nothing in the browser changed: the page only ever sends the flag as a number
  and its three text fields as text, so no edit it makes can meet the refusal.

## [2.1.0] - 2026-07-31

### Added
- **`music` is a model type of its own, alongside `chat`, `image`, `video`,
  `embedding`, `rerank`, `tts` and `stt`.** A music generator was previously
  recorded as `tts`, which says speech.
- **The app now tells a music model from a speech model, which nothing upstream
  does.** The two are the same shape in the data - both take text and answer
  with audio, with identical modality lists - so the type is decided by what the
  model's own description says it makes: a description naming music generation
  (`music generation`, `text-to-music`, `generate music`, `song generation`, and
  a few more) gives the row `music`, and everything else that answers with audio
  stays `tts`. The test is deliberately narrow: it looks for the act of making
  music, not for the word, so a speech model that mentions music in passing is
  not re-labelled. It can only ever apply to a model that outputs audio.

### Changed
- **Schema v11 corrects the rows already stored under the wrong type.** A
  refresh cannot reach them: an upsert writes an incoming type only over an
  empty or `chat` one, which is what protects a curated type, and a row already
  reading `tts` is neither - so it would have kept that value forever. The
  migration selects the rows typed `tts`, applies the same description test, and
  sets `music` on the ones that match. It writes that one column, leaves every
  other row and column alone, and is re-runnable.
- The served `GET /api` guide, `README.md`, `ARCHITECTURE.md` and the
  `update-database-manually` skill all name the new value and say what separates
  it from `tts`.

## [2.0.1] - 2026-07-31

### Fixed
- **A save the server refuses stops at the first answer instead of being sent
  twelve times.** The page treated every non-2xx answer as a passing glitch, so
  a save for a model the database does not hold - the row was ignored or deleted
  in another window, and this tab's table was loaded before that - was retried
  for about 24 seconds and then blocked the page with a `CRITICAL ERROR` dialog.
  An answer of `400`, `404`, `405` or `409` now ends the attempt at once, and the
  server's own sentence (*No model named "X"...*) goes in the status line. There
  is no dialog on that path, and the connection indicator stays `online`, because
  the server answered.
- A refused edit is dropped rather than retried, and the status line says so:
  *"It is not being retried, so what you see here may not be what the database
  holds - reload the page to see what is stored"*. The batch is written to the
  browser console.
- A request that got no answer, and a `5xx`, still retry as before: 12 attempts,
  2 seconds apart. The code comment and `DESIGN.md` both said that budget was a
  minute; it is about 24 seconds, and both now say so.

## [2.0.0] - 2026-07-31

### Added
- **A model can be added by hand, over the API: `POST /api/models/add`.** Send one
  object naming the model - `{"name":"My Local Build","notes":"...","parameters":27}` -
  and it is created, answered with `201`. It takes the same fields a save takes,
  and nothing else: identity, capability and pricing columns belong to the
  refresh, `hidden` belongs to the hide endpoint, and the four columns of a
  downloadable build belong to the build importer. The row it writes is
  `source: manual`, so nothing upstream re-creates it.
- Adding refuses a name that is taken and a name on your ignore list, both with
  `409` and with nothing written, so a model you withheld cannot be brought back
  through it either.

### Changed
- **BREAKING: `POST /api/save` no longer creates models.** A save naming a model
  this database has never seen used to insert a row for it. It now answers `404`
  and writes nothing, so a typo in a name fails instead of quietly adding a
  model nobody meant to add, carrying whatever was sent. Anything that relied on
  a save to create a model must call `POST /api/models/add` instead. Nothing in
  the app relied on it: the browser only ever saves rows already in the table.
- The served API guide (`GET /api`) documents the new endpoint, and its
  "an unknown model name creates a model" warning is gone, because that is no
  longer what happens.

## [1.43.0] - 2026-07-30

### Changed
- **A model that stops being sold at OpenRouter keeps its row, and loses only
  its price.** When OpenRouter quotes `-1` for a model - its way of saying the
  model is no longer servable there - the refresh used to DELETE that model's
  row. Anything you had written about it went too: the note, the favourite and
  every custom-column value, because those rows cascade away with the model and
  exist in no other file. The row is now kept and its price columns are emptied,
  which is what `-1` actually means. No row is deleted for want of a price, ever.
- The refresh summary no longer has a "not stored" line for those models, since
  they are stored.

## [1.42.2] - 2026-07-30

### Fixed
- **Pressing Clear made the page flicker for several seconds.** Clearing the
  filter bar empties fifteen dropdowns and every box, and each one was setting
  off its own full redraw of the table, its own save and its own address
  rewrite - 19 redraws, 38 saves and 471 rebuilds of the table body for one
  click, which is why the pointer kept flickering between an arrow and a hand
  over rows that were being destroyed and rebuilt underneath it. Clear now does
  that work once, for the whole clear: one redraw, one save, one address
  rewrite.

## [1.42.1] - 2026-07-30

### Fixed
- **Clearing every filter left a long address behind.** The column layout was
  being written into the address alongside the filters, and a layout is never
  unset - there is always some column order - so a table whose columns had been
  dragged, or which had gained a column in a later build, kept a `col_order`
  parameter no matter how empty the filter bar was. Pressing **Clear** now
  empties the address completely.
- The column layout is no longer carried in a link at all. Which columns you show
  and the order you dragged them into belong to your own computer, not to the
  rows a link points at, so they are read from your saved settings whether the
  address carries filters or not. Opening somebody's link keeps YOUR columns and
  shows THEIR filters.

## [1.42.0] - 2026-07-30

### Changed
- **The address bar now follows your filters, live.** Every filter you set, the
  sort order and the column layout are written into the page's own address the
  moment you set them, so the address is at all times a link to the table exactly
  as it stands. Copy it out of the address bar and open it in another tab or on
  another machine and you get the same view. A filter you have **not** set does
  not appear: an unfiltered table has a bare address, and one filter puts one
  name in it.
- **A tab opened from such a link is an ordinary tab.** It saves like any other:
  change a filter in it and the change is written to your settings, and the
  address follows what you changed. The Info dialog's own settings - the
  *Automatically download updates* tick and the whole *Automatic backups* block -
  stick there too.
- Pressing **Back** still leaves the page. Filtering does not fill the browser's
  history, however many filters you change.
- The search box waits for a pause in your typing before rewriting the address,
  so it is rewritten once per pause rather than once per letter.

### Removed
- **The Copy view link button is gone**, along with the rule that a page opened
  from a link never saved anything. Both existed to make an address you had to
  ask for; the address now writes itself, so neither has a job. Nothing that
  travelled in a link before is left out of the address now.

### Unchanged
- What may appear in the address is still a fixed list of view controls, and
  nothing else. The folder your backups are written into, the backup schedule and
  the auto-download preference are read from your own settings and never from an
  address, so a link cannot move where your backups go - and naming them in an
  address by hand still does nothing.

## [1.41.1] - 2026-07-30

### Fixed
- **The startup message now counts the models it stored, not the records it was
  offered.** OpenRouter lists a handful of *routers* - entries that pick a model
  for you rather than being one - and quotes their price as `-1`, meaning it
  depends on whichever model your request lands on. There is no price to record,
  so ModelsDB does not store them. The count in the log did not know that: it
  added them up anyway, so a refresh could report five brand-new models that were
  never written, and, because a model that is never stored leaves no row for the
  next run to recognize, report the same five as new again on every single
  start. The count now comes from the write itself, so it always matches what
  the database holds afterwards.
- **Models left out for want of a price are now named.** A separate line lists
  them, so the difference between how many models OpenRouter lists and how many
  ModelsDB holds is explained rather than silent.

## [1.41.0] - 2026-07-30

### Added
- **Downloadable versions of a model.** A model listed here is a hosted service you
  call. The same model can also exist as a file you fetch and run yourself - a
  *quantized build*, storing each weight in fewer bits so it fits on your own
  machine, published on HuggingFace by whoever made it. ModelsDB can now record
  which of those you care about, so you can compare them against the memory you
  actually have. Nothing is downloaded: what is recorded is where the file is, how
  big it is and how it is stored.
- **A two-step picker, because neither step can be decided by a program.** The new
  **Var** column shows a download icon on every model with a HuggingFace link.
  Pressing it asks which repository publishes versions of this model - there are
  usually dozens, from many owners, and not all of them really are versions of your
  model - and then which file in it, listed with each one's exact size and bit
  width. The app lists; you choose.
- **The versions appear under their model, not beside it.** The **Var** column's
  arrow opens them in a list attached to the model's own row, so they follow it
  through every sort, filter and page change instead of scattering across the
  table. Which models are open is remembered between visits, and **Clear** does not
  close them.
- **They stay out of your model list, and out of everyone else's.** A version starts
  hidden, so it is not a row in the table and not in what `GET /api/models` returns.
  An eye control beside each one includes it in those answers when you want that -
  it is the same hide flag every row carries, and which rows it applies to stays
  your choice.
- **Four columns and two endpoints carry it.** `variant_of` (the name of the model a
  version was built from, and empty on every ordinary row), `bit`, `format`, and a
  `model_details` object holding the repository, the commit read, the file names,
  the exact byte count and the architecture config. `GET /api/models/variations` and
  `GET /api/models/variations/files` are the two reads, `POST /api/models/variations`
  records one, and all three are documented in `GET /api`.
- **Nobody types a bit width.** The import re-reads the repository itself and stores
  the size, format and bit width it read, so a request cannot post numbers of its
  own and a value in either column is always a measurement. `POST /api/save` cannot
  write any of the four.
- **The two expected failures answer with a sentence and write nothing.** A machine
  with no network, and a repository whose owner requires a HuggingFace account -
  whose metadata reads fine while its files do not, so a record of it would look
  available while being impossible to fetch.

### Changed
- Schema **v10** adds those four columns. It is purely additive: nothing is dropped,
  renamed or rewritten, so every stored value survives it untouched. Rehearsed
  against a copy of a real 531-model install before release - integrity check ok,
  every count identical, and a fingerprint over every row's name, note and
  favourite byte-identical before and after.
- Two comments describing the ignore list's name-keyed fallback said it covered a
  row shape the insert paths do not produce. It covers one they can: the OpenRouter
  id is copied through with no non-empty check, so a record arriving without one
  becomes a row with nothing else to key on.

## [1.40.3] - 2026-07-30

### Changed
- Two documents said a price was "typed in". Nothing types a price: the only
  writers are the refresh phases and the direct SQL a backfill pass runs, and
  `POST /api/save` ignores every price key.

## [1.40.2] - 2026-07-29

### Changed
- Three code comments named the config file as `config.json`. The file the app
  writes and prefers is `config.jsonc`; the extension-less older name is only
  still accepted when reading one that already exists.

## [1.40.1] - 2026-07-29

### Fixed
- **A collection re-scrape could wipe a price filled in by a backfill pass.** The same
  blanking that was fixed for the pricing unit and the pricing caveat was still
  live in five more columns: the In $ and Out $ prices, the image price, the
  human pricing summary and the link the price came from. When a scraped model
  landed on a row that already existed under the same display name - which is
  what a model you added by hand looks like - the import overwrote all five with
  the empty values it carries, and those prices exist in no other file. An
  incoming empty value now leaves the stored one alone in every pricing column,
  while a real incoming price still wins. A test proves hand-set pricing survives
  an import that carries none.
- **Repeated edits to one model in the same half-second are sent once again.** The
  browser's save queue meant to keep a single pending item per model and never
  did, because the check compared against nothing, so a burst of edits on one row
  went to the server as several items. They now fold into one item carrying the
  newest value of each field. What gets saved is unchanged - only how much is
  sent.

### Changed
- The `cache_dir` comment in your `config.jsonc` now lists all four things that
  folder holds. `backup-schedule.json`, where the daily backup records its own
  state, was missing from it.

### Notes
- The two remaining refresh phases, **collections** and **pricing**, are now
  covered by a test proving neither writes any file into the data folder - the
  database stays the only copy of your data. Both are exercised against a local
  stand-in server, so the test needs no internet.
- No new endpoint, no database change, and no change to any response format.

## [1.40.0] - 2026-07-29

### Added
- **Copy view link: hand the table you are looking at to another tab, another
  machine, or another person.** A **Copy view link** button in the filter bar,
  just before **Clear**, puts a link to the view exactly as it stands on your
  clipboard: the filters, the search text, the sort order, which columns are
  showing and in what order, and whether hidden models are drawn. Open it
  anywhere and you get that view. It is also how you keep two different views of
  the catalog open side by side, which one saved-settings file could never do.
- **Opening a link changes nothing you have saved.** A tab showing a link's view
  writes nothing to disk for as long as it is open - filter in it, re-sort it,
  press **Clear** in it, and your own saved filters are untouched. So opening a
  link you saved three weeks ago cannot overwrite what you are working on today.
  While a tab is in that state the Info dialog's own settings (the automatic
  download tick and the whole automatic-backups block) do not stick either; use
  an ordinary tab for those.
- A link carries the view and nothing else. It cannot carry your backup folder,
  your automatic-download choice or your price colour thresholds: the link is
  built from a fixed list of view controls, and every other name in an address is
  ignored - so a link somebody sends you can never repoint where your backups are
  written.

### Changed
- A tab opened with no link behaves exactly as before: your saved view is
  restored and every change is saved. A link is a snapshot rather than a mirror -
  keep changing filters after copying one and the link still holds the view you
  copied, so press the button again for a link to what you see now.
- A filter value that no longer exists is now ignored instead of emptying its
  control. A saved (or linked) pricing unit no model carries any more used to
  leave that dropdown showing nothing at all, which read as a broken filter; it
  now stays on **All**.

### Notes
- No new endpoint, no database change, and no change to `GET /api/models` or any
  of its parameters. This is browser-side only: the link is the page's own
  address, not an API call.

## [1.39.0] - 2026-07-29

### Fixed
- **A collection re-scrape could wipe a pricing caveat you had typed.** When a
  scraped model landed on a row that already existed under the same display
  name, the import overwrote `measurement` (the pricing unit) and `pricing_note`
  (the objective pricing caveat) with the empty values it carries - and those two
  columns exist in no other file, so the text was simply gone. An incoming empty
  value now leaves the stored one alone, and a test proves both survive.

### Changed
- **`GET /api` now documents the whole API, not half of it.** The markdown guide
  the app hands to an AI agent (or anyone who asks) described 10 of the 20
  endpoints; ten were missing entirely, including `POST /api/save` - the one that
  writes your fields. Every endpoint now has a section of its own, with every
  HTTP method it accepts and what it does, and the four destructive ones say
  plainly what they destroy.
- **The guide now explains writing.** There is a table of the ten fields
  `POST /api/save` accepts, and for each one: the JSON type it takes, whether an
  empty value clears it or is ignored, and whether a refresh can overwrite it
  later. It also states three things that used to be written down nowhere: the
  hidden flag cannot be set through a save (`POST /api/models/hide` is its only
  writer), a save naming a model that does not exist CREATES it rather than
  failing, and a field you leave out keeps its stored value.
- **`GET /api/settings` and `POST /api/settings` are documented, with a warning.**
  They belong to the browser app: a POST replaces the whole file with the body you
  send, and that file holds your filter bar and your daily-backup schedule - so a
  partial write silently turns the daily backup off.
- The API reference cannot fall behind again: a test fails when an endpoint is
  registered without a section in the guide, and when the guide names an endpoint
  that does not exist.

### Notes
- No new endpoint, parameter, or response format. Nothing you already call
  changes, and no data is migrated.

## [1.38.0] - 2026-07-29

### Added
- **An opt-in daily backup, off by default.** Open the small **i** (info) button, and in **Files & updates** tick **Back up automatically, once a day** and type a destination folder. From then on the app writes one backup into that folder every 24 hours - the same complete, self-contained copy `modelsdb backup` produces - but only when the database has actually changed since the last one, so an idle install never piles up identical files. Tick **Replace the same file each time** to keep one file that is overwritten each run instead of a new timestamped one every time.
- **A "Back up now" button** in the same panel takes one immediately, whether or not the daily schedule is on.
- **The panel lists the backup files it finds in your chosen folder** - name, size, and when each was written - so you can see at a glance that it is working, and three honest messages when there is nothing to show: no folder configured, the folder is gone, or it is empty.
- **`modelsdb backups` now prints two labelled sections**: the app's own pre-migration snapshots (as before), and your own destination-folder backups, so the same command shows both kinds without you having to guess which is which.

### Notes
- This is off by default and stays that way for existing installs. Where your backups live is a decision about your own disk, and the app does not make it for you.
- Nothing about `modelsdb backup <folder>`, `modelsdb restore <file>`, or the pre-migration snapshots in `<data>/.backups/` changes.

## [1.37.0] - 2026-07-29

### Changed
- **"Unlisted" is now "hidden", and it finally does something.** The mark used to
  mean "left out of the catalogue file this app published", and that file is
  gone - so the flag has been given the job people expected it to have all along:
  a hidden model is kept in full and simply stops being listed. Nothing you
  marked is lost. Every model you had marked is still marked, and is now hidden.
- **A "Show hidden" box in the filter bar** says how many you have - *Show hidden
  (17)* - and draws them when you tick it. **Clear does not untick it**: clearing
  the filters is about narrowing a list, and this decides what the list is of.
  Your choice is remembered between visits.
- **In the delete question, "Make unlisted instead" is now "Hide it instead"**,
  and it tells you where the model went.

### Changed - for other programs reading this app's data
- **Read this if you have anything talking to `GET /api/models`** - the VRAM
  calculator included. Two things change for you, and both are silent:
  - **Hidden models are no longer returned.** Add `include_hidden=1` to the query
    to get them back, e.g. `GET /api/models?all=1&include_hidden=1`. Passing
    `?all=1` on its own is NOT enough and never was intended to be: it matches
    nothing, and only exists to make the query string non-empty so you get JSON
    instead of the usage guide.
  - **The per-model field `unlisted` is now called `hidden`.** Same true/false
    meaning, new name. You only ever see it on a request that asked for hidden
    models.
- **`POST /api/models/unlist` is now `POST /api/models/hide`**, and the field in
  its body is `hidden` rather than `unlisted`. There is no alias for the old
  spellings.
- The usage guide at `GET /api` documents all of this, including the thing that
  catches everyone out: a request with an empty query string returns the guide,
  not models.

### Notes
- Your database is upgraded automatically on the next launch, and a full backup
  of it is written first, as with every schema change. The flag column is renamed
  rather than rebuilt, so no stored value is touched.

## [1.36.1] - 2026-07-29

### Changed
- **Taking a backup no longer writes to the database it is backing up.**
  `modelsdb backup <folder>` now asks SQLite itself for the copy, so your live
  `modelsdb.db` and its `modelsdb.db-wal` sidecar come out of a backup exactly
  as they went in - same size, same timestamp, not a byte moved. It also no
  longer locks the database while it works, so you can take a backup with the
  app running and the browser open without anything you are doing being held up.
  What you get is unchanged: one complete, self-contained `.db` file holding
  every model, note, favourite, custom-column value and ignore-list entry, with
  the same name and in the same folder as before.
- **A backup that goes wrong can no longer cost you the one you already had.**
  The copy is now written under a working name and given its real name only once
  it is finished, so a full disk or an unplugged drive part-way through leaves
  the previous backup untouched instead of destroying it. As before, a backup
  that cannot be produced whole writes no file at all and tells you why.

### Notes
- If you open a backup file with another SQLite tool, it will have no `-wal` or
  `-shm` sidecar of its own. That is normal and nothing is missing: those files
  are rebuilt the moment ModelsDB opens the database. Backups are also created
  readable by their owner only, since they carry your personal notes.

## [1.36.0] - 2026-07-29

### Added
- **A real backup command: `modelsdb backup <folder>`.** It writes one complete
  copy of your database into a folder you name, stamped with the date and time.
  That single file holds everything - the whole catalog, your notes, your
  favourites, every value in every column you made yourself, and your ignore
  list - and it opens on its own, so you can move it to another disk and put it
  back later by renaming it to `modelsdb.db`. It never touches anything else in
  your folder: it does not tidy up, delete older backups, or cap how many you
  keep.
- **It folds in the newest changes before it copies, and refuses rather than
  guess.** The database keeps its most recent writes in a `modelsdb.db-wal`
  sidecar file, which is why copying `modelsdb.db` by hand can quietly give you a
  backup that opens perfectly and is missing your latest work. The command folds
  that sidecar back in first. If it cannot, it writes **no file at all** and says
  so, rather than leaving a copy that looks complete and is not.

### Removed
- **`curated.json` is gone.** The app no longer writes a second, partial copy of
  the catalog beside the database, and no longer rebuilds an empty database from
  one. It was never a backup: it carried the objective model facts and left out
  every note, every favourite, every value in your own columns and your whole
  ignore list, so restoring from it silently lost all of them. A copy of the
  database loses none of it, and `modelsdb backup` now makes that copy properly.
  If you have a `curated.json` in your data folder it is simply ignored; delete
  it whenever you like.
- **The `export` command**, replaced by `backup`.
- **The version gate on the catalog file.** With no catalog file to read, there
  is nothing left for an older build to refuse.

### Changed
- **Backing up is now one file, not three.** The README's **Backing up your
  data** section leads with `modelsdb backup <folder>`; copying
  `modelsdb.db`, `modelsdb.db-wal` and `modelsdb.db-shm` together by hand still
  works and is still described. The README also spells out how that differs from
  `modelsdb backups` / `modelsdb restore`, which list and roll back the automatic
  snapshots the app takes before it changes the database's shape - those are a
  safety net for upgrades, are capped at 30, and are not a backup of your data.
- **The **i** (info) dialog's file list** no longer shows a `curated.json` line,
  and says plainly that everything lives in the database file above it.
- **Marking a model unlisted only marks it.** The row and everything you wrote
  about it stay exactly as they were, and it still carries the crossed-out-eye
  mark you can click to clear. With no exported file, the mark currently withholds
  it from nothing.

## [1.35.1] - 2026-07-28

### Fixed
- **Backing up your data now tells you to copy three files, not one.** The
  database keeps its newest changes in a `modelsdb.db-wal` sidecar file and folds
  them into `modelsdb.db` later, so a copy of `modelsdb.db` alone can be behind by
  hundreds of models and everything written about them - while still opening
  cleanly and looking complete. Every place that explains backups now says to copy
  `modelsdb.db`, `modelsdb.db-wal` and `modelsdb.db-shm` together, and the README
  has a **Backing up your data** section spelling out why.
- **The agent guide at `GET /api` described un-ignoring wrongly.** It said that
  removing one ignore entry brings the model back on the next refresh. A model can
  be held by more than one entry, and it stays out while any of them names it, so
  the guide now says to clear every entry carrying that name. The in-app **Allow
  again** button already did this.

## [1.35.0] - 2026-07-28

### Removed
- **The catalog that shipped inside the app.** ModelsDB no longer carries any
  models of its own. A fresh install starts with an empty database and gets
  everything from OpenRouter - fetched automatically the moment it starts, or on
  demand from **Update models**. The catalog file that used to travel inside the
  binary, and the code that seeded a new database from it, are gone.
- **The build step that refreshed that file** from the maintainer's database
  before every build.

### Added
- **A panel that says what an empty catalog means.** When the page loads with no
  models in it - a first launch that could not reach the internet - it now says
  the app starts empty and points at the **Update models** button, instead of
  showing a table with nothing in it and no explanation.

### Changed
- **An existing install is untouched.** Your models, notes, favourites and custom
  columns are exactly where they were: the database is the source of truth, and
  nothing here reads or writes it differently. Clearing the database still
  rebuilds the objective data from that install's own `curated.json` in the data
  folder, as before.
- **`modelsdb export <file>`** is now for taking a snapshot or an off-machine
  copy of the objective catalog. It writes the same file it always did.

## [1.34.0] - 2026-07-28

### Added
- **A mark on any model that is kept out of the published catalog**, sitting on
  the row itself, and clicking it publishes that model again. Choosing "Make
  unlisted instead" in the delete question is now something you can see on the
  row afterwards and undo from there, instead of a change that leaves no trace.

### Fixed
- **Every row offers the removal verb that actually works on it.** Which verb a
  row shows depends on whether anything would put the model back by itself, not
  on whether the row carries an identifier from somewhere else. A HuggingFace
  model with no OpenRouter listing now offers **Delete**, because nothing
  re-creates it - it carries a HuggingFace address, which used to be read as a
  sign that something would bring it back.
- **The info dialog can be used from the keyboard and read aloud correctly.**
  Escape closes it, the page behind it is properly out of reach while it is
  open, focus returns to the "i" button that opened it, its headings sit in the
  right order for a screen reader, and each **Allow again** button says which
  model it allows rather than being one of a column of identical buttons. The
  delete question is built the same way.

### Changed
- **The Name column can no longer be hidden.** It carries the links, the note,
  the star and the remove control, and on a narrow screen it is the card's
  title, so hiding it left a row with no name and no way to act on it.
- **The delete question names each custom column that holds a value** for the
  model being deleted, rather than speaking of custom columns in general - and
  says plainly when there is nothing personal to lose.
- **The ignore list can record a model by name**, for a model that comes back on
  its own but carries no address to be recognised by. Nothing in the catalog is
  in that position today, so no model you are already ignoring changes; the
  database is upgraded in place on first launch and every existing entry is
  carried across.

## [1.33.0] - 2026-07-28

### Added
- **A control on every row for taking a model out of the catalog**, at the end
  of the icon row beside the favourite star. There are two verbs and you are
  never asked to pick between them: a model that came from OpenRouter or
  HuggingFace gets **Ignore** (a crossed circle), and a model you added by hand
  gets **Delete** (a waste basket). Only one of the two would work on any given
  row - deleting an upstream model does not remove it, because the catalog
  refresh puts it straight back, blank - so the row decides, not the reader.
- **An "Ignored models" tab in the info dialog**, listing every model you are
  holding back with the keys holding it, the date, and an **Allow again**
  button that lets it return on the next refresh. A model can be held back by
  two keys at once (an OpenRouter id and a HuggingFace slug) and stays out
  while either survives, so the list groups by model and one button clears the
  whole group. This makes the info dialog the app's first tabbed dialog, and
  turns it from a read-only panel into a place where something can be changed.
- **A delete confirmation that names what dies with the row** - your note, its
  favourite star, and its value in every custom column, none of which exist
  anywhere else and none of which Undo can reach - and offers the
  non-destructive alternative in the same breath: **Leave it out of the
  published catalog**, which keeps the model and everything written about it
  and only skips it when `curated.json` is written.
- **Five endpoints behind those controls**: `GET /api/ignored`,
  `DELETE /api/ignored?kind=&key=`, `POST /api/models/ignore`,
  `POST /api/models/delete` and `POST /api/models/unlist`. The `/api` agent
  guide and the README list them.
- **`unlisted` in the row shape served by `GET /api/models`**, so a client can
  see which models are kept out of the published catalog.

### Changed
- **The delete endpoint refuses any request that does not carry
  `confirm_delete: true` in its body**, and reads that flag from the body only,
  never from the URL. Every write endpoint in this app checks nothing but the
  HTTP method - no authentication, no origin check, no rate limit - and this is
  the first one that destroys data no other file holds, so the request itself
  has to say so; a dialog in a browser is not on the wire, and a URL is the
  part that gets linked, prefetched, logged and pasted.
- **Ignoring, deleting or unlisting a model re-exports `curated.json`.** All
  three change which models the catalog holds, so without it the file would
  keep describing a model the database no longer has - and would seed a deleted
  hand-added model straight back into an emptied database.

## [1.32.0] - 2026-07-28

### Added
- **An ignore list, so a model you do not want stops coming back.** Deleting
  an OpenRouter model's row never removed it: the catalog refresh runs on
  every startup and simply re-inserts it, stripped of its notes, favorite and
  curated numbers - a silent reset dressed up as a delete. The database now
  carries an `ignored_models` table keyed on the upstream key the model would
  return under (its OpenRouter id, or its HuggingFace repo slug), and all four
  paths that can create a model row consult it: the OpenRouter refresh, a
  collection import, the empty-database seed, and a save naming a model the
  database has never seen. Keyed on the upstream id rather than the display
  name, because the display name is the table's primary key and an upstream
  rename would let an ignored model back in under its new name. Ignore
  entries are personal, like notes and custom-column values: they live only
  in `modelsdb.db` and are never written to `curated.json` or the embedded
  catalog.
- **Two store verbs behind it.** `IgnoreModel` records the keys and removes
  the row, for a model that came from upstream; `DeleteModelPermanently`
  just removes the row, for a model added by hand that nothing upstream will
  re-create. Both destroy that model's note, its favorite flag and its value
  in every custom column, which cascade away with the row and have no backup
  but a copy of `modelsdb.db`.

### Changed
- **Schema version 5 -> 7**, in the usual transactional, backed-up,
  integrity-checked path. v6 adds `ignored_models`; v7 drops the dead
  `hidden` column (below).
- **Every migration run is now checked for custom-column data loss.** The
  number of stored custom-column values is counted before and after the whole
  run, and a decrease aborts and restores the pre-migration backup. Those
  rows cascade away silently whenever a `models` row is deleted, which is
  exactly what a table rebuild does, and they exist nowhere but the database
  file.

### Removed
- **Dead `models.hidden` column and its plumbing.** Nothing ever set it to
  true, and nothing in the frontend read the `hidden` field the API shipped -
  it was dead in both directions. Withholding a model from the catalog is now
  the ignore list's job, which stops the row existing rather than flagging one
  that does, so a per-row flag would be a second answer to the same question.
  Removed from the schema, from `UpsertAPIModel`, from `NormalizeModel`, and
  from the row shape served to the browser.

## [1.31.1] - 2026-07-28

### Changed
- **The `update-database-manually` skill now specifies how to read a
  HuggingFace repository, instead of telling the reader to open its web
  page.** The rewritten section names the three machine-readable addresses -
  `api/models/<repo>?blobs=true` for metadata plus every file's exact byte
  size, `resolve/main/model.safetensors.index.json` for the shards a loader
  actually reads, and `resolve/main/config.json` for the architecture - and
  spells out the traps around them: always `/resolve/main/` and never
  `/raw/main/` (a Git LFS file served through `/raw/` comes back as a
  three-line pointer with HTTP 200, which silently turns a weight-size
  calculation into a whole-repository sum); GGUF repositories return 404 for
  `config.json` and need the `base_model` tag followed to the base
  repository; how to list a model's quantized builds and why that list needs
  a human; and the cross-checks (bytes per parameter, `safetensors.total`
  counting packed elements rather than parameters, configs nested under
  `text_config`, and why a cache figure must not be derived from field
  names). Includes a Mermaid flowchart of the fetch-and-decide flow. Every
  claim was measured against the live HuggingFace API.

### Removed
- **Dead `curatedCols` variable in `internal/dbcore/db.go`.** It was declared
  with a comment claiming it protected curated fields from the OpenRouter
  refresh and was referenced nowhere. The actual protection is the explicit
  column list in `UpsertAPIModel`'s `ON CONFLICT DO UPDATE SET`, which
  ARCHITECTURE.md's "Data model" section now states. No behavior change.

## [1.31.0] - 2026-07-25

### Added
- **Go-based build-time minification of `app.js`/`style.css` (Lighthouse
  Phase 2).** `build.sh` now minifies the project's own `app.js`/`style.css`
  (~57% and ~56% smaller respectively, ~100KB combined) via
  [`tdewolff/minify`](https://github.com/tdewolff/minify) before building the
  release binaries, and embeds that minified copy instead of the source
  files. This is build-time-only and Go-only: `internal/app/static/app.js`
  and `style.css` stay exactly as authored and readable in the repo, no
  npm/Node dependency was added, and `go run ./cmd/modelsdb` (plus every
  `start_dev*.sh` script) is completely unaffected - dev mode always serves
  the unminified source. See DESIGN.md's "Stack and why" and ARCHITECTURE.md's
  "Self-contained binary" for how the build-tag-selected `//go:embed` works.
  Functionally verified against the minified build (sort, filter, inline
  edit, undo/redo, custom columns, drag-reorder, mobile card layout) with no
  regressions.

### Investigated (no code change)
- **jQuery/DataTables dependency spike.** Confirmed via DataTables' own
  official docs and forum answers that DataTables 2.x requires jQuery as a
  hard runtime dependency, even under its "vanilla" `new DataTable(...)`
  initialization syntax - there is no jQuery-free path to migrate to, so no
  migration was attempted and the UI stays jQuery + DataTables.

## [1.30.3] - 2026-07-25

### Fixed
- **"Check for updates" in the Info dialog now offers a real Apply/Restart
  flow instead of a plain download link.** Previously, clicking **Check for
  updates** always wrote a plain `download` link into the dialog, even on
  builds where the app's in-app self-update could actually run - and it also
  silently popped open the top update banner as a side effect. Now the
  dialog gets its own self-contained **Apply update** button (shown only
  when a newer release exists *and* self-update can actually succeed here -
  the same platform/signing-key/non-dev-build check the banner already
  used), a **Restart to apply** button once the update is staged, progress
  text for downloading/verifying/staged/failed, and an always-visible **Get
  binary from source** link to the GitHub release page. Clicking the
  dialog's Check-for-updates button no longer opens the banner. The banner's
  own behavior is unchanged. See `DESIGN.md`'s **Update banner and in-app
  self-update** section for how the two surfaces share one implementation.

## [1.30.2] - 2026-07-25

### Fixed
- **Lighthouse Performance/Accessibility/Agentic-Browsing fixes.** The three
  bundled `<script>` tags (`jquery.min.js`, `datatables.min.js`, `app.js`) now
  load with `defer`, so they no longer block first paint (First Contentful
  Paint measured ~35% faster in testing) while still running in the same
  order as before. Every `<select>` now has a real accessible name: the **HF
  Link** and **OpenRouter** filters gained a visible label matching the other
  filters, the hidden multiselects behind the Year/Input/Output
  checkbox-dropdown widgets gained an `aria-label`, and every per-row custom
  column dropdown gained an `aria-label` naming the column and the model. The
  page's primary content is now wrapped in a `<main>` landmark, and the
  HuggingFace link's accessible name now contains its own visible "HF" text
  (WCAG 2.5.3). Together these fixed every in-scope Lighthouse Accessibility
  and Agentic-Browsing finding on both the desktop table and the mobile card
  layout (re-measured: Accessibility 87 -> 93 desktop, 86 -> 93 mobile;
  Agentic Browsing 50 -> 99 desktop, 17 -> 67 mobile). Total Blocking Time was
  not measurably reduced by this round - see `DESIGN.md`'s new
  **Accessibility** section for the conventions this introduced.

## [1.30.1] - 2026-07-24

### Added
- **Every custom column is now filterable.** A filter control is generated
  automatically for each one - a dropdown offering **All** plus the column's
  own values for a dropdown-type column, or a substring text box for a
  text-type column - and it appears or disappears as you create or delete
  columns. It combines with every other filter (search, year, etc.) the same
  way they combine with each other.

## [1.30.0] - 2026-07-24

### Added
- **Custom columns.** Define your own personal columns from the new **Custom
  columns** button in the filter bar: a plain text field, or a single-select
  dropdown of text or number labels, in whatever order you list the dropdown
  values. Every install already starts with three - **Speed**, **Rating**, and
  **OCR** - carrying forward any values you had already set for them. Delete a
  column any time (with a clear warning first, since it permanently erases every
  model's stored value for it). Like notes and favorites, custom columns are
  always personal: stored only in your local database, never exported to
  `curated.json` or shared across installs.

### Changed
- **Speed, Rating, and OCR are now regular custom columns**, not fixed,
  hardcoded ones - so they can be deleted like any other, and any per-value
  colouring on Rating's old 0-4 scale is gone (no custom column carries colour).

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
