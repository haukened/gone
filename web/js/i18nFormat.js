'use strict';

// Argument formatting for goneI18n: numbers, sizes, durations, dates,
// relative times, compact numbers and nested messages, in the current
// language. Loaded before i18n.js. Exposed as window.goneI18nFormat.
(function i18nFormatModule() {
  if (window.goneI18nFormat) return;

  // create returns the formatters for ctx: locale() is the current language
  // and t(key, args) renders a message.
  function create(ctx) {
    const t = ctx.t;
    const FORMATS = formats();

    // formats lists the object argument kinds the server also emits, each with
    // its renderer.
    function formats() {
      return [
        ['num', function (v) { return fixed(v.num, v.digits); }],
        ['bytes', function (v) { return bytes(v.bytes); }],
        ['seconds', function (v) { return duration(v.seconds, v.style); }],
        ['date', function (v) { return date(v.date, v.style); }],
        ['rel', function (v) { return new Intl.RelativeTimeFormat(ctx.locale(), { numeric: 'auto' }).format(v.rel, v.unit); }],
        ['compact', function (v) { return new Intl.NumberFormat(ctx.locale(), { notation: 'compact', compactDisplay: 'long', maximumFractionDigits: 0 }).format(v.compact); }],
        ['msg', function (v) { return t(v.msg, v.args); }],
      ];
    }

    // format renders one argument value in the current locale.
    function format(v) {
      if (typeof v === 'number') return new Intl.NumberFormat(ctx.locale()).format(v);
      if (typeof v === 'string') return v;
      if (!v || typeof v !== 'object') return '';
      const kind = FORMATS.find(function (f) { return f[0] in v; });
      return kind ? kind[1](v) : '';
    }

    function fixed(n, digits) {
      const d = digits || 0;
      return new Intl.NumberFormat(ctx.locale(), { minimumFractionDigits: d, maximumFractionDigits: d }).format(n);
    }

    // bytes renders a size in the largest unit below 1024 of the next.
    function bytes(n) {
      const BYTE_UNITS = ['kb', 'mb', 'gb', 'tb', 'pb'];
      if (n < 1024) return t('common.size.b', { n: n });
      let f = n;
      let i = -1;
      do {
        f /= 1024;
        i++;
      } while (f >= 1024 && i < BYTE_UNITS.length - 1);
      return t('common.size.' + BYTE_UNITS[i], { n: { num: f, digits: 1 } });
    }

    // duration renders seconds in their largest whole unit.
    function duration(sec, style) {
      const units = [['days', 86400], ['hours', 3600], ['minutes', 60], ['seconds', 1]];
      const s = style === 'short' ? 'short' : 'long';
      const unit = units.find(function (u) { return sec > 0 && sec % u[1] === 0; });
      if (!unit) return t('common.duration.' + s + '.seconds', { n: 0 });
      return t('common.duration.' + s + '.' + unit[0], { n: sec / unit[1] });
    }

    // date renders an ISO time as "datetime" (the default), "date" or "time".
    function date(iso, style) {
      const styles = {
        datetime: { dateStyle: 'medium', timeStyle: 'short' },
        date: { dateStyle: 'medium' },
        time: { timeStyle: 'short' },
      };
      const when = new Date(iso);
      if (Number.isNaN(when.getTime())) return '';
      return when.toLocaleString(ctx.locale(), styles[style] || styles.datetime);
    }

    return { format: format };
  }

  window.goneI18nFormat = Object.freeze({ create: create });
})();
