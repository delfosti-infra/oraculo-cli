// Recorder en vivo de Oráculo (Diseño A).
// Reemplaza el `playwright codegen` + replay: usa el recorder de Playwright
// (context._enableRecorder) y captura un screenshot EN VIVO por cada acción que el
// usuario ejecuta (eventSink.actionAdded), en el estado real y autenticado — sin replay.
// Lo invoca cmd/record.go vía `node`. Config por env vars (ORACULO_*).
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

let step = 0;
let capturing = Promise.resolve();

const capture = (page, n) => {
  capturing = capturing.then(async () => {
    try {
      await page.waitForLoadState('domcontentloaded').catch(() => {});
      await page.waitForTimeout(120);
      await page.screenshot({
        path: path.join(SHOTS_DIR, `step-${n}.png`),
        fullPage: false,
      });
    } catch {
      // una captura fallida no debe romper la grabación
    }
  });
};

// context._enableRecorder es la misma API interna que usa `playwright codegen`.
// El eventSink emite actionAdded/actionUpdated por cada acción del usuario, con la
// `page` donde ocurrió — capturamos ahí, en el estado real (no en un replay).
await context._enableRecorder(
  {
    language: 'playwright-test',
    mode: 'recording',
    outputFile: SPEC_PATH,
    handleSIGINT: false,
  },
  {
    actionAdded: (page) => {
      step += 1;
      capture(page, step);
    },
    actionUpdated: (page) => {
      // misma acción refinándose (ej: fill mientras se tipea) → re-captura el paso actual
      if (step > 0) capture(page, step);
    },
  },
);

const page = await context.newPage();
await page.goto(BASE_URL).catch(() => {});

await new Promise((resolve) => browser.on('disconnected', resolve));
await capturing;
console.log(`ORACULO_RECORDER_OK: ${step}`);
process.exit(0);
