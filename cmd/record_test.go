package cmd

import (
	"strings"
	"testing"
)

func TestInstrumentSpec_WrapsAllActions(t *testing.T) {
	spec := `import { test, expect } from '@playwright/test';
test('t', async ({ page }) => {
  await page.goto('https://x');
  await page.locator('.oe_kanban_color_0').click();
  await page.getByRole('textbox', { name: 'X' }).dblclick();
  await page.getByRole('button').filter({ hasText: /^$/ }).nth(2).click();
  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('menuitem', { name: 'Y' }).click();
  const download = await downloadPromise;
});`

	out := instrumentSpec(spec, "/tmp/s")

	if c := strings.Count(out, "catch (__oraculoErr)"); c != 5 {
		t.Errorf("se esperaban 5 acciones envueltas, hay %d:\n%s", c, out)
	}
	for _, frag := range []string{
		"await page.locator('.oe_kanban_color_0').click();",
		".dblclick();",
		".filter({ hasText: /^$/ }).nth(2).click();",
	} {
		if !strings.Contains(out, frag) {
			t.Errorf("la acción %q debe seguir presente y envuelta", frag)
		}
	}
	if !strings.Contains(out, "const downloadPromise = page.waitForEvent('download');") {
		t.Errorf("las declaraciones const no deben envolverse:\n%s", out)
	}
}

func TestInstrumentSpec_RetriesAmbiguousActionWithFirst(t *testing.T) {
	spec := `import { test, expect } from '@playwright/test';
test('t', async ({ page }) => {
  await page.getByRole('radio', { name: 'Medio' }).click();
  await page.getByRole('button', { name: '9' }).first().click();
  await page.getByRole('cell', { name: '1.00' }).nth(1).click();
});`

	out := instrumentSpec(spec, "/tmp/s")

	if !strings.Contains(out, "strict mode violation") {
		t.Errorf("debe reintentar la acción ambigua en strict-mode:\n%s", out)
	}
	if !strings.Contains(out, "await page.getByRole('radio', { name: 'Medio' }).first().click();") {
		t.Errorf("el reintento debe acotar el radio 'Medio' a .first():\n%s", out)
	}
	if c := strings.Count(out, "strict mode violation"); c != 1 {
		t.Errorf("solo la acción sin desambiguar debe reintentar; las .first()/.nth() no, hay %d:\n%s", c, out)
	}
}

func TestCleanRecordedSpec_CollapsesIncrementalFillsAndNoise(t *testing.T) {
	input := `import { test, expect } from '@playwright/test';

test('test', async ({ page }) => {
  await page.getByRole('textbox', { name: 'Recomendado por' }).click();
  await page.getByRole('textbox', { name: 'Recomendado por' }).press('CapsLock');
  await page.getByRole('textbox', { name: 'Recomendado por' }).fill('O');
  await page.getByRole('textbox', { name: 'Recomendado por' }).press('CapsLock');
  await page.getByRole('textbox', { name: 'Recomendado por' }).fill('Odoo ');
  await page.getByRole('textbox', { name: 'Recomendado por' }).fill('Odoo Inc');
});`

	out := cleanRecordedSpec(input)

	if c := strings.Count(out, "CapsLock"); c != 0 {
		t.Errorf("se esperaba 0 press('CapsLock'), quedaron %d:\n%s", c, out)
	}
	if c := strings.Count(out, ".fill("); c != 1 {
		t.Errorf("se esperaba 1 fill() colapsado, quedaron %d:\n%s", c, out)
	}
	if !strings.Contains(out, "fill('Odoo Inc')") {
		t.Errorf("el fill colapsado debe conservar el último valor 'Odoo Inc':\n%s", out)
	}
	if !strings.Contains(out, "await page.getByRole('textbox', { name: 'Recomendado por' }).click();") {
		t.Errorf("el click previo al input no debe descartarse:\n%s", out)
	}
}

func TestCleanRecordedSpec_KeepsDistinctFills(t *testing.T) {
	input := `  await page.getByRole('textbox', { name: 'A' }).fill('uno');
  await page.getByRole('textbox', { name: 'B' }).fill('dos');`
	out := cleanRecordedSpec(input)
	if c := strings.Count(out, ".fill("); c != 2 {
		t.Errorf("fills sobre locators distintos no deben colapsarse, quedaron %d:\n%s", c, out)
	}
}

func TestInstrumentSpec_MultilineWrapAndSkipsDeclarations(t *testing.T) {
	spec := `import { test, expect } from '@playwright/test';
test('t', async ({ page }) => {
  await page.goto('https://x');
  const downloadPromise = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Guardar' }).click();
  const download = await downloadPromise;
});`

	out := instrumentSpec(spec, "/tmp/shots")

	if c := strings.Count(out, "catch (__oraculoErr)"); c != 2 {
		t.Errorf("se esperaban 2 acciones envueltas (goto + click), hay %d:\n%s", c, out)
	}
	if !strings.Contains(out, "const downloadPromise = page.waitForEvent('download');") ||
		!strings.Contains(out, "const download = await downloadPromise;") {
		t.Errorf("las declaraciones const deben pasar sin envolver:\n%s", out)
	}
	if !strings.Contains(out, "try {\n") {
		t.Errorf("el wrap debe ser multilínea (try { en su propia línea):\n%s", out)
	}
	if !strings.Contains(out, "step-1.png") || !strings.Contains(out, "step-2.png") {
		t.Errorf("debe capturar un screenshot por paso envuelto:\n%s", out)
	}
}

func TestParseFailedSteps_IgnoresSourceEcho(t *testing.T) {
	real := "  " + stepFailMarker + " 6: locator.click: Error: strict mode violation"
	echo := "      try { await page.x(); } catch (__oraculoErr) { console.log('" +
		stepFailMarker + " 8: ' + (__oraculoErr instanceof Error ? __oraculoErr.message : String(__oraculoErr)).split('\\n')[0]); }"

	failed := parseFailedSteps(real + "\n" + echo)

	if len(failed) != 1 {
		t.Fatalf("se esperaba 1 paso fallido real, se obtuvieron %d: %v", len(failed), failed)
	}
	if failed[0] != "Paso 6: locator.click: Error: strict mode violation" {
		t.Errorf("paso fallido mal parseado: %q", failed[0])
	}
}
