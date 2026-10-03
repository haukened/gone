'use strict';

// Fake manage page (web/manage.tmpl.html) for manageView and manage tests.

const { h } = require('./harness');

// btn builds a button with a <span> label.
function btn(id, label, props) {
  return h('button', Object.assign({ id }, props), [h('span', { textContent: label })]);
}

// managePage appends the four manage views to env's body. opts.skip omits
// ids (only leaf ids and whole views are supported).
function managePage(env, opts) {
  const skip = new Set((opts && opts.skip) || []);
  const add = (id, node) => (skip.has(id) ? [] : [node]);
  const view = (name, hidden, kids) => add(`view-${name}`, h('div', { id: `view-${name}`, hidden },
    [...add(`${name}-heading`, h('h1', { id: `${name}-heading` })), ...kids]));
  const check = view('check', false, [
    ...add('check-status', h('p', { id: 'check-status', textContent: 'Checking\u2026' })),
    ...add('check-error', h('div', { id: 'check-error', hidden: true }, add('check-error-text', h('p', { id: 'check-error-text' })))),
    ...add('check-retry', h('button', { id: 'check-retry', hidden: true, textContent: 'Try again' }))
  ]);
  const pending = view('pending', true, [
    ...['manage-created', 'manage-expires', 'manage-checked'].flatMap((id) => add(id, h('time', { id }))),
    ...add('check-again', btn('check-again', 'Check again')),
    ...add('pending-status', h('p', { id: 'pending-status' })),
    ...add('pending-error', h('div', { id: 'pending-error', hidden: true }, add('pending-error-text', h('p', { id: 'pending-error-text' })))),
    ...add('delete-start', h('div', { id: 'delete-start' }, add('delete-now', btn('delete-now', 'Delete now')))),
    ...add('delete-confirm', h('div', { id: 'delete-confirm', hidden: true }, [
      ...add('delete-confirm-text', h('p', { id: 'delete-confirm-text' })),
      ...add('delete-yes', btn('delete-yes', 'Delete it')),
      ...add('delete-no', h('button', { id: 'delete-no', textContent: 'Keep it' }))
    ]))
  ]);
  env.document.body.append(...check, ...pending, ...view('deleted', true, []), ...view('gone', true, []));
  return (id) => env.document.getElementById(id);
}

module.exports = { managePage };
