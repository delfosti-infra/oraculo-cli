import { chromium } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const env = process.env;
const SPEC_PATH = env.ORACULO_SPEC_PATH;
const SHOTS_DIR = env.ORACULO_SHOTS_DIR;
const BASE_URL = env.ORACULO_BASE_URL;
const AUTH_STATE = env.ORACULO_AUTH_STATE || undefined;
const HEADLESS = env.ORACULO_HEADLESS === '1';

const parseJson = (raw, fallback) => {
  if (!raw) return fallback;
  try {
    return JSON.parse(raw);
  } catch {
    return fallback;
  }
};
const PERMISSIONS = parseJson(env.ORACULO_PERMISSIONS, undefined);
const GEO = parseJson(env.ORACULO_GEO, undefined);

if (!SPEC_PATH || !SHOTS_DIR || !BASE_URL) {
  console.error('ORACULO_RECORDER_ERR: faltan ORACULO_SPEC_PATH / ORACULO_SHOTS_DIR / ORACULO_BASE_URL');
  process.exit(2);
}

fs.mkdirSync(SHOTS_DIR, { recursive: true });
try {
  fs.unlinkSync(SPEC_PATH);
} catch {
  /* */
}

const browser = await chromium.launch({
  headless: HEADLESS,
  args: [
    '--use-fake-ui-for-media-stream',
    '--use-fake-device-for-media-stream',
  ],
});

const contextOptions = { ignoreHTTPSErrors: true };
if (AUTH_STATE) contextOptions.storageState = AUTH_STATE;
if (PERMISSIONS) contextOptions.permissions = PERMISSIONS;
if (GEO && typeof GEO.lat === 'number' && typeof GEO.lng === 'number') {
  contextOptions.geolocation = { latitude: GEO.lat, longitude: GEO.lng };
  if (!PERMISSIONS) contextOptions.permissions = ['geolocation'];
}

const context = await browser.newContext(contextOptions);

let activePage = null;
let resolveDone;
const done = new Promise((r) => {
  resolveDone = r;
});
let finished = false;
const finish = () => {
  if (finished) return;
  finished = true;
  resolveDone();
};

context.on('page', (p) => {
  activePage = p;
  p.on('close', () => {
    const open = context.pages().filter((pg) => !pg.isClosed());
    if (open.length === 0) finish();
  });
});
browser.on('disconnected', finish);
context.on('close', finish);

await context._enableRecorder({
  language: 'playwright-test',
  mode: 'recording',
  outputFile: SPEC_PATH,
  handleSIGINT: false,
});

const page = await context.newPage();
activePage = page;
if (BASE_URL) await page.goto(BASE_URL).catch(() => {});

const ACTION_RE = /^\s*await\s+\S.*\.(goto|click|dblclick|fill|press|check|uncheck|selectOption|setInputFiles|tap|hover|focus|setChecked|clear)\(/;
const countActions = (txt) => txt.split('\n').filter((l) => ACTION_RE.test(l)).length;

let lastCount = 0;
let step = 0;
let queue = Promise.resolve();

const captureNew = () => {
  let txt = '';
  try {
    txt = fs.readFileSync(SPEC_PATH, 'utf8');
  } catch {
    return;
  }
  const current = countActions(txt);
  while (lastCount < current) {
    lastCount += 1;
    step += 1;
    const n = step;
    const pageAtAction = activePage;
    queue = queue.then(async () => {
      try {
        await pageAtAction.waitForLoadState('domcontentloaded').catch(() => {});
        await pageAtAction.waitForTimeout(180);
        await pageAtAction.screenshot({
          path: path.join(SHOTS_DIR, `step-${n}.png`),
          fullPage: false,
        });
      } catch {
        /* */
      }
    });
  }
};

const poll = setInterval(captureNew, 200);

await done;
clearInterval(poll);
captureNew();
await queue;
console.log(`ORACULO_RECORDER_OK: ${step}`);
await browser.close().catch(() => {});
process.exit(0);
