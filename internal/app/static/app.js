$(async function() {
  // Every column that earns its own width. The HuggingFace link, the OpenRouter
  // link, the personal note and the favourite star ride inside the Name cell; the
  // pricing unit and its note badge ride inside the In $/Out $ cells; the ZDR state
  // is a capability icon on the Name cell. Date keeps its own column so it stays
  // sortable, and also renders inside the Name cell for the card layout. The
  // user-defined personal columns (text, or a single-select dropdown of text or
  // number labels - the generic replacement for the old fixed Speed/Rating/OCR
  // fields) are appended below, once their definitions are fetched.
  let colConfig = [
    { title: 'Name', target: 'name' },
    { title: 'Date', target: 'created_at' },
    { title: 'Cont', target: 'context_length', titleHtml: '<th title="Context size in thousands">Cont</th>' },
    { title: 'Input', target: 'input_modalities' },
    { title: 'Output', target: 'output_modalities' },
    { title: 'In $', target: 'prompt', titleHtml: '<th title="Input price, with its pricing unit abbreviated beneath it. A note icon marks a pricing note - hover to read, click to edit.">In $</th>' },
    { title: 'Out $', target: 'completion', titleHtml: '<th title="Output price, with its pricing unit abbreviated beneath it. A note icon marks a pricing note - hover to read, click to edit.">Out $</th>' },
    { title: 'P', target: 'parameters', titleHtml: '<th title="Parameters (in billions)">P</th>' },
    { title: 'AP', target: 'active_parameters', titleHtml: '<th title="Active parameters (in billions)">AP</th>' },
    { title: 'Size (GB)', target: 'disk_size_gb', titleHtml: '<th title="On-disk size in GB (rounded up) of the native-precision weights on the canonical HuggingFace repo (excludes quantized/GGUF mirrors)">Size (GB)</th>' }
  ];

  let rawData = [];
  try {
    const r = await fetch('/api/models?all=1');
    if (!r.ok) throw new Error(await r.text());
    rawData = await r.json();
  } catch (e) {
    console.error("Models fetch error:", e);
    statusErr("Failed to load models: " + (e.message || e));
    return;
  }

  // Saved UI state (filters, colours, sort order, column order). Fetched ONCE
  // here, before the table is built, so a persisted column order can be applied
  // up front. loadFilters() reuses this same object instead of re-fetching.
  let savedSettings = {};
  try {
    const sr = await fetch('/api/settings');
    savedSettings = (await sr.json()) || {};
  } catch (e) { console.error('Failed to load settings', e); savedSettings = {}; }

  // User-defined personal columns: fetched once here, alongside settings, so
  // their definitions can extend colConfig BEFORE the table is built - the same
  // reason settings are fetched up front. Each becomes one colConfig entry with
  // target 'cc_<id>' (stable across a rename, since there is none - it is tied
  // to the column's id) and a `custom` back-reference to its definition (type +
  // options), which renderCell/the edit handlers use to render and sort it
  // generically instead of switching on a hardcoded field name.
  let customColumns = [];
  try {
    const ccr = await fetch('/api/custom-columns');
    customColumns = (await ccr.json()) || [];
  } catch (e) { console.error('Failed to load custom columns', e); customColumns = []; }
  let customColumnsById = {};
  customColumns.forEach(function(c) {
    customColumnsById[c.id] = c;
    colConfig.push({ title: c.name, target: 'cc_' + c.id, custom: c });
  });
  // The mobile sort field select is a fixed set of common columns in index.html;
  // extend it with one <option> per custom column so it stays reachable from the
  // mobile sort bar too (matching how #f_year/#f_measurement below build their
  // option lists from live data).
  customColumns.forEach(function(c) {
    $('#m_sort').append($('<option></option>').attr('value', 'cc_' + c.id).text(c.name));
  });

  // One filter control per custom column, generated into #cc-filters (a
  // display:contents wrapper in the filter bar, right after the Unit filter) -
  // there is no fixed set to hardcode, since columns are created/deleted at
  // runtime. A dropdown-type column (dropdown_text/dropdown_number) gets a
  // <select> of "All" plus its own option values, matching the UX of the old
  // fixed f_speed/f_ocr single-selects this feature replaced. A text-type
  // column gets a plain text <input> that substring-matches, matching #f_search.
  // Every id is 'f_custom_' + the column's id, read by the central search
  // predicate below and by saveFilters/loadFilters. Built once here, before the
  // 'change keyup' delegation below binds - a fresh page load (the same
  // rebuild-on-load the rest of custom-column management already uses for
  // create/delete) is what keeps this set in sync with live columns.
  function buildCustomColumnFilters() {
    var $c = $('#cc-filters').empty();
    customColumns.forEach(function(col) {
      if (col.type === 'text') {
        $c.append(
          $('<input type="text" class="filter-input">')
            .attr('id', 'f_custom_' + col.id)
            .attr('placeholder', col.name + '...')
        );
      } else {
        var $sel = $('<select class="filter-select-sm"></select>').attr('id', 'f_custom_' + col.id);
        $sel.append($('<option value=""></option>').text('All'));
        (col.options || []).forEach(function(o) {
          $sel.append($('<option></option>').attr('value', o).text(o));
        });
        $c.append($('<label class="filters-label"></label>').text(col.name).append($sel));
      }
    });
  }
  buildCustomColumnFilters();

  // Reorder colConfig in place to match a saved column order (an array of
  // target names). Unknown/missing targets are ignored; any column not named in
  // the saved order is appended in its original position, so a newly added
  // column still appears. Because all rendering and filtering look columns up by
  // target via getColIdx(), reordering colConfig keeps the DOM and every cell
  // read consistent - nothing else needs to change.
  function applyColOrder(order) {
    if (!Array.isArray(order) || !order.length) return;
    const byTarget = {};
    colConfig.forEach(function(c) { byTarget[c.target] = c; });
    const seen = {};
    const reordered = [];
    order.forEach(function(t) {
      if (byTarget[t] && !seen[t]) { reordered.push(byTarget[t]); seen[t] = true; }
    });
    colConfig.forEach(function(c) { if (!seen[c.target]) reordered.push(c); });
    colConfig = reordered;
  }
  applyColOrder(savedSettings.col_order);

  // Build thead
  let theadHtml = '<thead><tr>';
  for (let c of colConfig) {
    theadHtml += c.titleHtml ? c.titleHtml : `<th>${esc(c.title)}</th>`;
  }
  theadHtml += '</tr></thead>';
  $('#models').append(theadHtml);

  // Add tbody
  $('#models').append('<tbody></tbody>');
  
  let tbodyHtml = '';
  for (let row of rawData) {
    let cells = '';
    for (let c of colConfig) {
      cells += renderCell(row, c);
    }
    // Row-level model facts the filters read. data-meas carries the RAW pricing
    // unit ("per 1M tokens"); the In $/Out $ cells render only its abbreviation, and
    // a row attribute keeps the unit readable even when those columns are hidden.
    tbodyHtml += `<tr data-name="${esc(row.name)}" data-reasoning="${row.supports_reasoning?'1':'0'}" data-tool="${row.tool==='yes'?'1':'0'}" data-moe="${row.moe==='yes'?'1':'0'}" data-zdr="${row.zdr===true?'1':'0'}" data-meas="${esc(row.measurement || '')}">${cells}</tr>`;
  }
  $('#models tbody').html(tbodyHtml);

  // Set DB status to online after successful load
  dbStatus('online');

  // Periodic health check every 30 seconds. Skip while a full update is
  // running so it does not clobber the syncing/processing icon.
  setInterval(async () => {
    if (updateRunning) return;
    try {
      const r = await fetch('/api/health');
      if (r.ok) {
        dbStatus('online');
      } else {
        dbStatus('offline');
      }
    } catch (e) {
      dbStatus('offline');
    }
  }, 30000);

  let dtCols = [];
  // Columns that need numeric sorting via data-dt-order. Dropdown-type custom
  // columns sort by their option position (generalizing the old fixed
  // Speed/OCR/Rating ORDER maps - see renderCell), so their targets are added
  // below; text-type custom columns sort alphabetically instead and are
  // deliberately left out of this list.
  let numericCols = ['context_length', 'prompt', 'completion', 'parameters', 'active_parameters', 'disk_size_gb'];
  customColumns.forEach(function(c) {
    if (c.type !== 'text') numericCols.push('cc_' + c.id);
  });
  colConfig.forEach((c, idx) => {
    let d = { targets: [idx] };
    // Only a class here, never a width: DataTables renders columnDefs.width into a
    // <colgroup><col style="width:..."> in px. The column widths live in the
    // stylesheet as .col-name / .col-narrow, so one place governs them.
    if (c.target === 'name') d.className = 'col-name';
    // Every dropdown-type custom column gets the same narrow treatment the old
    // Speed/OCR columns had (each control sizes to its own content - see
    // .col-narrow in style.css); a text-type one is left at its normal width
    // since its values are not short fixed labels.
    if (c.custom && c.custom.type !== 'text') d.className = 'col-narrow';
    if (numericCols.includes(c.target)) {
      d.orderDataType = 'data-dt-order';
      d.type = 'num';
    } else if (c.custom && c.custom.type === 'text') {
      // Alphabetical sort: extract the same data-dt-order attribute (kept in
      // sync with the cell's text on every edit) but skip the 'num' type so
      // DataTables' automatic type detection sorts it as a string.
      d.orderDataType = 'data-dt-order';
    }
    dtCols.push(d);
  });

  // Find column indexes dynamically
  function getColIdx(target) {
    return colConfig.findIndex(c => c.target === target);
  }

  var years = new Set();
  $('#models tbody tr').each(function() {
     var t = $(this).find('td[data-col="created_at"]').text();
     if(t) { var y = t.split('-')[0]; if(y) years.add(y); }
  });
  Array.from(years).sort().reverse().forEach(function(y) {
     $('#f_year').append($('<option></option>').attr('value', y).text(y));
  });

  // --- Fixed-size summary multiselect (checkbox dropdown) ------------------
  // Input, Output and Year are multi-value filters, but their CLOSED control must
  // never grow with the number of selections (a tag list would). Each stays a
  // hidden native <select multiple> that remains the single source of truth - so
  // $('#id').val(), .val([...]).trigger('change'), the ext.search reads, and the
  // saveFilters/loadFilters persistence are all unchanged. Over it sits a
  // fixed-width button whose label summarises the state (the field label when
  // empty, the single value when one is picked, else "Multiple selected") and a
  // dropdown panel of checkboxes that drive the hidden select. Toggling a checkbox
  // writes the select's value and fires its change event, so filtering + saving run
  // exactly as they did for the old tag control.
  function buildCheckDropdown(selectId, label) {
    var $sel = $('#' + selectId);
    var $wrap = $('<div class="cdd"></div>');
    var $btn = $('<button type="button" class="cdd-btn"></button>');
    var $panel = $('<div class="cdd-panel"></div>');
    $wrap.append($btn).append($panel);
    $sel.after($wrap);

    function rebuildItems() {
      $panel.empty();
      $sel.find('option').each(function() {
        var $o = $(this);
        $panel.append('<label class="cdd-item"><input type="checkbox" value="' +
          esc($o.attr('value')) + '"' + (this.selected ? ' checked' : '') + '> ' +
          esc($o.text()) + '</label>');
      });
    }
    function syncSummary() {
      var texts = [];
      $sel.find('option').each(function() { if (this.selected) texts.push($(this).text()); });
      $btn.text(texts.length === 0 ? label : (texts.length === 1 ? texts[0] : 'Multiple selected'));
      $btn.toggleClass('cdd-active', texts.length > 0);
    }
    function syncChecks() {
      var vals = $sel.val() || [];
      $panel.find('input[type=checkbox]').each(function() {
        this.checked = vals.indexOf($(this).val()) !== -1;
      });
    }
    rebuildItems();
    syncSummary();

    $btn.on('click', function(e) {
      e.stopPropagation();
      $('.cdd').not($wrap).removeClass('open');
      $wrap.toggleClass('open');
    });
    $panel.on('click', function(e) { e.stopPropagation(); });
    $panel.on('change', 'input[type=checkbox]', function() {
      var vals = [];
      $panel.find('input[type=checkbox]:checked').each(function() { vals.push($(this).val()); });
      $sel.val(vals).trigger('change');
    });
    // Programmatic changes (loadFilters restore, Clear) re-sync the UI from the
    // select. Setting checkbox.checked here does not re-fire change, so no loop.
    $sel.on('change', function() { syncChecks(); syncSummary(); });
  }
  buildCheckDropdown('f_year', 'Year');
  buildCheckDropdown('f_in', 'Input');
  buildCheckDropdown('f_out', 'Output');
  // A press anywhere outside an open checkbox dropdown closes it.
  $(document).on('click', function() { $('.cdd').removeClass('open'); });

  // The pricing unit rides on the row as data-meas (the In $/Out $ cells show only
  // its abbreviation), so the option list is built from the raw values there.
  var measurements = new Set();
  $('#models tbody tr').each(function() {
     var v = ($(this).attr('data-meas') || '').trim();
     if(v) measurements.add(v);
  });
  Array.from(measurements).sort().forEach(function(m) {
     $('#f_measurement').append($('<option></option>').attr('value', m).text(m));
  });

  $.fn.dataTable.ext.search.push(function(settings, data, dataIndex) {
    var $tr = $(settings.aoData[dataIndex].nTr);
    
    var st = $('#f_search').val().toLowerCase().trim();
    if(st) {
      var name = $tr.find('td[data-col="name"] .name-open').text().toLowerCase();
      var notes = rowNoteText($tr).toLowerCase();
      var desc = $tr.find('.tooltiptext').text().toLowerCase();
      
      var queryGroups = st.split(',').map(q => q.trim()).filter(q => q.length > 0);
      var matches = false;
      
      for(var i = 0; i < queryGroups.length; i++) {
        var terms = queryGroups[i].split(/\s+/).filter(t => t.length > 0);
        var groupMatches = true;
        for(var j = 0; j < terms.length; j++) {
          var term = terms[j];
          if(name.indexOf(term) === -1 && notes.indexOf(term) === -1 && desc.indexOf(term) === -1) {
            groupMatches = false;
            break;
          }
        }
        if(groupMatches) { matches = true; break; }
      }
      if(!matches) return false;
    }

    var yrs = $('#f_year').val() || [];
    if(yrs.length > 0) {
      var rowYear = data[getColIdx('created_at')].substring(0, 4);
      if(yrs.indexOf(rowYear) === -1) return false;
    }

    // Both link filters test for the icon in the Name cell, where the HuggingFace
    // and OpenRouter links live. .hf-link renders only for models with a
    // HuggingFace slug; .or-link only for models that hold an OpenRouter id.
    var hf = $('#f_hf').val();
    if(hf === 'yes' && !$tr.find('td[data-col="name"] .hf-link').length) return false;
    if(hf === 'no' && $tr.find('td[data-col="name"] .hf-link').length) return false;

    var or = $('#f_or').val();
    if(or === 'yes' && !$tr.find('td[data-col="name"] .or-link').length) return false;
    if(or === 'no' && $tr.find('td[data-col="name"] .or-link').length) return false;

    // The favourite star sits at the end of the Name cell's icon row.
    var fFav = $('#f_fav').val();
    if(fFav === 'yes' && !$tr.find('td[data-col="name"] .fav-star.is-fav').length) return false;

    var ctx = parseInt($('#f_context').val(), 10);
    if(!isNaN(ctx)) {
       var rv = parseInt($tr.find('td[data-col="context_length"]').attr('data-dt-order') || '0', 10);
       if(rv < ctx * 1000) return false;
    }

    var inps = $('#f_in').val() || [];
    if(inps.length > 0) {
       var dIn = data[getColIdx('input_modalities')].toLowerCase();
       if(!inps.some(i => dIn.includes(i))) return false;
    }

    var outs = $('#f_out').val() || [];
    if(outs.length > 0) {
       var dOut = data[getColIdx('output_modalities')].toLowerCase();
       if(!outs.some(i => dOut.includes(i))) return false;
    }

    var maxIn = parseFloat($('#f_max_in').val());
    if(!isNaN(maxIn)) {
       var inVal = parseFloat($tr.find('td[data-col="prompt"]').attr('data-dt-order') || '9999999999');
       if(inVal > maxIn) return false;
    }

    var maxOut = parseFloat($('#f_max_out').val());
    if(!isNaN(maxOut)) {
       var outVal = parseFloat($tr.find('td[data-col="completion"]').attr('data-dt-order') || '9999999999');
       if(outVal > maxOut) return false;
    }

    var fThink = $('#f_thinking').val();
    if(fThink === 'yes' && $tr.attr('data-reasoning') !== '1') return false;
    if(fThink === 'no'  && $tr.attr('data-reasoning') === '1') return false;
    
    var fTool = $('#f_tool').val();
    if(fTool === 'yes' && $tr.attr('data-tool') !== '1') return false;
    if(fTool === 'no'  && $tr.attr('data-tool') === '1') return false;
    
    var fMoe = $('#f_moe').val();
    if(fMoe === 'yes' && $tr.attr('data-moe') !== '1') return false;
    if(fMoe === 'no'  && $tr.attr('data-moe') === '1') return false;
    
    // The personal note lives on the third line of the Name cell.
    var fNotes = $('#f_has_notes').val();
    if(fNotes) {
      var notesVal = rowNoteText($tr);
      if(fNotes === 'yes' && notesVal === '') return false;
      if(fNotes === 'no'  && notesVal !== '') return false;
    }

    var minParams = parseFloat($('#f_min_params').val());
    if(!isNaN(minParams)) {
      var pVal = parseFloat($tr.find('td[data-col="parameters"]').attr('data-dt-order') || '9999999999');
      if(pVal === 9999999999 || pVal < minParams) return false;
    }

    var minActive = parseFloat($('#f_min_active').val());
    if(!isNaN(minActive)) {
      var aVal = parseFloat($tr.find('td[data-col="active_parameters"]').attr('data-dt-order') || '9999999999');
      if(aVal === 9999999999 || aVal < minActive) return false;
    }

    var maxSize = parseFloat($('#f_max_size').val());
    if(!isNaN(maxSize)) {
      var $szCell = $tr.find('td[data-col="disk_size_gb"]');
      // Blank-size rows always pass (kept in results, sorted to the bottom);
      // only a row with a real size ABOVE the max is filtered out.
      if($szCell.text().trim() !== '') {
        var szVal = parseFloat($szCell.attr('data-dt-order'));
        if(!isNaN(szVal) && szVal > maxSize) return false;
      }
    }

    var fZdr = $('#f_zdr').val();
    if(fZdr === 'yes' && $tr.attr('data-zdr') !== '1') return false;
    if(fZdr === 'no'  && $tr.attr('data-zdr') === '1') return false;

    var fMeas = $('#f_measurement').val();
    if(fMeas) {
       if(fMeas !== ($tr.attr('data-meas') || '').trim()) return false;
    }

    // One predicate per custom column, generated the same way its filter
    // control was (buildCustomColumnFilters above) - AND-combined with every
    // filter above, exactly like the fixed ones. A dropdown-type column's
    // control lives in the row itself (.cc-select), read the same way queueSave
    // reads it; a text-type column reuses rowCustomTextValue, the same helper
    // queueSave and editHistory already use, so there is one definition of "this
    // row's value" for a custom column. If the column's own cell is not in the
    // DOM (its table column is hidden), there is nothing to test - like every
    // other cell-sourced filter (see DESIGN.md's Filters section), hiding a
    // column silently disables filtering on it rather than excluding every row.
    for(var cci = 0; cci < customColumns.length; cci++) {
      var cc = customColumns[cci];
      var $cf = $('#f_custom_' + cc.id);
      if(!$cf.length) continue;
      var cv = $cf.val();
      if(cc.type === 'text') {
        var cq = (cv || '').toLowerCase().trim();
        if(cq) {
          var ctext = rowCustomTextValue($tr, cc.id);
          if(ctext !== undefined && ctext.toLowerCase().indexOf(cq) === -1) return false;
        }
      } else if(cv) {
        var $ccSel = $tr.find('.cc-select[data-col-id="' + cc.id + '"]');
        if($ccSel.length && $ccSel.val() !== cv) return false;
      }
    }

    return true;
  });

  // Custom sorting: use data-dt-order attribute on td cells
  $.fn.dataTable.ext.order['data-dt-order'] = function(settings, col) {
    return this.api().column(col, {order:'index'}).nodes().map(function(td, i) {
      return $(td).attr('data-dt-order') || '';
    });
  };

  // Custom pager button sequence: previous, page 1, page 2, an ellipsis, the LAST
  // page, next - once there are more than four pages. With four or fewer pages it
  // lists every page (no ellipsis needed). DataTables 2.x invokes a pager function
  // with no arguments, so the page count is read from the live table instance,
  // looked up by selector (available from the first init-complete render onward).
  // Page tokens are zero-based indices; 'ellipsis'/'previous'/'next' are DataTables
  // tokens. If the instance is not resolvable yet, fall back to the stock numbers.
  $.fn.dataTable.ext.pager.mdbCompact = function() {
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

  var table = $('#models').DataTable({
    // Unpaged to start; applyViewPaging() below switches to 25/page in card view.
    pageLength: -1,
    lengthMenu: [[25, 50, 100, -1], [25, 50, 100, 'All']],
    language: { lengthMenu: '_MENU_ / page' },
    order: [],
    columnDefs: dtCols,
    // DataTables 2.x layout API. The info line, the pager, and the page-length
    // control all sit in the bottom row (no top row); the mobile CSS re-flows this
    // same row into a grid. bottomStart -> info (left), bottom -> paging, bottomEnd
    // -> pageLength (right).
    layout: {
      topStart: null,
      topEnd: null,
      bottomStart: 'info',
      bottom: 'paging',
      bottomEnd: 'pageLength'
    },
    pagingType: 'mdbCompact',
    autoWidth: false
  });

  // Paging follows the layout. Card view is where an unpaged catalog becomes an
  // endless scroll, so it pages 25 at a time while the table lists every row. The
  // query below is the SAME one the card-view block in style.css is keyed on, so
  // there is one breakpoint and the two can never disagree. It is set where the
  // table stops fitting: below 1300 the table cannot render without scrolling
  // sideways, so that is where cards take over.
  var cardsQuery = window.matchMedia('(max-width: 1299px)');
  function applyViewPaging() {
    var want = cardsQuery.matches ? 25 : -1;
    if (table.page.len() !== want) table.page.len(want).draw(false);
  }
  cardsQuery.addEventListener('change', applyViewPaging);
  applyViewPaging();

  // --- "N of M" model counter ----------------------------------------------
  // One counter, two DOM homes: #model-count in the desktop filter bar (after
  // Clear) and #model-count-m in the mobile sort bar (before the Sort label). CSS
  // shows whichever belongs to the current layout; both are updated together here.
  // The counts come from DataTables' own page.info(), the same source that feeds its
  // .dt-info line, so "displayed" is the filtered/searched row count and "total" is
  // every loaded row - never a separate tally that could drift from the table.
  function updateModelCount() {
    var info = table.page.info();
    var txt = info.recordsDisplay + ' of ' + info.recordsTotal;
    $('#model-count, #model-count-m').text(txt);
  }
  // draw.dt fires for every filter, search, sort, paging and column-visibility
  // change, so the counter stays in step with the table without a second trigger.
  table.on('draw.dt', updateModelCount);
  updateModelCount();

  // colOrderOverride (an array of target names) is supplied only by the column
  // reorder path, which computes the new order WITHOUT mutating the live
  // colConfig (so the sort-order targets below still map correctly). All other
  // callers pass nothing - and the order.dt event passes an Event object, hence
  // the Array.isArray guard.
  function saveFilters(colOrderOverride) {
    var state = {
      f_search: $('#f_search').val(),
      f_year: $('#f_year').val(),
      f_hf: $('#f_hf').val(),
      f_or: $('#f_or').val(),
      f_fav: $('#f_fav').val(),
      f_context: $('#f_context').val(),
      f_in: $('#f_in').val(),
      f_out: $('#f_out').val(),
      f_max_in: $('#f_max_in').val(),
      f_max_out: $('#f_max_out').val(),
      f_min_params: $('#f_min_params').val(),
      f_max_size: $('#f_max_size').val(),
      f_min_active: $('#f_min_active').val(),
      f_thinking: $('#f_thinking').val(),
      f_tool: $('#f_tool').val(),
      f_moe: $('#f_moe').val(),
      f_has_notes: $('#f_has_notes').val(),
      f_zdr: $('#f_zdr').val(),
      f_measurement: $('#f_measurement').val(),
      auto_update: $('#pref-auto-update').is(':checked'),
      color_ranges: colorRanges,
      // Column order as target names (survives renames of the visual layout) and
      // the sort order keyed by target name too, so a later column reorder cannot
      // leave the saved sort pointing at the wrong column.
      col_order: Array.isArray(colOrderOverride) ? colOrderOverride : colConfig.map(function(c) { return c.target; }),
      // Hidden columns as target names (the complement of the show/hide checkboxes).
      // Stored as the HIDDEN set so a column added in a later build defaults to
      // visible. Applied on load alongside col_order, so visibility and drag-order
      // persist independently and coexist.
      col_hidden: currentHiddenTargets(),
      order: table.order().map(function(o) { return [colConfig[o[0]].target, o[1]]; })
    };
    // One 'f_custom_<id>' key per custom column, alongside the fixed f_* keys
    // above - settings.json is an opaque blob the server just stores and
    // returns, so a dynamic key per column needs no server-side change.
    customColumns.forEach(function(c) {
      var $f = $('#f_custom_' + c.id);
      if ($f.length) state['f_custom_' + c.id] = $f.val();
    });
    fetch('/api/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(state)
    }).catch(e => console.error("Failed to save settings", e));
  }

  async function loadFilters() {
    try {
      // Reuse the settings fetched up front (the same object that drove the
      // column-order layout); no second round-trip needed.
      let s = savedSettings;
      if (!s || Object.keys(s).length === 0) return;

      if(s.f_search) $('#f_search').val(s.f_search);
      if(s.f_year && s.f_year.length)    { $('#f_year').val(s.f_year).trigger('change'); }
      if(s.f_hf)    $('#f_hf').val(s.f_hf);
      if(s.f_or)    $('#f_or').val(s.f_or);
      if(s.f_fav)   $('#f_fav').val(s.f_fav);
      if(s.f_context) $('#f_context').val(s.f_context);
    if(s.f_in  && s.f_in.length)  { $('#f_in').val(s.f_in).trigger('change'); }
    if(s.f_out && s.f_out.length) { $('#f_out').val(s.f_out).trigger('change'); }
    if(s.f_max_in)  $('#f_max_in').val(s.f_max_in);
    if(s.f_max_out) $('#f_max_out').val(s.f_max_out);
    if(s.f_min_params) $('#f_min_params').val(s.f_min_params);
    if(s.f_max_size) $('#f_max_size').val(s.f_max_size);
    if(s.f_min_active) $('#f_min_active').val(s.f_min_active);
    if(s.f_thinking) $('#f_thinking').val(s.f_thinking);
    if(s.f_tool) $('#f_tool').val(s.f_tool);
    if(s.f_moe) $('#f_moe').val(s.f_moe);
    if(s.f_has_notes) $('#f_has_notes').val(s.f_has_notes);
    if(s.f_zdr) $('#f_zdr').val(s.f_zdr);
    if(s.f_measurement) $('#f_measurement').val(s.f_measurement);
    // Restore each custom column's filter from its own 'f_custom_<id>' key. A
    // column deleted since the settings were saved simply has no matching
    // control, so its stale key is harmlessly ignored (not cleaned up here -
    // saveFilters overwrites the whole blob on the next save anyway).
    customColumns.forEach(function(c) {
      var v = s['f_custom_' + c.id];
      if (v) $('#f_custom_' + c.id).val(v);
    });
    $('#pref-auto-update').prop('checked', !!s.auto_update);
    if(s.color_ranges) {
      // Merge saved thresholds over the defaults so new measurement types still
      // get a sensible range if they were added after the settings were saved.
      colorRanges = Object.assign(JSON.parse(JSON.stringify(DEFAULT_COLOR_RANGES)), s.color_ranges);
      recolorPrices();
    }
    if(s.order && s.order.length) {
      // Saved sort order is [target, dir] pairs; map back to current column
      // indices. Pairs whose target no longer exists are dropped.
      var idxOrder = s.order
        .map(function(o) { return [getColIdx(o[0]), o[1]]; })
        .filter(function(o) { return o[0] >= 0; });
      if (idxOrder.length) table.order(idxOrder);
    }
    table.draw();
    } catch(e) { console.error('Failed to load settings', e); }
  }

  await loadFilters();

  $('.filters input, .filters select').on('change keyup', function() {
    saveFilters();
    table.draw();
  });
  // The auto-download preference lives in the info dialog, not the filter bar, so
  // it persists through the same /api/settings flow via its own change handler.
  $('#pref-auto-update').on('change', function() { saveFilters(); });
  $('#models').on('order.dt', saveFilters);

  // --- Mobile sort bar (narrow viewports) ----------------------------------
  // A field select + direction toggle that drive the SAME DataTables order as a
  // desktop header click. table.order().draw() fires the order.dt -> saveFilters
  // persistence, so mobile and desktop share one saved sort - no second data path.
  function mobileSortGlyph(dir) { return dir === 'desc' ? '\u2193' : '\u2191'; }
  function applyMobileSort() {
    var t = $('#m_sort').val();
    var dir = $('#m_sort_dir').attr('data-dir') || 'asc';
    var idx = getColIdx(t);
    if (idx >= 0) table.order([[idx, dir]]).draw();
  }
  (function initMobileSort() {
    // Seed the field + direction from the current (saved) table order so the bar
    // reflects the persisted state on load.
    var ord = table.order();
    if (ord && ord.length) {
      var idx = ord[0][0], dir = ord[0][1];
      var target = colConfig[idx] ? colConfig[idx].target : '';
      if ($('#m_sort option[value="' + target + '"]').length) $('#m_sort').val(target);
      $('#m_sort_dir').attr('data-dir', dir).text(mobileSortGlyph(dir));
    } else {
      $('#m_sort_dir').attr('data-dir', 'asc').text(mobileSortGlyph('asc'));
    }
  })();
  $('#m_sort').on('change', applyMobileSort);
  $('#m_sort_dir').on('click', function() {
    var dir = ($(this).attr('data-dir') || 'asc') === 'asc' ? 'desc' : 'asc';
    $(this).attr('data-dir', dir).text(mobileSortGlyph(dir));
    applyMobileSort();
  });

  // --- Drag-and-drop column reordering -------------------------------------
  // Each header is draggable. Dropping one onto another moves it to that slot,
  // we persist the new order (as target names) and reload so the whole table
  // rebuilds in the new layout. Because colConfig drives every cell read, a
  // reload is the simplest fully-consistent way to apply the new order; the
  // order is already saved server-side, so the rebuild is deterministic. A plain
  // header click (no drag) still sorts as before.
  function reorderColumns(fromIdx, toIdx) {
    if (fromIdx === toIdx || fromIdx < 0 || toIdx < 0) return;
    // Compute the new order as target names without mutating the live colConfig,
    // then persist and reload (the rebuild applies col_order deterministically).
    var targets = colConfig.map(function(c) { return c.target; });
    var moved = targets.splice(fromIdx, 1)[0];
    targets.splice(toIdx, 0, moved);
    saveFilters(targets);
    statusText('Reordering columns...');
    setTimeout(function() { location.reload(); }, 250);
  }

  var dragFromIdx = null;
  $('#models thead').on('dragstart', 'th', function(e) {
    dragFromIdx = $(this).index();
    e.originalEvent.dataTransfer.effectAllowed = 'move';
    // Firefox requires data to be set for the drag to start.
    e.originalEvent.dataTransfer.setData('text/plain', String(dragFromIdx));
    $(this).addClass('th-dragging');
  });
  $('#models thead').on('dragend', 'th', function() {
    $(this).removeClass('th-dragging');
    $('#models thead th').removeClass('th-drop-target');
    dragFromIdx = null;
  });
  $('#models thead').on('dragover', 'th', function(e) {
    if (dragFromIdx === null) return;
    e.preventDefault();
    e.originalEvent.dataTransfer.dropEffect = 'move';
  });
  $('#models thead').on('dragenter', 'th', function() {
    if (dragFromIdx === null || $(this).index() === dragFromIdx) return;
    $(this).addClass('th-drop-target');
  });
  $('#models thead').on('dragleave', 'th', function() {
    $(this).removeClass('th-drop-target');
  });
  $('#models thead').on('drop', 'th', function(e) {
    e.preventDefault();
    var toIdx = $(this).index();
    var fromIdx = dragFromIdx;
    dragFromIdx = null;
    if (fromIdx !== null) reorderColumns(fromIdx, toIdx);
  });
  function makeHeadersDraggable() {
    $('#models thead th').attr('draggable', 'true').addClass('th-reorderable');
  }
  makeHeadersDraggable();

  // --- Column show/hide dropdown -------------------------------------------
  // A checkbox per column drives DataTables column().visible(). Unchecking a box
  // hides that column (its cells are dropped from the DOM, so they also vanish
  // from the mobile cards - expected). The hidden set persists in settings as
  // col_hidden (target names), applied on load next to col_order, so column
  // visibility and drag-reorder are independent and both survive a reload.
  function currentHiddenTargets() {
    return colConfig
      .filter(function(c) { return !table.column(getColIdx(c.target)).visible(); })
      .map(function(c) { return c.target; });
  }
  function applyColHidden(hidden) {
    if (!Array.isArray(hidden) || !hidden.length) return;
    colConfig.forEach(function(c) {
      var vis = hidden.indexOf(c.target) === -1;
      table.column(getColIdx(c.target)).visible(vis, false);
    });
    table.columns.adjust();
  }
  function buildColTogglePanel() {
    var $p = $('#col-toggle-panel').empty();
    colConfig.forEach(function(c) {
      var vis = table.column(getColIdx(c.target)).visible();
      $p.append('<label class="col-toggle-item"><input type="checkbox" data-target="' +
        esc(c.target) + '"' + (vis ? ' checked' : '') + '> ' + esc(c.title) + '</label>');
    });
  }
  applyColHidden(savedSettings.col_hidden);
  buildColTogglePanel();
  $('#col-toggle-btn').on('click', function(e) {
    e.stopPropagation();
    buildColTogglePanel();
    $('#col-toggle').toggleClass('open');
  });
  $('#col-toggle-panel').on('click', function(e) { e.stopPropagation(); });
  $('#col-toggle-panel').on('change', 'input[type=checkbox]', function() {
    table.column(getColIdx($(this).attr('data-target'))).visible(this.checked, false);
    table.columns.adjust();
    saveFilters();
  });
  $(document).on('click', function() { $('#col-toggle').removeClass('open'); });

  // --- Filters off-canvas drawer (card view only) --------------------------
  // The Filters button and backdrop only render in card view (CSS). Toggling the
  // body class slides the SAME filter panel in/out via CSS transform - no filter
  // node is re-parented, so the filter controls and their save/load bindings stay intact.
  $('#btn-filters').on('click', function() { $('body').toggleClass('filters-drawer-open'); });
  // The backdrop and the drawer's own close (X) button both just drop the open
  // class - the same close the backdrop tap performs. The X only renders in card
  // view (CSS), so it never appears on the desktop filter bar.
  $('#filters-backdrop, #filters-close').on('click', function() { $('body').removeClass('filters-drawer-open'); });

  $('#f_clear').click(function() {
    $('.filters input[type=text], .filters input[type=number]').val('');
    $('.filters select').val('').trigger('change');
    $('#f_year, #f_in, #f_out').val([]).trigger('change');
    table.order([]).draw();
    saveFilters(); // clears the filters but KEEPS the colour ranges
  });

  const saveQueue = [];
  let isSaving = false;
  let lastSaveTime = 0;

  async function processQueue() {
      if (isSaving || saveQueue.length === 0) return;
      isSaving = true;

      const batch = saveQueue.splice(0, saveQueue.length);
      
      const MAX_RETRIES = 12; // 5 sec * 12 = 1 min
      let attempts = 0;
      let success = false;

      while (!success && attempts < MAX_RETRIES) {
          try {
              dbStatus('processing');
              statusText('Saving...');
              const startTime = Date.now();
              const r = await fetch('/api/save', {
                  method: 'POST',
                  headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify(batch)
              });
              const saveDuration = Date.now() - startTime;
              lastSaveTime = Date.now();
              
              if (r.ok) {
                  const resp = await r.json();
                  if (resp.ok) {
                      success = true;
                      dbStatus('online');
                      statusOk('Saved!');
                      setTimeout(hideStatus, 2000);
                      // We don't need to reload, local changes keep UI updated
                  } else {
                      throw new Error(resp.error || 'Unknown error');
                  }
              } else {
                  throw new Error(await r.text());
              }
          } catch(e) {
              attempts++;
              statusErr(`Save retry ${attempts}/${MAX_RETRIES}...`);
              await new Promise(r => setTimeout(r, 2000));
          }
      }

      if (!success) {
          dbStatus('offline');
          statusErr('Save failed permanently! Check console.');
          alert("CRITICAL ERROR: Failed to save changes to the server. The data API returned an enduring failure.");
          console.error("Failed batch:", batch);
      }

      isSaving = false;
      if (saveQueue.length > 0) {
          processQueue();
      }
  }

  // Collect this row's personal fields and queue them for the debounced save.
  //
  // EVERY field here is read from a control in the row, and a control is only in the
  // DOM while its column is shown. Hiding a column drops its cells, so the read
  // returns undefined - and a default (`|| ''`, `|| '0'`) would turn "I cannot see
  // this field" into "set this field to empty", which /api/save would faithfully
  // write. store.SaveCurated is a PARTIAL update: it writes only the keys the payload
  // carries and leaves every other curated field at its stored value. So a field with
  // no control to read is OMITTED, and the database keeps what it already has. Read
  // once, and include the key only if the control was really there.
  //
  // Custom columns (the generic personal-column system) follow the SAME omission
  // rule, collected into one custom_values object keyed by column id: a hidden
  // column's control is not in the DOM, so it is left out of custom_values
  // entirely rather than sent as an empty value that would clear it server-side.
  function queueSave(tr) {
      if (!tr.attr('data-name')) return;
      const updateObj = { name: tr.attr('data-name') };

      // The favourite star sits at the end of the Name cell's icon row.
      const fav = tr.find('.fav-star').attr('data-fav');
      if (fav !== undefined) updateObj.favorite = parseInt(fav, 10) || 0;

      // The personal note lives on the Name cell's third line.
      if (tr.find('.notes-display, .notes-input').length) {
          updateObj.notes = rowNoteText(tr);
      }

      // One pricing note per model, carried in data-pnote on the badge. The In $ and
      // Out $ cells each show a badge for it, and setPnoteBadges keeps them equal,
      // so reading the first is reading the note.
      const pnote = tr.find('.pnote-icon').attr('data-pnote');
      if (pnote !== undefined) updateObj.pricing_note = pnote;

      const cv = {};
      let hasCV = false;
      customColumns.forEach(function(c) {
          if (c.type === 'text') {
              const v = rowCustomTextValue(tr, c.id);
              if (v !== undefined) { cv[c.id] = v; hasCV = true; }
          } else {
              const $sel = tr.find('.cc-select[data-col-id="' + c.id + '"]');
              if ($sel.length) { cv[c.id] = $sel.val() || ''; hasCV = true; }
          }
      });
      if (hasCV) updateObj.custom_values = cv;

      // Remove older updates for same model
      for (let i = saveQueue.length - 1; i >= 0; i--) {
          if (saveQueue[i].name === name) saveQueue.splice(i, 1);
      }
      saveQueue.push(updateObj);

      clearTimeout(window.saveDebounce);
      window.saveDebounce = setTimeout(processQueue, 500);
  }

  // --- Session-only undo/redo of personal data edits -----------------------
  // A self-contained in-memory history of the six editable personal fields
  // (favorite, notes, speed, rating, ocr_quality, pricing_note). Each committed
  // edit pushes {name, field, oldValue, newValue} onto the undo stack and clears
  // the redo stack. Undo restores oldValue; redo re-applies newValue; both drive
  // the SAME cell-apply + queueSave path a manual re-edit uses, so the DB always
  // matches the visible cell. The stacks live only in this page - a reload or
  // restart clears them (there is no persistence). Kept as one closed module so a
  // later UI rewrite can reuse it unchanged; it touches the outside world only
  // through queueSave, the cell DOM, and the two toolbar buttons.
  var editHistory = (function() {
    var undoStack = [];
    var redoStack = [];
    var applying = false; // re-entrancy guard: a restore must never record itself

    // Model names are free text, so an attribute-selector could break; match by
    // reading data-name on each row instead.
    function rowByName(name) {
      return $('#models tbody tr').filter(function() {
        return $(this).attr('data-name') === name;
      }).first();
    }

    // Set one field's cell to an exact value, then persist. For the select
    // fields this reuses the field's own change handler (via trigger) so the
    // data-dt-order bookkeeping and queueSave run exactly as in a manual edit;
    // the record() call inside that handler is suppressed by the applying guard.
    function applyField($tr, field, value) {
      if (!$tr || !$tr.length) return;
      if (field === 'favorite') {
        var fav = parseInt(value, 10) || 0;
        setFavStar($tr.find('.fav-star'), fav);
        queueSave($tr);
      } else if (field === 'notes') {
        var $td = $tr.find('td[data-col="name"]');
        var nv = value || '';
        var disp = nv ? esc(nv) : '<em class="notes-placeholder">' + NOTES_PLACEHOLDER + '</em>';
        // The note may be mid-edit (a textarea is open); swap that back to a
        // display span, otherwise just rewrite the existing span.
        var $inp = $td.find('.notes-input');
        if ($inp.length) {
          $inp.replaceWith($('<span class="notes-display"></span>').html(disp));
        } else {
          $td.find('.notes-display').html(disp);
        }
        queueSave($tr);
      } else if (field === 'pricing_note') {
        setPnoteBadges($tr, value);
        queueSave($tr);
      } else if (field.indexOf('custom:') === 0) {
        // A user-defined personal column, identified by "custom:<column id>" (the
        // generic replacement for the old hardcoded 'speed'/'rating'/'ocr_quality'
        // field names). A dropdown restores through its own change handler, same
        // as a manual re-select; a text column rewrites its display span directly,
        // mirroring the 'notes' branch above.
        var colId = field.slice(7);
        var col = customColumnsById[colId];
        if (!col) return;
        if (col.type === 'text') {
          var $ccTd = $tr.find('td[data-col="cc_' + colId + '"]');
          var ccVal = value || '';
          var ccDisp = ccVal ? esc(ccVal) : '<em class="notes-placeholder">' + CC_TEXT_PLACEHOLDER + '</em>';
          var $ccInp = $ccTd.find('.cc-text-input');
          if ($ccInp.length) {
            $ccInp.replaceWith($('<span class="cc-text-display" data-col-id="' + colId + '"></span>').html(ccDisp));
          } else {
            $ccTd.find('.cc-text-display').html(ccDisp);
          }
          $ccTd.attr('data-dt-order', ccVal);
          queueSave($tr);
        } else {
          $tr.find('.cc-select[data-col-id="' + colId + '"]').val(value || '').trigger('change');
        }
      }
    }

    function updateButtons() {
      $('#btn-undo').prop('disabled', undoStack.length === 0);
      $('#btn-redo').prop('disabled', redoStack.length === 0);
    }

    function record(name, field, oldValue, newValue) {
      if (applying) return;                 // a restore is in progress
      if (oldValue === newValue) return;    // no real change to remember
      undoStack.push({ name: name, field: field, oldValue: oldValue, newValue: newValue });
      redoStack.length = 0;                 // a fresh edit invalidates the redo trail
      updateButtons();
    }

    function undo() {
      if (!undoStack.length) return;
      var rec = undoStack.pop();
      applying = true;
      try { applyField(rowByName(rec.name), rec.field, rec.oldValue); }
      finally { applying = false; }
      redoStack.push(rec);
      updateButtons();
    }

    function redo() {
      if (!redoStack.length) return;
      var rec = redoStack.pop();
      applying = true;
      try { applyField(rowByName(rec.name), rec.field, rec.newValue); }
      finally { applying = false; }
      undoStack.push(rec);
      updateButtons();
    }

    return { record: record, undo: undo, redo: redo, refresh: updateButtons };
  })();

  $('#btn-undo').on('click', function() { editHistory.undo(); });
  $('#btn-redo').on('click', function() { editHistory.redo(); });
  // Ctrl/Cmd+Z undoes, Ctrl/Cmd+Shift+Z (or Ctrl+Y) redoes. Ignored while typing
  // in a field so the shortcut never fights a text input's own undo.
  $(document).on('keydown', function(e) {
    if (!(e.ctrlKey || e.metaKey) || e.altKey) return;
    var key = (e.key || '').toLowerCase();
    if (key !== 'z' && key !== 'y') return;
    var tag = (e.target && e.target.tagName || '').toLowerCase();
    if (tag === 'input' || tag === 'textarea' || tag === 'select' || (e.target && e.target.isContentEditable)) return;
    if (key === 'y' || (key === 'z' && e.shiftKey)) { e.preventDefault(); editHistory.redo(); }
    else if (key === 'z') { e.preventDefault(); editHistory.undo(); }
  });
  editHistory.refresh();

  function dbStatus(status) {
      const el = document.getElementById('db-status');
      if (!el) return;
      el.className = '';
      if (status === 'online') {
        el.classList.add('online');
        el.title = 'Connected to database';
      } else if (status === 'offline') {
        el.classList.add('offline');
        el.title = 'Database connection lost';
      } else if (status === 'syncing') {
        el.classList.add('syncing');
        el.title = 'Syncing with database...';
      } else if (status === 'processing') {
        el.classList.add('processing');
        el.title = 'Saving changes...';
      }
  }

  function statusText(msg) {
      let st = document.getElementById('status');
      st.className = 'visible'; st.style.color = '#3b82f6'; st.textContent = msg;
  }
  function statusOk(msg) {
      let st = document.getElementById('status');
      st.className = 'visible'; st.style.color = '#16a34a'; st.textContent = msg;
  }
  function statusErr(msg) {
      let st = document.getElementById('status');
      st.className = 'visible err'; st.style.color = '#dc2626'; st.textContent = msg;
  }
  function hideStatus() {
      if(document.getElementById('status').textContent === 'Saved!') {
        document.getElementById('status').className = '';
      }
  }

  // --- Full DB update (POST /api/update + poll /api/update/status) ---------
  // updateRunning lets the periodic health check leave the status icon alone
  // while a refresh drives it (so it does not flip back to 'online' mid-run).
  let updateRunning = false;
  let updatePollTimer = null;
  const PHASE_LABELS = {
    catalog: 'Catalog',
    collections: 'Collections',
    pricing: 'Pricing'
  };

  function setUpdateBtnDisabled(disabled) {
    const btn = document.getElementById('btn-update');
    if (btn) {
      btn.disabled = disabled;
      btn.textContent = disabled ? 'Updating models...' : 'Update models';
    }
  }

  // Reflect a status snapshot in the icon + status text.
  function showUpdateProgress(st) {
    const label = PHASE_LABELS[st.phase] || (st.phase || '');
    // 'processing' = pricing (slow scrape), 'syncing' for the lighter phases.
    dbStatus(st.phase === 'pricing' ? 'processing' : 'syncing');
    statusText('Updating: ' + label + ' ...');
  }

  function pollUpdateStatus() {
    fetch('/api/update/status')
      .then(r => r.json())
      .then(st => {
        if (st.running) {
          showUpdateProgress(st);
          updatePollTimer = setTimeout(pollUpdateStatus, 2500);
          return;
        }
        // Finished.
        updateRunning = false;
        updatePollTimer = null;
        setUpdateBtnDisabled(false);
        dbStatus('online');
        if (st.errors && st.errors.length) {
          statusErr('Update finished with errors: ' + st.errors.join('; '));
        } else {
          statusOk('Update complete - reloading...');
          // Reload so the table (and persisted filters) pick up fresh data.
          setTimeout(() => location.reload(), 800);
        }
      })
      .catch(e => {
        console.error('Update status poll failed:', e);
        updatePollTimer = setTimeout(pollUpdateStatus, 2500);
      });
  }

  // Begin tracking an in-progress run: disable the button and start polling.
  function beginUpdateTracking() {
    updateRunning = true;
    setUpdateBtnDisabled(true);
    dbStatus('syncing');
    statusText('Updating: starting ...');
    if (!updatePollTimer) pollUpdateStatus();
  }

  $('#btn-update').on('click', function() {
    if (updateRunning) return;
    setUpdateBtnDisabled(true);
    fetch('/api/update', { method: 'POST' })
      .then(r => r.json().then(body => ({ status: r.status, body })))
      .then(({ status, body }) => {
        if (status === 202 && body.started) {
          beginUpdateTracking();
        } else if (status === 409 && body.running) {
          // Already running (e.g. another tab started it): track it.
          beginUpdateTracking();
        } else {
          setUpdateBtnDisabled(false);
          statusErr('Update could not be started.');
        }
      })
      .catch(e => {
        console.error('Update start failed:', e);
        setUpdateBtnDisabled(false);
        statusErr('Update request failed: ' + (e.message || e));
      });
  });

  // On page load, if a refresh is already in progress (e.g. a mid-update
  // refresh of the browser), reflect it and start polling.
  fetch('/api/update/status')
    .then(r => r.json())
    .then(st => { if (st.running) beginUpdateTracking(); })
    .catch(() => {});

  // --- Update banner: version label, notify, and in-app self-update --------
  // The banner shows when a newer release exists. On platforms the release
  // publishes (linux/amd64, windows/amd64) it offers an in-app "Update now" that
  // downloads + verifies + swaps the binary, then a "Restart to apply". On any
  // other platform (or if the server has no self-update) it falls back to the
  // plain download link (notify-only).
  function addDownloadLink(t, info) {
    var a = document.createElement('a');
    a.textContent = 'Download the new release';
    a.href = info.url || '#';
    a.target = '_blank';
    a.rel = 'noopener';
    t.appendChild(a);
  }

  // Render the self-update controls from a status snapshot.
  function renderAppUpdate(st) {
    var nowBtn = document.getElementById('app-update-now');
    var restartBtn = document.getElementById('app-update-restart');
    var prog = document.getElementById('app-update-progress');
    if (!nowBtn || !restartBtn || !prog) return;
    var state = (st && st.state) || 'idle';
    nowBtn.style.display = 'none';
    restartBtn.style.display = 'none';
    prog.textContent = '';
    if (state === 'idle') {
      nowBtn.disabled = false;
      nowBtn.style.display = '';
    } else if (state === 'failed') {
      nowBtn.disabled = false;
      nowBtn.style.display = '';
      prog.textContent = st && st.error ? 'Update failed: ' + st.error : 'Update failed.';
    } else if (state === 'downloading') {
      prog.textContent = 'Downloading update...';
    } else if (state === 'verifying') {
      prog.textContent = 'Verifying update...';
    } else if (state === 'staged') {
      restartBtn.disabled = false;
      restartBtn.style.display = '';
      prog.textContent = 'Update staged - restart to apply.';
    }
  }

  // Poll the staging status until it reaches a terminal state (staged/failed).
  function pollAppUpdateUntilDone() {
    fetch('/api/app-update/status')
      .then(function(r) { return r.json(); })
      .then(function(st) {
        renderAppUpdate(st);
        if (st.state === 'downloading' || st.state === 'verifying') {
          setTimeout(pollAppUpdateUntilDone, 2000);
        }
      })
      .catch(function() { setTimeout(pollAppUpdateUntilDone, 2000); });
  }

  // After a restart request, wait for the old server to drop and the new one to
  // answer /api/health, then reload so the new UI/version is picked up. Bounded:
  // after ~90s of no answer it stops polling and shows a manual-reload action, so
  // a process that never comes back can't leave a health check looping forever.
  var RECONNECT_MAX_TRIES = 128; // ~90s at 700ms per attempt
  function reconnectFailed(prog) {
    if (!prog) return;
    prog.textContent = 'Update restart failed to reconnect. ';
    var a = document.createElement('a');
    a.textContent = 'Reload manually';
    a.href = '#';
    a.addEventListener('click', function(e) { e.preventDefault(); location.reload(); });
    prog.appendChild(a);
  }
  function reconnectAfterRestart() {
    var prog = document.getElementById('app-update-progress');
    if (prog) prog.textContent = 'Restarting...';
    var sawDown = false;
    var tries = 0;
    (function ping() {
      tries++;
      if (tries > RECONNECT_MAX_TRIES) { reconnectFailed(prog); return; }
      fetch('/api/health', { cache: 'no-store' })
        .then(function(r) {
          if (!r.ok) throw new Error('not ok');
          if (sawDown) {
            if (prog) prog.textContent = 'Reconnected - reloading...';
            setTimeout(function() { location.reload(); }, 400);
            return;
          }
          setTimeout(ping, 700); // still the old process; wait for it to drop
        })
        .catch(function() {
          sawDown = true;
          if (prog) prog.textContent = 'Reconnecting...';
          setTimeout(ping, 700);
        });
    })();
  }

  function showUpdateBanner(info) {
    var b = document.getElementById('update-banner');
    var t = document.getElementById('update-banner-text');
    if (!b || !t) return;
    t.textContent = 'ModelsDB v' + info.latest + ' is available (you have v' + (info.current || '?') + '). ';
    b.classList.add('show');
    fetch('/api/app-update/status')
      .then(function(r) { return r.json(); })
      .then(function(st) {
        // Offer the in-app update only when it can actually succeed (supported
        // platform + real signing key + non-dev build). Otherwise fall back to the
        // plain download link so we never present a button that only fails closed.
        if (st && st.updatable) {
          renderAppUpdate(st);
          if (st.state === 'downloading' || st.state === 'verifying') pollAppUpdateUntilDone();
        } else {
          addDownloadLink(t, info);
        }
      })
      .catch(function() { addDownloadLink(t, info); });
  }

  (function() {
    var nowBtn = document.getElementById('app-update-now');
    if (nowBtn) nowBtn.addEventListener('click', function() {
      nowBtn.disabled = true;
      var prog = document.getElementById('app-update-progress');
      if (prog) prog.textContent = 'Starting update...';
      fetch('/api/app-update/apply', { method: 'POST' })
        .then(function(r) { return r.json().then(function(body) { return { status: r.status, body: body }; }); })
        .then(function(res) {
          if (res.status === 202) {
            renderAppUpdate({ state: 'downloading', supported: true });
            pollAppUpdateUntilDone();
          } else {
            nowBtn.disabled = false;
            if (prog) prog.textContent = 'Could not start update: ' + (res.body.error || res.status);
          }
        })
        .catch(function(e) {
          nowBtn.disabled = false;
          if (prog) prog.textContent = 'Update request failed: ' + (e.message || e);
        });
    });

    var restartBtn = document.getElementById('app-update-restart');
    if (restartBtn) restartBtn.addEventListener('click', function() {
      restartBtn.disabled = true;
      fetch('/api/app-update/restart', { method: 'POST' })
        .then(function() { reconnectAfterRestart(); })
        .catch(function() { reconnectAfterRestart(); });
    });
  })();

  fetch('/api/update-check')
    .then(function(r) { return r.json(); })
    .then(function(info) {
      var v = document.getElementById('app-version');
      if (v && info && info.current) {
        // "dev" is the only version with no number to show (a plain `go run` with no
        // ldflags). Everything else is shown verbatim, including a "-dev" suffix:
        // with a dev and a production server often open side by side, the label has
        // to say both which version this is and whether it is a release.
        v.textContent = info.current === 'dev' ? 'dev' : 'v' + info.current;
        v.title = info.current === 'dev' ? 'Development build (no version injected)' : 'Version ' + info.current;
      }
      if (info && info.available && info.latest) showUpdateBanner(info);
    })
    .catch(function() {});
  (function() {
    var ud = document.getElementById('update-dismiss');
    if (ud) ud.addEventListener('click', function() {
      var b = document.getElementById('update-banner');
      if (b) b.classList.remove('show');
    });
  })();

  // "Check for updates" button in the Info modal: force an immediate check
  // (?force=1 bypasses the once-a-day throttle) and report the result inline.
  (function() {
    var btn = document.getElementById('btn-check-update');
    var status = document.getElementById('check-update-status');
    if (!btn) return;
    btn.addEventListener('click', function() {
      btn.disabled = true;
      status.textContent = 'Checking...';
      fetch('/api/update-check?force=1')
        .then(function(r) { return r.json(); })
        .then(function(info) {
          btn.disabled = false;
          var vlabel = document.getElementById('app-version');
          if (vlabel && info && info.current) {
            vlabel.textContent = info.current === 'dev' ? 'dev' : 'v' + info.current;
          }
          if (info && info.available && info.latest) {
            status.innerHTML = 'Update available: v' + info.latest +
              ' - <a href="' + info.url + '" target="_blank" rel="noopener">download</a>';
            showUpdateBanner(info);
          } else if (info && info.latest) {
            status.textContent = "You're on the latest version (v" + info.current + ').';
          } else {
            status.textContent = 'Could not reach the update server. Try again shortly.';
          }
        })
        .catch(function() {
          btn.disabled = false;
          status.textContent = 'Could not reach the update server. Try again shortly.';
        });
    });
  })();

  // --- Light/dark theme toggle ---------------------------------------------
  // The active theme is applied pre-paint by the inline <head> script (from
  // localStorage, falling back to the OS preference). This only handles the
  // manual flip and persists the choice per-device. The sun/moon glyph swap is
  // pure CSS keyed off the data-theme attribute.
  (function() {
    var btn = document.getElementById('btn-theme');
    if (!btn) return;
    function syncTitle() {
      var dark = document.documentElement.getAttribute('data-theme') === 'dark';
      var label = dark ? 'Switch to light theme' : 'Switch to dark theme';
      btn.title = label;
      btn.setAttribute('aria-label', label);
    }
    syncTitle();
    btn.addEventListener('click', function() {
      var dark = document.documentElement.getAttribute('data-theme') === 'dark';
      var next = dark ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', next);
      try { localStorage.setItem('theme', next); } catch (e) {}
      syncTitle();
    });
  })();

  function toggleFav(el) {
    var $star = $(el);
    var current = parseInt($star.attr('data-fav'), 10) || 0;
    var next = current === 1 ? 0 : 1;
    setFavStar($star, next);
    var $tr = $star.closest('tr');
    editHistory.record($tr.attr('data-name'), 'favorite', current, next);
    queueSave($tr);
  }
  $('#models').on('click', '.fav-star', function() { toggleFav(this); });
  $('#models').on('keydown', '.fav-star', function(e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault();
    toggleFav(this);
  });

  // Copy helper for the details-modal "Copy JSON" button. navigator.clipboard
  // needs a secure context (present on 127.0.0.1 but NOT on a plain-http
  // Tailscale/LAN origin), so fall back to a temporary textarea + execCommand
  // where it is absent.
  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      return navigator.clipboard.writeText(text);
    }
    return new Promise(function(resolve, reject) {
      try {
        var ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.top = '-1000px';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.focus();
        ta.select();
        var ok = document.execCommand('copy');
        document.body.removeChild(ta);
        ok ? resolve() : reject(new Error('execCommand copy failed'));
      } catch (e) { reject(e); }
    });
  }
  // Click-to-edit the personal note on the Name cell's third line. The line is
  // hidden while the note is empty, so the note icon on the second line is the way
  // in to write a first note; both routes land here.
  function openNoteEditor(display) {
    var $display = $(display);
    var currentText = $display.text().trim();
    if (currentText === NOTES_PLACEHOLDER) currentText = '';
    // Stash the pre-edit text on the textarea so the commit handler can record
    // the old value for undo without re-reading the (now replaced) cell.
    var $ta = $('<textarea class="notes-input"></textarea>').val(currentText);
    $ta.attr('data-orig', currentText);
    $display.replaceWith($ta);
    // The note line is revealed by the textarea replacing the placeholder span, so
    // force the style recalc to land before focusing: focus() on a node the browser
    // still has as display:none does nothing, and the user loses their first keystrokes.
    void document.documentElement.offsetHeight;
    // Focus the textarea, not the span: replaceWith detaches the span, and focusing a
    // detached node does nothing, which costs the user a second click before typing.
    $ta.focus();
  }
  $('#models').on('click', '.notes-display', function() { openNoteEditor(this); });
  // The note icon on the Name cell's second line opens the same editor. It is the
  // only affordance while a model has no note (the note line is hidden until then).
  $('#models').on('click', '.notes-edit', function(e) {
    e.stopPropagation();
    var $display = $(this).closest('td').find('.notes-display');
    if ($display.length) openNoteEditor($display);
  });
  $('#models').on('keydown', '.notes-edit', function(e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    e.preventDefault();
    e.stopPropagation();
    var $display = $(this).closest('td').find('.notes-display');
    if ($display.length) openNoteEditor($display);
  });
  
  $('#models').on('blur keydown', '.notes-input', function(e) {
    if (e.type === 'keydown' && (e.key !== 'Enter' || e.shiftKey)) return;
    if (e.type === 'keydown') e.preventDefault();
    // Committing swaps this textarea back to a display span via replaceWith().
    // Detaching a focused textarea fires a second, synchronous blur that re-enters
    // this handler on the same element; guard so that re-entry does not run
    // replaceWith() on an already-detached node (which throws a replaceChild error)
    // or queue a duplicate save.
    if (this._notesCommitting) return;
    this._notesCommitting = true;
    let val = $(this).val();
    const orig = $(this).attr('data-orig') || '';
    let disp = val ? esc(val) : '<em class="notes-placeholder">' + NOTES_PLACEHOLDER + '</em>';
    // Capture tr reference BEFORE replaceWith removes this element from DOM
    const tr = $(this).closest('tr');
    $(this).replaceWith($('<span class="notes-display"></span>').html(disp));
    editHistory.record(tr.attr('data-name'), 'notes', orig, val);
    queueSave(tr);
  });

  // --- Custom columns (generic personal-column system) ---------------------
  // Replaces the old fixed Speed/OCR/Rating dropdowns and the notes-style
  // click-to-edit text field with one generic mechanism, driven entirely by
  // each column's own definition (customColumnsById) rather than a hardcoded
  // field name.

  // Dropdown-type column: sort order is the option's position in the column's
  // own list (generalizing the old fixed Speed/OCR/Rating ORDER maps) - unset
  // always sorts last (UNSET_ORDER), whatever the option count.
  var CC_UNSET_ORDER = 9999;
  $('#models').on('change', '.cc-select', function() {
    const $sel = $(this);
    const colId = $sel.attr('data-col-id');
    const col = customColumnsById[colId];
    const opts = (col && col.options) || [];
    const idx = opts.indexOf($sel.val());
    $sel.closest('td').attr('data-dt-order', idx >= 0 ? idx + 1 : CC_UNSET_ORDER);
    const $tr = $sel.closest('tr');
    editHistory.record($tr.attr('data-name'), 'custom:' + colId, $sel.attr('data-prev') || '', $sel.val());
    $sel.attr('data-prev', $sel.val());
    queueSave($tr);
  });

  // Text-type column: click-to-edit, modeled on the personal `notes` field
  // (openNoteEditor above) - a display span swaps to an input on click, and
  // commits back on blur/Enter. data-dt-order is kept in sync with the plain
  // text on every commit, so the column sorts alphabetically (see the
  // orderDataType wiring in dtCols above).
  function openCcTextEditor(display) {
    const $display = $(display);
    const colId = $display.attr('data-col-id');
    let currentText = $display.text().trim();
    if (currentText === CC_TEXT_PLACEHOLDER) currentText = '';
    const $inp = $('<input type="text" class="cc-text-input" data-col-id="' + colId + '">').val(currentText);
    $inp.attr('data-orig', currentText);
    $display.replaceWith($inp);
    $inp.focus();
  }
  $('#models').on('click', '.cc-text-display', function() { openCcTextEditor(this); });
  $('#models').on('blur keydown', '.cc-text-input', function(e) {
    if (e.type === 'keydown' && e.key !== 'Enter') return;
    if (e.type === 'keydown') e.preventDefault();
    if (this._ccCommitting) return;
    this._ccCommitting = true;
    const $inp = $(this);
    const colId = $inp.attr('data-col-id');
    const val = $inp.val();
    const orig = $inp.attr('data-orig') || '';
    const disp = val ? esc(val) : '<em class="notes-placeholder">' + CC_TEXT_PLACEHOLDER + '</em>';
    const tr = $inp.closest('tr');
    $inp.closest('td').attr('data-dt-order', val || '');
    $inp.replaceWith($('<span class="cc-text-display" data-col-id="' + colId + '"></span>').html(disp));
    editHistory.record(tr.attr('data-name'), 'custom:' + colId, orig, val);
    queueSave(tr);
  });

  // --- Pricing-note badge: hover preview + click-to-edit pinned modal --------
  // The "!" badge in the Unit cell carries the note text in data-pnote. Hovering
  // shows a read-only tip; clicking opens a small pinned modal with a textarea so
  // the note can be edited and its text selected. The modal stays open until you
  // click outside it (or press Esc/Save), at which point any change is saved.
  var $pnoteTip = $('#pnote-tip');
  var $pnoteModal = $('#pnote-modal');
  var pnoteIcon = null;   // the badge currently being edited
  var pnoteTr = null;     // its row

  function placeFloating($el, rect) {
    // Position a fixed-position element just below the badge, kept on-screen.
    var top = rect.bottom + 6;
    var left = rect.left;
    var vw = window.innerWidth, w = $el.outerWidth();
    if (left + w > vw - 8) left = Math.max(8, vw - 8 - w);
    $el.css({ top: top + 'px', left: left + 'px' });
  }

  $('#models').on('mouseenter', '.pnote-icon', function() {
    if ($pnoteModal.is(':visible')) return; // don't fight the open editor
    var note = $(this).attr('data-pnote') || '';
    $pnoteTip.text(note.trim() ? note : 'No pricing note - click to add one.').show();
    placeFloating($pnoteTip, this.getBoundingClientRect());
  });
  $('#models').on('mouseleave', '.pnote-icon', function() {
    $pnoteTip.hide();
  });

  function closePnoteModal(save) {
    if (pnoteIcon && save) {
      var val = $('#pnote-modal-text').val();
      var old = pnoteIcon.attr('data-pnote') || '';
      if (val !== old && pnoteTr) {
        setPnoteBadges(pnoteTr, val);
        editHistory.record(pnoteTr.attr('data-name'), 'pricing_note', old, val);
        queueSave(pnoteTr);
      }
    }
    $pnoteModal.hide();
    pnoteIcon = null;
    pnoteTr = null;
  }

  $('#models').on('click', '.pnote-icon', function(e) {
    e.stopPropagation();
    $pnoteTip.hide();
    pnoteIcon = $(this);
    pnoteTr = pnoteIcon.closest('tr');
    $('#pnote-modal-text').val(pnoteIcon.attr('data-pnote') || '');
    $pnoteModal.show();
    placeFloating($pnoteModal, this.getBoundingClientRect());
    $('#pnote-modal-text').focus();
  });
  // Interactions inside the modal (incl. selecting textarea text) must not bubble
  // to the document outside-close handlers below.
  $pnoteModal.on('mousedown touchstart', function(e) { e.stopPropagation(); });
  $('#pnote-modal-save').on('click', function() { closePnoteModal(true); });
  $('#pnote-modal-close').on('click', function() { closePnoteModal(true); });
  $('#pnote-modal-text').on('keydown', function(e) {
    if (e.key === 'Escape') { e.preventDefault(); closePnoteModal(false); }
  });
  // A press anywhere outside the open modal commits and closes it. touchstart
  // gives touch devices the same outside-close as mouse; the flag stops the
  // browser's synthesized mousedown from firing a second close on the same tap.
  var pnoteTouchClosed = false;
  $(document).on('touchstart', function() {
    if ($pnoteModal.is(':visible')) { pnoteTouchClosed = true; closePnoteModal(true); }
  });
  $(document).on('mousedown', function() {
    if (pnoteTouchClosed) { pnoteTouchClosed = false; return; }
    if ($pnoteModal.is(':visible')) closePnoteModal(true);
  });

  // The model name opens the Details modal (its JSON rides in data-json). Openable
  // by click or by keyboard (Enter/Space) since it is a role=button element.
  function openDetails(el) {
    $('#modal-json').text($(el).attr('data-json'));
    $('#modal').css('display', 'flex');
  }
  $('#models').on('click', '.name-open', function() { openDetails(this); });
  $('#models').on('keydown', '.name-open', function(e) {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openDetails(this); }
  });

  // Copy-id icon beside the name: copies the OpenRouter model id via copyText
  // (which handles a non-secure origin). Briefly swaps to a check glyph. Openable
  // by click or keyboard; stops propagation so it never opens the Details modal.
  function copyModelId(el) {
    var $el = $(el);
    copyText($el.attr('data-id')).then(function() {
      $el.addClass('copied').html(CHECK_ICON);
      setTimeout(function() { $el.removeClass('copied').html(COPY_ICON); }, 1100);
    }).catch(function() {});
  }
  $('#models').on('click', '.copy-id', function(e) { e.stopPropagation(); copyModelId(this); });
  $('#models').on('keydown', '.copy-id', function(e) {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); e.stopPropagation(); copyModelId(this); }
  });
  $('#modal-close, #modal').on('click', function(e) {
    if (e.target === this) $('#modal').css('display', 'none');
  });
  // Copy the details JSON (mobile modal button). Reuses copyText, which handles a
  // non-secure origin via the textarea fallback. Flashes "Copied" briefly.
  $('#modal-copy').on('click', function() {
    var btn = $(this);
    copyText($('#modal-json').text()).then(function() {
      btn.text('Copied');
      setTimeout(function() { btn.text('Copy JSON'); }, 1100);
    }).catch(function() {
      btn.text('Copy failed');
      setTimeout(function() { btn.text('Copy JSON'); }, 1100);
    });
  });

  // ---- Per-measurement In/Out colour ranges ------------------------------
  // Recolour every In/Out cell from the current thresholds, across ALL rows
  // (table.rows().nodes() includes filtered-out and paged-out rows).
  function recolorPrices() {
    $(table.rows().nodes()).each(function() {
      var meas = ($(this).attr('data-meas') || '').trim();
      $(this).find('.price-cell').each(function() {
        var v = parseFloat($(this).attr('data-val'));
        if (!isNaN(v)) $(this).css('background-color', priceColor(meas, v));
      });
    });
  }

  // Build the panel rows: every known measurement plus any extra unit present
  // in the data (so a newly scraped unit still gets an editable row).
  function buildColorPanel() {
    var $tb = $('#color-table tbody').empty();
    var keys = Object.keys(DEFAULT_COLOR_RANGES);
    $('#f_measurement option').each(function() {
      var v = $(this).attr('value');
      if (v && keys.indexOf(v) === -1) keys.push(v);
    });
    keys.forEach(function(k) {
      var r = colorRanges[k] || DEFAULT_COLOR_RANGES[k] || { low: 1, high: 2 };
      var label = k === '' ? '(no unit)' : k;
      $tb.append('<tr>' +
        '<td class="cr-unit">' + esc(label) + '</td>' +
        '<td><input type="number" step="any" min="0" class="cr-low" data-meas="' + esc(k) + '" value="' + r.low + '"></td>' +
        '<td><input type="number" step="any" min="0" class="cr-high" data-meas="' + esc(k) + '" value="' + r.high + '"></td>' +
        '<td class="cr-swatch"><span class="swatch-green"></span><span class="swatch-yellow"></span><span class="swatch-red"></span></td>' +
        '</tr>');
    });
  }

  $('#color-btn').on('click', function() {
    buildColorPanel();
    $('#color-modal').css('display', 'flex');
  });
  $('#color-modal-close, #color-done').on('click', function() {
    $('#color-modal').css('display', 'none');
  });
  $('#color-modal').on('click', function(e) {
    if (e.target === this) $('#color-modal').css('display', 'none');
  });
  $('#color-table').on('input change', 'input.cr-low, input.cr-high', function() {
    var meas = $(this).attr('data-meas');
    if (!colorRanges[meas]) {
      colorRanges[meas] = Object.assign({}, DEFAULT_COLOR_RANGES[meas] || { low: 1, high: 2 });
    }
    var v = parseFloat($(this).val());
    if (isNaN(v)) return;
    if ($(this).hasClass('cr-low')) colorRanges[meas].low = v;
    else colorRanges[meas].high = v;
    recolorPrices();
    saveFilters();
  });
  $('#color-reset').on('click', function() {
    colorRanges = JSON.parse(JSON.stringify(DEFAULT_COLOR_RANGES));
    buildColorPanel();
    recolorPrices();
    saveFilters();
  });

  // --- Custom columns management dialog -------------------------------------
  // Create or delete a user-defined personal column. Reuses the Colors panel's
  // modal look (#color-modal-content etc. via shared classes). Both creating
  // and deleting change the table's column SET, so - exactly like a column
  // reorder (see reorderColumns above) - the simplest fully-consistent way to
  // apply the change is to persist it server-side and reload; there is no
  // partial-rebuild path for adding/removing a whole column.
  function buildCcManageTable() {
    var $tb = $('#cc-manage-table tbody').empty();
    customColumns.forEach(function(c) {
      var typeLabel = c.type === 'text' ? 'Text'
        : (c.type === 'dropdown_number' ? 'Dropdown (numbers)' : 'Dropdown (text)');
      var $row = $('<tr></tr>');
      $row.append($('<td></td>').text(c.name));
      $row.append($('<td></td>').text(typeLabel));
      var $del = $('<button type="button" class="btn-clear cc-delete-btn">Delete</button>')
        .attr('data-id', c.id).attr('data-name', c.name);
      $row.append($('<td></td>').append($del));
      $tb.append($row);
    });
  }
  $('#cc-manage-btn').on('click', function() {
    buildCcManageTable();
    $('#cc-new-name').val('');
    $('#cc-new-type').val('text');
    $('#cc-new-options').val('').hide();
    $('#cc-new-status').text('');
    $('#cc-manage-modal').css('display', 'flex');
  });
  $('#cc-manage-close, #cc-manage-modal').on('click', function(e) {
    if (e.target === this) $('#cc-manage-modal').css('display', 'none');
  });
  // The value textarea only makes sense for a dropdown type.
  $('#cc-new-type').on('change', function() {
    $('#cc-new-options').toggle($(this).val() !== 'text');
  });
  $('#cc-manage-table').on('click', '.cc-delete-btn', function() {
    var id = $(this).attr('data-id');
    var name = $(this).attr('data-name');
    if (!confirm('Delete the "' + name + '" column?\n\nThis permanently erases every model\'s stored value for it. This cannot be undone.')) return;
    fetch('/api/custom-columns?id=' + encodeURIComponent(id), { method: 'DELETE' })
      .then(function(r) { if (!r.ok) throw new Error('status ' + r.status); return r.json(); })
      .then(function() {
        statusText('Column deleted - reloading...');
        setTimeout(function() { location.reload(); }, 400);
      })
      .catch(function(e) {
        $('#cc-new-status').text('Delete failed: ' + (e.message || e));
      });
  });
  $('#cc-new-create').on('click', function() {
    var name = $('#cc-new-name').val().trim();
    var type = $('#cc-new-type').val();
    if (!name) { $('#cc-new-status').text('Name is required.'); return; }
    var options = [];
    if (type !== 'text') {
      options = $('#cc-new-options').val().split('\n').map(function(s) { return s.trim(); }).filter(function(s) { return s !== ''; });
      if (!options.length) { $('#cc-new-status').text('Enter at least one value, one per line.'); return; }
    }
    $('#cc-new-status').text('Adding...');
    fetch('/api/custom-columns', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name, type: type, options: options })
    })
      // A failure response body is plain text (http.Error), not JSON - read it
      // as text rather than assuming JSON, so a validation message ("a dropdown
      // column needs at least one value") reaches the user instead of being
      // swallowed by a JSON-parse error.
      .then(function(r) {
        if (r.ok) return null;
        return r.text().then(function(txt) { throw new Error(txt || ('status ' + r.status)); });
      })
      .then(function() {
        statusText('Column added - reloading...');
        setTimeout(function() { location.reload(); }, 400);
      })
      .catch(function(e) { $('#cc-new-status').text('Failed: ' + (e.message || e)); });
  });

  // --- Paths info dialog ---------------------------------------------------
  // Shows where this running instance keeps its data/config/cache files. Fetched
  // from GET /api/paths (read-only).
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
    ['database', 'curated.json', 'config.jsonc', 'settings.json', 'backups', 'logs', 'pid', 'update-check'].forEach(function(k) {
      if (files[k]) lines.push(padr(k + ':', 14) + files[k]);
    });
    lines.push('');
    lines.push('Personal data (notes, ratings, favorites, speed, OCR quality) is');
    lines.push('stored inside the database only - there is no separate personal file.');
    document.getElementById('paths-pre').textContent = lines.join('\n');
  }
  $('#btn-paths').on('click', function() {
    document.getElementById('paths-pre').textContent = 'Loading...';
    document.getElementById('paths-modal').style.display = 'flex';
    fetch('/api/paths').then(function(r) { return r.json(); }).then(showPaths).catch(function(e) {
      document.getElementById('paths-pre').textContent = 'Failed to load paths: ' + (e.message || e);
    });
  });
  $('#paths-close, #paths-modal').on('click', function(e) {
    if (e.target === this) document.getElementById('paths-modal').style.display = 'none';
  });
});

function esc(s) {
    if (s == null) return "";
    return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

// The personal note a row currently holds, as plain text ('' when there is none).
// The note sits on the third line of the Name cell, as a display span that swaps to
// a textarea while it is being edited - so read whichever of the two is present.
// The placeholder is an empty note, not text.
const NOTES_PLACEHOLDER = 'Click to add note...';
function rowNoteText($tr) {
    const $input = $tr.find('.notes-input');
    if ($input.length) return $input.val() || '';
    const text = $tr.find('.notes-display').text();
    return text === NOTES_PLACEHOLDER ? '' : text;
}

// The same click-to-edit-display pattern as rowNoteText/NOTES_PLACEHOLDER above,
// generalized to a text-type custom column's cell (identified by its column id,
// since there can be any number of them): reads the open input if the cell is
// mid-edit, otherwise the display span's text. Returns undefined when the
// column's cell is not in the DOM at all (its table column is hidden), so
// queueSave can omit it exactly like every other field.
const CC_TEXT_PLACEHOLDER = 'Click to add...';
function rowCustomTextValue($tr, colId) {
    const $input = $tr.find('.cc-text-input[data-col-id="' + colId + '"]');
    if ($input.length) return $input.val() || '';
    const $display = $tr.find('.cc-text-display[data-col-id="' + colId + '"]');
    if (!$display.length) return undefined;
    const text = $display.text();
    return text === CC_TEXT_PLACEHOLDER ? '' : text;
}

// Set the favourite star to an exact state. The star lives at the end of the Name
// cell's icon row and carries its own value in data-fav; it writes no data-dt-order,
// because favourite has no column of its own to sort - the Name cell's sort text is
// the model name, and stamping an order attribute there would sort Name by favourite.
function setFavStar($star, fav) {
    const on = fav === 1;
    $star.attr('data-fav', on ? 1 : 0)
        .toggleClass('is-fav', on)
        .attr('title', on ? 'Remove from favorites' : 'Mark as favorite')
        .attr('aria-label', on ? 'Remove from favorites' : 'Mark as favorite');
}

// Write one model's pricing note to every badge in its row. A model has ONE
// pricing note, and both the In $ and the Out $ cell show a badge for it, so an
// edit through either has to land on both or the two would disagree and the save
// path would read whichever came first.
function setPnoteBadges($tr, value) {
    const note = value || '';
    const has = note.trim() !== '';
    $tr.find('.pnote-icon')
        .attr('data-pnote', note)
        .toggleClass('has-note', has)
        .toggleClass('pnote-empty', !has)
        .html(has ? PNOTE_ICON : PNOTE_ADD_ICON)
        .attr('title', has ? 'Pricing note - click to read or edit' : 'Add a pricing note');
}

// The pricing unit as shown in the In $/Out $ cells. The stored values are long
// ("per 1M tokens"), and a price column cannot carry that much text, so each is
// abbreviated. An unrecognised unit falls through to its raw text rather than
// vanishing - upstream can add a unit at any time, and a wrong-looking label is
// recoverable where a silently dropped one is not.
const UNIT_ABBREV = {
    'per 1M tokens':     'tkn',
    'per 1M characters': 'chr',
    'per image':         'img',
    'per second':        'sec',
    'per minute':        'min',
    'per hour':          'hr',
    'per megapixel':     'mpx',
    'per search':        'srch'
};
function unitAbbrev(measurement) {
    const raw = String(measurement || '').trim();
    if (raw === '') return '';
    return UNIT_ABBREV[raw] || raw;
}

// In $/Out $ cell colours. Each measurement has its own pair of thresholds, set
// against the number AS DISPLAYED in the column (per-1M figure for token/char
// units, literal price otherwise). value < low -> green, < high -> yellow, else
// red. Editable from the header "Colors" panel and persisted in settings.json.
const PRICE_COLORS = { green: '#22c55e', yellow: '#eab308', red: '#ef4444' };
const DEFAULT_COLOR_RANGES = {
    '':                  { low: 1,      high: 2 },   // no unit (legacy/token)
    'per 1M tokens':     { low: 1,      high: 2 },
    'per 1M characters': { low: 10,     high: 20 },
    'per second':        { low: 0.05,   high: 0.15 },
    'per minute':        { low: 0.005,  high: 0.012 },
    'per hour':          { low: 0.1,    high: 0.3 },
    'per image':         { low: 0.02,   high: 0.05 },
    'per megapixel':     { low: 0.03,   high: 0.06 },
    'per search':        { low: 0.0015, high: 0.0025 }
};
var colorRanges = JSON.parse(JSON.stringify(DEFAULT_COLOR_RANGES));

function priceColor(meas, disp) {
    var r = colorRanges[meas] || colorRanges[''] || { low: 1, high: 2 };
    if (disp < r.low) return PRICE_COLORS.green;
    if (disp < r.high) return PRICE_COLORS.yellow;
    return PRICE_COLORS.red;
}

// OpenRouter official brand glyph (v2), inlined verbatim so the app stays fully
// self-contained (no external asset). It links to the model's OpenRouter page
// (nominative use). The path carries NO fill: the colour is set in CSS from the
// theme-keyed --or-fill token (purple in light, lime in dark), matching the two
// brand variants. No width/height attributes either - CSS sizes it by height and
// lets the width follow the 401.4:293.7 aspect ratio so it never distorts.
const OR_ICON = '<svg class="or-ico" viewBox="0 0 401.4 293.7" aria-hidden="true" focusable="false"><path d="M303.9475,17.19926c42.79734,0,77.48933,34.69327,77.48933,77.48933s-34.69199,77.48933-77.48933,77.48933l76.86166,76.86244c9.76367,9.76313,2.84903,26.45667-10.95697,26.45667h-220.88335c-71.32686,0-129.14889-57.82202-129.14889-129.14889S77.64197,17.19926,148.96884,17.19926h154.97866ZM148.96884,68.85881c-42.79607,0-77.48933,34.69327-77.48933,77.48933s34.69327,77.48933,77.48933,77.48933,77.48933-34.69327,77.48933-77.48933-34.69327-77.48933-77.48933-77.48933Z"/></svg>';

// Copy glyph (two overlapping sheets) for the copy-model-id control next to the
// name, and a check glyph flashed after a successful copy. Drawn in currentColor
// so they theme and inherit the control colour.
const COPY_ICON = '<svg class="copy-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>';
const CHECK_ICON = '<svg class="copy-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>';

// Personal-note glyph for the Name cell: a speech bubble with lines, drawn in
// currentColor so it themes and inherits the control colour. It marks the note
// affordance whether or not the model has a note - .has-note colours the set case.
// This is the PERSONAL note (the `notes` field), not the objective pricing note on
// the price cells, so it is a different mark on purpose.
const NOTE_ICON = '<svg class="note-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8z"/><path d="M8.5 10h7"/><path d="M8.5 13.5h4"/></svg>';

// Pricing-note glyph: a small document-with-lines icon drawn in currentColor (so
// it themes and inherits the badge colour). Replaces a plain "!" as the marker of
// a set pricing note. Sized in em via CSS so it scales with the badge on mobile.
const PNOTE_ICON = '<svg class="pnote-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/><path d="M9 13h6"/><path d="M9 17h4"/></svg>';

// Empty-state pricing-note glyph: a document outline with a small plus, the
// "add a note" affordance. Replaces the bare "+" so the empty badge reads as a
// note-to-add. Drawn in currentColor so it themes and inherits the badge colour.
const PNOTE_ADD_ICON = '<svg class="pnote-ico" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h6"/><path d="M14 3v5h5"/><path d="M9 12h4"/><path d="M18 15v6"/><path d="M15 18h6"/></svg>';

function renderCell(model, c) {
    let col = c.target;
    let label = c.title;
    // A custom column's value does not live directly on the row under its own
    // key (there could be any number of them); it rides in the row's
    // custom_values map, keyed by the column's id.
    let val = c.custom ? (model.custom_values || {})[c.custom.id] : model[col];
    if (val == null) val = "";

    let inner = "";
    let dataOrder = "";
    let tdClass = "";
    let dataParch = "";

    if (col === 'name') {
        // The Name cell is three lines:
        //   1. the model name, then the date
        //   2. the capability icons, then the HuggingFace, OpenRouter, copy-id and
        //      note controls
        //   3. the personal note, shown only when the model has one
        // Lines 1 and 2 never wrap; line 3 does. The date is also its own sortable
        // column, so .card-date here is hidden on desktop and shown on a card.
        let desc = esc(model.description);
        let icons = "";
        if (model.supports_reasoning) {
            icons += '<span class="cap-icon cap-think" title="Thinking Core">\u{1F9E0}</span>';
        }
        if (model.tool === 'yes') {
            icons += '<span class="cap-icon cap-tool" title="Tool Use">\u{1F527}</span>';
        }
        if (model.moe === 'yes') {
            icons += '<span class="cap-icon cap-moe" title="Mixture of Experts">\u{1F578}\u{FE0F}</span>';
        }
        if (model.zdr === true) {
            icons += '<span class="cap-icon cap-zdr" title="Zero Data Retention">\u{1F512}</span>';
        }
        // The name is the click target that opens the Details modal (the raw model
        // JSON rides in data-json). It is keyboard-openable (role=button, tabindex)
        // and does not navigate - the OpenRouter page link is its own icon below.
        let jsonStr = esc(JSON.stringify(model._raw || {}, null, 2));
        let name_html = `<span class="name-open" role="button" tabindex="0" data-json="${jsonStr}" title="Show model details">${esc(val)}</span>`;
        let dateStr = model.created_at ? esc(String(model.created_at).split('T')[0]) : '';
        let cardDate = dateStr ? `<span class="card-date">${dateStr}</span>` : '';
        let line1 = `<span class="name-line"><span class="tooltip">${name_html}<span class="tooltiptext">${desc}</span></span>${cardDate}</span>`;

        // HuggingFace repo link: only models that carry a slug have one.
        let hf = '';
        if (model.hf_slug) {
            hf = `<a href="https://huggingface.co/${esc(model.hf_slug)}" target="_blank" rel="noopener" class="hf-link" title="Open the HuggingFace page for ${esc(model.hf_slug)} (new tab)" aria-label="Open the HuggingFace page for ${esc(model.hf_slug)}">HF</a>`;
        }
        // OpenRouter page link + copy-id control: both need an OpenRouter model id,
        // which only source openrouter/collection models hold. HuggingFace-direct
        // and manual rows have neither.
        let or = '';
        let copyId = '';
        if (model.id) {
            let orSlug = model.slug || '';
            or = `<a href="https://openrouter.ai/${esc(orSlug)}" target="_blank" rel="noopener" class="or-link" title="Open the OpenRouter model page for ${esc(model.id)} (new tab)" aria-label="Open the OpenRouter model page for ${esc(model.id)}">${OR_ICON}</a>`;
            copyId = `<span class="copy-id" role="button" tabindex="0" data-id="${esc(model.id)}" title="Copy the OpenRouter model id (${esc(model.id)})" aria-label="Copy the model id ${esc(model.id)}">${COPY_ICON}</span>`;
        }
        let hasNote = String(model.notes || '').trim() !== '';
        let noteIcon = `<span class="notes-edit${hasNote ? ' has-note' : ''}" role="button" tabindex="0" title="${hasNote ? 'Edit this model\'s note' : 'Add a note about this model'}" aria-label="${hasNote ? 'Edit the note' : 'Add a note'}">${NOTE_ICON}</span>`;
        // The favourite star closes the icon row. It is the last mark on the line
        // because it is the only one that is a control rather than a link.
        let isFav = (model.favorite === 1 || model.favorite === true || String(model.favorite).toLowerCase() === 'true');
        let star = `<span class="fav-star${isFav ? ' is-fav' : ''}" data-fav="${isFav ? 1 : 0}" role="button" tabindex="0" title="${isFav ? 'Remove from favorites' : 'Mark as favorite'}" aria-label="${isFav ? 'Remove from favorites' : 'Mark as favorite'}">★</span>`;
        let line2 = `<span class="name-line meta-line">${icons}${hf}${or}${copyId}${noteIcon}${star}</span>`;

        // The note line always renders (the editor swaps this span for a textarea in
        // place), and CSS collapses it while it holds only the placeholder, so a
        // model without a note costs no height.
        let noteText = model.notes ? esc(String(model.notes)) : '<em class="notes-placeholder">Click to add note...</em>';
        let line3 = `<span class="name-note"><span class="notes-display">${noteText}</span></span>`;
        inner = `${line1}${line2}${line3}`;
    } else if (col === 'created_at') {
        inner = val ? String(val).split('T')[0] : "";
        tdClass = "nowrap";
    } else if (c.custom) {
        // Generic custom-column rendering (the replacement for the old hardcoded
        // Speed/Rating/OCR cells): a dropdown type renders a <select> of the
        // column's own options (unset shows an em dash), sorting by the option's
        // position - generalizing the old fixed ORDER maps, with unset always
        // sorting last regardless of how many options the column has. A text type
        // renders a click-to-edit cell (see openCcTextEditor), sorting
        // alphabetically on the raw string. No coloring on any custom column, per
        // this feature's scope - unlike the old Rating dropdown, its options carry
        // no .ropt-* colour classes.
        let colId = c.custom.id;
        if (c.custom.type === 'text') {
            dataOrder = val;
            let disp = val ? esc(val) : '<em class="notes-placeholder">' + 'Click to add...' + '</em>';
            inner = `<span class="cc-text-display" data-col-id="${colId}">${disp}</span>`;
        } else {
            let opts = c.custom.options || [];
            let idx = opts.indexOf(val);
            dataOrder = idx >= 0 ? idx + 1 : 9999;
            let optsHtml = [`<option value=""${val ? '' : ' selected'}>—</option>`];
            for (let o of opts) {
                let selected = (o === val) ? ' selected' : '';
                optsHtml.push(`<option value="${esc(o)}"${selected}>${esc(o)}</option>`);
            }
            // data-prev holds the last committed value so an edit can record the
            // old value for undo without relying on a focus event firing first.
            inner = `<select class="cc-select" data-col-id="${colId}" data-prev="${esc(val)}">${optsHtml.join('')}</select>`;
        }
        tdClass = 'cc-cell';
    } else if (col === 'disk_size_gb') {
        // Native-precision on-disk weight size in GB (curated), rounded up to a
        // whole number; the unit lives in the "Size (GB)" header. Blank when
        // unset. Sorts by the exact value via data-dt-order.
        if (val !== '' && val != null) {
            let num_val = parseFloat(val);
            if (!isNaN(num_val)) {
                dataOrder = num_val;
                inner = Math.ceil(num_val).toLocaleString();
            }
        } else {
            // Unset size sorts to the very bottom (ascending) so blank-size rows
            // trail the sized ones instead of leading them.
            dataOrder = 9999999999;
        }
    } else if (['context_length', 'prompt', 'completion', 'parameters', 'active_parameters'].includes(col)) {
        let num_val = 9999999999;
        if (val !== '' && val != null) {
            num_val = parseFloat(val);
            if (['prompt', 'completion'].includes(col)) {
                // In/Out store a per-unit price. For per-1M units (tokens / characters,
                // plus legacy rows with no unit) the stored number is per single unit,
                // so multiply by 1M to show the per-1M figure. For per-single-unit
                // measurements (per second/image/megapixel/minute/hour/search) the
                // stored number is already the literal price - show it as-is.
                let meas = String(model.measurement || '');
                let perMillion = (meas === '' || meas.indexOf('1M') !== -1);
                let disp = perMillion ? num_val * 1000000 : num_val;
                dataOrder = disp;
                let display_val = perMillion
                    ? disp.toLocaleString(undefined, {minimumFractionDigits:0, maximumFractionDigits:4})
                    : disp.toLocaleString(undefined, {maximumFractionDigits:8});
                // Colour by the per-measurement thresholds (editable from the header
                // "Colors" panel). data-val keeps the displayed number so the cell can
                // be recoloured live without re-rendering the table.
                // Only the background-color is inline: it is a continuous
                // threshold colour that cannot be expressed as a fixed class.
                let bg_color = priceColor(meas, disp);
                // Under the price sits the abbreviated pricing unit, so the figure
                // says what it is priced per without a column of its own, and beside
                // it the pricing-note badge (an objective caveat about SKUs this
                // price omits). There is ONE note per model and both price cells
                // show it, so the two badges are kept identical by setPnoteBadges.
                let pnote = model.pricing_note ? String(model.pricing_note) : '';
                let hasPnote = pnote.trim() !== '';
                let badge = `<span class="pnote-icon ${hasPnote ? 'has-note' : 'pnote-empty'}" data-pnote="${esc(pnote)}" title="${hasPnote ? 'Pricing note - click to read or edit' : 'Add a pricing note'}">${hasPnote ? PNOTE_ICON : PNOTE_ADD_ICON}</span>`;
                let unit = unitAbbrev(meas);
                let unitLine = `<span class="price-unit" title="${esc(meas)}">${esc(unit)}${badge}</span>`;
                inner = `<span class="price-cell" data-val="${disp}" style="background-color:${bg_color}">${display_val}</span>${unitLine}`;
            } else {
                dataOrder = num_val;
                if (col === 'context_length') {
                    // Context displays as rounded thousands + "k" (e.g. "128k") in
                    // both views; sorting stays on the raw number via data-dt-order.
                    inner = Math.round(num_val / 1000) + 'k';
                } else {
                    inner = num_val.toLocaleString(undefined, {maximumFractionDigits:0});
                }
                // The mobile card packs total + active params into ONE cell as
                // "398B / 94B". data-parch is the suffix the card appends after the
                // total number ("B" alone when there is no active count); the
                // separate active_parameters cell is hidden on the card. Inert on
                // desktop, where both keep their own columns.
                if (col === 'parameters') {
                    let ap = model.active_parameters;
                    if (ap !== '' && ap != null && !isNaN(parseFloat(ap))) {
                        dataParch = 'B / ' + parseFloat(ap).toLocaleString(undefined, {maximumFractionDigits:0}) + 'B';
                    } else {
                        dataParch = 'B';
                    }
                }
            }
        } else {
            inner = esc(val);
        }
    } else {
        inner = esc(val);
    }
    
    let attr = dataOrder !== "" ? ` data-dt-order="${dataOrder}"` : "";
    let classAttr = tdClass ? ` class="${tdClass}"` : "";
    // data-label carries the column title so the mobile card CSS can surface it
    // via td::before. data-col lets the mobile card grid place each cell by field
    // (so a desktop column reorder never scrambles the cards). Both are inert on
    // desktop - filters/sort still read .text()/data-dt-order.
    let labelAttr = ` data-label="${esc(label)}"`;
    let colAttr = ` data-col="${esc(col)}"`;
    let parchAttr = dataParch !== "" ? ` data-parch="${esc(dataParch)}"` : "";
    return `<td${attr}${classAttr}${colAttr}${parchAttr}${labelAttr}>${inner}</td>`;
}
