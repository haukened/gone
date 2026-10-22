'use strict';

// Browser tests for the website (site/), separate from the app's suite: the
// built site (task site) served as GitHub Pages would serve it, in Chromium
// and Firefox. Run with `task site-test`.

const path = require('node:path');
const { defineConfig, devices } = require('@playwright/test');

const port = Number(process.env.SITE_PORT || 8299);

module.exports = defineConfig({
  testDir: path.join(__dirname, 'site'),
  testMatch: '*.site.js',
  fullyParallel: true,
  forbidOnly: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never', outputFolder: 'site-report' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'firefox', use: { ...devices['Desktop Firefox'] } },
  ],
  webServer: {
    command: 'node site/serve.js',
    url: `http://127.0.0.1:${port}/`,
    reuseExistingServer: !process.env.CI,
    env: { SITE_PORT: String(port) },
  },
});
