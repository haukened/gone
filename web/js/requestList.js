'use strict';

// Renders the requester's list of requests on /request: one row per request
// saved in this browser, with its label, age and state, linking to
// /request/{id}. Text is set with textContent or goneI18n only; a label the
// requester typed is never translated. Exposed as window.goneRequestList.
(function requestListModule() {
  if (window.goneRequestList || !window.goneUtil || !window.goneI18n) return;
  const util = window.goneUtil;
  const i18n = window.goneI18n;
  const STATES = { waiting: 'common.life.waiting', ready: 'js.request.stateReady' };
  const MINUTE = 60000;
  const HOUR = 60 * MINUTE;
  const DAY = 24 * HOUR;

  function byId(id) {
    return document.getElementById(id);
  }

  // since describes how long ago a timestamp was, in the largest whole
  // unit, as a message key and args.
  function since(ms, now) {
    const d = Math.max(0, now - ms);
    if (d < MINUTE) return { key: 'js.request.askedJustNow' };
    const unit = d < HOUR ? ['minute', MINUTE] : d < DAY ? ['hour', HOUR] : ['day', DAY];
    return { key: 'js.request.asked', args: { when: { rel: -Math.floor(d / unit[1]), unit: unit[0] } } };
  }

  // titleNode shows the requester's label, or "Untitled request".
  function titleNode(entry) {
    const span = util.el('span', { className: 'name' }, []);
    if (entry.label) i18n.plain(span, entry.label);
    else i18n.set(span, 'js.request.untitled');
    return span;
  }

  function pill(state) {
    const span = util.el('span', { className: 'pill-status' }, []);
    i18n.set(span, STATES[state] || STATES.waiting);
    return span;
  }

  function row(entry, now) {
    const state = entry.state === 'ready' ? 'ready' : 'waiting';
    const p = pill(state);
    p.dataset.state = state;
    const when = util.el('span', { className: 'when' }, []);
    const ago = since(entry.createdAt, now);
    i18n.set(when, ago.key, ago.args);
    const link = util.el('a', { className: 'request-row', href: `/request/${entry.id}` }, [titleNode(entry), when, p]);
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

  // announce puts a message key in the list's live region.
  function announce(key, args) {
    i18n.set(byId('request-list-status'), key, args);
  }

  window.goneRequestList = Object.freeze({ render: render, announce: announce, since: since });
})();
