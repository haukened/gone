// Docs search: a dialog that searches an index Hugo builds (docs/index.json),
// fetched from this site the first time search opens. "/" or Ctrl/Cmd-K opens
// it; arrows move through results; Enter opens one; Escape closes. A result
// links to the section, variable or flag that matched. Without this script
// the search buttons stay hidden and nothing else changes.
(function () {
  'use strict';

  const MAX_RESULTS = 12;
  const SNIPPET = 80;

  const dialog = document.querySelector('dialog.search');
  if (!dialog || typeof dialog.showModal !== 'function') return;
  const input = dialog.querySelector('#search-input');
  const list = dialog.querySelector('#search-results');
  const status = dialog.querySelector('#search-status');
  const empty = dialog.querySelector('.search-empty');

  let loading = null;
  let active = -1;

  // A place a result can point to: a section, a variable or a flag.
  function target(id, name, body, isRow) {
    return { id: id, name: name, body: body, isRow: isRow, low: name.toLowerCase(), lowBody: body.toLowerCase() };
  }

  function prepare(page) {
    const spots = page.h.map((h) => target(h.i, h.t, h.x, false))
      .concat(page.r.map((r) => target(r.i, r.t, r.x, true)));
    return {
      page: page,
      title: (page.t + ' ' + page.l).toLowerCase(),
      spots: spots,
      text: [page.x].concat(spots.map((s) => s.body)).join(' ').toLowerCase(),
    };
  }

  function load() {
    if (!loading) {
      loading = fetch(dialog.dataset.index, { credentials: 'same-origin' })
        .then((r) => { if (!r.ok) throw new Error('index ' + r.status); return r.json(); })
        .then((pages) => pages.map(prepare));
    }
    return loading;
  }

  // compact folds a name for exact matching: "trusted proxies" and
  // "GONE_TRUSTED_PROXIES" both contain "trustedproxies".
  function compact(s) {
    return s.toLowerCase().replace(/[^a-z0-9]/g, '');
  }

  // spotScore rates one place: a term in its name counts three times one in
  // its text, and a variable or flag whose name holds the whole query
  // outranks everything else.
  function spotScore(spot, terms, whole) {
    let n = 0;
    for (const t of terms) n += (spot.low.includes(t) ? 3 : 0) + (spot.lowBody.includes(t) ? 1 : 0);
    if (spot.isRow && whole.length > 2 && compact(spot.name).includes(whole)) n += 20;
    return n;
  }

  function bestSpot(entry, terms, whole) {
    let best = null;
    let most = 0;
    for (const spot of entry.spots) {
      const n = spotScore(spot, terms, whole);
      if (n > most) { best = spot; most = n; }
    }
    return { spot: best, count: most };
  }

  // pageScore ranks a page: a term in the title counts most, then in a
  // heading or name, then in the text. A page missing any term scores 0.
  function pageScore(entry, terms) {
    let total = 0;
    for (const t of terms) {
      const inTitle = entry.title.includes(t);
      const inSpot = entry.spots.some((s) => s.low.includes(t));
      if (!inTitle && !inSpot && !entry.text.includes(t)) return 0;
      total += inTitle ? 10 : inSpot ? 4 : 1;
    }
    return total;
  }

  function find(query, pages) {
    const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
    if (!terms.length) return [];
    const whole = compact(query);
    return pages
      .map((entry) => {
        const best = bestSpot(entry, terms, whole);
        return { entry: entry, spot: best.spot, score: pageScore(entry, terms) + best.count * 2, terms: terms };
      })
      .filter((r) => r.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, MAX_RESULTS);
  }

  // snippet returns the text around the first term, from the matched place
  // when there is one.
  function snippet(r) {
    const source = r.spot ? r.spot.body : r.entry.page.x;
    const at = source.toLowerCase().indexOf(r.terms[0]);
    if (at < 0) return source.slice(0, SNIPPET * 2);
    const from = Math.max(0, at - SNIPPET);
    return (from > 0 ? '… ' : '') + source.slice(from, at + SNIPPET) + ' …';
  }

  function escapeRe(s) {
    return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  // marked builds text with each search term in a <mark>, from text nodes
  // only.
  function marked(text, terms) {
    const out = document.createDocumentFragment();
    const re = new RegExp('(' + terms.map(escapeRe).join('|') + ')', 'gi');
    text.split(re).forEach(function (part, i) {
      if (i % 2) {
        const m = document.createElement('mark');
        m.textContent = part;
        out.append(m);
      } else if (part) {
        out.append(document.createTextNode(part));
      }
    });
    return out;
  }

  function span(className, content) {
    const s = document.createElement('span');
    s.className = className;
    s.append(content);
    return s;
  }

  function resultItem(r, i) {
    const p = r.entry.page;
    const li = document.createElement('li');
    li.setAttribute('role', 'presentation');
    const a = document.createElement('a');
    a.id = 'search-result-' + i;
    a.href = p.u + (r.spot ? '#' + r.spot.id : '');
    a.setAttribute('role', 'option');
    const name = p.l + (r.spot ? ' › ' + r.spot.name : '');
    a.append(span('search-title', marked(name, r.terms)), span('search-where', p.s),
      span('search-text', marked(snippet(r), r.terms)));
    li.append(a);
    return li;
  }

  function select(i) {
    const items = list.querySelectorAll('a');
    if (active >= 0 && items[active]) items[active].removeAttribute('aria-selected');
    active = i;
    if (i < 0 || !items[i]) { input.removeAttribute('aria-activedescendant'); return; }
    items[i].setAttribute('aria-selected', 'true');
    items[i].scrollIntoView({ block: 'nearest' });
    input.setAttribute('aria-activedescendant', items[i].id);
  }

  function report(query, count) {
    empty.hidden = !query.trim() || count > 0;
    if (!query.trim()) status.textContent = '';
    else if (!count) status.textContent = 'No pages match “' + query.trim() + '”.';
    else status.textContent = count + (count === 1 ? ' page matches.' : ' pages match.');
  }

  function show(query, pages) {
    const results = find(query, pages);
    list.replaceChildren(...results.map(resultItem));
    select(results.length ? 0 : -1);
    input.setAttribute('aria-expanded', results.length ? 'true' : 'false');
    report(query, results.length);
  }

  function update() {
    const query = input.value;
    load().then((pages) => { if (input.value === query) show(query, pages); }, () => {
      status.textContent = 'Search isn’t available right now. Use the menu instead.';
    });
  }

  function open() {
    if (dialog.open) return;
    dialog.showModal();
    input.select();
    load().catch(() => {});
  }

  function onKey(e) {
    const items = list.querySelectorAll('a');
    if (e.key === 'ArrowDown' && items.length) { e.preventDefault(); select((active + 1) % items.length); }
    else if (e.key === 'ArrowUp' && items.length) { e.preventDefault(); select((active - 1 + items.length) % items.length); }
    else if (e.key === 'Enter' && items[active]) { e.preventDefault(); items[active].click(); }
  }

  // typing reports whether a key press belongs to a text field.
  function typing(e) {
    const t = e.target;
    return t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName);
  }

  function shortcuts(e) {
    const slash = e.key === '/' && !typing(e);
    const cmdK = (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k';
    if (slash || cmdK) { e.preventDefault(); open(); }
  }

  function init() {
    document.querySelectorAll('[data-search-open]').forEach(function (b) {
      b.hidden = false;
      b.addEventListener('click', open);
    });
    document.addEventListener('keydown', shortcuts);
    input.addEventListener('input', update);
    input.addEventListener('keydown', onKey);
    // A result on this page only moves the page, and the backdrop (the
    // dialog itself, outside its box) dismisses: both close the dialog.
    dialog.addEventListener('click', function (e) {
      if (e.target === dialog || e.target.closest('#search-results a, .search-empty a')) dialog.close();
    });
  }

  init();
})();
