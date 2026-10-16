'use strict';

// Shared fixtures. Every page a test opens is watched, and the test fails
// if any of them logs a console error, throws, breaks the CSP, or gets an
// error status for a page or static asset. API errors are left to the
// tests, because a 404 is how the API says a secret is gone.

const base = require('@playwright/test');

const { expect } = base;

// Browsers also log a failed fetch as a console error. Those are checked
// through the response status instead, where API calls can be told apart.
const RESOURCE_NOISE = /Failed to load resource|status of \d{3}/;

// reportViolations runs in the page before its own scripts.
function reportViolations() {
  /* global document */
  document.addEventListener('securitypolicyviolation', function (e) {
    console.error('CSP violation: ' + e.violatedDirective + ' blocked ' + (e.blockedURI || 'inline'));
  });
}

// isAllowed reports whether text matches a pattern the test expects.
function isAllowed(allowed, text) {
  return allowed.some(function (re) { return re.test(text); });
}

// watch records the problems on page into errors.
function watch(page, errors, allowed) {
  page.on('console', function (msg) {
    const text = msg.text();
    if (msg.type() === 'error' && !RESOURCE_NOISE.test(text) && !isAllowed(allowed, text)) {
      errors.push('console: ' + text);
    }
  });
  page.on('pageerror', function (err) {
    errors.push('page error: ' + err.message);
  });
  page.on('response', function (res) {
    const url = new URL(res.url());
    if (res.status() < 400 || url.pathname.startsWith('/api/')) return;
    if (isAllowed(allowed, url.pathname)) return;
    errors.push('HTTP ' + res.status() + ': ' + url.pathname);
  });
  return page.addInitScript(reportViolations);
}

const test = base.test.extend({
  // errors collects problems from every watched page in the test.
  // Playwright requires the destructuring pattern, even when empty.
  // eslint-disable-next-line no-empty-pattern
  errors: [async ({}, use) => {
    const errors = [];
    errors.allowed = [];
    // allow lets a test expect an error: a console message or a page path
    // with an error status that matches re.
    errors.allow = function (re) { errors.allowed.push(re); };
    await use(errors);
    expect([...errors], 'browser errors').toEqual([]);
  }, { auto: true }],

  page: async ({ page, errors }, use) => {
    await watch(page, errors, errors.allowed);
    await use(page);
  },

  // newPerson opens a page in a fresh browser context: another person,
  // with their own storage and no shared state.
  newPerson: async ({ browser, errors }, use) => {
    const contexts = [];
    await use(async function () {
      const ctx = await browser.newContext();
      contexts.push(ctx);
      const page = await ctx.newPage();
      await watch(page, errors, errors.allowed);
      return page;
    });
    await Promise.all(contexts.map(function (ctx) { return ctx.close(); }));
  },
});

module.exports = { test, expect };
