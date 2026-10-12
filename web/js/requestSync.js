'use strict';

// Keeps the requester's saved requests in step with the server on /request:
// refreshes entries, re-renders the list, and polls while any still wait.
// Status is read-only on the server. Exposed as window.goneRequestSync.
(function requestSyncModule() {
  if (window.goneRequestSync || !window.goneRequestDom) return;
  const ctx = window.goneRequestDom;
  const store = ctx.store;
  const api = ctx.api;
  const list = ctx.list;
  const page = ctx.page;

  const MAX_POLLED = 5;

  // refresh asks the server about one entry and saves what changed.
  // Returns the entry's new state: 'waiting', 'ready' or 'gone'.
  async function refresh(entry) {
    const st = await api.status(entry.id, entry.manageToken);
    if (st.state === 'gone') {
      await store.remove(entry.id);
      return 'gone';
    }
    if (entry.state !== st.state || entry.expiresAt !== st.expiresAt.getTime()) {
      entry.state = st.state;
      entry.expiresAt = st.expiresAt.getTime();
      await store.put(entry);
    }
    return st.state;
  }

  // checkList refreshes the most recent waiting requests, re-renders, and
  // tells the poller whether anything is still waiting.
  async function checkList() {
    const states = await Promise.all(page.entries.filter(isWaiting).slice(0, MAX_POLLED).map(refresh));
    const arrived = states.filter(function (s) { return s === 'ready'; }).length;
    page.entries = await store.list();
    list.render(page.entries);
    if (arrived) {
      list.announce('js.request.arrived', { count: arrived });
      ctx.flagReady();
    }
    return page.entries.some(isWaiting) ? 'continue' : 'stop';
  }

  function isWaiting(entry) {
    return entry.state !== 'ready';
  }

  async function loadList() {
    try {
      page.entries = await store.list();
    } catch {
      page.entries = [];
    }
    if (page.created) return;
    list.render(page.entries);
    if (!page.entries.some(isWaiting)) return;
    page.listPoll = window.goneRequestPoll.create(checkList);
    page.listPoll.start();
  }

  window.goneRequestSync = Object.freeze({ refresh: refresh, loadList: loadList });
})();
