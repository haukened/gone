'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load } = require('./harness');
const { managePage } = require('./managePage');

const INFO = { createdAt: new Date('2030-01-01T00:00:00Z'), expiresAt: new Date('2030-01-02T00:00:00Z') };
const NOW = new Date('2030-01-01T00:30:00Z');

function setup(opts) {
  const env = reset('https://gone.test/manage/x');
  const $ = managePage(env, opts);
  load('util', 'manageView');
  return { env, $, view: window.goneManageView };
}

const visible = ($) => ['check', 'pending', 'deleted', 'gone'].filter((v) => $(`view-${v}`) && !$(`view-${v}`).hidden);

test('requires util; loads once; present reflects the page', () => {
  reset();
  load('manageView');
  assert.equal(window.goneManageView, undefined);
  const s = setup();
  load('manageView');
  assert.equal(window.goneManageView, s.view);
  assert.ok(Object.isFrozen(s.view));
  assert.equal(s.view.present, true);
  assert.equal(setup({ skip: ['view-pending'] }).view.present, false);
});

test('check view: checking, then errors with optional retry', () => {
  const { $, view } = setup();
  view.showCheckError('boom', true);
  assert.equal($('check-heading').textContent, 'Couldn\u2019t check your secret.');
  assert.equal($('check-status').textContent, '');
  assert.equal($('check-error-text').textContent, 'boom');
  assert.equal($('check-error').hidden, false);
  assert.equal($('check-retry').hidden, false);
  view.showChecking();
  assert.equal($('check-heading').textContent, 'Checking on your secret.');
  assert.equal($('check-status').textContent, 'Checking\u2026');
  assert.equal($('check-error').hidden, true);
  assert.equal($('check-retry').hidden, true);
  view.showCheckError('bad link', false);
  assert.equal($('check-retry').hidden, true);
});

test('showPending fills facts, switches view and focuses the heading', () => {
  const { env, $, view } = setup();
  view.showPending(INFO, NOW);
  assert.deepEqual(visible($), ['pending']);
  assert.equal(env.document.activeElement, $('pending-heading'));
  assert.equal(env.document.title, 'Gone \u00b7 Your secret is waiting');
  assert.equal($('manage-created').getAttribute('datetime'), INFO.createdAt.toISOString());
  assert.equal($('manage-expires').getAttribute('datetime'), INFO.expiresAt.toISOString());
  assert.equal($('manage-checked').getAttribute('datetime'), NOW.toISOString());
  assert.ok($('manage-created').textContent.length > 0);
  assert.equal($('pending-status').textContent, '');
});

test('a repeat showPending keeps focus and announces the result', () => {
  const { env, $, view } = setup();
  view.showPending(INFO, NOW);
  $('check-again').focus();
  view.showPending(INFO, new Date('2030-01-01T00:45:00Z'));
  assert.equal(env.document.activeElement, $('check-again'));
  assert.match($('pending-status').textContent, /^Still waiting as of .+\. Nobody has opened it\.$/);
});

test('setChecking and setDeleting toggle busy labels and buttons', () => {
  const { $, view } = setup();
  view.setChecking(true);
  assert.equal($('check-again').disabled, true);
  assert.equal($('delete-now').disabled, true);
  assert.equal($('check-again').textContent, 'Checking\u2026');
  assert.equal($('pending-status').textContent, 'Checking\u2026');
  view.setChecking(false);
  assert.equal($('check-again').disabled, false);
  assert.equal($('delete-now').disabled, false);
  assert.equal($('check-again').textContent, 'Check again');
  view.setDeleting(true);
  assert.equal($('delete-yes').disabled, true);
  assert.equal($('delete-no').disabled, true);
  assert.equal($('check-again').disabled, true);
  assert.equal($('delete-yes').textContent, 'Deleting\u2026');
  view.setDeleting(false);
  assert.equal($('delete-yes').disabled, false);
  assert.equal($('delete-yes').textContent, 'Delete it');
});

test('pending errors show and clear', () => {
  const { $, view } = setup();
  $('pending-status').textContent = 'old';
  view.showPendingError('nope');
  assert.equal($('pending-status').textContent, '');
  assert.equal($('pending-error-text').textContent, 'nope');
  assert.equal($('pending-error').hidden, false);
  view.clearPendingError();
  assert.equal($('pending-error').hidden, true);
});

test('confirm opens and closes with focus moves', () => {
  const { env, $, view } = setup();
  view.openConfirm();
  assert.equal($('delete-start').hidden, true);
  assert.equal($('delete-confirm').hidden, false);
  assert.equal(env.document.activeElement, $('delete-confirm-text'));
  view.closeConfirm();
  assert.equal($('delete-start').hidden, false);
  assert.equal($('delete-confirm').hidden, true);
  assert.equal(env.document.activeElement, $('delete-now'));
});

test('showDeleted and showGone switch views', () => {
  const { env, $, view } = setup();
  view.showDeleted();
  assert.deepEqual(visible($), ['deleted']);
  assert.equal(env.document.title, 'Gone \u00b7 Secret deleted');
  assert.equal(env.document.activeElement, $('deleted-heading'));
  view.showGone();
  assert.deepEqual(visible($), ['gone']);
  assert.equal(env.document.title, 'Gone \u00b7 This secret is gone');
});

test('bind wires controls and Escape cancels the confirm', () => {
  const { $, view } = setup();
  const seen = [];
  const rec = (name) => () => seen.push(name);
  view.bind({ retry: rec('retry'), check: rec('check'), ask: rec('ask'), confirm: rec('confirm'), cancel: rec('cancel') });
  $('check-retry').click();
  $('check-again').click();
  $('delete-now').click();
  $('delete-yes').click();
  $('delete-no').click();
  $('delete-confirm').dispatch('keydown', { key: 'Enter' });
  $('delete-confirm').dispatch('keydown', { key: 'Escape' });
  assert.deepEqual(seen, ['retry', 'check', 'ask', 'confirm', 'cancel', 'cancel']);
});

test('missing optional nodes are tolerated', () => {
  const ids = ['check-status', 'check-error', 'check-error-text', 'check-retry', 'manage-created', 'manage-expires',
    'manage-checked', 'check-again', 'pending-status', 'pending-error', 'pending-error-text', 'delete-start',
    'delete-now', 'delete-confirm', 'delete-confirm-text', 'delete-yes', 'delete-no', 'pending-heading', 'view-deleted'];
  const { env, view } = setup({ skip: ids });
  const fn = () => {};
  view.bind({ retry: fn, check: fn, ask: fn, confirm: fn, cancel: fn });
  view.showChecking();
  view.showCheckError('x', true);
  view.showPending(INFO, NOW);
  view.showPending(INFO, NOW);
  view.setChecking(true);
  view.setDeleting(true);
  view.showPendingError('x');
  view.clearPendingError();
  view.openConfirm();
  view.closeConfirm();
  view.showDeleted();
  assert.equal(env.document.title, 'Gone \u00b7 Secret deleted');
});

test('labels fall back to button text without a span', () => {
  const { $, view } = setup();
  $('delete-yes').replaceChildren();
  view.setDeleting(true);
  assert.equal($('delete-yes').textContent, 'Deleting\u2026');
});
