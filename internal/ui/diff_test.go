package ui

import "testing"

func TestSpecsEqual_IgnoresLineEndingsAndTrailingNewlines(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"identical", "a\nb\n", "a\nb\n", true},
		{"crlf vs lf", "a\r\nb\r\n", "a\nb\n", true},
		{"trailing newline", "a\nb", "a\nb\n", true},
		{"real change", "a\nb\n", "a\nc\n", false},
		{"added line", "a\nb\n", "a\nb\nc\n", false},
	}
	for _, c := range cases {
		if got := SpecsEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: SpecsEqual = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDiffLines_ClassifiesAddDeleteEqual(t *testing.T) {
	remote := []string{"line1", "line2", "line3"}
	local := []string{"line1", "line2-edit", "line3", "line4"}

	ops := diffLines(remote, local)

	adds, dels, eqs := 0, 0, 0
	for _, op := range ops {
		switch op.op {
		case diffAdd:
			adds++
		case diffDelete:
			dels++
		case diffEqual:
			eqs++
		}
	}

	if dels != 1 {
		t.Errorf("se esperaba 1 línea borrada (line2), hubo %d: %+v", dels, ops)
	}
	if adds != 2 {
		t.Errorf("se esperaban 2 líneas agregadas (line2-edit, line4), hubo %d: %+v", adds, ops)
	}
	if eqs != 2 {
		t.Errorf("se esperaban 2 líneas iguales (line1, line3), hubo %d: %+v", eqs, ops)
	}
}

func TestDiffLines_EmptyRemoteIsAllAdds(t *testing.T) {
	ops := diffLines([]string{""}, []string{"nuevo", ""})
	for _, op := range ops {
		if op.op == diffDelete {
			t.Errorf("sin contenido remoto no debería haber borrados: %+v", ops)
		}
	}
}
