package cmd

import (
	"strings"
	"testing"
)

func TestPushSummary(t *testing.T) {
	cases := []struct {
		name      string
		uploaded  int
		replaced  int
		unchanged int
		want      string
	}{
		{"solo nuevos", 2, 0, 0, "2 nuevo(s)"},
		{"solo reemplazos", 0, 3, 0, "3 reemplazado(s)"},
		{"solo sin cambios", 0, 0, 4, "ya estaban al día"},
		{"nuevo y reemplazo", 1, 1, 0, "1 nuevo(s) · 1 reemplazado(s)"},
		{"mezcla completa", 1, 1, 1, "1 nuevo(s) · 1 reemplazado(s) · 1 sin cambios"},
	}
	for _, c := range cases {
		got := pushSummary(c.uploaded, c.replaced, c.unchanged)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s: pushSummary(%d,%d,%d) = %q, debe contener %q",
				c.name, c.uploaded, c.replaced, c.unchanged, got, c.want)
		}
	}
}

func TestResolveSpecPath_PrefersMobileWhenPresent(t *testing.T) {
	dir := t.TempDir()
	webPath := resolveSpecPath(dir, "login")
	if !strings.HasSuffix(webPath, "login.spec.ts") {
		t.Errorf("sin .mobile.json debe resolver al .spec.ts, dio %q", webPath)
	}
}
