'use strict';

const { reset, load, h, captureConsole, waitFor } = require('./harness');
const { FakeXHR } = require('./fakes');

const MODULES = ['util', 'crypto', 'fileMeta', 'envelope', 'icons', 'submitFiles', 'submitMeter', 'submitUpload', 'submitResult'];
const KDF_WAIT = 15000;
const PASS_MODULES = [...MODULES, 'wordlist', 'passgen', 'submitPassphrase'];

function optionalNode(skip, id, node) {
  return skip.has(id) ? [] : [node];
}

function passFields() {
  return [h('details', { id: 'pass-disclosure', open: false }, [
    h('input', { id: 'passphrase', type: 'password', value: '' }),
    h('button', { id: 'pass-toggle' }, [h('span', { textContent: 'Show' })]),
    h('button', { id: 'pass-generate' }),
    h('span', { id: 'pass-strength' })
  ])];
}

function formChildren(skip, opts, label) {
  return [
    ...optionalNode(skip, 'secret', h('textarea', { id: 'secret' })),
    ...optionalNode(skip, 'drop-zone', h('div', { id: 'drop-zone' }, optionalNode(skip, 'secret-files', h('input', { id: 'secret-files' })))),
    ...optionalNode(skip, 'file-list', h('ul', { id: 'file-list', hidden: true })),
    ...sizeControls(skip),
    ...(opts.pass ? passFields() : []),
    ...optionalNode(skip, 'button', h('button', { type: 'submit' }, label)),
    ...optionalNode(skip, 'upload-progress', h('progress', { id: 'upload-progress', hidden: true })),
    ...optionalNode(skip, 'submit-error', h('div', { id: 'submit-error', hidden: true }, optionalNode(skip, 'submit-error-content', h('p', { id: 'submit-error-content' }))))
  ];
}

function sizeControls(skip) {
  return [
    ...optionalNode(skip, 'size-box', h('div', { id: 'size-box' })),
    ...optionalNode(skip, 'size-meter', h('progress', { id: 'size-meter' })),
    ...optionalNode(skip, 'size-label', h('span', { id: 'size-label' })),
    ...optionalNode(skip, 'size-warning', h('div', { id: 'size-warning', hidden: true }, optionalNode(skip, 'size-warning-text', h('p', { id: 'size-warning-text' }))))
  ];
}

function resultView() {
  return h('div', { id: 'result', hidden: true }, [
    h('h1', { id: 'result-heading' }),
    h('input', { id: 'share-link' }),
    h('button', { id: 'copy-link' }, [h('span', { textContent: 'Copy link' })]),
    h('span', { id: 'copy-status' }),
    h('time', { id: 'result-expiry' }),
    h('details', { id: 'manage-disclosure', hidden: true }, [
      h('input', { id: 'manage-link' }),
      h('button', { id: 'copy-manage' }, [h('span', { textContent: 'Copy manage link' })]),
      h('span', { id: 'manage-copy-status' })
    ]),
    h('p', { id: 'result-pass-note', hidden: true })
  ]);
}

function page(env, opts) {
  const o = opts || {};
  const skip = new Set(o.skip || []);
  const label = o.noLabel ? [] : [h('span', { textContent: 'Create one-time link' })];
  const ttl = o.ttl === null ? null : { value: o.ttl || '1h' };
  const dataset = maxBytesDataset(o.maxBytes);
  const form = h('form', { id: 'create-secret', dataset, elements: { namedItem: (n) => (n === 'ttl' ? ttl : null) } }, formChildren(skip, o, label));
  env.document.body.append(h('div', { id: 'compose' }, optionalNode(skip, 'create-secret', form)), resultView());
}

function maxBytesDataset(maxBytes) {
  if (maxBytes === undefined) return { maxBytes: '1000' };
  return maxBytes === null ? {} : { maxBytes };
}

function boot(t, opts) {
  const o = opts || {};
  const env = reset(o.url || 'https://gone.test/');
  page(env, o);
  const logs = captureConsole(t);
  FakeXHR.reset();
  globalThis.XMLHttpRequest = FakeXHR;
  load(...(o.modules || MODULES), 'submit');
  const $ = (id) => env.document.getElementById(id);
  const btn = env.document.body.querySelector('button[type="submit"]');
  const blocked = () => btn.getAttribute('aria-disabled') === 'true';
  return { env, logs, $, btn, blocked, label: btn && btn.querySelector('span') };
}

const submit = (b) => b.$('create-secret').dispatch('submit');
const type = (b, text) => { b.$('secret').value = text; b.$('secret').dispatch('input'); };
const typePass = (b, text) => { b.$('passphrase').value = text; b.$('passphrase').dispatch('input'); };
const addFiles = (b, files) => { b.$('secret-files').files = files; b.$('secret-files').dispatch('change'); };

module.exports = { MODULES, KDF_WAIT, PASS_MODULES, boot, submit, type, typePass, addFiles, reset, load, waitFor };
