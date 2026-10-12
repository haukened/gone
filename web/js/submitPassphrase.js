'use strict';

// Optional passphrase field on the create form: show/hide toggle, Generate
// button, live entropy and time-to-guess hint, and length validation. Requires
// window.goneI18n, window.goneCrypto, window.gonePassgen and
// window.goneSubmitStrength (submitStrength.js). Exposed as
// window.gonePassphraseField.
(function passphraseFieldModule() {
  if (window.gonePassphraseField || !window.goneI18n || !window.goneCrypto || !window.gonePassgen || !window.goneSubmitStrength) return;
  const i18n = window.goneI18n;
  const passgen = window.gonePassgen;
  const showStrength = window.goneSubmitStrength.showStrength;

  const SHORT_PROBLEM = { key: 'js.pass.shortProblem', args: { min: passgen.minChars } };

  // create wires the passphrase controls.
  //
  // els: {disclosure, input, toggle, generate, strength}; onChange is called
  // after every edit. Returns null when the input is missing, otherwise
  // {value, problem, overhead, setBusy, clear, reveal}.
  function create(els, onChange) {
    const input = els.input;
    if (!input) return null;
    const toggleLabel = els.toggle ? els.toggle.querySelector('span') : null;
    let generated = '';

    function value() {
      return input.value;
    }

    // problem returns {key, args} when a non-empty passphrase is too short.
    function problem() {
      return passgen.strength(input.value) === 'short' ? SHORT_PROBLEM : '';
    }

    // overhead returns the extra ciphertext bytes a passphrase adds (the v2 header).
    function overhead() {
      return input.value ? window.goneCrypto.v2HeaderBytes : 0;
    }

    function setShown(shown) {
      input.type = shown ? 'text' : 'password';
      i18n.set(toggleLabel, shown ? 'js.common.hide' : 'js.common.show');
    }

    // entropy is exact for an untouched generated passphrase, else estimated.
    function entropy() {
      return generated && input.value === generated ? passgen.generatedBits : passgen.bits(input.value);
    }

    function render() {
      const entropyBits = entropy();
      const level = passgen.strength(input.value, entropyBits);
      showStrength(els.strength, level, entropyBits);
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
        generated = passgen.generate();
        input.value = generated;
        setShown(true);
        changed();
      });
    }
    render();
    return { value: value, problem: problem, overhead: overhead, setBusy: setBusy, clear: clear, reveal: reveal };
  }

  window.gonePassphraseField = Object.freeze({ create: create });
})();
