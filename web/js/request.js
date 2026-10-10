'use strict';

// The requester's page, /request: make a request, show its link, and list
// the requests saved in this browser. Making a request generates an ECDH key
// pair here; the private key goes straight into IndexedDB as a
// non-extractable CryptoKey and the public key into the link's fragment, so
// the server never sees either. Waiting requests are polled while the page
// is visible; status is read-only on the server.
(function requestFlow() {
  const util = window.goneUtil;
  const i18n = window.goneI18n;
  const deps = [window.goneCryptoV3, window.goneRequestStore, window.goneRequestApi, window.goneRequestPoll, window.goneRequestList];
  const form = document.getElementById('create-request');
  if (!util || !i18n || !form || !util.allPresent(deps)) return;
  const v3 = window.goneCryptoV3;
  const store = window.goneRequestStore;
  const api = window.goneRequestApi;
  const list = window.goneRequestList;

  const MAX_POLLED = 5;
  const NO_CRYPTO = 'js.request.noCrypto';
  const SAVE_FAILED = 'js.request.saveFailed';
  const UNEXPECTED = 'js.request.unexpected';

  const byId = function (id) { return document.getElementById(id); };
  const btn = byId('create-request-btn');
  const btnLabel = btn ? btn.querySelector('span') : null;
  const idleLabel = i18n.snapshot(btnLabel);
  const flags = { busy: false, blocked: false, ready: false };
  const page = { entries: [], titleKey: 'request.pageTitle', listPoll: null, created: false };

  // showError shows an error message key, or hides the error when key is ''.
  function showError(key) {
    if (key) i18n.set(byId('request-error-text'), key);
    else i18n.clear(byId('request-error-text'));
    byId('request-error').hidden = !key;
  }

  function setBusy(busy) {
    flags.busy = busy;
    btn.setAttribute('aria-disabled', String(busy || flags.blocked));
    if (busy) btn.setAttribute('aria-busy', 'true');
    else btn.removeAttribute('aria-busy');
    if (busy) i18n.set(btnLabel, 'js.request.makingKeys');
    else i18n.restore(btnLabel, idleLabel);
  }

  function block(hint) {
    flags.blocked = true;
    btn.setAttribute('aria-disabled', 'true');
    if (hint) i18n.set(byId('request-hint'), hint);
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
    if (document.visibilityState !== 'hidden' || flags.ready) return;
    flags.ready = true;
    i18n.setTitle('js.request.titleReady', { title: i18n.t(page.titleKey) });
  }

  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState !== 'visible' || !flags.ready) return;
    flags.ready = false;
    i18n.setTitle(page.titleKey);
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
      list.announce('js.request.arrived', { count: arrived });
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
    i18n.value(expiry, { date: when.toISOString(), style: 'datetime' });
    expiry.setAttribute('datetime', when.toISOString());
    byId('view-compose').hidden = true;
    byId('view-created').hidden = false;
    page.titleKey = 'js.request.titleCreated';
    i18n.setTitle(page.titleKey);
    byId('created-heading').focus();
    window.goneRequestPoll.create(function () { return checkCreated(entry); }).start();
  }

  async function checkCreated(entry) {
    const state = await refresh(entry);
    if (state === 'waiting') return 'continue';
    const ready = state === 'ready';
    const pillEl = byId('created-state');
    pillEl.dataset.state = ready ? 'ready' : 'gone';
    i18n.set(pillEl, ready ? 'js.request.stateReady' : 'common.life.gone');
    i18n.set(byId('created-status'), ready ? 'js.request.arrivedOpen' : 'js.request.gone');
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
    i18n.set(byId('created-open'), 'js.request.openReply');
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
      if (ok) util.flashCopied(copy, status, 'js.request.linkCopied');
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
