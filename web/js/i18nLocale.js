'use strict';

// Language switching support for goneI18n: fetching a catalog once, the
// gone_lang cookie, and a language's text direction. Loaded before i18n.js.
// Exposed as window.goneI18nLocale.
(function i18nLocaleModule() {
  if (window.goneI18nLocale) return;

  // create returns the helpers for the page data (its catalogs and locales).
  function create(data) {
    const catalogs = {};

    function writeCookie(tag) {
      const secure = window.location && window.location.protocol === 'https:' ? '; Secure' : '';
      document.cookie = 'gone_lang=' + encodeURIComponent(tag) + '; Path=/; Max-Age=31536000; SameSite=Lax' + secure;
    }

    // loadCatalog fetches tag's full catalog once.
    async function loadCatalog(tag) {
      if (catalogs[tag]) return catalogs[tag];
      const url = data.catalogs && data.catalogs[tag];
      if (!url) throw new Error('unknown locale');
      const res = await fetch(url, { credentials: 'same-origin' });
      if (!res.ok) throw new Error('catalog ' + res.status);
      catalogs[tag] = await res.json();
      return catalogs[tag];
    }

    function dirFor(tag) {
      const found = (data.locales || []).find(function (l) { return l.tag === tag; });
      return found && found.dir === 'rtl' ? 'rtl' : 'ltr';
    }

    return { writeCookie: writeCookie, loadCatalog: loadCatalog, dirFor: dirFor };
  }

  window.goneI18nLocale = Object.freeze({ create: create });
})();
