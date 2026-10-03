'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h } = require('./harness');

// page builds the compose and result views; omit lists ids to leave out.
function page(omit) {
  const skip = new Set(omit || []);
  const env = reset();
  const make = (id, tag, props, kids) => (skip.has(id) ? null : h(tag, Object.assign({ id }, props), kids));
  const nodes = {
    compose: make('compose', 'div'),
    view: make('result', 'div', { hidden: true }),
    heading: make('result-heading', 'h1'),
    input: make('share-link', 'input'),
    label: h('span', { textContent: 'Copy link' }),
    status: make('copy-status', 'span'),
    expiry: make('result-expiry', 'time'),
    manageInput: make('manage-link', 'input'),
    manageLabel: h('span', { textContent: 'Copy manage link' }),
    manageStatus: make('manage-copy-status', 'span')
  };
  nodes.btn = make('copy-link', 'button', {}, [h('svg'), nodes.label]);
  nodes.manageBtn = make('copy-manage', 'button', {}, [nodes.manageLabel]);
  nodes.manage = make('manage-disclosure', 'details', { hidden: true, open: true },
    [nodes.manageInput, nodes.manageBtn, nodes.manageStatus].filter(Boolean));
  const kids = [nodes.heading, nodes.input, nodes.btn, nodes.status, nodes.expiry, nodes.manage].filter(Boolean);
  if (nodes.view) nodes.view.append(...kids);
  env.document.body.append(...[nodes.compose, nodes.view].filter(Boolean));
  load('util', 'submitResult');
  return Object.assign({ env, panel: window.goneResultPanel }, nodes);
}

test('requires util; loads once', () => {
  reset();
  load('submitResult');
  assert.equal(window.goneResultPanel, undefined);
  const p = page();
  load('submitResult');
  assert.equal(window.goneResultPanel, p.panel);
  assert.ok(Object.isFrozen(p.panel));
});

test('show fills the link and expiry, swaps views and focuses the heading', () => {
  const p = page();
  const at = '2030-01-02T03:04:05Z';
  const view = p.panel.show({ shareURL: 'https://gone.test/secret/x#k', expiresAt: at });
  assert.equal(view, p.view);
  assert.equal(p.input.value, 'https://gone.test/secret/x#k');
  assert.equal(p.expiry.getAttribute('datetime'), new Date(at).toISOString());
  assert.ok(p.expiry.textContent.length > 0);
  assert.equal(p.compose.hidden, true);
  assert.equal(p.view.hidden, false);
  assert.equal(p.env.document.title, 'Gone \u00b7 Your link is ready');
  assert.equal(p.env.document.activeElement, p.heading);
});

test('focus:false and missing optional nodes are tolerated', () => {
  const p = page(['compose', 'result-heading', 'result-expiry', 'copy-status']);
  assert.equal(p.panel.show({ shareURL: 'u', expiresAt: 0, focus: false }), p.view);
  assert.equal(p.env.document.activeElement, null);
  const q = page();
  q.panel.show({ shareURL: 'u', expiresAt: 0, focus: false });
  assert.equal(q.env.document.activeElement, null);
});

test('show returns null when required nodes are missing', () => {
  for (const id of ['result', 'share-link', 'copy-link']) {
    const p = page([id]);
    assert.equal(p.panel.show({ shareURL: 'u', expiresAt: 0 }), null, id);
  }
});

test('copy button copies and announces, or selects the link on failure', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const p = page();
  p.panel.show({ shareURL: 'https://gone.test/s#k', expiresAt: 0 });
  p.panel.show({ shareURL: 'https://gone.test/s#k2', expiresAt: 0 });
  await p.btn.click().settled;
  assert.equal(p.env.clipboard.text, 'https://gone.test/s#k2');
  assert.equal(p.label.textContent, 'Copied');
  assert.equal(p.status.textContent, 'Link copied to clipboard.');
  t.mock.timers.tick(2200);
  p.env.clipboard.fail = true;
  await p.btn.click().settled;
  assert.equal(p.input.selected, true);
  assert.equal(p.env.document.activeElement, p.input);
  assert.match(p.status.textContent, /press Ctrl\+C/);
  assert.equal(p.label.textContent, 'Copy link');
});

test('manage section shows a collapsed manage link with its own copy button', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const p = page();
  const url = 'https://gone.test/manage/x#t';
  p.panel.show({ shareURL: 'u', manageURL: url, expiresAt: 0 });
  p.panel.show({ shareURL: 'u', manageURL: url + '2', expiresAt: 0 });
  assert.equal(p.manage.hidden, false);
  assert.equal(p.manage.open, false);
  assert.equal(p.manageInput.value, url + '2');
  await p.manageBtn.click().settled;
  assert.equal(p.env.clipboard.text, url + '2');
  assert.equal(p.manageLabel.textContent, 'Copied');
  assert.equal(p.manageStatus.textContent, 'Manage link copied to clipboard.');
  assert.equal(p.status.textContent, '');
  t.mock.timers.tick(2200);
  p.env.clipboard.fail = true;
  await p.manageBtn.click().settled;
  assert.equal(p.manageInput.selected, true);
  assert.match(p.manageStatus.textContent, /press Ctrl\+C/);
});

test('manage section hides without a manage link and tolerates missing nodes', () => {
  const p = page();
  p.panel.show({ shareURL: 'u', manageURL: 'https://gone.test/manage/x#t', expiresAt: 0 });
  p.panel.show({ shareURL: 'u', expiresAt: 0 });
  assert.equal(p.manage.hidden, true);
  assert.equal(p.manageInput.value, '');
  for (const id of ['manage-disclosure', 'manage-link', 'copy-manage', 'manage-copy-status']) {
    const q = page([id]);
    assert.equal(q.panel.show({ shareURL: 'u', manageURL: 'https://gone.test/manage/x#t', expiresAt: 0 }), q.view, id);
  }
});
