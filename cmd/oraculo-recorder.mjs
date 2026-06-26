// Recorder en vivo de Oráculo (Diseño A).
// Reemplaza el "codegen + re-ejecutar el spec para sacar screenshots" (que tragaba los
// fallos de selector y capturaba la misma pantalla en todos los pasos) por capturar EN
// VIVO: usa el recorder de Playwright (context._enableRecorder, el mismo que codegen) que
// escribe el spec INCREMENTALMENTE — una línea por acción — y toma un screenshot apenas
// aparece cada acción nueva, en el estado real y autenticado. SIN replay.
//
// No usamos el eventSink del recorder (es API privada y no dispara en todas las versiones
// de Playwright); en su lugar observamos el archivo del spec, que es comportamiento estable
// y version-independiente (codegen --output siempre lo escribe). Lo invoca cmd/record.go
// vía `node`. Config por env vars (ORACULO_*).
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
  // no existía, ok
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

// La página donde el usuario está actuando (se actualiza con popups/tabs nuevos).
let activePage = null;
context.on('page', (p) => {
  activePage = p;
});

// El recorder escribe el spec; arrancamos la grabación (sin eventSink).
await context._enableRecorder({
  language: 'playwright-test',
  mode: 'recording',
  outputFile: SPEC_PATH,
  handleSIGINT: false,
});

const page = await context.newPage();
activePage = page;
if (BASE_URL) await page.goto(BASE_URL).catch(() => {});

// Cuenta las líneas de acción del spec (una por acción del usuario).
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
        // una captura fallida no debe romper la grabación
      }
    });
  }
};

const poll = setInterval(captureNew, 200);

await new Promise((resolve) => browser.on('disconnected', resolve));
clearInterval(poll);
captureNew();
await queue;
console.log(`ORACULO_RECORDER_OK: ${step}`);
process.exit(0);
