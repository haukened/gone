'use strict';

// The requester's page, /request: make a request, show its link, and list
// the requests saved in this browser. Making a request generates an ECDH key
// pair here; the private key goes straight into IndexedDB as a
// non-extractable CryptoKey and the public key into the link's fragment, so
// the server never sees either. Waiting requests are polled while the page
// is visible; status is read-only on the server. The page's DOM and state
// live in window.goneRequestDom, list polling in window.goneRequestSync and
// the link view in window.goneRequestCreated.
(function requestFlow() {
  const ctx = window.goneRequestDom;
  if (!ctx || !window.goneRequestSync || !window.goneRequestCreated) return;
  const util = ctx.util;
  const i18n = ctx.i18n;
  const v3 = ctx.v3;
  const store = ctx.store;
  const api = ctx.api;
  const form = ctx.form;
  const byId = ctx.byId;
  const btn = ctx.btn;
  const flags = ctx.flags;
  const page = ctx.page;
  const created = window.goneRequestCreated;
  const sync = window.goneRequestSync;

  const NO_CRYPTO = 'js.request.noCrypto';
  const SAVE_FAILED = 'js.request.saveFailed';
  const UNEXPECTED = 'js.request.unexpected';

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
    if (busy) i18n.set(ctx.btnLabel, 'js.request.makingKeys');
    else i18n.restore(ctx.btnLabel, ctx.idleLabel);
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

  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState !== 'visible' || !flags.ready) return;
    flags.ready = false;
    i18n.setTitle(page.titleKey);
  });

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
    const made = await api.create(selectedTTL());
    const entry = {
      id: made.id, label: label, createdAt: Date.now(), expiresAt: made.expiresAt.getTime(),
      manageToken: made.manageToken, replyLink: replyLink(made.id, keys.publicKey, made.fillToken),
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
      created.showCreated(await createRequest());
    } catch (e) {
      if (!api.isRequestError(e)) console.error('[gone] request failed', e);
      showError(api.isRequestError(e) ? e.message : UNEXPECTED);
    } finally {
      setBusy(false);
    }
  }

  async function start() {
    form.addEventListener('submit', onSubmit);
    created.wireCopy();
    if (!util.cryptoAvailable()) {
      block(NO_CRYPTO);
      return;
    }
    if (!(await store.available())) {
      byId('storage-alert').hidden = false;
      block('');
      return;
    }
    await sync.loadList();
  }

  start();
})();
