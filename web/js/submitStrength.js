'use strict';

// Strength hint for the create form's passphrase field: its entropy and the
// average time an attacker holding the link would need to guess it. Requires
// window.goneI18n and window.gonePassgen. Exposed as window.goneSubmitStrength.
(function submitStrengthModule() {
  if (window.goneSubmitStrength || !window.goneI18n || !window.gonePassgen) return;
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

  window.goneSubmitStrength = Object.freeze({ describe: describe, showStrength: showStrength });
})();
