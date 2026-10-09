'use strict';

// Size meter for the create form: validates the selection against the file
// count and server size limits and renders the meter, label, and warning.
// Text is set as goneI18n message keys. Exposed as window.goneSizeMeter.
(function sizeMeterModule() {
  if (window.goneSizeMeter || !window.goneI18n) return;
  const i18n = window.goneI18n;

  // selectionProblem returns why the selection cannot be sent, as
  // {key, args}, or '' when it is acceptable. maxBytes of 0 means no limit.
  function selectionProblem(count, size, maxFiles, maxBytes) {
    if (count > maxFiles) {
      return { key: 'js.meter.tooMany', args: { count: count - maxFiles, max: maxFiles } };
    }
    if (maxBytes && size > maxBytes) {
      return { key: 'js.meter.over', args: { size: { bytes: size - maxBytes } } };
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
    const key = maxBytes && size > maxBytes ? 'js.meter.sizeOver' : 'js.meter.size';
    i18n.set(label, key, { used: { bytes: size }, max: { bytes: maxBytes } });
  }

  // renderWarning shows problem, rewriting the live region only when the
  // problem changes so it isn't announced again on every keystroke.
  function renderWarning(els, problem) {
    const sig = problem ? problem.key + JSON.stringify(problem.args || {}) : '';
    if (els.warningText && els.warningText.dataset.problem !== sig) {
      els.warningText.dataset.problem = sig;
      if (problem) i18n.set(els.warningText, problem.key, problem.args);
      else i18n.clear(els.warningText);
    }
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
