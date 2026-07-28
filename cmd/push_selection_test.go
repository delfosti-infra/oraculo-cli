package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
)

func writeFlow(t *testing.T, e2eDir, slug, spec string, meta *flowMeta) {
	t.Helper()
	if err := os.MkdirAll(e2eDir, 0o755); err != nil {
		t.Fatalf("no se pudo crear %s: %v", e2eDir, err)
	}
	specPath := filepath.Join(e2eDir, slug+".spec.ts")
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatalf("no se pudo escribir %s: %v", specPath, err)
	}
	if meta != nil {
		if err := saveFlowMeta(e2eDir, slug, *meta); err != nil {
			t.Fatalf("no se pudo guardar el meta de %s: %v", slug, err)
		}
	}
}

func activeProject() *types.Config {
	return &types.Config{Project: "Proyecto 3", RefId: "project-3"}
}

func resetPushFlags(t *testing.T) {
	t.Helper()
	pushForceFlag = false
	pushAllFlag = false
	t.Cleanup(func() {
		pushForceFlag = false
		pushAllFlag = false
	})
}

func TestSelectFlowsToPush(t *testing.T) {
	t.Run("solo sube los flows del proyecto activo", func(t *testing.T) {
		resetPushFlags(t)
		e2eDir := t.TempDir()
		writeFlow(t, e2eDir, "mine", "spec mine", &flowMeta{ProjectRefId: "project-3", ProjectName: "Proyecto 3"})
		writeFlow(t, e2eDir, "other", "spec other", &flowMeta{ProjectRefId: "project-1", ProjectName: "Proyecto 1"})

		selection := selectFlowsToPush(e2eDir, activeProject(), []string{"mine", "other"}, false)

		if len(selection.Push) != 1 || selection.Push[0] != "mine" {
			t.Errorf("Push = %v, want [mine]", selection.Push)
		}
		if len(selection.Foreign) != 1 || selection.Foreign[0].ProjectName != "Proyecto 1" {
			t.Errorf("Foreign = %+v, want el flow de Proyecto 1", selection.Foreign)
		}
	})

	t.Run("adopta los flows sin proyecto declarado", func(t *testing.T) {
		resetPushFlags(t)
		e2eDir := t.TempDir()
		writeFlow(t, e2eDir, "legacy", "spec legacy", nil)

		selection := selectFlowsToPush(e2eDir, activeProject(), []string{"legacy"}, false)

		if len(selection.Push) != 1 || selection.Push[0] != "legacy" {
			t.Errorf("Push = %v, want [legacy]", selection.Push)
		}
		if len(selection.Adopted) != 1 || selection.Adopted[0] != "legacy" {
			t.Errorf("Adopted = %v, want [legacy]", selection.Adopted)
		}
	})

	t.Run("omite los flows que no cambiaron desde la última subida", func(t *testing.T) {
		resetPushFlags(t)
		e2eDir := t.TempDir()
		spec := "spec sin cambios"
		writeFlow(t, e2eDir, "stable", spec, &flowMeta{
			ProjectRefId: "project-3",
			SpecHash:     specFingerprint(spec),
			PushedAt:     "2026-07-27T10:00:00Z",
		})

		selection := selectFlowsToPush(e2eDir, activeProject(), []string{"stable"}, false)

		if len(selection.Push) != 0 {
			t.Errorf("Push = %v, want vacío", selection.Push)
		}
		if len(selection.Unchanged) != 1 || selection.Unchanged[0].Slug != "stable" {
			t.Errorf("Unchanged = %+v, want [stable]", selection.Unchanged)
		}
		if selection.Unchanged[0].PushedAt.IsZero() {
			t.Error("PushedAt debería parsearse desde el meta")
		}
	})

	t.Run("sube un flow del proyecto activo cuyo spec cambió", func(t *testing.T) {
		resetPushFlags(t)
		e2eDir := t.TempDir()
		writeFlow(t, e2eDir, "edited", "spec nuevo", &flowMeta{
			ProjectRefId: "project-3",
			SpecHash:     specFingerprint("spec viejo"),
		})

		selection := selectFlowsToPush(e2eDir, activeProject(), []string{"edited"}, false)

		if len(selection.Push) != 1 || selection.Push[0] != "edited" {
			t.Errorf("Push = %v, want [edited]", selection.Push)
		}
	})

	t.Run("--all revisa incluso los que no cambiaron", func(t *testing.T) {
		resetPushFlags(t)
		pushAllFlag = true
		e2eDir := t.TempDir()
		spec := "spec sin cambios"
		writeFlow(t, e2eDir, "stable", spec, &flowMeta{
			ProjectRefId: "project-3",
			SpecHash:     specFingerprint(spec),
		})

		selection := selectFlowsToPush(e2eDir, activeProject(), []string{"stable"}, false)

		if len(selection.Push) != 1 {
			t.Errorf("Push = %v, want [stable] con --all", selection.Push)
		}
		if len(selection.Unchanged) != 0 {
			t.Errorf("Unchanged = %+v, want vacío con --all", selection.Unchanged)
		}
	})

	t.Run("--force sube un flow de otro proyecto", func(t *testing.T) {
		resetPushFlags(t)
		pushForceFlag = true
		e2eDir := t.TempDir()
		writeFlow(t, e2eDir, "other", "spec other", &flowMeta{ProjectRefId: "project-1", ProjectName: "Proyecto 1"})

		selection := selectFlowsToPush(e2eDir, activeProject(), []string{"other"}, true)

		if len(selection.Push) != 1 || selection.Push[0] != "other" {
			t.Errorf("Push = %v, want [other] con --force", selection.Push)
		}
		if len(selection.Foreign) != 0 {
			t.Errorf("Foreign = %+v, want vacío con --force", selection.Foreign)
		}
	})

	t.Run("un push explícito no se saltea por falta de cambios", func(t *testing.T) {
		resetPushFlags(t)
		e2eDir := t.TempDir()
		spec := "spec sin cambios"
		writeFlow(t, e2eDir, "stable", spec, &flowMeta{
			ProjectRefId: "project-3",
			SpecHash:     specFingerprint(spec),
		})

		selection := selectFlowsToPush(e2eDir, activeProject(), []string{"stable"}, true)

		if len(selection.Push) != 1 {
			t.Errorf("Push = %v, want [stable] en un push explícito", selection.Push)
		}
	})

	t.Run("el escenario reportado: 15 flows de 3 proyectos suben solo los 5 propios", func(t *testing.T) {
		resetPushFlags(t)
		e2eDir := t.TempDir()
		var slugs []string
		for _, project := range []struct{ refId, name string }{
			{"project-1", "Proyecto 1"},
			{"project-2", "Proyecto 2"},
			{"project-3", "Proyecto 3"},
		} {
			for i := 0; i < 5; i++ {
				slug := project.refId + "-flow-" + string(rune('a'+i))
				writeFlow(t, e2eDir, slug, "spec "+slug, &flowMeta{
					ProjectRefId: project.refId,
					ProjectName:  project.name,
				})
				slugs = append(slugs, slug)
			}
		}

		selection := selectFlowsToPush(e2eDir, activeProject(), slugs, false)

		if len(selection.Push) != 5 {
			t.Errorf("Push tiene %d flows, want 5", len(selection.Push))
		}
		if len(selection.Foreign) != 10 {
			t.Errorf("Foreign tiene %d flows, want 10", len(selection.Foreign))
		}
	})
}
