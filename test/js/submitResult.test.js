'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h, captureConsole } = require('./harness');

const URL_ = 'https://gone.test/secret/abc#v1:KEY';
const EXPIRES = '2030-01-02T03:04:05Z';

function setup(t) {
  const env = reset();
  load('util', 'submitResult');
  return { env, logs: captureConsole(t), rp: window.goneResultPanel };
}

test('requires util; loads once', (t) => {
  reset();
  load('submitResult');
  assert.equal(window.goneResultPanel, undefined);
  const { rp } = setup(t);
  load('submitResult');
  assert.equal(window.goneResultPanel, rp);
});

test('show replaces the target and focuses the link start', (t) => {
  const { env, rp, logs } = setup(t);
  const target = h('section', { className: 'card' });
  env.document.body.appendChild(target);
  const panel = rp.show({ shareURL: URL_, expiresAt: EXPIRES, replaceTarget: target });
  assert.equal(env.document.body.children[0], panel);
  assert.equal(target.parentNode, null);
  const input = panel.querySelector('#share-link');
  assert.equal(input.value, URL_);
  assert.equal(input.readOnly, true);
  assert.equal(env.document.activeElement, input);
  assert.equal(input.selectionStart, 0);
  assert.equal(input.scrollLeft, 0);
  assert.equal(panel.querySelector('h2').textContent, 'Share This Link');
  assert.match(panel.querySelector('.security-warning-card').textContent, /exactly once/);
  assert.match(panel.querySelector('.security-warning-card').innerHTML, /<svg/);
  const time = panel.querySelector('time');
  assert.equal(time.getAttribute('datetime'), EXPIRES);
  assert.equal(time.textContent, new Date(EXPIRES).toLocaleString());
  assert.match(panel.querySelector('.hint').textContent, /^Expires at /);
  const back = panel.querySelector('a');
  assert.equal(back.href, '/');
  assert.match(back.innerHTML, /Create Another/);
  assert.deepEqual(logs.log, ['[gone] result panel shown']);
});

test('show appends to body without a target and can skip focus', (t) => {
  const { env, rp } = setup(t);
  const panel = rp.show({ shareURL: URL_, expiresAt: EXPIRES, focus: false });
  assert.equal(env.document.body.children[0], panel);
  assert.equal(env.document.activeElement, null);
});

test('focus survives a failing requestAnimationFrame', (t) => {
  const { env, rp } = setup(t);
  globalThis.requestAnimationFrame = () => { throw new Error('no raf'); };
  const panel = rp.show({ shareURL: URL_, expiresAt: EXPIRES });
  assert.equal(env.document.activeElement, panel.querySelector('#share-link'));
});

test('copy button copies the link, or selects it when copy fails', async (t) => {
  const { env, rp } = setup(t);
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const panel = rp.show({ shareURL: URL_, expiresAt: EXPIRES, focus: false });
  const copy = panel.querySelector('button');
  assert.equal(copy.getAttribute('aria-label'), 'Copy full share link');
  const ev = copy.click();
  assert.deepEqual(await ev.settled, [true]);
  assert.equal(env.clipboard.text, URL_);
  assert.match(copy.innerHTML, /^Copied!/);
  t.mock.timers.tick(2200);
  assert.match(copy.innerHTML, /^Copy Link/);

  env.clipboard.fail = true;
  assert.deepEqual(await copy.click().settled, [false]);
  const input = panel.querySelector('#share-link');
  assert.equal(env.document.activeElement, input);
  assert.equal(input.selected, true);
  assert.match(copy.innerHTML, /^Copy Link/);
});
