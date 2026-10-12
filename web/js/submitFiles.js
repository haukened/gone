'use strict';

// File selection for the create form: picker/drag-and-drop input, duplicate
// filtering, and the removable file list. Requires window.goneUtil,
// window.goneI18n, window.goneFileMeta, window.goneIcons and
// window.goneSubmitDrop (submitDrop.js). Exposed as window.goneFileSelection.
(function fileSelectionModule() {
  if (window.goneFileSelection || !window.goneUtil || !window.goneI18n || !window.goneFileMeta || !window.goneIcons || !window.goneSubmitDrop) return;
  const util = window.goneUtil;
  const i18n = window.goneI18n;
  const meta = window.goneFileMeta;
  const icons = window.goneIcons;
  const drop = window.goneSubmitDrop;

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
      drop.addUnique(selected, list);
      if (input) input.value = '';
      onAdd();
      render();
    }

    if (input) input.addEventListener('change', function () { add(input.files); });
    drop.setupDropZone(form, opts.dropZone || form, add);

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
