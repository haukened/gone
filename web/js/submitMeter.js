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

  function renderBar(meter, size, maxBytes, problem) {
    if (!meter) return;
    meter.max = maxBytes || 1;
    meter.value = Math.min(size, meter.max);
    meter.classList.toggle('over', Boolean(problem));
  }

  function renderLabel(label, size, maxBytes) {
    if (!label) return;
    const over = maxBytes && size > maxBytes ? ' (over limit)' : '';
    label.textContent = `${formatBytes(size)} of ${formatBytes(maxBytes)}${over}`;
  }

  function renderWarning(warning, problem) {
    if (!warning) return;
    warning.textContent = problem;
    warning.hidden = !problem;
  }

  // render updates the optional meter/label/warning elements in els.
  function render(els, size, maxBytes, problem) {
    renderBar(els.meter, size, maxBytes, problem);
    renderLabel(els.label, size, maxBytes);
    renderWarning(els.warning, problem);
  }

  window.goneSizeMeter = Object.freeze({
    selectionProblem: selectionProblem,
    render: render
  });
})();
