'use strict';

// Optional passphrase field on the create form: show/hide toggle, Generate
// button, live entropy and time-to-guess hint, and length validation. Requires
// window.goneI18n, window.goneCrypto and window.gonePassgen. Exposed as
// window.gonePassphraseField.
(function passphraseFieldModule() {
  if (window.gonePassphraseField || !window.goneI18n || !window.goneCrypto || !window.gonePassgen) return;
  const i18n = window.goneI18n;
  const passgen = window.gonePassgen;

  const YEAR = 365.25 * 24 * 3600;
  const UNITS = [[60, 'seconds'], [3600, 'minutes'], [86400, 'hours'], [YEAR, 'days']];
  const BIG_YEARS = [1000000000, 1000000, 1000];
  const UNIVERSE_YEARS = 13800000000;

  // duration describes seconds roughly, as a nested message for the hint:
  // "3 hours", "58 years", "12 thousand years".
  function duration(seconds) {
    let prev = 1;
    for (const [limit, unit] of UNITS) {
      if (seconds < limit) return unitMsg(unit, Math.max(1, Math.round(seconds / prev)));
      prev = limit;
    }
    const years = seconds / YEAR;
    const big = BIG_YEARS.find(function (n) { return years >= n; });
    if (!big) return unitMsg('years', Math.round(years));
    const rounded = Math.round(years / big) * big;
    return { msg: 'js.pass.years', args: { count: rounded, n: { compact: rounded } } };
  }

  function unitMsg(unit, n) {
    return { msg: 'js.pass.' + unit, args: { count: n, n: n } };
  }

  // describe returns the hint for a passphrase as {key, args}, or null when
  // empty: its entropy and the average time an attacker holding the link
  // would need to guess it.
  function describe(level, entropyBits) {
    if (level === 'empty') return null;
    if (level === 'short') return { key: 'js.pass.short', args: { min: passgen.minChars } };
    const seconds = passgen.crackSeconds(entropyBits);
    const args = { bits: Math.round(entropyBits) };
    let key = 'js.pass.guessTime';
    if (seconds < 1) key = 'js.pass.guessInstant';
    else if (seconds / YEAR > UNIVERSE_YEARS) key = 'js.pass.guessUniverse';
    else args.time = duration(seconds);
    if (level !== 'weak') return { key: key, args: args };
    return { key: 'js.pass.weak', args: { text: { msg: key, args: args } } };
  }

  // showStrength writes the strength hint into node, if there is one.
  function showStrength(node, level, entropyBits) {
    if (!node) return;
    node.dataset.level = level;
    const hint = describe(level, entropyBits);
    if (hint) i18n.set(node, hint.key, hint.args);
    else i18n.clear(node);
  }

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
