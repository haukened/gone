// Docs search: a dialog that searches an index Hugo builds (docs/index.json),
// fetched from this site the first time search opens. "/" or Ctrl/Cmd-K opens
// it; arrows move through results; Enter opens one; Escape closes. Without
// this script the search buttons stay hidden and nothing else changes.
(function () {
  'use strict';

  const MAX_RESULTS = 12;
  const SNIPPET = 80;

  const dialog = document.querySelector('dialog.search');
  if (!dialog || typeof dialog.showModal !== 'function') return;
  const input = dialog.querySelector('#search-input');
  const list = dialog.querySelector('#search-results');
  const status = dialog.querySelector('#search-status');

  let loading = null;
  let results = [];
  let active = -1;

  function prepare(page) {
    const heads = page.h.map((h) => ({ id: h.i, text: h.t, body: h.x, low: h.t.toLowerCase(), lowBody: h.x.toLowerCase() }));
    return {
      page: page,
      title: (page.t + ' ' + page.l).toLowerCase(),
      heads: heads,
      text: [page.x].concat(heads.map((h) => h.body)).join(' ').toLowerCase(),
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

  // bestSection returns the section the terms point to: a term in its
  // heading counts three times one in its text.
  function bestSection(entry, terms) {
    let best = null;
    let most = 0;
    for (const h of entry.heads) {
      let n = 0;
      for (const t of terms) n += (h.low.includes(t) ? 3 : 0) + (h.lowBody.includes(t) ? 1 : 0);
      if (n > most) { best = h; most = n; }
    }
    return { head: best, count: most };
  }

  // score ranks a page: a term in the title counts most, then in a
  // heading, then in the text. A page missing any term scores 0.
  function score(entry, terms) {
    let total = 0;
    for (const t of terms) {
      const inTitle = entry.title.includes(t);
      const inHead = entry.heads.some((h) => h.low.includes(t));
      if (!inTitle && !inHead && !entry.text.includes(t)) return 0;
      total += inTitle ? 10 : inHead ? 4 : 1;
    }
    return total;
  }

  // snippet shows the text around the first term, from the matched
  // section when there is one.
  function snippet(r) {
    const source = r.head ? r.head.body : r.entry.page.x;
    const at = source.toLowerCase().indexOf(r.terms[0]);
    if (at < 0) return source.slice(0, SNIPPET * 2);
    const from = Math.max(0, at - SNIPPET);
    return (from > 0 ? '… ' : '') + source.slice(from, at + SNIPPET) + ' …';
  }

  function find(query, pages) {
    const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
    if (!terms.length) return [];
    return pages
      .map((entry) => {
        const best = bestSection(entry, terms);
        return { entry: entry, head: best.head, score: score(entry, terms) + best.count * 2, terms: terms };
      })
      .filter((r) => r.score > 0)
      .sort((a, b) => b.score - a.score)
      .slice(0, MAX_RESULTS);
  }

  function resultItem(r, i) {
    const p = r.entry.page;
    const li = document.createElement('li');
    li.setAttribute('role', 'presentation');
    const a = document.createElement('a');
    a.id = 'search-result-' + i;
    a.href = p.u + (r.head ? '#' + r.head.id : '');
    a.setAttribute('role', 'option');
    const title = document.createElement('span');
    title.className = 'search-title';
    title.textContent = p.l + (r.head ? ' › ' + r.head.text : '');
    const where = document.createElement('span');
    where.className = 'search-where';
    where.textContent = p.s;
    const text = document.createElement('span');
    text.className = 'search-text';
    text.textContent = snippet(r);
    a.append(title, where, text);
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

  function show(query, pages) {
    results = find(query, pages);
    list.replaceChildren(...results.map(resultItem));
    select(results.length ? 0 : -1);
    input.setAttribute('aria-expanded', results.length ? 'true' : 'false');
    if (!query.trim()) status.textContent = '';
    else if (!results.length) status.textContent = 'No pages match “' + query.trim() + '”.';
    else status.textContent = results.length + (results.length === 1 ? ' page matches.' : ' pages match.');
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
    // A result on this page only moves the page, so close the dialog too.
    list.addEventListener('click', function (e) { if (e.target.closest('a')) dialog.close(); });
    // A click on the backdrop (the dialog itself, outside its box) closes it.
    dialog.addEventListener('click', function (e) { if (e.target === dialog) dialog.close(); });
  }

  init();
})();
