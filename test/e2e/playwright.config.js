'use strict';

// Browser end-to-end tests: the real goned binary (prod build, so the
// minified, embedded assets that ship), the real templates and the real CSP,
// served over HTTPS through a test TLS proxy with the rate limits on, in
// Chromium, Firefox and WebKit. Run with `task e2e`, which builds
// bin/goned first.

const path = require('node:path');
const { defineConfig, devices } = require('@playwright/test');

const { plain, servers, idp, dataRoot } = require('./servers');
const binary = path.resolve(__dirname, '../../bin/goned');

// gonedServer starts the prod binary for one of the servers in servers.js on
// a fresh data directory, with that server's rate limits and any extra
// settings it lists (such as an operator policy). Readiness is
// checked on goned's own port, which is the public one only without the proxy.
function gonedServer(name) {
  const s = servers[name];
  const dir = path.join(dataRoot, name);
  return {
    command: `rm -rf "${dir}" && mkdir -p "${dir}" && exec "${binary}"`,
    url: `http://127.0.0.1:${s.backendPort}/readyz`,
    reuseExistingServer: !process.env.CI,
    timeout: 30000,
    env: {
      GONE_ADDR: `127.0.0.1:${s.backendPort}`,
      GONE_DATA_DIR: dir,
      GONE_RATE_CREATE: s.rate.create,
      GONE_RATE_READ: s.rate.read,
      GONE_RATE_BURST: String(s.rate.burst),
      // The TLS proxy appends the peer it sees to X-Forwarded-For, which is
      // how ratelimit.spec.js gives each test its own client.
      GONE_TRUSTED_PROXIES: '127.0.0.1',
      ...s.env,
    },
  };
}

// proxyServer builds and starts the TLS proxy in front of every server, with
// the sign-in stand-ins and their identity provider. It starts first and is
// ready once its own issuer answers, so the certificate file exists before
// the signin server first fetches keys.
function proxyServer() {
  const pairs = Object.values(servers).map(function (s) {
    return `127.0.0.1:${s.port}=127.0.0.1:${s.backendPort}${s.signIn ? '+' + s.signIn : ''}`;
  });
  const exe = path.join(dataRoot, 'tlsproxy');
  const idpArgs = `idp=127.0.0.1:${idp.port} certout="${idp.certFile}"`;
  return {
    command: `mkdir -p "${dataRoot}" && go build -o "${exe}" ./tlsproxy && exec "${exe}" ${idpArgs} ${pairs.join(' ')}`,
    url: `${idp.origin}/.well-known/openid-configuration`,
    ignoreHTTPSErrors: true,
    reuseExistingServer: !process.env.CI,
    timeout: 120000,
  };
}

module.exports = defineConfig({
  testDir: __dirname,
  testMatch: '*.spec.js',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    // https://127.0.0.1 through tlsproxy (a self-signed certificate), or
    // plain http://127.0.0.1 with GONE_E2E_PLAIN=1. Both are secure contexts,
    // which WebCrypto needs.
    baseURL: servers.main.origin,
    ignoreHTTPSErrors: true,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'firefox', use: { ...devices['Desktop Firefox'] } },
    { name: 'webkit', use: { ...devices['Desktop Safari'] } },
  ],
  // A fresh data directory each run for each server; see servers.js.
  webServer: (plain ? [] : [proxyServer()]).concat(Object.keys(servers).map(gonedServer)),
});
