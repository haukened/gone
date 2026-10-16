'use strict';

// The servers the end-to-end tests talk to, shared by playwright.config.js
// and the specs that need a server of their own.
//
// Default: the prod goned binary sits behind tlsproxy (tlsproxy/main.go), a
// TLS-terminating proxy with a throwaway self-signed certificate, so the
// browsers see a real https:// origin. GONE_E2E_PLAIN=1 serves plain
// http://127.0.0.1 straight from goned instead, for debugging.
//
//   main: the origin most specs use. The default rate limits are on, with a
//         burst of 1000 so one client (every browser connects from
//         127.0.0.1) never runs a whole suite's worth of requests dry.
//   rate: a server with a tiny budget for ratelimit.spec.js, which gives each
//         test its own client through X-Forwarded-For (goned trusts the
//         proxy's address, and the proxy appends the real peer).
//   policy: a server with an operator policy for policy.spec.js: every
//         secret needs a passphrase, and files are turned off.
//   brand: a branded server for brand.spec.js: a name, logo and accent,
//         and contact, imprint and privacy links.

const path = require('node:path');

const plain = Boolean(process.env.GONE_E2E_PLAIN);
const port = Number(process.env.GONE_E2E_PORT || 8199);
const scheme = plain ? 'http' : 'https';

// BACKEND_OFFSET separates goned's own port from the public one when a
// proxy sits in front.
const BACKEND_OFFSET = 100;

const servers = {
  main: { port: port, rate: { create: '10/m', read: '30/m', burst: 1000 } },
  rate: { port: port + 1, rate: { create: '30/m', read: '30/m', burst: 3 } },
  policy: {
    port: port + 2,
    rate: { create: '10/m', read: '30/m', burst: 1000 },
    env: { GONE_REQUIRE_PASSPHRASE: 'true', GONE_MAX_FILES: '0' },
  },
  brand: {
    port: port + 3,
    rate: { create: '10/m', read: '30/m', burst: 1000 },
    env: {
      GONE_BRAND_NAME: 'Acme, Inc. Secrets',
      GONE_BRAND_LOGO: path.join(__dirname, 'brand-logo.svg'),
      GONE_BRAND_ACCENT: '#1d4ed8/#93c5fd',
      GONE_CONTACT_URL: 'mailto:it@example.com',
      GONE_IMPRINT_URL: 'https://example.com/imprint',
      GONE_PRIVACY_URL: 'https://example.com/privacy',
    },
  },
};

for (const s of Object.values(servers)) {
  s.origin = `${scheme}://127.0.0.1:${s.port}`;
  s.backendPort = plain ? s.port : s.port + BACKEND_OFFSET;
}

module.exports = { plain, scheme, servers };
