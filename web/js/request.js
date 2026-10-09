'use strict';

// The requester's page, /request: make a request, show its link, and list
// the requests saved in this browser. Making a request generates an ECDH key
// pair here; the private key goes straight into IndexedDB as a
// non-extractable CryptoKey and the public key into the link's fragment, so
// the server never sees either. Waiting requests are polled while the page
// is visible; status is read-only on the server.
(function requestFlow() {
  const util = window.goneUtil;
  const deps = [window.goneCryptoV3, window.goneRequestStore, window.goneRequestApi, window.goneRequestPoll, window.goneRequestList];
  const form = document.getElementById('create-request');
  if (!util || !form || !util.allPresent(deps)) return;
  const v3 = window.goneCryptoV3;
  const store = window.goneRequestStore;
  const api = window.goneRequestApi;
  const list = window.goneRequestList;

  const MAX_POLLED = 5;
  const NO_CRYPTO = 'Turned off: this page isn\u2019t HTTPS, so this browser can\u2019t make keys.';
  const SAVE_FAILED = 'This browser couldn\u2019t save the request\u2019s key, so it was cancelled. Try a normal (not private) window.';
  const UNEXPECTED = 'Something went wrong making the request. Try again.';
  const READY_TITLE = '\u25cf Reply ready \u00b7 ';

  const byId = function (id) { return document.getElementById(id); };
  const btn = byId('create-request-btn');
  const btnLabel = btn ? btn.querySelector('span') : null;
  const idleLabel = btnLabel ? btnLabel.textContent : '';
  const flags = { busy: false, blocked: false };
  const page = { entries: [], title: document.title, listPoll: null, created: false };

  function showError(msg) {
    util.setText(byId('request-error-text'), msg);
    byId('request-error').hidden = !msg;
  }

  function setBusy(busy) {
    flags.busy = busy;
    btn.setAttribute('aria-disabled', String(busy || flags.blocked));
    if (busy) btn.setAttribute('aria-busy', 'true');
    else btn.removeAttribute('aria-busy');
    util.setText(btnLabel, busy ? 'Making keys\u2026' : idleLabel);
  }

  function block(hint) {
    flags.blocked = true;
    btn.setAttribute('aria-disabled', 'true');
    if (hint) util.setText(byId('request-hint'), hint);
  }

  function selectedTTL() {
    const ttl = form.elements.namedItem('ttl');
    return ttl ? ttl.value : '';
  }

  function replyLink(id, publicKey, fill) {
    return `${location.origin}/reply/${id}#${v3.replyFragment(publicKey, fill)}`;
  }

  // flagReady marks the tab title when a reply arrives while it is hidden.
  function flagReady() {
    if (document.visibilityState === 'hidden' && !document.title.startsWith(READY_TITLE)) document.title = READY_TITLE + page.title;
  }

  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState === 'visible') document.title = page.title;
  });

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
      list.announce(arrived === 1 ? 'A reply arrived.' : `${arrived} replies arrived.`);
      flagReady();
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

  // showCreated swaps in the link view and polls the new request; the
  // hidden list stops polling.
  function showCreated(entry) {
    page.created = true;
    if (page.listPoll) page.listPoll.stop();
    byId('reply-link').value = entry.replyLink;
    byId('created-open').href = `/request/${entry.id}`;
    const expiry = byId('created-expiry');
    const when = new Date(entry.expiresAt);
    expiry.textContent = when.toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
    expiry.setAttribute('datetime', when.toISOString());
    byId('view-compose').hidden = true;
    byId('view-created').hidden = false;
    document.title = page.title = 'Gone \u00b7 Request link ready';
    byId('created-heading').focus();
    window.goneRequestPoll.create(function () { return checkCreated(entry); }).start();
  }

  async function checkCreated(entry) {
    const state = await refresh(entry);
    if (state === 'waiting') return 'continue';
    const ready = state === 'ready';
    const pillEl = byId('created-state');
    pillEl.dataset.state = ready ? 'ready' : 'gone';
    util.setText(pillEl, ready ? 'Reply ready' : 'Gone');
    util.setText(byId('created-status'), ready ? 'A reply arrived. Open it from this request.' : 'This request is gone.');
    if (ready) markReplied();
    return 'stop';
  }

  // markReplied advances the rail and promotes the open link once a reply
  // has arrived.
  function markReplied() {
    const waitingStep = byId('created-step-waiting');
    const repliedStep = byId('created-step-replied');
    waitingStep.classList.remove('is-now');
    waitingStep.classList.add('is-done');
    waitingStep.removeAttribute('aria-current');
    repliedStep.classList.add('is-now');
    repliedStep.setAttribute('aria-current', 'step');
    byId('created-open').className = 'btn btn-primary';
    util.setText(byId('created-open'), 'Open the reply');
    flagReady();
  }

  // save keeps the new request in this browser; if that fails the request
  // is cancelled, since nothing could ever open its reply.
  async function save(entry) {
    try {
      await store.put(entry);
    } catch (e) {
      console.error('[gone] could not save request', e);
      await api.cancel(entry.id, entry.manageToken).catch(function () {});
      throw api.RequestError(SAVE_FAILED, false);
    }
    store.persist();
  }

  async function createRequest() {
    const label = byId('request-label').value.trim().slice(0, 80);
    const keys = await v3.generateKeyPair();
    const created = await api.create(selectedTTL());
    const entry = {
      id: created.id, label: label, createdAt: Date.now(), expiresAt: created.expiresAt.getTime(),
      manageToken: created.manageToken, replyLink: replyLink(created.id, keys.publicKey, created.fillToken),
      publicKey: keys.publicKey, privateKey: keys.privateKey, state: 'waiting'
    };
    await save(entry);
    return entry;
  }

  async function onSubmit(ev) {
    ev.preventDefault();
    if (flags.busy || flags.blocked) return;
    showError('');
    setBusy(true);
    try {
      showCreated(await createRequest());
    } catch (e) {
      if (!api.isRequestError(e)) console.error('[gone] request failed', e);
      showError(api.isRequestError(e) ? e.message : UNEXPECTED);
    } finally {
      setBusy(false);
    }
  }

  function wireCopy() {
    const copy = byId('copy-reply');
    const input = byId('reply-link');
    copy.addEventListener('click', async function () {
      const status = byId('copy-status');
      const ok = await util.copyText(input.value, function () { input.focus(); input.select(); }, status);
      if (ok) util.flashCopied(copy, status, 'Request link copied to clipboard.');
    });
  }

  async function start() {
    form.addEventListener('submit', onSubmit);
    wireCopy();
    if (!util.cryptoAvailable()) {
      block(NO_CRYPTO);
      return;
    }
    if (!(await store.available())) {
      byId('storage-alert').hidden = false;
      block('');
      return;
    }
    await loadList();
  }

  start();
})();
