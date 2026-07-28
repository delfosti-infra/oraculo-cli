package cmd

import "testing"

func TestClassifyFlowOwnership(t *testing.T) {
	tests := []struct {
		name         string
		meta         flowMeta
		projectRefId string
		want         flowOwnership
	}{
		{
			name:         "flow grabado para el proyecto activo",
			meta:         flowMeta{ProjectRefId: "project-1"},
			projectRefId: "project-1",
			want:         ownershipMine,
		},
		{
			name:         "flow grabado para otro proyecto",
			meta:         flowMeta{ProjectRefId: "project-2"},
			projectRefId: "project-1",
			want:         ownershipForeign,
		},
		{
			name:         "flow sin proyecto declarado",
			meta:         flowMeta{},
			projectRefId: "project-1",
			want:         ownershipOrphan,
		},
		{
			name:         "flow con proyecto en blanco",
			meta:         flowMeta{ProjectRefId: "   "},
			projectRefId: "project-1",
			want:         ownershipOrphan,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyFlowOwnership(tt.meta, tt.projectRefId); got != tt.want {
				t.Errorf("classifyFlowOwnership() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpecFingerprint(t *testing.T) {
	t.Run("mismo contenido produce el mismo hash", func(t *testing.T) {
		spec := "test('login', async () => {});"
		if specFingerprint(spec) != specFingerprint(spec) {
			t.Error("el hash no es estable para el mismo contenido")
		}
	})

	t.Run("ignora el fin de línea de Windows", func(t *testing.T) {
		unix := "line one\nline two\n"
		windows := "line one\r\nline two\r\n"
		if specFingerprint(unix) != specFingerprint(windows) {
			t.Error("CRLF y LF deberían producir el mismo hash")
		}
	})

	t.Run("ignora newlines finales", func(t *testing.T) {
		if specFingerprint("body") != specFingerprint("body\n\n") {
			t.Error("los newlines finales no deberían cambiar el hash")
		}
	})

	t.Run("detecta un cambio real", func(t *testing.T) {
		if specFingerprint("click('a')") == specFingerprint("click('b')") {
			t.Error("specs distintos deberían producir hashes distintos")
		}
	})
}

func TestIsAlreadyPushed(t *testing.T) {
	spec := "test('login', async () => {});"

	t.Run("meta sin hash nunca cuenta como subido", func(t *testing.T) {
		if isAlreadyPushed(flowMeta{}, spec) {
			t.Error("un flow sin specHash no puede considerarse ya subido")
		}
	})

	t.Run("hash igual cuenta como subido", func(t *testing.T) {
		meta := flowMeta{SpecHash: specFingerprint(spec)}
		if !isAlreadyPushed(meta, spec) {
			t.Error("el spec no cambió, debería contar como ya subido")
		}
	})

	t.Run("hash distinto cuenta como pendiente", func(t *testing.T) {
		meta := flowMeta{SpecHash: specFingerprint("otro spec")}
		if isAlreadyPushed(meta, spec) {
			t.Error("el spec cambió, no debería contar como ya subido")
		}
	})
}

func TestDescribeProjectOwner(t *testing.T) {
	tests := []struct {
		name string
		meta flowMeta
		want string
	}{
		{
			name: "prefiere el nombre del proyecto",
			meta: flowMeta{ProjectRefId: "ref-1", ProjectName: "Volcan"},
			want: "Volcan",
		},
		{
			name: "cae al refId cuando no hay nombre",
			meta: flowMeta{ProjectRefId: "ref-1"},
			want: "proyecto ref-1",
		},
		{
			name: "texto genérico cuando no hay nada",
			meta: flowMeta{},
			want: "otro proyecto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeProjectOwner(tt.meta); got != tt.want {
				t.Errorf("describeProjectOwner() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupForeignByProject(t *testing.T) {
	foreign := []foreignFlow{
		{Slug: "editar-rol", ProjectName: "Proyecto 2"},
		{Slug: "login", ProjectName: "Proyecto 1"},
		{Slug: "crear-rol", ProjectName: "Proyecto 2"},
	}

	lines := groupForeignByProject(foreign)

	want := []string{
		"Proyecto 1: login",
		"Proyecto 2: crear-rol, editar-rol",
	}
	if len(lines) != len(want) {
		t.Fatalf("groupForeignByProject() devolvió %d líneas, want %d", len(lines), len(want))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("línea %d = %q, want %q", i, lines[i], want[i])
		}
	}
}
