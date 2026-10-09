'use strict';

// File selection for the create form: picker/drag-and-drop input, duplicate
// filtering, and the removable file list. Requires window.goneUtil,
// window.goneI18n, window.goneFileMeta and window.goneIcons. Exposed as
// window.goneFileSelection.
(function fileSelectionModule() {
  if (window.goneFileSelection || !window.goneUtil || !window.goneI18n || !window.goneFileMeta || !window.goneIcons) return;
  const util = window.goneUtil;
  const i18n = window.goneI18n;
  const meta = window.goneFileMeta;
  const icons = window.goneIcons;

  function sameFile(a, b) {
    return a.name === b.name && a.size === b.size && a.lastModified === b.lastModified;
  }

  function hasFiles(ev) {
    return Boolean(ev.dataTransfer) && Array.from(ev.dataTransfer.types || []).includes('Files');
  }

  // setupDropZone accepts files dragged onto form, highlighting target while
  // a file drag is over it and passing dropped files to add.
  function setupDropZone(form, target, add) {
    ['dragenter', 'dragover'].forEach(function (type) {
      form.addEventListener(type, function (ev) {
        if (!hasFiles(ev)) return;
        ev.preventDefault();
        target.classList.add('dragging');
      });
    });
    ['dragleave', 'dragend'].forEach(function (type) {
      form.addEventListener(type, function (ev) {
        if (ev.relatedTarget && form.contains(ev.relatedTarget)) return;
        target.classList.remove('dragging');
      });
    });
    form.addEventListener('drop', function (ev) {
      target.classList.remove('dragging');
      if (!ev.dataTransfer || !ev.dataTransfer.files.length) return;
      ev.preventDefault();
      add(ev.dataTransfer.files);
    });
  }

  function noop() {}

  // create wires a file selection to the given elements.
  //
  // opts: {form, input, dropZone, listEl, isBusy(), onAdd(), onChange()}.
  // Returns {files(), metas(), count(), add(list), remove(i), clear(), render()}.
  function create(opts) {
    const form = opts.form;
    const input = opts.input;
    const listEl = opts.listEl;
    const isBusy = opts.isBusy || function () { return false; };
    const onAdd = opts.onAdd || noop;
    const onChange = opts.onChange || noop;
    let selected = [];

    function renderItem(file, index) {
      const name = meta.sanitizeFileName(file.name);
      const remove = util.el('button', { type: 'button', className: 'linkbtn' });
      i18n.set(remove, 'js.files.remove');
      i18n.setAttr(remove, 'aria-label', 'js.files.removeNamed', { name: name });
      remove.addEventListener('click', function () { removeAt(index); });
      const size = util.el('span', { className: 'size' });
      i18n.value(size, { bytes: file.size });
      return util.el('li', {}, [
        icons.make('clip'),
        util.el('span', { className: 'name', textContent: name }),
        size,
        remove
      ]);
    }

    function render() {
      if (listEl) {
        listEl.replaceChildren(...selected.map(renderItem));
        listEl.hidden = selected.length === 0;
      }
      onChange();
    }

    function removeAt(index) {
      if (isBusy()) return;
      selected.splice(index, 1);
      render();
      if (input) input.focus();
    }

    function add(list) {
      if (isBusy() || !list) return;
      Array.from(list).forEach(function (f) {
        if (!selected.some(function (s) { return sameFile(s, f); })) selected.push(f);
      });
      if (input) input.value = '';
      onAdd();
      render();
    }

    if (input) input.addEventListener('change', function () { add(input.files); });
    setupDropZone(form, opts.dropZone || form, add);

    return {
      files: function () { return selected.slice(); },
      metas: function () {
        return selected.map(function (f) { return { name: f.name, type: f.type, size: f.size }; });
      },
      count: function () { return selected.length; },
      add: add,
      remove: removeAt,
      clear: function () { selected = []; },
      render: render
    };
  }

  window.goneFileSelection = Object.freeze({ create: create });
})();
