'use strict';

// The link view of the requester's page, /request: shows a newly made
// request's reply link, polls it, and marks it replied once a reply
// arrives. Exposed as window.goneRequestCreated.
(function requestCreatedModule() {
  if (window.goneRequestCreated || !window.goneRequestDom || !window.goneRequestSync) return;
  const ctx = window.goneRequestDom;
  const util = ctx.util;
  const i18n = ctx.i18n;
  const byId = ctx.byId;
  const page = ctx.page;
  const refresh = window.goneRequestSync.refresh;

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
    ctx.flagReady();
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

  window.goneRequestCreated = Object.freeze({ showCreated: showCreated, wireCopy: wireCopy });
})();
