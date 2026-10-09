'use strict';

// Fake request pages (web/request.tmpl.html, web/request-detail.tmpl.html)
// for request.js, requestDetailView.js and requestDetail.js tests.

const { h } = require('./harness');

function btn(id, label, props) {
  return h('button', Object.assign({ id }, props), [h('span', { textContent: label })]);
}

function alertBox(id, textId) {
  return h('div', { id, hidden: true }, [h('p', { id: textId })]);
}

// requestPage builds /request. ttl is the selected TTL value.
function requestPage(env, ttl) {
  const radio = { value: ttl || '1h' };
  const form = h('form', { id: 'create-request', elements: { namedItem: (n) => (n === 'ttl' ? radio : null) } }, [
    h('input', { id: 'request-label', value: '' }),
    btn('create-request-btn', 'Create request link'),
    h('span', { id: 'request-hint' }),
    h('div', { id: 'storage-alert', hidden: true }),
    alertBox('request-error', 'request-error-text')
  ]);
  const compose = h('div', { id: 'view-compose' }, [
    form,
    h('ul', { id: 'request-list', hidden: true }),
    h('p', { id: 'request-empty' }),
    h('span', { id: 'request-list-status' })
  ]);
  const created = h('div', { id: 'view-created', hidden: true }, [
    h('h1', { id: 'created-heading' }),
    h('input', { id: 'reply-link' }),
    btn('copy-reply', 'Copy link'),
    h('span', { id: 'copy-status' }),
    h('span', { id: 'created-state', dataset: { state: 'waiting' } }),
    h('span', { id: 'created-status' }),
    h('a', { id: 'created-open', href: '/request', className: 'btn btn-secondary' }),
    h('time', { id: 'created-expiry' }),
    h('li', { id: 'created-step-waiting', className: 'is-now' }),
    h('li', { id: 'created-step-replied' })
  ]);
  env.document.body.append(compose, created);
  return (id) => env.document.getElementById(id);
}

function view(name, hidden, kids) {
  return h('div', { id: `view-${name}`, hidden }, [h('h1', { id: `${name}-heading` }), ...kids]);
}

// detailPage builds /request/{id}, including the receive views consumeView
// drives.
function detailPage(env) {
  const check = view('check', false, [
    h('p', { id: 'check-status', textContent: 'Checking…' }),
    alertBox('check-error', 'check-error-text'),
    h('button', { id: 'check-retry', hidden: true, textContent: 'Try again' })
  ]);
  const waiting = view('waiting', true, [
    h('span', { id: 'waiting-label' }),
    ...['request-created', 'request-expires', 'request-checked'].map((id) => h('time', { id })),
    h('input', { id: 'waiting-link' }),
    btn('copy-waiting-link', 'Copy link'),
    h('span', { id: 'waiting-copy-status' }),
    btn('check-again', 'Check again'),
    h('span', { id: 'waiting-status' }),
    alertBox('waiting-error', 'waiting-error-text'),
    h('div', { id: 'cancel-start' }, [btn('cancel-now', 'Cancel request')]),
    h('div', { id: 'cancel-confirm', hidden: true }, [
      h('p', { id: 'cancel-confirm-text' }),
      btn('cancel-yes', 'Cancel it'),
      h('button', { id: 'cancel-no', textContent: 'Keep waiting' })
    ])
  ]);
  const open = view('open', true, [
    h('span', { id: 'open-label' }),
    btn('open-secret', 'Open reply'),
    h('progress', { id: 'download-progress', hidden: true }),
    h('span', { id: 'consume-status' }),
    h('time', { id: 'open-expires' }),
    alertBox('consume-error', 'consume-error-text')
  ]);
  const revealed = view('revealed', true, [
    h('div', { id: 'ack-warning', hidden: true }),
    h('div', { id: 'message-panel', hidden: true }, [
      btn('show-secret', 'Show'),
      btn('copy-secret', 'Copy'),
      h('span', { id: 'copy-status' }),
      h('div', { id: 'secret-cover' }),
      h('pre', { id: 'secret-cover-dots' }),
      h('span', { id: 'secret-size' }),
      h('div', { id: 'secret-output', hidden: true }),
      h('input', { id: 'always-show', type: 'checkbox' })
    ]),
    h('div', { id: 'file-section', hidden: true }, [h('ul', { id: 'file-output-list' }), h('button', { id: 'download-all', hidden: true })])
  ]);
  env.document.body.append(check, view('missing', true, []), waiting, view('cancelled', true, []), open, revealed, view('gone', true, []));
  return (id) => env.document.getElementById(id);
}

module.exports = { requestPage, detailPage };
