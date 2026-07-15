package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDetectedPermissions(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "marcadores limpios",
			in:   "__ORACULO_PERM__ geolocation\nalgo\n__ORACULO_PERM__ camera",
			want: []string{"camera", "geolocation"},
		},
		{
			name: "dedupe",
			in:   "__ORACULO_PERM__ geolocation\n__ORACULO_PERM__ geolocation",
			want: []string{"geolocation"},
		},
		{
			name: "ignora eco de fuente",
			in:   "const mark = (p) => { console.log('__ORACULO_PERM__ ' + p); };",
			want: nil,
		},
		{
			name: "ignora permiso no soportado",
			in:   "__ORACULO_PERM__ bluetooth",
			want: nil,
		},
		{
			name: "sin marcadores",
			in:   "Running 1 test\n  ok\n",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDetectedPermissions(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInjectPermissionDetection(t *testing.T) {
	spec := "import { test, expect } from '@playwright/test';\n\n" +
		"test('x', async ({ page }) => {\n  await page.goto('/');\n});\n"

	out := injectPermissionDetection(spec)

	if !strings.Contains(out, "page.addInitScript") {
		t.Fatalf("debe inyectar el init script:\n%s", out)
	}
	if strings.Index(out, "addInitScript") > strings.Index(out, "page.goto") {
		t.Errorf("el init script debe ir antes del primer goto:\n%s", out)
	}

	noMatch := "const x = 1;\n"
	if injectPermissionDetection(noMatch) != noMatch {
		t.Errorf("sin cuerpo de test debe devolver el spec intacto")
	}
}
