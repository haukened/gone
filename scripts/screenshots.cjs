'use strict';

// Regenerates the README screenshots in docs/img: the send form, the Open
// page and a revealed (hidden) secret, each in light and dark.
//
// Run it with `task screenshots`, which builds goned and installs Playwright
// into .tmp/. It starts goned on a free loopback port with a throwaway data
// directory, and the browser reaches it through an https:// origin (requests
// are routed to the local server), so pages render as they do in production:
// a secure context with no plain-HTTP warning.

const { spawn } = require('node:child_process');
const fs = require('node:fs');
const net = require('node:net');
const os = require('node:os');
const path = require('node:path');
const { chromium } = require('playwright');

const ROOT = path.resolve(__dirname, '..');
const OUT = path.join(ROOT, 'docs', 'img');
const ORIGIN = 'https://gone.example';
const VIEWPORT = { width: 760, height: 1000 };
const SCALE = 2;
const MARGIN = 40;
const MESSAGE = [
  'Staging database',
  'host: db.staging.example',
  'user: deploy',
  'password: Kx7#tR!q9wVm2pL'
].join('\n');

function freePort() {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.once('error', reject);
    srv.listen(0, '127.0.0.1', () => {
      const { port } = srv.address();
      srv.close(() => resolve(port));
    });
  });
}

async function waitReady(base) {
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(`${base}/readyz`)).ok) return;
    } catch {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error('goned did not become ready');
}

async function startServer() {
  const port = await freePort();
  const dataDir = fs.mkdtempSync(path.join(os.tmpdir(), 'gone-shots-'));
  const proc = spawn(path.join(ROOT, 'bin', 'goned'), [], {
    cwd: ROOT,
    env: { ...process.env, GONE_ADDR: `127.0.0.1:${port}`, GONE_DATA_DIR: path.join(dataDir, 'data') },
    stdio: ['ignore', 'ignore', 'inherit']
  });
  const base = `http://127.0.0.1:${port}`;
  await waitReady(base);
  return {
    base,
    stop() {
      proc.kill();
      fs.rmSync(dataDir, { recursive: true, force: true });
    }
  };
}

async function newContext(browser, base, theme) {
  const ctx = await browser.newContext({
    viewport: VIEWPORT,
    deviceScaleFactor: SCALE,
    colorScheme: theme,
    reducedMotion: 'reduce'
  });
  await ctx.route(`${ORIGIN}/**`, async (route) => {
    const url = route.request().url().replace(ORIGIN, base);
    route.fulfill({ response: await route.fetch({ url }) });
  });
  return ctx;
}

// shoot saves the page from the top through the visible pane plus a margin,
// leaving out the footer. The viewport is first grown to that height: the
// backdrop is position: fixed, so a full-page capture would paint it only
// for the first screenful.
async function shoot(page, name) {
  await page.setViewportSize(VIEWPORT);
  const pane = await page.locator('.view:not([hidden]) .pane').boundingBox();
  const height = Math.ceil(pane.y + pane.height + MARGIN);
  await page.setViewportSize({ width: VIEWPORT.width, height: Math.max(height, VIEWPORT.height) });
  const file = path.join(OUT, `${name}.jpg`);
  await page.screenshot({ path: file, type: 'jpeg', quality: 90, clip: { x: 0, y: 0, width: VIEWPORT.width, height } });
  console.log(`wrote ${path.relative(ROOT, file)}`);
}

async function createLink(browser, base) {
  const ctx = await newContext(browser, base, 'light');
  const page = await ctx.newPage();
  await page.goto(`${ORIGIN}/`);
  await page.fill('#secret', MESSAGE);
  await page.click('button[type=submit]');
  await page.waitForSelector('#result:not([hidden])');
  const link = await page.inputValue('#share-link');
  await ctx.close();
  return link;
}

async function shootTheme(browser, base, theme) {
  const ctx = await newContext(browser, base, theme);
  const page = await ctx.newPage();
  await page.goto(`${ORIGIN}/`);
  await page.fill('#secret', MESSAGE);
  await page.locator('#secret').blur();
  await shoot(page, `send-${theme}`);

  await page.goto(await createLink(browser, base));
  await page.waitForSelector('#view-open:not([hidden])');
  await shoot(page, `open-${theme}`);

  await page.click('#open-secret');
  await page.waitForSelector('#view-revealed:not([hidden])');
  await page.locator('#revealed-heading').blur();
  await shoot(page, `revealed-${theme}`);
  await ctx.close();
}

async function main() {
  fs.mkdirSync(OUT, { recursive: true });
  const server = await startServer();
  const browser = await chromium.launch();
  try {
    for (const theme of ['light', 'dark']) await shootTheme(browser, server.base, theme);
  } finally {
    await browser.close();
    server.stop();
  }
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
