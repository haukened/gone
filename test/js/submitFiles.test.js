'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { reset, load, h } = require('./harness');

const mk = (name, text, lastModified) => new File([text], name, { type: 'text/plain', lastModified: lastModified || 1 });

// setup builds a form with input, drop zone and list, then creates a selection.
function setup(opts) {
  const env = reset();
  const input = h('input', { id: 'secret-files' });
  const dropZone = h('div', { id: 'drop-zone' });
  const listEl = h('ul', { id: 'file-list', hidden: true });
  const inner = h('span');
  const form = h('form', {}, [dropZone, input, listEl, inner]);
  env.document.body.appendChild(form);
  load('util', 'fileMeta', 'icons', 'submitFiles');
  const counts = { add: 0, change: 0 };
  const state = { busy: false };
  const sel = window.goneFileSelection.create(Object.assign({
    form, input, dropZone, listEl,
    isBusy: () => state.busy,
    onAdd: () => counts.add++,
    onChange: () => counts.change++
  }, opts));
  return { env, form, input, dropZone, listEl, inner, sel, counts, state };
}

test('requires util, fileMeta and icons; loads once', () => {
  reset();
  load('submitFiles');
  assert.equal(window.goneFileSelection, undefined);
  setup();
  const first = window.goneFileSelection;
  load('submitFiles');
  assert.equal(window.goneFileSelection, first);
});

test('add dedupes, renders sanitized entries and resets the input', () => {
  const { sel, input, listEl, counts } = setup();
  input.value = 'C:\\fakepath\\a.txt';
  sel.add([mk('dir/a.txt', 'AAA'), mk('b.txt', 'B')]);
  sel.add([mk('dir/a.txt', 'AAA'), mk('b.txt', 'B', 2)]);
  assert.equal(sel.count(), 3);
  assert.equal(input.value, '');
  assert.equal(counts.add, 2);
  assert.equal(counts.change, 2);
  assert.equal(listEl.hidden, false);
  const items = listEl.querySelectorAll('li');
  assert.equal(items.length, 3);
  assert.equal(items[0].querySelector('.name').textContent, 'a.txt');
  assert.equal(items[0].querySelector('.size').textContent, '3 B');
  assert.equal(items[0].children[0].getAttribute('class'), 'ico');
  assert.ok(items[0].querySelector('button').classList.contains('linkbtn'));
  assert.equal(items[0].querySelector('button').textContent, 'Remove');
  assert.equal(items[0].querySelector('button').getAttribute('aria-label'), 'Remove a.txt');
  assert.deepEqual(sel.metas(), [
    { name: 'dir/a.txt', type: 'text/plain', size: 3 },
    { name: 'b.txt', type: 'text/plain', size: 1 },
    { name: 'b.txt', type: 'text/plain', size: 1 }
  ]);
  const files = sel.files();
  files.pop();
  assert.equal(sel.count(), 3);
});

test('add ignores null lists and busy state', () => {
  const { sel, state, counts } = setup();
  sel.add(null);
  state.busy = true;
  sel.add([mk('a', 'x')]);
  assert.equal(sel.count(), 0);
  assert.equal(counts.add, 0);
});

test('remove buttons drop the entry and refocus the input unless busy', () => {
  const { env, sel, input, listEl, state } = setup();
  sel.add([mk('a', '1'), mk('b', '2')]);
  state.busy = true;
  listEl.querySelectorAll('button')[0].click();
  assert.equal(sel.count(), 2);
  state.busy = false;
  listEl.querySelectorAll('button')[0].click();
  assert.deepEqual(sel.metas().map((m) => m.name), ['b']);
  assert.equal(env.document.activeElement, input);
  sel.remove(0);
  assert.equal(listEl.hidden, true);
});

test('clear empties without rendering', () => {
  const { sel, counts, listEl } = setup();
  sel.add([mk('a', '1')]);
  sel.clear();
  assert.equal(sel.count(), 0);
  assert.equal(counts.change, 1);
  assert.equal(listEl.children.length, 1);
  sel.render();
  assert.equal(listEl.children.length, 0);
});

test('the input change event adds the chosen files', () => {
  const { sel, input } = setup();
  input.files = [mk('picked', 'x')];
  input.dispatch('change');
  assert.equal(sel.count(), 1);
});

test('drag and drop highlights the zone and adds dropped files', () => {
  const { sel, form, dropZone, inner } = setup();
  const files = { types: ['Files'] };
  for (const type of ['dragenter', 'dragover']) {
    const ev = form.dispatch(type, { dataTransfer: files });
    assert.equal(ev.defaultPrevented, true);
    assert.ok(dropZone.classList.contains('dragging'));
  }
  assert.equal(form.dispatch('dragover', { dataTransfer: { types: ['text/plain'] } }).defaultPrevented, false);
  assert.equal(form.dispatch('dragover', {}).defaultPrevented, false);
  assert.equal(form.dispatch('dragover', { dataTransfer: {} }).defaultPrevented, false);

  form.dispatch('dragleave', { relatedTarget: inner });
  assert.ok(dropZone.classList.contains('dragging'));
  form.dispatch('dragleave', { relatedTarget: null });
  assert.equal(dropZone.classList.contains('dragging'), false);
  form.dispatch('dragenter', { dataTransfer: files });
  form.dispatch('dragend', {});
  assert.equal(dropZone.classList.contains('dragging'), false);

  assert.equal(form.dispatch('drop', {}).defaultPrevented, false);
  assert.equal(form.dispatch('drop', { dataTransfer: { files: [] } }).defaultPrevented, false);
  const ev = form.dispatch('drop', { dataTransfer: { files: [mk('d', 'x')] } });
  assert.equal(ev.defaultPrevented, true);
  assert.equal(sel.count(), 1);
});

test('optional elements and callbacks may be omitted', () => {
  const env = reset();
  const form = h('form');
  env.document.body.appendChild(form);
  load('util', 'fileMeta', 'icons', 'submitFiles');
  const sel = window.goneFileSelection.create({ form });
  sel.add([mk('a', 'x')]);
  sel.remove(0);
  form.dispatch('dragenter', { dataTransfer: { types: ['Files'] } });
  assert.ok(form.classList.contains('dragging'));
  assert.equal(sel.count(), 0);
});
