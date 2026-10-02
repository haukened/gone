'use strict';

// Size meter for the create form: validates the selection against the file
// count and server size limits and renders the meter, label, and warning.
// Requires window.goneFileMeta. Exposed as window.goneSizeMeter.
(function sizeMeterModule() {
  if (window.goneSizeMeter || !window.goneFileMeta) return;
  const formatBytes = window.goneFileMeta.formatBytes;

  // selectionProblem returns a user-facing reason the selection cannot be
  // sent, or '' when it is acceptable. maxBytes of 0 means no size limit.
  function selectionProblem(count, size, maxFiles, maxBytes) {
    if (count > maxFiles) {
      return `Too many files: remove ${count - maxFiles} to stay within ${maxFiles}.`;
    }
    if (maxBytes && size > maxBytes) {
      return `Over the limit by ${formatBytes(size - maxBytes)}. Remove a file or shorten the message.`;
    }
    return '';
  }

  function renderBar(els, size, maxBytes, problem) {
    if (els.meter) {
      els.meter.max = maxBytes || 1;
      els.meter.value = Math.min(size, els.meter.max);
    }
    if (els.box) els.box.classList.toggle('over', Boolean(problem));
  }

  function renderLabel(label, size, maxBytes) {
    if (!label) return;
    const over = maxBytes && size > maxBytes ? ' (over limit)' : '';
    label.textContent = `${formatBytes(size)} of ${formatBytes(maxBytes)}${over}`;
  }

  function renderWarning(els, problem) {
    if (els.warningText && els.warningText.textContent !== problem) els.warningText.textContent = problem;
    if (els.warning) els.warning.hidden = !problem;
  }

  // render updates the optional elements in els: {box, meter, label,
  // warning, warningText}. box gets the "over" class while problem is set.
  function render(els, size, maxBytes, problem) {
    renderBar(els, size, maxBytes, problem);
    renderLabel(els.label, size, maxBytes);
    renderWarning(els, problem);
  }

  window.goneSizeMeter = Object.freeze({
    selectionProblem: selectionProblem,
    render: render
  });
})();
