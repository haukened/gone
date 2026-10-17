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
//   signin: sign-in through a signed OIDC token (GONE_AUTH=oidc). The proxy
//         stands in for oauth2-proxy and an identity provider (idp below).
//   custom: sign-in through a plain header (GONE_AUTH=custom), with a
//         sign-in button pointing at the proxy's stub login page.

const path = require('node:path');

const plain = Boolean(process.env.GONE_E2E_PLAIN);
const dataRoot = path.join(__dirname, '.data');
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
  signin: { port: port + 4, rate: { create: '10/m', read: '30/m', burst: 1000 }, signIn: 'oidc' },
  custom: {
    port: port + 5,
    rate: { create: '10/m', read: '30/m', burst: 1000 },
    signIn: 'custom',
    env: { GONE_AUTH: 'custom', GONE_AUTH_HEADER: 'X-Gone-User', GONE_AUTH_FORMAT: 'email', GONE_AUTH_SIGNIN_URL: '/e2e-login?rd={return}' },
  },
};

// The proxy's stand-in identity provider for the signin server. goned trusts
// its self-signed certificate through SSL_CERT_FILE (Linux only).
const idp = { port: port + 6, certFile: path.join(dataRoot, 'idp-cert.pem') };
idp.origin = `https://127.0.0.1:${idp.port}`;
servers.signin.env = {
  GONE_AUTH: 'oidc', GONE_AUTH_ISSUER: idp.origin, GONE_AUTH_AUDIENCE: 'gone-e2e', SSL_CERT_FILE: idp.certFile,
};

for (const s of Object.values(servers)) {
  s.origin = `${scheme}://127.0.0.1:${s.port}`;
  s.backendPort = plain ? s.port : s.port + BACKEND_OFFSET;
}

module.exports = { plain, scheme, servers, idp, dataRoot };
