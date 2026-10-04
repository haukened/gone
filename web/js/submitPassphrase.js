'use strict';

// Optional passphrase field on the create form: show/hide toggle, Generate
// button, live entropy and time-to-guess hint, and length validation. Requires
// window.goneUtil, window.goneCrypto and window.gonePassgen. Exposed as
// window.gonePassphraseField.
(function passphraseFieldModule() {
  if (window.gonePassphraseField || !window.goneUtil || !window.goneCrypto || !window.gonePassgen) return;
  const util = window.goneUtil;
  const passgen = window.gonePassgen;

  const SHORT_TEXT = `Too short: use at least ${passgen.minChars} characters.`;
  const WEAK_NUDGE = ' Longer is better, or press Generate.';
  const YEAR = 365.25 * 24 * 3600;
  const UNITS = [[60, 'second'], [3600, 'minute'], [86400, 'hour'], [YEAR, 'day']];
  const BIG_YEARS = [[1e9, 'billion'], [1e6, 'million'], [1e3, 'thousand']];
  const UNIVERSE_YEARS = 1.38e10;

  function plural(n, unit) {
    return `${n} ${unit}${n === 1 ? '' : 's'}`;
  }

  // duration renders seconds as rough, plain English: "3 hours", "58 years",
  // "12 thousand years".
  function duration(seconds) {
    let prev = 1;
    for (const [limit, unit] of UNITS) {
      if (seconds < limit) return plural(Math.max(1, Math.round(seconds / prev)), unit);
      prev = limit;
    }
    const years = seconds / YEAR;
    for (const [n, word] of BIG_YEARS) {
      if (years >= n) return `${Math.round(years / n)} ${word} years`;
    }
    return plural(Math.round(years), 'year');
  }

  // describe returns the hint for a passphrase: its entropy and the average
  // time an attacker holding the link would need to guess it.
  function describe(level, entropyBits) {
    if (level === 'empty') return '';
    if (level === 'short') return SHORT_TEXT;
    const seconds = passgen.crackSeconds(entropyBits);
    const years = seconds / YEAR;
    let crack;
    if (seconds < 1) crack = 'guessed almost instantly';
    else if (years > UNIVERSE_YEARS) crack = 'longer than the age of the universe to guess';
    else crack = `about ${duration(seconds)} to guess`;
    const text = `About ${Math.round(entropyBits)} bits of entropy, ${crack}.`;
    return level === 'weak' ? text + WEAK_NUDGE : text;
  }

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
    let generated = '';

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

    // entropy is exact for an untouched generated passphrase, else estimated.
    function entropy() {
      return generated && input.value === generated ? passgen.generatedBits : passgen.bits(input.value);
    }

    function render() {
      const entropyBits = entropy();
      const level = passgen.strength(input.value, entropyBits);
      if (els.strength) {
        els.strength.dataset.level = level;
        util.setText(els.strength, describe(level, entropyBits));
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
