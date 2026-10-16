'use strict';

// Browser end-to-end tests: the real goned binary (prod build, so the
// minified, embedded assets that ship), the real templates and the real CSP,
// in Chromium, Firefox and WebKit. Run with `task e2e`, which builds
// bin/goned first.

const path = require('node:path');
const { defineConfig, devices } = require('@playwright/test');

const port = Number(process.env.GONE_E2E_PORT || 8199);
const dataDir = path.join(__dirname, '.data');
const binary = path.resolve(__dirname, '../../bin/goned');

module.exports = defineConfig({
  testDir: __dirname,
  testMatch: '*.spec.js',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    // 127.0.0.1 is a secure context, which WebCrypto needs.
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'firefox', use: { ...devices['Desktop Firefox'] } },
    { name: 'webkit', use: { ...devices['Desktop Safari'] } },
  ],
  webServer: {
    // A fresh data directory each run. Every browser connects from
    // 127.0.0.1, so the rate limits are off.
    command: `rm -rf "${dataDir}" && mkdir -p "${dataDir}" && exec "${binary}"`,
    url: `http://127.0.0.1:${port}/readyz`,
    reuseExistingServer: !process.env.CI,
    timeout: 30000,
    env: {
      GONE_ADDR: `127.0.0.1:${port}`,
      GONE_DATA_DIR: dataDir,
      GONE_RATE_CREATE: '0',
      GONE_RATE_READ: '0',
    },
  },
});
