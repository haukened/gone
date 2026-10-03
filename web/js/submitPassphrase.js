'use strict';

// Optional passphrase field on the create form: show/hide toggle, Generate
// button, live strength hint and length validation. Requires
// window.goneUtil, window.goneCrypto and window.gonePassgen. Exposed as
// window.gonePassphraseField.
(function passphraseFieldModule() {
  if (window.gonePassphraseField || !window.goneUtil || !window.goneCrypto || !window.gonePassgen) return;
  const util = window.goneUtil;
  const passgen = window.gonePassgen;

  const STRENGTH_TEXT = new Map([
    ['empty', ''],
    ['short', `Too short: use at least ${passgen.minChars} characters.`],
    ['weak', 'Strength: weak. Longer is better, or press Generate.'],
    ['fair', 'Strength: fair.'],
    ['strong', 'Strength: strong.']
  ]);
  const SHORT_PROBLEM = `Make the passphrase at least ${passgen.minChars} characters, or leave it empty.`;

  // create wires the passphrase controls.
  //
  // els: {disclosure, input, toggle, generate, strength}; onChange is called
  // after every edit. Returns null when the input is missing, otherwise
  // {value, problem, overhead, setBusy, clear, reveal}.
  function create(els, onChange) {
    const input = els.input;
    if (!input) return null;
    const toggleLabel = els.toggle ? els.toggle.querySelector('span') : null;

    function value() {
      return input.value;
    }

    // problem returns a user-facing message when a non-empty passphrase is too short.
    function problem() {
      return passgen.strength(input.value) === 'short' ? SHORT_PROBLEM : '';
    }

    // overhead returns the extra ciphertext bytes a passphrase adds (the v2 header).
    function overhead() {
      return input.value ? window.goneCrypto.v2HeaderBytes : 0;
    }

    function setShown(shown) {
      input.type = shown ? 'text' : 'password';
      util.setText(toggleLabel, shown ? 'Hide' : 'Show');
    }

    function render() {
      const level = passgen.strength(input.value);
      if (els.strength) {
        els.strength.dataset.level = level;
        util.setText(els.strength, STRENGTH_TEXT.get(level));
      }
      input.setAttribute('aria-invalid', String(level === 'short'));
    }

    function changed() {
      render();
      onChange();
    }

    // reveal opens the disclosure and focuses the field, e.g. after a failed submit.
    function reveal() {
      if (els.disclosure) els.disclosure.open = true;
      input.focus();
    }

    function setBusy(busy) {
      input.readOnly = busy;
      [els.toggle, els.generate].forEach(function (b) { if (b) b.disabled = busy; });
    }

    // clear empties and hides the field once the secret is sealed.
    function clear() {
      input.value = '';
      setShown(false);
      render();
    }

    input.addEventListener('input', changed);
    if (els.toggle) els.toggle.addEventListener('click', function () { setShown(input.type === 'password'); });
    if (els.generate) {
      els.generate.addEventListener('click', function () {
        input.value = passgen.generate();
        setShown(true);
        changed();
      });
    }
    render();
    return { value: value, problem: problem, overhead: overhead, setBusy: setBusy, clear: clear, reveal: reveal };
  }

  window.gonePassphraseField = Object.freeze({ create: create });
})();
