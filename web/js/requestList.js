'use strict';

// Renders the requester's list of requests on /request: one row per request
// saved in this browser, with its label, age and state, linking to
// /request/{id}. Text is set with textContent only. Exposed as
// window.goneRequestList.
(function requestListModule() {
  if (window.goneRequestList || !window.goneUtil) return;
  const util = window.goneUtil;
  const UNTITLED = 'Untitled request';
  const STATES = { waiting: 'Waiting', ready: 'Reply ready' };
  const MINUTE = 60000;
  const HOUR = 60 * MINUTE;
  const DAY = 24 * HOUR;

  function byId(id) {
    return document.getElementById(id);
  }

  // since renders how long ago a timestamp was, in the largest whole unit.
  function since(ms, now) {
    const d = Math.max(0, now - ms);
    if (d < MINUTE) return 'just now';
    if (d < HOUR) return `${Math.floor(d / MINUTE)} min ago`;
    if (d < DAY) return `${Math.floor(d / HOUR)} h ago`;
    return `${Math.floor(d / DAY)} d ago`;
  }

  function title(entry) {
    return entry.label || UNTITLED;
  }

  function pill(state) {
    return util.el('span', { className: 'pill-status', textContent: STATES[state] || STATES.waiting }, []);
  }

  function row(entry, now) {
    const state = entry.state === 'ready' ? 'ready' : 'waiting';
    const p = pill(state);
    p.dataset.state = state;
    const link = util.el('a', { className: 'request-row', href: `/request/${entry.id}` }, [
      util.el('span', { className: 'name', textContent: title(entry) }, []),
      util.el('span', { className: 'when', textContent: `Asked ${since(entry.createdAt, now)}` }, []),
      p
    ]);
    const li = util.el('li', {}, [link]);
    li.dataset.id = entry.id;
    return li;
  }

  // render replaces the list with entries (newest first) and toggles the
  // empty-state note.
  function render(entries, now) {
    const list = byId('request-list');
    const empty = byId('request-empty');
    if (!list) return;
    const at = now === undefined ? Date.now() : now;
    list.replaceChildren(...entries.map(function (e) { return row(e, at); }));
    list.hidden = entries.length === 0;
    if (empty) empty.hidden = entries.length > 0;
  }

  // announce puts message in the list's live region.
  function announce(message) {
    util.setText(byId('request-list-status'), message);
  }

  window.goneRequestList = Object.freeze({ render: render, announce: announce, since: since, title: title });
})();
