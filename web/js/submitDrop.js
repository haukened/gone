'use strict';

// Drag-and-drop and duplicate filtering for the create form's file
// selection (submitFiles.js). Exposed as window.goneSubmitDrop.
(function submitDropModule() {
  if (window.goneSubmitDrop) return;

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

  // addUnique appends each file in list to selected unless the same file
  // (name, size and modification time) is already there.
  function addUnique(selected, list) {
    Array.from(list).forEach(function (f) {
      if (!selected.some(function (s) { return sameFile(s, f); })) selected.push(f);
    });
  }

  window.goneSubmitDrop = Object.freeze({ setupDropZone: setupDropZone, addUnique: addUnique });
})();
