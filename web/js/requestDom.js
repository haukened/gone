'use strict';

// Shared DOM lookup, dependency checks and page state for the requester's
// page, /request. The request flow modules read and update this one
// context. Exposed as window.goneRequestDom.
(function requestDomModule() {
  if (window.goneRequestDom) return;
  const util = window.goneUtil;
  const i18n = window.goneI18n;
  const deps = [window.goneCryptoV3, window.goneRequestStore, window.goneRequestApi, window.goneRequestPoll, window.goneRequestList];
  const form = document.getElementById('create-request');
  if (!util || !i18n || !form || !util.allPresent(deps)) return;

  const byId = function (id) { return document.getElementById(id); };
  const btn = byId('create-request-btn');
  const btnLabel = btn ? btn.querySelector('span') : null;
  const flags = { busy: false, blocked: false, ready: false };
  const page = { entries: [], titleKey: 'request.pageTitle', listPoll: null, created: false };

  // flagReady marks the tab title when a reply arrives while it is hidden.
  function flagReady() {
    if (document.visibilityState !== 'hidden' || flags.ready) return;
    flags.ready = true;
    i18n.setTitle('js.request.titleReady', { title: i18n.t(page.titleKey) });
  }

  window.goneRequestDom = {
    util: util,
    i18n: i18n,
    v3: window.goneCryptoV3,
    store: window.goneRequestStore,
    api: window.goneRequestApi,
    list: window.goneRequestList,
    form: form,
    byId: byId,
    btn: btn,
    btnLabel: btnLabel,
    idleLabel: i18n.snapshot(btnLabel),
    flags: flags,
    page: page,
    flagReady: flagReady
  };
})();
