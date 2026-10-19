// ModelsDB - the browser page, over the cell store.
//
// EVERY FACT ON SCREEN IS A CELL. GET /api/models answers a row as identity plus
// a cells object keyed by column key, plus answered_by and disputed maps
// naming which source won each column and where two or more disagree. GET
// /api/columns says what each key is: its heading and what kind of value it
// takes. So this file holds no list of fields at all: the table, the filters,
// the editors and the forms are all built from the column list the server
// sends.
//
// WHAT A COLUMN IS, AND WHETHER A CELL CAN BE TYPED INTO, are read off the
// column list: every column carries its "kind" (fetched, measured, derived,
// stamped, yours) and its "sources" in rank order, and kindOf below is the
// one function every dialog draws a column's sentence from. A cell of a
// fetched, measured, derived or stamped column is READ ONLY the moment it
// holds an automatic answer, AND WHILE IT IS EMPTY: nothing on the page ever
// types into such a cell, and readOnlyReason says why, in both cases. The one
// way such a column holds a value nobody automatic has stated is research,
// written over the API or, for a brand-new row, through the Add model form
// (so a new row can take every field); a cell already
// showing a researched value stays editable in the page until a source
// overtakes it. A column of yours always takes what is typed as yours. What a
// column is ABOUT - which group it falls under for the filter
// panel, the column manager and the add/edit form - is a separate
// classification, domainOf, driven by the static DOMAIN_MAP below rather than
// by who writes it.

$(async function () {

  // --- The search syntax ----------------------------------------------------
  // Words, "quoted phrases", a leading ! for NOT, AND and OR. The Go twin is
  // parseSearch in internal/app/searchsyntax.go; both are held to the cases in
  // internal/app/testdata/search_cases.json. Parsing never fails, because a box
  // is read on every keystroke. Keep the markers: the case runner cuts this
  // block out by them.
  // SEARCH-SYNTAX-BEGIN
  function searchBlank(c) { return c === ' ' || c === '\t' || c === '\n' || c === '\r'; }

  // parseSearch returns an array of groups (ORed); a group is an array of
  // {text, negated} terms (ANDed). No groups means the box said nothing.
  function parseSearch(text) {
    var groups = [], group = [];
    var i = 0, n = text.length;
    function closeGroup() {
      if (group.length) { groups.push(group); group = []; }
    }
    while (i < n) {
      if (searchBlank(text.charAt(i))) { i++; continue; }
      var negated = false;
      if (text.charAt(i) === '!') { negated = true; i++; }
      var term, quoted = false;
      if (i < n && text.charAt(i) === '"') {
        quoted = true;
        var end = text.indexOf('"', i + 1);
        if (end < 0) { term = text.slice(i + 1); i = n; }
        else { term = text.slice(i + 1, end); i = end + 1; }
      } else {
        var start = i;
        while (i < n && !searchBlank(text.charAt(i))) i++;
        term = text.slice(start, i);
      }
      if (!negated && !quoted) {
        if (term === 'AND') continue;
        if (term === 'OR') { closeGroup(); continue; }
      }
      if (term === '') continue;
      group.push({ text: term.toLowerCase(), negated: negated });
    }
    closeGroup();
    return groups;
  }

  // searchMatches answers whether a cell satisfies a parsed search. members are
  // the texts the cell holds. A positive term is true when any member holds it;
  // a negated term is true when no member holds it, which an empty cell meets.
  function searchMatches(groups, members) {
    if (!groups.length) return true;
    var lowered = members.map(function (m) { return String(m).toLowerCase(); });
    return groups.some(function (group) {
      return group.every(function (term) {
        var found = lowered.some(function (m) { return m.indexOf(term.text) !== -1; });
        return found !== term.negated;
      });
    });
  }

  // searchUsesSyntax reports whether the raw text holds anything beyond plain
  // words: a quote, a term starting with !, or a standalone AND or OR.
  function searchUsesSyntax(text) {
    if (text.indexOf('"') !== -1) return true;
    return text.split(/[ \t\n\r]+/).some(function (w) {
      return w === 'AND' || w === 'OR' || w.charAt(0) === '!';
    });
  }
  // SEARCH-SYNTAX-END

  // --- Reading the server ---------------------------------------------------

  function jsonOrThrow(r) {
    if (r.ok) return r.json();
    return r.text().then(function (txt) { throw new Error(txt || ('status ' + r.status)); });
  }

  // The bars at the top of the page (index.html) are hidden until something has
  // to be said. They share one class with the update banner, so this is the one
  // place that decides a bar is on screen.
  function showBanner(id) {
    var bar = document.getElementById(id);
    if (bar) bar.classList.add('show');
  }

  var columns = [];
  var columnByKey = {};
  loadProgress('Reading the column list...');
  try {
    columns = await fetch('/api/columns').then(jsonOrThrow);
  } catch (e) {
    loadFailed('The column list could not be read: ' + (e.message || e));
    return;
  }
  columns.forEach(function (c) { columnByKey[c.key] = c; });

  // The sources declared for a column, in rank order, as the column list
  // carries them. A column with no list is read as a column of yours, which
  // is the safe default: never silently read-only.
  function sourcesOf(column) {
    return (column && column.sources) || ['hand'];
  }
  // The display words the page holds for the sources; the server sends names.
  var SOURCE_WORD = { openrouter: 'OpenRouter', huggingface: 'HuggingFace', research: 'Research', hand: 'You', app: 'The app' };
  function sourceWord(name) { return SOURCE_WORD[name] || name; }
  // The AUTOMATIC sources that can state this column: the ones an update
  // maintains, which a typed value never replaces.
  function automaticSources(column) {
    return sourcesOf(column).filter(function (s) { return s === 'openrouter' || s === 'huggingface'; });
  }
  // What a column IS, in one word the server computed: fetched, measured,
  // derived, stamped or yours. Every dialog draws its sentence from this.
  function kindOf(column) {
    if (column && column.kind) return column.kind;
    return sourcesOf(column).length === 1 && sourcesOf(column)[0] === 'hand' ? 'yours' : 'fetched';
  }
  function isYours(column) { return kindOf(column) === 'yours'; }
  // Whether a cell was answered by hand, the fact both the "edited by me"
  // filter and the cell's text colour rest on. Every hand cell counts, a
  // column of the reader's own included: a hand cell means "a column of
  // yours", which is the distinction the page draws.
  function isHandCell(row, column) {
    return !!column && (row.answered_by || {})[column.key] === 'hand';
  }
  // Whether a model has at least one such cell anywhere - what the top-level
  // "Edited by me" dropdown filters the table by.
  function hasEditedByMe(row) {
    var answered = row.answered_by || {};
    return columns.some(function (c) { return answered[c.key] === 'hand'; });
  }
  // The Repository is the one source that can measure a build, so a column
  // it is read for is what the reimport-overwrite warning is about.
  function isMeasured(column) {
    return kindOf(column) === 'measured';
  }

  // --- The five kinds, and the sentence for each -----------------------------
  // The vocabulary is fixed so the Columns dialog, its legend, the form note,
  // the cell's refusal and the answers panel all use the same words. "Source",
  // "provenance" and "rank" never appear on screen.
  var KIND_PHRASE = {
    fetched: 'Fetched from OpenRouter', measured: 'Measured from HuggingFace',
    derived: 'Worked out by the app', stamped: 'Stamped by the app', yours: 'Yours'
  };
  var KIND_ORDER = ['fetched', 'measured', 'derived', 'stamped', 'yours'];
  // The sentence a kind's legend entry carries, and the one a row's phrase
  // offers on hover.
  var KIND_LEGEND = {
    fetched: 'Fetched from OpenRouter and updated on every refresh. It cannot be typed here, filled or empty. Where it is empty, nothing has stated it yet.',
    measured: 'Measured from the model\'s HuggingFace repository and rewritten whenever the repository is read again. It cannot be typed here, filled or empty. Where it is empty, nothing has stated it yet.',
    derived: 'Worked out by the app from one OpenRouter fact; the row says which. It cannot be typed here, filled or empty. Where it is empty, nothing has stated it yet.',
    stamped: 'Stamped by the app once, when the model entered your catalog. Nothing changes it, and nothing can fill it in.',
    yours: 'Yours. Only you write it, in the page, and no update ever touches it.'
  };
  // One glyph per kind, 16px: the OpenRouter mark in its own brand colour,
  // the HF mark the Name cell already uses, fx, a padlock, a person. Reused in
  // the row's own button, the Written-by dropdown and the help list;
  // aria-hidden in the latter two, because the phrase sits beside it there.
  var KIND_ICON = {
    fetched: OR_ICON,
    measured: '<span class="hf-link cmg-ico-hf" aria-hidden="true">HF</span>',
    derived: '<svg class="cmg-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M4 20l4-16h4"/><path d="M6 12h5"/><path d="M13 12l7 8M20 12l-7 8"/></svg>',
    stamped: '<svg class="cmg-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2"><rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/></svg>',
    yours: '<svg class="cmg-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/></svg>'
  };
  function kindPhrase(column) { return KIND_PHRASE[kindOf(column)] || kindOf(column); }
  // The full sentence for ONE column: the kind's sentence, with the two-source
  // and worked-out variants where the column has them.
  function kindSentence(column) {
    var kind = kindOf(column);
    var automatic = automaticSources(column);
    var two = automatic.length === 2;
    switch (kind) {
      case 'fetched':
        if (column.resolve === 'union') {
          return 'Fetched from OpenRouter and measured from HuggingFace, added together. Updated on every refresh and cannot be typed here, filled or empty. Where it is empty, neither has stated it yet.';
        }
        if (two) {
          return 'Fetched from OpenRouter, or measured from HuggingFace where OpenRouter says nothing. Updated on every refresh and cannot be typed here, filled or empty. Where it is empty, neither has stated it yet.';
        }
        return KIND_LEGEND.fetched;
      case 'measured':
        if (two) {
          return 'Measured from HuggingFace, or fetched from OpenRouter where the repository says nothing. It cannot be typed here, filled or empty. Where it is empty, neither has stated it yet.';
        }
        return KIND_LEGEND.measured;
      case 'derived': {
        var sentence = 'Worked out by the app from ' + (column.derived_from || 'an OpenRouter fact') +
          ', on every refresh. It cannot be typed here, filled or empty. Where it is empty, OpenRouter has not stated the fact it is worked out from yet.';
        if (two) {
          sentence += ' HuggingFace can also state it, and ' + sourceWord(automatic[0]) + ' is the one shown when both do.';
        }
        return sentence;
      }
      case 'stamped':
        return KIND_LEGEND.stamped;
      default:
        return column.sealed
          ? KIND_LEGEND.yours
          : 'Yours: a column you made. Only you write it, no update ever touches it, and you can rename, colour or delete it.';
    }
  }
  // The one detail that differs per column, under the phrase in the Columns
  // dialog, or '' where there is none.
  function kindDetail(column) {
    var automatic = automaticSources(column);
    switch (kindOf(column)) {
      case 'fetched':
        return automatic.length === 2 ? 'then HuggingFace' : '';
      case 'measured':
        return automatic.length === 2 ? 'then OpenRouter' : '';
      case 'derived':
        return column.derived_from ? 'from ' + column.derived_from : '';
      case 'stamped':
        return 'once, when added';
      default:
        return column.sealed ? '' : 'a column you made';
    }
  }

  // --- Grouping a column by what it is ABOUT, not by who answers it ----------
  // Which group a column sits under in the panels is separate from who can
  // answer it (sourcesOf). This map names the groups the add/edit form uses.
  // A column the map does not name (a future shipped column, one of yours)
  // falls to "yours_alone" when nothing but hand can ever answer it, and to
  // "model" otherwise, so nothing here can leave a column undrawn.
  var DOMAIN_SECTIONS = ['yours_alone', 'model', 'capability', 'size', 'price', 'location'];
  var DOMAIN_SECTION_NAME = {
    yours_alone: 'Yours alone', model: 'The model', capability: 'What it can do',
    size: 'Size', price: 'Price', location: 'Where it lives'
  };
  var DOMAIN_MAP = {
    model_type: 'model', vendor: 'model', tokenizer: 'model', instruct_type: 'model',
    knowledge_cutoff: 'model', expiration_date: 'model', alias_target: 'model',
    description: 'model', created_at: 'model',
    base_model: 'model', zdr: 'model', eu_routing: 'model', added: 'model',
    context_length: 'capability', supports_tools: 'capability', supports_reasoning: 'capability',
    moe: 'capability', supported_parameters: 'capability', max_completion_tokens: 'capability',
    moderated: 'capability', reasoning_mandatory: 'capability', input_modalities: 'capability',
    output_modalities: 'capability',
    parameters: 'size', active_parameters: 'size', weights_size_gb: 'size',
    build_bits_per_weight: 'size', build_format: 'size', build_files: 'size',
    build_size_bytes: 'size', gguf_architecture: 'size',
    price_prompt: 'price', price_completion: 'price', price_image: 'price', price_audio: 'price',
    price_internal_reasoning: 'price', price_cache_read: 'price', price_cache_write: 'price',
    price_cache_write_1h: 'price', price_audio_cache_read: 'price', price_audio_output: 'price',
    price_web_search: 'price', price_image_output: 'price', price_unit: 'price',
    id: 'location', slug: 'location', repository: 'location', repo_owner: 'location',
    build_revision: 'location'
  };
  // A column the refresh made from the feed is not in DOMAIN_MAP, since the
  // feed decides which ones exist; its feed path says what it is about, by
  // the branch of the feed record it sits under. Anything under no branch
  // named here is about the model itself.
  var FEED_PREFIX_DOMAIN = {
    'pricing.': 'price',
    'architecture.': 'capability', 'benchmarks.': 'capability', 'reasoning.': 'capability',
    'top_provider.': 'location', 'links.': 'location'
  };
  function feedPathDomain(column) {
    var paths = column.feed_paths || [];
    for (var i = 0; i < paths.length; i++) {
      for (var prefix in FEED_PREFIX_DOMAIN) {
        if (paths[i].indexOf(prefix) === 0) return FEED_PREFIX_DOMAIN[prefix];
      }
    }
    return null;
  }
  function domainOf(column) {
    if (DOMAIN_MAP[column.key]) return DOMAIN_MAP[column.key];
    if (isYours(column)) return 'yours_alone';
    return feedPathDomain(column) || 'model';
  }

  loadProgress('Reading your view...');
  // A view that could not be READ must never be written back over. The page
  // would otherwise draw its defaults, the reader would move one filter, and
  // those defaults would be saved over the layout they spent an evening on. So a
  // failed read draws the defaults, says so in a bar, and saves nothing at all
  // until a reload reads them.
  var savedSettings = {};
  var settingsUnreadable = false;
  try { savedSettings = (await fetch('/api/settings').then(jsonOrThrow)) || {}; }
  catch (e) {
    savedSettings = {};
    settingsUnreadable = true;
    console.error('Failed to read settings', e);
    showBanner('settings-read-failed');
  }
  if (savedSettings['server.notice.settings_reset']) showBanner('settings-notice');

  // --- Which arrangement of the table this is -------------------------------
  // Everything the page remembers about the TABLE lives under view.<id>., one
  // stored row per preference, so the reader keeps several arrangements beside
  // each other. What is left at the top level belongs to the app rather than to
  // any one arrangement: the docked panel, the update preference, and the
  // backup schedule the server itself reads.
  //
  // WHICH ARRANGEMENT IS ON SCREEN IS NOT IN THE DATABASE. It is this tab's in
  // sessionStorage and this device's in localStorage, the way the theme already
  // is, so one device can never move another device, and two tabs can sit on
  // two arrangements at once.

  var VIEW_STORAGE_KEY = 'modelsdb.view';
  // The table keys a view holds. name and created are its other two rows and
  // are written rather than read off the table.
  var VIEW_TABLE_KEYS = ['filters', 'f_show_hidden', 'f_variations', 'f_edited_by_me',
    'col_order', 'col_hidden', 'col_filter_off', 'col_known', 'page_length', 'hide_empty', 'show_marks', 'full_text',
    'cards_below', 'order'];

  function viewRowKey(id, key) { return 'view.' + id + '.' + key; }

  // The day a view was made, as the local date. It sorts as a string and reads
  // as a date to anybody who opens the database by hand.
  function todayStamp() {
    var now = new Date();
    function two(n) { return (n < 10 ? '0' : '') + n; }
    return now.getFullYear() + '-' + two(now.getMonth() + 1) + '-' + two(now.getDate());
  }

  // A stored key belongs to a view when it reads view.<id>.<key> and the id is
  // a v and digits. THE ID IS NEVER THE NAME: a rename would otherwise rewrite
  // every one of a view's rows, and two names that spell one key would be one
  // view. It also cannot spell a key in the server's reserved namespace.
  function readStoredViews(all) {
    var found = {};
    Object.keys(all).forEach(function (key) {
      var parts = /^view\.(v[0-9]+)\.(.+)$/.exec(key);
      if (!parts) return;
      var v = found[parts[1]];
      if (!v) v = found[parts[1]] = { id: parts[1], number: Number(parts[1].slice(1)), keys: {}, rowKeys: [] };
      v.keys[parts[2]] = all[key];
      v.rowKeys.push(key);
    });
    return found;
  }

  // A SET OF ROWS WITH NO NAME IS NOT A VIEW. It is what a half-finished write
  // leaves behind, and reading it as one would put an entry on screen that
  // nothing can name and nothing will ever remove.
  function storedViewName(v) {
    var name = v.keys.name;
    return (typeof name === 'string' && name.trim()) ? name.trim() : '';
  }

  // A view's stored place in the list, or null when it was never given one. The
  // row is view.<id>.position (never "order", which holds the table's sort).
  // Nothing writes it until the reader first moves a view, and that move writes
  // it for every view at once.
  function viewPositionOf(raw) {
    if (raw === undefined || raw === null || raw === '' || typeof raw === 'boolean') return null;
    var n = Number(raw);
    return Number.isFinite(n) ? n : null;
  }

  // The list's order. Views with a position come first, in that order; a view
  // with none follows, oldest first. So a list nobody has reordered keeps its
  // oldest-first order, and a view made after a move arrives at the end. Two
  // views made on one day are separated by their ids, which only count up.
  function compareViewOrder(a, b) {
    if (a.position !== b.position) {
      if (a.position === null) return 1;
      if (b.position === null) return -1;
      return a.position - b.position;
    }
    return String(a.created).localeCompare(String(b.created)) || (a.number - b.number);
  }

  var storedViews = readStoredViews(savedSettings);
  var namedViews = Object.keys(storedViews)
    .map(function (id) { return storedViews[id]; })
    .filter(function (v) { return storedViewName(v); })
    .sort(function (a, b) {
      return compareViewOrder(
        { position: viewPositionOf(a.keys.position), created: a.keys.created || '', number: a.number },
        { position: viewPositionOf(b.keys.position), created: b.keys.created || '', number: b.number });
    });

  // Every number this page has ISSUED or DELETED, so neither is handed out
  // again: reissuing the number a live undo is holding would make that undo
  // overwrite a view the reader created after the delete.
  var highestNumberThisPageUsed = 0;
  function reserveViewNumber(number) {
    if (!Number.isSafeInteger(number) || !(number < Number.MAX_SAFE_INTEGER)) return;
    if (number > highestNumberThisPageUsed) highestNumberThisPageUsed = number;
  }

  // One above every number any stored view row names, so an id this page issues
  // can never land on rows that are already there. An id too long to be a whole
  // number is passed over rather than counted from: a 'v' and 1e21 spell
  // v1e+21, which matches no reader's pattern, so the arrangement would be
  // written to rows nothing can ever find while the keys it came from were
  // removed in the same write.
  function nextViewNumber() {
    var highest = highestNumberThisPageUsed;
    Object.keys(storedViews).forEach(function (id) {
      var number = storedViews[id].number;
      // The largest whole number JavaScript can tell apart from its neighbour is
      // passed over too, so what this returns stays inside that range: one above
      // it is 2^53, a number a hand-written id can already spell, and the
      // adoption would then name the same rows in both halves of its POST.
      if (!Number.isSafeInteger(number) || !(number < Number.MAX_SAFE_INTEGER)) return;
      if (number > highest) highest = number;
    });
    return highest + 1;
  }

  function readRememberedViewId() {
    try {
      var thisTab = window.sessionStorage.getItem(VIEW_STORAGE_KEY);
      if (thisTab) return thisTab;
    } catch (e) { /* a browser refusing storage opens on the oldest view */ }
    try { return window.localStorage.getItem(VIEW_STORAGE_KEY) || ''; }
    catch (e) { return ''; }
  }
  function rememberViewId(id) {
    try { window.sessionStorage.setItem(VIEW_STORAGE_KEY, id); } catch (e) { /* as above */ }
    try { window.localStorage.setItem(VIEW_STORAGE_KEY, id); } catch (e) { /* as above */ }
  }

  // An install that has an arrangement and no view is ADOPTED once: what is on
  // screen becomes the first view, named Main, and the keys it came from are
  // removed in the same write. Nothing is said, since one view named Main
  // looks like the app without views.
  var adoptingView = !settingsUnreadable && namedViews.length === 0;
  // Rows under v1 with no name are a half-written set, not a view, so adoption
  // takes the next free number rather than merging the reader's arrangement
  // into them. The load-time sweep below is what removes them.
  var currentViewId = adoptingView ? (storedViews.v1 ? 'v' + nextViewNumber() : 'v1') : '';
  // The current view's stored rows by their bare key. Every read of the saved
  // table state goes through this and none goes through savedSettings.
  var view = {};
  if (adoptingView) {
    VIEW_TABLE_KEYS.forEach(function (key) {
      if (savedSettings[key] !== undefined) view[key] = savedSettings[key];
    });
  } else if (namedViews.length) {
    var wantedViewId = readRememberedViewId();
    var opening = namedViews.filter(function (v) { return v.id === wantedViewId; })[0] || namedViews[0];
    currentViewId = opening.id;
    view = opening.keys;
  } else {
    // The settings could not be read. The page draws its defaults, says so in
    // the bar, and writes nothing at all, so this id never reaches the store.
    currentViewId = 'v1';
  }
  // Nothing is remembered from a load that could not read the settings: the id
  // above names no stored view and would only mislead the next load.
  if (!settingsUnreadable) rememberViewId(currentViewId);

  // What this load has to remove, named rather than swept by a whole-set write.
  // Both kinds are invisible on screen, so nothing a reader does would ever
  // collect them: table keys left at the top level by a version that knows
  // nothing of views, and the rows of a set that has no name.
  var staleSettingKeys = [];
  VIEW_TABLE_KEYS.forEach(function (key) {
    if (savedSettings[key] !== undefined) staleSettingKeys.push(key);
  });
  Object.keys(storedViews).forEach(function (id) {
    if (storedViewName(storedViews[id])) return;
    storedViews[id].rowKeys.forEach(function (key) { staleSettingKeys.push(key); });
  });

  // Raised for as long as this load's adoption is not stored: from the moment
  // the write is issued until its answer says it landed, and for the rest of the
  // page if it did not. Until it lands the view has no name row, so every save
  // in the meantime would add rows that are debris by this page's own
  // definition: invisible on the next load, and named in a second tab's sweep
  // while this tab is still writing them. The page stops saving instead of going
  // on producing them.
  var viewNotStored = false;

  // Whether the docked filter panel is open on a wide screen. The class goes on
  // early, before the table is built, so a panel somebody closed never flashes
  // open during the load. Open is the default a fresh install starts with.
  var filtersOpen = savedSettings.filters_open !== false;
  if (!filtersOpen) document.body.classList.add('filters-collapsed');

  // --- Which columns the table draws, and in what order ----------------------
  // The name is a column of the page's own, because it is the row's identity
  // rather than a cell, and the variations control likewise. Everything between
  // them is a real column, ordered by the position the server keeps.

  var NAME_TARGET = '_name';
  var VAR_TARGET = '_variations';
  // The column holding the date a model entered this catalog. It is a real
  // column and is named here because the page reaches for it by key: it is what
  // the bar about a finished update sorts the table by.
  var ADDED_KEY = 'added';

  // The order the app SHIPS: the name, then the columns as /api/columns answered
  // them, then the variations control. It is the fallback for anything the saved
  // order does not name, and the order the two model forms always use.
  function columnEntries() {
    var entries = [{ target: NAME_TARGET, title: 'Name' }];
    columns.slice().sort(function (a, b) { return a.position - b.position; })
      .forEach(function (c) { entries.push({ target: c.key, title: c.label, column: c }); });
    entries.push({ target: VAR_TARGET, title: 'Var' });
    return entries;
  }

  var colConfig = columnEntries();

  // A column somebody hid stays hidden across reloads. The set is kept by KEY,
  // never by a number: a key names one column for ever, so a saved layout can
  // never come back pointing at a different column.
  //
  // A SAVED LAYOUT DECIDES ONLY THE COLUMNS IT KNEW ABOUT. col_known is the
  // whole column list as it stood when the layout was saved, and col_hidden the
  // ones taken out of the table. A column added since is in neither, and the
  // server's own visible flag is what decides it - which is what makes shipping
  // a column hidden mean something on a catalog somebody has already arranged.
  // Reading an absence as "shown" instead would draw every new column in every
  // table whatever the app ships it as.
  //
  // A layout with no col_known names no columns, so everything it does not
  // hide is shown; the next save records the catalog.
  function resolveHiddenTargets(keys) {
    var hidden = {};
    if (Array.isArray(keys.col_hidden)) {
      keys.col_hidden.forEach(function (t) { hidden[t] = true; });
    }
    if (Array.isArray(keys.col_known)) {
      var knownToLayout = {};
      keys.col_known.forEach(function (t) { knownToLayout[t] = true; });
      columns.forEach(function (c) {
        if (!knownToLayout[c.key] && !c.visible) hidden[c.key] = true;
      });
    } else if (!Array.isArray(keys.col_hidden)) {
      columns.forEach(function (c) { if (!c.visible) hidden[c.key] = true; });
    }
    return hidden;
  }
  var hiddenTargets = resolveHiddenTargets(view);

  // Which columns have NO filter in the filter panel, kept apart from
  // hiddenTargets because a column can be out of the table with its filter
  // still doing its work. A layout saved before this was a setting has no
  // col_filter_off, and then every column's filter follows its table tick, so a
  // view looks the same on its first load as it did. Once stored, a column the
  // layout has not seen yet follows the server's visible flag exactly as
  // resolveHiddenTargets reads it.
  function resolveFilterOffTargets(keys) {
    if (!Array.isArray(keys.col_filter_off)) return resolveHiddenTargets(keys);
    var off = {};
    keys.col_filter_off.forEach(function (t) { off[t] = true; });
    if (Array.isArray(keys.col_known)) {
      var knownToLayout = {};
      keys.col_known.forEach(function (t) { knownToLayout[t] = true; });
      columns.forEach(function (c) {
        if (!knownToLayout[c.key] && !c.visible) off[c.key] = true;
      });
    }
    return off;
  }
  var filterOffTargets = resolveFilterOffTargets(view);

  // Hiding what holds nothing, one preference over two layouts: on a wide screen
  // it takes out the COLUMNS no row on show has a value in, and on a card it
  // takes out that card's own empty cells. It is a layout preference, so it is
  // stored with the rest of the view and never written into the address. On is
  // the default:
  // with many columns, most of them empty on most models, off is a table of
  // blanks.
  var hideEmpty = view.hide_empty !== false;
  // The class goes on before the table is built, so a card never flashes its
  // empty cells on the way to hiding them.
  $('body').toggleClass('hide-empty', hideEmpty);
  // The small marks beside a value that say who stated it. Stored with the
  // view exactly as hide-empty is, and OFF by default: with two automatic
  // sources on most columns the marks would sit on most cells.
  var showMarks = view.show_marks === true;
  $('body').toggleClass('hide-marks', !showMarks);
  // Whether a long text value (Notes, a text column of your own)
  // shows in full rather than clipped at CLIP_AT. Stored with the view exactly
  // as hide-empty is, and off by default: nobody's screen changes until they
  // opt in.
  var fullText = view.full_text === true;
  $('body').toggleClass('full-text', fullText);
  // The width below which the table becomes cards, one per model. Stored with
  // the view exactly as hide-empty is: out of the address, untouched by
  // Clear. 1642 is the shipped switch width and what a view with no
  // cards_below key reads as; 0 means never (the table stays a
  // table at any width). CARDS_BELOW_OPTIONS is the picker's own option list,
  // and a stored figure that is not one of them is trusted no further than any
  // other saved figure would be: it reads back as the default instead.
  var CARDS_BELOW_OPTIONS = [1642, 1400, 1200, 1000, 800, 0];
  function sanitiseCardsBelow(raw) {
    var n = Number(raw);
    return CARDS_BELOW_OPTIONS.indexOf(n) === -1 ? 1642 : n;
  }
  var cardsBelow = sanitiseCardsBelow(view.cards_below);
  // The columns hide-empty is taking out right now, worked out again on every
  // draw. It is kept apart from hiddenTargets, which is what somebody chose by
  // hand: the two ADD UP, and turning this off restores exactly the hand-made
  // state rather than a guess at it.
  var emptyTargets = {};
  function columnDrawn(target) { return !hiddenTargets[target] && !emptyTargets[target]; }

  // What "Reset order clears col_order" means on disk: with nothing stored the
  // table follows whatever order the app ships, today and after a release that
  // adds a column.
  var colOrderIsShipped = true;

  // The saved order decides where the columns it NAMES go, and nothing else. A
  // column it does not name - one shipped since the order was saved, or one the
  // reader has just made - lands beside the neighbours it ships with rather than
  // after everything.
  //
  // A Map answers "is this a live target", because the stored order arrives as
  // JSON somebody could have put anything in, and a bare object literal answers
  // yes to "__proto__".
  function applyColOrder(order) {
    colOrderIsShipped = !(Array.isArray(order) && order.length);
    var defaults = columnEntries();
    var byTarget = new Map();
    defaults.forEach(function (c) { byTarget.set(c.target, c); });
    var placed = new Map(), working = [];
    if (Array.isArray(order)) {
      order.forEach(function (t) {
        if (typeof t !== 'string' || !byTarget.has(t) || placed.has(t)) return;
        placed.set(t, true);
        working.push(byTarget.get(t));
      });
    }
    defaults.forEach(function (c, i) {
      if (placed.has(c.target)) return;
      var at = 0;
      for (var j = i - 1; j >= 0; j--) {
        var found = working.indexOf(defaults[j]);
        if (found >= 0) { at = found + 1; break; }
      }
      working.splice(at, 0, c);
      placed.set(c.target, true);
    });
    // Name is the row's identity and the frozen left edge, so it stays first
    // whatever the stored order says. Every other column, the variations control
    // included, sits where the order puts it.
    var nameEntry = byTarget.get(NAME_TARGET);
    working = working.filter(function (c) { return c !== nameEntry; });
    if (nameEntry) working.unshift(nameEntry);
    colConfig = working;
  }
  applyColOrder(view.col_order);

  function getColIdx(target) {
    return colConfig.findIndex(function (c) { return c.target === target; });
  }

  // --- The rows -------------------------------------------------------------

  var rawData = [];
  var rowsByName = {};
  var variationsByParent = {};
  var topLevelRows = [];
  // Whether the server narrowed the rows now held (a filter was sent), so an
  // empty answer can mean "nothing matches" rather than "the catalog is empty".
  var rowsAreFiltered = false;
  // How many rows the whole catalog draws as rows of their own, whatever filter
  // narrowed the rows held. It is the "of" in "N of M". Null until it is known.
  var catalogRowCount = null;

  function cellsOf(row) { return (row && row.cells) || {}; }
  function cellValue(row, key) { var v = cellsOf(row)[key]; return v === undefined ? '' : v; }

  // One column answers each of these questions however many sources state it,
  // because who stated a value is recorded on the cell and the server has
  // already picked which of them this row shows. The page never chooses.
  function repositoryOf(row) { return cellValue(row, 'repository'); }
  function upstreamIdOf(row) { return cellValue(row, 'id'); }
  function descriptionOf(row) { return cellValue(row, 'description'); }

  // The unit a row's prices are quoted in, which decides how a price is drawn
  // and which colour band it falls in.
  function priceUnitOf(row) { return cellValue(row, 'price_unit'); }

  function indexRows(rows) {
    rawData = rows || [];
    rowsAreFiltered = !!(rows && rows.filtered);
    rowsByName = {};
    variationsByParent = {};
    topLevelRows = [];
    rawData.forEach(function (r) { rowsByName[r.name] = r; });
    // A row is grouped under another only when that other row is really here and
    // is not itself a variation; anything else is drawn as an ordinary row, so
    // no row is left in no list at all.
    rawData.forEach(function (r) {
      var parent = r.variant_of || '';
      var parentRow = parent ? rowsByName[parent] : null;
      if (parentRow && !(parentRow.variant_of || '')) {
        (variationsByParent[parent] = variationsByParent[parent] || []).push(r);
        return;
      }
      topLevelRows.push(r);
    });
    topLevelRows.forEach(function (r) {
      variationsByParent[r.name] = variationsByParent[r.name] || [];
    });
    if (!rowsAreFiltered) catalogRowCount = topLevelRows.length;
  }

  // --- The filters, as a query in column keys --------------------------------
  // Every filter names a COLUMN BY ITS KEY and is answered by the server, which
  // is the one place the grammar lives. So the address bar carries the same
  // parameters the API takes, and a link is a query somebody can read.
  //
  // A parameter naming no column is DROPPED rather than sent: sending it would
  // answer 400 and leave the page blank, and guessing which column it meant
  // could silently show a different one.

  var NON_COLUMN_PARAMS = { name: true, sort: true, include_hidden: true, hide_free: true, links: true, hidden: true, variations: true, edited: true };

  // THE LINKS FILTER is not a column. A model has the OpenRouter link when its
  // id cell holds a value and the HuggingFace link when its repository cell
  // does, the test the row's own link icons use; "either link" looks at two
  // columns at once, which only the server's links parameter can say. Its two
  // rows keep their state in valueFilters under the key 'links', so the
  // three-state rows, the address and the saved view all work as they do for a
  // column, and sanitiseLinks keeps only conditions the server accepts.
  var LINKS_FILTER = { key: 'links', label: 'Links', value_type: 'list' };
  var LINK_ROWS = [
    { value: 'openrouter', name: 'OpenRouter', column: 'id' },
    { value: 'huggingface', name: 'HuggingFace', column: 'repository' }
  ];
  function linkRowFor(value) {
    return LINK_ROWS.filter(function (row) { return row.value === value; })[0] || null;
  }
  // The links conditions a stored view or an address carries, each rewritten as
  // any:/not: over the two known names only; a condition naming nothing the
  // server knows is dropped rather than sent, because the server refuses it.
  function sanitiseLinks(raw) {
    var out = [];
    [].concat(raw == null ? [] : raw).forEach(function (condition) {
      var text = String(condition).trim();
      var kind = text.indexOf('any:') === 0 ? 'any:' : text.indexOf('not:') === 0 ? 'not:' : '';
      if (!kind) return;
      var names = [];
      splitMembers(text.slice(kind.length)).forEach(function (member) {
        member = member.trim().toLowerCase();
        if (linkRowFor(member) && names.indexOf(member) === -1) names.push(member);
      });
      if (names.length) out.push(kind + names.join(','));
    });
    return out;
  }
  // The column behind a value-list control: a real column, or the Links filter.
  function vlfColumn(key) { return key === LINKS_FILTER.key ? LINKS_FILTER : columnByKey[key]; }

  // The two price columns whose filter carries a Hide 0 tick. Each tick is its
  // own filter on its own column, and travels as that column's key inside
  // hide_free (hide_free=price_prompt,price_completion). A bare 1, true or yes,
  // which older links and saved views carry, means both.
  var HIDE_FREE_COLUMNS = ['price_prompt', 'price_completion'];
  // The ticked columns a stored or linked hide_free names, as the canonical
  // comma list the address and the query carry, kept only for a column whose
  // filter is in the panel: a filter applies exactly when its control is there.
  // Empty when none is left.
  function hideFreeValue(raw) {
    var text = String(raw == null ? '' : raw).trim().toLowerCase();
    var named = (text === '1' || text === 'true' || text === 'yes')
      ? HIDE_FREE_COLUMNS
      : text.split(',').map(function (part) { return part.trim(); });
    return HIDE_FREE_COLUMNS.filter(function (key) {
      return named.indexOf(key) !== -1 && !!columnByKey[key] && !filterOffTargets[key];
    }).join(',');
  }
  // Whether a stored or linked condition has a control in the filter panel to
  // show and clear it. A filter applies exactly when its control is there.
  function filterHasControl(key) {
    return !filterOffTargets[key];
  }

  // THE VIEW'S FILTERS ARE NOT WHAT IS ON SCREEN. A link's query wins for the
  // visit, and a save writes the value below, so arriving on somebody's link and
  // then hiding a column cannot write that link's query into a named
  // arrangement. Only a filter act moves it, in applyFilters.
  //
  // It is SEEDED from the stored object and never from the controls, and a key
  // naming no live column is dropped on the way in - which is what writes the
  // leftover filter of a deleted column out of a view on its next save.
  function sanitiseFilters(stored) {
    var out = {};
    if (!stored || typeof stored !== 'object' || Array.isArray(stored)) return out;
    Object.keys(stored).forEach(function (key) {
      if (key === 'hide_free') {
        var storedTicks = hideFreeValue(stored[key]);
        if (storedTicks) out[key] = storedTicks;
        return;
      }
      if (key === LINKS_FILTER.key) {
        var storedLinks = sanitiseLinks(stored[key]);
        if (storedLinks.length) out[key] = storedLinks;
        return;
      }
      if (NON_COLUMN_PARAMS[key]) { out[key] = stored[key]; return; }
      if (!columnByKey[key]) return;
      // A condition whose column has no filter in the panel could never be
      // seen or cleared, so a saved view does not carry it into the query.
      if (!filterHasControl(key)) return;
      // A by: condition has no control on screen to show or clear it, so it is
      // dropped here; the column's OTHER conditions, if any, are kept.
      var conditions = [].concat(stored[key]).filter(function (v) {
        return String(v).indexOf('by:') !== 0;
      });
      if (conditions.length) out[key] = conditions;
    });
    return out;
  }
  var keptFilters = sanitiseFilters(view.filters);
  // The same seam for the two selects, which writeAddress puts into the address
  // beside the filters on every apply: a link copied out of the address bar
  // carries them, so without a kept value of their own, hiding a column would
  // write somebody else's hidden=only into a named arrangement. Only a filter
  // act moves them, in applyFilters and in the change handler the two share.
  //
  // A stored value is kept only when the select in index.html actually offers
  // it. Anything else means all of them, which is what the select itself answers
  // for a value it has no option for - and keeping it instead would write a
  // value no reader can ever have chosen back to the store on every save.
  function sanitiseSelectValue(stored, offered) {
    return offered.indexOf(stored) === -1 ? '' : stored;
  }
  var SHOW_HIDDEN_VALUES = ['', 'hide', 'only'];
  var VARIATIONS_VALUES = ['', 'yes', 'no'];
  var EDITED_VALUES = ['', 'only'];
  var keptShowHidden = sanitiseSelectValue(view.f_show_hidden, SHOW_HIDDEN_VALUES);
  var keptVariations = sanitiseSelectValue(view.f_variations, VARIATIONS_VALUES);
  var keptEditedByMe = sanitiseSelectValue(view.f_edited_by_me, EDITED_VALUES);

  function readQueryFromAddress() {
    var q = new URLSearchParams(window.location.search);
    var out = {}, dropped = [];
    q.forEach(function (value, key) {
      if (key === 'hide_free') {
        // The same rule a saved view follows: a condition with no control in
        // the panel to show it is left out, so the rows match what is shown.
        var linkedTicks = hideFreeValue(value);
        if (linkedTicks) out[key] = linkedTicks;
        else dropped.push(key);
        return;
      }
      if (key === LINKS_FILTER.key) {
        // Named more than once, like a column: an include list and an exclude
        // list ride as two parameters that AND together.
        var linkedLinks = sanitiseLinks(value);
        if (linkedLinks.length) (out[key] = out[key] || []).push(linkedLinks[0]);
        else dropped.push(key);
        return;
      }
      if (NON_COLUMN_PARAMS[key]) { out[key] = value; return; }
      // A column may be named MORE THAN ONCE - an include list and an exclude
      // list ride as two parameters that AND together - so a column's entry
      // is an array of every condition the address carries for it.
      if (columnByKey[key]) {
        // A by: condition has no control on screen to show or clear it, so it
        // is dropped here (see sanitiseFilters for the same rule on a saved view).
        if (!filterHasControl(key)) { dropped.push(key); return; }
        if (String(value).indexOf('by:') !== 0) (out[key] = out[key] || []).push(value);
        return;
      }
      dropped.push(key);
    });
    if (dropped.length) {
      console.warn('These parameters have no control in the filter panel here, so they were left out:', dropped.join(', '));
    }
    return out;
  }

  // The filter controls, one per visible column, plus the name search. Each
  // holds the value it contributes to the query, so reading the bar IS building
  // the query.
  function filterValueFor(column, raw) {
    var value = String(raw == null ? '' : raw).trim();
    if (value === '') return '';
    switch (column.value_type) {
      case 'number':
        return isPriceColumn(column)
          ? 'max:' + fixedNumberString(parseFloat(value) / 1000000)
          : 'min:' + value;
      case 'text':
      case 'list':
        return 'contains:' + value;
      default:
        return value;               // a flag or a choice is matched exactly
    }
  }

  // What one column's value-list filter has chosen: value -> 'include' or
  // 'exclude'. The EMPTY STRING is a real key here and means "no value at
  // all", which is what the Empty row cycles.
  var valueFilters = {};

  function vlfChosen(key) { return valueFilters[key] || {}; }
  function vlfState(key, value) { return vlfChosen(key)[value] || 'off'; }
  function setVlfState(key, value, state) {
    var chosen = valueFilters[key] = valueFilters[key] || {};
    if (state === 'off') {
      delete chosen[value];
      if (!Object.keys(chosen).length) delete valueFilters[key];
      return;
    }
    chosen[value] = state;
  }

  // The chosen values as the parameters the server takes: one any: for the
  // include set, one not: for the exclude set, a literal comma written \, -
  // which is why a repeated parameter meaning AND is what lets one column
  // carry both at once.
  function escapeMember(value) { return String(value).replace(/,/g, '\\,'); }
  // The other direction, for reading an address back: members split on the
  // commas that are not written \, - the same rule the server splits by, so a
  // link round-trips to the exact values that were chosen.
  function splitMembers(raw) {
    var members = [], current = '';
    for (var i = 0; i < raw.length; i++) {
      var ch = raw.charAt(i);
      if (ch === '\\' && raw.charAt(i + 1) === ',') { current += ','; i++; continue; }
      if (ch === ',') { members.push(current); current = ''; continue; }
      current += ch;
    }
    members.push(current);
    return members;
  }
  function valueFilterParams(key) {
    var chosen = vlfChosen(key);
    var include = [], exclude = [];
    Object.keys(chosen).forEach(function (value) {
      (chosen[value] === 'include' ? include : exclude).push(escapeMember(value));
    });
    var out = [];
    if (include.length) out.push('any:' + include.join(','));
    if (exclude.length) out.push('not:' + exclude.join(','));
    return out;
  }

  function buildQuery() {
    var params = {};
    var search = String($('#f_search').val() || '').trim();
    if (search) params.name = search;
    // A FILTER APPLIES EXACTLY WHEN ITS CONTROL IS IN THE PANEL, which is
    // decided by filterOffTargets and never by whether the column is drawn in the
    // table: a column taken out of the table keeps its filter.
    colConfig.forEach(function (c) {
      if (!c.column || filterOffTargets[c.target]) return;
      // A column's entry is EVERY condition on it: what its value list chose,
      // and what its box or dropdown holds - a text column can carry both.
      var conditions = valueFilterParams(c.target);
      if (c.column.date) {
        var from = String($('#' + dateControlId(c.target, 'from')).val() || '').trim();
        var to = String($('#' + dateControlId(c.target, 'to')).val() || '').trim();
        if (from) conditions.push('from:' + from);
        if (to) conditions.push('to:' + to);
      } else {
        var $control = $('#flt_' + cssId(c.target));
        if ($control.length) {
          var value = filterValueFor(c.column, $control.val());
          if (value) conditions.push(value);
        }
      }
      if (conditions.length) params[c.target] = conditions;
    });
    var linkConditions = valueFilterParams(LINKS_FILTER.key);
    if (linkConditions.length) params.links = linkConditions;
    var ticked = HIDE_FREE_COLUMNS.filter(function (key) {
      return $('#flt_hide_free_' + key).is(':checked');
    });
    if (ticked.length) params.hide_free = ticked.join(',');
    return params;
  }

  function queryString(params) {
    var q = new URLSearchParams();
    Object.keys(params).forEach(function (k) {
      [].concat(params[k]).forEach(function (v) { q.append(k, v); });
    });
    // A colon and a comma are legal in a query string and the server reads them
    // either way, so they are left as themselves: a link somebody can read is a
    // link they can check before they send it.
    return q.toString().replace(/%3A/g, ':').replace(/%2C/g, ',');
  }

  function writeAddress(params) {
    var qs = queryString(params);
    try {
      window.history.replaceState(window.history.state, '',
        window.location.origin + window.location.pathname + (qs ? '?' + qs : ''));
    } catch (e) { /* the address stays as it was; the view is unaffected */ }
  }

  // Fetch the rows this query asks for. include_hidden is always sent: the page
  // draws a variation as a row under its parent and a variation is hidden, and
  // the Hidden control decides what is DRAWN without another round trip.
  function fetchRows(params) {
    var q = Object.assign({}, params);
    delete q.sort; delete q.hidden; delete q.variations; delete q.edited;
    var filtered = Object.keys(q).length > 0;
    q.include_hidden = '1';
    return fetch('/api/models?' + queryString(q)).then(jsonOrThrow).then(function (rows) {
      rows.filtered = filtered;
      return rows;
    });
  }

  var activeQuery = readQueryFromAddress();

  // A link's query is the view for this visit. With no query in the address the
  // view is the saved one, and the first read asks for exactly what the filter
  // controls will show once restoreFilters has filled them, so the rows and the
  // controls never disagree after a reload.
  var firstQuery = Object.keys(activeQuery).length ? activeQuery : Object.assign({}, keptFilters);

  // A first read narrowed by a filter cannot say how big the whole catalog is,
  // and the "of" of the count is drawn with the first rows, so the server's own
  // count is asked for alongside them. A failed read leaves it unknown, and the
  // later read of the same answer (refreshCatalogEmptiness) fills it in.
  var firstCatalogCount = fetch('/api/columns/answers').then(jsonOrThrow)
    .then(function (answer) { return typeof answer.top_level === 'number' ? answer.top_level : null; })
    .catch(function () { return null; });

  var firstRows;
  loadProgress('Reading the models...');
  try {
    firstRows = await fetchRows(firstQuery);
  } catch (e) {
    if (firstQuery === activeQuery || !Object.keys(firstQuery).length) {
      loadFailed('The models could not be read: ' + (e.message || e));
      return;
    }
    // The saved filters are what the server refused, and a page that cannot
    // open because of them could not be used to change them. They are set aside
    // for this load, so the controls and the rows both show no filter.
    console.warn('The saved filters were refused, so this load has none:', e.message || e);
    keptFilters = {};
    firstQuery = {};
    try {
      firstRows = await fetchRows(firstQuery);
    } catch (e2) {
      loadFailed('The models could not be read: ' + (e2.message || e2));
      return;
    }
  }
  indexRows(firstRows);
  if (rowsAreFiltered) {
    var firstCount = await firstCatalogCount;
    if (firstCount !== null) catalogRowCount = firstCount;
  }

  // --- Drawing a cell -------------------------------------------------------

  function isPriceColumn(column) { return column.unit === 'USD per token'; }

  // A price is stored per single token and read per million, which is how every
  // price list quotes one. The unit the row was priced in decides it: a figure
  // charged per image or per second is already the literal price.
  function isPerMillionUnit(unit) {
    var u = String(unit || '');
    return u === '' || u.indexOf('1M') !== -1;
  }

  function shownPrice(stored, unit) {
    var n = parseFloat(stored);
    if (!isFinite(n)) return null;
    return isPerMillionUnit(unit) ? n * 1000000 : n;
  }

  function listText(stored) {
    var raw = String(stored == null ? '' : stored);
    if (raw.charAt(0) !== '[') return raw;
    try {
      var list = JSON.parse(raw);
      return Array.isArray(list) ? list.join(', ') : raw;
    } catch (e) { return raw; }
  }

  // What a cell paints, and what it sorts by. The kind of value the column holds
  // is what decides both, so a column somebody made themselves is drawn by the
  // same rules as one the app ships.
  function cellFace(row, column) {
    var stored = cellValue(row, column.key);
    if (String(stored).trim() === '') return { text: '', order: '', html: '' };
    switch (column.value_type) {
      case 'number': {
        var n = parseFloat(stored);
        if (!isFinite(n)) return { text: String(stored), order: '', html: esc(stored) };
        if (isPriceColumn(column)) {
          var unit = priceUnitOf(row);
          var shown = shownPrice(stored, unit);
          var text = shown.toLocaleString(undefined, { maximumFractionDigits: isPerMillionUnit(unit) ? 4 : 8 });
          return {
            text: text, order: shown,
            html: painted(esc(text), colorForValue(column, stored, shown)) +
              '<span class="price-unit" title="' + esc(unit) + '">' + esc(unitAbbrev(unit)) + '</span>'
          };
        }
        if (column.unit === 'tokens') {
          var k = Math.round(n / 1000) + 'k';
          return { text: k, order: n, html: painted(esc(k), colorForValue(column, stored, n)) };
        }
        return { text: String(stored), order: n, html: painted(esc(stored), colorForValue(column, stored, n)) };
      }
      case 'flag': {
        var yes = isYes(stored);
        var word = yes ? 'yes' : 'no';
        var flagColor = colorForValue(column, stored, yes ? 1 : 0);
        return { text: word, order: yes ? 1 : 0,
          html: flagColor
            ? painted(word, flagColor)
            : '<span class="flag-cell' + (yes ? ' is-yes' : '') + '">' + word + '</span>' };
      }
      case 'list': {
        var text = listText(stored);
        return { text: text, order: text, html: esc(text) };
      }
      case 'choice':
        return { text: String(stored), order: choiceOrder(column, stored),
          html: painted(esc(stored), colorForValue(column, stored, choiceOrder(column, stored))) };
      default: {
        var full = String(stored);
        return { text: full, order: full, html: clipHtml(full, column.label) };
      }
    }
  }

  // The mark beside a filled cell's value: a small filled dot for your own
  // answer, the word Research on a
  // value nothing automatic has stated, the winning source's name on every
  // other filled cell, and the RED warning mark where OpenRouter and
  // HuggingFace disagree - a warning rather than a to-do, because nothing
  // settles it. All but the warning hide under body.hide-marks.
  function provenanceMark(row, column) {
    var winner = (row.answered_by || {})[column.key];
    if (!winner) return '';
    var disputed = !!(row.disputed || {})[column.key];
    var attrs = ' data-name="' + esc(row.name) + '" data-key="' + esc(column.key) + '"';
    if (disputed) {
      return '<button type="button" class="prov-mark prov-dispute"' + attrs + ' title="' + esc(column.label) +
        ': OpenRouter and HuggingFace say different things. Click to see both.">!</button>';
    }
    if (winner === 'hand') {
      return '<button type="button" class="prov-mark prov-hand"' + attrs +
        ' title="' + esc(column.label) + ' is your own answer."></button>';
    }
    if (winner === 'research') {
      return '<button type="button" class="prov-mark prov-source"' + attrs + ' title="' + esc(column.label) +
        ' was filled in by research and is shown because nothing automatic has stated it. Click to see when.">Research</button>';
    }
    // Every other filled cell names its source, single-source columns
    // included. The tick means "say who said it", on every cell.
    var label = winner === 'merged' ? 'OpenRouter and HuggingFace' : sourceWord(winner);
    var several = automaticSources(column).length >= 2;
    return '<button type="button" class="prov-mark prov-source"' + attrs + ' title="' + esc(column.label) +
      ' is what ' + esc(label) + ' stated. Click to see ' + (several ? 'what each of them said.' : 'when.') + '">' +
      esc(label) + '</button>';
  }

  function choiceOrder(column, value) {
    var at = (column.options || []).indexOf(value);
    return at >= 0 ? at : 9999;
  }

  // Whether a value typed into this cell has somewhere to go: a column of
  // yours always; a fetched, measured or worked-out column only where it
  // already holds a researched value, which is not fetched information and so
  // may be retyped or cleared; a stamped column never. An EMPTY fetched,
  // measured, derived or stamped cell is read-only too, exactly like a filled
  // one: the page never starts a researched value, only the Add model form (a
  // new row) or a write to the API does.
  function cellWritable(row, column) {
    if (isYours(column)) return true;
    return (row && row.answered_by || {})[column.key] === 'research';
  }

  // Why a cell refuses to be typed into, in one sentence per kind, or '' when
  // it accepts. An empty cell gets the same sentence as its filled neighbour,
  // with one clause naming why: nobody has said it yet.
  function readOnlyReason(row, column) {
    if (cellWritable(row, column)) return '';
    var reason;
    switch (kindOf(column)) {
      case 'measured':
        reason = column.label + ' is measured from the HuggingFace repository, so it cannot be typed here. Read the repository again from the model\'s details.';
        break;
      case 'derived':
        reason = column.label + ' is worked out by the app from ' + (column.derived_from || 'an OpenRouter fact') + ', so it cannot be typed here.';
        break;
      case 'stamped':
        reason = column.label + ' is stamped once when a model enters your catalog, so it cannot be typed here.';
        break;
      default:
        reason = column.label + ' is fetched from OpenRouter and updated on every refresh, so it cannot be typed here.';
    }
    if (cellIsEmpty(row, column)) reason += ' Nothing has stated it yet.';
    return reason;
  }

  // Whether a cell shows nothing, answered without building what it would show.
  // cellFace answers the same question on the way past, but the table asks it
  // once to paint a cell and again to class it, and building the value twice
  // for every drawn cell is what this avoids.
  function cellIsEmpty(row, column) {
    var stored = cellValue(row, column.key);
    if (String(stored).trim() === '') return true;
    return column.value_type === 'list' && listText(stored) === '';
  }

  // A cell's own marks, which the table writes onto the cell it built and
  // renderCell writes into the <td> it builds itself. is-empty is what
  // hide-empty acts on: a card drops the cell entirely (a card has no header
  // row, so an empty cell there is a caption over nothing), and a wide screen
  // drops the whole column once no row on show fills it.
  function cellMarks(row, entry) {
    if (entry.target === NAME_TARGET) return { col: NAME_TARGET, label: 'Name', refused: '', empty: false, hand: false };
    if (entry.target === VAR_TARGET) return { col: VAR_TARGET, label: 'Var', refused: '', empty: false, hand: false };
    var column = entry.column;
    return {
      col: column.key, label: column.label,
      refused: readOnlyReason(row, column), empty: cellIsEmpty(row, column),
      hand: isHandCell(row, column)
    };
  }

  // The marks, onto a cell the table has just built. Written again whenever the
  // row is handed new data, so every mark is set from the value in front of it
  // rather than left over from the value before.
  function markCell(td, row, entry) {
    var m = cellMarks(row, entry);
    td.setAttribute('data-col', m.col);
    td.setAttribute('data-label', m.label);
    if (m.refused) {
      td.setAttribute('title', m.refused);
      td.setAttribute('data-readonly', '1');
    } else {
      td.removeAttribute('title');
      td.removeAttribute('data-readonly');
    }
    td.classList.toggle('is-empty', m.empty);
    td.classList.toggle('cell-hand', m.hand);
  }

  // Every cell of one row, marked again. The table builds a cell once and
  // rewrites its contents from then on, so this is what a row that changed needs.
  //
  // The cells are reached through the table, by row and column index, rather
  // than by walking the <tr>: a column that is not drawn keeps its cell BUILT
  // but detached from the row, and the <tr> cannot see it. Left unmarked, that
  // cell holds the marks of the value it had when it was built, and shows them
  // the moment the column comes back - which is exactly what filling in a
  // column hide-empty was holding back does.
  function markRow(at, row) {
    colConfig.forEach(function (entry, idx) {
      var td = table.cell(at, idx).node();
      if (td) markCell(td, row, entry);
    });
  }

  // What goes INSIDE one cell. The table builds the cell around this itself.
  function cellInner(row, entry) {
    if (entry.target === NAME_TARGET) return nameInner(row);
    if (entry.target === VAR_TARGET) return variationsInner(row);

    var column = entry.column;
    var face = cellFace(row, column);
    var refused = readOnlyReason(row, column);
    var inner = face.html;

    if (refused) {
      inner = inner === '' ? '' : '<span class="cell-fixed">' + inner + '</span>' + provenanceMark(row, column);
    } else if (inner === '') {
      // An empty cell holds NO TEXT of its own, so it can be hidden outright
      // wherever a blank would only be noise. What stands in for the value is a
      // control: a real button, so the keyboard reaches and works it as it does
      // any other, naming the column it fills in. It opens the very editor a
      // click on a filled cell opens. Reached only for a column of yours: an
      // empty cell of any other kind is refused above, whatever it holds.
      var addName = 'Add ' + column.label + ' for ' + row.name;
      inner = '<button type="button" class="cell-add" data-key="' + esc(column.key) +
        '" title="' + esc(addName) + '" aria-label="' + esc(addName) + '">' + ADD_CELL_ICON + '</button>';
    } else {
      var editName = (row.answered_by || {})[column.key] === 'research'
        ? 'Edit ' + column.label + ' for ' + row.name + '. Filled in by research; kept until OpenRouter or HuggingFace states it.'
        : 'Click to edit ' + column.label + ' for ' + row.name;
      inner = '<span class="cell-edit" role="button" tabindex="0" data-key="' + esc(column.key) +
        '" title="' + esc(editName) + '" aria-label="' + esc(editName) + '">' + inner + '</span>' +
        provenanceMark(row, column);
    }

    return inner;
  }

  // One cell as a whole <td>, for a variation's child row. Those rows are put
  // into the page as they are and the table never sees them, so nothing else
  // builds their cells. col-narrow is left off on purpose: it comes from the
  // column definition, which reaches the table's own rows and not these.
  function renderCell(row, entry) {
    var m = cellMarks(row, entry);
    var cls = entry.target === NAME_TARGET ? 'col-name'
      : entry.target === VAR_TARGET ? 'var-cell'
        : (m.empty ? 'is-empty' : '');
    if (m.hand) cls = (cls ? cls + ' ' : '') + 'cell-hand';
    return '<td' + (cls ? ' class="' + cls + '"' : '') +
      ' data-col="' + esc(m.col) + '" data-label="' + esc(m.label) + '"' +
      (m.refused ? ' title="' + esc(m.refused) + '" data-readonly="1"' : '') +
      '>' + cellInner(row, entry) + '</td>';
  }

  // The three questions the table asks of one cell. It builds only the page it
  // shows, so 'display' is asked once per row on the page and never for the rest, and
  // 'sort' is asked of every row but only of the column being sorted.
  //
  // 'filter' is answered with nothing on purpose. This table has no search box
  // of its own: the filter bar asks the server, and the two predicates in
  // ext.search read the row object. DataTables still builds a filter string for
  // EVERY cell of EVERY row before it runs them, which is one string per cell
  // that nothing reads. Answer it properly if a search box is ever added.
  function cellData(row, entry, type) {
    if (type === 'filter') return '';
    if (entry.target === NAME_TARGET) return type === 'display' ? nameInner(row) : row.name;
    if (entry.target === VAR_TARGET) {
      return type === 'display' ? variationsInner(row) : (variationsByParent[row.name] || []).length;
    }
    if (type === 'sort' || type === 'type') return cellFace(row, entry.column).order;
    return cellInner(row, entry);
  }

  // Which rows may be deleted, and it is the one word exactly: every other value
  // models.source takes - manual, huggingface and
  // the empty string the schema defaults to - names a row no refresh creates.
  // The server applies the same rule and is what actually enforces it.
  function deletable(row) { return row.source !== 'openrouter'; }

  // The Name cell: the model's name, which opens its details, then the links and
  // the controls that act on the whole row. Every one of them is read from a
  // cell, so a row that states no repository simply carries no repository link.
  function nameInner(row) {
    var repo = repositoryOf(row);
    var id = upstreamIdOf(row);
    var slug = cellValue(row, 'slug') || id;
    var listed = cellValue(row, 'created_at');
    var date = listed ? String(listed).split('T')[0] : '';

    var nameHtml = '<span class="name-open" role="button" tabindex="0" title="Show model details">' +
      esc(row.name) + '</span>';
    // The arrow this row is filed under, drawn before the name so it reads as
    // part of it in both the table and the card. It is only ever a note about
    // grouping, not a control, so it carries a title and nothing to press.
    var arrowHtml = row.variant_of
      ? '<span class="var-arrow" aria-hidden="true" title="A build of ' + esc(row.variant_of) + '">' +
        VAR_ARROW_ICON + '</span>'
      : '';
    var line1 = '<span class="name-line">' + arrowHtml + '<span class="tooltip">' + nameHtml +
      '<span class="tooltiptext">' + esc(descriptionOf(row)) + '</span></span>' +
      // A real space before the date: it is the only place the line may break between
      // the last word and the date, so the word stays on its line and the date drops
      // alone. Without it the last word and "(date)" are one unbreakable unit.
      (date ? ' <span class="card-date">' + esc(date) + '</span>' : '') + '</span>';

    // Every mark is written with a space before it. On a card the marks are plain
    // inline items after the date, and each space is where the line may break and
    // the gap between two marks; on the desktop row (a flex container) the spaces
    // are not drawn.
    var marks = '';
    if (repo) {
      marks += ' <a href="https://huggingface.co/' + esc(repo) + '" target="_blank" rel="noopener" class="hf-link" ' +
        'title="Open the HF (HuggingFace) page for ' + esc(repo) + ' (new tab)" ' +
        'aria-label="Open the HF (HuggingFace) page for ' + esc(repo) + '">HF</a>';
    }
    if (id) {
      marks += ' <a href="https://openrouter.ai/' + esc(slug) + '" target="_blank" rel="noopener" class="or-link" ' +
        'title="Open the OpenRouter model page for ' + esc(id) + ' (new tab)" ' +
        'aria-label="Open the OpenRouter model page for ' + esc(id) + '">' + OR_ICON + '</a>';
      marks += ' <span class="copy-id" role="button" tabindex="0" data-id="' + esc(id) +
        '" title="Copy the OpenRouter model id (' + esc(id) + ')" aria-label="Copy the model id ' + esc(id) +
        '">' + COPY_ICON + '</span>';
    }
    marks += ' <span class="row-edit" role="button" tabindex="0" title="Edit every column of ' + esc(row.name) +
      '" aria-label="Edit ' + esc(row.name) + '">' + EDIT_ICON + '</span>';
    // The way back out, on the row that is filed. Filing is one press and
    // nothing else in this app clears it, so without a control here a row filed
    // under the wrong model would stay a child of it - and a model OpenRouter
    // lists cannot be deleted out of the problem either.
    if (row.variant_of) {
      marks += ' <span class="row-unfile" role="button" tabindex="0" title="Take ' + esc(row.name) +
        ' out from under ' + esc(row.variant_of) + ', so it is a model of its own again. It stays hidden until you show it." ' +
        'aria-label="Unfile ' + esc(row.name) + ' from ' + esc(row.variant_of) + '">' + UNFILE_ICON + '</span>';
    }
    // Whether the row is on the list, as one mark in two states: the open eye
    // on a row that is showing, the crossed one on a row that is hidden.
    // Exactly one is ever drawn, and either one clicked turns the flag over.
    // Both are visible at rest - a state nobody can see is not reported, and a
    // touch screen has no hover to reveal it.
    if (row.hidden === true) {
      marks += ' <span class="hidden-mark" role="button" tabindex="0" title="Hidden from the list. Click to show ' +
        esc(row.name) + ' again." aria-label="' + esc(row.name) +
        ' is hidden from the list. Show it again.">' + HIDDEN_ICON + '</span>';
    } else {
      marks += ' <span class="row-hide" role="button" tabindex="0" title="Hide ' + esc(row.name) +
        ' from the list. The row and everything you wrote about it stay." aria-label="Hide ' +
        esc(row.name) + '">' + SHOWING_ICON + '</span>';
    }
    // Delete is drawn only where it is allowed. A row OpenRouter lists is
    // written again by the next refresh, so the route refuses to delete it
    // (app.handleDeleteModel) and the control that would ask for it is not
    // offered.
    if (deletable(row)) {
      marks += ' <span class="row-remove row-delete" role="button" tabindex="0" title="Delete ' + esc(row.name) +
        ' permanently, with every value stored about it" aria-label="Delete ' + esc(row.name) + '">' +
        DELETE_ICON + '</span>';
    }

    return line1 + '<span class="name-line meta-line">' + marks + '</span>';
  }

  function variationsInner(row) {
    var builds = variationsByParent[row.name] || [];
    var controls = '';
    if (builds.length) {
      controls += '<span class="var-count" title="' + builds.length + ' variation' +
        (builds.length === 1 ? '' : 's') + ' of ' + esc(row.name) +
        ' are recorded, and are drawn as rows under this one while hidden rows are shown">' +
        builds.length + '</span>';
    }
    if (!row.variant_of) {
      controls += '<span class="var-add" role="button" tabindex="0" title="Add a variation of ' + esc(row.name) +
        ': from HuggingFace, or a model already in your catalog" aria-label="Add a variation of ' +
        esc(row.name) + '">' + VAR_ADD_ICON + '</span>';
    }
    return controls;
  }

  // One variation's build, as a <tr>. These rows are attached to their parent
  // as child rows and the table never sees them, so they are built here and by
  // the DRAWN columns only - the opposite of the table's own rows, and right
  // for the opposite reason.
  function rowNode(row, entries) {
    var cells = entries.map(function (c) { return renderCell(row, c); }).join('');
    var $tr = $('<tr></tr>')
      .attr('data-name', row.name)
      .html(cells);
    if (row.variant_of) $tr.addClass('variation-row').attr('data-parent', row.variant_of);
    return $tr;
  }

  // --- The table ------------------------------------------------------------

  var $thead = $('<thead><tr></tr></thead>');
  $('#models').append($thead).append('<tbody></tbody>');

  // The heading row is written ONCE, before DataTables is started over the
  // table. It is never rebuilt afterwards: DataTables takes the heading cells of
  // the hidden columns out of the page and keeps them, so writing a fresh row
  // would put every one of them back.
  function drawHeaders() {
    var $row = $('#models thead tr').empty();
    colConfig.forEach(function (c) {
      // The key is on the cell, because a header drag must never read the
      // column off a DOM position: with columns hidden the position names a
      // different column.
      var $th = $('<th></th>').attr('data-target', c.target);
      if (c.target !== NAME_TARGET) $th.addClass('th-reorderable');
      if (!c.column) { $th.text(c.title); $row.append($th); return; }
      $th.append(document.createTextNode(c.column.label));
      $th.attr('title', c.column.about
        ? c.column.label + ' (' + c.column.key + '). ' + c.column.about
        : c.column.key);
      $row.append($th);
    });
  }
  drawHeaders();

  // A renamed column's heading cell, which DataTables may be holding off the
  // page while that column is hidden. Only the text changes: the key beside it
  // is what every value, filter and saved layout names the column by.
  function renameHeader(key, label) {
    var idx = getColIdx(key);
    if (idx < 0) return;
    var $th = table ? $(table.column(idx).header()) : $('#models thead th').eq(idx);
    var $into = $th.find('.dt-column-title');
    var $target = $into.length ? $into : $th;
    $target.contents().filter(function () { return this.nodeType === 3; }).remove();
    $target.prepend(document.createTextNode(label));
  }

  // The table is handed the model rows THEMSELVES and builds every cell from
  // them, so `data: null` says the whole row is the cell's source and cellData
  // answers each of the table's questions off it.
  function buildDtCols(entries) {
    return entries.map(function (c, idx) {
      var d = {
        targets: [idx],
        data: null,
        render: function (unused, type, row) { return cellData(row, c, type); },
        createdCell: function (td, unused, row) { markCell(td, row, c); }
      };
      if (c.target === NAME_TARGET) d.className = 'col-name';
      if (c.target === VAR_TARGET) { d.className = 'col-narrow var-cell'; d.type = 'num'; }
      if (c.column) {
        var numeric = c.column.value_type === 'number' || c.column.value_type === 'flag' ||
          c.column.value_type === 'choice';
        if (numeric) d.type = 'num';
        if (c.column.value_type === 'flag' || c.column.value_type === 'choice') d.className = 'col-narrow';
      }
      return d;
    });
  }

  $.fn.dataTable.ext.pager.mdbCompact = function () {
    var api = $('#models').DataTable();
    var pages = (api && api.page && api.page.info()) ? api.page.info().pages : 0;
    if (!pages) return ['previous', 'numbers', 'next'];
    if (pages <= 4) {
      var seq = ['previous'];
      for (var i = 0; i < pages; i++) seq.push(i);
      seq.push('next');
      return seq;
    }
    return ['previous', 0, 1, 'ellipsis', pages - 1, 'next'];
  };

  // Which hidden rows are drawn, and whether a model without variations is
  // shown at all. Both are read off the row itself, which the table hands in as
  // the fourth argument: a filter runs over EVERY row and most of them have no
  // cell in the page at all, so there is no element here to ask.
  $.fn.dataTable.ext.search.push(function (settings, data, dataIndex, row) {
    var fVar = $('#f_variations').val();
    var hasVar = (variationsByParent[row.name] || []).length > 0;
    if (fVar === 'yes' && !hasVar) return false;
    if (fVar === 'no' && hasVar) return false;

    var rowIsHidden = row.hidden === true;
    var mode = hiddenMode();
    if (mode === 'only') return rowIsHidden;
    if (mode === 'hide' && rowIsHidden && !(fVar === 'yes' && hasVar)) return false;

    if ($('#f_edited_by_me').val() === 'only' && !hasEditedByMe(row)) return false;
    return true;
  });

  var table = null;
  // Set around fillTable's own table.draw(false) so the draw.dt handler below
  // does not repeat the geometry read fillTable is about to take itself, after
  // applyColumnVisibility, applyViewPaging and applyFixedHeader have run.
  var drawingFromFillTable = false;

  // The table is given the model rows AS DATA and builds the cells it draws
  // itself, one page at a time (deferRender). A full catalog is tens of
  // thousands of cells; one page is a few thousand, and the browser lays out
  // only what it was given.
  //
  // On filter changes we do NOT destroy and recreate the DataTable. We keep the
  // instance, clear its rows, add the fresh set, and redraw. Destroying and
  // rebuilding costs a forced layout per column plus the full DOM teardown,
  // which is where most of the delay on typing comes from.
  function fillTable() {
    var order = table ? table.order() : null;
    // Every column in colConfig is defined, drawn or not: the table hides a
    // column by leaving its cell out of the row it builds, and a definition it
    // does not have is the "Requested unknown parameter" alert it otherwise
    // puts in the reader's face.
    var entries = colConfig;
    if (table) {
      // Keep the DataTable instance - just clear and repopulate the rows.
      table.clear();
      table.rows.add(topLevelRows);
      if (order && order.length) {
        table.order(order);
      }
      // Cleared in a finally: a throw out of draw() that left the flag set
      // would silence the draw.dt strip measurement for the life of the page.
      drawingFromFillTable = true;
      try {
        table.draw(false);
      } finally {
        drawingFromFillTable = false;
      }
    } else {
      // The height of the top bar is measured HERE, before the table exists, so
      // the reading is taken over an empty page. Taken later it forces the
      // browser to lay out every row and column first.
      topbarHeight();
      $('#models tbody').empty();
      table = $('#models').DataTable({
        data: topLevelRows,
        deferRender: true,
        pageLength: pageLen,
        lengthMenu: [[25, 50, 100, -1], [25, 50, 100, 'All']],
        language: { lengthMenu: '_MENU_ / page' },
        order: order || [],
        columnDefs: buildDtCols(entries),
        // The row's identity, on the row. Every control in a row finds the model
        // it acts on by walking up to the <tr> and reading this, so it is written
        // the moment the table builds one.
        createdRow: function (tr, row) {
          var $tr = $(tr);
          $tr.attr('data-name', row.name);
          if (row.variant_of) $tr.addClass('variation-row').attr('data-parent', row.variant_of);
        },
        // The pager and the page-size menu are drawn TWICE, above the table and
        // below it: with 25 rows on screen the foot of the table is a screen away
        // from the head, which is the same reason a second scrollbar sits above
        // it. Each layout cell builds its own instance of the feature and every
        // one of them is written again on every draw, so the two pagers cannot
        // disagree. The table layout's "N of M" sits at the head, in the topStart
        // cell on the row of the pager and the page-size menu, so it shows
        // whether the filters pane is open or shut; the card layout keeps its own
        // counter, #model-count-m, in the sort bar.
        // Every cell this table does not want is named null: the stock layout puts
        // a page-size menu at topStart and a search box at topEnd, and a key left
        // out here keeps the stock value rather than emptying it.
        layout: {
          topStart: modelCountElement, top: 'paging', topEnd: 'pageLength',
          bottomStart: 'info', bottom: 'paging', bottomEnd: 'pageLength'
        },
        pagingType: 'mdbCompact',
        fixedHeader: { header: true, headerOffset: stickyTopHeight() },
        autoWidth: false
      });
    }
    // The first-install panel speaks for a catalog with no models in it. Rows
    // narrowed by a filter that match nothing are not that: the server's own
    // count of the catalog says which it is.
    var catalogIsEmpty = rowsAreFiltered ? (catalogEmptinessRead && catalogHasNoModels) : topLevelRows.length === 0;
    $('#empty-catalog').attr('hidden', catalogIsEmpty ? null : '');
    // The real rows are in the page, so the first-paint skeleton has done its
    // job. Removed, not hidden: it exists for one load only.
    $('#load-skeleton').remove();
    emptyTargets = emptyColumnTargets();
    applyColumnVisibility();
    applyViewPaging();
    applyFixedHeader();
    attachTopScroll();
    updateModelCount();
    attachVariationRows();
    refreshShowHidden();
    // The strip spacer is measured once per rebuild here rather than inside
    // applyColumnVisibility and attachTopScroll: draw.dt's own handler is
    // suppressed for this draw (drawingFromFillTable), so this is the only
    // geometry read fillTable causes, and it lands after every DOM and style
    // write above has run.
    updateTopScrollWidth();
  }

  // Which columns the table draws. Two things make this the most expensive act
  // on the page, and both are answered here rather than lived with.
  //
  // Setting a column's visibility fires DataTables' column-visibility event, and
  // the FixedHeader extension answers EVERY one of them by measuring the table -
  // a read that forces the browser to lay out every row and column before it can
  // answer. Left alone that is one full-table layout per column, so the
  // extension is stood down for the batch and put back once at the end. Nothing
  // is painted in between, because the whole batch is one task.
  //
  // The event fires even where the answer did not change, so the columns that
  // are already as they want to be are read first and left alone - which is the
  // usual case, because this runs after every draw and the answer rarely moves.
  // Standing the extension down costs a measurement of its own, so it is done
  // only when there is really something to change.
  function applyColumnVisibility() {
    var show = [], hide = [];
    colConfig.forEach(function (c, idx) {
      if (c.target === NAME_TARGET) return;
      var wanted = columnDrawn(c.target);
      if (table.column(idx).visible() === wanted) return;
      (wanted ? show : hide).push(idx);
    });
    if (show.length || hide.length) {
      var pinned = table.fixedHeader.enabled();
      if (pinned) table.fixedHeader.enable(false);
      // One call per answer rather than one per column: DataTables takes a whole
      // set at once and moves the cells in a single pass over the rows.
      if (show.length) table.columns(show).visible(true, false);
      if (hide.length) table.columns(hide).visible(false, false);
      if (pinned) table.fixedHeader.enable(true);
    }
    table.columns.adjust();
    // A variation is a child row DataTables does not manage, built with the
    // hidden columns left out - so a visibility change rebuilds the children,
    // or the ones already attached keep cells for columns no longer drawn.
    attachVariationRows();
    syncOneColumnNote();
    // updateTopScrollWidth is deferred to the end of fillTable, so the strip
    // spacer is measured once per rebuild rather than once per column visibility
    // change and once per top-scroll attach.
  }

  // --- The pinned heading row, and the frozen Name column's scroll glue ------
  // The heading is pinned by the vendored FixedHeader extension, which clones
  // the thead and fixes the clone against the WINDOW - the page is what scrolls
  // up and down here, never the panel. The clone lives in
  // div.dtfh-floatingparent-head, a fixed, overflow-hidden box that follows the
  // panel's sideways scroll only through its own scrollLeft, so the panel's
  // scroll handler below mirrors the panel into it. Card view has no heading
  // row at all, so the extension is switched off there.

  // How far down the pinned heading sits. Measuring the bar is a geometry read,
  // and a geometry read taken while the table's own layout is out of date makes
  // the browser lay the WHOLE table out before it can answer - which is why the
  // answer is kept rather than taken again on every rebuild. The bar is one line
  // of fixed padding whose contents truncate instead of wrapping, so the only
  // thing that moves it is the window itself; both handlers that answer a window
  // change forget the measurement first.
  var topbarPx = null;
  function topbarHeight() {
    if (topbarPx === null) topbarPx = Math.ceil($('#topbar').outerHeight() || 0);
    return topbarPx;
  }
  function forgetTopbarHeight() { topbarPx = null; }
  // The bars at the top of the page stick directly below the top bar on a wide
  // screen, so the height they stick at is the top bar's own, measured and not
  // written down. stickyTopHeight is how much of the screen's top both of them
  // cover: the pinned table heading starts below it, and so does the room a
  // focused control is scrolled into (--sticky-top feeds scroll-padding-top).
  function stackIsSticky() { return getComputedStyle(document.getElementById('banner-stack')).position === 'sticky'; }
  function stickyTopHeight() {
    var stack = document.getElementById('banner-stack');
    return topbarHeight() + (stackIsSticky() ? Math.ceil(stack.getBoundingClientRect().height) : 0);
  }
  function syncStickyTop() {
    var root = document.documentElement.style;
    var barH = getComputedStyle(document.getElementById('topbar')).position === 'sticky' ? Math.ceil(document.getElementById('topbar').getBoundingClientRect().height) : 0;
    root.setProperty('--topbar-h', barH + 'px');
    root.setProperty('--sticky-top', (barH ? stickyTopHeight() : 0) + 'px');
    applyFixedHeader();
  }
  function applyFixedHeader() {
    if (!table) return;
    table.fixedHeader.enable(!cardsQuery.matches);
    if (!cardsQuery.matches) table.fixedHeader.headerOffset(stickyTopHeight());
  }
  (function () {
    // The top bar or the bars changing height (a bar appears, text wraps) moves
    // where the bars stick and where the heading pins.
    var again = function () { forgetTopbarHeight(); syncStickyTop(); };
    if (window.ResizeObserver) {
      var observer = new ResizeObserver(again);
      observer.observe(document.getElementById('topbar'), { box: 'border-box' });
      observer.observe(document.getElementById('banner-stack'), { box: 'border-box' });
    }
    window.addEventListener('resize', again);
    again();
  })();

  // A browser draws one scrollbar per scrolling box, so a second bar above the
  // table is a second box: a thin strip whose spacer is kept as wide as the
  // table's content. The strip and the panel copy each other's scrollLeft; a
  // write that changes nothing fires no scroll event, and the strip drops the
  // event its own write caused (see stripWrittenLeft), which is what stops the
  // two chasing each other. The same handler drives the FixedHeader clone and
  // the frozen column's edge shadow, so all three read one scroll position.
  function tableContainer() { return $('#models').closest('.dt-container'); }
  function updateTopScrollWidth() {
    var $c = tableContainer();
    var $strip = $('.table-scroll-top');
    if (!$c.length || !$strip.length) return;
    $strip.toggleClass('has-overflow', $c[0].scrollWidth > $c[0].clientWidth + 1);
    $strip.children().first().css('width', $c[0].scrollWidth + 'px');
    setStripLeft($strip, $c.scrollLeft());
  }
  // The strip's position as this code last wrote it. A scroll event is
  // delivered a frame after the write that caused it and reads the strip's
  // position when it fires, so a stale one from an earlier stop can land after
  // a keyboard correction moved the panel again and put the old position back
  // (the strip's handler copies the strip into the panel). An event whose
  // position is the one this code wrote is therefore its own echo and is
  // dropped; a position the reader put there (a drag) is never equal to it,
  // because the drag handler clears the record.
  var stripWrittenLeft = null;
  function setStripLeft($strip, left) {
    if ($strip.scrollLeft() === left) return;
    $strip.scrollLeft(left);
    stripWrittenLeft = $strip.scrollLeft();
  }
  function mirrorPanelScroll(left) {
    tableContainer().toggleClass('is-hscrolled', left > 0);
    var $fp = $('div.dtfh-floatingparent-head');
    if (!$fp.length) return;
    $fp.toggleClass('is-hscrolled', left > 0);
    // FixedHeader sizes its clone wrapper to the table's own parent, which in
    // this layout is as wide as the whole table - a wrapper with nothing to
    // clip cannot scroll. It is re-fitted here to the panel's visible inner
    // box, which is what lets its scrollLeft follow the panel. In an animation
    // frame so it lands AFTER FixedHeader's own handler, which re-writes the
    // wrapper's box every time the heading starts floating.
    window.requestAnimationFrame(function () {
      if ($fp.css('position') !== 'fixed') return;
      var c = tableContainer()[0];
      if (!c) return;
      var rect = c.getBoundingClientRect();
      var cs = window.getComputedStyle(c);
      var padLeft = parseFloat(cs.paddingLeft) || 0;
      var padRight = parseFloat(cs.paddingRight) || 0;
      $fp.css({
        left: (rect.left + padLeft) + 'px',
        width: (c.clientWidth - padLeft - padRight) + 'px'
      });
      $fp.scrollLeft(tableContainer().scrollLeft());
    });
  }
  // The sticky Name column does not shrink the panel's scrollport, so the
  // browser treats a header or control lying behind it as already visible and
  // does not scroll it out. And with the pinned heading showing, it scrolls the
  // clone wrapper instead of the panel, which pulls the headings off their
  // columns. After a KEYBOARD focus move (a click must not jump) the panel is
  // therefore scrolled by the measured overlap with the Name header's right
  // edge, and the clone re-aligned with the panel. Measured each time, so a
  // resized Name column is followed. It runs inside the focus event itself,
  // not a frame later: the browser has scrolled by then, and a frame later left
  // some Tab stops at 900px wide still covered.
  function revealFocusedColumn(target) {
    var c = tableContainer()[0];
    if (!c || !target || !target.closest) return;
    var nameTh = c.querySelector('thead th.col-name');
    if (!nameTh || getComputedStyle(nameTh).position !== 'sticky') return;
    var measured = target;
    var fp = target.closest('div.dtfh-floatingparent-head');
    if (fp) {
      // The clone's heading stands for the real one: take its place in the row
      // and measure the real cell, whose sideways position is the true one.
      var cell = target.closest('th');
      var realRow = c.querySelector('thead tr');
      if (!cell || !realRow) return;
      measured = realRow.children[Array.prototype.indexOf.call(cell.parentNode.children, cell)];
      if (!measured) return;
      fp.scrollLeft = c.scrollLeft;
    } else if (!c.contains(target) || target.closest('thead th.col-name, td.col-name')) {
      return;
    } else if (target.closest('thead th')) {
      measured = target.closest('thead th');
    }
    var box = measured.getBoundingClientRect();
    var edge = nameTh.getBoundingClientRect().right;
    var inner = c.getBoundingClientRect().left + c.clientLeft + c.clientWidth;
    // Whole pixels, rounded up: a scroll position is rounded to a device pixel,
    // and a fractional overlap rounded down leaves a sliver still covered.
    if (box.left < edge) c.scrollLeft -= Math.ceil(edge - box.left);
    else if (box.right > inner) c.scrollLeft += Math.ceil(Math.min(box.right - inner, box.left - edge));
  }
  document.addEventListener('focusin', function (e) {
    var t = e.target;
    if (!t || !t.matches || !t.matches(':focus-visible')) return;
    revealFocusedColumn(t);
  }, true);
  function attachTopScroll() {
    var $c = tableContainer();
    if (!$c.length) return;
    var $strip = $('.table-scroll-top');
    if (!$strip.length) {
      $strip = $('<div class="table-scroll-top" aria-hidden="true"><div></div></div>');
      $strip.on('scroll', function () {
        var left = $(this).scrollLeft();
        if (left === stripWrittenLeft) { stripWrittenLeft = null; return; }
        stripWrittenLeft = null;
        var $panel = tableContainer();
        if ($panel.scrollLeft() !== left) $panel.scrollLeft(left);
      });
    }
    // The panel is built anew every time the table is, so the strip is moved
    // back above the current one and the handler rebound on it.
    $strip.insertBefore($c);
    $c.off('scroll.mdbTopScroll').on('scroll.mdbTopScroll', function () {
      var left = $(this).scrollLeft();
      var $s = $('.table-scroll-top');
      // Late layout shifts (a scrollbar appearing, a child row widening the
      // table) can leave the spacer stale, so it is healed on the spot: a
      // stale spacer caps how far the strip can drag the panel.
      var spacer = $s.children().first()[0];
      if (spacer && parseFloat(spacer.style.width) !== this.scrollWidth) {
        spacer.style.width = this.scrollWidth + 'px';
      }
      setStripLeft($s, left);
      mirrorPanelScroll(left);
    });
    // updateTopScrollWidth is deferred to the end of fillTable, so the strip
    // spacer is measured once per rebuild rather than once per attach.
    mirrorPanelScroll($c.scrollLeft());
  }
  // A sort or a page change driven through DataTables' own UI - never through
  // fillTable - can redraw the rows at different widths, and the strip's
  // spacer must keep matching the panel's real scroll range. Skipped while
  // fillTable is driving the draw, because fillTable takes this same reading
  // itself once every other layout-affecting call has run.
  $('#models').on('draw.dt', function () {
    if (drawingFromFillTable) return;
    updateTopScrollWidth();
  });

  // Which columns hold nothing at all among the rows the filters are showing.
  // Judged over EVERY row that passes the filters rather than the page on
  // screen, because a column emptying itself as you turn the pages would be
  // unreadable, and over the variations drawn under those rows, so a value only
  // one build states still keeps its column. A card answers this question cell
  // by cell in its own stylesheet, so nothing is taken out of the table there.
  function emptyColumnTargets() {
    var out = {};
    if (!hideEmpty || cardsQuery.matches || !table) return out;
    var holds = {};
    function note(row) {
      colConfig.forEach(function (c) {
        if (!c.column || holds[c.target]) return;
        if (String(cellValue(row, c.target)).trim() !== '') holds[c.target] = true;
      });
    }
    table.rows({ search: 'applied' }).every(function () {
      var row = this.data();
      note(row);
      (variationsByParent[row.name] || []).forEach(note);
    });
    colConfig.forEach(function (c) { if (c.column && !holds[c.target]) out[c.target] = true; });
    return out;
  }

  function sameTargetSet(a, b) {
    var ka = Object.keys(a), kb = Object.keys(b);
    return ka.length === kb.length && ka.every(function (k) { return b[k]; });
  }

  // Run after a draw, where the set of rows passing the filters is settled.
  // The columns are only re-hidden when the answer really changed, because
  // rewriting the whole table's visibility on every draw would cost a layout
  // pass per keystroke in a filter box.
  function applyHideEmpty() {
    var next = emptyColumnTargets();
    if (sameTargetSet(next, emptyTargets)) return;
    emptyTargets = next;
    applyColumnVisibility();
    // The Columns dialog says, on the row itself, which ticked columns this cut
    // is holding back - so it is drawn again whenever the answer changes under
    // it, and never left saying something that has stopped being true.
    if (columnsDialogOpen()) buildColumnChooser();
  }

  // The Columns dialog is a plain overlay, shown and hidden by its display,
  // so this is the one test of whether it is on screen.
  function columnsDialogOpen() {
    return $('#col-manage-modal').css('display') === 'flex';
  }
  // Whether any dialog at all is on screen: a native one opened with
  // showModal(), or any element carrying role="dialog" that is displayed (the
  // Columns overlay, the model details overlay, the views panel). A native
  // dialog makes the page under it inert, but a key pressed on a button inside
  // any of them still bubbles to the document, so the page's own shortcuts
  // have to ask this before they act. Asking by role rather than by id is
  // what keeps the next overlay covered without a line here.
  function anyDialogOpen() {
    if (document.querySelector('dialog[open]')) return true;
    return Array.prototype.some.call(document.querySelectorAll('[role="dialog"]'), function (el) {
      return $(el).css('display') !== 'none';
    });
  }

  // The table starts at 25 rows a page, and the size chosen in the page-size
  // menu is remembered per computer with the rest of the view and holds in
  // every layout.
  var PAGE_LENGTHS = [25, 50, 100, -1];
  var pageLen = 25;
  if (PAGE_LENGTHS.indexOf(Number(view.page_length)) !== -1) {
    pageLen = Number(view.page_length);
  }
  function applyViewPaging() {
    if (table.page.len() !== pageLen) table.page.len(pageLen).draw(false);
  }

  // --- The card breakpoint: the chosen width, live through the CSSOM --------
  // A media query cannot read a setting, so the width is written into every
  // stylesheet rule that carries it: every "(min-width: 1643px)" rule is the
  // wide side, every "(max-width: 1642px)" one without "pointer" the card
  // side, and the one with "pointer: coarse" the touch side. They are found
  // once by the figures they still carry from the file and kept rather than
  // searched for again on every change.
  var cardBreakpointRules = null;
  function findCardBreakpointRules() {
    var wide = [], cards = [], touch = [];
    Array.prototype.forEach.call(document.styleSheets, function (sheet) {
      var rules;
      try { rules = sheet.cssRules; } catch (e) { return; }
      if (!rules) return;
      Array.prototype.forEach.call(rules, function (rule) {
        if (!(rule instanceof CSSMediaRule)) return;
        var mt = rule.media.mediaText;
        if (mt.indexOf('1643px') !== -1) wide.push(rule);
        else if (mt.indexOf('1642px') !== -1 && mt.indexOf('pointer') !== -1) touch.push(rule);
        else if (mt.indexOf('1642px') !== -1) cards.push(rule);
      });
    });
    return { wide: wide, cards: cards, touch: touch };
  }
  // px is the chosen width, or 0 for "never". A width matches the docked panel
  // at px+1 and above and the cards at px and below, the same split the
  // shipped 1642/1643 pair keeps, so the two never overlap and never gap.
  // "Never" writes a max-width the card rules cannot match at any real window
  // width and a min-width the docked-panel rules always match, rather than
  // disabling the rules: no selector changes, only the figure in each one.
  function applyCardBreakpoint(px) {
    if (!cardBreakpointRules) cardBreakpointRules = findCardBreakpointRules();
    var r = cardBreakpointRules;
    var wideWidth = px === 0 ? 0 : px + 1;
    r.cards.forEach(function (rule) { rule.media.mediaText = '(max-width: ' + px + 'px)'; });
    r.touch.forEach(function (rule) {
      rule.media.mediaText = '(max-width: ' + px + 'px) and (pointer: coarse)';
    });
    r.wide.forEach(function (rule) { rule.media.mediaText = '(min-width: ' + wideWidth + 'px)'; });
  }

  // The MediaQueryList the rest of the page reads cardsQuery.matches off, and
  // the four things a crossing of it drives. Named functions, not inline
  // ones, because a rebuild has to remove them from the query it is replacing
  // before it can give the new one the same set.
  var cardsQuery = null;
  function cardsQueryTopbar() {
    forgetTopbarHeight();
    applyFixedHeader();
    updateTopScrollWidth();
  }
  function cardsQueryColumnsRedraw() {
    // The Columns dialog switches row shape at the same breakpoint the table
    // switches table-vs-cards at, so a resize with the dialog open has to
    // redraw it too, with no tick, selection or held column lost.
    if (columnsDialogOpen()) buildColumnChooser();
  }
  // The stylesheet's other width queries ("(max-width: 700px)", the Columns
  // dialog's 960px/961px pair) mean "a screen this wide". When the browser lays
  // the page out wider than the screen (--glass above 1, see index.html) and the
  // cards are showing, the glass is only 1/--glass of that, so each threshold is
  // multiplied by --glass and the narrow forms apply as they do on a phone laid
  // out at its own width. A min-width is (N - 1) times --glass plus a hair, so it
  // meets the matching max-width without a gap an integer width could fall in;
  // at --glass 1 (and in a table view) the original text is put back. The Columns
  // dialog's pair is the exception, below. The card breakpoint rules are left to
  // applyCardBreakpoint.
  var glassWidthRules = null;
  function applyGlassWidthQueries(unscaled) {
    if (!cardBreakpointRules) return;
    if (!glassWidthRules) {
      var skip = cardBreakpointRules.wide.concat(cardBreakpointRules.cards, cardBreakpointRules.touch);
      var found = [];
      var walk = function (rules) {
        Array.prototype.forEach.call(rules, function (rule) {
          if (!(rule instanceof CSSMediaRule)) return;
          if (skip.indexOf(rule) === -1 && /\((min|max)-width:\s*[\d.]+px\)/.test(rule.media.mediaText)) {
            found.push({ rule: rule, text: rule.media.mediaText });
          }
          walk(rule.cssRules);
        });
      };
      Array.prototype.forEach.call(document.styleSheets, function (sheet) {
        var rules;
        try { rules = sheet.cssRules; } catch (e) { return; }
        if (rules) walk(rules);
      });
      glassWidthRules = found;
    }
    var cards = unscaled !== true && cardsQuery && cardsQuery.matches;
    var g = cards ? (parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--glass')) || 1) : 1;
    // The Columns dialog's wide table needs a width that grows with the text size,
    // which the touch rules enlarge on every touch screen showing cards, a tablet
    // included, so on those it is never the form to use. 100000px is out of reach
    // of any window, so the max-width half always matches and the min-width never.
    var narrowColumns = cards && window.matchMedia('(pointer: coarse)').matches;
    glassWidthRules.forEach(function (w) {
      if (narrowColumns && (w.text === '(max-width: 960px)' || w.text === '(min-width: 961px)')) {
        w.rule.media.mediaText = '(' + w.text.slice(1, 4) + '-width: 100000px)';
        return;
      }
      w.rule.media.mediaText = g === 1 ? w.text : w.text.replace(/\((min|max)-width:\s*([\d.]+)px\)/g, function (m, kind, n) {
        var v = parseFloat(n);
        return '(' + kind + '-width: ' + (kind === 'max' ? v * g : (v - 1) * g + 0.01).toFixed(2) + 'px)';
      });
    });
  }
  function attachCardsQueryListeners(mql) {
    mql.addEventListener('change', cardsQueryTopbar);
    mql.addEventListener('change', applyGlassWidthQueries);
    // Crossing the breakpoint changes which half of hide-empty is in force,
    // and a column taken out on a wide screen has to come back for the cards.
    mql.addEventListener('change', applyHideEmpty);
    mql.addEventListener('change', cardsQueryColumnsRedraw);
    mql.addEventListener('change', syncFilterFunnel);
  }
  function detachCardsQueryListeners(mql) {
    mql.removeEventListener('change', cardsQueryTopbar);
    mql.removeEventListener('change', applyGlassWidthQueries);
    mql.removeEventListener('change', applyHideEmpty);
    mql.removeEventListener('change', cardsQueryColumnsRedraw);
    mql.removeEventListener('change', syncFilterFunnel);
  }
  // Rewrites the stylesheet for px, swaps cardsQuery for one built on the same
  // width, and moves every listener from the old query to the new one. A
  // change of the width can itself cross it at the window's CURRENT size -
  // 1500px wide was below 1642 and is above 1200 - so where fire is true the
  // four effects are called once by hand whenever the match state came out
  // different, the same as a real resize would have driven them.
  function rebuildCardsQuery(px, fire) {
    var was = cardsQuery ? cardsQuery.matches : null;
    applyCardBreakpoint(px);
    if (cardsQuery) detachCardsQueryListeners(cardsQuery);
    cardsQuery = window.matchMedia(px === 0 ? '(max-width: 0px)' : '(max-width: ' + px + 'px)');
    attachCardsQueryListeners(cardsQuery);
    applyGlassWidthQueries();
    if (fire && was !== null && was !== cardsQuery.matches) {
      cardsQueryTopbar();
      applyHideEmpty();
      cardsQueryColumnsRedraw();
      syncFilterFunnel();
    }
  }
  // Built before the table below, so the stylesheet already carries the chosen width
  // the first time anything measures against it.
  rebuildCardsQuery(cardsBelow, false);
  // The sticky top bar's height is the floating heading's offset, and both it
  // and the panel's overflow change with the window, so a resize re-reads them.
  var fhResizeTimer = null;
  // The window's width against the screen's sets --glass (see index.html), so a
  // change of Chrome's zoom or of the orientation re-reads it before the bar is.
  // The window is measured with the scaling taken off first: a layout that the
  // scaled rules (and a bigger text) made wider than the screen is measured wider
  // than it will be once the new width's rules are in, so a jump from a very wide
  // layout straight to the screen's own width read a width a little over the
  // screen and kept a factor above 1 until the next resize. That costs a layout
  // of the whole page at the smaller text, so it runs only when the window's width
  // or the screen's differs from the last resize: Chrome on Android fires resize
  // for the address bar showing and hiding, which changes the height only.
  var glassLastW = window.innerWidth, glassLastS = window.screen.width;
  // The card at the top of the screen and how far down it sits, noted as the page
  // scrolls. A change of width re-flows every card, and the scroll offset in pixels
  // then lands on a different card (hundreds away at 695 cards), so after a width
  // change the same card is put back where it was.
  var glassAnchor = null, glassAnchorQueued = false;
  function noteGlassAnchor() {
    if (glassAnchorQueued) return;
    glassAnchorQueued = true;
    requestAnimationFrame(function () {
      glassAnchorQueued = false;
      var tb = document.querySelector('#models tbody');
      var rows = tb ? tb.rows : null;
      if (!rows || !rows.length) return;
      var lo = 0, hi = rows.length - 1;
      while (lo < hi) {
        var mid = (lo + hi) >> 1;
        if (rows[mid].getBoundingClientRect().bottom > 1) hi = mid; else lo = mid + 1;
      }
      glassAnchor = { row: rows[lo], name: rows[lo].getAttribute('data-name'), top: rows[lo].getBoundingClientRect().top };
    });
  }
  function restoreGlassAnchor(a) {
    if (!a || !a.name) return;
    var row = a.row.isConnected ? a.row : $('#models tbody tr').filter(function () { return this.getAttribute('data-name') === a.name; })[0];
    if (!row) return;
    var d = row.getBoundingClientRect().top - a.top;
    if (Math.abs(d) > 1) window.scrollBy(0, d);
  }
  window.addEventListener('scroll', noteGlassAnchor, { passive: true });
  var glassRestore = null;
  $(window).on('resize', function () {
    var w = window.innerWidth, s = window.screen.width;
    if (window.setGlassScale && (w !== glassLastW || s !== glassLastS)) {
      glassLastW = w; glassLastS = s;
      var anchor = glassAnchor;
      document.documentElement.style.setProperty('--glass', '1');
      applyGlassWidthQueries(true);
      void document.documentElement.offsetWidth;
      window.setGlassScale();
      applyGlassWidthQueries();
      restoreGlassAnchor(anchor);
      glassRestore = anchor;
    }
    forgetTopbarHeight();
    clearTimeout(fhResizeTimer);
    fhResizeTimer = setTimeout(function () {
      if (glassRestore) { restoreGlassAnchor(glassRestore); glassRestore = null; }
      applyFixedHeader(); updateTopScrollWidth();
    }, 150);
  });
  // The floating clone is created the moment the heading first floats, which
  // can happen while the panel is already scrolled sideways - so the panel's
  // position is mirrored into it on every window scroll, not only on the
  // panel's own scroll.
  // Mirrored once per scroll event AND once after the scrolling settles: the
  // extension throttles its own handler, so its last run can land after ours
  // and un-fit the wrapper it floats the clone in.
  var fhScrollSettleTimer = null;
  $(window).on('scroll', function () {
    var $c = tableContainer();
    if (!$c.length) return;
    mirrorPanelScroll($c.scrollLeft());
    clearTimeout(fhScrollSettleTimer);
    fhScrollSettleTimer = setTimeout(function () {
      var $cc = tableContainer();
      if ($cc.length) mirrorPanelScroll($cc.scrollLeft());
    }, 250);
  });
  $('#models').on('length.dt', function (e, settings, len) {
    if (!table) return;
    if (PAGE_LENGTHS.indexOf(len) === -1 || len === pageLen) return;
    pageLen = len;
    saveSettings();
  });

  function hiddenMode() {
    var v = $('#f_show_hidden').val();
    return (v === 'hide' || v === 'only') ? v : 'show';
  }
  function hiddenRowCount() {
    var n = 0;
    table.rows().data().each(function (row) { if (row.hidden === true) n++; });
    return n;
  }
  function refreshShowHidden() {
    var n = hiddenRowCount();
    $('#f_show_hidden_count').text(n > 0 ? ' (' + n + ')' : '');
  }

  // The table layout's "N of M" element, built into the topStart layout cell.
  // The table is rebuilt from scratch on every fill, so this runs again each time.
  function modelCountElement() {
    return $('<span id="model-count" class="model-count" title="Models shown of total"></span>')[0];
  }

  function updateModelCount() {
    var info = table.page.info();
    $('#model-count, #model-count-m').text(info.recordsDisplay + ' of ' + catalogTotal(info));
  }
  // The catalog's size for the "of" of a count: the whole catalog, not the rows a
  // server-side filter left in the table.
  function catalogTotal(info) {
    return catalogRowCount === null ? info.recordsTotal : catalogRowCount;
  }
  // Bound to the table ELEMENT rather than to one DataTables instance, so the
  // two passes every draw needs survive the table being started again over a
  // new set of rows.
  // The first draw happens while DataTables is still being constructed, before
  // there is an instance to ask anything of, so that one draw is left to
  // fillTable, which runs both passes itself once the instance exists.
  $('#models').on('draw.dt', function () {
    if (!table) return;
    updateModelCount();
    applyHideEmpty();
    attachVariationRows();
  });

  // A variation is attached to its parent as a CHILD ROW, so it follows that row
  // through every sort, filter and page change instead of drifting away from the
  // model it was built from.
  function attachVariationRows() {
    var showing = hiddenMode() !== 'hide' || $('#f_variations').val() === 'yes';
    table.rows({ page: 'current' }).every(function () {
      var row = this;
      var name = row.data().name;
      var builds = variationsByParent[name] || [];
      $(row.node()).toggleClass('has-variations', builds.length > 0 && showing);
      if (!builds.length || !showing) {
        if (row.child.isShown()) row.child.hide();
        return;
      }
      // A child row is put into the page AS IT IS, and never enters DataTables'
      // own list of rows: nothing maps its cells onto columns and nothing takes a
      // cell out again. So it carries a cell for the DRAWN columns only, which is
      // the opposite of a main row and right for the opposite reason. A main row
      // is handed to DataTables, which reads its cells by position over EVERY
      // column and then removes the hidden ones itself. Here the heading row and
      // the rows around it have already had their hidden cells removed, so a cell
      // for a hidden column would push every later value one heading to the right.
      row.child(sortedBuilds(builds).map(function (v) {
        var $node = rowNode(v, colConfig.filter(function (c) { return columnDrawn(c.target); }));
        return $node.get(0);
      })).show();
    });
  }
  // Smallest build first: the question in front of a list of builds is which one
  // fits the machine you have, and that reads from the top.
  function sortedBuilds(builds) {
    return builds.slice().sort(function (x, y) {
      var a = parseFloat(cellValue(x, 'build_size_bytes'));
      var b = parseFloat(cellValue(y, 'build_size_bytes'));
      if (isNaN(a) && isNaN(b)) return 0;
      if (isNaN(a)) return 1;
      if (isNaN(b)) return -1;
      return a - b;
    });
  }

  fillTable();
  dbStatus('online');

  // --- Saving a cell --------------------------------------------------------

  // One write, one answer. The row is read back afterwards rather than patched
  // here, because a computed column is worked out again by the server and the
  // page has no way to work out what it now says.
  function saveCells(name, cells) {
    return fetch('/api/save', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify([{ name: name, cells: cells }])
    }).then(function (r) {
      return r.text().then(function (text) {
        if (!r.ok) throw new Error(text.trim() || ('status ' + r.status));
        return text ? JSON.parse(text) : {};
      });
    });
  }

  function refreshRow(name) {
    // The name goes as a quoted phrase so characters the search syntax reads
    // (!, AND, OR) cannot change which row comes back. A quote cannot sit in a
    // phrase, so such a name asks for its longest quote-free stretch and the
    // exact row is picked from the answer below.
    var phrase = name.split('"').reduce(function (a, b) { return b.length > a.length ? b : a; }, '');
    return fetch('/api/models?name=' + encodeURIComponent('"' + phrase + '"') + '&include_hidden=1')
      .then(jsonOrThrow)
      .then(function (rows) {
        var fresh = (rows || []).find(function (r) { return r.name === name; });
        if (!fresh) return null;
        rowsByName[name] = fresh;
        var at = topLevelRows.findIndex(function (r) { return r.name === name; });
        if (at >= 0) topLevelRows[at] = fresh;
        Object.keys(variationsByParent).forEach(function (parent) {
          var list = variationsByParent[parent];
          var i = list.findIndex(function (r) { return r.name === name; });
          if (i >= 0) list[i] = fresh;
        });
        // The row is handed its new data and the table builds its cells again.
        // A variation is not one of the table's rows, so nothing is handed over
        // for it: the draw below rebuilds the child rows from the list already
        // updated above.
        var row = tableRow(name);
        if (row) {
          row.data(fresh);
          if (row.node()) markRow(row.index(), fresh);
        }
        table.draw(false);
        return fresh;
      });
  }

  // The table's row for one model, or null. Asked by name rather than found in
  // the page, because the table holds every row and draws only one page.
  function tableRow(name) {
    var found = table.row(function (idx, data) { return data.name === name; });
    return found.any() ? found : null;
  }

  // Write one column's value on one model and show what the database then
  // holds. Resolves true when the write went through and false when it was
  // refused or failed.
  function writeCell(name, column, value) {
    var row = rowsByName[name] || {};
    if (!cellWritable(row, column)) { statusErr(readOnlyReason(row, column)); return Promise.resolve(false); }
    var cells = {};
    cells[column.key] = value;
    statusText('Saving...');
    return saveCells(name, cells)
      .then(function () { return refreshRow(name); })
      .then(function () {
        statusOk('Saved!');
        setTimeout(hideStatus, 1500);
        refreshCatalogEmptiness();
        return true;
      })
      .catch(function (e) {
        statusErr('Not saved: ' + (e.message || e));
        return refreshRow(name).then(function () { return false; });
      });
  }

  // --- Editing a cell in place ----------------------------------------------

  // What the editor starts with, in the units the cell shows: a price is typed
  // per million because that is what the cell paints, and stored per token.
  function editorStartValue(row, column) {
    var stored = cellValue(row, column.key);
    if (String(stored).trim() === '') return '';
    if (isPriceColumn(column)) {
      var shown = shownPrice(stored, priceUnitOf(row));
      return shown === null ? '' : String(shown);
    }
    if (column.value_type === 'list') return listText(stored);
    return String(stored);
  }

  function storedFromEditor(row, column, typed) {
    var raw = String(typed == null ? '' : typed).trim();
    if (raw === '') return '';
    if (isPriceColumn(column)) {
      var n = parseFloat(raw);
      if (!isFinite(n)) return null;
      return fixedNumberString(isPerMillionUnit(priceUnitOf(row)) ? n / 1000000 : n);
    }
    if (column.value_type === 'number' && !isFinite(parseFloat(raw))) return null;
    return raw;
  }

  var openEditor = null;

  function openCellEditor(el) {
    closeCellEditor(false);
    var $display = $(el);
    var key = $display.attr('data-key');
    var column = columnByKey[key];
    if (!column) return;
    var $tr = $display.closest('tr');
    var name = $tr.attr('data-name') || '';
    var row = rowsByName[name] || {};
    var start = editorStartValue(row, column);

    var $input;
    if (column.value_type === 'flag') {
      $input = $('<select class="cell-input"></select>')
        .append('<option value="">-</option>')
        .append('<option value="yes">yes</option>')
        .append('<option value="no">no</option>')
        .val(String(start).trim() === '' ? '' : (isYes(start) ? 'yes' : 'no'));
    } else if (column.value_type === 'choice') {
      $input = $('<select class="cell-input"></select>').append('<option value="">-</option>');
      (column.options || []).forEach(function (o) {
        $input.append($('<option></option>').attr('value', o).text(o));
      });
      $input.val(start);
    } else if (column.value_type === 'number') {
      $input = $('<input type="number" step="any" inputmode="decimal" class="cell-input">').val(start);
    } else {
      $input = $('<textarea class="cell-input cc-text-area"></textarea>').val(start);
    }
    $input.attr('aria-label', 'Edit ' + column.label + ' for ' + name);
    $display.replaceWith($input);
    openEditor = { key: key, name: name, before: start };
    $input.trigger('focus');
    var el0 = $input.get(0);
    if (el0 && el0.select) { try { el0.select(); } catch (e) { /* a select element cannot */ } }
  }

  function closeCellEditor(commit) {
    if (!openEditor) return;
    var state = openEditor;
    openEditor = null;
    var $input = $('#models .cell-input');
    if (!$input.length) return;
    var typed = $input.val();
    var column = columnByKey[state.key];
    var row = rowsByName[state.name] || {};
    $input.remove();
    if (!commit || String(typed) === String(state.before)) { refreshRow(state.name); return; }
    var stored = storedFromEditor(row, column, typed);
    if (stored === null) {
      statusErr(column.label + ' for "' + state.name + '" was not changed: "' + typed + '" is not a number.');
      refreshRow(state.name);
      return;
    }
    writeCell(state.name, column, stored).then(function (written) {
      if (written) editHistory.record(state.name, state.key, state.before, typed);
    });
  }

  $('#models').on('click', '.cell-edit', function (e) { e.stopPropagation(); openCellEditor(this); });
  // A real button answers Enter and Space by firing a click, so the keyboard
  // needs nothing of its own here.
  $('#models').on('click', '.cell-add', function (e) { e.stopPropagation(); openCellEditor(this); });
  $('#models').on('keydown', '.cell-edit', function (e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault(); e.stopPropagation();
    openCellEditor(this);
  });
  // A cell that cannot be typed into SAYS SO when it is pressed, rather than
  // doing nothing: a control that swallows a keystroke in silence reads as
  // broken, and the reason is the useful half of the answer. statusWarn, not
  // statusErr: nothing was attempted and failed, so this reaches the polite
  // #status-announce region rather than the assertive one an actual error
  // interrupts with.
  $('#models').on('click', 'td[data-readonly="1"]', function () {
    var column = columnByKey[$(this).attr('data-col')];
    var row = rowsByName[$(this).closest('tr').attr('data-name') || ''] || {};
    if (column) statusWarn(readOnlyReason(row, column));
  });
  $('#models').on('blur', '.cell-input', function () { closeCellEditor(true); });
  $('#models').on('keydown', '.cell-input', function (e) {
    if (e.key === 'Escape') { e.preventDefault(); closeCellEditor(false); return; }
    if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); closeCellEditor(true); }
  });
  $('#models').on('change', 'select.cell-input', function () { closeCellEditor(true); });

  // --- Undo and redo, for this session only ---------------------------------
  var editHistory = (function () {
    var undoStack = [], redoStack = [], applying = false;
    function apply(rec, value) {
      var column = columnByKey[rec.key];
      var row = rowsByName[rec.name];
      if (!column || !row) return;
      var stored = storedFromEditor(row, column, value);
      if (stored === null) return;
      writeCell(rec.name, column, stored);
    }
    function buttons() {
      $('#btn-undo').prop('disabled', undoStack.length === 0);
      $('#btn-redo').prop('disabled', redoStack.length === 0);
    }
    return {
      record: function (name, key, before, after) {
        if (applying || String(before) === String(after)) return;
        undoStack.push({ name: name, key: key, before: before, after: after });
        redoStack.length = 0;
        buttons();
      },
      undo: function () {
        var rec = undoStack.pop();
        if (!rec) return;
        applying = true;
        try { apply(rec, rec.before); } finally { applying = false; }
        redoStack.push(rec);
        buttons();
      },
      redo: function () {
        var rec = redoStack.pop();
        if (!rec) return;
        applying = true;
        try { apply(rec, rec.after); } finally { applying = false; }
        undoStack.push(rec);
        buttons();
      },
      refresh: buttons
    };
  })();
  $('#btn-undo').on('click', function () { editHistory.undo(); });
  $('#btn-redo').on('click', function () { editHistory.redo(); });
  editHistory.refresh();

  // --- The filter bar -------------------------------------------------------

  function cssId(key) { return String(key).replace(/[^a-zA-Z0-9]/g, '_'); }

  // The id of one of a date column's two pickers; which is 'from' or 'to'.
  function dateControlId(key, which) { return 'flt_' + cssId(key) + '_' + which; }

  // A date column's filter: a From and a To picker, either one optional. The
  // browser's own date input is used, so its value is always YYYY-MM-DD or
  // empty, which is the only shape the server takes.
  function dateFilterControl(column) {
    var $wrap = $('<div class="flt-control flt-date"></div>').attr('data-key', column.key);
    $wrap.append(filterLabel(column));
    var $rows = $('<div class="flt-date-rows"></div>');
    ['from', 'to'].forEach(function (which) {
      var word = which === 'from' ? 'From' : 'To';
      var id = dateControlId(column.key, which);
      $rows.append($('<label class="flt-date-row"></label>')
        .append($('<span class="flt-date-word"></span>').text(word))
        .append($('<input type="date" class="filter-input flt-date-input">')
          .attr('id', id).attr('name', id).attr('autocomplete', 'off')
          .attr('aria-label', column.label + ', ' + word.toLowerCase() + ' this day')));
    });
    $wrap.append($rows);
    return $wrap;
  }

  // A filter's title, in the left column of its row. The full label is its
  // tooltip because a long one is cut with an ellipsis. A control that is a
  // real field passes its id and the title becomes its <label for>; the value
  // list and the date pickers name their own controls, so theirs is a span.
  function filterLabel(column, forId) {
    var $label = forId
      ? $('<label class="flt-label"></label>').attr('for', forId)
      : $('<span class="flt-label"></span>');
    return $label.text(column.label).attr('title', column.label);
  }

  // The Hide 0 tick a price column's filter carries. It belongs to its own
  // column alone: ticking one never ticks the other.
  function hideFreeTick(columnKey) {
    var tickId = 'flt_hide_free_' + columnKey;
    return $('<label class="flt-hide-free-row"></label>')
      .attr('title', 'Hide the models whose price in this column is 0, and the models OpenRouter lists as free variants (an id ending :free or a name ending (free)). Video and image models often show 0 because OpenRouter does not price them per token, so they go too.')
      .append($('<input type="checkbox" class="flt-hide-free">')
        .attr('id', tickId).attr('name', tickId).attr('autocomplete', 'off'))
      .append($('<span></span>').text('Hide 0'));
  }

  // One control per VISIBLE column, built from what that column holds: a flag
  // offers yes and no, a choice offers its own labels, a number takes a figure
  // and a text column takes a few letters to look for. The controls are grouped
  // into the same groups the add/edit form uses, so what a column is ABOUT
  // decides its section, not who can answer it.
  // Open or collapsed is kept for this visit only: a section somebody closed to
  // get past it should not stay buried on every later load.
  var filterSectionOpen = {};

  function filterControlFor(column) {
    // A choice or a list column offers the values that are really there, as a
    // three-state list; a text column starts as a box
    // and upgradeTextControls decides from the data whether the list takes
    // its place. A flag keeps All/Yes/No (a list of two is a dropdown), and a
    // number keeps its figure box (a list of every distinct number is not a
    // filter anybody wants).
    if (column.date) return dateFilterControl(column);
    if (column.value_type === 'choice' || column.value_type === 'list') {
      var $vlfWrap = $('<div class="flt-control"></div>').attr('data-key', column.key);
      $vlfWrap.append(filterLabel(column));
      attachVlf($vlfWrap, column);
      return $vlfWrap;
    }
    var id = 'flt_' + cssId(column.key);
    var $control;
    if (column.value_type === 'flag') {
      $control = $('<select class="filter-select-sm"></select>')
        .append('<option value="">All</option>')
        .append('<option value="yes">Yes</option>')
        .append('<option value="no">No</option>')
        .append('<option value="any:">Empty</option>');
    } else if (column.value_type === 'number') {
      $control = $('<input type="number" step="any" class="filter-input w-130">')
        .attr('placeholder', (isPriceColumn(column) ? 'Max' : 'Min') + '...');
    } else {
      $control = $('<input type="text" class="filter-input">').attr('placeholder', 'Contains... (!no, "a phrase", OR)');
    }
    $control.attr('id', id).attr('name', id).attr('autocomplete', 'off')
      .attr('data-key', column.key).attr('title', column.label);
    var $wrap = $('<div class="flt-control"></div>').attr('data-key', column.key);
    $wrap.append(filterLabel(column, id)).append($control);
    if (HIDE_FREE_COLUMNS.indexOf(column.key) !== -1) $wrap.append(hideFreeTick(column.key));
    return $wrap;
  }

  // The Links filter: the same button-over-a-panel as a value list, drawn over
  // the two fixed rows instead of a column's values.
  function linksFilterControl() {
    var $wrap = $('<div class="flt-control"></div>').attr('data-key', LINKS_FILTER.key);
    $wrap.append(filterLabel(LINKS_FILTER));
    attachVlf($wrap, LINKS_FILTER);
    return $wrap;
  }

  // --- The value-list filter --------------------------------------------------
  // A button naming what is chosen, over a panel listing the values the column
  // really holds (GET /api/columns/values), each row a THREE-STATE control:
  // off, include (+), exclude (-). Three signals per state and none of them is
  // colour: the glyph in the box, the sign before the value, and the
  // strikethrough on an excluded one. Each row is an ordinary button whose
  // accessible name is rewritten on every change - never role=checkbox with
  // aria-checked=mixed, which a screen reader would announce as "partially
  // checked" for a value the reader deliberately excluded.

  var VLF_THRESHOLD = 200;

  var VLF_BOXES = {
    off: '<svg class="vlf-box-ico" viewBox="0 0 16 16" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="1.6"><rect x="2" y="2" width="12" height="12" rx="2.5"/></svg>',
    include: '<svg class="vlf-box-ico" viewBox="0 0 16 16" aria-hidden="true" focusable="false"><rect x="2" y="2" width="12" height="12" rx="2.5" fill="currentColor"/><path d="M5 8.4l2.2 2.2 4-4.8" fill="none" stroke="#fff" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    exclude: '<svg class="vlf-box-ico" viewBox="0 0 16 16" aria-hidden="true" focusable="false"><rect x="2" y="2" width="12" height="12" rx="2.5" fill="currentColor"/><path d="M5.4 5.4l5.2 5.2M10.6 5.4l-5.2 5.2" fill="none" stroke="#fff" stroke-width="1.8" stroke-linecap="round"/></svg>'
  };
  var VLF_STATE_WORD = { off: 'not filtered', include: 'included', exclude: 'excluded' };
  var VLF_SIGN = { off: '', include: '+', exclude: '-' };
  var VLF_CYCLE = { off: 'include', include: 'exclude', exclude: 'off' };

  function vlfShownName(value, column) {
    if (value === '') return 'Empty';
    var link = column && column.key === LINKS_FILTER.key ? linkRowFor(value) : null;
    return link ? link.name : value;
  }

  function attachVlf($wrap, column) {
    if ($wrap.find('.vlf-btn').length) return;
    $wrap.addClass('vlf');
    $wrap.append($('<button type="button" class="vlf-btn"></button>')
      .attr('aria-expanded', 'false'));
    $wrap.append($('<div class="vlf-panel" hidden></div>'));
    updateVlfButton($wrap, column);
  }

  function updateVlfButton($wrap, column) {
    var $btn = $wrap.find('.vlf-btn');
    if (!$btn.length) return;
    var chosen = vlfChosen(column.key);
    var parts = Object.keys(chosen).map(function (value) {
      return (chosen[value] === 'include' ? '+' : '-') + vlfShownName(value, column);
    });
    $btn.text(parts.length
      ? parts.slice(0, 3).join(' ') + (parts.length > 3 ? ' and ' + (parts.length - 3) + ' more' : '')
      : 'All');
    $btn.attr('aria-label', column.label + ' filter: ' +
      (parts.length ? parts.join(', ') : 'no values chosen'));
  }

  function closeVlfPanels(except) {
    var $panels = $('#col-filters .vlf-panel');
    if (except) $panels = $panels.not(except);
    $panels.each(function () {
      $(this).prop('hidden', true)
        .closest('.vlf').find('.vlf-btn').attr('aria-expanded', 'false');
    });
  }

  function openVlfPanel($wrap, column, focusSearch) {
    var $panel = $wrap.find('.vlf-panel');
    closeVlfPanels($panel);
    if (!$panel.data('built')) buildVlfPanel($wrap, column);
    $panel.prop('hidden', false);
    $wrap.find('.vlf-btn').attr('aria-expanded', 'true');
    if (focusSearch) $panel.find('.vlf-search').trigger('focus');
  }

  function buildVlfPanel($wrap, column) {
    var $panel = $wrap.find('.vlf-panel').data('built', true).empty();
    var isLinks = column.key === LINKS_FILTER.key;
    // The Links filter is two fixed rows: nothing to search and nothing to
    // include in bulk.
    if (!isLinks) {
      $panel.append($('<input type="search" class="vlf-search filter-input">')
        .attr('placeholder', 'Search these values... (!no, "a phrase", OR)')
        .attr('aria-label', 'Search the values of ' + column.label));
    }
    $panel.append($('<p class="vlf-legend">Click a value to cycle: off, include (+), exclude (-).</p>'));
    // All/None act on the rows the panel is showing right now, never on a
    // value the search or the server's cap left off screen - the same
    // "listed" set drawVlfRows just drew, read straight back out of the DOM
    // rather than re-fetched, so a server-mode column never fetches more.
    if (!isLinks) {
      $panel.append($('<div class="vlf-bulk"></div>')
        .append($('<button type="button" class="btn-clear vlf-bulk-all"></button>').text('All')
          .attr('aria-label', 'Include every listed value of ' + column.label))
        .append($('<button type="button" class="btn-clear vlf-bulk-none"></button>').text('None')
          .attr('aria-label', 'Clear every listed value of ' + column.label)));
    }
    $panel.append($('<div class="vlf-list" role="group"></div>')
      .attr('aria-label', 'Values of ' + column.label)
      .append($('<p class="vlf-empty">Reading the values...</p>')));
    $panel.append($('<button type="button" class="btn-clear vlf-clear"></button>')
      .text(isLinks ? 'Clear Links' : 'Clear this column'));
    if (isLinks) linksFetch($wrap, column);
    else vlfFetch($wrap, column, '');
  }

  // The values come from the server, over the WHOLE catalog. Below the
  // truncation cap the search box narrows the list already in hand; above it,
  // it asks the server again with contains=, so a value can still be found in
  // a column of thousands.
  function vlfFetch($wrap, column, contains) {
    var seq = ($wrap.data('vlfSeq') || 0) + 1;
    $wrap.data('vlfSeq', seq);
    fetch('/api/columns/values?key=' + encodeURIComponent(column.key) +
      (contains ? '&contains=' + encodeURIComponent(contains) : ''))
      .then(jsonOrThrow)
      .then(function (answer) {
        if ($wrap.data('vlfSeq') !== seq) return;
        // Only the unnarrowed answer decides the mode. A narrowed answer is
        // short by construction; letting it switch the panel to browser mode
        // would make every later search filter that short list instead of
        // asking the server.
        if (!contains) $wrap.data('vlfServerMode', answer.truncated || answer.distinct_total > VLF_THRESHOLD);
        $wrap.data('vlfValues', answer.values || []);
        $wrap.data('vlfEmpty', typeof answer.empty === 'number' ? answer.empty : null);
        drawVlfRows($wrap, column);
      })
      .catch(function (e) {
        if ($wrap.data('vlfSeq') !== seq) return;
        $wrap.find('.vlf-list').empty()
          .append($('<p class="vlf-empty"></p>').text('The values could not be read: ' + (e.message || e)));
      });
  }

  // The Links rows' counts: the models the catalog holds less the models with
  // no value in the column behind each link, so a count is exactly what the
  // filter returns for that row.
  function linksFetch($wrap, column) {
    var seq = ($wrap.data('vlfSeq') || 0) + 1;
    $wrap.data('vlfSeq', seq);
    Promise.all([
      fetch('/api/columns/answers').then(jsonOrThrow),
      Promise.all(LINK_ROWS.map(function (row) {
        return fetch('/api/columns/values?key=' + encodeURIComponent(row.column) + '&limit=1').then(jsonOrThrow);
      }))
    ]).then(function (got) {
      if ($wrap.data('vlfSeq') !== seq) return;
      var counts = {};
      LINK_ROWS.forEach(function (row, i) { counts[row.value] = got[0].models - got[1][i].empty; });
      $wrap.data('vlfLinkCounts', counts);
      drawVlfRows($wrap, column);
    }).catch(function (e) {
      if ($wrap.data('vlfSeq') !== seq) return;
      $wrap.find('.vlf-list').empty()
        .append($('<p class="vlf-empty"></p>').text('The links could not be counted: ' + (e.message || e)));
    });
  }

  function drawVlfRows($wrap, column) {
    var $list = $wrap.find('.vlf-list').empty();
    if (column.key === LINKS_FILTER.key) {
      var linkCounts = $wrap.data('vlfLinkCounts') || {};
      LINK_ROWS.forEach(function (row) {
        $list.append(vlfRowNode(column, row.value, linkCounts[row.value]));
      });
      return;
    }
    var serverMode = !!$wrap.data('vlfServerMode');
    var query = parseSearch(String($wrap.find('.vlf-search').val() || ''));
    var values = ($wrap.data('vlfValues') || []).slice();
    var listed = {};
    values.forEach(function (v) { listed[v.value] = true; });
    // A value already chosen stays on the screen even when the search or the
    // server's cap left it out of the answer, or it could not be cleared.
    Object.keys(vlfChosen(column.key)).sort().reverse().forEach(function (value) {
      if (value !== '' && !listed[value]) values.unshift({ value: value, models: null });
    });
    // The Empty row, always first and never narrowed by the search: included,
    // it keeps the models holding nothing; excluded, it keeps only the models
    // that state something. Its count comes with the values.
    $list.append(vlfRowNode(column, '', $wrap.data('vlfEmpty')));
    values.forEach(function (v) {
      if (!serverMode && !searchMatches(query, [String(v.value)])) return;
      $list.append(vlfRowNode(column, v.value, v.models));
    });
  }

  function vlfRowNode(column, value, count) {
    var $row = $('<button type="button" class="vlf-row"></button>').attr('data-value', value);
    $row.append($('<span class="vlf-box"></span>'));
    $row.append($('<span class="vlf-sign" aria-hidden="true"></span>'));
    var $value = $('<span class="vlf-value"></span>').text(vlfShownName(value, column));
    if (value === '') $value.addClass('vlf-none');
    $row.append($value);
    if (count !== null && count !== undefined) {
      var what = value === '' ? 'hold no value' : column.key === LINKS_FILTER.key ? 'have this link' : 'hold this value';
      $row.append($('<span class="vlf-count"></span>').text(count)
        .attr('title', count + ' model' + (count === 1 ? '' : 's') + ' ' + what));
    }
    paintVlfRow($row, column, value);
    return $row;
  }

  // The row NODE is kept and repainted on a change, never replaced, so the
  // keyboard focus cycling it stays where it is.
  function paintVlfRow($row, column, value) {
    var state = vlfState(column.key, value);
    $row.attr('data-state', state)
      .attr('aria-label', vlfShownName(value, column) + ', ' + VLF_STATE_WORD[state]);
    $row.find('.vlf-box').html(VLF_BOXES[state]);
    $row.find('.vlf-sign').text(VLF_SIGN[state]);
  }

  // A text column is upgraded FROM THE DATA: few enough distinct values and
  // the box becomes a value list; more than that (or a contains: already
  // typed, which a list cannot hold) and the box stays, with a link offering
  // the list anyway. Runs after restoreFilters, so a column whose address
  // already carries any:/not: gets its list without asking the server first.
  function upgradeTextControls() {
    $('#col-filters .flt-control').each(function () {
      var $wrap = $(this);
      var column = vlfColumn($wrap.attr('data-key'));
      if (!column || column.value_type !== 'text' || column.date) return;
      if ($wrap.find('.vlf-btn').length || $wrap.find('.vlf-anyway').length) return;
      if (Object.keys(vlfChosen(column.key)).length) { swapToValueList($wrap, column); return; }
      fetch('/api/columns/values?key=' + encodeURIComponent(column.key) + '&limit=1')
        .then(jsonOrThrow)
        .then(function (answer) {
          var typed = String($wrap.find('input').val() || '').trim();
          if (answer.distinct_total <= VLF_THRESHOLD && !typed) swapToValueList($wrap, column);
          else addAnywayLink($wrap, column, answer.distinct_total);
        })
        .catch(function () { /* the box it has today stays */ });
    });
  }

  function swapToValueList($wrap, column) {
    $wrap.empty();
    $wrap.append(filterLabel(column));
    attachVlf($wrap, column);
    markEmptyFilter($wrap);
  }

  function addAnywayLink($wrap, column, total) {
    $wrap.append($('<button type="button" class="btn-clear vlf-anyway"></button>')
      .text('Show the ' + total + ' values anyway'));
    markEmptyFilter($wrap);
  }

  $('#col-filters').on('click', '.vlf-anyway', function () {
    var $wrap = $(this).closest('.flt-control');
    var column = vlfColumn($wrap.attr('data-key'));
    if (!column) return;
    $(this).remove();
    attachVlf($wrap, column);
    markEmptyFilter($wrap);
    openVlfPanel($wrap, column, true);
  });

  $('#col-filters').on('click', '.vlf-btn', function () {
    var $wrap = $(this).closest('.flt-control');
    var column = vlfColumn($wrap.attr('data-key'));
    if (!column) return;
    if ($wrap.find('.vlf-panel').prop('hidden')) openVlfPanel($wrap, column, false);
    else {
      $wrap.find('.vlf-panel').prop('hidden', true);
      $(this).attr('aria-expanded', 'false');
    }
  });

  // Click, Space and Enter all land here: a real button fires click for all
  // three. The change is said out loud through the polite region, because the
  // repainted glyphs say nothing to a screen reader.
  $('#col-filters').on('click', '.vlf-row', function () {
    var $wrap = $(this).closest('.flt-control');
    var column = vlfColumn($wrap.attr('data-key'));
    if (!column) return;
    var value = $(this).attr('data-value');
    var next = VLF_CYCLE[vlfState(column.key, value)];
    setVlfState(column.key, value, next);
    paintVlfRow($(this), column, value);
    updateVlfButton($wrap, column);
    announce('status-announce', vlfShownName(value, column) + ', ' + VLF_STATE_WORD[next]);
    // A row's own state change is a "filter set" for #filter-feedback whatever
    // it cycled TO - off included, since a value-list control can hold several
    // conditions and one row going off does not mean the column is empty (that
    // is .vlf-clear below). Names the column only, never the value.
    pendingFilterFeedback = { kind: 'set', label: column.label };
    applyFilters(true);
  });

  var vlfSearchTimer = null;
  $('#col-filters').on('input', '.vlf-search', function () {
    var $wrap = $(this).closest('.flt-control');
    var column = vlfColumn($wrap.attr('data-key'));
    if (!column) return;
    if ($wrap.data('vlfServerMode')) {
      clearTimeout(vlfSearchTimer);
      var contains = String($(this).val() || '').trim();
      vlfSearchTimer = setTimeout(function () { vlfFetch($wrap, column, contains); }, 300);
      return;
    }
    drawVlfRows($wrap, column);
  });

  // All: every value the panel currently lists, the Empty row
  // excluded, goes to include. The row itself carries the value (data-value
  // on the button), so this never re-reads vlfValues and never fetches -
  // the "listed" rows are exactly what is already in the DOM.
  $('#col-filters').on('click', '.vlf-bulk-all', function () {
    var $wrap = $(this).closest('.flt-control');
    var column = vlfColumn($wrap.attr('data-key'));
    if (!column) return;
    $wrap.find('.vlf-list .vlf-row').each(function () {
      var value = $(this).attr('data-value');
      if (value === '') return;
      setVlfState(column.key, value, 'include');
      paintVlfRow($(this), column, value);
    });
    updateVlfButton($wrap, column);
    announce('status-announce', column.label + ' filter: every listed value included');
    pendingFilterFeedback = { kind: 'set', label: column.label };
    applyFilters(true);
  });

  // None: every listed value, Empty included, goes to off.
  $('#col-filters').on('click', '.vlf-bulk-none', function () {
    var $wrap = $(this).closest('.flt-control');
    var column = vlfColumn($wrap.attr('data-key'));
    if (!column) return;
    $wrap.find('.vlf-list .vlf-row').each(function () {
      var value = $(this).attr('data-value');
      setVlfState(column.key, value, 'off');
      paintVlfRow($(this), column, value);
    });
    updateVlfButton($wrap, column);
    announce('status-announce', column.label + ' filter: every listed value cleared');
    // Every LISTED row goes off, not necessarily every condition the column
    // holds - a search can narrow what is listed - so this reads as "set"
    // rather than the whole-control "cleared" .vlf-clear below produces.
    pendingFilterFeedback = { kind: 'set', label: column.label };
    applyFilters(true);
  });

  $('#col-filters').on('click', '.vlf-clear', function () {
    var $wrap = $(this).closest('.flt-control');
    var column = vlfColumn($wrap.attr('data-key'));
    if (!column) return;
    delete valueFilters[column.key];
    drawVlfRows($wrap, column);
    updateVlfButton($wrap, column);
    announce('status-announce', column.label + ' filter cleared');
    // The control's own value emptied entirely, unlike the row/bulk actions
    // above - this is the "cleared" wording, naming the column.
    pendingFilterFeedback = { kind: 'cleared', label: column.label };
    applyFilters(true);
  });

  $('#col-filters').on('keydown', '.vlf', function (e) {
    if (e.key !== 'Escape') return;
    var $panel = $(this).find('.vlf-panel');
    if ($panel.prop('hidden')) return;
    e.preventDefault();
    e.stopPropagation();
    $panel.prop('hidden', true);
    $(this).find('.vlf-btn').attr('aria-expanded', 'false').trigger('focus');
  });

  // A click anywhere else closes an open panel, like every other transient
  // panel on the page. A target no longer in the document was removed by its
  // own handler (the "Show the N values anyway" link is), so its position
  // says nothing - it never closes a panel.
  $(document).on('click', function (e) {
    if (!document.contains(e.target)) return;
    if ($(e.target).closest('.vlf').length) return;
    closeVlfPanels();
  });

  function buildFilterControls() {
    var $into = $('#col-filters').empty();
    DOMAIN_SECTIONS.forEach(function (domain) {
      var inSection = colConfig.filter(function (c) {
        return c.column && !filterOffTargets[c.target] && domainOf(c.column) === domain;
      });
      // The Links filter has no column, so it sits in "Where it lives" whether
      // or not that section holds a column filter.
      if (!inSection.length && domain !== 'location') return;
      var open = filterSectionOpen[domain] !== false;
      var bodyId = 'flt_sec_' + domain;
      // A real button with aria-expanded, never a bare <details>: closing a
      // details hides every child, and a SET filter must survive the close.
      var $head = $('<button type="button" class="flt-section-head"></button>')
        .attr('aria-expanded', open ? 'true' : 'false')
        .attr('aria-controls', bodyId)
        .append($('<span class="flt-section-name"></span>').text(DOMAIN_SECTION_NAME[domain] || domain))
        .append($('<span class="flt-section-count" hidden></span>')
          .attr('title', 'Filters set in this section'))
        .append($('<span class="flt-section-arrow" aria-hidden="true"></span>'));
      var $body = $('<div class="flt-section-body"></div>').attr('id', bodyId);
      if (domain === 'location') $body.append(linksFilterControl());
      inSection.forEach(function (c) { $body.append(filterControlFor(c.column)); });
      $into.append($('<div class="flt-section"></div>')
        .attr('data-ns', domain)
        .attr('data-open', open ? 'true' : 'false')
        .append($head).append($body));
    });
    updateFilterMarks();
    markEmptyFilters();
  }

  // --- A filter for a column nothing has ever stated ---------------------------
  // A control that can only ever answer "no models" is worth saying so about
  // before it is used. The question is about the WHOLE catalog, not the rows a
  // filter left on screen, so it is read off the rows the page already holds
  // whenever those ARE the whole catalog - the usual case - and off one plain
  // read of the catalog when a filter narrowed them. Never off
  // /api/columns/values: that answers one column at a time, and this asks about
  // every column at once.

  // How many rows of the catalog have an answer in each column. A count and not
  // a yes-or-no, because the Columns dialog prints the figure on every row;
  // markEmptyFilter reads the same value as a truth, and 0 and undefined are
  // both falsy.
  var columnHasValues = {};
  var catalogEmptinessRead = false;
  // Whether the read that produced those counts saw no models at all. Read off
  // the rows that were counted rather than off the page, because a filter can
  // leave the page empty while the catalog is not.
  var catalogHasNoModels = false;
  // A read that failed, so a reader is told that rather than left wondering why
  // nothing is counted.
  var catalogEmptinessFailed = false;

  function noteColumnAnswers(answer) {
    columnHasValues = (answer && answer.answers) || {};
    catalogHasNoModels = !(answer && answer.models > 0);
    if (answer && typeof answer.top_level === 'number') catalogRowCount = answer.top_level;
    catalogEmptinessRead = true;
    catalogEmptinessFailed = false;
    markEmptyFilters();
    redrawChooserIfOpen();
    if (table) {
      updateModelCount();
      if (rowsAreFiltered) $('#empty-catalog').attr('hidden', catalogHasNoModels ? null : '');
    }
  }

  // The Columns dialog prints these figures on every row, so a read landing
  // while it is open is drawn rather than waited for.
  function redrawChooserIfOpen() {
    if (columnsDialogOpen()) buildColumnChooser();
  }

  // Marks ONE control. Anything that rewrites a control's insides calls this
  // again afterwards, because the note is a child of the control and goes with
  // them - and it is appended last, so it reads under whatever the control
  // grew into.
  function markEmptyFilter($wrap) {
    // The Links filter is not a column, so it never has "no values" to note.
    if (!catalogEmptinessRead || $wrap.attr('data-key') === LINKS_FILTER.key) return;
    var none = !columnHasValues[$wrap.attr('data-key')];
    $wrap.toggleClass('flt-no-values', none);
    $wrap.find('.flt-none-note').remove();
    if (none) $wrap.append($('<span class="flt-none-note"></span>').text('no values'));
  }

  function markEmptyFilters() {
    if (!catalogEmptinessRead) return;
    $('#col-filters .flt-control').each(function () { markEmptyFilter($(this)); });
  }

  // A sequence number, the same pattern applySeq guards applyFilters with: a
  // second name search started before the first one's suggestions come back
  // must not have the first one's answer land after it and overwrite the
  // panel with a stale name.
  var didYouMeanSeq = 0;

  // maybeShowDidYouMean is called after every fetch applyFilters makes. It
  // asks GET /api/models/similar only when this search found NOTHING and the
  // search box actually asked for something - an empty box finding nothing is
  // an empty catalog, not a typo, and #empty-catalog already speaks to that.
  function maybeShowDidYouMean(params, rows) {
    var seq = ++didYouMeanSeq;
    if (rows.length > 0 || !params.name || searchUsesSyntax(params.name)) {
      $('#did-you-mean').attr('hidden', '');
      return;
    }
    fetch('/api/models/similar?name=' + encodeURIComponent(params.name))
      .then(jsonOrThrow)
      .then(function (suggestions) {
        if (seq !== didYouMeanSeq) return;
        renderDidYouMean(suggestions);
      })
      .catch(function () {
        if (seq !== didYouMeanSeq) return;
        $('#did-you-mean').attr('hidden', '');
      });
  }

  // renderDidYouMean draws the suggestion list and shows the panel, or hides
  // it when nothing came back close enough to offer. Each suggestion is a
  // button that puts that name into the search box and searches again -
  // clicking it is the whole interaction, so nobody has to retype what was
  // only misspelt.
  function renderDidYouMean(suggestions) {
    var $list = $('#did-you-mean-list').empty();
    if (!suggestions || !suggestions.length) {
      $('#did-you-mean').attr('hidden', '');
      return;
    }
    suggestions.forEach(function (s) {
      $('<button type="button" class="did-you-mean-item"></button>')
        .text(s.name)
        .on('click', function () {
          $('#f_search').val(s.name);
          applyFilters(true);
        })
        .appendTo($list);
    });
    $('#did-you-mean').attr('hidden', null);
  }

  // The counts are asked of the server, whatever the filters are. They are a
  // fact about the CATALOG and not about the page, so the rows the reader is
  // looking at can never answer them.
  function refreshCatalogEmptiness() {
    fetch('/api/columns/answers').then(jsonOrThrow).then(noteColumnAnswers)
      .catch(function () {
        // Nothing is marked rather than marked wrongly, and the dialog says so
        // rather than printing a count of nothing.
        catalogEmptinessFailed = true;
        redrawChooserIfOpen();
      });
  }

  $('#col-filters').on('click', '.flt-section-head', function () {
    var $section = $(this).closest('.flt-section');
    var open = $section.attr('data-open') !== 'true';
    $section.attr('data-open', open ? 'true' : 'false');
    $(this).attr('aria-expanded', open ? 'true' : 'false');
    filterSectionOpen[$section.attr('data-ns')] = open;
  });

  // Which controls hold a non-default value, read off the query itself:
  // buildQuery names exactly the controls that contribute a parameter, so the
  // marks can never disagree with the address bar. The .is-set class is what
  // lets a set filter survive its section collapsing (the collapsed section's
  // CSS hides everything without it), each section header counts its own, and
  // the top bar funnel's badge counts them all.
  function updateFilterMarks() {
    var params = buildQuery();
    // Each Hide 0 tick is a filter of its own on its own column: it marks that
    // column's control, so it survives a collapsed section, and counts once
    // beside the column's own condition.
    var freeTicks = String(params.hide_free || '').split(',');
    function freeSet(key) { return freeTicks.indexOf(key) !== -1; }
    $('#col-filters .flt-control').each(function () {
      var key = $(this).attr('data-key');
      $(this).toggleClass('is-set', params[key] !== undefined || freeSet(key));
    });
    $('#col-filters .flt-section').each(function () {
      var n = 0;
      $(this).find('.flt-control').each(function () {
        var key = $(this).attr('data-key');
        if (params[key] !== undefined) n++;
        if (freeSet(key)) n++;
      });
      $(this).find('.flt-section-count').text(n).attr('hidden', n ? null : '');
    });
    // Hidden, Variations and Edited by me sit outside the column sections and
    // are part of the remembered view, so one can be set on a bare address. Each
    // counted one is marked on its own label, so the badge always points at
    // something the panel shows as set.
    var total = Object.keys(params).length;
    if (params.hide_free !== undefined) total += freeTicks.length - 1;
    ['#f_show_hidden', '#f_variations', '#f_edited_by_me'].forEach(function (sel) {
      var set = !!$(sel).val();
      $(sel).closest('label').toggleClass('is-set', set);
      if (set) total++;
    });
    // Two badges, one count: the funnel carries it on a wide screen and the sort
    // bar's Filters button below the card breakpoint, where the funnel is not drawn. The
    // badge's own digits say nothing to a screen reader, so the Filters button
    // carries the count in its accessible name as well.
    $('#filters-count, #filters-count-m').text(total).attr('hidden', total ? null : '');
    $('#btn-filters').attr('aria-label', total ? 'Filters, ' + total + ' set' : 'Filters');
  }

  // The panel DOCKS on a wide screen - the table shrinks beside it - and
  // overlays on a narrow one. Only the wide state is remembered (filters_open):
  // a drawer that reopens itself on a phone steals the screen, so the drawer
  // always starts closed.
  function applyFiltersOpen() {
    $('body').toggleClass('filters-collapsed', !filtersOpen);
    syncFilterFunnel();
    // The table area's width changes with the dock, so everything sized to it
    // is measured again once the new layout has painted: the column widths,
    // the pinned heading's clone, and the top scroll strip's spacer.
    window.requestAnimationFrame(function () {
      if (!table) return;
      table.columns.adjust();
      applyFixedHeader();
      table.fixedHeader.adjust();
      updateTopScrollWidth();
      var $c = tableContainer();
      if ($c.length) mirrorPanelScroll($c.scrollLeft());
    });
  }
  function setFiltersOpen(open) {
    filtersOpen = open;
    applyFiltersOpen();
    saveSettings();
  }
  // The top bar's funnel toggles whichever form the panel takes at this width:
  // the docked panel (remembered in filters_open) on a wide screen, the overlay
  // drawer (never remembered) on a narrow one. Its aria-expanded describes the
  // form in force right now, so it is re-read on every change of either state
  // and when the width crosses the breakpoint.
  function filterPanelShown() {
    return cardsQuery.matches ? $('body').hasClass('filters-drawer-open') : filtersOpen;
  }
  function syncFilterFunnel() {
    // Both buttons open the same panel and only one of them is drawn at a given
    // width, so both are written and the one in force is always right.
    $('#btn-funnel, #btn-filters').attr('aria-expanded', filterPanelShown() ? 'true' : 'false');
  }
  // Attached to cardsQuery once, by attachCardsQueryListeners above, so a
  // change of the chosen width carries this listener to the new query too.
  $('#btn-funnel').on('click', function () {
    if (cardsQuery.matches) {
      $('body').toggleClass('filters-drawer-open');
      syncFilterFunnel();
      return;
    }
    setFiltersOpen(!filtersOpen);
  });

  // Put the address and the saved settings back into the controls. The address
  // wins where it carries anything, because a link is the whole view rather than
  // a patch on somebody's saved one.
  function restoreFilters() {
    var source = Object.keys(activeQuery).length ? activeQuery : keptFilters;
    valueFilters = {};
    if (source.name) $('#f_search').val(source.name);
    var restoredTicks = hideFreeValue(source.hide_free).split(',');
    $('#col-filters .flt-hide-free').each(function () {
      $(this).prop('checked', restoredTicks.indexOf($(this).closest('.flt-control').attr('data-key')) !== -1);
    });
    sanitiseLinks(source[LINKS_FILTER.key]).forEach(function (condition) {
      var linkState = condition.indexOf('any:') === 0 ? 'include' : 'exclude';
      splitMembers(condition.slice(4)).forEach(function (member) {
        setVlfState(LINKS_FILTER.key, member, linkState);
      });
    });
    Object.keys(source).forEach(function (key) {
      if (NON_COLUMN_PARAMS[key]) return;
      var column = columnByKey[key];
      if (!column) return;
      if (column.date) {
        // A range is two conditions, a from: and a to:, each a picker's value.
        [].concat(source[key]).forEach(function (raw) {
          var value = String(raw);
          var which = value.indexOf('from:') === 0 ? 'from' : value.indexOf('to:') === 0 ? 'to' : '';
          if (which) $('#' + dateControlId(key, which)).val(value.slice(which.length + 1));
        });
        return;
      }
      // A column may carry several conditions - an include set, an exclude
      // set, and what its box holds - so every one of them is put back.
      [].concat(source[key]).forEach(function (raw) {
        var value = String(raw);
        // A Yes/No filter asks for the models with no value as any: alone, and
        // that is the dropdown's Empty option, not a row of a value list.
        if (value === 'any:' && column.value_type === 'flag' && $('#flt_' + cssId(key)).length) {
          $('#flt_' + cssId(key)).val('any:');
          return;
        }
        if (value.indexOf('any:') === 0 || value.indexOf('not:') === 0) {
          var state = value.indexOf('any:') === 0 ? 'include' : 'exclude';
          splitMembers(value.slice(4)).forEach(function (member) {
            setVlfState(key, member, state);
          });
          return;
        }
        var $control = $('#flt_' + cssId(key));
        if (!$control.length) {
          // A value-list control has no box. An exact value a link wrote for
          // a choice or list column is one of the list's own rows, so it
          // restores as an include; a contains: has no row to tick, so it is
          // dropped rather than guessed at.
          if (value.indexOf(':') > 0) return;
          setVlfState(key, value, 'include');
          return;
        }
        var at = value.indexOf(':');
        if (at > 0) value = value.slice(at + 1);
        if (isPriceColumn(column)) {
          var n = parseFloat(value);
          if (isFinite(n)) value = String(n * 1000000);
        }
        $control.val(value);
      });
    });
    // A value the address bar carries for one of these that is not one of the
    // control's own options is treated as if it were absent, the same way a
    // stored view's value already is (sanitiseSelectValue) - never left as a
    // string the <select> has no option for, which is what leaves it showing
    // nothing at all.
    function restoreDrawTimeSelect(key, offered, $control, kept) {
      var raw = activeQuery[key];
      if (raw) {
        var value = sanitiseSelectValue(raw, offered);
        if (!value) {
          console.warn('This value for "' + key + '" names no real choice, so it was left out:', raw);
        }
        $control.val(value);
        return;
      }
      if (kept) $control.val(kept);
    }
    restoreDrawTimeSelect('hidden', SHOW_HIDDEN_VALUES, $('#f_show_hidden'), keptShowHidden);
    restoreDrawTimeSelect('variations', VARIATIONS_VALUES, $('#f_variations'), keptVariations);
    restoreDrawTimeSelect('edited', EDITED_VALUES, $('#f_edited_by_me'), keptEditedByMe);
    // Every value-list button says what was just put back, and a text column
    // whose address already carries a chosen value gets its list on the spot.
    $('#col-filters .flt-control').each(function () {
      var column = vlfColumn($(this).attr('data-key'));
      if (column && $(this).find('.vlf-btn').length) updateVlfButton($(this), column);
    });
    upgradeTextControls();
    updateFilterMarks();
    // The Hidden and Variations controls are read LIVE by the search predicate
    // and by attachVariationRows on every draw, and nothing else redraws after
    // a restore.
    if (table) table.draw(false);
  }

  // Set for the length of a teardown-and-rebuild for a new column order. There
  // is no DataTable to ask for a sort during that window, and the sort it would
  // be asked for is the one being put back, so it is answered from here instead.
  var orderRebuild = null;

  // What the table looks like right now, as the rows a view holds. It is
  // read by the save and by the snapshot a delete keeps, so an undo puts back
  // what was on the screen rather than what was last read off the store.
  function currentViewState() {
    return {
      filters: keptFilters,
      f_show_hidden: keptShowHidden,
      f_variations: keptVariations,
      f_edited_by_me: keptEditedByMe,
      col_order: colOrderIsShipped ? [] : colConfig.map(function (c) { return c.target; }),
      col_hidden: Object.keys(hiddenTargets),
      col_filter_off: Object.keys(filterOffTargets),
      // Which columns this layout was arranged over. A column absent from it is
      // one that did not exist yet, and the server's visible flag decides that
      // one rather than this file.
      col_known: columns.map(function (c) { return c.key; }),
      page_length: pageLen,
      hide_empty: hideEmpty,
      show_marks: showMarks,
      full_text: fullText,
      cards_below: cardsBelow,
      order: orderRebuild ? orderRebuild.sort.slice()
        : table.order().map(function (o) { return [colConfig[o[0]] ? colConfig[o[0]].target : '', o[1]]; })
    };
  }

  // onFail is passed by the reorder path alone, which is the one caller that has
  // something to put back when the write does not land. Every other caller stays
  // fire-and-forget, and every one of them says on screen that the write did not
  // land: an arrangement the reader has named must never be silently unstored.
  //
  // extraSet and removeKeys belong to the load-time write alone. Nothing else
  // names another view's rows, in either half: this page reads every view and
  // writes exactly one, which is what lets a second tab hold a view this one has
  // never heard of.
  //
  // It writes NOTHING while the stored views could not be read, while this
  // load's adoption is not stored, or while a switch is in flight. Each of
  // those three answers onFail with the reason, because a caller holding
  // something to put back must never be left thinking its change was stored.
  //
  // onDone belongs to the load-time write alone, which is the one caller that
  // has to know its write LANDED rather than only that it did not fail.
  function saveSettings(onFail, extraSet, removeKeys, onDone) {
    function refuse(reason) { if (onFail) onFail(new Error(reason)); }
    if (settingsUnreadable) { refuse('the stored views could not be read'); return; }
    if (viewNotStored) { refuse('this page is not storing anything'); return; }
    // A switch is in flight: the table still holds the view being left, and
    // writing it under the id of the one arriving would overwrite the very
    // arrangement being fetched. The rebuild replaces the table moments later,
    // so nothing the reader chose is lost by standing back here.
    if (viewSwitching) { refuse('the view you are moving to is still being read'); return; }
    // The view this body is FOR, read as the body is built rather than when the
    // answer comes back, so a save still in flight when the reader moves on
    // lands on the view the change was made in.
    var id = currentViewId;
    var state = currentViewState();
    var set = {};
    Object.keys(state).forEach(function (key) { set[viewRowKey(id, key)] = state[key]; });
    // The two rows that make those keys A VIEW rather than debris, written with
    // them every time. A second tab can delete this view while this one is
    // still on it, and the save that follows would otherwise store the table
    // keys as nameless rows: invisible on the next load, swept away by it, and
    // the reader's arrangement gone without a word. Named together, the same
    // save puts the whole view back. viewList is built further down the load,
    // and the one write made before it exists carries both rows in extraSet.
    //
    // The id's OWN entry and nothing else. currentView() falls through to the
    // first view in the list, so a page whose id has left the list would stamp
    // another view's name and created onto it: two views under one name, which
    // is the state the clash refusal exists to prevent. With no entry the
    // table keys go on their own.
    var self = viewList && viewById(id);
    if (self) {
      set[viewRowKey(id, 'name')] = self.name;
      set[viewRowKey(id, 'created')] = self.created;
      if (self.position !== null) set[viewRowKey(id, 'position')] = self.position;
    }
    // The app's own preferences, which every view shares.
    set.filters_open = filtersOpen;
    set.auto_update = $('#pref-auto-update').is(':checked');
    set.daily_model_update = $('#daily-update-enabled').is(':checked');
    set.backup_enabled = $('#backup-enabled').is(':checked');
    set.backup_dir = $('#backup-dir').val();
    set.backup_overwrite = $('#backup-overwrite').is(':checked');
    if (extraSet) Object.keys(extraSet).forEach(function (key) { set[key] = extraSet[key]; });
    // The route stores what "set" names and removes what "delete" names, and
    // leaves every other stored key alone - which is what stops this page
    // removing a view it loaded too early to know about.
    fetch('/api/settings', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ set: set, 'delete': removeKeys || [] })
    }).then(function (response) {
      if (!response.ok) throw new Error('the server answered ' + response.status);
      if (onDone) onDone();
    }).catch(function (e) {
      console.error('Failed to save settings', e);
      // A caller that answers a refusal in its own words gets those words and no
      // others: two assertive lines for one press land in the same live region,
      // and the first is overwritten mid-sentence.
      if (onFail) onFail(e);
      else statusErr('That change was not saved: ' + (e.message || e));
    });
  }

  var applyTimer = null;
  // Two quick changes - a value cycled through include to exclude is one -
  // put two fetches in flight, and the FIRST answer can arrive last. Only the
  // newest query may draw, or the table shows a filter nobody has set.
  var applySeq = 0;
  // A filter act the reader made while a switch was still reading its rows, held
  // back rather than run. The switch replays it once the table is the arriving
  // view's, so the act is not lost; a switch that FAILS drops it, because the
  // controls it was made on have been put back and the line saying what went
  // wrong is the last thing that should be painted over.
  //
  // It holds the act's VALUES rather than a bare flag. The rebuild that ends the
  // switch empties the filter controls and fills them from the kept values, so an
  // act left sitting in the controls would be gone before the replay could read
  // it, and the replay would re-run the arriving view's own stored filters over
  // the top of what the reader did.
  var filterActHeldBySwitch = null;
  // What #filter-feedback says about the act that is about to fetch, set by
  // whichever handler called applyFilters right before it did, and consumed
  // (then cleared) the moment that fetch lands - never carried into a fetch
  // nothing set it for, which is why every other draw in this file leaves it
  // alone.
  var pendingFilterFeedback = null;
  function applyFilters(immediate) {
    clearTimeout(applyTimer);
    applyTimer = setTimeout(function () {
      // Until a switch's rows are in the table, the table is the outgoing view's
      // and colConfig, hiddenTargets and the filter controls are the arriving
      // one's. A fetch started here bumps applySeq, which drops the switch's own
      // rows, so the rebuild that makes the two agree never runs: the draw mixes
      // two views and the next header click stores a sort read through the wrong
      // column. Standing back for the length of the read is what keeps them one
      // view.
      if (viewSwitching) {
        filterActHeldBySwitch = {
          filters: buildQuery(),
          showHidden: $('#f_show_hidden').val() || '',
          variations: $('#f_variations').val() || '',
          editedByMe: $('#f_edited_by_me').val() || ''
        };
        return;
      }
      var params = buildQuery();
      // Everything that reaches applyFilters is the reader working the filter
      // bar, so this is where a link's query becomes the view's own: from here
      // on the controls are refilled from keptFilters and not from the link.
      activeQuery = {};
      keptFilters = Object.assign({}, params);
      keptShowHidden = $('#f_show_hidden').val() || '';
      keptVariations = $('#f_variations').val() || '';
      keptEditedByMe = $('#f_edited_by_me').val() || '';
      updateFilterMarks();
      if ($('#f_show_hidden').val()) params.hidden = $('#f_show_hidden').val();
      if ($('#f_variations').val()) params.variations = $('#f_variations').val();
      if ($('#f_edited_by_me').val()) params.edited = $('#f_edited_by_me').val();
      writeAddress(params);
      saveSettings();
      statusText('Reading the models...');
      var seq = ++applySeq;
      fetchRows(params)
        .then(function (rows) {
          if (seq !== applySeq) return;
          // Read right before indexRows/fillTable overwrite what
          // table.page.info() answers - the last moment the table still
          // shows the read that is about to be replaced.
          var feedbackOldInfo = table.page.info();
          indexRows(rows);
          fillTable();
          refreshCatalogEmptiness();
          hideStatusNow();
          maybeShowDidYouMean(params, rows);
          if (pendingFilterFeedback) {
            var newInfo = table.page.info();
            showFilterFeedback(filterFeedbackText(pendingFilterFeedback.kind, pendingFilterFeedback.label,
              feedbackOldInfo.recordsDisplay, newInfo.recordsDisplay, catalogTotal(newInfo)));
          }
          pendingFilterFeedback = null;
        })
        .catch(function (e) {
          pendingFilterFeedback = null;
          if (seq !== applySeq) return;
          statusErr('That filter was refused: ' + (e.message || e));
        });
    }, immediate ? 0 : 350);
  }

  buildFilterControls();
  restoreFilters();
  // A bare address that opened on the saved filters is put in the address bar,
  // so the address is a link to the rows on screen from the first moment.
  if (firstQuery !== activeQuery && Object.keys(firstQuery).length) {
    var restoredParams = buildQuery();
    if ($('#f_show_hidden').val()) restoredParams.hidden = $('#f_show_hidden').val();
    if ($('#f_variations').val()) restoredParams.variations = $('#f_variations').val();
    if ($('#f_edited_by_me').val()) restoredParams.edited = $('#f_edited_by_me').val();
    writeAddress(restoredParams);
  }
  applyFiltersOpen();
  refreshCatalogEmptiness();

  // Hide-empty is a LAYOUT preference, like the page size and the docked panel:
  // it is stored with the view, stays out of the address (a link hands over
  // rows, not somebody's idea of a tidy table) and Clear leaves it alone.
  $('#f_hide_empty').prop('checked', hideEmpty);
  $('#f_hide_empty').on('change', function () {
    hideEmpty = $(this).is(':checked');
    $('body').toggleClass('hide-empty', hideEmpty);
    applyHideEmpty();
    announce('status-announce', hideEmpty
      ? 'Columns and cells with no value are hidden'
      : 'Columns and cells with no value are shown');
    saveSettings();
  });
  // Cards below is a LAYOUT preference too, stored with the view and excluded
  // from the address the same way hide-empty is. Changing it rewrites the
  // stylesheet and swaps cardsQuery for one built on the new width, live, with
  // no reload - see rebuildCardsQuery above.
  $('#f_cards_below').val(String(cardsBelow));
  $('#f_cards_below').on('change', function () {
    cardsBelow = sanitiseCardsBelow($(this).val());
    rebuildCardsQuery(cardsBelow, true);
    announce('status-announce', cardsBelow
      ? 'The table becomes cards below ' + cardsBelow + ' pixels wide'
      : 'The table never becomes cards');
    saveSettings();
  });
  $('#f_show_marks').prop('checked', showMarks);
  $('#f_show_marks').on('change', function () {
    showMarks = $(this).is(':checked');
    $('body').toggleClass('hide-marks', !showMarks);
    announce('status-announce', showMarks ? 'Marks beside values are shown' : 'Marks beside values are hidden');
    saveSettings();
  });
  // Full text is a LAYOUT preference too, stored and excluded from the address
  // the same way hide-empty and show-marks are: it says how a value is
  // PAINTED, not which rows are wanted, so a link never carries it and Clear
  // never touches it.
  $('#f_full_text').prop('checked', fullText);
  $('#f_full_text').on('change', function () {
    fullText = $(this).is(':checked');
    $('body').toggleClass('full-text', fullText);
    announce('status-announce', fullText
      ? 'Long text is shown in full'
      : 'Long text is shortened, with the rest on hover');
    saveSettings();
    // clipHtml reads the class this just toggled, and DataTables keeps every
    // row's rendered markup across an ordinary draw - invalidate('data') is
    // what tells it the CELL CONTENT itself needs building again, not only the
    // set of rows and their order.
    if (table) table.rows().invalidate('data').draw(false);
  });
  if (Array.isArray(view.order) && view.order.length) {
    var idxOrder = view.order
      .map(function (o) { return [getColIdx(o[0]), o[1]]; })
      .filter(function (o) { return o[0] >= 0; });
    if (idxOrder.length) table.order(idxOrder).draw();
  }
  updateModelCount();

  // The label #filter-feedback names for each of the three fixed selects -
  // their own <label> text minus the emoji, which nobody reads out loud.
  var FIXED_CONTROL_LABEL = {
    f_show_hidden: 'Hidden', f_variations: 'Variations', f_edited_by_me: 'Edited by me'
  };
  // What the next applyFilters call should tell #filter-feedback, read off the
  // control that is about to fire it: the name search is its own label, a
  // per-column control is its column's, and set/cleared is read off whether
  // the control's OWN value is empty right now, before the fetch that follows
  // can change anything.
  function noteFilterFeedbackSource($el) {
    var label = $el.is('#f_search') ? 'Name search' : null;
    if (!label && $el.is('.flt-hide-free')) {
      var tickColumn = columnByKey[$el.closest('.flt-control').attr('data-key')];
      label = tickColumn ? 'Hide 0 in ' + tickColumn.label : 'Hide 0';
    }
    if (!label) {
      var column = columnByKey[$el.closest('.flt-control').attr('data-key')];
      label = column ? column.label : null;
    }
    if (!label) { pendingFilterFeedback = null; return; }
    var empty = $el.is(':checkbox') ? !$el.is(':checked') : String($el.val() == null ? '' : $el.val()) === '';
    pendingFilterFeedback = { kind: empty ? 'cleared' : 'set', label: label };
  }

  // A value-list panel's own search box narrows the PANEL, never the table, so
  // it must not reach the handlers that reload the rows.
  $('.filters').on('change', '#col-filters input, #col-filters select, #f_search', function () {
    if ($(this).closest('.vlf-panel').length) return;
    noteFilterFeedbackSource($(this));
    applyFilters(true);
  });
  $('.filters').on('keyup', '#col-filters input, #f_search', function () {
    if ($(this).closest('.vlf-panel').length) return;
    noteFilterFeedbackSource($(this));
    applyFilters(false);
  });
  // The two controls that decide which of the rows already here are DRAWN need
  // no second read of the server, so #filter-feedback is shown straight off
  // the draw this triggers rather than through applyFilters' own fetch.
  $('#f_show_hidden, #f_variations, #f_edited_by_me').on('change', function () {
    var params = buildQuery();
    activeQuery = {};
    keptFilters = Object.assign({}, params);
    keptShowHidden = $('#f_show_hidden').val() || '';
    keptVariations = $('#f_variations').val() || '';
    keptEditedByMe = $('#f_edited_by_me').val() || '';
    // This control neither fetches nor bumps applySeq, so it runs during a
    // switch - and a switch is one of the three states in which a save is
    // refused. The choice stands on the screen either way, so the reader has to
    // be told the one thing they cannot see: that it was not stored.
    saveSettings(function (e) {
      statusErr('That choice was not saved (' + (e.message || e) +
        '), so it stands until you reload.');
    });
    if ($('#f_show_hidden').val()) params.hidden = $('#f_show_hidden').val();
    if ($('#f_variations').val()) params.variations = $('#f_variations').val();
    if ($('#f_edited_by_me').val()) params.edited = $('#f_edited_by_me').val();
    writeAddress(params);
    updateFilterMarks();
    var feedbackLabel = FIXED_CONTROL_LABEL[$(this).attr('id')];
    var feedbackKind = $(this).val() ? 'set' : 'cleared';
    var feedbackOldInfo = table.page.info();
    // The switch's own rebuild draws the table moments later, over the columns
    // this draw would not have - and it is the switch's arriving view that
    // #filter-feedback would need to speak of, not this one, so nothing is
    // shown here while a switch is in flight.
    if (!viewSwitching) {
      table.draw();
      var feedbackNewInfo = table.page.info();
      showFilterFeedback(filterFeedbackText(feedbackKind, feedbackLabel,
        feedbackOldInfo.recordsDisplay, feedbackNewInfo.recordsDisplay, catalogTotal(feedbackNewInfo)));
    }
  });
  // A rebuild sorts the fresh table itself, and saves once when it is done, so
  // the sorts it fires on the way through are not choices anybody made. The
  // same holds for the redraw a change to the shown columns makes: it changes
  // no sort, and that path saves once itself when it is done.
  $('#models').on('order.dt', function () {
    if (!table || orderRebuild || shownRedraw) return;
    saveSettings();
  });

  $('#f_clear').on('click', function () {
    $('#col-filters input, #col-filters select').not(':checkbox').val('');
    $('#col-filters .flt-hide-free').prop('checked', false);
    $('#f_search').val('');
    $('#f_show_hidden, #f_variations, #f_edited_by_me').val('');
    // The value lists empty with everything else: the state goes, and every
    // button and every already-built panel is repainted to say so.
    valueFilters = {};
    closeVlfPanels();
    $('#col-filters .flt-control').each(function () {
      var column = vlfColumn($(this).attr('data-key'));
      if (!column || !$(this).find('.vlf-btn').length) return;
      updateVlfButton($(this), column);
      if ($(this).find('.vlf-panel').data('built')) drawVlfRows($(this), column);
    });
    table.order([]);
    pendingFilterFeedback = { kind: 'clear-all', label: null };
    applyFilters(true);
  });

  // --- The column chooser ---------------------------------------------------
  // Every column this database holds, whether it is drawn, what it is called,
  // and - for a column of yours - the way to delete it. It manages them through
  // /api/columns, which is the one place a column is made, relabelled or
  // removed.

  // How the manager's list is narrowed, ordered and selected. The toolbar
  // filters (search, Sort, Answers, Only shown, Subject, Written by) are
  // remembered as one app preference, CMG_FILTERS_KEY, and the dialog reopens
  // with them; the selection and its anchor are never remembered, because a
  // selection never survives the dialog closing. Sorting this LIST never
  // reorders the TABLE: the table's own order lives in col_order, which
  // nothing here writes.
  function freshCmgState() {
    return {
      search: '', sort: 'table', drawnOnly: false, answers: 'any',
      // The two dropdowns: a tick means shown, which is the only reading a
      // checkbox has.
      subjects: DOMAIN_SECTIONS.slice(), kinds: KIND_ORDER.slice(),
      selection: [], anchor: null
    };
  }

  // The stored toolbar filters. The two dropdowns are stored as the boxes left
  // UNTICKED, so a subject or kind added by a later version arrives ticked and
  // one that has gone is simply not found.
  var CMG_FILTERS_KEY = 'columns_dialog_filters';
  var CMG_ANSWERS = ['any', 'has', 'none'];

  // The state the dialog opens with: the defaults, with every part of the
  // stored value that is still valid laid over them. A value of the wrong
  // shape, or a part that no longer applies, is dropped without a word.
  function cmgStateFromStored(stored) {
    var state = freshCmgState();
    if (!stored || typeof stored !== 'object' || Array.isArray(stored)) return state;
    if (typeof stored.search === 'string') state.search = stored.search;
    if (typeof stored.sort === 'string' && Object.prototype.hasOwnProperty.call(CMG_SORTS, stored.sort)) state.sort = stored.sort;
    if (CMG_ANSWERS.indexOf(stored.answers) !== -1) state.answers = stored.answers;
    if (stored.drawn_only === true) state.drawnOnly = true;
    if (Array.isArray(stored.subjects_off)) {
      state.subjects = DOMAIN_SECTIONS.filter(function (v) { return stored.subjects_off.indexOf(v) === -1; });
    }
    if (Array.isArray(stored.kinds_off)) {
      state.kinds = KIND_ORDER.filter(function (v) { return stored.kinds_off.indexOf(v) === -1; });
    }
    return state;
  }

  // The value written for a state: the shape cmgStateFromStored reads.
  function cmgStoredFromState(state) {
    return {
      search: state.search, sort: state.sort, answers: state.answers,
      drawn_only: state.drawnOnly,
      subjects_off: DOMAIN_SECTIONS.filter(function (v) { return state.subjects.indexOf(v) === -1; }),
      kinds_off: KIND_ORDER.filter(function (v) { return state.kinds.indexOf(v) === -1; })
    };
  }

  // CMG_SORTS is declared further down, so the stored value is laid over the
  // defaults on the first open of the dialog rather than here.
  var cmgState = freshCmgState();
  var cmgFiltersRestored = false;
  var cmgSaveTimer = null;

  // Stores the toolbar filters as they stand. A write the settings refuse
  // (a switch in flight, an unreadable store) is only logged: the filter still
  // applies for this visit, and the next change writes again.
  function saveCmgFilters() {
    if (cmgSaveTimer) { clearTimeout(cmgSaveTimer); cmgSaveTimer = null; }
    var extra = {};
    extra[CMG_FILTERS_KEY] = cmgStoredFromState(cmgState);
    saveSettings(function (e) { console.warn('Columns filters not stored', e); }, extra);
  }
  // Typing writes once it pauses, not once per key.
  function saveCmgFiltersSoon() {
    if (cmgSaveTimer) clearTimeout(cmgSaveTimer);
    cmgSaveTimer = setTimeout(saveCmgFilters, 400);
  }

  // Whether any toolbar control is off its default.
  function cmgFiltersActive() {
    return cmgState.search !== '' || cmgState.sort !== 'table' || cmgState.answers !== 'any' ||
      cmgState.drawnOnly || cmgState.subjects.length < DOMAIN_SECTIONS.length ||
      cmgState.kinds.length < KIND_ORDER.length;
  }

  // Makes every toolbar control show cmgState.
  function drawCmgToolbar() {
    $('#cmg-search').val(cmgState.search);
    $('#cmg-sort').val(cmgState.sort);
    $('#cmg-answers').val(cmgState.answers);
    $('#cmg-drawn-only').attr('aria-pressed', cmgState.drawnOnly ? 'true' : 'false');
    buildCmgDropdown('subjects');
    buildCmgDropdown('kinds');
  }
  // The columns this list is showing right now, in the order it drew them. The
  // tick in the Shown heading acts on exactly these and on nothing else.
  var cmgKept = [];

  // Every sort but Subject is one flat list: Table order because it is the
  // order the table draws and drawing it in subject boxes would be a lie about
  // the table, Title because ranking it by subject first would still be
  // ranking by a group before the letters. Subject is the one sort that groups,
  // under the same six headings the edit form and the filter panel use, and it
  // stays a sort: the list is still flat rows, with a heading drawn before the
  // first row of each group, and the move controls are refused under it like
  // under every sort that is not the table's own order.
  //
  // type reads how many of the columns ON SCREEN share a value type, counted
  // fresh into cmgTypeFreq right before the sort runs, so "most first" means
  // most among what the reader is looking at rather than the whole catalog.
  var cmgTypeFreq = null;
  function typeFrequencyAmong(entries) {
    var freq = {};
    entries.forEach(function (c) { freq[c.column.value_type] = (freq[c.column.value_type] || 0) + 1; });
    return freq;
  }
  var CMG_SORTS = {
    alpha: function (a, b) { return a.column.label.localeCompare(b.column.label); },
    key: function (a, b) { return a.column.key.localeCompare(b.column.key); },
    type: function (a, b) {
      var fa = (cmgTypeFreq && cmgTypeFreq[a.column.value_type]) || 0;
      var fb = (cmgTypeFreq && cmgTypeFreq[b.column.value_type]) || 0;
      if (fa !== fb) return fb - fa;
      if (a.column.value_type !== b.column.value_type) return a.column.value_type.localeCompare(b.column.value_type);
      return a.column.label.localeCompare(b.column.label);
    },
    answers: function (a, b) {
      var na = columnHasValues[a.column.key] || 0, nb = columnHasValues[b.column.key] || 0;
      if (na !== nb) return nb - na;
      return a.column.label.localeCompare(b.column.label);
    },
    subject: function (a, b) {
      var da = DOMAIN_SECTIONS.indexOf(domainOf(a.column)), db = DOMAIN_SECTIONS.indexOf(domainOf(b.column));
      if (da !== db) return da - db;
      return getColIdx(a.column.key) - getColIdx(b.column.key);
    }
  };

  // What the list draws, top to bottom, decided once for both row shapes: the
  // kept columns in their sorted order, and under Subject a heading before the
  // first column of each group. A group the search or a filter has emptied
  // never gets a heading, because there is no first column to put it before.
  function cmgDrawPlan(kept) {
    var plan = [];
    var lastDomain = null;
    kept.forEach(function (c) {
      if (cmgState.sort === 'subject') {
        var domain = domainOf(c.column);
        if (domain !== lastDomain) {
          plan.push({ heading: DOMAIN_SECTION_NAME[domain] || domain });
          lastDomain = domain;
        }
      }
      plan.push({ entry: c });
    });
    return plan;
  }

  // A group heading carries no tick and no control: the keyboard walk and
  // Shift-extend cross the ticks, the Shown tick acts on cmgKept, and the
  // selection reads .cmg-row, so a heading is outside every one of them by
  // being none of those things. It is a heading, so a screen reader can jump
  // between groups the way it jumps between the form's own.
  function cmgGroupName(name) {
    return $('<h4 class="cmg-group-name"></h4>').text(name);
  }
  function cmgGroupRowForTable(name) {
    return $('<tr class="cmg-group" role="row"></tr>').append(
      $('<td role="cell" colspan="10"></td>').append(cmgGroupName(name)));
  }

  // A rebuild throws away the very element the reader is standing on, and the
  // browser drops focus to <body>, which starts the next Tab at the top of the
  // document. Focus is put back by IDENTITY - what the control is, not the
  // element it was - and the first candidate that still exists wins. Each
  // candidate is a selector or a function returning one.
  function focusByIdentity(candidates) {
    for (var i = 0; i < candidates.length; i++) {
      var $found = typeof candidates[i] === 'function' ? candidates[i]() : $(candidates[i]);
      if (!$found || !$found.length) continue;
      var $one = $found.first();
      $one.trigger('focus');
      $one.get(0).scrollIntoView({ block: 'nearest', behavior: 'auto' });
      return;
    }
  }

  // Keeps Tab and Shift+Tab inside an overlay div. Such an overlay is not a
  // native <dialog>, so nothing else stops the key walking out into the page
  // behind it. Called from the overlay's own keydown handler, so only a key
  // pressed inside it arrives; a native dialog opened on top of it sits
  // elsewhere in the document and is never reached from here. Focus that is on
  // the overlay but on no stop (its title, which takes focus on open) enters
  // at the first stop, or at the last on Shift+Tab.
  function trapTabIn(e, container) {
    if (e.key !== 'Tab' || !container) return;
    var stops = Array.prototype.filter.call(
      container.querySelectorAll('a[href], button, input:not([type="hidden"]), select, textarea, [tabindex]:not([tabindex="-1"])'),
      function (el) { return !el.disabled && el.offsetParent !== null; });
    if (!stops.length) { e.preventDefault(); return; }
    var at = stops.indexOf(document.activeElement);
    var first = stops[0];
    var last = stops[stops.length - 1];
    if (e.shiftKey && (at <= 0)) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && (at < 0 || at === stops.length - 1)) { e.preventDefault(); first.focus(); }
  }

  // Whether a figure can be printed at all. A catalog with no models in it has
  // no column anything could have answered, so the counting half of the dialog
  // says nothing rather than calling every column dead.
  function answersAreCountable() {
    return catalogEmptinessRead && !catalogHasNoModels;
  }

  function cmgKeeps(column) {
    if (cmgState.answers !== 'any' && answersAreCountable()) {
      var has = (columnHasValues[column.key] || 0) > 0;
      if (cmgState.answers === 'has' && !has) return false;
      if (cmgState.answers === 'none' && has) return false;
    }
    // The list holds every column the database has; the table draws a fraction
    // of them. This tick is the way to see only that fraction, so two rows
    // next to each other here are next to each other in the table too.
    if (cmgState.drawnOnly && !columnDrawn(column.key)) return false;
    if (cmgState.subjects.indexOf(domainOf(column)) === -1) return false;
    if (cmgState.kinds.indexOf(kindOf(column)) === -1) return false;
    var q = parseSearch(cmgState.search);
    // Heading OR id OR a feed path that fills it, because a column is known by
    // any of them: what the table shows is the heading, what a link or a saved
    // layout names is the id, and what the feed calls it is the word a reader
    // who knows the feed types - "hugging_face_id" fills Repository, and
    // nobody looking for that types "rep".
    if (!searchMatches(q, [column.label, column.key].concat(column.feed_paths || []))) return false;
    return true;
  }

  // A column can be ticked here and still be absent from the table, because
  // hiding what is empty is a second, automatic cut that ADDS to this one.
  function heldBackFromTheTable(key) {
    return !hiddenTargets[key] && !!emptyTargets[key];
  }

  // The one sentence a ticked-but-empty column says, on the tick's own warning
  // mark and its title alike.
  var HELD_BACK_SENTENCE = 'Ticked, but not drawn: no model on screen has a value in it, ' +
    'so "Hide empty" in the filters is taking it out. Untick that to draw it.';

  // Writes, or removes, that mark beside ONE row's tick. Redrawn in place
  // rather than by rebuilding the list, so the tick keeps the keyboard focus
  // it just took.
  function drawHeldBackNote($row, key) {
    var $cell = $row.find('.cmg-c-tick');
    $cell.find('.cmg-held-glyph, .cmg-held-desc').remove();
    if (!heldBackFromTheTable(key)) return;
    var descId = 'cmgheld_' + cssId(key);
    $cell.append($('<span class="cmg-held-glyph" tabindex="0" aria-hidden="false"></span>')
      .attr('title', HELD_BACK_SENTENCE)
      .attr('aria-describedby', descId)
      .text('\u26A0'))
      .append($('<span class="sr-only cmg-held-desc"></span>').attr('id', descId).text(HELD_BACK_SENTENCE));
  }

  // Every one of these changes which rows the list is showing, and each one
  // empties the selection with it: a selection reaching columns that are no
  // longer on the page is a group move the reader cannot see the shape of.
  $('#cmg-search').on('input', function () {
    cmgState.search = String($(this).val() || '');
    saveCmgFiltersSoon();
    clearSelection('');
    buildColumnChooser();
  });
  $('#cmg-answers').on('change', function () {
    cmgState.answers = $(this).val();
    saveCmgFilters();
    clearSelection('');
    buildColumnChooser();
  });
  $('#cmg-drawn-only').on('click', function () {
    cmgState.drawnOnly = !cmgState.drawnOnly;
    $(this).attr('aria-pressed', cmgState.drawnOnly ? 'true' : 'false');
    saveCmgFilters();
    clearSelection('');
    buildColumnChooser();
  });
  // One press puts every toolbar control back to its default, and the defaults
  // are what is then remembered.
  $('#cmg-filters-clear').on('click', function () {
    var kept = { selection: cmgState.selection, anchor: cmgState.anchor };
    cmgState = freshCmgState();
    cmgState.selection = kept.selection;
    cmgState.anchor = kept.anchor;
    closeCmgDropdowns();
    drawCmgToolbar();
    saveCmgFilters();
    clearSelection('');
    buildColumnChooser();
    $('#cmg-search').trigger('focus');
  });
  $('#cmg-help-toggle').on('click', function () {
    var open = $(this).attr('aria-expanded') === 'true';
    $(this).attr('aria-expanded', open ? 'false' : 'true');
    $('#cmg-help').prop('hidden', open);
  });

  // --- The two dropdowns: Subject and Written by ----------------------------
  // The filter panel's own value-list shell (attachVlf builds a .vlf-btn and a
  // .vlf-panel), with native checkboxes standing in for the three-state row a
  // value list needs when "excluded" is a state worth having - a two-state
  // list has no use for it. Built once per dialog open, state read back into
  // cmgState on every change.
  function cmgDdAll(which) { return which === 'subjects' ? DOMAIN_SECTIONS : KIND_ORDER; }
  function cmgDdPanel(which) { return $(which === 'subjects' ? '#cmg-subject-panel' : '#cmg-kind-panel'); }
  function cmgDdButton(which) { return $(which === 'subjects' ? '#cmg-subject-btn' : '#cmg-kind-btn'); }
  function cmgDdWord(which) { return which === 'subjects' ? 'Subject' : 'Written by'; }
  function cmgDdName(which, v) { return which === 'subjects' ? (DOMAIN_SECTION_NAME[v] || v) : KIND_PHRASE[v]; }

  function buildCmgDropdown(which) {
    var $panel = cmgDdPanel(which).empty();
    var all = cmgDdAll(which);
    all.forEach(function (v) {
      var $row = $('<label class="cmg-dd-row"></label>');
      $row.append($('<input type="checkbox">').attr('data-value', v)
        .prop('checked', cmgState[which].indexOf(v) !== -1));
      if (which === 'kinds') {
        $row.append($('<span aria-hidden="true"></span>').html(KIND_ICON[v]));
        $row.attr('title', KIND_LEGEND[v]);
      }
      $row.append($('<span></span>').text(cmgDdName(which, v)));
      $panel.append($row);
    });
    $panel.append($('<button type="button" class="btn-clear cmg-dd-foot">Tick all</button>'));
    cmgDdRefresh(which);
  }

  function cmgDdRefresh(which) {
    var all = cmgDdAll(which);
    var ticked = cmgState[which];
    cmgDdPanel(which).find('.cmg-dd-foot').prop('hidden', ticked.length === all.length);
    var word = cmgDdWord(which);
    var names = ticked.map(function (v) { return cmgDdName(which, v); });
    var text, accessible;
    if (ticked.length === all.length) { text = 'All'; accessible = 'all'; }
    else if (ticked.length === 0) { text = 'None'; accessible = 'none'; }
    else if (ticked.length <= 2) { text = names.join(', '); accessible = names.join(', '); }
    else { text = ticked.length + ' of ' + all.length; accessible = ticked.length + ' of ' + all.length; }
    cmgDdButton(which).text(word + ': ' + text).attr('aria-label', word + ' filter: ' + accessible);
  }

  function cmgDdChange(which) {
    cmgState[which] = cmgDdPanel(which).find('input:checked').map(function () { return $(this).attr('data-value'); }).get();
    cmgDdRefresh(which);
    saveCmgFilters();
    clearSelection('');
    buildColumnChooser();
  }
  $('#cmg-subject-panel').on('change', 'input[type=checkbox]', function () { cmgDdChange('subjects'); });
  $('#cmg-kind-panel').on('change', 'input[type=checkbox]', function () { cmgDdChange('kinds'); });
  $('#cmg-subject-panel, #cmg-kind-panel').on('click', '.cmg-dd-foot', function () {
    var which = $(this).closest('.cmg-dd-panel').attr('id') === 'cmg-subject-panel' ? 'subjects' : 'kinds';
    $(this).siblings('.cmg-dd-row').find('input[type=checkbox]').prop('checked', true);
    cmgState[which] = cmgDdAll(which).slice();
    cmgDdRefresh(which);
    saveCmgFilters();
    clearSelection('');
    buildColumnChooser();
  });

  function closeCmgDropdowns(except) {
    $('.cmg-dd-panel').not(except || null).each(function () {
      if ($(this).prop('hidden')) return;
      $(this).prop('hidden', true);
      $(this).closest('.cmg-dd').find('.cmg-dd-btn').attr('aria-expanded', 'false');
    });
  }
  $('.cmg-toolbar').on('click', '.cmg-dd-btn', function () {
    var $panel = $(this).closest('.cmg-dd').find('.cmg-dd-panel');
    var wasOpen = !$panel.prop('hidden');
    closeCmgDropdowns();
    if (wasOpen) return;
    $panel.prop('hidden', false);
    $(this).attr('aria-expanded', 'true');
  });
  $('.cmg-toolbar').on('keydown', '.cmg-dd', function (e) {
    if (e.key !== 'Escape') return;
    var $panel = $(this).find('.cmg-dd-panel');
    if ($panel.prop('hidden')) return;
    e.preventDefault();
    e.stopPropagation();
    closeCmgDropdowns();
    $(this).find('.cmg-dd-btn').trigger('focus');
  });
  $(document).on('click', function (e) {
    if (!document.contains(e.target)) return;
    if ($(e.target).closest('.cmg-dd').length) return;
    closeCmgDropdowns();
  });

  // --- The help list ----------------------------------------------------------
  // Closed on every open of the dialog, built fresh so the view line names the
  // view on screen. KIND_PHRASE and KIND_LEGEND are used here so the help
  // cannot drift from what a row's own icon says on hover.
  function buildCmgHelp() {
    var $ul = $('#cmg-help').empty();
    $ul.append($('<li id="cmg-help-view-line"></li>'));
    [
      'Tick a column to draw it. Hide empty in the filters can still leave it out; a row that is ticked but not drawn shows a warning mark beside its tick.',
      "Rename changes only the heading. A link and a saved layout name a column by its id, which never changes.",
      'Delete removes a column you made and every value in it. A column the app ships cannot be deleted.',
      'Move a column with its up and down buttons, or drag its row: hold the mouse button down on the row and slide it. Both work while this list is in Table order. Hold Shift while pressing an up or down button to send the column to the top or the bottom.',
      'Move several at once: hold Ctrl and click the rows, then drag one of them, or press its up or down button. On a phone, hold a row until it is selected, tap the others, then drag one of them.',
      'When this list is narrowed by the search or a filter, a move steps over columns the list is not showing.',
      "Sorting this list is a way to find a column. It never moves the table's columns.",
      'Undo and Redo step back and forward over every tick and move made here, until you leave the page.'
    ].forEach(function (text) { $ul.append($('<li></li>').text(text)); });
    var $kindsLi = $('<li></li>').text('Written by:');
    var $kindsUl = $('<ul class="cmg-help-kinds"></ul>');
    KIND_ORDER.forEach(function (kind) {
      $kindsUl.append($('<li></li>')
        .append($('<span aria-hidden="true"></span>').html(KIND_ICON[kind]))
        .append($('<span class="cmg-help-kind-phrase"></span>').text(KIND_PHRASE[kind] + ': '))
        .append($('<span class="cmg-help-kind-sentence"></span>').text(KIND_LEGEND[kind])));
    });
    $kindsLi.append($kindsUl);
    $ul.append($kindsLi);
    drawColumnsViewLine();
  }

  // The count line, with the filters that narrowed it.
  function drawCmgCount(all) {
    if (catalogEmptinessRead && catalogHasNoModels) {
      $('#cmg-count').text('No models yet, so no column can have been answered. ' +
        'Press Update models in the menu at the top right to fetch the catalog.');
      return;
    }
    var text = cmgKept.length + ' of ' + all + ' ' + (all === 1 ? 'column' : 'columns');
    var narrowedBy = [];
    if (cmgState.search.trim()) narrowedBy.push('search');
    if (cmgState.answers !== 'any') narrowedBy.push('Answers');
    if (cmgState.subjects.length < DOMAIN_SECTIONS.length) narrowedBy.push('Subject');
    if (cmgState.kinds.length < KIND_ORDER.length) narrowedBy.push('Written by');
    if (cmgState.drawnOnly) narrowedBy.push('Only shown');
    if (narrowedBy.length) text += ', narrowed by ' + narrowedBy.join(', ');
    $('#cmg-count').text(text);
    $('#cmg-filters-clear').prop('hidden', !cmgFiltersActive());
  }

  // The tick over the whole list, read off the rows this list is showing:
  // ticked when every one of them is shown, part-way when some are, empty when
  // none are. A list the search has emptied leaves it nothing to act on, so it
  // is disabled.
  function drawCmgAll() {
    var shown = cmgKept.filter(function (c) { return !hiddenTargets[c.column.key]; }).length;
    var all = $('#cmg-all')[0];
    if (!all) return;
    all.checked = cmgKept.length > 0 && shown === cmgKept.length;
    all.indeterminate = shown > 0 && shown < cmgKept.length;
    all.disabled = cmgKept.length === 0;
  }

  // The narrowing by answers: absent while there is nothing to count, refused
  // with its reason while the read has not landed or has failed, and never left
  // enabled to select every column in the catalog silently.
  function drawCmgAnswers() {
    var $pick = $('#cmg-answers').closest('.cmg-pick');
    if (catalogEmptinessRead && catalogHasNoModels) { $pick.prop('hidden', true); return; }
    $pick.prop('hidden', false);
    var why = '';
    if (catalogEmptinessFailed) why = 'Answers could not be counted: reading the catalog failed';
    else if (!catalogEmptinessRead) why = 'The catalog has not been read yet, so nothing can be counted';
    $('#cmg-answers').prop('disabled', !!why)
      .attr('title', why || 'Show only the columns something has answered, or only the columns nothing has');
  }

  // --- The left-to-right order the reader sets -------------------------------
  // One order, stored as col_order, shared by the table and the
  // cards. A press moves the column past the next one DRAWN AT THIS WIDTH, which
  // is a different set below 1642px because hide-empty takes no columns out of a
  // card - so the announcement names the neighbour that was really crossed here.

  function firstRealIndex() {
    return colConfig.findIndex(function (c) { return !!c.column; });
  }
  function lastRealIndex() {
    for (var i = colConfig.length - 1; i >= 0; i--) if (colConfig[i].column) return i;
    return -1;
  }

  // Where the column at i lands, or null when there is nothing that way for it to
  // cross. A column nobody can see has nothing on screen to step past, so it
  // steps one place in the full order. A drawn column crosses a DRAWN one or the
  // press is refused: sending it to the far end of a long column list
  // moves nothing on screen, and a control whose effect is invisible is broken
  // however correct its arithmetic.
  function moveDestination(i, dir) {
    var step = dir === 'up' ? -1 : 1;
    var drawn = columnDrawn(colConfig[i].target);
    for (var j = i + step; j >= 0 && j < colConfig.length; j += step) {
      // Name is the frozen left edge and nothing goes in front of it. The
      // variations control is crossed like any other drawn column.
      if (colConfig[j].target === NAME_TARGET) break;
      if (!drawn || columnDrawn(colConfig[j].target)) return j;
    }
    return null;
  }

  function modelFormOpen() {
    var add = document.getElementById('add-modal');
    var edit = document.getElementById('edit-modal');
    return !!((add && add.open) || (edit && edit.open));
  }

  // Why a press would do nothing, in the words the button carries in its own
  // accessible name. Empty means it will act.
  function moveRefusal(i, dir) {
    if (cmgState.sort !== 'table') return 'only in Table order';
    if (modelFormOpen()) return 'not while a model form is open';
    if (i < 0 || moveDestination(i, dir) === null) {
      return dir === 'up' ? 'already first' : 'already last';
    }
    return '';
  }

  // A row's own move button acts alone, unless the row is part of a selection
  // of more than one, in which case it moves the whole block.
  function setMoveState($button, key, dir) {
    var group = isSelected(key) && selectedKeys().length > 1;
    var why = group ? groupRefusal(dir) : moveRefusal(getColIdx(key), dir);
    var label = group ? ('the ' + selectedKeys().length + ' selected columns') : labelOf(key);
    var name = 'Move ' + label + ' ' + dir + (why ? ' (' + why + ')' : '');
    $button.attr('aria-disabled', why ? 'true' : 'false')
      .attr('aria-label', name)
      .attr('title', name + '. Hold Shift to send ' + (group ? 'them' : 'it') + ' to the ' +
        (dir === 'up' ? 'top' : 'bottom') + '.');
  }
  function refreshMoveControls() {
    $('#col-manage-list .cmg-move').each(function () {
      setMoveState($(this), $(this).attr('data-key'), $(this).attr('data-dir'));
    });
  }

  // What the table must be put back to after it is rebuilt. The sort is taken by
  // TARGET rather than by index, because the index it names is about to mean a
  // different column. movedKeys names the columns a FLIP highlight should land
  // on after the rebuild - the entries a button, a drag, Undo or Redo actually
  // relocated, as opposed to every row a move merely displaced.
  function captureTableState(movedKeys) {
    return {
      sort: table.order().map(function (o) {
        var c = colConfig[o[0]];
        return [c ? c.target : '', o[1]];
      }),
      page: table.page(),
      pageScroll: window.scrollY,
      sideScroll: tableContainer().scrollLeft() || 0,
      // The dialog's list is thousands of pixels tall and the rebuild empties
      // it, so without this a move made far down the list answers by throwing
      // the reader back to the top of it. The scroller is #col-manage-content;
      // #col-manage-list does not scroll.
      listScroll: $('#col-manage-content').scrollTop() || 0,
      movedKeys: movedKeys || null
    };
  }

  // A drag or a button press animates: the entries a rebuild relocates slide to
  // their new place, and every row they crossed slides the other way by
  // exactly the distance it moved (a FLIP - First, Last, Invert, Play). Read
  // before rebuildTableInOrder touches the dialog's rows, played after.
  function cmgFlipBefore() {
    if (!columnsDialogOpen()) return null;
    var tops = {};
    $('#col-manage-list .cmg-row').each(function () {
      tops[$(this).attr('data-key')] = this.getBoundingClientRect().top;
    });
    return tops;
  }
  function cmgFlipPlay(tops, movedKeys) {
    if (!tops || !columnsDialogOpen()) return;
    var reduced = reducedMotion.matches;
    var rows = $('#col-manage-list .cmg-row').get();
    var any = false;
    rows.forEach(function (el) {
      var key = el.getAttribute('data-key');
      var was = tops[key];
      var dy = (was === undefined) ? 0 : was - el.getBoundingClientRect().top;
      var highlight = movedKeys ? movedKeys.indexOf(key) !== -1 : Math.abs(dy) >= 0.5;
      if (Math.abs(dy) >= 0.5 && !reduced) {
        el.style.transition = 'none';
        el.style.transform = 'translateY(' + dy + 'px)';
        any = true;
      }
      if (highlight) {
        el.classList.add('cmg-moved');
        window.setTimeout(function () { el.classList.remove('cmg-moved'); }, 900);
      }
    });
    if (!any) return;
    rows[0].getBoundingClientRect();
    window.requestAnimationFrame(function () {
      rows.forEach(function (el) {
        if (!el.style.transform) return;
        el.style.transition = 'transform 220ms cubic-bezier(.2,0,0,1)';
        el.style.transform = '';
      });
    });
  }

  // The heading row is written once, before DataTables is started over the
  // table, and the extension that reorders columns is not vendored - so a new
  // order means tearing the table down and building it again. A reorder is
  // deliberate and rare, unlike the filter path, so the cost is paid here.
  function rebuildTableInOrder(state) {
    closeCellEditor(false);
    orderRebuild = state;
    var flipTops = cmgFlipBefore();
    table.destroy();
    table = null;
    $('#models thead tr').empty();
    $('#models tbody').empty();
    drawHeaders();
    fillTable();
    var sort = state.sort
      .map(function (o) { return [getColIdx(o[0]), o[1]]; })
      .filter(function (o) { return o[0] >= 0; });
    if (sort.length) table.order(sort);
    table.page(state.page).draw(false);
    buildFilterControls();
    restoreFilters();
    buildMobileSort();
    buildColumnChooser();
    cmgFlipPlay(flipTops, state.movedKeys);
    $('#col-manage-content').scrollTop(state.listScroll);
    tableContainer().scrollLeft(state.sideScroll);
    updateTopScrollWidth();
    window.scrollTo(0, state.pageScroll);
    orderRebuild = null;
  }

  // Two quick layout changes put two writes in flight and the FIRST answer can
  // arrive last, so a rollback is offered only by the newest of them: an older
  // one failing would restore a layout the reader has already moved past. This
  // is applyFilters's applySeq, shared by the order save and the visibility
  // save because each of them has something to put back.
  var layoutSaveSeq = 0;

  // The layout to go back to is named by VALUE (a layoutSnapshot), so two
  // quick presses cannot put back an order that was itself never stored.
  //
  // False when the write was refused before it left this page and the columns
  // are already back: its caller has a move to describe and describing it then
  // would put "moved to the front" into a screen reader immediately after the
  // assertive line saying nothing moved. onRolledBack runs after the page is
  // back, whichever way the refusal arrived.
  function saveOrder(before, state, onRolledBack) {
    var seq = ++layoutSaveSeq;
    var stored = true;
    saveSettings(function (e) {
      if (seq !== layoutSaveSeq) return;
      stored = false;
      restoreHidden(before);
      applyColOrder(before.shipped ? [] : before.targets);
      colOrderIsShipped = before.shipped;
      rebuildTableInOrder(state);
      if (onRolledBack) onRolledBack();
      statusErr('That order was not saved (' + (e.message || e) +
        '), so the columns are back the way they were.');
    });
    return stored;
  }

  function entryLabel(c) {
    return c ? (c.column ? c.column.label : c.title) : '';
  }

  // The one commit, whichever route asked for the move: a move button and a
  // header drag both land here, so the two can never disagree about what a move
  // writes or about the words it is announced in.
  function commitColumnMove(i, to, dir) {
    var before = layoutSnapshot();
    var entry = colConfig[i];
    var state = captureTableState([entry.target]);
    var crossed = colConfig[to];
    var label = entryLabel(entry);
    var editorWasOpen = !!openEditor || !!colorEditorKey;
    colConfig.splice(i, 1);
    colConfig.splice(to, 0, entry);
    colOrderIsShipped = false;
    closeColorEditor();
    rebuildTableInOrder(state);
    var step = rememberStep('moved ' + label, before);
    var stored = saveOrder(before, state, function () { forgetStep(step); });
    if (editorWasOpen) flashOk('The reorder closed the editor that was open.');
    // The columns are back and the refusal has already been said, in the region
    // that interrupts, so the move is not described.
    if (stored) {
      var msg;
      if (to === firstRealIndex()) msg = label + ' moved to the front';
      else if (to === lastRealIndex()) msg = label + ' moved to the end';
      else msg = label + ' moved ' + (dir === 'up' ? 'before ' : 'after ') + entryLabel(crossed);
      if (editorWasOpen) msg += '. The editor that was open was closed';
      announce('status-announce', msg);
    }
  }

  function moveColumn(key, dir) {
    var i = getColIdx(key);
    if (i < 0) return;
    var to = moveDestination(i, dir);
    if (to === null) return;
    commitColumnMove(i, to, dir);
    // The button pressed is still found and focused: a rollback rebuilt the
    // list under the reader's hands.
    var $button = $('#col-manage-list .cmg-move').filter(function () {
      return $(this).attr('data-key') === key && $(this).attr('data-dir') === dir;
    }).first();
    if (!$button.length) return;
    $button.trigger('focus');
    $button.get(0).scrollIntoView({ block: 'nearest', behavior: 'auto' });
  }

  // --- Dragging a heading ---------------------------------------------------
  // The short move on the table itself: press a heading, slide it over another
  // heading, let go, and the column takes the place that heading held. The
  // dialog's controls are the route for everything else, and for every reader
  // who is not using a pointer at all - nothing here is the only way to move a
  // column.
  //
  // Three things about the header make this different from a list drag, and all
  // three come from the pinned heading row. FixedHeader MOVES the real <thead>
  // to a fixed box on <body> and leaves a deep clone of it inside the table, so
  // while the heading is pinned every column has two <th> elements in the page.
  // So: the handlers are delegated on document, because a handler bound inside
  // the table never fires for the heading the reader can see; the column is read
  // off the element under the pointer and never looked up by attribute, because
  // a lookup can return the copy nobody is touching; and the landing mark is a
  // class on that same <th>, because an absolutely positioned line drawn inside
  // the table cannot mark a heading that lives on <body>.
  var headDrag = null;
  // A drag that moved something ends in a click on the heading, and a click on a
  // heading sorts the table. It is swallowed once, and forgotten on the next
  // frame if it never comes.
  var swallowNextClick = false;
  var HEAD_DRAG_START_PX = 5;
  var HEAD_EDGE_PX = 60;
  var HEAD_SCROLL_PER_SEC = 600;
  var HEAD_STEP_MS = 200;
  var reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');

  // Every route is refused while a model form is open, and a row drag in the
  // Columns dialog owns the move until it is placed or cancelled.
  function headerDragRefused() {
    return cardsQuery.matches || modelFormOpen() || !!rowDrag;
  }

  function thTargetOf(node) {
    var th = node && node.closest ? node.closest('th[data-target]') : null;
    return th ? { th: th, target: th.getAttribute('data-target') } : null;
  }

  // Where a drop may land: never before Name, and never on the column being
  // dragged. Dropping on a heading gives the dragged column the place that
  // heading holds right now, which is what the move buttons do one step at a
  // time.
  function headDropIndex(target) {
    if (!target || target === NAME_TARGET || target === headDrag.target) return -1;
    var to = getColIdx(target);
    return to < 1 ? -1 : to;
  }

  function drawHeadCaret(th) {
    if (headDrag.caret === th) return;
    if (headDrag.caret) $(headDrag.caret).removeClass('th-drop-target');
    headDrag.caret = th;
    if (th) $(th).addClass('th-drop-target');
  }

  // The table scrolls sideways inside .dt-container, and a drag held against
  // either edge scrolls it, so a column can be taken past the edge of the
  // screen. Under prefers-reduced-motion it jumps a step at a time instead of
  // sliding.
  function headDragScroll(now) {
    if (!headDrag) return;
    headDrag.raf = window.requestAnimationFrame(headDragScroll);
    var c = tableContainer()[0];
    if (!c) return;
    var rect = c.getBoundingClientRect();
    var dir = 0;
    if (headDrag.x < rect.left + HEAD_EDGE_PX) dir = -1;
    else if (headDrag.x > rect.right - HEAD_EDGE_PX) dir = 1;
    if (!dir) { headDrag.scrollAt = 0; return; }
    if (!headDrag.scrollAt) { headDrag.scrollAt = now; return; }
    var elapsed = now - headDrag.scrollAt;
    var step;
    if (reducedMotion.matches) {
      if (elapsed < HEAD_STEP_MS) return;
      step = dir * HEAD_EDGE_PX;
    } else {
      // A frame the browser was late for must not send the table flying.
      step = dir * HEAD_SCROLL_PER_SEC * Math.min(elapsed, 50) / 1000;
    }
    headDrag.scrollAt = now;
    tableContainer().scrollLeft(c.scrollLeft + step);
  }

  // Nothing is written and nothing is announced beyond the words given: a
  // cancel leaves the order exactly as it was.
  function endHeadDrag(msg) {
    if (!headDrag) return;
    drawHeadCaret(null);
    $(headDrag.from).removeClass('th-dragging');
    if (headDrag.raf) window.cancelAnimationFrame(headDrag.raf);
    var moved = headDrag.moved;
    headDrag = null;
    if (moved) {
      swallowNextClick = true;
      window.requestAnimationFrame(function () { swallowNextClick = false; });
    }
    // The dialog list is not rebuilt under a drag, so a rebuild the drag held
    // off is paid now.
    if (chooserRebuildDue && !rowDrag) { chooserRebuildDue = false; buildColumnChooser(); }
    if (msg) announce('status-announce', msg);
  }

  $(document).on('pointerdown', 'th[data-target]', function (e) {
    var ev = e.originalEvent || e;
    if (ev.button !== 0 || headDrag) return;
    if (headerDragRefused()) return;
    var found = thTargetOf(ev.target);
    if (!found || found.target === NAME_TARGET) return;
    headDrag = {
      target: found.target, from: found.th, pointerId: ev.pointerId,
      startX: ev.clientX, startY: ev.clientY, x: ev.clientX,
      moved: false, caret: null, scrollAt: 0, raf: 0
    };
  });

  $(document).on('pointermove', function (e) {
    if (!headDrag) return;
    var ev = e.originalEvent || e;
    if (ev.pointerId !== headDrag.pointerId) return;
    headDrag.x = ev.clientX;
    if (!headDrag.moved) {
      // Below the threshold this is still a click, and a click on a heading
      // sorts the table.
      if (Math.abs(ev.clientX - headDrag.startX) < HEAD_DRAG_START_PX &&
        Math.abs(ev.clientY - headDrag.startY) < HEAD_DRAG_START_PX) return;
      headDrag.moved = true;
      // A press and a slide selects the heading text on the way past.
      var sel = window.getSelection && window.getSelection();
      if (sel && sel.removeAllRanges) sel.removeAllRanges();
      $(headDrag.from).addClass('th-dragging');
      headDrag.raf = window.requestAnimationFrame(headDragScroll);
    }
    e.preventDefault();
    var over = thTargetOf(document.elementFromPoint(ev.clientX, ev.clientY));
    drawHeadCaret(over && headDropIndex(over.target) >= 0 ? over.th : null);
  });

  $(document).on('pointerup', function (e) {
    if (!headDrag) return;
    var ev = e.originalEvent || e;
    if (ev.pointerId !== headDrag.pointerId) return;
    if (!headDrag.moved) { endHeadDrag(''); return; }
    var i = getColIdx(headDrag.target);
    var to = headDrag.caret ? headDropIndex(headDrag.caret.getAttribute('data-target')) : -1;
    var over = thTargetOf(document.elementFromPoint(ev.clientX, ev.clientY));
    var onItself = !!over && over.target === headDrag.target;
    if (to < 0 || i < 0 || to === i) {
      // Back where it started is a move that did nothing; anywhere else is a
      // release over nothing, which cancels.
      endHeadDrag(onItself ? 'Nothing moved.' : 'The move was cancelled.');
      return;
    }
    var dir = to > i ? 'down' : 'up';
    endHeadDrag('');
    commitColumnMove(i, to, dir);
  });

  $(document).on('pointercancel', function (e) {
    var ev = e.originalEvent || e;
    if (headDrag && ev.pointerId === headDrag.pointerId) endHeadDrag('The move was cancelled.');
  });

  $(document).on('keydown', function (e) {
    if (headDrag && e.key === 'Escape') endHeadDrag('The move was cancelled.');
  });

  // In the capture phase, so the click never reaches the sort handler
  // DataTables binds on the heading.
  document.addEventListener('click', function (e) {
    if (!swallowNextClick) return;
    swallowNextClick = false;
    e.stopPropagation();
    e.preventDefault();
  }, true);

  // --- Moving rows: buttons, drag, and multi-select --------------------------
  // A drop, Shift plus a move button and a group send to an edge commit
  // through commitReorder: one undo step, the order saved, the same
  // announcements. The up and down buttons (moveColumn/commitColumnMove
  // above) are the keyboard's route.

  // A row's move button, found by comparing attributes rather than by building
  // a selector, because a column key is free text and may hold a quote.
  function moveButtonFor(key, dir) {
    return $('#col-manage-list .cmg-move').filter(function () {
      return $(this).attr('data-key') === key && $(this).attr('data-dir') === dir;
    });
  }

  // Says where a finished move landed, and what else it changed.
  function announceMove(label, entries, noneDrawn, editorWasOpen) {
    var landed = colConfig.indexOf(entries[0]);
    var after = colConfig.indexOf(entries[entries.length - 1]) + 1;
    var msg;
    if (landed === firstRealIndex()) msg = label + ' moved to the front';
    else if (!colConfig[after] || !colConfig[after].column) msg = label + ' moved to the end';
    else msg = label + ' moved before ' + colConfig[after].column.label;
    if (noneDrawn) msg += '. ' + label + ' is not drawn at this width, so nothing on screen changed';
    if (editorWasOpen) msg += '. The editor that was open was closed';
    announce('status-announce', msg);
  }

  function labelOf(key) {
    return (columnByKey[key] || {}).label || key;
  }

  // Why a drag, a button or a selection will not act, in the words every
  // refusal on this list shares.
  function reorderRefusal() {
    if (cmgState.sort !== 'table') return 'only in Table order';
    if (modelFormOpen()) return 'not while a model form is open';
    return '';
  }

  // The one place a sort is set, whether the select or a header button asked
  // for it: a sorted list's own order contradicts the table's, so nothing can
  // be held or dragged in one and whatever was picked up is put down first.
  function applyCmgSort(sort) {
    if (cmgState.sort === sort) return;
    cmgState.sort = sort;
    $('#cmg-sort').val(sort);
    saveCmgFilters();
    clearSelection('');
    if (rowDrag) { endRowDragVisuals(); rowDrag = null; }
    cancelTouchPress();
    buildColumnChooser();
  }
  $('#cmg-sort').on('change', function () { applyCmgSort($(this).val()); });
  $('#col-manage-list').on('click', '.cmg-th-btn', function () {
    applyCmgSort($(this).attr('data-sort'));
  });

  // --- Several columns at once ----------------------------------------------
  // A selection is a set of columns that move together. It exists in this
  // dialog only, only in Table order, and only for this visit: closing the
  // dialog throws it away with the rest of cmgState. It is built by Ctrl and a
  // click on the row surface, by Shift and a click to extend it, or by a long
  // press on touch.

  // The members that still exist, in the table's own order. A column the reader
  // deleted while it was selected leaves the selection here rather than by a
  // handler somewhere else remembering to take it out.
  function selectedKeys() {
    return cmgState.selection
      .filter(function (key) { return getColIdx(key) >= 0; })
      .sort(function (a, b) { return getColIdx(a) - getColIdx(b); });
  }
  function isSelected(key) { return cmgState.selection.indexOf(key) !== -1; }

  // The keys the list is showing right now, in the order it drew them. Shift
  // extends across THESE, because the rows between two rows on screen are what
  // the reader is pointing at, whatever the full order holds between them.
  function listedKeys() {
    return $('#col-manage-list .cmg-row').filter(':visible').map(function () {
      return $(this).attr('data-key');
    }).get();
  }

  function rowFor(key) {
    return $('#col-manage-list .cmg-row').filter(function () { return $(this).attr('data-key') === key; });
  }
  function tickFor(key) {
    return $('#col-manage-list .col-show').filter(function () { return $(this).attr('data-target') === key; });
  }
  // Every route into a selection moves focus onto the row's own tick: a
  // Ctrl-click lands on a title span (tabindex="-1", which Chrome never
  // focuses on its own), and a long press or a drag start moves no focus at
  // all by default. Escape is bound to the document (cmgEscapeHandler), but a
  // key event needs a focused element to start from.
  function focusRowTick(key) {
    var $tick = tickFor(key);
    if ($tick.length) $tick.trigger('focus');
  }

  function setSelection(keys, anchor) {
    cmgState.selection = keys;
    if (anchor !== undefined) cmgState.anchor = anchor;
    drawSelectionState();
  }

  // Empties the selection. msg is said out loud, because the bar's own count
  // line goes empty and an empty status region says nothing at all.
  function clearSelection(msg) {
    if (!cmgState.selection.length) return;
    cmgState.selection = [];
    cmgState.anchor = null;
    drawSelectionState();
    if (msg) announce('status-announce', msg);
  }

  function toggleSelect(key) {
    if (reorderRefusal()) return;
    var next = cmgState.selection.filter(function (k) { return k !== key; });
    if (next.length === cmgState.selection.length) next.push(key);
    setSelection(next, key);
    focusRowTick(key);
  }

  // From the anchor to this row, over the rows the list is showing. Without an
  // anchor there is nothing to extend from, so the press selects this row alone
  // and becomes the anchor.
  function extendSelectTo(key) {
    if (reorderRefusal()) return;
    var listed = listedKeys();
    var from = listed.indexOf(cmgState.anchor);
    var to = listed.indexOf(key);
    if (from < 0 || to < 0) { setSelection([key], key); focusRowTick(key); return; }
    var lo = Math.min(from, to), hi = Math.max(from, to);
    setSelection(listed.slice(lo, hi + 1), cmgState.anchor);
    focusRowTick(key);
  }

  function selectionWord(n) {
    return n === 1 ? '1 column selected' : n + ' columns selected';
  }

  // Why the block move on a selected row's own button will not act: the block
  // moves as one, so the refusal is the leading member's.
  function groupRefusal(dir) {
    var keys = selectedKeys();
    if (!keys.length) return 'nothing is selected';
    var key = dir === 'up' ? keys[0] : keys[keys.length - 1];
    return moveRefusal(getColIdx(key), dir);
  }

  // The bar's half of a selection: the count and Clear. The row tint, the
  // accent bar and every move button's name are refreshed with it.
  function drawSelectionState() {
    var keys = selectedKeys();
    $('#cmg-selected').text(keys.length ? selectionWord(keys.length) : '');
    $('#cmg-sel-clear').prop('hidden', !keys.length);
    $('#col-manage-list').toggleClass('cmg-select-mode', keys.length > 0);
    $('#col-manage-list .cmg-row').each(function () {
      $(this).toggleClass('is-selected', isSelected($(this).attr('data-key')));
    });
    refreshMoveControls();
  }

  // The block move: the whole selection steps one drawn place, and the
  // selection SURVIVES it, because putting a set of columns where the reader
  // wants them is nearly always several presses of the same button.
  function groupMove(dir) {
    if (groupRefusal(dir)) return;
    var keys = selectedKeys();
    var entries = keys.map(function (k) { return colConfig[getColIdx(k)]; });
    // The block's leading member is what crosses on the way up and its
    // trailing member on the way down, so nothing it steps over is itself a
    // member and the members coalesce into one run wherever they started.
    var edge = dir === 'up' ? keys[0] : keys[keys.length - 1];
    var to = moveDestination(getColIdx(edge), dir);
    if (to === null) return;
    var crossed = colConfig[to];
    var rest = colConfig.filter(function (c) { return entries.indexOf(c) === -1; });
    var at = rest.indexOf(crossed) + (dir === 'down' ? 1 : 0);
    var next = rest.slice(0, at).concat(entries, rest.slice(at));
    var before = layoutSnapshot();
    if (sameOrder(before.targets, next.map(function (c) { return c.target; }))) return;
    var label = keys.length === 1 ? labelOf(keys[0]) : keys.length + ' columns';
    var state = captureTableState(keys);
    var noneDrawn = keys.every(function (k) { return !columnDrawn(k); });
    var editorWasOpen = !!openEditor || !!colorEditorKey;
    colConfig = next;
    colOrderIsShipped = false;
    closeColorEditor();
    rebuildTableInOrder(state);
    var step = rememberStep('moved ' + label, before);
    var stored = saveOrder(before, state, function () { forgetStep(step); });
    drawSelectionState();
    focusByIdentity([function () { return moveButtonFor(edge, dir); }, '#cmg-search']);
    if (editorWasOpen) flashOk('The reorder closed the editor that was open.');
    // The columns are back and the refusal has already been said, in the region
    // that interrupts, so the move is not described.
    if (!stored) return;
    announceMove(label, entries, noneDrawn, editorWasOpen);
  }

  $('#cmg-sel-clear').on('click', function () { clearSelection('The selection was cleared.'); });

  // Ctrl+click on a row's surface toggles it, Shift+click extends. Ctrl+click
  // on the tick itself still ticks, because a click on a checkbox is a browser
  // gesture this dialog does not own - closest() below excludes it. A plain
  // click on the row does nothing: it is the drag surface.
  $('#col-manage-list').on('click', '.cmg-row', function (e) {
    if ($(e.target).closest('button, input, label, a').length) return;
    if (!e.ctrlKey && !e.metaKey && !e.shiftKey) return;
    if (reorderRefusal()) return;
    var key = $(this).attr('data-key');
    if (!key) return;
    if (e.shiftKey) { extendSelectTo(key); return; }
    toggleSelect(key);
  });

  // ArrowUp and ArrowDown on a row's tick walk to the previous or next row's
  // tick; Shift extends the selection from there; Ctrl+Space toggles it. Arrow
  // keys do nothing native on a lone checkbox, so this is the whole of the
  // keyboard's route across the list.
  $('#col-manage-list').on('keydown', '.col-show', function (e) {
    if (e.ctrlKey && (e.key === ' ' || e.key === 'Spacebar')) {
      e.preventDefault();
      toggleSelect($(this).attr('data-target'));
      return;
    }
    if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
    var $ticks = $('#col-manage-list .col-show').filter(':visible');
    var $next = $ticks.eq($ticks.index(this) + (e.key === 'ArrowUp' ? -1 : 1));
    if (!$next.length) return;
    e.preventDefault();
    if (e.shiftKey) {
      var key = $(this).attr('data-target');
      if (!cmgState.anchor) cmgState.anchor = key;
      extendSelectTo($next.attr('data-target'));
    }
    $next.trigger('focus');
    $next.get(0).scrollIntoView({ block: 'nearest', behavior: 'auto' });
  });

  // --- Dragging a row ----------------------------------------------------
  // pointerdown on the row surface (not a control), then 5px of movement,
  // starts a drag: a selected row drags the whole selection, any other row
  // drags alone and clears the selection. On touch, the same gesture is a
  // long press first (500ms, under 10px), which selects the row; the same
  // finger, still down, becomes the drag the moment it moves further.
  var rowDrag = null;
  var touchPress = null;
  // A rebuild that fired while a row was mid-drag, owed until the drag ends:
  // the list must not change under a reader's finger or pointer.
  var chooserRebuildDue = false;
  var ROW_DRAG_START_PX = 5;
  var ROW_EDGE_PX = 60;
  var ROW_SCROLL_PER_SEC = 600;
  var TOUCH_LONGPRESS_MS = 500;
  var TOUCH_SLOP_PX = 10;

  function endRowDragVisuals() {
    if (rowDrag && rowDrag.raf) window.cancelAnimationFrame(rowDrag.raf);
    $('#col-manage-list').removeClass('cmg-dragging');
    $('#col-manage-list .cmg-row').removeClass('is-dragging cmg-drop-before cmg-drop-after');
  }
  function clearDropLines() {
    $('#col-manage-list .cmg-row').removeClass('cmg-drop-before cmg-drop-after');
  }
  function swallowDragClick() {
    swallowNextClick = true;
    window.requestAnimationFrame(function () { swallowNextClick = false; });
  }
  function cancelTouchPress() {
    if (touchPress && touchPress.timer) window.clearTimeout(touchPress.timer);
    touchPress = null;
  }
  // A rebuild another change queued while this drag or long press held it off.
  function payRebuildIfDue() {
    if (chooserRebuildDue && !rowDrag && !touchPress) { chooserRebuildDue = false; buildColumnChooser(true); }
  }

  // The next colConfig entry after `key` that is a real column, or null at the
  // true end - what "drop after the last LISTED row" has to mean when a
  // search or a filter is hiding the column that really follows it.
  function afterAnchorKey(key) {
    var i = getColIdx(key);
    for (var j = i + 1; j < colConfig.length; j++) {
      if (colConfig[j].column) return colConfig[j].target;
    }
    return null;
  }

  // Where a drop lands, from the pointer's own position: the row under it,
  // upper or lower half, or the nearest boundary row when the pointer has
  // left the list entirely (past the first row's top or the last row's
  // bottom).
  function rowDropInfo(x, y) {
    var $rows = $('#col-manage-list .cmg-row').filter(':visible');
    if (!$rows.length) return null;
    var el = document.elementFromPoint(x, y);
    var rowEl = el && el.closest ? el.closest('.cmg-row') : null;
    if (rowEl && $(rowEl).is(':visible')) {
      var key = rowEl.getAttribute('data-key');
      if (rowDrag.keys.indexOf(key) !== -1) return { onSelf: true, row: rowEl };
      var rect = rowEl.getBoundingClientRect();
      return { row: rowEl, key: key, edge: y < rect.top + rect.height / 2 ? 'before' : 'after' };
    }
    var first = $rows.first()[0];
    var last = $rows.last()[0];
    if (y <= first.getBoundingClientRect().top) return { row: first, key: first.getAttribute('data-key'), edge: 'before' };
    if (y >= last.getBoundingClientRect().bottom) return { row: last, key: last.getAttribute('data-key'), edge: 'after' };
    return null;
  }

  function updateDropLine() {
    clearDropLines();
    var info = rowDropInfo(rowDrag.x, rowDrag.y);
    rowDrag.dropInfo = info;
    if (!info || info.onSelf) return;
    $(info.row).addClass(info.edge === 'before' ? 'cmg-drop-before' : 'cmg-drop-after');
  }

  function rowDragScroll(now) {
    if (!rowDrag) return;
    rowDrag.raf = window.requestAnimationFrame(rowDragScroll);
    var c = document.getElementById('col-manage-content');
    if (!c) return;
    var rect = c.getBoundingClientRect();
    var dir = 0;
    if (rowDrag.y < rect.top + ROW_EDGE_PX) dir = -1;
    else if (rowDrag.y > rect.bottom - ROW_EDGE_PX) dir = 1;
    if (!dir) { rowDrag.scrollAt = 0; return; }
    if (!rowDrag.scrollAt) { rowDrag.scrollAt = now; return; }
    var elapsed = now - rowDrag.scrollAt;
    var step;
    if (reducedMotion.matches) {
      if (elapsed < HEAD_STEP_MS) return;
      step = dir * ROW_EDGE_PX;
    } else {
      step = dir * ROW_SCROLL_PER_SEC * Math.min(elapsed, 50) / 1000;
    }
    rowDrag.scrollAt = now;
    c.scrollTop = c.scrollTop + step;
  }

  // The order colConfig would hold with `entries` taken out and put back
  // contiguously before `anchorEntry`, or at the end of the real columns where
  // there is no anchor. Every dragged or block-moved column travels as one
  // block, in the order it holds in the table and never in the order the rows
  // were clicked, and the unselected columns that sat between the members end
  // up on one side of it.
  function orderWithBlockBefore(entries, anchorEntry) {
    var rest = colConfig.filter(function (c) { return entries.indexOf(c) === -1; });
    var at = rest.length;
    if (anchorEntry) at = rest.indexOf(anchorEntry);
    else {
      for (var i = rest.length - 1; i >= 0; i--) { if (rest[i].column) { at = i + 1; break; } }
    }
    return rest.slice(0, at).concat(entries, rest.slice(at));
  }

  function sameOrder(a, b) {
    return a.length === b.length && a.every(function (t, i) { return t === b[i]; });
  }

  // The one commit, whichever route asked: a button (moveColumn/groupMove
  // above) or a drop here. Same undo step, same saved order, same words.
  function commitReorder(keys, beforeKey) {
    if (reorderRefusal()) return;
    keys = keys.filter(function (k) { return getColIdx(k) >= 0; });
    if (!keys.length) { announce('status-announce', 'The move was cancelled.'); return; }
    var anchorEntry = null;
    if (beforeKey !== null) {
      var j = getColIdx(beforeKey);
      if (j < 0) { announce('status-announce', 'The move was cancelled.'); return; }
      anchorEntry = colConfig[j];
    }
    var label = keys.length === 1 ? labelOf(keys[0]) : keys.length + ' columns';
    var entries = keys.map(function (k) { return colConfig[getColIdx(k)]; });
    var before = layoutSnapshot();
    var next = orderWithBlockBefore(entries, anchorEntry);
    if (sameOrder(before.targets, next.map(function (c) { return c.target; }))) {
      announce('status-announce', 'Nothing moved.');
      return;
    }
    var state = captureTableState(keys);
    var noneDrawn = keys.every(function (k) { return !columnDrawn(k); });
    var editorWasOpen = !!openEditor || !!colorEditorKey;
    cmgState.selection = [];
    cmgState.anchor = null;
    colConfig = next;
    colOrderIsShipped = false;
    closeColorEditor();
    rebuildTableInOrder(state);
    var step = rememberStep('moved ' + label, before);
    var stored = saveOrder(before, state, function () { forgetStep(step); });
    drawSelectionState();
    focusByIdentity([function () { return tickFor(keys[0]); }, '#cmg-search']);
    if (editorWasOpen) flashOk('The reorder closed the editor that was open.');
    if (!stored) return;
    announceMove(label, entries, noneDrawn, editorWasOpen);
  }

  function startRowDragCandidate(ev, keys) {
    rowDrag = {
      keys: keys, pointerId: ev.pointerId, startX: ev.clientX, startY: ev.clientY,
      x: ev.clientX, y: ev.clientY, moved: false, dropInfo: null, scrollAt: 0, raf: 0
    };
  }

  $(document).on('pointerdown', '.cmg-row', function (e) {
    var ev = e.originalEvent || e;
    if (!columnsDialogOpen() || rowDrag || touchPress) return;
    if ($(ev.target).closest('button, input, label, a').length) return;
    var key = $(this).attr('data-key');
    if (!key) return;

    if (ev.pointerType === 'mouse') {
      // Refused or not, the candidate still starts: a drag never actually
      // engages under a refusal, but the first 5px of movement is what says
      // so (pointermove below), and silence until then would read as nothing
      // happening at all.
      if (ev.button !== 0) return;
      startRowDragCandidate(ev, isSelected(key) ? selectedKeys() : [key]);
      return;
    }
    if (ev.pointerType !== 'touch') return;
    if (reorderRefusal()) return;

    if (isSelected(key)) {
      // A finger on an already-selected row drags the selection straight
      // away - the row's own touch-action: none is what let this handler see
      // the move instead of the browser scrolling under it.
      startRowDragCandidate(ev, selectedKeys());
      return;
    }
    if (cmgState.selection.length) {
      // Select mode is already on: a plain tap toggles this row.
      touchPress = { mode: 'tap', key: key, startX: ev.clientX, startY: ev.clientY, pointerId: ev.pointerId };
      return;
    }
    // Nothing selected yet: this press may become a long-press select.
    touchPress = { mode: 'hold', key: key, startX: ev.clientX, startY: ev.clientY, pointerId: ev.pointerId };
    touchPress.timer = window.setTimeout(function () {
      if (!touchPress || touchPress.mode !== 'hold' || reorderRefusal()) { touchPress = null; return; }
      var k = touchPress.key, pid = touchPress.pointerId, sx = touchPress.startX, sy = touchPress.startY;
      touchPress = null;
      setSelection([k], k);
      focusRowTick(k);
      if (navigator.vibrate) navigator.vibrate(30);
      announce('status-announce', labelOf(k) + ' selected. Tap other rows to add them, then drag one of them up or down.');
      // The same finger, still down, becomes the drag once it moves.
      rowDrag = { keys: [k], pointerId: pid, startX: sx, startY: sy, x: sx, y: sy, moved: false, dropInfo: null, scrollAt: 0, raf: 0 };
    }, TOUCH_LONGPRESS_MS);
  });

  $(document).on('pointermove', function (e) {
    var ev = e.originalEvent || e;
    if (touchPress && ev.pointerId === touchPress.pointerId) {
      if (touchPress.mode === 'hold' &&
        (Math.abs(ev.clientX - touchPress.startX) >= TOUCH_SLOP_PX ||
          Math.abs(ev.clientY - touchPress.startY) >= TOUCH_SLOP_PX)) {
        cancelTouchPress();
      }
      return;
    }
    if (!rowDrag) return;
    if (ev.pointerId !== rowDrag.pointerId) return;
    rowDrag.x = ev.clientX; rowDrag.y = ev.clientY;
    if (!rowDrag.moved) {
      if (Math.abs(ev.clientX - rowDrag.startX) < ROW_DRAG_START_PX &&
        Math.abs(ev.clientY - rowDrag.startY) < ROW_DRAG_START_PX) return;
      if (reorderRefusal()) {
        announce('status-announce', 'Reordering works only in Table order. Sort by Table order to move columns.');
        rowDrag = null;
        return;
      }
      rowDrag.moved = true;
      var sel = window.getSelection && window.getSelection();
      if (sel && sel.removeAllRanges) sel.removeAllRanges();
      $('#col-manage-list').addClass('cmg-dragging');
      rowDrag.keys.forEach(function (k) { rowFor(k).addClass('is-dragging'); });
      rowDrag.raf = window.requestAnimationFrame(rowDragScroll);
      focusRowTick(rowDrag.keys[0]);
    }
    e.preventDefault();
    updateDropLine();
  });

  $(document).on('pointerup', function (e) {
    var ev = e.originalEvent || e;
    if (touchPress && ev.pointerId === touchPress.pointerId) {
      var tp = touchPress;
      cancelTouchPress();
      if (tp.mode === 'tap') {
        var movedFar = Math.abs(ev.clientX - tp.startX) >= TOUCH_SLOP_PX || Math.abs(ev.clientY - tp.startY) >= TOUCH_SLOP_PX;
        if (!movedFar) toggleSelect(tp.key);
      }
      payRebuildIfDue();
      return;
    }
    if (!rowDrag) return;
    if (ev.pointerId !== rowDrag.pointerId) return;
    if (!rowDrag.moved) { rowDrag = null; payRebuildIfDue(); return; }
    var info = rowDrag.dropInfo;
    var keys = rowDrag.keys;
    endRowDragVisuals();
    rowDrag = null;
    if (!info) { announce('status-announce', 'The move was cancelled.'); payRebuildIfDue(); return; }
    if (info.onSelf) { announce('status-announce', 'Nothing moved.'); swallowDragClick(); payRebuildIfDue(); return; }
    swallowDragClick();
    commitReorder(keys, info.edge === 'before' ? info.key : afterAnchorKey(info.key));
  });

  $(document).on('pointercancel', function (e) {
    var ev = e.originalEvent || e;
    if (touchPress && ev.pointerId === touchPress.pointerId) { cancelTouchPress(); payRebuildIfDue(); return; }
    if (rowDrag && ev.pointerId === rowDrag.pointerId) {
      var moved = rowDrag.moved;
      endRowDragVisuals();
      rowDrag = null;
      if (moved) announce('status-announce', 'The move was cancelled.');
      payRebuildIfDue();
    }
  });

  // Escape, in order: an open kind tip or dropdown closes; a drag or a long
  // press cancels; a selection clears; then the dialog closes. One handler
  // covers every case, bound to the DOCUMENT rather than to the dialog
  // element, and only while the dialog is open (openColumnsDialog binds it,
  // closeColumnsDialog unbinds it): a mouse drag never carries keyboard
  // focus, and neither does a row selected by Ctrl-click landing on its
  // tabindex="-1" title span, so a handler scoped to a focused descendant of
  // #col-manage-modal would never see the key in either case.
  function cmgEscapeHandler(e) {
    if (e.key !== 'Escape') return;
    // The Colours dialog is modal and closes itself on Escape; its close event
    // restores focus. This handler must leave the key alone while it is open.
    if (colorDialog.open) return;
    e.preventDefault();
    var $openTip = $('.cmg-kind-btn[aria-expanded="true"]');
    if ($openTip.length) { $openTip.attr('aria-expanded', 'false'); $openTip.trigger('focus'); return; }
    var $openDd = $('.cmg-dd-panel').filter(function () { return !$(this).prop('hidden'); });
    if ($openDd.length) {
      var $btn = $openDd.closest('.cmg-dd').find('.cmg-dd-btn');
      closeCmgDropdowns();
      $btn.trigger('focus');
      return;
    }
    if (rowDrag) {
      var moved = rowDrag.moved;
      endRowDragVisuals();
      rowDrag = null;
      if (moved) announce('status-announce', 'The move was cancelled.');
      payRebuildIfDue();
      return;
    }
    if (touchPress) {
      cancelTouchPress();
      payRebuildIfDue();
      return;
    }
    if (cmgState.selection.length) {
      clearSelection('The selection was cleared.');
      return;
    }
    closeColumnsDialog();
  }

  // --- Steps back and forward -----------------------------------------------
  // Every change to the view's layout made in this dialog, or by dragging a
  // heading on the table, is a step: a tick, the Shown tick over the whole
  // list, a placement, a move, Reset order. Each step keeps the layout as it
  // stood BEFORE it, by value, because colConfig is replaced and hiddenTargets
  // is mutated in place and replaced on a view switch, so a reference to either
  // is not a record. Undo walks back through them and Redo forward, the way
  // every editor does. The stacks are page memory: a reload empties them, and a
  // view switch empties them too, because a layout belongs to the view it was
  // arranged in and must never be offered inside another.
  //
  // Never Ctrl+Z: that is the cell-edit history, and a shortcut that sometimes
  // restores a value and sometimes moves a column is one nobody can press with
  // confidence.
  var undoSteps = [];
  var redoSteps = [];
  var LAYOUT_STEPS_KEPT = 50;

  // The layout as it stands, copied out of the live objects.
  function layoutSnapshot() {
    return {
      targets: colConfig.map(function (c) { return c.target; }),
      shipped: colOrderIsShipped,
      hidden: Object.keys(hiddenTargets),
      filterOff: Object.keys(filterOffTargets),
      // The conditions as they stand, so undoing the removal of a filter can
      // put back the condition that removal cleared.
      filters: JSON.parse(JSON.stringify(keptFilters))
    };
  }
  // Puts back the two sets a snapshot holds: the columns out of the table and the
  // columns with no filter in the panel.
  //
  // A condition whose control comes back with the snapshot comes back with it:
  // taking a filter out cleared its condition, and putting the filter back must
  // leave the rows as they were. filtersRestored tells the caller to read the
  // rows again.
  var filtersRestored = false;
  function restoreHidden(snap) {
    var controlBefore = {};
    Object.keys(snap.filters || {}).forEach(function (key) { controlBefore[key] = filterHasControl(key); });
    hiddenTargets = {};
    snap.hidden.forEach(function (key) { hiddenTargets[key] = true; });
    filterOffTargets = {};
    snap.filterOff.forEach(function (key) { filterOffTargets[key] = true; });
    Object.keys(snap.filters || {}).forEach(function (key) {
      if (controlBefore[key] || !filterHasControl(key) || keptFilters[key] !== undefined) return;
      keptFilters[key] = JSON.parse(JSON.stringify(snap.filters[key]));
      activeQuery = {};
      filtersRestored = true;
    });
  }

  // Reads the rows again when restoreHidden put a condition back.
  function applyRestoredFilters() {
    if (!filtersRestored) return;
    filtersRestored = false;
    applyFilters(true);
  }

  function drawUndoState() {
    function drawStepButton(id, verb, steps) {
      var top = steps[steps.length - 1];
      var name = top ? verb + ': ' + top.what : verb + ' (nothing to put back)';
      $(id).attr('aria-disabled', top ? 'false' : 'true')
        .attr('aria-label', name)
        .attr('title', name);
    }
    drawStepButton('#cmg-undo', 'Undo', undoSteps);
    drawStepButton('#cmg-redo', 'Redo', redoSteps);
  }
  drawUndoState();

  // Records one step: `what` says what the change did, in the words the Undo
  // button will carry ("hid 12 columns"), and `before` is the layoutSnapshot
  // taken before the change was made. A new step forgets the redo history.
  // The token handed back is what forgetStep takes when the step's save is
  // refused and the page is rolled back, so the stack never holds a step the
  // store never saw.
  function rememberStep(what, before) {
    var step = { what: what, targets: before.targets, shipped: before.shipped, hidden: before.hidden, filterOff: before.filterOff, filters: before.filters };
    var token = { step: step, redoBefore: redoSteps };
    undoSteps.push(step);
    redoSteps = [];
    if (undoSteps.length > LAYOUT_STEPS_KEPT) undoSteps.shift();
    drawUndoState();
    return token;
  }

  function forgetStep(token) {
    var at = undoSteps.indexOf(token.step);
    if (at < 0) return;
    undoSteps.splice(at, 1);
    // The redo history the step threw away comes back only when the step was
    // the newest thing on the stack, which is the only case where nothing has
    // been done since that would have thrown it away anyway.
    if (at === undoSteps.length) redoSteps = token.redoBefore;
    drawUndoState();
  }

  // A view switch. A layout save still in flight belongs to the view being
  // left, and its rollback would paint that view's layout onto the arriving
  // view's table, where the next save would store it under the wrong name, so
  // the switch stands every pending rollback down along with the steps.
  function forgetSteps() {
    undoSteps = [];
    redoSteps = [];
    layoutSaveSeq++;
    drawUndoState();
  }

  // Puts a snapshot back on the page and stores it, as one save. A snapshot
  // whose order is the one on screen goes through the visibility path, which
  // patches the table's columns in place; any other goes through the rebuild
  // an order change already pays. onRolledBack is called when that save is
  // refused and the page has been put back the way it was.
  function restoreLayout(snap, onRolledBack) {
    var before = layoutSnapshot();
    restoreHidden(snap);
    if (snap.shipped === before.shipped && sameOrder(snap.targets, before.targets)) {
      return columnsShownChanged(before, onRolledBack);
    }
    var state = captureTableState();
    applyColOrder(snap.shipped ? [] : snap.targets);
    colOrderIsShipped = snap.shipped;
    closeColorEditor();
    rebuildTableInOrder(state);
    applyRestoredFilters();
    return saveOrder(before, state, onRolledBack);
  }

  // Undo and Redo are one movement in two directions: the step leaves one
  // stack, the layout on screen goes onto the other under the step's own
  // words, and the step's layout goes onto the page. A refused save reverses
  // the movement, so the two stacks always describe what the store holds.
  function walkSteps(from, onto, verb) {
    var step = from.pop();
    if (!step) return;
    if (rowDrag) { endRowDragVisuals(); rowDrag = null; }
    cancelTouchPress();
    clearSelection('');
    var back = layoutSnapshot();
    back.what = step.what;
    onto.push(back);
    drawUndoState();
    // Nothing is announced when the write was refused: the layout is back
    // where it started and the refusal has already interrupted.
    if (restoreLayout(step, function () { onto.pop(); from.push(step); drawUndoState(); })) {
      announce('status-announce', verb + ': ' + step.what);
    }
  }
  $('#cmg-undo').on('click', function () { walkSteps(undoSteps, redoSteps, 'Undone'); });
  $('#cmg-redo').on('click', function () { walkSteps(redoSteps, undoSteps, 'Redone'); });

  // The answers cell: the bare number, right-aligned, or nothing where the
  // catalog has not been counted yet. Its title says what the figure counts;
  // an sr-only " answers" follows the number so a screen reader hears what it
  // counts.
  function drawAnswersCell($td, column) {
    $td.empty();
    if (!answersAreCountable()) {
      $td.attr('title', catalogEmptinessFailed
        ? 'Answers could not be counted: reading the catalog failed'
        : 'Answers are not counted yet');
      return;
    }
    var n = columnHasValues[column.key] || 0;
    if (n) {
      $td.attr('title', n.toLocaleString() + (n === 1 ? ' answer' : ' answers') +
        ', counted over every model and build in the catalog, hidden ones included');
    } else {
      $td.attr('title', isYours(column)
        ? 'Nothing here yet: use the pencil in a row to fill it in'
        : 'Nothing has stated this yet');
    }
    $td.append(document.createTextNode(n.toLocaleString()));
    $td.append($('<span class="sr-only"></span>').text(' answers'));
  }

  // The written-by cell: one button drawing the kind's icon, its tip holding
  // the phrase, the column's own sentence and the one detail that differs.
  function cmgKindButton(column) {
    var kind = kindOf(column);
    var descId = 'cmgkd_' + cssId(column.key);
    var detail = kindDetail(column);
    var $btn = $('<button type="button" class="cmg-kind-btn" aria-expanded="false"></button>')
      .attr('aria-label', kindPhrase(column))
      .attr('aria-describedby', descId)
      .append($(KIND_ICON[kind]));
    var $tip = $('<span class="cmg-kind-tip" role="tooltip"></span>')
      .append($('<span class="cmg-kind-tip-phrase"></span>').text(kindPhrase(column)))
      .append($('<span></span>').text(kindSentence(column)));
    if (detail) $tip.append($('<span></span>').text(' ' + detail));
    $btn.append($tip);
    $btn.append($('<span class="sr-only"></span>').attr('id', descId)
      .text(kindSentence(column) + (detail ? ' ' + detail : '')));
    return $btn;
  }

  // One row of the list, every width: the two ticks through the actions. Below
  // the dialog's own 960px breakpoint style.css turns the same <tr> into a
  // two-line grid with `order` on the <td>s, never on the buttons inside them.
  function cmgRowFor(column) {
    var key = column.key;
    var $row = $('<tr class="cmg-row" role="row"></tr>').attr('data-key', key)
      .toggleClass('is-selected', isSelected(key));

    var $tickTd = $('<td role="cell" class="cmg-c-tick"></td>');
    $tickTd.append($('<label class="cmg-tick-label"></label>')
      .append($('<input type="checkbox" class="col-show">').attr('data-target', key)
        .prop('checked', !hiddenTargets[key]))
      .append($('<span class="sr-only"></span>').text(column.label + ' shown in the table')));
    $row.append($tickTd);

    $row.append($('<td role="cell" class="cmg-c-filter"></td>')
      .append($('<label class="cmg-tick-label"></label>')
        .append($('<input type="checkbox" class="col-filter">').attr('data-target', key)
          .prop('checked', !filterOffTargets[key]))
        .append($('<span class="sr-only"></span>').text(column.label + ' filter in the filter panel'))));

    $row.append($('<td role="cell" class="cmg-c-title"></td>')
      .append($('<span class="cmg-label-focus"></span>').attr('tabindex', '-1').attr('data-key', key).text(column.label)));

    var $idTd = $('<td role="cell" class="cmg-c-id"></td>').text(key);
    if ((column.feed_paths || []).length) $idTd.attr('title', 'feed: ' + column.feed_paths.join(', '));
    $row.append($idTd);

    $row.append($('<td role="cell" class="cmg-c-type"></td>').text(column.value_type));

    var $answersTd = $('<td role="cell" class="cmg-c-answers"></td>');
    drawAnswersCell($answersTd, column);
    $row.append($answersTd);

    $row.append($('<td role="cell" class="cmg-c-subject"></td>').text(DOMAIN_SECTION_NAME[domainOf(column)] || domainOf(column)));

    $row.append($('<td role="cell" class="cmg-c-writtenby"></td>').append(cmgKindButton(column)));

    // The flex layout for the up/down pair lives on an inner span, never on
    // the <td> itself: flexing a table cell drops it out of table layout,
    // which doubled the row's height on wide screens.
    var $orderTd = $('<td role="cell" class="cmg-c-order"></td>');
    var $orderWrap = $('<span class="cmg-order-wrap"></span>');
    ['up', 'down'].forEach(function (dir) {
      var $button = $('<button type="button" class="cmg-move"></button>')
        .attr('data-key', key).attr('data-dir', dir)
        .append($('<span aria-hidden="true"></span>').text(dir === 'up' ? '\u2191' : '\u2193'));
      setMoveState($button, key, dir);
      $orderWrap.append($button);
    });
    $orderTd.append($orderWrap);
    $row.append($orderTd);

    var $actTd = $('<td role="cell" class="cmg-c-actions"></td>');
    var $actWrap = $('<span class="cmg-actions-wrap"></span>');
    var actionsId = 'cmgact_' + cssId(key);
    var $buttons = $('<span class="cmg-action-buttons"></span>').attr('id', actionsId).prop('hidden', true);
    $buttons.append($('<button type="button" class="col-rename-pencil"></button>')
      .attr('data-key', key).attr('aria-label', 'Rename ' + column.label).attr('title', 'Rename ' + column.label)
      .append(EDIT_ICON));
    if (takesColors(column)) {
      var colorsName = 'Colours for ' + column.label + (column.color_rules ? '. It has colours now.' : '');
      $buttons.append($('<button type="button" class="col-colors"></button>')
        .attr('data-key', key)
        .toggleClass('cmg-has-colors', !!column.color_rules)
        .attr('aria-label', colorsName).attr('title', colorsName)
        .append(COLOR_ICON));
    }
    if (!column.sealed) {
      $buttons.append($('<button type="button" class="col-delete"></button>')
        .attr('data-key', key).attr('aria-label', 'Delete ' + column.label).attr('title', 'Delete ' + column.label)
        .append(DELETE_ICON));
    }
    var $more = $('<button type="button" class="cmg-more" aria-expanded="false"></button>')
      .attr('aria-controls', actionsId)
      .attr('aria-label', 'More for ' + column.label)
      .attr('title', 'Rename, colours and delete for ' + column.label)
      .append($('<span aria-hidden="true"></span>').text('\u22EF'));
    $actWrap.append($more).append($buttons);
    $actTd.append($actWrap);
    $row.append($actTd);

    drawHeldBackNote($row, key);
    return $row;
  }

  // Every sortable header carries aria-sort off the same state #cmg-sort
  // holds, so a screen reader and the caret glyph in style.css never disagree
  // with what a header click or the select just set.
  function refreshSortHeaders() {
    $('.cmg-th-btn').each(function () {
      var sort = $(this).attr('data-sort');
      $(this).closest('th').attr('aria-sort', sort === cmgState.sort ? 'ascending' : 'none');
    });
    var why = reorderRefusal();
    var $orderBtn = $('.cmg-th-btn[data-sort="table"]');
    var name = 'Sort by table order' + (why ? ', and return every column to it' : '');
    $orderBtn.attr('title', why ? name + '. Reordering is ' + why + '.' : name);
  }

  // callerRestoresFocus belongs to the callers that put focus somewhere of their
  // own after the rebuild, and it stops this one from landing first. knownRow is
  // for the caller that already dropped the reader's focus before calling: it
  // names the row to go back to, because activeElement no longer can.
  function buildColumnChooser(callerRestoresFocus, knownRow) {
    // Nothing moves under a reader who is mid-drag, mid-long-press, or
    // dragging the main table's own heading.
    if (rowDrag || touchPress || headDrag) { chooserRebuildDue = true; return; }
    // The rebuild throws away the control the reader is standing on and the
    // browser drops focus to <body>, where the next Tab starts at the top of
    // the page.
    var was = document.activeElement;
    var fromRow = knownRow !== undefined ? knownRow
      : (!callerRestoresFocus && was && $(was).closest('#col-manage-modal').length
        ? ($(was).closest('.cmg-row').attr('data-key') || '')
        : null);
    var all = colConfig.filter(function (c) { return c.column; });
    var kept = all.filter(function (c) { return cmgKeeps(c.column); });
    var cmp = CMG_SORTS[cmgState.sort];
    if (cmgState.sort === 'type') cmgTypeFreq = typeFrequencyAmong(kept);
    if (cmp) kept = kept.slice().sort(cmp);
    cmgKept = kept;
    drawCmgCount(all.length);
    drawCmgAnswers();
    drawCmgAll();
    drawSelectionState();
    var $into = $('#cmg-rows-table').empty();
    var plan = cmgDrawPlan(kept);
    plan.forEach(function (item) {
      $into.append(item.heading ? cmgGroupRowForTable(item.heading) : cmgRowFor(item.entry.column));
    });
    refreshSortHeaders();
    return finishChooser();
    function finishChooser() {
      // Only where focus was LOST: what survived the rebuild is left exactly
      // where the reader put it. The row's own tick is the identity to go back
      // to, and the search box is where a control outside the rows lands. A
      // knownRow caller has already hidden the control the reader was standing
      // on, and the browser has not moved focus off it yet, so its word is taken
      // over what activeElement still says.
      if (fromRow === null) return;
      var on = document.activeElement;
      if (knownRow !== undefined || !on || on === document.body || on === document.documentElement) {
        focusByIdentity([
          function () { return fromRow ? tickFor(fromRow) : $(); },
          '#cmg-search',
          '#col-manage-close'
        ]);
      }
    }
  }

  // Shift with a move button sends the column, or the whole selection, to the
  // top or the bottom of the order instead of one place. It is the same commit a
  // drag makes, so it is one Undo step and says the same words; unlike a drag it
  // leaves the selection standing and puts the focus back on the button, as a
  // plain move does. The reason for a refusal is spoken, because a press that
  // does nothing and says nothing reads as a broken button.
  function sendToEdge(key, dir) {
    var why = reorderRefusal();
    if (why) { announce('status-announce', 'Nothing moved: ' + why + '.'); return; }
    var group = isSelected(key) && selectedKeys().length > 1;
    var keys = group ? selectedKeys() : [key];
    if (getColIdx(key) < 0) return;
    var entries = keys.map(function (k) { return colConfig[getColIdx(k)]; });
    var anchorKey = null;
    if (dir === 'up') {
      var first = colConfig.filter(function (c) { return c.column && entries.indexOf(c) === -1; })[0];
      if (first) anchorKey = first.target;
    }
    var label = keys.length === 1 ? labelOf(keys[0]) : keys.length + ' columns';
    var next = orderWithBlockBefore(entries,
      anchorKey === null ? null : colConfig[getColIdx(anchorKey)]);
    if (sameOrder(layoutSnapshot().targets, next.map(function (c) { return c.target; }))) {
      announce('status-announce', label + ' ' + (keys.length === 1 ? 'is' : 'are') +
        ' already ' + (dir === 'up' ? 'first' : 'last') + '.');
      return;
    }
    var savedSelection = selectedKeys();
    var savedAnchor = cmgState.anchor;
    commitReorder(keys, anchorKey);
    if (savedSelection.length) setSelection(savedSelection, savedAnchor);
    focusByIdentity([function () { return moveButtonFor(key, dir); }, '#cmg-search']);
  }

  $('#col-manage-list').on('click', '.cmg-move', function (e) {
    var key = $(this).attr('data-key');
    var dir = $(this).attr('data-dir');
    if (e.shiftKey) { sendToEdge(key, dir); return; }
    if (isSelected(key) && selectedKeys().length > 1) {
      if (groupRefusal(dir)) return;
      groupMove(dir);
      return;
    }
    if (moveRefusal(getColIdx(key), dir)) return;
    moveColumn(key, dir);
  });

  // Opening one row's More leaves every other row as it was, and nothing is
  // remembered between visits to the dialog.
  $('#col-manage-list').on('click', '.cmg-more', function () {
    var open = $(this).attr('aria-expanded') === 'true';
    $(this).attr('aria-expanded', open ? 'false' : 'true');
    $(this).closest('.cmg-actions-wrap').find('.cmg-action-buttons').prop('hidden', open);
  });

  // The kind icon's tip: hover and focus show it (mouseenter measures the room
  // above and flips it below near the top of the dialog, as the table's own
  // tooltip does), a press toggles it, one open at a time.
  // The tip is sized and shifted to sit inside the dialog's visible width (a
  // very narrow dialog leaves a few dozen px), by --tip-left on the tip.
  function placeKindTip(btn) {
    var tip = btn.querySelector('.cmg-kind-tip');
    if (!tip) return;
    var content = document.getElementById('col-manage-content');
    var bar = document.querySelector('#col-manage-list .cmg-bar');
    var obstruction = content ? content.getBoundingClientRect().top : 0;
    if (bar) obstruction = Math.max(obstruction, bar.getBoundingClientRect().bottom);
    if (content) {
      var visL = content.getBoundingClientRect().left + content.clientLeft + 4;
      var avail = content.clientWidth - 8;
      tip.style.maxWidth = Math.floor(Math.min(avail, window.innerWidth * 0.8)) + 'px';
      var w = tip.offsetWidth;
      var b = btn.getBoundingClientRect();
      var left = b.left + b.width / 2 - w / 2;
      var fitted = Math.max(visL, Math.min(left, visL + avail - w));
      tip.style.setProperty('--tip-left', Math.round(fitted - b.left) + 'px');
    }
    // Above when it fits, else below when it fits, else the side with more room
    // with the tip cut to that room and scrolling inside itself. Room is
    // measured to the dialog's and the screen's top and bottom edges; the 12px
    // covers the gap between the button and the tip.
    var br = btn.getBoundingClientRect();
    var floor = Math.min(content ? content.getBoundingClientRect().bottom : window.innerHeight, window.innerHeight);
    var ceil = Math.max(obstruction, 0);
    tip.style.maxHeight = '';
    var need = tip.offsetHeight + 12;
    var above = br.top - ceil;
    var below = floor - br.bottom;
    var useBelow = above < need && (below >= need || below > above);
    var room = useBelow ? below : above;
    if (room < need) tip.style.maxHeight = Math.max(60, Math.floor(room - 12)) + 'px';
    btn.classList.toggle('tip-below', useBelow);
  }
  $('#col-manage-list').on('mouseenter focusin', '.cmg-kind-btn', function () { placeKindTip(this); });
  $('#col-manage-list').on('click', '.cmg-kind-btn', function (e) {
    e.stopPropagation();
    var open = $(this).attr('aria-expanded') === 'true';
    $('.cmg-kind-btn[aria-expanded="true"]').not(this).attr('aria-expanded', 'false');
    $(this).attr('aria-expanded', open ? 'false' : 'true');
    if (!open) placeKindTip(this);
  });

  // Twenty columns arranged by hand is twenty more presses to undo, so there is
  // a way back. It destroys hand-made state, so it asks first, in the pattern
  // deleting a column of your own already uses.
  $('#cmg-reset').on('click', function () {
    if (!confirm('Put the columns back in the order the app ships?\n\n' +
      'This clears the order you arranged. Undo puts it back until you leave the page.')) return;
    clearSelection('');
    var before = layoutSnapshot();
    var state = captureTableState();
    applyColOrder([]);
    closeColorEditor();
    rebuildTableInOrder(state);
    var step = rememberStep('reset the order', before);
    // Nothing is announced when the write was refused: the columns are back
    // where they started and the refusal has already interrupted.
    if (saveOrder(before, state, function () { forgetStep(step); })) {
      announce('status-announce', 'The columns are back in the order the app ships');
    }
  });

  // A model form holds text nobody has saved, so a reorder is refused while one
  // is open rather than closing it. Both are <dialog>s, which fire close.
  $('#add-modal, #edit-modal').on('close', refreshMoveControls);

  $('#col-toggle-btn').on('click', function () {
    openColumnsDialog();
  });

  // The control the dialog was opened from, read before the views panel closes
  // and takes one of the two openers off the screen with it.
  var columnsOpener = null;

  // Every route into the dialog comes through here, so the views panel is
  // closed by all of them and the two are never on screen at once.
  function openColumnsDialog() {
    columnsOpener = document.activeElement;
    closeViewPanel(false);
    // This button lives inside the filter panel, which on a narrow screen is an
    // overlay drawer sitting above the dialog. The drawer has done its job, so
    // it closes, and the funnel is re-read because its aria-expanded describes
    // whichever form the panel takes right now. Unconditional: above the
    // breakpoint the class governs nothing.
    $('body').removeClass('filters-drawer-open');
    syncFilterFunnel();
    // The toolbar filters come back as they were left; the selection starts
    // empty and the dropdown panels and the help start closed.
    cmgState = cmgStateFromStored(cmgFiltersRestored ? cmgStoredFromState(cmgState) : savedSettings[CMG_FILTERS_KEY]);
    cmgFiltersRestored = true;
    rowDrag = null;
    touchPress = null;
    $('#cmg-help').prop('hidden', true);
    $('#cmg-help-toggle').attr('aria-expanded', 'false');
    closeCmgDropdowns();
    drawCmgToolbar();
    buildCmgHelp();
    // One retry, because a reader who opens this dialog is asking about the
    // catalog and a read that failed earlier would otherwise stay failed for
    // the life of the page.
    if (!catalogEmptinessRead) refreshCatalogEmptiness();
    buildColumnChooser();
    closeColorEditor();
    $('#col-new-status').text('');
    $('#col-manage-modal').css('display', 'flex');
    $(document).on('keydown', cmgEscapeHandler);
    // Into the dialog, on the control the dialog exists for: finding one column
    // in a list that is thousands of pixels tall.
    focusByIdentity(['#cmg-search', '#col-manage-close']);
  }

  // Where the focus goes when the dialog closes. The opener is preferred and
  // not assumed: the button inside the views panel opens this dialog and the
  // panel closes behind it, and focus on a hidden element is focus on nothing.
  function columnsOpenerStop() {
    if (!columnsOpener || !document.contains(columnsOpener)) return $();
    var $opener = $(columnsOpener);
    return $opener.is(':visible') ? $opener : $();
  }

  function closeColumnsDialog() {
    if (rowDrag) { endRowDragVisuals(); rowDrag = null; }
    cancelTouchPress();
    chooserRebuildDue = false;
    $('#col-manage-modal').css('display', 'none');
    $(document).off('keydown', cmgEscapeHandler);
    focusByIdentity([columnsOpenerStop, '#col-toggle-btn']);
  }

  $('#col-manage-modal').on('keydown', function (e) { trapTabIn(e, this); });

  // The button holds an icon, so a click on it lands on the <svg> or its <path>
  // and never on the button itself: only the backdrop is told apart from the
  // dialog body by what was clicked.
  $('#col-manage-close').on('click', closeColumnsDialog);
  $('#col-manage-modal').on('click', function (e) {
    if (e.target !== this) return;
    closeColumnsDialog();
  });


  // What a change to which columns are shown costs on the page, however many
  // columns it reached: the table's column visibility, the filter panel built
  // from the drawn columns, the filters put back onto it, the mobile sort, and
  // the dialog's own rows patched in place so a tick keeps the focus it just
  // took. Done ONCE however many columns changed, rather than once per column.
  var shownRedraw = false;

  // A condition is kept only while its control is in the filter panel. When a
  // change takes a control out (its Filter tick, or a step undone) the condition
  // it held goes with it and the rows are read again, so the rows on screen are
  // always the ones the controls describe.
  function dropFiltersWithoutControl() {
    var stale = Object.keys(keptFilters).filter(function (key) {
      if (key === 'hide_free') return hideFreeValue(keptFilters[key]) !== keptFilters[key];
      return !!columnByKey[key] && !filterHasControl(key);
    });
    if (!stale.length) return;
    stale.forEach(function (key) {
      if (key === 'hide_free') {
        // Only the ticks whose price filter left the panel go; the others stay.
        var kept = hideFreeValue(keptFilters[key]);
        if (kept) {
          keptFilters[key] = kept;
          if (activeQuery[key] !== undefined) activeQuery[key] = kept;
          return;
        }
      }
      delete keptFilters[key];
      delete activeQuery[key];
      delete valueFilters[key];
    });
    var first = columnByKey[stale[0]];
    pendingFilterFeedback = { kind: 'cleared', label: stale[0] === 'hide_free' ? 'Hide 0' : (first ? first.label : null) };
    applyFilters(true);
  }

  function drawShownColumns() {
    // Raised for the redraw restoreFilters makes, which fires order.dt and
    // would otherwise save the view a second time, or a first time under a
    // rollback that must not write. Lowered whatever the redraw threw.
    shownRedraw = true;
    try {
      applyColumnVisibility();
      buildFilterControls();
      restoreFilters();
    } finally {
      shownRedraw = false;
    }
    dropFiltersWithoutControl();
    applyRestoredFilters();
    buildMobileSort();
    // Which columns are drawn is what a move steps over, so the ends of the drawn
    // set have just moved and every button has to say so again.
    refreshMoveControls();
    $('#col-manage-list .cmg-row').each(function () {
      var key = $(this).attr('data-key');
      $(this).find('.col-show').prop('checked', !hiddenTargets[key]);
      $(this).find('.col-filter').prop('checked', !filterOffTargets[key]);
      drawHeldBackNote($(this), key);
    });
    drawCmgCount(colConfig.filter(function (c) { return c.column; }).length);
    drawCmgAll();
  }

  // The page and the store after which columns are shown changed. `before` is
  // the layoutSnapshot to go back to when the save is refused, and with it
  // the refusal is answered the way an order save answers one: the columns go
  // back, onRolledBack runs, and the reader is told. Without it the save is
  // fire-and-forget, which is what the Show button on the models-added banner
  // wants: it shows one column from outside this dialog and has nothing to put back.
  // False, as saveOrder answers, when the write was refused before it left
  // this page and the columns are already back.
  function columnsShownChanged(before, onRolledBack) {
    drawShownColumns();
    if (!before) { saveSettings(); return true; }
    var seq = ++layoutSaveSeq;
    var stored = true;
    saveSettings(function (e) {
      if (seq !== layoutSaveSeq) return;
      stored = false;
      restoreHidden(before);
      drawShownColumns();
      if (onRolledBack) onRolledBack();
      statusErr('That change was not saved (' + (e.message || e) +
        '), so the columns are back the way they were.');
    });
    return stored;
  }

  $('#col-manage-list').on('change', '.col-show', function () {
    var key = $(this).attr('data-target');
    var before = layoutSnapshot();
    if (this.checked) delete hiddenTargets[key];
    else hiddenTargets[key] = true;
    var step = rememberStep((this.checked ? 'showed ' : 'hid ') + labelOf(key), before);
    columnsShownChanged(before, function () { forgetStep(step); });
    if (this.checked && heldBackFromTheTable(key)) {
      announce('status-announce', (columnByKey[key] || {}).label +
        ' is shown, but no model on screen has a value in it, so Hide empty is keeping it out of the table');
    }
  });

  // The Filter tick: whether the column has a filter in the filter panel. It is
  // independent of the Table tick. Turning it off takes the control out and
  // clears the condition it held (dropFiltersWithoutControl, run by the redraw).
  $('#col-manage-list').on('change', '.col-filter', function () {
    var key = $(this).attr('data-target');
    var before = layoutSnapshot();
    if (this.checked) delete filterOffTargets[key];
    else filterOffTargets[key] = true;
    var step = rememberStep((this.checked ? 'added the filter of ' : 'removed the filter of ') + labelOf(key), before);
    columnsShownChanged(before, function () { forgetStep(step); });
  });

  // One tick for every column the list is showing. It never reaches a column
  // the search or the answers pick has left out, which is what makes it usable
  // on a catalog of many columns, and it is one step to undo however many
  // columns it changed, exactly as it is one save.
  $('#cmg-all').on('change', function () {
    var show = this.checked;
    var before = layoutSnapshot();
    cmgKept.forEach(function (c) {
      if (show) delete hiddenTargets[c.column.key];
      else hiddenTargets[c.column.key] = true;
    });
    var step = rememberStep((show ? 'showed ' : 'hid ') + columnsWord(cmgKept.length), before);
    columnsShownChanged(before, function () { forgetStep(step); });
    var held = cmgKept.filter(function (c) { return heldBackFromTheTable(c.column.key); }).length;
    announce('status-announce', show
      ? columnsWord(cmgKept.length) + ' shown' + (held ? ', ' + held + ' of them empty here and so not drawn' : '')
      : columnsWord(cmgKept.length) + ' hidden');
  });

  // The PATCH a rename sends, and the state every column-labelled thing on
  // screen shares. The caller decides what to show while it waits and where
  // focus goes.
  function submitRename(key, label) {
    return fetch('/api/columns', {
      method: 'PATCH', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: key, label: label })
    })
      .then(jsonOrThrow)
      .then(function (fresh) {
        columnByKey[key] = Object.assign(columnByKey[key] || {}, fresh);
        renameHeader(key, fresh.label);
        buildFilterControls();
        restoreFilters();
        buildMobileSort();
        return fresh;
      });
  }

  function pencilFor(key) {
    return $('#col-manage-list .col-rename-pencil').filter(function () {
      return $(this).attr('data-key') === key;
    });
  }
  function labelSpanFor(key) {
    return $('#col-manage-list .cmg-label-focus').filter(function () {
      return $(this).attr('data-key') === key;
    });
  }
  function restoreLabelCell($cell, key) {
    var column = columnByKey[key];
    $cell.empty().append($('<span class="cmg-label-focus"></span>')
      .attr('tabindex', '-1').attr('data-key', key).text(column ? column.label : key));
  }

  // The pencil turns the Title cell itself into the editor, at every width.
  $('#col-manage-list').on('click', '.col-rename-pencil', function () {
    var key = $(this).attr('data-key');
    var column = columnByKey[key];
    if (!column) return;
    var $cell = labelSpanFor(key).closest('td');
    var $input = $('<input type="text" class="cmg-rename-input">').val(column.label);
    $cell.empty().append($input);
    $input.trigger('focus');
    $input.get(0).select();
  });
  $('#col-manage-list').on('keydown', '.cmg-rename-input', function (e) {
    if (e.key !== 'Escape' && e.key !== 'Enter') return;
    e.preventDefault();
    // Cancelling a rename must not also reach cmgEscapeHandler on the document,
    // which would close the whole Columns dialog.
    e.stopPropagation();
    var $input = $(this);
    var $cell = $input.closest('td');
    var key = $cell.closest('tr').attr('data-key');
    if (e.key === 'Escape') {
      restoreLabelCell($cell, key);
      focusByIdentity([function () { return pencilFor(key); }]);
      return;
    }
    var label = $input.val().trim();
    var column = columnByKey[key];
    if (!label || label === column.label) {
      restoreLabelCell($cell, key);
      focusByIdentity([function () { return pencilFor(key); }]);
      return;
    }
    submitRename(key, label)
      .then(function (fresh) {
        buildColumnChooser(true);
        flashOk('"' + fresh.label + '" it is');
        focusByIdentity([function () { return pencilFor(key); }]);
      })
      .catch(function (e2) {
        $('#col-new-status').text('Not renamed: ' + (e2.message || e2));
        focusByIdentity([function () { return $input; }]);
      });
  });

  // Deleting a column destroys every model's value for it at once, and this
  // database is their only copy - so the question names that before the request
  // is sent, and the request carries the confirmation the server requires.
  $('#col-manage-list').on('click', '.col-delete', function () {
    var key = $(this).attr('data-key');
    var column = columnByKey[key];
    if (!confirm('Delete the "' + column.label + '" column (' + key + ')?\n\n' +
      'This permanently deletes every model\'s value for it. It cannot be undone, and only your own ' +
      'backup of modelsdb.db holds another copy.')) return;
    fetch('/api/columns', {
      method: 'DELETE', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: key, confirm: true })
    })
      .then(jsonOrThrow)
      .then(function () {
        statusText('Column deleted - reloading...');
        setTimeout(function () { location.reload(); }, 400);
      })
      .catch(function (e) { $('#col-new-status').text('Not deleted: ' + (e.message || e)); });
  });

  // --- What colour a column paints its cells --------------------------------
  // The rule belongs to the COLUMN and is stored with it, beside its heading and
  // whether it is shown, so any column can carry one and a column somebody
  // invented colours exactly as the price columns the app ships do. The shape
  // follows the kind of value: thresholds on a number, two answers on a
  // yes-or-no, one colour per label on a one-of-a-list.

  function takesColors(column) {
    return column.value_type === 'number' || column.value_type === 'flag' || column.value_type === 'choice';
  }

  var colorEditorKey = null;
  var colorDialog = document.getElementById('col-color-modal');
  var BAND_COLORS = ['#22c55e', '#eab308', '#ef4444'];

  // The Colours button of the row that opened the editor, in whichever layout
  // is on screen - a live query, because the row it names may have been redrawn
  // (a sort, a rebuild) between opening the editor and closing it.
  function colorsButtonFor(key) {
    return $('#col-manage-list .col-colors').filter(function () {
      return $(this).attr('data-key') === key;
    });
  }

  // <input type="color"> takes six digits and silently drops anything else, so a
  // three-digit code is written out before it is put in one.
  function hex6(color) {
    var c = safeColor(color);
    if (c.length !== 4) return c;
    return '#' + c[1] + c[1] + c[2] + c[2] + c[3] + c[3];
  }
  function colorInput(cls, value, fallback) {
    return $('<input type="color">').addClass(cls).val(hex6(value) || fallback);
  }

  function colorEditorHint(column) {
    if (column.value_type === 'number') {
      return 'Every value BELOW a threshold takes that threshold\'s colour, smallest first, and whatever is left ' +
        'over takes the last colour. The figures are the ones the table SHOWS' +
        (isPriceColumn(column) ? ', which for a price is the price per million tokens.' : '.');
    }
    if (column.value_type === 'flag') return 'One colour for yes, and one for no.';
    return 'One colour per label. A label you leave alone is still painted - to paint none of them, press "No colours".';
  }

  function bandRow(below, color) {
    return $('<div class="cc-band"></div>')
      .append($('<label>Below</label>').append($('<input type="number" step="any" class="cc-below">').val(below)))
      .append(colorInput('cc-color', color, '#22c55e'))
      .append($('<button type="button" class="btn-clear cc-band-drop">Remove</button>'));
  }

  function drawColorFields(column, rules) {
    var $into = $('#col-color-fields').empty();
    if (column.value_type === 'number') {
      var $bands = $('<div id="cc-bands"></div>');
      (rules.thresholds || []).forEach(function (t) { $bands.append(bandRow(t.below, t.color)); });
      if (!(rules.thresholds || []).length) $bands.append(bandRow('', BAND_COLORS[0]));
      $into.append($bands);
      $into.append($('<button type="button" class="btn-clear" id="cc-band-add">Add a threshold</button>'));
      $into.append($('<div class="cc-band"></div>')
        .append($('<label>Everything else</label>'))
        .append(colorInput('cc-otherwise', rules.otherwise, '#ef4444')));
      return;
    }
    if (column.value_type === 'flag') {
      $into.append($('<div class="cc-band"></div>').append($('<label>Yes</label>'))
        .append(colorInput('cc-yes', rules.yes, '#22c55e')));
      $into.append($('<div class="cc-band"></div>').append($('<label>No</label>'))
        .append(colorInput('cc-no', rules.no, '#ef4444')));
      return;
    }
    (column.options || []).forEach(function (option, i) {
      var have = (rules.options || {})[option];
      $into.append($('<div class="cc-band"></div>')
        .append($('<label></label>').text(option))
        .append(colorInput('cc-option', have, BAND_COLORS[i % BAND_COLORS.length]).attr('data-option', option)));
    });
  }

  // What the fields now say, as the rule the server takes. A threshold with no
  // figure in it is left out rather than sent as a nought, which would colour
  // everything.
  function rulesFromFields(column) {
    if (column.value_type === 'number') {
      var thresholds = [];
      $('#cc-bands .cc-band').each(function () {
        var below = parseFloat($(this).find('.cc-below').val());
        if (!isFinite(below)) return;
        thresholds.push({ below: below, color: $(this).find('.cc-color').val() });
      });
      thresholds.sort(function (a, b) { return a.below - b.below; });
      return { thresholds: thresholds, otherwise: $('.cc-otherwise').val() };
    }
    if (column.value_type === 'flag') {
      return { yes: $('.cc-yes').val(), no: $('.cc-no').val() };
    }
    var options = {};
    $('.cc-option').each(function () { options[$(this).attr('data-option')] = $(this).val(); });
    return { options: options };
  }

  // Closing is asking the dialog to close, never resetting state directly: the
  // dialog's own "close" event is what every closing path (Save, No colours,
  // Cancel, Escape, backdrop click) funnels through, so focus goes back to the
  // Colours button that opened it exactly once, whichever path got there.
  function closeColorEditor() {
    if (colorDialog.open) colorDialog.close();
  }
  $(colorDialog).on('close', function () {
    var key = colorEditorKey;
    colorEditorKey = null;
    focusByIdentity([
      function () { return colorsButtonFor(key); },
      '#col-manage-close'
    ]);
  });
  $(colorDialog).on('click', function (e) { if (e.target === this) closeColorEditor(); });
  $('#col-color-modal-close').on('click', closeColorEditor);

  // One write, then the table is drawn again from the rows already in hand: a
  // colour changes how a value is drawn and never what it is, so nothing is
  // re-read from the server.
  function saveColorRules(key, rules) {
    $('#col-color-status').text('Saving...');
    fetch('/api/columns', {
      method: 'PATCH', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key: key, color_rules: rules })
    })
      .then(jsonOrThrow)
      .then(function (fresh) {
        columnByKey[key].color_rules = fresh.color_rules;
        // Hiding the panel takes the Save button the reader is standing on out
        // of reach, and focus lands on <body> once this has run. The row is
        // named here because activeElement can no longer say which one it was.
        closeColorEditor();
        buildColumnChooser(false, key);
        fillTable();
        flashOk(fresh.color_rules ? '"' + fresh.label + '" is coloured' : '"' + fresh.label + '" has no colours');
      })
      .catch(function (e) { $('#col-color-status').text('Not saved: ' + (e.message || e)); });
  }

  $('#col-manage-list').on('click', '.col-colors', function () {
    openColorEditor($(this).attr('data-key'));
  });
  function openColorEditor(key) {
    var column = columnByKey[key];
    if (!column || !takesColors(column)) return;
    colorEditorKey = key;
    $('#col-color-title').text('Colours for "' + column.label + '" (' + key + ')');
    $('#col-color-hint').text(colorEditorHint(column));
    drawColorFields(column, column.color_rules || {});
    $('#col-color-status').text('');
    if (!colorDialog.open) colorDialog.showModal();
    $('#col-color-title').trigger('focus');
  }
  $('#col-color-fields').on('click', '#cc-band-add', function () {
    var at = $('#cc-bands .cc-band').length;
    $('#cc-bands').append(bandRow('', BAND_COLORS[at % BAND_COLORS.length]));
  });
  $('#col-color-fields').on('click', '.cc-band-drop', function () { $(this).closest('.cc-band').remove(); });
  $('#col-color-cancel').on('click', closeColorEditor);
  $('#col-color-save').on('click', function () {
    if (!colorEditorKey) return;
    saveColorRules(colorEditorKey, rulesFromFields(columnByKey[colorEditorKey]));
  });
  $('#col-color-clear').on('click', function () {
    if (!colorEditorKey) return;
    saveColorRules(colorEditorKey, {});
  });

  // The page after a column was made here: the column is already in
  // `columns`, and the table, the filter panel and the dialog list are
  // rebuilt over it. A reader who never arranged their own
  // order keeps none: applyColOrder([]) reads the new column off
  // columnEntries() in its natural shipped place, and colOrderIsShipped stays
  // true. One who has arranged their own order keeps it, with the new column
  // landing beside its shipped neighbour the same way any column shipped since
  // that order was saved already does. The table's state is read BEFORE the
  // order is reapplied, because it names the sort by the index the old table
  // gave it, and that index means another column once the new one is in.
  function columnArrived() {
    var state = captureTableState();
    applyColOrder(colOrderIsShipped ? [] : colConfig.map(function (c) { return c.target; }));
    rebuildTableInOrder(state);
  }

  $('#col-new-type').on('change', function () {
    $('#col-new-options').toggle($(this).val() === 'choice');
  });
  $('#col-new-create').on('click', function () {
    var label = String($('#col-new-name').val() || '').trim();
    var type = $('#col-new-type').val();
    if (!label) { $('#col-new-status').text('A heading is required.'); return; }
    var options = [];
    if (type === 'choice') {
      options = String($('#col-new-options').val() || '').split('\n')
        .map(function (s) { return s.trim(); }).filter(function (s) { return s !== ''; });
      if (!options.length) { $('#col-new-status').text('A choice column needs at least one label, one per line.'); return; }
    }
    $('#col-new-status').text('Adding...');
    fetch('/api/columns', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ label: label, value_type: type, options: options })
    })
      .then(jsonOrThrow)
      .then(function (fresh) {
        columns.push(fresh);
        columnByKey[fresh.key] = fresh;
        columnArrived();
        $('#col-new-name').val('');
        $('#col-new-options').val('').toggle(false);
        $('#col-new-status').text('"' + fresh.label + '" was added.');
        // The rebuild above already moved focus (to the search box, having found
        // no row to return it to), so this runs after it and wins.
        focusByIdentity([function () { return labelSpanFor(fresh.key); }]);
      })
      .catch(function (e) { $('#col-new-status').text('Not created: ' + (e.message || e)); });
  });

  // --- Rows leaving the catalog ---------------------------------------------

  function flashOk(msg) {
    statusOk(msg);
    setTimeout(function () {
      var st = document.getElementById('status');
      if (st && st.textContent === msg) st.className = '';
    }, 3000);
  }

  function dropRow($tr) {
    table.row($tr).remove().draw(false);
    var info = table.page.info();
    if (info.pages > 0 && info.page >= info.pages) table.page('last').draw(false);
    refreshShowHidden();
  }

  // What a row loses when it goes: every column of yours that holds a value
  // for it, and every value filled in by research, each named one by one,
  // because that is what the question is about.
  function valuesOn(name) {
    var row = rowsByName[name] || {};
    var answeredBy = row.answered_by || {};
    var keys = Object.keys(cellsOf(row)).filter(function (key) { return columnByKey[key]; });
    return {
      mine: keys.filter(function (key) { return answeredBy[key] === 'hand'; }).map(function (key) { return columnByKey[key].label; }),
      research: keys.filter(function (key) { return answeredBy[key] === 'research'; }).map(function (key) { return columnByKey[key].label; })
    };
  }
  // The delete dialog's sentence, from the two lists: either may be absent,
  // and with both absent the sentence says nothing of yours is stored.
  function deleteLossSentence(lost) {
    if (!lost.mine.length && !lost.research.length) {
      return 'This erases the model. Nothing of your own is stored against it, so the row is all that goes. It cannot be undone.';
    }
    var parts = [];
    if (lost.mine.length) parts.push('your ' + joinAnd(lost.mine) + ' for it');
    if (lost.research.length) {
      parts.push('the ' + lost.research.length + (lost.research.length === 1 ? ' value' : ' values') +
        ' filled in by research: ' + joinAnd(lost.research));
    }
    return 'This erases the model, ' + parts.join(', and ') +
      '. It cannot be undone, and only your own backup of modelsdb.db holds another copy.';
  }
  function joinAnd(list) {
    if (list.length <= 1) return list[0] || '';
    return list.slice(0, -1).join(', ') + ' and ' + list[list.length - 1];
  }

  var deleteDialog = document.getElementById('delete-modal');
  var deleteTr = null;
  $('#models').on('click', '.row-delete', function () {
    deleteTr = $(this).closest('tr');
    var name = deleteTr.attr('data-name') || '';
    var hidden = (rowsByName[name] || {}).hidden === true;
    $('#delete-modal-title').text('Delete "' + name + '"?');
    $('#delete-modal-what').text(deleteLossSentence(valuesOn(name)));
    $('#delete-modal-alt').text(hidden
      ? 'It is already hidden, so it is out of your list already.'
      : 'To keep the model and everything you wrote about it, choose "Hide it instead".');
    $('#delete-hide').toggle(!hidden).prop('disabled', false);
    $('#delete-confirm').prop('disabled', false);
    $('#delete-status').text('');
    deleteDialog.showModal();
    $('#delete-cancel').trigger('focus');
  });
  function closeDeleteDialog() { if (deleteDialog.open) deleteDialog.close(); deleteTr = null; }
  $('#delete-cancel, #delete-modal-close').on('click', closeDeleteDialog);
  $(deleteDialog).on('click', function (e) { if (e.target === this) closeDeleteDialog(); });

  $('#delete-confirm').on('click', function () {
    if (!deleteTr) return;
    var $tr = deleteTr, name = $tr.attr('data-name') || '';
    $('#delete-confirm').prop('disabled', true);
    $('#delete-status').text('Deleting...');
    fetch('/api/models/delete', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name, confirm_delete: true })
    })
      .then(jsonOrThrow)
      .then(function () {
        dropRow($tr);
        closeDeleteDialog();
        flashOk('Deleted "' + name + '"');
        refreshCatalogEmptiness();
      })
      .catch(function (e) {
        $('#delete-confirm').prop('disabled', false);
        $('#delete-status').text('Could not delete it: ' + (e.message || e));
      });
  });

  function setHidden($tr, hidden) {
    var name = $tr.attr('data-name') || '';
    return fetch('/api/models/hide', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name, hidden: hidden })
    })
      .then(jsonOrThrow)
      .then(function () { return refreshRow(name); })
      .then(function () {
        refreshShowHidden();
        table.draw(false);
        refreshCatalogEmptiness();
      });
  }
  $('#delete-hide').on('click', function () {
    if (!deleteTr) return;
    var $tr = deleteTr, name = $tr.attr('data-name') || '';
    $('#delete-hide').prop('disabled', true);
    setHidden($tr, true)
      .then(function () { closeDeleteDialog(); flashOk('"' + name + '" is hidden'); })
      .catch(function (e) {
        $('#delete-hide').prop('disabled', false);
        $('#delete-status').text('Could not change it: ' + (e.message || e));
      });
  });
  // The Name-cell description opens UPWARDS, and near the top of the window
  // that runs it into the top bar, which nothing in the table may cover. So on
  // the way in it measures the room above the name - down to whichever of the
  // top bar and FixedHeader's floating heading sits lowest - and opens
  // downwards instead when the tip would not fit. mouseenter does not bubble,
  // which is why jQuery's delegated form is used: it emulates it per element.
  // The tip is visibility:hidden rather than display:none at rest, so its
  // height is measurable before it is shown.
  $('#models').on('mouseenter', '.tooltip', function () {
    var tip = this.querySelector('.tooltiptext');
    if (!tip) return;
    var obstruction = topbarHeight();
    var floating = document.querySelector('.dtfh-floatingparent');
    if (floating) obstruction = Math.max(obstruction, floating.getBoundingClientRect().bottom);
    var room = this.getBoundingClientRect().top - obstruction;
    this.classList.toggle('tip-below', room < tip.offsetHeight + 8);
  });
  $('#models').on('click', '.hidden-mark', function () {
    var $tr = $(this).closest('tr');
    var name = $tr.attr('data-name') || '';
    setHidden($tr, false)
      .then(function () { flashOk('"' + name + '" is back in the list'); })
      .catch(function (e) { statusErr('Could not change "' + name + '": ' + (e.message || e)); });
  });
  $('#models').on('click', '.row-hide', function () {
    var $tr = $(this).closest('tr');
    var name = $tr.attr('data-name') || '';
    setHidden($tr, true)
      .then(function () { flashOk('"' + name + '" is hidden'); })
      .catch(function (e) { statusErr('Could not change "' + name + '": ' + (e.message || e)); });
  });
  // Taking a row back out from under its parent. reloadRows rather than
  // refreshRow, because the row changes CATEGORY: it stops being one of its
  // parent's children and becomes one of the table's own rows, and only a fresh
  // read re-sorts it into the right list. No page reload.
  $('#models').on('click', '.row-unfile', function () {
    var $el = $(this);
    var name = $el.closest('tr').attr('data-name') || '';
    var parent = $el.closest('tr').attr('data-parent') || '';
    fetch('/api/models/variations/unfile', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name })
    })
      .then(jsonOrThrow)
      .then(function () { return reloadRows(); })
      .then(function () {
        flashOk('"' + name + '" is a model of its own again' + (parent ? ', out from under "' + parent + '"' : ''));
      })
      .catch(function (e) { statusErr('Could not unfile "' + name + '": ' + (e.message || e)); });
  });
  // A span carrying role="button" is not a button: the browser gives it no key
  // behaviour of its own, so the two states of the visibility mark are given
  // the same Enter and Space the row's edit control has.
  $('#models').on('keydown', '.row-hide, .hidden-mark, .row-unfile', function (e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault(); e.stopPropagation();
    $(this).trigger('click');
  });

  // --- The answers panel ---------------------------------------------------
  // Opened from any mark on a cell. Reads GET /api/models/{name}/answers/{column}
  // and lists every source's answer in the column's fixed order, dated. Nothing
  // here changes which answer is shown.

  var answersDialog = document.getElementById('answers-modal');
  function closeAnswersDialog() { if (answersDialog.open) answersDialog.close(); }
  $('#answers-modal-close').on('click', closeAnswersDialog);
  $(answersDialog).on('click', function (e) { if (e.target === this) closeAnswersDialog(); });

  $('#models').on('click', '.prov-mark', function () {
    var name = $(this).attr('data-name') || '';
    var key = $(this).attr('data-key') || '';
    openAnswersDialog(name, key);
  });

  function openAnswersDialog(name, key) {
    var column = columnByKey[key];
    $('#answers-modal-title').text(column ? column.label : key);
    $('#answers-modal-lead').text('Loading...');
    $('#answers-modal-list').empty();
    answersDialog.showModal();
    fetch('/api/models/' + encodeURIComponent(name) + '/answers/' + encodeURIComponent(key))
      .then(jsonOrThrow)
      .then(function (info) { renderAnswersDialog(info); })
      .catch(function (e) { $('#answers-modal-lead').text('Could not read the sources: ' + (e.message || e)); });
  }

  // A cell's date, as the page shows every other date: the day alone.
  function dayOf(stamp) { return stamp ? String(stamp).split('T')[0] : ''; }

  // The lead sentence for each combination of sources.
  function answersLead(info) {
    var by = {};
    info.answers.forEach(function (a) { by[a.source] = a; });
    var or = by.openrouter && by.openrouter.value !== null ? by.openrouter : null;
    var hf = by.huggingface && by.huggingface.value !== null ? by.huggingface : null;
    var research = by.research && by.research.value !== null ? by.research : null;
    if (info.resolve === 'union' && (or || hf)) {
      return 'The value shown is OpenRouter\'s and HuggingFace\'s answers added together.';
    }
    if (or && hf) {
      if (or.value === hf.value) {
        return 'OpenRouter and HuggingFace agree. ' + sourceWord(info.winner) + '\'s answer is shown.';
      }
      return 'OpenRouter and HuggingFace say different things. ' + sourceWord(info.winner) +
        ' is preferred on this column, so its answer is shown. Nothing here changes that.';
    }
    if (or || hf) {
      var spoke = or ? 'OpenRouter' : 'HuggingFace';
      var silent = or ? 'HuggingFace' : 'OpenRouter';
      if (by.openrouter && by.huggingface) return spoke + ' stated this. ' + silent + ' has not.';
      return spoke + ' stated this.';
    }
    if (research) {
      return 'Nothing automatic has stated this. The value shown was filled in by research on ' +
        dayOf(research.written_at) + ' and stays until OpenRouter or HuggingFace states it.';
    }
    if (by.hand && by.hand.value !== null) return 'This is your own answer, in a column of yours.';
    return 'Nothing has stated this yet.';
  }

  function renderAnswersDialog(info) {
    $('#answers-modal-title').text(info.label);
    $('#answers-modal-lead').text(answersLead(info));
    var $list = $('#answers-modal-list').empty();
    var overtaken = info.answers.some(function (a) {
      return (a.source === 'openrouter' || a.source === 'huggingface') && a.value !== null;
    });
    info.answers.forEach(function (a) {
      var $li = $('<li class="answers-row"></li>');
      var isWinner = a.source === info.winner && info.resolve !== 'union';
      $li.append($('<span class="answers-source"></span>').text(sourceWord(a.source)));
      if (isWinner) $li.append($('<span class="answers-shown"></span>').text('shown'));
      $li.append($('<span class="answers-value"></span>').text(a.value === null ? 'Nothing stated' : a.value));
      if (a.value !== null) {
        var date = dayOf(a.written_at);
        if (a.source === 'research' && overtaken) {
          date += ' \u00B7 no longer shown: ' + sourceWord(info.winner) + ' has stated it';
        }
        if (date) $li.append($('<span class="answers-date cmg-meta"></span>').text(date));
      }
      $list.append($li);
    });
  }

  // --- The variation picker -------------------------------------------------

  var variationDialog = document.getElementById('variation-modal');
  var variationParent = null;
  var variationRepoList = [], variationBuildList = [], variationBuildRepo = '';

  function variationStatus(msg, isError) {
    var el = document.getElementById('variation-status');
    el.textContent = msg || '';
    el.classList.toggle('var-status-err', !!isError);
  }

  var VARIATION_REPO_SORTS = {
    downloads: function (a, b) { return (b.downloads || 0) - (a.downloads || 0); },
    likes: function (a, b) { return (b.likes || 0) - (a.likes || 0); },
    name: function (a, b) { return String(a.repo).localeCompare(String(b.repo)); },
    listed: null
  };
  var VARIATION_BUILD_SORTS = {
    'size-asc': { key: function (b) { return b.bytes || null; }, dir: 1 },
    'size-desc': { key: function (b) { return b.bytes || null; }, dir: -1 },
    'bit-asc': { key: function (b) { return b.bit || null; }, dir: 1 },
    'bit-desc': { key: function (b) { return b.bit || null; }, dir: -1 }
  };

  function drawVariationRepos() {
    var cmp = VARIATION_REPO_SORTS[$('#variation-repo-order').val()];
    var list = cmp ? variationRepoList.slice().sort(cmp) : variationRepoList.slice();
    var $list = $('#variation-repos').empty();
    list.forEach(function (r) {
      var meta = [];
      if (r.format) meta.push(r.format);
      if (r.likes) meta.push(r.likes + ' likes');
      if (r.downloads) meta.push(r.downloads.toLocaleString() + ' downloads');
      if (r.gated) meta.push('needs a HuggingFace account');
      var $btn = $('<button type="button" class="var-pick"></button>')
        .attr('data-repo', r.repo)
        .append($('<span class="var-pick-name"></span>').text(r.repo))
        .append($('<span class="var-pick-meta"></span>').text(meta.join(' - ')));
      if (r.gated) $btn.addClass('var-pick-gated');
      $list.append($btn);
    });
  }

  function drawVariationBuilds() {
    var s = VARIATION_BUILD_SORTS[$('#variation-build-order').val()] || VARIATION_BUILD_SORTS['size-asc'];
    var list = variationBuildList.slice().sort(function (x, y) {
      if (!!x.projector !== !!y.projector) return x.projector ? 1 : -1;
      var a = s.key(x), b = s.key(y);
      if (a === null && b === null) return 0;
      if (a === null) return 1;
      if (b === null) return -1;
      return (a - b) * s.dir;
    });
    var $list = $('#variation-files').empty();
    list.forEach(function (b) {
      var meta = [];
      if (b.bit) meta.push(b.bit + '-bit');
      if (b.bytes) meta.push(twoDecimals(b.bytes / 1e9) + ' GB');
      if (b.files && b.files.length > 1) meta.push(b.files.length + ' files');
      if (b.projector) meta.push('a vision projector - loaded alongside a version, not one itself');
      $list.append($('<button type="button" class="var-pick"></button>')
        .attr('data-repo', variationBuildRepo).attr('data-build', b.id)
        .append($('<span class="var-pick-name"></span>').text(b.format + ' ' + b.label))
        .append($('<span class="var-pick-meta"></span>').text(meta.join(' - '))));
    });
  }

  function openVariationPicker($tr) {
    variationParent = $tr.attr('data-name') || '';
    $('#variation-modal-title').text('Add a variation of "' + variationParent + '"');
    $('#variation-repos').empty();
    $('#variation-files').empty().hide();
    $('#variation-files-head').hide();
    variationRepoList = []; variationBuildList = [];
    $('#variation-repos-sort, #variation-files-sort').prop('hidden', true);
    $('#variation-existing-search').val('');
    $('#variation-existing-status').text('');
    loadExistingCandidates();
    variationDialog.showModal();

    var repo = repositoryOf(rowsByName[variationParent] || {});
    if (!repo) {
      variationStatus('This model has no HuggingFace repository recorded, so there is nothing to look up there. ' +
        'You can still file a model you already have under it, below.');
      return;
    }
    variationStatus('Asking HuggingFace which repositories publish versions of this model...');
    var asked = variationParent;
    fetch('/api/models/variations?name=' + encodeURIComponent(asked))
      .then(jsonOrThrow)
      .then(function (res) {
        if (variationParent !== asked) return;
        var repos = res.repos || [];
        if (!repos.length) {
          variationStatus('HuggingFace lists no published versions of ' + res.hf_slug + '.');
          return;
        }
        variationStatus(repos.length + ' repositor' + (repos.length === 1 ? 'y' : 'ies') +
          ' publish versions of ' + res.hf_slug +
          '. Not every one of them really is a version of this model - read the names and pick.' +
          (res.truncated ? ' There were more than could be listed here; these are the first of them.' : ''));
        variationRepoList = repos;
        $('#variation-repos-sort').prop('hidden', repos.length < 2);
        drawVariationRepos();
      })
      .catch(function (e) { if (variationParent === asked) variationStatus(String(e.message || e), true); });
  }

  $('#variation-repo-order').on('change', drawVariationRepos);
  $('#variation-build-order').on('change', drawVariationBuilds);

  $('#variation-repos').on('click', '.var-pick', function () {
    var repo = $(this).attr('data-repo');
    var asked = variationParent;
    $('#variation-files').empty().show();
    $('#variation-files-head').text('Versions in ' + repo).show();
    variationStatus('Reading ' + repo + '...');
    fetch('/api/models/variations/files?repo=' + encodeURIComponent(repo))
      .then(jsonOrThrow)
      .then(function (res) {
        if (variationParent !== asked) return;
        var builds = res.builds || [];
        if (res.gated) {
          variationStatus(repo + ' requires an account on HuggingFace, so its files cannot be read here.', true);
          return;
        }
        if (!builds.length) { variationStatus(repo + ' holds no model files.', true); return; }
        variationStatus('Pick the version you would run. The size is the exact size of that version\'s own files.');
        variationBuildList = builds;
        variationBuildRepo = repo;
        $('#variation-files-sort').prop('hidden', builds.length < 2);
        drawVariationBuilds();
      })
      .catch(function (e) { if (variationParent === asked) variationStatus(String(e.message || e), true); });
  });

  $('#variation-files').on('click', '.var-pick', function () {
    var repo = $(this).attr('data-repo'), build = $(this).attr('data-build');
    var parent = variationParent;
    variationStatus('Recording it...');
    fetch('/api/models/variations', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: parent, repo: repo, build: build })
    })
      .then(jsonOrThrow)
      .then(function (res) { closeVariationDialog(); reloadRows().catch(reloadFailed); flashOk('Added ' + res.format + ' to "' + parent + '"'); })
      .catch(function (e) { variationStatus(String(e.message || e), true); });
  });

  // Filing a model that is already here under another one. Nothing is fetched
  // and nothing is measured: the row keeps every value it holds and gains one
  // fact - which model it is a build of.
  var EXISTING_CANDIDATE_LIMIT = 40;
  var existingCandidates = [];

  // The models this one could be filed under are read from the SERVER, not from
  // the rows on screen: the table is showing whatever the filters asked for, and
  // the model somebody wants to file is usually not among them. Only the few
  // columns the list shows are asked for.
  function loadExistingCandidates() {
    existingCandidates = [];
    $('#variation-existing-list').empty().append($('<p class="paths-hint"></p>').text('Reading your catalog...'));
    fetch('/api/models?include_hidden=1&fields=' + encodeURIComponent(
      'name,variant_of,repo_owner,repository,weights_size_gb,build_format'))
      .then(jsonOrThrow)
      .then(function (rows) {
        // A model that already has variations of its own cannot be filed under
        // another one: its own builds would be left with nowhere to sit.
        var parents = {};
        (rows || []).forEach(function (r) { if (r.variant_of) parents[r.variant_of] = true; });
        existingCandidates = (rows || []).filter(function (r) {
          return r.name && !r.variant_of && !parents[r.name];
        });
        drawExistingCandidates();
      })
      .catch(function (e) {
        $('#variation-existing-list').empty()
          .append($('<p class="paths-hint"></p>').text('Your catalog could not be read: ' + (e.message || e)));
      });
  }

  function drawExistingCandidates() {
    var query = parseSearch(String($('#variation-existing-search').val() || ''));
    var $list = $('#variation-existing-list').empty();
    var shown = 0;
    for (var i = 0; i < existingCandidates.length && shown < EXISTING_CANDIDATE_LIMIT; i++) {
      var m = existingCandidates[i];
      if (m.name === variationParent) continue;
      if (!searchMatches(query, [m.name])) continue;
      var meta = [];
      // The account that PUBLISHED the repository, which is usually somebody who
      // republished a build rather than the model's maker - so it is the owner
      // that identifies a build, never a vendor column.
      var owner = cellValue(m, 'repo_owner');
      if (owner) meta.push(owner);
      var repo = repositoryOf(m);
      if (repo) meta.push(repo);
      var gb = parseFloat(cellValue(m, 'weights_size_gb'));
      if (!isNaN(gb)) meta.push(twoDecimals(gb) + ' GB');
      var format = cellValue(m, 'build_format');
      if (format) meta.push(format);
      $list.append($('<button type="button" class="var-pick"></button>')
        .attr('data-name', m.name)
        .append($('<span class="var-pick-name"></span>').text(m.name))
        .append($('<span class="var-pick-meta"></span>').text(meta.join(' - '))));
      shown++;
    }
    if (!shown) {
      $list.append($('<p class="paths-hint"></p>').text(query.length
        ? 'No model here matches that, and one that already has variations of its own cannot be filed under another model.'
        : 'There is no other model in this catalog to file under this one.'));
    }
  }
  $('#variation-existing-search').on('input', drawExistingCandidates);
  $('#variation-existing-list').on('click', '.var-pick', function () {
    var name = $(this).attr('data-name'), parent = variationParent;
    $('#variation-existing-status').text('Filing "' + name + '" under "' + parent + '"...');
    fetch('/api/models/variations/existing', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name, parent: parent })
    })
      .then(jsonOrThrow)
      .then(function () {
        closeVariationDialog();
        reloadRows().catch(reloadFailed);
        flashOk('"' + name + '" is now a hidden variation of "' + parent + '"');
      })
      .catch(function (e) { $('#variation-existing-status').text(String(e.message || e)); });
  });

  function closeVariationDialog() {
    if (variationDialog.open) variationDialog.close();
    variationParent = null;
  }
  $('#variation-close, #variation-cancel').on('click', closeVariationDialog);
  $(variationDialog).on('click', function (e) { if (e.target === this) closeVariationDialog(); });
  $('#models').on('click', '.var-add', function () { openVariationPicker($(this).closest('tr')); });

  // The tail of a write that re-reads the rows without waiting on it: a failed
  // read says so instead of vanishing as an unhandled rejection.
  function reloadFailed(e) { statusErr('The rows could not be read again: ' + (e.message || e)); }

  function reloadRows() {
    var params = buildQuery();
    return fetchRows(params).then(function (rows) {
      indexRows(rows);
      fillTable();
      refreshCatalogEmptiness();
    });
  }

  // --- The per-model form, shared by Add and Edit ---------------------------
  // One field per column, in the order the app ships them, grouped by what the
  // column is about. A fetched, measured or worked-out column holding an
  // automatic value is a read-only line with the sentence saying why, not a
  // disabled input: the form exists to type into. An empty one, or one
  // holding research, is an input. A column of yours is an input.

  function formFieldId(prefix, key) { return prefix + '_' + cssId(key); }

  function buildModelForm($into, prefix, mode, row) {
    $into.empty().addClass('model-form');
    row = row || {};

    if (mode === 'add') {
      var $set = $('<fieldset></fieldset>').append($('<legend></legend>').text('The model'));
      var $grid = $('<div class="form-grid"></div>');
      var $field = $('<div class="form-field form-full"></div>');
      $field.append($('<label></label>').attr('for', prefix + '_name').text('Name')
        .append('<span class="form-required"> *</span>'));
      $field.append($('<input type="text" required>').attr('id', prefix + '_name')
        .attr('placeholder', 'Vendor: Model Name'));
      $field.append($('<p class="field-note"></p>').text('Only the name is required. Every other field is empty because nothing has stated it: what you type is kept as research, and if this model ever appears on OpenRouter or its repository is read, what they state replaces it.'));
      $grid.append($field);
      $set.append($grid);
      $into.append($set);
    }

    // Adding writes the same fields editing does: every field, not a curated
    // subset, because the reader may know an answer no fetched source has
    // stated yet.
    DOMAIN_SECTIONS.forEach(function (domain) {
      // The SHIPPED order, never the reader's: the table's arrangement is a
      // reading choice, the form is a sequence to type through.
      var inSection = columnEntries().filter(function (c) {
        if (!c.column || domainOf(c.column) !== domain) return false;
        // The app stamps it when Add model is pressed, so it has no field
        // on the add form.
        return !(mode === 'add' && kindOf(c.column) === 'stamped');
      });
      if (!inSection.length) return;
      var $set = $('<fieldset></fieldset>').append($('<legend></legend>')
        .text(DOMAIN_SECTION_NAME[domain] || domain));
      var $grid = $('<div class="form-grid"></div>');
      inSection.forEach(function (c) { $grid.append(buildFormField(prefix, c.column, row, mode)); });
      $set.append($grid);
      $into.append($set);
    });
  }

  // The note under a form field: what the
  // value is, who maintains it, and what typing there does. mode 'add' is
  // its own case, ahead of cellWritable: a brand-new row has no answered_by
  // at all, so cellWritable(row, column) reads false for every automatic
  // column on it, which is right for the TABLE and the EDIT form - nothing
  // has stated it there yet - but wrong here, where adding a row takes every
  // field, landing as research.
  function formFieldNote(column, row, mode) {
    var automatic = automaticSources(column);
    if (mode === 'add') {
      return isYours(column) ? (column.about || '')
        : 'Nothing has stated this yet. What you type is kept as research and replaced when OpenRouter or HuggingFace states it.';
    }
    if (!cellWritable(row, column)) {
      // Empty is its own case, ahead of the "holds an automatic value"
      // sentences below: nothing has stated it, so those would claim more
      // than is true. stamped never reaches here empty in practice - added is
      // set the moment a row exists - but if it ever did, its own sentence
      // still holds: stamped never takes a fill, full or empty.
      if (cellIsEmpty(row, column) && kindOf(column) !== 'stamped') {
        switch (kindOf(column)) {
          case 'measured':
            return automatic.length === 2
              ? 'Nothing has stated this yet. Only the model\'s repository or OpenRouter can fill it.'
              : 'Nothing has stated this yet. Only the model\'s repository can fill it.';
          case 'derived':
            return 'Nothing has stated this yet. It fills in once OpenRouter states the fact it is worked out from.';
          default:
            return 'Nothing has stated this yet. Only OpenRouter or the model\'s repository can fill it.';
        }
      }
      switch (kindOf(column)) {
        case 'stamped': return 'Stamped by the app when this model entered your catalog.';
        case 'derived': return 'Worked out by the app from ' + (column.derived_from || 'an OpenRouter fact') + '. It cannot be typed here.';
        case 'measured':
          if (automatic.length === 2) return 'Measured from HuggingFace, which is preferred over OpenRouter here. It cannot be typed here.';
          return 'Measured from the HuggingFace repository. Read the repository again from the model\'s details to refresh it; it cannot be typed here.';
        default:
          if (automatic.length === 2) return 'Fetched from OpenRouter, which is preferred over HuggingFace here. It cannot be typed here.';
          return 'Fetched from OpenRouter. Updated on every refresh; it cannot be typed here.';
      }
    }
    if (isYours(column)) return column.about || '';
    // cellWritable is true and the column is not yours only where it already
    // holds a researched value - the one way a fetched, measured or derived
    // field still takes an input.
    return 'Filled in by research. It stays until OpenRouter or HuggingFace states it. Clear the field to remove it.';
  }

  function buildFormField(prefix, column, row, mode) {
    var id = formFieldId(prefix, column.key);
    var value = editorStartValue(row, column);
    // Add is its own case: a new row has nothing on file yet, so
    // cellWritable's "does this already hold research" test always reads
    // false there. Adding a row takes ALL FIELDS, so add
    // never refuses one - stamped columns (added) are the one kind never
    // offered here at all, filtered out in buildModelForm before this runs.
    var refused = mode !== 'add' && !cellWritable(row, column);
    var $field = $('<div class="form-field"></div>').attr('data-key', column.key);
    var note = formFieldNote(column, row, mode);

    // A read-only line: the label as a span and the value in the same box a
    // disabled input had, so the field keeps its place, and nothing to Tab to
    // - except the one control an overtaken researched value needs, the
    // button that clears it, because an empty value is the one write the
    // server still takes on such a cell.
    if (refused) {
      $field.append($('<span class="form-field-label"></span>').text(column.label));
      $field.append($('<p class="field-value"></p>').text(value === '' ? '\u2014' : value));
      if (isPriceColumn(column) && value !== '') {
        $field.append($('<p class="field-note"></p>').text('Shown ' + (priceUnitOf(row) || 'per 1M tokens') + ', as the column shows it.'));
      }
      $field.append($('<p class="field-note"></p>').text(note));
      if ((row.overtaken || {})[column.key]) {
        $field.append($('<p class="field-note field-overtaken"></p>')
          .text('A value filled in by research sits beneath it and is no longer shown. ')
          .append($('<button type="button" class="btn-clear field-clear-research"></button>')
            .attr('data-key', column.key)
            .attr('title', 'Remove the researched ' + column.label + '. What ' + sourceWord((row.answered_by || {})[column.key] === 'merged' ? automaticSources(column)[0] : (row.answered_by || {})[column.key]) + ' states stays.')
            .text('Clear it')));
      }
      return $field;
    }

    var $label = $('<label></label>').attr('for', id).text(column.label);
    var $control;

    if (column.value_type === 'flag') {
      $control = $('<select></select>')
        .append('<option value="">(not set)</option>')
        .append('<option value="yes">Yes</option>')
        .append('<option value="no">No</option>')
        .val(String(value).trim() === '' ? '' : (isYes(value) ? 'yes' : 'no'));
    } else if (column.value_type === 'choice') {
      $control = $('<select></select>').append('<option value="">(not set)</option>');
      (column.options || []).forEach(function (o) {
        $control.append($('<option></option>').attr('value', o).text(o));
      });
      $control.val(value);
    } else if (column.value_type === 'number') {
      $control = $('<input type="number" step="any">').val(value);
    } else if (column.value_type === 'text' && column.key === 'notes') {
      $control = $('<textarea rows="4"></textarea>').val(value);
      $field.addClass('form-full');
    } else {
      $control = $('<input type="text">').val(value);
    }

    $control.attr('id', id).attr('data-key', column.key);
    $field.append($label).append($control);
    if (isPriceColumn(column)) {
      $field.append($('<p class="field-note"></p>').text('Entered ' + (priceUnitOf(row) || 'per 1M tokens') +
        ', as the column shows it.'));
    }
    if (note) $field.append($('<p class="field-note"></p>').text(note));
    return $field;
  }

  // The line at the top of the edit form: how many fields are read-only, how
  // many are empty and can be filled in, and how many are yours. One message
  // per count, chosen by the plural rules, never glued from pieces.
  var PLURAL = new Intl.PluralRules('en');
  function countedMessage(n, forms) {
    var form = forms[PLURAL.select(n)] || forms.other;
    return form.replace('{n}', String(n));
  }
  var OWNED_NOTE_FORMS = {
    readOnly: { one: '{n} field is fetched or measured and cannot be typed here', other: '{n} fields are fetched or measured and cannot be typed here' },
    empty: { one: '{n} is empty and can be filled in', other: '{n} are empty and can be filled in' },
    yours: { one: '{n} is yours', other: '{n} are yours' }
  };
  function ownedNote(row) {
    var readOnly = 0, empty = 0, yours = 0;
    columns.forEach(function (column) {
      if (isYours(column)) yours++;
      else if (!cellWritable(row, column)) readOnly++;
      else empty++;
    });
    return countedMessage(readOnly, OWNED_NOTE_FORMS.readOnly) + '; ' +
      countedMessage(empty, OWNED_NOTE_FORMS.empty) + '; ' +
      countedMessage(yours, OWNED_NOTE_FORMS.yours) + '.';
  }

  // What the form now holds, as cells to send: only the columns that really
  // changed, because a save is an overlay and an empty value CLEARS a cell.
  function changedFormCells(prefix, row, mode) {
    var cells = {}, bad = null;
    colConfig.forEach(function (c) {
      if (!c.column) return;
      var column = c.column;
      // Add is exempt from cellWritable the same way buildFormField is: a
      // new row's blank answered_by would otherwise refuse every automatic
      // column. A stamped column has no control here at all - it was never
      // built as a field - so it falls out at the next check regardless.
      if (mode !== 'add' && !cellWritable(row, column)) return;
      var $control = $('#' + formFieldId(prefix, column.key));
      if (!$control.length || $control.is(':disabled')) return;
      var typed = String($control.val() == null ? '' : $control.val()).trim();
      var before = mode === 'add' ? '' : editorStartValue(row, column);
      if (typed === String(before)) return;
      var stored = storedFromEditor(row, column, typed);
      if (stored === null) { bad = column.label + ' is not a number.'; return; }
      cells[column.key] = stored;
    });
    return { cells: cells, error: bad };
  }

  // --- Adding a model -------------------------------------------------------

  var addDialog = document.getElementById('add-modal');
  var ADD_TITLES = { choose: 'Add a model', hf: 'Add a model from HuggingFace', form: 'Add a custom model', result: 'Model added' };

  function addStatus(msg, isError) {
    var el = document.getElementById('add-status');
    el.textContent = msg || '';
    el.classList.toggle('var-status-err', !!isError);
    if (!msg) $('#add-extra').empty();
  }
  function showAddState(state) {
    $('#add-modal-title').text(ADD_TITLES[state]);
    $('#add-modal-lead').toggle(state === 'choose');
    $('#add-choose').toggle(state === 'choose');
    $('#add-hf-form').prop('hidden', state !== 'hf');
    $('#add-form').prop('hidden', state !== 'form');
    $('#add-result').prop('hidden', state !== 'result');
    if (state === 'choose') { addStatus(''); $('#add-pick-hf').trigger('focus'); }
    if (state === 'hf') $('#add-hf-url').trigger('focus');
    if (state === 'form') $('#af_name').trigger('focus');
    if (state === 'result') $('#add-done').trigger('focus');
  }

  $('#btn-add-model').on('click', function () {
    $('#add-hf-url').val('');
    $('#add-hf-go').prop('disabled', false).text('Import');
    $('#add-form-status').removeClass('is-shown').empty();
    buildModelForm($('#add-form-fields'), 'af', 'add', {});
    addStatus('');
    addDialog.showModal();
    refreshMoveControls();
    showAddState('choose');
  });
  $('#add-close, #add-done').on('click', function () { if (addDialog.open) addDialog.close(); });
  $('#add-modal').on('click', function (e) { if (e.target === this) addDialog.close(); });
  $('#add-pick-hf').on('click', function () { showAddState('hf'); });
  $('#add-pick-custom').on('click', function () { showAddState('form'); });
  $('#add-hf-back, #add-form-back, #add-result-back').on('click', function () { showAddState('choose'); });

  $('#add-hf-form').on('submit', function (e) {
    e.preventDefault();
    var typed = $('#add-hf-url').val();
    $('#add-extra').empty();
    $('#add-hf-go').prop('disabled', true).text('Reading the repository...');
    addStatus('Reading ' + typed + ' on HuggingFace. This takes a few seconds.');
    fetch('/api/models/import', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ url: typed })
    })
      .then(function (r) { return r.text().then(function (t) { return { status: r.status, text: t }; }); })
      .then(function (res) {
        $('#add-hf-go').prop('disabled', false).text('Import');
        if (res.status === 201 || res.status === 200) {
          var body = JSON.parse(res.text);
          addStatus('');
          $('#add-result-what').text('Added "' + body.name + '".');
          $('#add-result-read').text('');
          showAddState('result');
          briefTextFor(body.name).then(function (text) { $('#add-result-brief').val(text); });
          reloadRows().catch(reloadFailed);
          return;
        }
        addStatus(res.text.trim() || ('The import failed (status ' + res.status + ').'), true);
      })
      .catch(function (err) {
        $('#add-hf-go').prop('disabled', false).text('Import');
        addStatus('The import could not be sent: ' + (err.message || err), true);
      });
  });

  $('#add-form').on('submit', function (e) {
    e.preventDefault();
    var name = String($('#af_name').val() || '').trim();
    if (!name) { $('#add-form-status').addClass('is-shown').text('A name is required.'); return; }
    var read = changedFormCells('af', {}, 'add');
    if (read.error) { $('#add-form-status').addClass('is-shown').text(read.error); return; }
    $('#add-form-save').prop('disabled', true);
    addStatus('Adding "' + name + '"...');
    fetch('/api/models/add', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: name, cells: read.cells })
    })
      .then(function (r) { return r.text().then(function (t) { return { status: r.status, text: t }; }); })
      .then(function (res) {
        $('#add-form-save').prop('disabled', false);
        if (res.status !== 201) {
          $('#add-form-status').addClass('is-shown').text(res.text.trim() ||
            ('The model was not added (status ' + res.status + ').'));
          addStatus('');
          return;
        }
        var added = JSON.parse(res.text).name;
        addStatus('');
        $('#add-result-what').text('Added "' + added + '".');
        $('#add-result-read').text('It was added exactly as you typed it. Nothing was looked up.');
        showAddState('result');
        briefTextFor(added).then(function (text) { $('#add-result-brief').val(text); });
        reloadRows().catch(reloadFailed);
      })
      .catch(function (err) {
        $('#add-form-save').prop('disabled', false);
        addStatus('The model could not be sent: ' + (err.message || err), true);
      });
  });

  // --- Editing one model in a form -----------------------------------------

  var editDialog = document.getElementById('edit-modal');
  var editRowName = null;

  $('#models').on('keydown', '.row-edit', function (e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault(); e.stopPropagation();
    $(this).trigger('click');
  });
  $('#models').on('click', '.row-edit', function (e) {
    e.stopPropagation();
    var name = $(this).closest('tr').attr('data-name') || '';
    editRowName = name;
    $('#edit-modal-title').text('Edit "' + name + '"');
    $('#edit-form-status').removeClass('is-shown').empty();
    $('#edit-form-fields').empty().text('Loading...');
    $('#edit-status').text('');
    editDialog.showModal();
    refreshMoveControls();
    refreshRow(name).then(function (row) {
      if (!row) { $('#edit-status').text('"' + name + '" is no longer in this database.'); return; }
      buildModelForm($('#edit-form-fields'), 'ef', 'edit', row);
      $('#edit-owned-note').text(ownedNote(row)).prop('hidden', false);
      $('#edit-save').prop('disabled', false);
    }).catch(function (e) {
      $('#edit-form-fields').empty();
      $('#edit-status').text('"' + name + '" could not be read: ' + (e.message || e));
    });
  });
  // Clearing an overtaken researched value from its read-only line: the one
  // write the server takes on a cell an automatic source holds. The form is
  // rebuilt from the row read back, so the line loses its note.
  $('#edit-form-fields').on('click', '.field-clear-research', function () {
    var key = $(this).attr('data-key');
    var name = editRowName;
    var cells = {};
    cells[key] = '';
    $(this).prop('disabled', true);
    $('#edit-status').text('Clearing...');
    saveCells(name, cells)
      .then(function () { return refreshRow(name); })
      .then(function (row) {
        $('#edit-status').text('');
        if (!row) return;
        buildModelForm($('#edit-form-fields'), 'ef', 'edit', row);
        $('#edit-owned-note').text(ownedNote(row));
        flashOk('Cleared the researched ' + ((columnByKey[key] || {}).label || key));
        focusByIdentity(['#' + formFieldId('ef', key), '#edit-form-fields .form-field[data-key="' + key + '"]', '#edit-save']);
      })
      .catch(function (err) {
        $('#edit-status').text('');
        $('#edit-form-status').addClass('is-shown').text(String(err.message || err));
      });
  });
  $('#edit-close, #edit-cancel').on('click', function () { if (editDialog.open) editDialog.close(); });
  $('#edit-modal').on('click', function (e) { if (e.target === this) editDialog.close(); });

  $('#edit-form').on('submit', function (e) {
    e.preventDefault();
    var name = editRowName;
    var row = rowsByName[name] || {};
    var read = changedFormCells('ef', row, 'edit');
    if (read.error) { $('#edit-form-status').addClass('is-shown').text(read.error); return; }
    if (!Object.keys(read.cells).length) { $('#edit-status').text('Nothing was changed, so nothing was saved.'); return; }
    $('#edit-save').prop('disabled', true);
    $('#edit-status').text('Saving...');
    saveCells(name, read.cells)
      .then(function () { return refreshRow(name); })
      .then(function () {
        $('#edit-save').prop('disabled', false);
        editDialog.close();
        flashOk('Saved "' + name + '"');
        refreshCatalogEmptiness();
      })
      .catch(function (err) {
        $('#edit-save').prop('disabled', false);
        $('#edit-status').text('');
        $('#edit-form-status').addClass('is-shown').text(String(err.message || err));
      });
  });

  // --- Details, and the brief for an AI assistant ---------------------------

  function briefTextFor(name) {
    return fetch('/api/models/brief?name=' + encodeURIComponent(name) +
      '&origin=' + encodeURIComponent(window.location.origin))
      .then(jsonOrThrow)
      .then(function (body) { return body.brief || ''; })
      .catch(function (err) { return 'The brief could not be read from this server: ' + (err.message || err); });
  }

  // --- Reading a model's repository again, from its details -----------------
  // A model already in the catalog is met HERE - the add dialog is for one that
  // is not here yet - so this is where the re-import has to be, or it is
  // reachable only by adding the model a second time. It re-reads the
  // repository and rewrites what the importer MEASURES, which is the
  // repository's own columns and nothing else.

  var detailsName = null;
  var REIMPORT_NAMES_SHOWN = 8;

  // What a re-import will overwrite, NAMED before anything is written, because
  // nothing here can be undone. The columns it names are the repository's own
  // columns that already hold a value on this row, which are the ones with
  // something to lose; a repository column this row has no value in is filled
  // rather than overwritten, so it is counted instead of listed.
  function reimportOverwriteSummary(row) {
    var holding = [], empty = 0;
    columns.forEach(function (column) {
      if (!isMeasured(column)) return;
      if (String(cellValue(row, column.key)).trim() !== '') holding.push(column.label);
      else empty++;
    });
    var shown = holding.slice(0, REIMPORT_NAMES_SHOWN).join(', ');
    if (holding.length > REIMPORT_NAMES_SHOWN) {
      shown += ' and ' + (holding.length - REIMPORT_NAMES_SHOWN) + ' more';
    }
    var parts = ['Reading the repository again rewrites what ModelsDB MEASURES there, which is the ' +
      'repository\'s own columns and nothing else.'];
    parts.push(holding.length
      ? 'It overwrites ' + holding.length + ' of them, which hold a value now: ' + shown + '.'
      : 'No repository column holds a value on this row yet, so nothing is overwritten.');
    if (empty) {
      parts.push('Up to ' + empty + ' more repository column' + (empty === 1 ? '' : 's') +
        ' can be filled where this reading states something the row has no value for.');
    }
    return parts.join(' ');
  }

  var REIMPORT_KEEPS = 'Nothing of yours is touched: your note, your favourite star, ' +
    'every column you made and every value in it, and whether the row is hidden all stay as they are. ' +
    'A value filled in by research on a column the repository measures is replaced where the repository ' +
    'states one, and kept where it does not.';

  function closeReimportPanel() {
    $('#modal-reimport-panel').prop('hidden', true);
    $('#modal-reimport-status').text('');
    $('#modal-reimport-go').prop('disabled', false).text('Read the repository again');
  }

  // The control the details were opened from, so closing puts focus back on
  // it rather than dropping it to the top of the page.
  var detailsOpener = null;

  function closeDetails() {
    $('#modal').css('display', 'none');
    if (detailsOpener && document.contains(detailsOpener) && $(detailsOpener).is(':visible')) {
      $(detailsOpener).trigger('focus');
    }
    detailsOpener = null;
  }

  $('#models').on('click', '.name-open', function () {
    var name = $(this).closest('tr').attr('data-name') || '';
    var row = rowsByName[name] || {};
    detailsName = name;
    detailsOpener = this;
    $('#modal-json').text(JSON.stringify(row, null, 2));
    $('#modal-brief').val('Working out what this model is missing...');
    briefTextFor(name).then(function (text) { $('#modal-brief').val(text); });
    $('#modal-copy-status').text('');
    // Offered only where there is a repository to re-read, which is the only
    // kind of row an import can say anything about.
    $('#modal-reimport').prop('hidden', !repositoryOf(row));
    closeReimportPanel();
    $('#modal').css('display', 'flex');
    // Into the overlay, on its title: Escape is handled on the overlay itself,
    // so focus left behind on the table would leave the key with nothing to
    // reach, and a keyboard reader would be standing behind what they opened.
    $('#modal-title').trigger('focus');
  });

  // Escape closes the overlay, as it closes every native dialog on the page.
  // It is a plain overlay rather than a <dialog>, so the key is answered here.
  $('#modal').on('keydown', function (e) {
    if (e.key === 'Tab') { trapTabIn(e, this); return; }
    if (e.key !== 'Escape') return;
    e.preventDefault();
    closeDetails();
  });

  $('#modal-reimport').on('click', function () {
    var row = rowsByName[detailsName] || {};
    $('#modal-reimport-what').text(reimportOverwriteSummary(row));
    $('#modal-reimport-safe').text(REIMPORT_KEEPS);
    $('#modal-reimport-panel').prop('hidden', false);
    $('#modal-reimport-go').trigger('focus');
  });
  $('#modal-reimport-cancel').on('click', closeReimportPanel);

  $('#modal-reimport-go').on('click', function () {
    var name = detailsName;
    var repo = repositoryOf(rowsByName[name] || {});
    if (!repo) return;
    $('#modal-reimport-go').prop('disabled', true).text('Reading the repository...');
    $('#modal-reimport-status').text('Reading ' + repo + ' on HuggingFace. This takes a few seconds.');
    fetch('/api/models/import', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: repo, reimport: true })
    })
      .then(function (r) { return r.text().then(function (t) { return { status: r.status, text: t }; }); })
      .then(function (res) {
        $('#modal-reimport-go').prop('disabled', false).text('Read the repository again');
        if (res.status !== 200 && res.status !== 201) {
          $('#modal-reimport-status').text(res.text.trim() || ('The re-import failed (status ' + res.status + ').'));
          return;
        }
        var body = JSON.parse(res.text);
        $('#modal-reimport-status').text('Read again. ' + importReadLine(body) +
          ' What ModelsDB measures was rewritten; everything of yours is as it was.');
        // The row on the page is read back so the details JSON and the table
        // both show what the database now holds.
        refreshRow(body.name).then(function (fresh) {
          if (fresh) $('#modal-json').text(JSON.stringify(fresh, null, 2));
          briefTextFor(body.name).then(function (text) { $('#modal-brief').val(text); });
        });
        refreshCatalogEmptiness();
      })
      .catch(function (err) {
        $('#modal-reimport-go').prop('disabled', false).text('Read the repository again');
        $('#modal-reimport-status').text('The re-import could not be sent: ' + (err.message || err));
      });
  });

  // What the import read, in one short line per value it found.
  function importReadLine(body) {
    var read = body.read || {};
    var parts = [];
    if (body.context_length) parts.push('Context ' + Number(body.context_length).toLocaleString() + ' tokens.');
    if (read.parameters) parts.push(twoDecimals(read.parameters) + 'B parameters.');
    if (read.weights_size_gb) parts.push(twoDecimals(read.weights_size_gb) + ' GB on disk.');
    return parts.join(' ');
  }
  $('#modal-close, #modal').on('click', function (e) {
    if (e.target === this) closeDetails();
  });
  $('#modal-agent').on('click', function () { copyOnButton($(this), $('#modal-brief').val() || '', 'Copy agent brief', '#modal-copy-status'); });
  $('#modal-copy').on('click', function () { copyOnButton($(this), $('#modal-json').text(), 'Copy JSON', '#modal-copy-status'); });
  $('#add-copy-instructions').on('click', function () {
    copyOnButton($(this), $('#add-result-brief').val() || '', 'Copy agent brief', '#add-copy-status');
  });

  $('#models').on('click', '.copy-id', function (e) {
    e.stopPropagation();
    var $el = $(this);
    copyText($el.attr('data-id')).then(function () {
      $el.addClass('copied').html(CHECK_ICON);
      setTimeout(function () { $el.removeClass('copied').html(COPY_ICON); }, 1100);
    }).catch(function () {
      statusErr('Could not copy the id.');
    });
  });
  // The copy-id mark is a span with role="button", which the browser gives no
  // key behaviour; Enter and Space do what a click does, and Space must not
  // scroll the page.
  $('#models').on('keydown', '.copy-id', function (e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault(); e.stopPropagation();
    $(this).trigger('click');
  });

  function copyOnButton($btn, text, label, statusSelector) {
    var $said = statusSelector ? $(statusSelector) : $();
    copyText(text).then(function () {
      $said.text('');
      $btn.text('Copied');
      setTimeout(function () { $btn.text(label); }, 1100);
    }).catch(function () {
      $said.text('The copy did not go through. The text is on this panel - select it and copy it by hand.');
      $btn.text('Copy failed');
      setTimeout(function () { $btn.text(label); }, 1100);
    });
  }

  // navigator.clipboard exists only in a secure context, which a page served
  // over plain http at a network address is not - so the textarea route is the
  // one that actually runs for anybody reading this app from another machine.
  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) return navigator.clipboard.writeText(text);
    return new Promise(function (resolve, reject) {
      var open = Array.prototype.filter.call(document.querySelectorAll('dialog'), function (d) { return d.open; });
      var host = open.length ? (open.find(function (d) { return d.contains(document.activeElement); }) || open[open.length - 1]) : document.body;
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('aria-hidden', 'true');
      ta.style.position = 'fixed';
      ta.style.top = '-1000px';
      var copied = false;
      ta.addEventListener('copy', function () { copied = true; });
      host.appendChild(ta);
      try { ta.focus(); ta.select(); document.execCommand('copy'); } catch (e) { /* judged by the event */ }
      host.removeChild(ta);
      if (copied) resolve(); else reject(new Error('this browser did not let the page write to the clipboard'));
    });
  }

  // --- Refreshing the catalog ----------------------------------------------

  var updateRunning = false;
  var updatePollTimer = null;
  var PHASE_LABELS = { catalog: 'Catalog' };

  // What the last update added, and the way to look at it. The Added column
  // ships HIDDEN, so a bare count names rows the reader cannot find; the
  // action shows that column and sorts by it, newest first. The layout
  // changes only when it is pressed.
  function paintModelsAdded(added) {
    $('#models-added-text').text(added + ' model' + (added === 1 ? '' : 's') +
      ' new to your catalog ' + (added === 1 ? 'was' : 'were') + ' added by this update.');
    showBanner('models-added-banner');
  }
  $('#models-added-dismiss').on('click', function () {
    $('#models-added-banner').removeClass('show');
  });
  $('#models-added-show').on('click', function () {
    if (hiddenTargets[ADDED_KEY]) {
      delete hiddenTargets[ADDED_KEY];
      delete filterOffTargets[ADDED_KEY];
      columnsShownChanged();
    }
    var idx = getColIdx(ADDED_KEY);
    if (idx < 0) { statusWarn('This catalog has no ' + ADDED_KEY + ' column to sort by.'); return; }
    table.order([[idx, 'desc']]).draw();
    $('#m_sort').val(ADDED_KEY);
    $('#m_sort_dir').attr('data-dir', 'desc').text(mobileSortGlyph('desc'));
    $('#models-added-banner').removeClass('show');
    announce('status-announce', 'Sorted by ' + ADDED_KEY + ', newest first');
  });

  function setUpdateBtnDisabled(disabled) {
    var btn = document.getElementById('btn-update');
    if (!btn) return;
    btn.disabled = disabled;
    btn.textContent = disabled ? 'Updating models...' : 'Update models';
  }
  function pollUpdateStatus() {
    fetch('/api/update/status').then(function (r) { return r.json(); }).then(function (st) {
      if (st.running) {
        dbStatus('syncing');
        statusText('Updating: ' + (PHASE_LABELS[st.phase] || st.phase || '') + ' ...');
        updatePollTimer = setTimeout(pollUpdateStatus, 2500);
        return;
      }
      updateRunning = false;
      updatePollTimer = null;
      setUpdateBtnDisabled(false);
      dbStatus('online');
      // How many models the update brought in, off the status the page is
      // already polling: the number costs no request of its own and nothing
      // here ever calls OpenRouter.
      var added = (typeof st.added === 'number' && st.added > 0) ? Math.floor(st.added) : 0;
      // This page ran this update, so it is not told about it again by the bar
      // that announces runs it did not start.
      handledRunSeq = (st.last_run && st.last_run.seq) || handledRunSeq;
      // Where the update added models the rows are re-read in place, because a
      // reload would wipe the bar that reports the count; an update that added
      // nothing clears the previous count's bar.
      if (added) { paintModelsAdded(added); reloadRows().catch(reloadFailed); }
      else $('#models-added-banner').removeClass('show');
      if (st.errors && st.errors.length) statusErr('Update finished with errors: ' + st.errors.join('; '));
      // A reload is asked for only where the rows were not re-read.
      else if (st.warnings && st.warnings.length) statusWarn((added ? 'Update complete. ' : 'Update complete, reload to see the change. ') + st.warnings.map(function (w) { return 'Warning: ' + w; }).join(' '));
      else if (added) statusOk('Update complete');
      else { statusOk('Update complete - reloading...'); setTimeout(function () { location.reload(); }, 800); }
    }).catch(function () { updatePollTimer = setTimeout(pollUpdateStatus, 2500); });
  }
  function beginUpdateTracking() {
    updateRunning = true;
    setUpdateBtnDisabled(true);
    dbStatus('syncing');
    statusText('Updating: starting ...');
    if (!updatePollTimer) pollUpdateStatus();
  }
  // The empty-catalog panel's own button. It is the same act as the menu's, so
  // it is the same handler rather than a second copy of it.
  $('#empty-catalog-update').on('click', function () { $('#btn-update').trigger('click'); });
  $('#btn-update').on('click', function () {
    if (updateRunning) return;
    setUpdateBtnDisabled(true);
    fetch('/api/update', { method: 'POST' })
      .then(function (r) { return r.json().then(function (body) { return { status: r.status, body: body }; }); })
      .then(function (res) {
        if ((res.status === 202 && res.body.started) || (res.status === 409 && res.body.running)) beginUpdateTracking();
        else { setUpdateBtnDisabled(false); statusErr('Update could not be started.'); }
      })
      .catch(function (e) { setUpdateBtnDisabled(false); statusErr('Update request failed: ' + (e.message || e)); });
  });
  // A page loaded while a SCHEDULED run is in progress does not join it: the
  // daily update is silent, so no button text, status line or reload comes of it.
  // Pressing Update models during it still gets the 409 and joins, as for any run.
  fetch('/api/update/status').then(function (r) { return r.json(); })
    .then(function (st) {
      if (st.running && !st.scheduled) beginUpdateTracking();
      noteRunStatus(st, true);
    }).catch(function () {});

  setInterval(function () {
    if (updateRunning) return;
    fetch('/api/health').then(function (r) { dbStatus(r.ok ? 'online' : 'offline'); })
      .catch(function () { dbStatus('offline'); });
    // The same tick asks whether a run this page did not start has finished. It
    // is a second request on the existing timer, not a timer of its own.
    fetch('/api/update/status').then(function (r) { return r.json(); })
      .then(function (st) { noteRunStatus(st, false); }).catch(function () {});
  }, 30000);

  // --- The daily model update --------------------------------------------------
  // The server can run the same update as the button once a day (the switch in
  // Info). It runs silently; this is what the page says about it afterwards.
  // Everything here is read off GET /api/update/status: last_run is the record of
  // the last finished run, whoever started it, and its seq rises with each one.
  //
  // handledRunSeq is the newest run this page has dealt with: at load, the run
  // that was already the last, and afterwards each run it has shown or ran
  // itself. A run newer than it is one the page did not start, which is the only
  // kind that gets the bar. The page NEVER reloads on its own because of one.
  var handledRunSeq = 0;
  var lastRunStatus = null;
  // The failure the reader dismissed, so an hourly retry that fails the same way
  // does not bring the bar back every hour; a different error or a success does.
  var dismissedFailure = null;
  var shownFailure = null;

  function clipError(text) {
    text = String(text || 'no reason given');
    return text.length > 120 ? text.slice(0, 120) + '...' : text;
  }
  function partsFailedWord(parts) {
    return parts.length + (parts.length === 1 ? ' part' : ' parts') + ' failed';
  }
  // What the bar says about a finished run, and what its button is.
  function showRunBar(run) {
    var when = new Date(run.finished_at).toLocaleString();
    var parts = run.failed_parts || [];
    var failed = run.outcome === 'failed';
    var text;
    if (failed) {
      var reason = clipError(run.error);
      if (dismissedFailure === reason) { $('#catalog-updated-banner').removeClass('show'); return; }
      shownFailure = reason;
      text = 'The model update failed at ' + when + ': ' + reason + '. Nothing changed.';
    } else {
      shownFailure = null;
      dismissedFailure = null;
      text = parts.length
        ? 'Models were updated at ' + when + ', but ' + partsFailedWord(parts) + ' (' + parts.join(', ') + '). Refresh to see them.'
        : 'Models were updated at ' + when + '. Refresh to see them.';
    }
    $('#catalog-updated-text').text(text);
    $('#catalog-updated-refresh').toggle(!failed);
    $('#catalog-updated-info').toggle(failed);
    showBanner('catalog-updated-banner');
    // A bar that goes from hidden to shown is not reliably announced by its own
    // role, so the same words go to the live region as well. Focus does not move.
    announce('status-announce', text);
  }
  // noteRunStatus takes one status answer. At load it records which run is the
  // last and shows the bar only for a failure; afterwards it shows the bar for a
  // run that finished since, once no run is in flight.
  function noteRunStatus(st, atLoad) {
    lastRunStatus = st;
    var run = st.last_run || {};
    var seq = run.seq || 0;
    renderDailyUpdateStatus();
    if (atLoad) {
      handledRunSeq = seq;
      if (run.outcome === 'failed') showRunBar(run);
      return;
    }
    if (updateRunning || st.running || seq <= handledRunSeq) return;
    handledRunSeq = seq;
    showRunBar(run);
  }
  $('#catalog-updated-refresh').on('click', function () { location.reload(); });
  $('#catalog-updated-info').on('click', function () { $('#btn-paths').trigger('click'); });
  $('#catalog-updated-dismiss').on('click', function () {
    if (shownFailure !== null) dismissedFailure = shownFailure;
    $('#catalog-updated-banner').removeClass('show');
  });

  // The line under the switch in Info. Times are the reader's own locale, as the
  // backup line's are.
  function dailyUpdateStatusText(st, enabled) {
    if (st.running) {
      var started = 'Updating now, started ' + new Date(st.started_at).toLocaleString() + '.';
      return enabled ? started : started + ' Switched off: this run finishes, no more after it.';
    }
    var run = st.last_run || {};
    if (!run.outcome) return 'Never run yet.';
    var when = new Date(run.finished_at).toLocaleString();
    var parts = run.failed_parts || [];
    if (run.outcome === 'failed') {
      return 'Last attempt: ' + when + ' - failed (' + clipError(run.error) + ').' + (enabled ? ' Retrying within the hour.' : '');
    }
    if (parts.length) return 'Last update: ' + when + ' - done, but ' + partsFailedWord(parts) + ': ' + parts.join(', ') + '.';
    return 'Last update: ' + when + ' - ok.';
  }
  function renderDailyUpdateStatus() {
    if (!pathsDialog.open || !lastRunStatus) return;
    $('#daily-update-status').text(dailyUpdateStatusText(lastRunStatus, $('#daily-update-enabled').is(':checked')));
  }
  // Read when Info opens, so the line is current without a timer of its own.
  function refreshDailyUpdateStatus() {
    $('#daily-update-status').text('Loading...');
    fetch('/api/update/status').then(jsonOrThrow).then(function (st) {
      lastRunStatus = st;
      renderDailyUpdateStatus();
    }).catch(function () {
      $('#daily-update-status').text('Could not read the update status.');
    });
  }
  $('#daily-update-enabled').on('change', function () {
    var box = this;
    var on = box.checked;
    renderDailyUpdateStatus();
    // No toast, like the other switches in Info: the toast is drawn behind this
    // dialog on a phone. A failed save is said in the status line and the box
    // goes back to what is stored.
    saveSettings(function () {
      box.checked = !on;
      $('#daily-update-status').text('Could not save the daily update setting.');
    });
  });

  // --- Updating the app itself ---------------------------------------------

  var BANNER_UPDATE_IDS = { now: 'app-update-now', restart: 'app-update-restart', progress: 'app-update-progress' };
  var MODAL_UPDATE_IDS = { now: 'modal-app-update-now', restart: 'modal-app-update-restart', progress: 'modal-app-update-progress' };

  function renderAppUpdate(st, ids) {
    var nowBtn = document.getElementById(ids.now);
    var restartBtn = document.getElementById(ids.restart);
    var prog = document.getElementById(ids.progress);
    if (!nowBtn || !restartBtn || !prog) return;
    var state = (st && st.state) || 'idle';
    nowBtn.style.display = 'none';
    restartBtn.style.display = 'none';
    prog.textContent = '';
    if (state === 'idle' || state === 'failed') {
      nowBtn.disabled = false;
      nowBtn.style.display = '';
      if (state === 'failed') prog.textContent = st && st.error ? 'Update failed: ' + st.error : 'Update failed.';
    } else if (state === 'downloading') prog.textContent = 'Downloading update...';
    else if (state === 'verifying') prog.textContent = 'Verifying update...';
    else if (state === 'staged') {
      restartBtn.disabled = false;
      restartBtn.style.display = '';
      prog.textContent = 'Update staged - restart to apply.';
    }
  }
  function pollAppUpdateUntilDone(ids) {
    fetch('/api/app-update/status').then(function (r) { return r.json(); }).then(function (st) {
      renderAppUpdate(st, ids);
      if (st.state === 'downloading' || st.state === 'verifying') setTimeout(function () { pollAppUpdateUntilDone(ids); }, 2000);
    }).catch(function () { setTimeout(function () { pollAppUpdateUntilDone(ids); }, 2000); });
  }
  var RECONNECT_MAX_TRIES = 128;
  function reconnectAfterRestart(ids) {
    var prog = document.getElementById(ids.progress);
    if (prog) prog.textContent = 'Restarting...';
    var sawDown = false, tries = 0;
    (function ping() {
      tries++;
      if (tries > RECONNECT_MAX_TRIES) { if (prog) prog.textContent = 'Update restart failed to reconnect.'; return; }
      fetch('/api/health', { cache: 'no-store' }).then(function (r) {
        if (!r.ok) throw new Error('not ok');
        if (sawDown) { if (prog) prog.textContent = 'Reconnected - reloading...'; setTimeout(function () { location.reload(); }, 400); return; }
        setTimeout(ping, 700);
      }).catch(function () { sawDown = true; if (prog) prog.textContent = 'Reconnecting...'; setTimeout(ping, 700); });
    })();
  }
  function wireAppUpdateControls(ids) {
    var nowBtn = document.getElementById(ids.now);
    if (nowBtn) nowBtn.addEventListener('click', function () {
      nowBtn.disabled = true;
      var prog = document.getElementById(ids.progress);
      if (prog) prog.textContent = 'Starting update...';
      fetch('/api/app-update/apply', { method: 'POST' })
        .then(function (r) { return r.json().then(function (body) { return { status: r.status, body: body }; }); })
        .then(function (res) {
          if (res.status === 202) { renderAppUpdate({ state: 'downloading' }, ids); pollAppUpdateUntilDone(ids); }
          else { nowBtn.disabled = false; if (prog) prog.textContent = 'Could not start update: ' + (res.body.error || res.status); }
        })
        .catch(function (e) { nowBtn.disabled = false; if (prog) prog.textContent = 'Update request failed: ' + (e.message || e); });
    });
    var restartBtn = document.getElementById(ids.restart);
    if (restartBtn) restartBtn.addEventListener('click', function () {
      restartBtn.disabled = true;
      fetch('/api/app-update/restart', { method: 'POST' })
        .then(function () { reconnectAfterRestart(ids); })
        .catch(function () { reconnectAfterRestart(ids); });
    });
  }
  wireAppUpdateControls(BANNER_UPDATE_IDS);
  wireAppUpdateControls(MODAL_UPDATE_IDS);

  fetch('/api/update-check').then(function (r) { return r.json(); }).then(function (info) {
    var v = document.getElementById('app-version');
    if (v && info && info.current) {
      v.textContent = info.current === 'dev' ? 'dev' : 'v' + info.current;
      v.title = info.current === 'dev' ? 'Development build (no version injected)' : 'Version ' + info.current;
    }
    if (info && info.available && info.latest) {
      var b = document.getElementById('update-banner');
      var t = document.getElementById('update-banner-text');
      if (!b || !t) return;
      t.textContent = 'ModelsDB v' + info.latest + ' is available (you have v' + (info.current || '?') + '). ';
      b.classList.add('show');
      fetch('/api/app-update/status').then(function (r) { return r.json(); }).then(function (st) {
        if (st && st.updatable) {
          renderAppUpdate(st, BANNER_UPDATE_IDS);
          if (st.state === 'downloading' || st.state === 'verifying') pollAppUpdateUntilDone(BANNER_UPDATE_IDS);
        } else {
          var a = document.createElement('a');
          a.textContent = 'Download the new release';
          a.href = info.url || '#';
          a.target = '_blank';
          a.rel = 'noopener';
          t.appendChild(a);
        }
      }).catch(function () {});
    }
  }).catch(function () {});
  $('#update-dismiss').on('click', function () { $('#update-banner').removeClass('show'); });

  // Dismissing the settings-reset notice is a decision, not a gesture: the
  // server deletes the row it kept, so the bar is gone for good rather than
  // until the next reload. The bar goes whatever the request answers - somebody
  // who has read it should not have to read it again because a write failed.
  $('#settings-notice-dismiss').on('click', function () {
    $('#settings-notice').removeClass('show');
    fetch('/api/settings/ack-notice', { method: 'POST' }).catch(function (e) {
      console.error('Failed to dismiss the settings notice', e);
    });
  });

  $('#settings-read-reload').on('click', function () { location.reload(); });
  $('#view-not-stored-reload').on('click', function () { location.reload(); });

  $('#btn-check-update').on('click', function () {
    var $btn = $(this);
    $btn.prop('disabled', true);
    $('#check-update-status').text('Checking...');
    fetch('/api/update-check?force=1').then(function (r) { return r.json(); }).then(function (info) {
      $btn.prop('disabled', false);
      if (info && info.available && info.latest) {
        $('#check-update-status').text('Update available: v' + info.latest);
        $('#modal-app-update-source').attr('href', info.url || '#').css('display', '');
        fetch('/api/app-update/status').then(function (r) { return r.json(); }).then(function (st) {
          if (st && st.updatable) renderAppUpdate(st, MODAL_UPDATE_IDS);
        }).catch(function () {});
      } else if (info && info.latest) {
        $('#check-update-status').text("You're on the latest version (v" + info.current + ').');
      } else {
        $('#check-update-status').text('Could not reach the update server. Try again shortly.');
      }
    }).catch(function () {
      $btn.prop('disabled', false);
      $('#check-update-status').text('Could not reach the update server. Try again shortly.');
    });
  });

  // --- The theme ------------------------------------------------------------

  (function () {
    var btn = document.getElementById('btn-theme');
    if (!btn) return;
    function syncTitle() {
      var dark = document.documentElement.getAttribute('data-theme') === 'dark';
      var label = dark ? 'Switch to light theme' : 'Switch to dark theme';
      btn.title = label;
      btn.setAttribute('aria-label', label);
    }
    syncTitle();
    btn.addEventListener('click', function () {
      var dark = document.documentElement.getAttribute('data-theme') === 'dark';
      var next = dark ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', next);
      try { localStorage.setItem('theme', next); } catch (e) {}
      syncTitle();
    });
  })();

  // --- The phone text size ---------------------------------------------------
  // One button in the top bar (drawn on a phone only, see style.css) steps the
  // text normal -> larger -> largest -> normal. The step is the data-text-size
  // attribute on <html>, which style.css multiplies into the two font tokens;
  // the inline script in index.html sets it before first paint from the same
  // localStorage key. Whole-sentence labels, no pieces glued together.
  (function () {
    var btn = document.getElementById('btn-textsize');
    if (!btn) return;
    var STEPS = ['normal', 'larger', 'largest'];
    var LABELS = {
      normal: 'Text size: normal. Press for larger.',
      larger: 'Text size: larger. Press for largest.',
      largest: 'Text size: largest. Press for normal.'
    };
    function current() {
      var v = document.documentElement.getAttribute('data-text-size');
      return STEPS.indexOf(v) === -1 ? 'normal' : v;
    }
    function sync() {
      var label = LABELS[current()];
      btn.title = label;
      btn.setAttribute('aria-label', label);
    }
    sync();
    btn.addEventListener('click', function () {
      var next = STEPS[(STEPS.indexOf(current()) + 1) % STEPS.length];
      if (next === 'normal') document.documentElement.removeAttribute('data-text-size');
      else document.documentElement.setAttribute('data-text-size', next);
      try {
        if (next === 'normal') localStorage.removeItem('modelsdb.text_size');
        else localStorage.setItem('modelsdb.text_size', next);
      } catch (e) {}
      sync();
      // The bar height and the scroll length are measured once and remembered.
      forgetTopbarHeight();
      applyFixedHeader();
      updateTopScrollWidth();
    });
  })();

  // --- The top bar's menu -----------------------------------------------------
  // A real ARIA menu, not a styled list: the button carries aria-haspopup,
  // aria-controls and a live aria-expanded; the popup is role=menu over
  // role=menuitem entries; the arrow keys move and wrap, Home/End jump to the
  // edges, Escape closes and puts focus back on the button, and Tab or a click
  // anywhere else closes it. The entries act through the handlers bound to
  // their ids; no entry's behaviour lives here.

  (function () {
    var btn = document.getElementById('btn-menu');
    var menu = document.getElementById('topbar-menu');
    if (!btn || !menu) return;
    // The entries the arrow keys visit. A disabled entry (Update models while
    // a refresh runs) is skipped: a disabled button cannot take focus, and its
    // own text and state already say why it cannot be pressed.
    function menuItems() {
      return Array.prototype.filter.call(menu.querySelectorAll('[role="menuitem"]'),
        function (el) { return !el.disabled; });
    }
    function openMenu(focusLast) {
      menu.hidden = false;
      btn.setAttribute('aria-expanded', 'true');
      var items = menuItems();
      if (items.length) items[focusLast ? items.length - 1 : 0].focus();
    }
    function closeMenu(refocus) {
      if (menu.hidden) return;
      menu.hidden = true;
      btn.setAttribute('aria-expanded', 'false');
      if (refocus) btn.focus();
    }
    // Enter and Space arrive as a click on a real button.
    btn.addEventListener('click', function () {
      if (menu.hidden) openMenu(false); else closeMenu(false);
    });
    btn.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown') { e.preventDefault(); openMenu(false); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); openMenu(true); }
    });
    menu.addEventListener('keydown', function (e) {
      var items = menuItems();
      var at = items.indexOf(document.activeElement);
      if (e.key === 'ArrowDown') { e.preventDefault(); if (items.length) items[(at + 1) % items.length].focus(); return; }
      if (e.key === 'ArrowUp') { e.preventDefault(); if (items.length) items[(at - 1 + items.length) % items.length].focus(); return; }
      if (e.key === 'Home') { e.preventDefault(); if (items.length) items[0].focus(); return; }
      if (e.key === 'End') { e.preventDefault(); if (items.length) items[items.length - 1].focus(); return; }
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); closeMenu(true); return; }
      if (e.key === 'Tab') { closeMenu(false); return; }
      // A link answers Enter by itself and Space not at all, so Space is wired
      // for the one entry that is a link.
      if (e.key === ' ' && document.activeElement && document.activeElement.tagName === 'A') {
        e.preventDefault();
        document.activeElement.click();
      }
    });
    // An entry that acts closes the menu behind itself; the dialogs the
    // entries open take the focus themselves, so nothing is refocused here.
    menu.addEventListener('click', function (e) {
      if (e.target.closest('[role="menuitem"]')) closeMenu(false);
    });
    document.addEventListener('click', function (e) {
      if (menu.hidden) return;
      if (btn.contains(e.target) || menu.contains(e.target)) return;
      closeMenu(false);
    });
  })();

  // --- The info dialog: paths and backups ------------------------------------

  var pathsDialog = document.getElementById('paths-modal');
  function padr(s, n) { s = String(s); while (s.length < n) s += ' '; return s; }
  function showPaths(p) {
    var dirs = p.dirs || {}, files = p.files || {};
    var lines = [];
    lines.push(padr('Executable:', 14) + (p.executable || ''));
    lines.push(padr('Version:', 14) + (p.version || '') + '    Port: ' + (p.port || ''));
    lines.push('');
    lines.push(padr('Config dir:', 14) + (dirs.config || ''));
    lines.push(padr('Data dir:', 14) + (dirs.data || ''));
    lines.push(padr('Cache dir:', 14) + (dirs.cache || ''));
    lines.push('');
    ['database', 'config.jsonc', 'backups', 'logs', 'pid', 'update-check'].forEach(function (k) {
      if (files[k]) lines.push(padr(k + ':', 14) + files[k]);
    });
    lines.push('');
    lines.push('Everything about your models - the catalog, your notes and every');
    lines.push('column you made - is inside the database file above. Copy that one');
    lines.push('file to back it all up.');
    document.getElementById('paths-pre').textContent = lines.join('\n');
  }
  $('#btn-paths').on('click', function () {
    document.getElementById('paths-pre').textContent = 'Loading...';
    selectInfoTab('tab-files', false);
    pathsDialog.showModal();
    fetch('/api/paths').then(function (r) { return r.json(); }).then(showPaths).catch(function (e) {
      document.getElementById('paths-pre').textContent = 'Failed to load paths: ' + (e.message || e);
    });
    refreshBackupStatus();
    refreshDailyUpdateStatus();
  });
  $('#paths-close').on('click', function () { if (pathsDialog.open) pathsDialog.close(); });
  $(pathsDialog).on('click', function (e) { if (e.target === this) pathsDialog.close(); });

  $('#pref-auto-update, #backup-enabled, #backup-overwrite').on('change', function () { saveSettings(); });
  $('#pref-auto-update').prop('checked', !!savedSettings.auto_update);
  $('#daily-update-enabled').prop('checked', savedSettings.daily_model_update !== false);
  $('#backup-enabled').prop('checked', !!savedSettings.backup_enabled);
  if (savedSettings.backup_dir) $('#backup-dir').val(savedSettings.backup_dir);
  $('#backup-overwrite').prop('checked', !!savedSettings.backup_overwrite);

  // --- The views ------------------------------------------------------------
  // One view is one arrangement of this table: its columns and their order, its
  // filters, its sort and its page size. The panel under the top bar is where a
  // reader moves between them, makes one, renames one and takes one away.
  //
  // This page's own list of them, seeded from the store and kept current from
  // here on. It is what the panel draws and what the second line of a row is
  // worked out from, so nothing on screen waits on a round trip.
  var VIEW_CAP = 20;
  var VIEW_NAME_MAX = 40;
  var viewPanel = document.getElementById('view-panel');
  var viewList = namedViews.map(function (v) {
    return {
      id: v.id, number: v.number, name: storedViewName(v),
      created: String(v.keys.created || ''), position: viewPositionOf(v.keys.position), keys: v.keys
    };
  });
  // An adoption's view has no name row until the load-time write lands, and a
  // load that could not read the settings has no stored view at all. Both are
  // looking at one arrangement all the same, so the panel says so.
  if (!viewList.length) {
    viewList.push({
      id: currentViewId,
      number: parseInt(currentViewId.slice(1), 10) || 1,
      name: 'Main', created: todayStamp(), position: null, keys: view
    });
  }
  viewList.forEach(function (v) { reserveViewNumber(v.number); });

  function sortViewList() { viewList.sort(compareViewOrder); }
  sortViewList();

  function viewById(id) {
    return viewList.filter(function (v) { return v.id === id; })[0] || null;
  }
  function currentView() { return viewById(currentViewId) || viewList[0]; }

  // What one view's stored rows say the table looks like. The view on screen
  // answers from the table itself, because its rows are only as fresh as the
  // last write and the line under its name is read while the reader is still
  // moving things.
  function viewKeys(v) {
    if (v.id === currentViewId && table && !viewSwitching) {
      var live = currentViewState();
      live.name = v.name;
      live.created = v.created;
      v.keys = live;
    }
    return v.keys;
  }

  // The model name is always drawn and is never in the chooser, so it counts: a
  // view with everything else hidden shows one column, which is what its line
  // says and what the note above the table is for.
  //
  // HIDE-EMPTY IS NOT PART OF THIS COUNT, and the note above the table asks the
  // same question through here for that reason. That cut is a preference over
  // the rows on show rather than something the reader chose about this view, so
  // counting it in one place and not the other is how a row reading "6 columns"
  // ends up beside a note saying the view shows only the model name.
  function countShownColumns(hidden) {
    var count = 1;
    columns.forEach(function (c) { if (!hidden[c.key]) count++; });
    return count;
  }
  function viewColumnCount(keys) { return countShownColumns(resolveHiddenTargets(keys)); }
  function viewFilterCount(keys) {
    var filters = sanitiseFilters(keys.filters);
    var count = Object.keys(filters).length;
    if (sanitiseSelectValue(keys.f_show_hidden, SHOW_HIDDEN_VALUES)) count++;
    if (sanitiseSelectValue(keys.f_variations, VARIATIONS_VALUES)) count++;
    if (sanitiseSelectValue(keys.f_edited_by_me, EDITED_VALUES)) count++;
    return count;
  }
  function viewSortLabel(keys) {
    var order = Array.isArray(keys.order) ? keys.order : [];
    var first = (order.length && Array.isArray(order[0])) ? order[0][0] : '';
    if (first === NAME_TARGET) return 'Name';
    if (first === VAR_TARGET) return 'Var';
    return columnByKey[first] ? columnByKey[first].label : '';
  }
  function columnsWord(count) { return count === 1 ? '1 column' : count + ' columns'; }
  function viewHintLine(keys) {
    var filters = viewFilterCount(keys);
    var sortedBy = viewSortLabel(keys);
    return columnsWord(viewColumnCount(keys)) + ', ' +
      (filters === 0 ? 'no filters' : filters === 1 ? '1 filter' : filters + ' filters') + ', ' +
      (sortedBy ? 'sorted by ' + sortedBy : 'unsorted');
  }

  function drawViewButton() {
    var name = currentView().name;
    $('#view-btn-name').text(name);
    $('#view-btn').attr('aria-label', 'Views: ' + name).attr('title', 'Views: ' + name);
  }
  // The help list's first item. Cheap to call whether or not the dialog, or the
  // list inside it, is on screen right now.
  function drawColumnsViewLine() {
    $('#cmg-help-view-line').text('Which columns are shown, and their left-to-right ' +
      'order, belong to the view you are in: ' + currentView().name +
      ". A column's name, its colours and a column of your own are the same in every view.");
  }

  // The note above the table that a view showing nothing but the model name
  // carries, so a new view is never an empty-looking table with no way out of
  // it. The empty catalog has its own panel and says its own thing.
  function syncOneColumnNote() {
    var say = countShownColumns(hiddenTargets) === 1 && $('#empty-catalog').is('[hidden]');
    $('#view-one-column').attr('hidden', say ? null : '');
  }

  function viewPanelOpen() { return !!viewPanel && !viewPanel.hidden; }

  // What the panel is in the middle of, on the current view's row: renaming it,
  // naming a new one, or asking whether to delete it. Nothing else in the panel
  // acts while one of them is open.
  var viewEditing = null;
  // The view a copy was made FROM, read at the moment of the copy, because the
  // line said when the naming box closes names both the copy and its source,
  // and the commit that says it carries only the name that was typed.
  var viewEditFrom = '';
  // Deleted views, newest first, for as long as this page lives.
  var viewUndos = [];
  // Raised from the moment a switch starts until its rows are in the table. In
  // that window the table still holds the view being left, so its state is not
  // the incoming view's and must not be read as it.
  var viewSwitching = false;
  // Which switch the flag belongs to. Two switches can be in flight at once,
  // and only the newest of them may take the flag down.
  var viewSwitchSeq = 0;

  function drawViewPanelMsg() {
    var msg = '';
    if (viewList.length <= 1) msg = 'This is your only view, so it cannot be deleted.';
    else if (viewList.length >= VIEW_CAP) msg = VIEW_CAP + ' views is the most this keeps. Delete one to make another.';
    $('#view-panel-msg').text(msg);
  }

  var VIEW_TICK_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"' +
    ' fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"' +
    ' stroke-linejoin="round"><path d="M5 13l4 4L19 7"/></svg>';

  function viewRowButton(v) {
    var isCurrent = v.id === currentViewId;
    var $row = $('<button type="button" class="view-row"></button>').attr('data-id', v.id);
    if (isCurrent) $row.attr('aria-current', 'true');
    var $tick = $('<span class="view-row-tick" aria-hidden="true"></span>');
    if (isCurrent) $tick.html(VIEW_TICK_ICON);
    var $lines = $('<span class="view-row-lines"></span>');
    $lines.append($('<span class="view-row-name"></span>').text(v.name));
    $lines.append($('<span class="view-row-hint"></span>').text(viewHintLine(viewKeys(v))));
    return $row.append($tick).append($lines);
  }

  var VIEW_UP_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"' +
    ' fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"' +
    ' stroke-linejoin="round"><path d="M6 15l6-6 6 6"/></svg>';
  var VIEW_DOWN_ICON = '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"' +
    ' fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"' +
    ' stroke-linejoin="round"><path d="M6 9l6 6 6-6"/></svg>';

  // Move up and Move down beside a view's row. They are never disabled at an
  // end of the list: a disabled button cannot be focused, so the refusal is
  // spoken in #view-panel-msg instead.
  function viewMoveButtons(v) {
    var $group = $('<span class="view-move"></span>');
    $group.append($('<button type="button" class="view-move-btn" data-dir="-1"></button>')
      .attr('aria-label', 'Move "' + v.name + '" up').html(VIEW_UP_ICON));
    $group.append($('<button type="button" class="view-move-btn" data-dir="1"></button>')
      .attr('aria-label', 'Move "' + v.name + '" down').html(VIEW_DOWN_ICON));
    return $group;
  }

  function drawViewUndos() {
    var $into = $('#view-undos').empty();
    viewUndos.forEach(function (u) {
      var $strip = $('<div class="view-undo"></div>');
      $strip.append($('<span class="view-undo-said"></span>').text('"' + u.name + '" deleted.'));
      $strip.append($('<button type="button" class="view-undo-btn">Undo</button>')
        .attr('data-id', u.id)
        .attr('aria-label', 'Undo deleting "' + u.name + '"'));
      $into.append($strip);
    });
  }

  function drawViewActions() {
    var name = currentView().name;
    $('#view-duplicate').attr('aria-label', 'Duplicate "' + name + '"');
    $('#view-rename').attr('aria-label', 'Rename "' + name + '"');
    $('#view-delete').attr('aria-label', 'Delete "' + name + '"');
    var full = viewList.length >= VIEW_CAP;
    $('#view-new').prop('disabled', full);
    $('#view-duplicate').prop('disabled', full);
    $('#view-delete').prop('disabled', viewList.length <= 1);
  }

  function renderViewPanel() {
    var $list = $('#view-list').empty();
    viewList.forEach(function (v) {
      var $item = $('<li></li>').attr('data-id', v.id);
      $item.append(viewRowButton(v));
      $item.append(viewMoveButtons(v));
      $list.append($item);
    });
    drawViewUndos();
    drawViewActions();
    drawViewPanelMsg();
  }

  function focusCurrentViewRow() {
    var $row = $('#view-list .view-row[aria-current="true"]');
    if (!$row.length) $row = $('#view-list .view-row').first();
    if ($row.length) $row.trigger('focus');
  }

  function openViewPanel() {
    if (!viewPanel || viewPanelOpen()) return;
    // The drawer and the panel are the same corner of a narrow screen, and the
    // funnel's aria-expanded has to say so the moment the drawer goes.
    $('body').removeClass('filters-drawer-open');
    syncFilterFunnel();
    viewEditing = null;
    renderViewPanel();
    viewPanel.hidden = false;
    $('#view-btn').attr('aria-expanded', 'true');
    focusCurrentViewRow();
  }

  // The panel closing is what ABANDONS a rename or a delete: clicking away from
  // a half-typed name is the one move nobody means as a save.
  function closeViewPanel(refocus) {
    if (!viewPanelOpen()) return;
    viewEditing = null;
    viewPanel.hidden = true;
    $('#view-btn').attr('aria-expanded', 'false');
    $('#view-panel-msg').text('');
    if (refocus) $('#view-btn').trigger('focus');
  }

  // A view is made, renamed, removed and put back by naming ITS OWN rows, never
  // by saveSettings, which writes the view on screen and the app's preferences
  // together. Nothing here may post the table state of one view under the id of
  // another.
  function postViewRows(set, removeKeys, onDone, onFail) {
    function refuse(e) {
      statusErr('That change was not saved: ' + (e.message || e));
      if (onFail) onFail(e);
    }
    if (settingsUnreadable) { refuse(new Error('the stored views could not be read')); return; }
    if (viewNotStored) { refuse(new Error('this page is not storing anything')); return; }
    fetch('/api/settings', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ set: set, 'delete': removeKeys || [] })
    }).then(function (response) {
      if (!response.ok) throw new Error('the server answered ' + response.status);
      if (onDone) onDone();
    }).catch(function (e) {
      console.error('Failed to save the views', e);
      refuse(e);
    });
  }

  // Everything a view holds goes into the controls, the address and this page's
  // own state, in one pass, and the rows are fetched after it. It is also what
  // puts the view being LEFT back when that fetch fails, which is why it takes
  // the id and the keys rather than reading them off the list: what goes back
  // is the arrangement that was on the table, read off the table.
  //
  // It writes NOTHING. Switching is a read of one view and the store already
  // holds both, so the only thing that changes anywhere is which id this device
  // remembers.
  function applyViewKeys(id, keys) {
    currentViewId = id;
    view = keys;
    rememberViewId(id);
    // The steps back and forward are this page's memory of ONE view's layout,
    // so they can never be offered against another's.
    forgetSteps();
    // A link's query decides the load it arrived on and nothing after it: the
    // reader has now asked for a view, and what that view holds is what goes
    // into the controls and back into the address.
    activeQuery = {};
    hiddenTargets = resolveHiddenTargets(keys);
    filterOffTargets = resolveFilterOffTargets(keys);
    hideEmpty = keys.hide_empty !== false;
    $('body').toggleClass('hide-empty', hideEmpty);
    $('#f_hide_empty').prop('checked', hideEmpty);
    showMarks = keys.show_marks === true;
    $('body').toggleClass('hide-marks', !showMarks);
    $('#f_show_marks').prop('checked', showMarks);
    fullText = keys.full_text === true;
    $('body').toggleClass('full-text', fullText);
    $('#f_full_text').prop('checked', fullText);
    cardsBelow = sanitiseCardsBelow(keys.cards_below);
    $('#f_cards_below').val(String(cardsBelow));
    // The arriving view can carry a different width than the one on screen,
    // so the breakpoint moves with it, live, the same way a manual change of
    // the control does.
    rebuildCardsQuery(cardsBelow, true);
    pageLen = PAGE_LENGTHS.indexOf(Number(keys.page_length)) === -1 ? 25 : Number(keys.page_length);
    keptFilters = sanitiseFilters(keys.filters);
    keptShowHidden = sanitiseSelectValue(keys.f_show_hidden, SHOW_HIDDEN_VALUES);
    keptVariations = sanitiseSelectValue(keys.f_variations, VARIATIONS_VALUES);
    keptEditedByMe = sanitiseSelectValue(keys.f_edited_by_me, EDITED_VALUES);
    applyColOrder(keys.col_order);
    // The controls are emptied first, because restoreFilters puts back what the
    // view HAS and says nothing about what it does not: the outgoing view's
    // search text would otherwise stay in the box and in the next query.
    $('#f_search').val('');
    $('#f_show_hidden, #f_variations, #f_edited_by_me').val('');
    closeVlfPanels();
    buildFilterControls();
    restoreFilters();
    syncOneColumnNote();
    drawViewButton();
    drawColumnsViewLine();
    var params = buildQuery();
    var drawnParams = Object.assign({}, params);
    if ($('#f_show_hidden').val()) drawnParams.hidden = $('#f_show_hidden').val();
    if ($('#f_variations').val()) drawnParams.variations = $('#f_variations').val();
    if ($('#f_edited_by_me').val()) drawnParams.edited = $('#f_edited_by_me').val();
    writeAddress(drawnParams);
    return params;
  }

  function switchToView(id) {
    var v = viewById(id);
    if (!v || id === currentViewId) return;
    // A filter typed a moment ago is still waiting on its timer, and it would
    // fire into the middle of this switch: it bumps the same applySeq, so the
    // rows arriving for the view being left would be the ones that draw.
    clearTimeout(applyTimer);
    var keys = v.keys;
    // The view being left, by VALUE and off the table as it stands, so a fetch
    // that fails puts the reader back where they were rather than leaving the
    // arriving view's id on the outgoing view's table - where the next header
    // click would store one view's arrangement under the other's name.
    var leavingId = currentViewId;
    var leavingKeys = currentViewState();
    // The steps belong to the view they were taken in, so they come back with
    // that view rather than being spent by a switch that never happened.
    var leavingUndo = undoSteps, leavingRedo = redoSteps;
    var params = applyViewKeys(id, keys);
    var state = {
      sort: (Array.isArray(keys.order) ? keys.order : []).filter(function (o) {
        return Array.isArray(o) && typeof o[0] === 'string';
      }).map(function (o) { return [o[0], o[1]]; }),
      page: 0, pageScroll: 0, sideScroll: 0, listScroll: 0
    };
    statusText('Reading the models...');
    // The switch's own counter, and not applySeq: a filter change bumps that
    // one, and a flag left raised by an early return here is a page that
    // silently stores nothing at all for the rest of its life. The flag comes
    // down whenever this is still the newest switch, whatever else has drawn.
    var mySwitch = ++viewSwitchSeq;
    var seq = ++applySeq;
    // A filter act held for an earlier switch was made on controls this one has
    // just replaced, so it goes with them.
    filterActHeldBySwitch = null;
    viewSwitching = true;
    fetchRows(params)
      .then(function (rows) {
        // Both come down together, and above the draw guard: the guard passing
        // is what says these rows may draw, and neither of these is about the
        // rows. A held act taken down here belongs to THIS switch, because the
        // newest switch is the only one that answers for the page and a stale
        // one settling late must not spend an act made for its successor.
        var held = null;
        if (mySwitch === viewSwitchSeq) {
          viewSwitching = false;
          held = filterActHeldBySwitch;
          filterActHeldBySwitch = null;
        }
        if (seq !== applySeq) return;
        indexRows(rows);
        // Ahead of the rebuild, because the rebuild fills the filter controls
        // from these three: put the held act in and the controls come back
        // holding what the reader chose, with the arriving view's columns around
        // it. Leave them and the rebuild puts the arriving view's stored filters
        // in the controls, and the replay below re-runs those instead.
        if (held) {
          keptFilters = held.filters;
          keptShowHidden = held.showHidden;
          keptVariations = held.variations;
          keptEditedByMe = held.editedByMe;
        }
        rebuildTableInOrder(state);
        syncOneColumnNote();
        refreshCatalogEmptiness();
        hideStatusNow();
        // The table and the controls describe one view again, so the filter the
        // reader set during the read runs now, against the view it was set in.
        if (held) applyFilters(true);
      })
      .catch(function (e) {
        // Only the newest switch answers for the page; an older one that
        // settles late puts nothing back and says nothing. What it says is not
        // behind the draw guard: a filter landing in the window moves applySeq,
        // and the state going back under the reader in silence is worse than
        // the line arriving beside a table they did not expect.
        if (mySwitch !== viewSwitchSeq) return;
        viewSwitching = false;
        filterActHeldBySwitch = null;
        var reason = e.message || e;
        // A delete takes its view out of the list before it switches, so the
        // view being left here can be one that no longer exists. Putting its id
        // back would leave the page naming a view the store has dropped, and
        // every save after it writing rows under that name.
        if (!viewById(leavingId)) {
          // The reader IS in the view the delete moved them to, so the table is
          // built over that view's columns and sort even though the rows in it
          // are the ones already read. Left alone, the table would keep the
          // deleted view's columns while colConfig described this one, and the
          // next save would read a sort off the table through the wrong column.
          rebuildTableInOrder(state);
          // The rebuild above is what makes this line exact: the columns, the
          // controls, the page size and the address are the arriving view's and
          // only the rows are the deleted view's. Take the rebuild away and the
          // line understates what the reader is looking at.
          statusErr('The models could not be read (' + reason +
            '), so you are now in "' + currentView().name +
            '", showing the rows of the view you deleted.');
          return;
        }
        applyViewKeys(leavingId, leavingKeys);
        undoSteps = leavingUndo;
        redoSteps = leavingRedo;
        drawUndoState();
        syncOneColumnNote();
        // A naming box open on the row of the view that has just stopped
        // being the current one would rename the view that came back
        // instead, so it is abandoned and the name already given stands.
        viewEditing = null;
        if (viewPanelOpen()) { renderViewPanel(); focusCurrentViewRow(); }
        statusErr('The models could not be read (' + reason +
          '), so you are still in "' + viewById(leavingId).name + '".');
      });
  }

  function announceSwitch() {
    var v = currentView();
    announce('status-announce', 'Switched to "' + v.name + '". ' +
      columnsWord(viewColumnCount(viewKeys(v))) + '.');
  }

  $('#view-list').on('click', '.view-row', function () {
    var id = $(this).attr('data-id');
    if (id === currentViewId) { closeViewPanel(true); return; }
    switchToView(id);
    closeViewPanel(true);
    announceSwitch();
  });

  // --- Naming a view --------------------------------------------------------
  function startViewEdit(kind, text, from) {
    var $item = $('#view-list li[data-id="' + currentViewId + '"]');
    if (!$item.length) return;
    viewEditing = kind;
    viewEditFrom = from || '';
    var $input = $('<input type="text" class="view-name-input">')
      .attr('aria-label', 'View name')
      .attr('maxlength', String(VIEW_NAME_MAX))
      .val(text);
    $item.empty().append($input);
    drawViewPanelMsg();
    $input.trigger('focus');
    if ($input.get(0).select) $input.get(0).select();
  }

  function finishViewEdit(kind, name) {
    viewEditing = null;
    var from = viewEditFrom;
    viewEditFrom = '';
    drawViewButton();
    drawColumnsViewLine();
    if (kind === 'new' || kind === 'duplicate') {
      closeViewPanel(false);
      // A view made from nothing draws one column, so the note above the table
      // is on and its button is where the reader goes next. A copy draws
      // whatever it was copied from, so the note is usually off and the button
      // they came from takes the focus back.
      if ($('#view-one-column').is('[hidden]')) $('#view-btn').trigger('focus');
      else $('#view-one-column-btn').trigger('focus');
      // A copy is not the one-column table the created line describes, and the
      // name it ends up with is one copyViewName may have cut in the middle, so
      // the line names both it and the view it came from. It is said INSTEAD of
      // the switch line: one press is one line, and a copy announced as a move
      // between two views the reader already had never says the view exists.
      if (kind === 'new') announce('status-announce', '"' + name + '" created. It shows only the model name.');
      else announce('status-announce', '"' + name + '" created from "' + from + '". It shows ' +
        columnsWord(viewColumnCount(viewKeys(currentView()))) + '.');
      return;
    }
    renderViewPanel();
    focusCurrentViewRow();
  }

  function refuseViewName(msg) { $('#view-panel-msg').text(msg); }

  function commitViewName(raw) {
    if (!viewEditing || viewEditing === 'delete') return;
    var kind = viewEditing;
    var name = String(raw == null ? '' : raw).trim();
    var current = currentView();
    if (!name) { refuseViewName('A view needs a name.'); return; }
    if (name.length > VIEW_NAME_MAX) {
      refuseViewName('A view name can be ' + VIEW_NAME_MAX + ' characters at most.');
      return;
    }
    // Two views the reader cannot tell apart in the list are two they cannot
    // choose between, so the comparison ignores case and the spaces around it.
    var clash = viewList.filter(function (v) {
      return v.id !== currentViewId && v.name.trim().toLowerCase() === name.toLowerCase();
    })[0];
    if (clash) {
      refuseViewName('Another view is already called "' + clash.name + '".');
      return;
    }
    // The box is settled from here on, so the blur that removing it fires says
    // nothing more.
    viewEditing = null;
    if (name === current.name) { finishViewEdit(kind, current.name); return; }
    var was = current.name;
    current.name = name;
    var set = {};
    set[viewRowKey(currentViewId, 'name')] = name;
    finishViewEdit(kind, name);
    postViewRows(set, [], null, function () {
      current.name = was;
      drawViewButton();
      drawColumnsViewLine();
      if (viewPanelOpen() && !viewEditing) renderViewPanel();
    });
  }

  $('#view-list').on('keydown', '.view-name-input', function (e) {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    commitViewName($(this).val());
  });
  $('#view-list').on('focusout', '.view-name-input', function () {
    // A blur while the panel stands is the reader moving on, and that commits.
    // The panel closing has already cleared viewEditing, so it does not.
    if (!viewEditing || viewEditing === 'delete' || !viewPanelOpen()) return;
    commitViewName($(this).val());
  });

  // --- A new view -----------------------------------------------------------
  function nextViewName() {
    var taken = {};
    viewList.forEach(function (v) { taken[v.name.trim().toLowerCase()] = true; });
    // Main is the first view, so the numbering a reader sees starts at two.
    for (var n = 2; ; n++) {
      if (!taken['view ' + n]) return 'View ' + n;
    }
  }

  $('#view-new').on('click', function () {
    if (viewEditing || viewList.length >= VIEW_CAP) return;
    var number = nextViewNumber();
    reserveViewNumber(number);
    var id = 'v' + number;
    var name = nextViewName();
    var created = todayStamp();
    // A view starts with nothing but the model name shown, no filters, the sort
    // and page size this app ships, and the shipped column order.
    var keys = {
      filters: {}, f_show_hidden: '', f_variations: '', f_edited_by_me: '',
      col_order: [],
      col_hidden: columns.map(function (c) { return c.key; }),
      col_known: columns.map(function (c) { return c.key; }),
      page_length: 25, hide_empty: true, show_marks: false, full_text: false,
      cards_below: 1642, order: []
    };
    var set = {};
    set[viewRowKey(id, 'name')] = name;
    set[viewRowKey(id, 'created')] = created;
    VIEW_TABLE_KEYS.forEach(function (key) { set[viewRowKey(id, key)] = keys[key]; });
    var $new = $('#view-new').prop('disabled', true);
    postViewRows(set, [], function () {
      $new.prop('disabled', false);
      viewList.push({ id: id, number: number, name: name, created: created, position: null, keys: keys });
      sortViewList();
      switchToView(id);
      renderViewPanel();
      startViewEdit('new', name);
    }, function () {
      $new.prop('disabled', false);
    });
  });

  // --- A copy of a view -----------------------------------------------------
  // A generated name keeps to the cap a typed one does, so the reader can rename
  // the copy to itself afterwards: the whole of "<name> copy" is cut to 40, and
  // where that name is taken the number that frees it is cut into the same 40.
  function copyViewName(from) {
    var base = String(from);
    for (var n = 1; ; n++) {
      var tail = ' copy' + (n === 1 ? '' : ' ' + n);
      var name = base.slice(0, VIEW_NAME_MAX - tail.length) + tail;
      var clash = viewList.filter(function (v) {
        return v.name.trim().toLowerCase() === name.trim().toLowerCase();
      })[0];
      if (!clash) return name;
    }
  }

  $('#view-duplicate').on('click', function () {
    if (viewEditing || viewList.length >= VIEW_CAP) return;
    var from = currentView();
    var number = nextViewNumber();
    reserveViewNumber(number);
    var id = 'v' + number;
    var name = copyViewName(from.name);
    // Today rather than the day the original was made, so a copy arrives at the
    // end of the list, where a view just made is looked for.
    var created = todayStamp();
    // Every key BY VALUE. An array or an object shared with the view it came
    // from would make the next change to one of them a change to both, and a
    // copy that moves its original is the one thing a copy must never do.
    var source = viewKeys(from);
    var keys = {};
    VIEW_TABLE_KEYS.forEach(function (key) {
      if (source[key] === undefined) return;
      keys[key] = JSON.parse(JSON.stringify(source[key]));
    });
    var set = {};
    set[viewRowKey(id, 'name')] = name;
    set[viewRowKey(id, 'created')] = created;
    VIEW_TABLE_KEYS.forEach(function (key) {
      if (keys[key] !== undefined) set[viewRowKey(id, key)] = keys[key];
    });
    var $duplicate = $('#view-duplicate').prop('disabled', true);
    postViewRows(set, [], function () {
      $duplicate.prop('disabled', false);
      viewList.push({ id: id, number: number, name: name, created: created, position: null, keys: keys });
      sortViewList();
      switchToView(id);
      renderViewPanel();
      startViewEdit('duplicate', name, from.name);
    }, function () {
      $duplicate.prop('disabled', false);
    });
  });

  $('#view-rename').on('click', function () {
    if (viewEditing) return;
    startViewEdit('rename', currentView().name);
  });

  // --- Taking a view away ---------------------------------------------------
  function startDeleteConfirm() {
    var $item = $('#view-list li[data-id="' + currentViewId + '"]');
    if (!$item.length) return;
    viewEditing = 'delete';
    var name = currentView().name;
    var $box = $('<div class="view-confirm"></div>');
    $box.append($('<p class="view-confirm-ask"></p>').text('Delete "' + name + '"?'));
    $box.append($('<p class="view-confirm-hint"></p>')
      .text('You can undo this until you reload the page.'));
    var $actions = $('<div class="view-confirm-actions"></div>');
    $actions.append($('<button type="button" class="btn-danger view-confirm-yes">Delete</button>'));
    $actions.append($('<button type="button" class="view-confirm-no">Cancel</button>'));
    $box.append($actions);
    $item.empty().append($box);
    // Cancel is the one under the hand, because a delete nobody meant is the
    // expensive answer.
    $box.find('.view-confirm-no').trigger('focus');
  }

  function cancelViewEdit() {
    viewEditing = null;
    renderViewPanel();
    focusCurrentViewRow();
  }

  $('#view-delete').on('click', function () {
    if (viewEditing || viewList.length <= 1) return;
    startDeleteConfirm();
  });
  $('#view-list').on('click', '.view-confirm-no', function () { cancelViewEdit(); });

  $('#view-list').on('click', '.view-confirm-yes', function () {
    var gone = currentView();
    if (!gone || viewList.length <= 1) return;
    var at = viewList.indexOf(gone);
    // The view above, or the one below when this is the first: the reader lands
    // beside where they were rather than at the top of the list.
    var next = viewList[at > 0 ? at - 1 : 1];
    // What the row held is kept BY VALUE, off the table as it stands, so an undo
    // puts back what was on the screen rather than what was last written.
    var rows = {};
    var state = currentViewState();
    rows[viewRowKey(gone.id, 'name')] = gone.name;
    rows[viewRowKey(gone.id, 'created')] = gone.created;
    if (gone.position !== null) rows[viewRowKey(gone.id, 'position')] = gone.position;
    VIEW_TABLE_KEYS.forEach(function (key) { rows[viewRowKey(gone.id, key)] = state[key]; });
    var $yes = $(this).prop('disabled', true);
    postViewRows({}, Object.keys(rows), function () {
      viewEditing = null;
      viewList.splice(at, 1);
      // The number stays spent for the rest of the page: handing it to a new
      // view would make this undo overwrite that one.
      reserveViewNumber(gone.number);
      viewUndos.unshift({ id: gone.id, number: gone.number, name: gone.name, created: gone.created, rows: rows });
      switchToView(next.id);
      renderViewPanel();
      $('#view-undos .view-undo-btn[data-id="' + gone.id + '"]').trigger('focus');
      announce('status-announce', '"' + gone.name + '" deleted. Undo is in the Views panel until you reload.');
    }, function () {
      $yes.prop('disabled', false);
    });
  });

  $('#view-undos').on('click', '.view-undo-btn', function () {
    var id = $(this).attr('data-id');
    var at = -1;
    viewUndos.forEach(function (u, i) { if (u.id === id) at = i; });
    if (at < 0) return;
    var back = viewUndos[at];
    // An undo puts a view BACK, so it obeys the two rules a view is made under.
    // The delete left room under the cap and left the name free; whatever the
    // reader did in between may have taken either, and a view past the cap or a
    // second view with one name is a list the reader cannot choose from. The
    // refusal comes before anything is written and the strip stays, so the way
    // back is still there once room is made or the name is freed.
    //
    // Each refusal names the view that cannot come back and the action the
    // reader has. The panel's standing line already carries the cap in its own
    // words whenever the list is full, which is the very state a capped undo is
    // refused in, so a refusal borrowing that wording would write the words
    // already on screen and read as nothing happening at all.
    if (viewList.length >= VIEW_CAP) {
      refuseViewName('"' + back.name + '" cannot come back: ' + VIEW_CAP + ' views is the most ' +
        'this keeps. Delete one, then press Undo again.');
      return;
    }
    var taken = viewList.filter(function (v) {
      return v.name.trim().toLowerCase() === back.name.trim().toLowerCase();
    })[0];
    if (taken) {
      refuseViewName('"' + back.name + '" cannot come back: another view is already ' +
        'called "' + taken.name + '". Rename that one, then press Undo again.');
      return;
    }
    var $btn = $(this).prop('disabled', true);
    postViewRows(back.rows, [], function () {
      viewUndos.splice(at, 1);
      var keys = {};
      VIEW_TABLE_KEYS.forEach(function (key) { keys[key] = back.rows[viewRowKey(back.id, key)]; });
      viewList.push({
        id: back.id, number: back.number, name: back.name, created: back.created,
        position: viewPositionOf(back.rows[viewRowKey(back.id, 'position')]), keys: keys
      });
      // It goes back where it was, because the position and created rows it came
      // in with are the ones that are written again.
      sortViewList();
      renderViewPanel();
      $('#view-list .view-row[data-id="' + back.id + '"]').trigger('focus');
    }, function () {
      $btn.prop('disabled', false);
    });
  });

  // --- Putting the views in order -------------------------------------------
  // Raised from the press until its write is answered, so a second press cannot
  // build an order from a list the first has not finished changing.
  var viewMoving = false;

  // A move writes the position of EVERY view in one request. One view's row
  // alone would leave the list half-ordered whenever any other view had none,
  // and a request that lands whole or not at all cannot leave it that way.
  function moveView(id, dir) {
    if (viewMoving) return;
    var v = viewById(id);
    if (!v) return;
    if (viewEditing) {
      refuseViewName('Finish the change you are making to a view first, then move it.');
      return;
    }
    var at = viewList.indexOf(v);
    var to = at + dir;
    if (to < 0) { refuseViewName('"' + v.name + '" is already the first view.'); return; }
    if (to >= viewList.length) { refuseViewName('"' + v.name + '" is already the last view.'); return; }
    var order = viewList.slice();
    order.splice(at, 1);
    order.splice(to, 0, v);
    var set = {};
    order.forEach(function (w, i) { set[viewRowKey(w.id, 'position')] = i + 1; });
    viewMoving = true;
    postViewRows(set, [], function () {
      viewMoving = false;
      order.forEach(function (w, i) { w.position = i + 1; });
      sortViewList();
      renderViewPanel();
      $('#view-list li[data-id="' + v.id + '"] .view-move-btn[data-dir="' + dir + '"]').trigger('focus');
      announce('status-announce', '"' + v.name + '" moved ' + (dir < 0 ? 'up' : 'down') +
        '. It is now view ' + (to + 1) + ' of ' + viewList.length + '.');
    }, function () {
      viewMoving = false;
    });
  }

  $('#view-list').on('click', '.view-move-btn', function () {
    moveView($(this).closest('li').attr('data-id'), Number($(this).attr('data-dir')));
  });

  // --- Opening and closing --------------------------------------------------
  $('#view-btn').on('click', function () {
    if (viewPanelOpen()) closeViewPanel(true); else openViewPanel();
  });
  $('#view-panel-close').on('click', function () { closeViewPanel(true); });
  $('#view-one-column-btn').on('click', function () { openColumnsDialog(); });

  // Everything in the panel that can be reached by keyboard, in the order the
  // browser reaches it. Tab off either end closes the panel rather than walking
  // the reader into the page behind it, and there is no trap at either end.
  function viewFocusStops() {
    if (!viewPanel) return [];
    return Array.prototype.filter.call(viewPanel.querySelectorAll('button, input'),
      function (el) { return !el.disabled && el.offsetParent !== null; });
  }

  $('#view-panel').on('keydown', function (e) {
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      // Escape out of a rename or a confirm abandons THAT and leaves the panel
      // standing; naming a new view or a copy is the exception, because the view
      // is already made and keeping the name it was given finishes the job.
      if (viewEditing === 'rename') { cancelViewEdit(); return; }
      if (viewEditing === 'delete') { cancelViewEdit(); return; }
      if (viewEditing === 'new' || viewEditing === 'duplicate') {
        var made = viewEditing;
        var typed = currentView().name;
        viewEditing = null;
        finishViewEdit(made, typed);
        return;
      }
      closeViewPanel(true);
      return;
    }
    if (e.key !== 'Tab') return;
    var stops = viewFocusStops();
    var at = stops.indexOf(document.activeElement);
    if (at < 0) return;
    if (e.shiftKey ? at === 0 : at === stops.length - 1) closeViewPanel(false);
  });

  $(document).on('click', function (e) {
    if (!viewPanelOpen()) return;
    if (!document.contains(e.target)) return;
    if (viewPanel.contains(e.target)) return;
    var button = document.getElementById('view-btn');
    if (button && button.contains(e.target)) return;
    closeViewPanel(false);
  });

  drawViewButton();
  drawColumnsViewLine();
  syncOneColumnNote();

  // The one write a load makes, and only when it has something to say: the
  // adoption of an arrangement that predates views into the first view, and the
  // debris no reader could ever collect. Nothing else about a load writes.
  //
  // It is issued at the very END of the load, because a save posts every value
  // it can read, and the backup controls, page size, sort and filter selects
  // only hold their stored values from here on. The load is one synchronous
  // run, so no control can be touched before it.
  function writeViewOnLoad() {
    if (settingsUnreadable) return;
    if (!adoptingView && !staleSettingKeys.length) return;
    var extra = {};
    if (adoptingView) {
      extra[viewRowKey(currentViewId, 'name')] = 'Main';
      extra[viewRowKey(currentViewId, 'created')] = todayStamp();
    }
    // A load changed nothing the reader did, so it says what it could not do in
    // its own words rather than saveSettings's. Only a failed ADOPTION stops the
    // page saving: the sweep of stale keys leaves a named view behind it either
    // way, and the next load sweeps again.
    //
    // A failure is said TWICE on purpose. The status bar carries the reason and
    // is painted over by whatever the reader does next; the bar at the top of
    // the page stays there for the rest of the load, which is what stops an
    // evening spent arranging a table nothing is storing.
    saveSettings(function (e) {
      var reason = e.message || e;
      if (adoptingView) {
        showBanner('view-not-stored');
        statusErr('Your arrangement of the table could not be stored (' + reason +
          '), so nothing on this page is being saved. Reload to try again.');
        return;
      }
      statusErr('Settings left over from an earlier version could not be tidied away (' +
        reason + '). Nothing is lost, and the next load tries again.');
    }, extra, staleSettingKeys, function () {
      viewNotStored = false;
    });
    // The flag goes up with the REQUEST and comes down on its answer, because
    // the round trip in between is a window in which the view has no name row
    // yet: a header click there would post rows this page's own reader collects
    // as debris.
    if (adoptingView) viewNotStored = true;
  }
  writeViewOnLoad();

  function backupOutcomeText(st) {
    if (!st.last_attempt_at && !st.last_backup_at) return 'Never attempted yet.';
    var when = new Date(st.last_backup_at || st.last_attempt_at).toLocaleString();
    switch (st.last_outcome) {
      case 'ok': return 'Last backup: ' + new Date(st.last_backup_at).toLocaleString() + ' - ok.';
      case 'unchanged': return 'Last attempt: ' + when + ' - nothing had changed, so nothing was written.';
      case 'invalid destination': return 'Last attempt: ' + when + ' - the destination folder is not usable (' + (st.last_error || 'invalid') + ').';
      case 'error': return 'Last attempt: ' + when + ' - failed (' + (st.last_error || 'unknown error') + ').';
      default: return 'Last attempt: ' + when + '.';
    }
  }
  function renderBackupListing(listing) {
    var el = document.getElementById('backup-listing');
    if (!listing) { el.textContent = ''; return; }
    switch (listing.state) {
      case 'none': el.textContent = 'No destination folder is configured yet.'; return;
      case 'missing': el.textContent = 'The configured folder does not currently exist.'; return;
      case 'empty': el.textContent = 'No backup files found in this folder yet.'; return;
      case 'listed':
        el.textContent = listing.files.map(function (f) {
          return f.name + '  (' + f.size + ' bytes, ' + new Date(f.modified).toLocaleString() + ')';
        }).join('\n');
        return;
      default: el.textContent = '';
    }
  }
  // The main-page bar for a daily backup that is failing. Shown only while the
  // backup is switched on and its last attempt was refused or failed, so a
  // destination nobody has looked at since choosing it cannot go on failing
  // unseen; hidden for a backup that is off, never attempted, ok or unchanged.
  // Redrawn from every status read, so a later success takes it down.
  function syncBackupFailingBar(st) {
    var failing = !!st.enabled && (st.last_outcome === 'invalid destination' || st.last_outcome === 'error');
    if (failing) {
      $('#backup-failing-text').text('Your daily backup is failing: ' +
        (st.last_outcome === 'invalid destination' ? 'the destination folder is not usable' : 'the last attempt failed') +
        ' (' + (st.last_error || 'no reason given') + '). Open Info to fix it.');
    }
    $('#backup-failing-banner').toggleClass('show', failing);
  }
  $('#backup-failing-info').on('click', function () { $('#btn-paths').trigger('click'); });
  // Read once at the end of every load. A failed read says nothing: the bar is
  // a warning, and a status the page could not fetch is not evidence of one.
  fetch('/api/backup/status').then(function (r) { return r.json(); }).then(syncBackupFailingBar).catch(function () {});

  function refreshBackupStatus() {
    document.getElementById('backup-listing').textContent = 'Loading...';
    fetch('/api/backup/status').then(function (r) { return r.json(); }).then(function (st) {
      syncBackupFailingBar(st);
      $('#backup-enabled').prop('checked', !!st.enabled);
      $('#backup-dir').val(st.dir || '');
      backupDirChecked = st.dir || '';
      $('#backup-overwrite').prop('checked', !!st.overwrite);
      $('#backup-last-status').text(backupOutcomeText(st));
      renderBackupListing(st.listing);
    }).catch(function (e) {
      document.getElementById('backup-listing').textContent = 'Failed to load: ' + (e.message || e);
    });
  }
  // The folder last sent for checking, so the change event and the blur that
  // follows it for one edit post one check.
  var backupDirChecked = null;
  function saveBackupDir(said) {
    var dir = $('#backup-dir').val();
    backupDirChecked = dir;
    var $verdict = $('#backup-dir-verdict');
    var kept = said ? 'Saved. ' : '';
    if (!dir) {
      $verdict.text(said ? 'Saved: no folder, so nothing is backed up automatically.' : '');
      saveSettings();
      return;
    }
    $verdict.text(said ? 'Saved. Checking the folder...' : 'Checking...');
    fetch('/api/backup/validate', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ dir: dir })
    }).then(function (r) { return r.json(); }).then(function (res) {
      $verdict.text(kept + (res.ok ? 'This folder is usable.' : 'Not usable yet: ' + res.error));
    }).catch(function (e) {
      $verdict.text(kept + 'Could not check: ' + (e.message || e));
    }).finally(function () { saveSettings(); });
  }
  $('#backup-dir').on('blur change', function (e) {
    if (e.type === 'blur' && $(e.relatedTarget).is('#backup-dir-save')) return;
    if ($(this).val() === backupDirChecked) return;
    saveBackupDir(false);
  });
  $('#backup-dir-save').on('click', function () { saveBackupDir(true); });
  $('#btn-backup-now').on('click', function () {
    var $btn = $(this);
    $btn.prop('disabled', true);
    $('#backup-run-status').text('Backing up...');
    fetch('/api/backup/run', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir: $('#backup-dir').val(), overwrite: $('#backup-overwrite').is(':checked') })
    }).then(function (r) { return r.json(); }).then(function (res) {
      $('#backup-run-status').text(res.ok ? ('Wrote ' + res.bytes + ' bytes to ' + res.path) : ('Failed: ' + res.error));
      refreshBackupStatus();
    }).catch(function (e) {
      $('#backup-run-status').text('Failed: ' + (e.message || e));
    }).finally(function () { $btn.prop('disabled', false); });
  });

  var $dlgTabs = $('#paths-modal .dlg-tab');
  function selectInfoTab(id, moveFocus) {
    $dlgTabs.each(function () {
      var on = this.id === id;
      $(this).toggleClass('dlg-tab-on', on)
        .attr('aria-selected', on ? 'true' : 'false')
        .attr('tabindex', on ? null : '-1');
      $('#' + $(this).attr('aria-controls')).prop('hidden', !on);
    });
    if (moveFocus) $('#' + id).trigger('focus');
    if (id === 'tab-stats') loadCatalogStats();
  }
  $dlgTabs.on('click', function () { selectInfoTab(this.id, false); });
  // The arrow keys move between the tabs and select as they go, wrapping at
  // the ends, with Home and End for the two edges - which is what lets only
  // the SELECTED tab sit in the tab order.
  $dlgTabs.on('keydown', function (e) {
    var at = $dlgTabs.index(this), to;
    if (e.key === 'ArrowLeft') to = (at - 1 + $dlgTabs.length) % $dlgTabs.length;
    else if (e.key === 'ArrowRight') to = (at + 1) % $dlgTabs.length;
    else if (e.key === 'Home') to = 0;
    else if (e.key === 'End') to = $dlgTabs.length - 1;
    else return;
    e.preventDefault();
    selectInfoTab($dlgTabs.eq(to).attr('id'), true);
  });

  // --- The catalog's numbers, on the info dialog's second tab -----------------
  // Counted afresh from the server on every select: the rows in hand are the
  // FILTERED ones, so totals read off them would lie under an active filter.
  // The one figure read off the table itself is "shown by the filters right
  // now", which is exactly the question it answers.

  function statsRow(cells) {
    var $tr = $('<tr></tr>');
    cells.forEach(function (c) {
      var $td = $('<td></td>');
      if (c && c.html !== undefined) $td.html(c.html);
      else if (c && c.num !== undefined) $td.addClass('num').text(c.num);
      else $td.text(c);
      $tr.append($td);
    });
    return $tr;
  }
  function domainCell(domain) {
    return { html: esc(DOMAIN_SECTION_NAME[domain] || domain) };
  }

  function loadCatalogStats() {
    $('#stats-status').text('Reading the catalog...');
    fetch('/api/models?include_hidden=1').then(jsonOrThrow)
      .then(function (rows) {
        $('#stats-status').text('');
        drawCatalogStats(rows || []);
      })
      .catch(function (e) {
        $('#stats-status').text('The catalog could not be read: ' + (e.message || e));
      });
  }

  function drawCatalogStats(rows) {
    var variations = 0, hiddenPlain = 0;
    rows.forEach(function (r) {
      if (r.variant_of) variations++;
      else if (r.hidden === true) hiddenPlain++;
    });
    $('#stats-models tbody').empty()
      .append(statsRow(['Models in this catalog', { num: rows.length }]))
      .append(statsRow(['of which variations', { num: variations }]))
      .append(statsRow(['of which hidden (not counting variations)', { num: hiddenPlain }]))
      .append(statsRow(['shown by the filters right now', { num: table ? table.page.info().recordsDisplay : 0 }]));

    var bySource = {};
    rows.forEach(function (r) {
      var s = r.source || '(none)';
      bySource[s] = (bySource[s] || 0) + 1;
    });
    var $sources = $('#stats-sources tbody').empty();
    Object.keys(bySource).sort(function (a, b) { return bySource[b] - bySource[a]; })
      .forEach(function (s) { $sources.append(statsRow([s, { num: bySource[s] }])); });

    // What each group knows NOW: how many of its columns exist, how many
    // models hold at least one of its values, and how many values in total.
    var colCount = {}, cellCount = {}, modelCount = {};
    DOMAIN_SECTIONS.forEach(function (d) { colCount[d] = 0; cellCount[d] = 0; modelCount[d] = 0; });
    columns.forEach(function (c) { colCount[domainOf(c)] = (colCount[domainOf(c)] || 0) + 1; });
    rows.forEach(function (r) {
      var seen = {};
      var cells = r.cells || {};
      Object.keys(cells).forEach(function (key) {
        var column = columnByKey[key];
        if (!column || String(cells[key]).trim() === '') return;
        var d = domainOf(column);
        cellCount[d]++;
        seen[d] = true;
      });
      Object.keys(seen).forEach(function (d) { modelCount[d]++; });
    });
    var $ns = $('#stats-namespaces tbody').empty();
    DOMAIN_SECTIONS.forEach(function (d) {
      $ns.append(statsRow([domainCell(d), { num: colCount[d] }, { num: modelCount[d] }, { num: cellCount[d] }]));
    });

    var shownCount = {}, yours = 0;
    columns.forEach(function (c) {
      var d = domainOf(c);
      if (!hiddenTargets[c.key]) shownCount[d] = (shownCount[d] || 0) + 1;
      if (!c.sealed) yours++;
    });
    var $cols = $('#stats-columns tbody').empty();
    DOMAIN_SECTIONS.forEach(function (d) {
      $cols.append(statsRow([domainCell(d), { num: colCount[d] }, { num: shownCount[d] || 0 }]));
    });
    $cols.append(statsRow(['Every group', { num: columns.length },
      { num: columns.filter(function (c) { return !hiddenTargets[c.key]; }).length }]).addClass('stats-total'));
    $('#stats-columns-note').text(yours === 1
      ? '1 of these columns is one you made yourself.'
      : yours + ' of these columns are ones you made yourself.');
  }

  // --- The filter drawer, and the mobile sort bar ---------------------------

  $('#btn-filters').on('click', function () {
    $('body').toggleClass('filters-drawer-open');
    syncFilterFunnel();
  });
  // One close control, two meanings: on a narrow screen it closes the overlay
  // drawer, and on a wide one it closes (and remembers) the docked panel.
  $('#filters-backdrop, #filters-close').on('click', function () {
    if (cardsQuery.matches) {
      $('body').removeClass('filters-drawer-open');
      syncFilterFunnel();
      return;
    }
    setFiltersOpen(false);
  });

  // Every column the table is drawing, and only those. It is built again after
  // ANYTHING that changes which columns are drawn or what they are called - the
  // order, a tick in the Columns dialog, a rename - because a select left alone
  // keeps offering the set it was built from, and a column somebody has just
  // shown is then the one thing they cannot sort by.
  function buildMobileSort() {
    var was = String($('#m_sort').val() || '');
    var $into = $('#m_sort').empty();
    var still = false;
    colConfig.forEach(function (c) {
      if (!c.column || hiddenTargets[c.target]) return;
      if (c.target === was) still = true;
      $into.append($('<option></option>').attr('value', c.target).text(c.column.label));
    });
    if (still) { $into.val(was); return; }
    // What it was sorted by has just left the list, so the select would fall onto
    // its first option while the table stayed ordered by something else. Where the
    // table's own sort column is still on offer it says that instead. Where it is
    // not - the column somebody just hid IS the sort column - the select keeps its
    // first option and the table keeps its order, which is what the wide layout
    // does with a hidden sort column too.
    var order = (table && table.order && table.order()) || [];
    var entry = order.length ? colConfig[order[0][0]] : null;
    if (!entry || !entry.column || hiddenTargets[entry.target]) return;
    $into.val(entry.target);
    var dir = order[0][1] === 'desc' ? 'desc' : 'asc';
    $('#m_sort_dir').attr('data-dir', dir).text(mobileSortGlyph(dir));
  }
  buildMobileSort();
  function mobileSortGlyph(dir) { return dir === 'desc' ? '\u2193' : '\u2191'; }
  $('#m_sort').on('change', function () {
    var idx = getColIdx($(this).val());
    var dir = $('#m_sort_dir').attr('data-dir') || 'asc';
    if (idx >= 0) table.order([[idx, dir]]).draw();
  });
  $('#m_sort_dir').on('click', function () {
    var dir = ($(this).attr('data-dir') || 'asc') === 'asc' ? 'desc' : 'asc';
    $(this).attr('data-dir', dir).text(mobileSortGlyph(dir));
    $('#m_sort').trigger('change');
  });

  // Ctrl/Cmd+Z undoes, Ctrl/Cmd+Shift+Z (or Ctrl+Y) redoes. Ignored while typing
  // in a field, so the shortcut never fights an input's own undo, and ignored
  // while any dialog is open: the key pressed on a button inside one still
  // reaches the document, and would otherwise write a cell value back behind
  // the dialog.
  $(document).on('keydown', function (e) {
    if (!(e.ctrlKey || e.metaKey) || e.altKey) return;
    var key = (e.key || '').toLowerCase();
    if (key !== 'z' && key !== 'y') return;
    var tag = (e.target && e.target.tagName || '').toLowerCase();
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return;
    if (anyDialogOpen()) return;
    if (key === 'y' || (key === 'z' && e.shiftKey)) { e.preventDefault(); editHistory.redo(); }
    else if (key === 'z') { e.preventDefault(); editHistory.undo(); }
  });

  // The first load (firstQuery/firstRows, read above) never goes through
  // applyFilters, the suggestion panel's only other caller, so a name that
  // matched nothing on load would otherwise show an empty table and no
  // suggestions until a filter is touched. Runs last: didYouMeanSeq (declared
  // above) must already be initialised.
  maybeShowDidYouMean(firstQuery, firstRows);
});

// --- Small pieces every part of the page uses -------------------------------

function esc(s) {
  if (s == null) return '';
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// A flag is stored as a word, and three of them mean the same thing.
function isYes(value) {
  var v = String(value == null ? '' : value).trim().toLowerCase();
  return v === 'yes' || v === 'true' || v === '1';
}

// How much of a long value a cell paints before it is cut short. A note can hold
// a paragraph and a table row is not the place to read one, so past this many
// characters the cell shows the beginning and the whole value rides in data-full
// where the editor reads it.
var CLIP_AT = 20;
// Reads the Full text tick off the body class, because it lives outside the
// page's own closure.
function clipHtml(full, label) {
  var value = String(full == null ? '' : full);
  var uncapped = document.body.classList.contains('full-text');
  var clipped = !uncapped && value.length > CLIP_AT;
  var shown = clipped ? esc(value.slice(0, CLIP_AT) + '...') : esc(value);
  return '<span class="cc-text-display" data-full="' + esc(value) + '" title="' + esc(label) + ': ' +
    esc(value) + '">' + shown + '</span>';
}

// A figure at two decimal places, as the shortest digits that say the same
// number. This is the browser's copy of the rule the server stores by, so what
// is on screen is what the database holds.
function twoDecimals(value) {
  var n = parseFloat(value);
  if (!isFinite(n)) return value == null ? '' : String(value);
  return String(Math.round(n * 100) / 100);
}

// A number as plain digits, never exponent notation: String(2.5 / 1e6) gives
// "2.5e-7", which reads back as a different-looking figure, and a price is
// stored as the text of its digits.
function fixedNumberString(n) {
  if (!isFinite(n)) return '';
  var s = n.toFixed(12);
  if (s.indexOf('.') !== -1) s = s.replace(/0+$/, '').replace(/\.$/, '');
  return s;
}

var UNIT_ABBREV = {
  'per 1M tokens': 'tkn', 'per 1M characters': 'chr', 'per image': 'img', 'per second': 'sec',
  'per minute': 'min', 'per hour': 'hr', 'per megapixel': 'mpx', 'per search': 'srch'
};
function unitAbbrev(unit) {
  var raw = String(unit || '').trim();
  if (raw === '') return '';
  return UNIT_ABBREV[raw] || raw;
}

// What colour a cell is painted, read off the COLUMN. Every column carries its
// own rule, so a column somebody invented shades exactly as the price columns
// the app ships do, and a column with no rule is painted not at all.
//
// The kind of value decides the shape of the rule: a number takes ordered
// thresholds and one colour for everything above the last of them, a yes-or-no
// takes one colour each, and a one-of-a-list takes a colour per label. Text and
// a list take none.
//
// The figure a threshold is read against is the one the cell SHOWS - the price
// per million rather than the price per token - so the numbers typed into the
// editor are the numbers read in the table.
function colorForValue(column, stored, shown) {
  var rules = column && column.color_rules;
  if (!rules) return '';
  switch (column.value_type) {
    case 'number': {
      var n = parseFloat(shown);
      if (!isFinite(n)) return '';
      var stops = rules.thresholds || [];
      for (var i = 0; i < stops.length; i++) {
        if (n < stops[i].below) return safeColor(stops[i].color);
      }
      return safeColor(rules.otherwise);
    }
    case 'flag':
      return safeColor(isYes(stored) ? rules.yes : rules.no);
    case 'choice':
      return safeColor((rules.options || {})[String(stored)]);
  }
  return '';
}

// A colour goes straight into a cell's background, so only a hex code is ever
// written there. The server refuses anything else too; this is the same rule at
// the point where the value reaches the page.
var HEX_COLOR = /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/;
function safeColor(color) {
  return HEX_COLOR.test(String(color || '')) ? String(color) : '';
}

// A value in its colour, or the value alone when the column has no colour for
// it. The pill's own look lives in style.css; only the background is written
// here, because a threshold colour cannot be enumerated in a stylesheet.
function painted(html, color) {
  if (!color) return html;
  return '<span class="cell-color" style="background-color:' + color + '">' + html + '</span>';
}

var OR_PATH = '<path d="M303.9475,17.19926c42.79734,0,77.48933,34.69327,77.48933,77.48933s-34.69199,77.48933-77.48933,77.48933l76.86166,76.86244c9.76367,9.76313,2.84903,26.45667-10.95697,26.45667h-220.88335c-71.32686,0-129.14889-57.82202-129.14889-129.14889S77.64197,17.19926,148.96884,17.19926h154.97866ZM148.96884,68.85881c-42.79607,0-77.48933,34.69327-77.48933,77.48933s34.69327,77.48933,77.48933,77.48933,77.48933-34.69327,77.48933-77.48933-34.69327-77.48933-77.48933-77.48933Z"/>';
var OR_ICON = '<svg class="or-ico" viewBox="0 0 401.4 293.7" aria-hidden="true" focusable="false">' + OR_PATH + '</svg>';

var COPY_ICON = '<svg class="copy-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
var CHECK_ICON = '<svg class="copy-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';
var DELETE_ICON = '<svg class="row-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7h16"/><path d="M10 11v6M14 11v6"/><path d="M6 7l1 13a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-13"/><path d="M9 7V4h6v3"/></svg>';
var SHOWING_ICON = '<svg class="row-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6-10-6-10-6z"/><circle cx="12" cy="12" r="2.6"/></svg>';
var HIDDEN_ICON = '<svg class="row-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6-10-6-10-6z"/><circle cx="12" cy="12" r="2.6"/><path d="M3.5 3.5l17 17"/></svg>';
// An arrow leaving a stack: the opposite act to VAR_ADD_ICON's arrow going into
// one, which is what a reader sees on the parent row.
var UNFILE_ICON = '<svg class="row-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 14V4"/><path d="M8 8l4-4 4 4"/><path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2"/></svg>';
var VAR_ADD_ICON = '<svg class="row-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v10"/><path d="M8 10l4 4 4-4"/><path d="M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2"/></svg>';
// The mark on the button an empty cell carries in place of a value.
var ADD_CELL_ICON = '<svg class="add-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>';
var EDIT_ICON = '<svg class="edit-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 20h4l10.5-10.5a2.1 2.1 0 0 0-3-3L5 17v3z"/><path d="M13.5 6.5l4 4"/></svg>';
// The palette the Columns dialog's Colours button carries.
var COLOR_ICON = '<svg class="cmg-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3a9 8 0 0 0 0 16c1.1 0 1.7-.6 1.7-1.5 0-.4-.2-.8-.4-1.1-.2-.3-.4-.6-.4-1 0-.7.6-1.2 1.3-1.2h1.6A4.2 4.2 0 0 0 20 10.2C20 6.2 16.4 3 12 3z"/><circle cx="7.5" cy="10.5" r="1.1" fill="currentColor" stroke="none"/><circle cx="10.5" cy="7" r="1.1" fill="currentColor" stroke="none"/><circle cx="15" cy="7.5" r="1.1" fill="currentColor" stroke="none"/></svg>';
// The tree connector a variation row's Name cell opens with: a line coming
// down from the parent row above, turning right into an arrowhead that points
// at this row's name. It carries no aria text of its own - the wrapping
// span's title does that.
var VAR_ARROW_ICON = '<svg class="var-arrow-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3v11a3 3 0 0 0 3 3h11"/><path d="M16 13l4 4-4 4"/></svg>';

// The toast, and the two hidden regions that say the same line out loud: polite
// for progress and success, assertive for a failure, which has to interrupt.
function announce(regionID, msg) {
  var region = document.getElementById(regionID);
  if (!region) return;
  if (region.textContent === msg) {
    region.textContent = '';
    setTimeout(function () { region.textContent = msg; }, 60);
    return;
  }
  region.textContent = msg;
}

// #filter-feedback: what a filter change just did to the row count, in one
// sentence. "still" replaces "(was {old})" whenever the count did not move -
// repeating the same number twice would read as a glitch, not confirmation -
// and it is the one wording every kind below shares, "clear all" included,
// since there is no single control to name in either case.
function filterFeedbackText(kind, label, oldCount, newCount, total) {
  var unchanged = oldCount === newCount;
  if (kind === 'clear-all') {
    return unchanged
      ? 'All filters cleared: still ' + newCount + ' of ' + total + ' models.'
      : 'All filters cleared: ' + newCount + ' of ' + total + ' models (was ' + oldCount + ').';
  }
  if (unchanged) return 'Filtered by ' + label + ': still ' + newCount + ' of ' + total + ' models.';
  if (kind === 'cleared') return label + ' cleared: ' + newCount + ' of ' + total + ' models (was ' + oldCount + ').';
  return 'Filtered by ' + label + ': ' + newCount + ' of ' + total + ' models (was ' + oldCount + ').';
}
var FILTER_FEEDBACK_MS = 3200;
var filterFeedbackTimer = null;
// Replaces, never stacks: a change while the pill is still up rewrites its
// text and restarts the timer.
function showFilterFeedback(text) {
  clearTimeout(filterFeedbackTimer);
  var el = document.getElementById('filter-feedback');
  if (el) { el.textContent = text; el.classList.add('visible'); }
  announce('status-announce', text);
  filterFeedbackTimer = setTimeout(function () {
    if (el) el.classList.remove('visible');
  }, FILTER_FEEDBACK_MS);
}

// The first-load skeleton's line of words (index.html #load-skeleton). Each
// line also goes to the polite screen-reader region; the visible line itself
// is not live, so nothing is said twice.
function loadProgress(msg) {
  var line = document.getElementById('load-progress');
  if (line) line.textContent = msg;
  announce('status-announce', msg);
}
// A failed first request replaces the skeleton with the failure text, so the
// page never shimmers over a load that has stopped.
function loadFailed(msg) {
  var line = document.getElementById('load-progress');
  if (line) { line.textContent = msg; line.className = 'load-failed'; }
  var skel = document.querySelector('#load-skeleton .skel-table');
  if (skel) skel.remove();
  statusErr(msg);
}
function statusText(msg) {
  var st = document.getElementById('status');
  st.className = 'visible'; st.style.color = '#3b82f6'; st.textContent = msg;
  announce('status-announce', msg);
}
function statusOk(msg) {
  var st = document.getElementById('status');
  st.className = 'visible'; st.style.color = '#16a34a'; st.textContent = msg;
  announce('status-announce', msg);
}
function statusErr(msg) {
  var st = document.getElementById('status');
  st.className = 'visible err'; st.style.color = '#dc2626'; st.textContent = msg;
  announce('status-alert', msg);
}
// Amber, and left on screen rather than auto-clearing into a reload: a
// warning names something to look at (a feed field appeared or vanished),
// not a failure, so it gets its own colour instead of borrowing err's red.
function statusWarn(msg) {
  var st = document.getElementById('status');
  st.className = 'visible'; st.style.color = '#b45309'; st.textContent = msg;
  announce('status-announce', msg);
}
function hideStatus() {
  var st = document.getElementById('status');
  if (st && st.textContent === 'Saved!') st.className = '';
}
// A read that has finished takes down its own progress line and nothing else. A
// refusal painted while it was in flight answers a different act of the
// reader's, so it stands in the bar until the next act paints over it.
function hideStatusNow() {
  var st = document.getElementById('status');
  if (st && !st.classList.contains('err')) st.className = '';
}

function dbStatus(status) {
  var el = document.getElementById('db-status');
  if (!el) return;
  el.className = '';
  if (status === 'online') { el.classList.add('online'); el.title = 'Connected to database'; }
  else if (status === 'offline') { el.classList.add('offline'); el.title = 'Database connection lost'; }
  else if (status === 'syncing') { el.classList.add('syncing'); el.title = 'Syncing with database...'; }
}
