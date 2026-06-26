package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/delfosti-infra/oraculo-cli/internal/ui"
)

type liveRecordOpts struct {
	specPath       string
	screenshotsDir string
	baseURL        string
	authStatePath  string
	geo            *geolocation
}

// runLiveRecord graba el flow capturando un screenshot EN VIVO por cada acción del
// usuario (vía el recorder de Playwright, sin replay). Devuelve cuántas capturó.
func runLiveRecord(e2eDir string, opts liveRecordOpts) (int, error) {
	scriptDir := filepath.Join(e2eDir, ".oraculo")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		return 0, fmt.Errorf("crear %s: %w", scriptDir, err)
	}
	scriptPath := filepath.Join(scriptDir, "oraculo-recorder.mjs")
	if err := os.WriteFile(scriptPath, []byte(oraculoRecorderScript), 0o644); err != nil {
		return 0, fmt.Errorf("escribir recorder: %w", err)
	}
	defer os.Remove(scriptPath)

	if err := os.MkdirAll(opts.screenshotsDir, 0o755); err != nil {
		return 0, err
	}

	absSpec, err := filepath.Abs(opts.specPath)
	if err != nil {
		return 0, err
	}
	absShots, err := filepath.Abs(opts.screenshotsDir)
	if err != nil {
		return 0, err
	}

	env := append(os.Environ(),
		"ORACULO_SPEC_PATH="+absSpec,
		"ORACULO_SHOTS_DIR="+absShots,
		"ORACULO_BASE_URL="+opts.baseURL,
	)
	if opts.authStatePath != "" {
		if absAuth, aerr := filepath.Abs(opts.authStatePath); aerr == nil {
			env = append(env, "ORACULO_AUTH_STATE="+absAuth)
		}
	}
	if opts.geo != nil {
		if b, merr := json.Marshal(opts.geo); merr == nil {
			env = append(env, "ORACULO_GEO="+string(b))
		}
	}

	cmd := exec.Command("node", scriptPath)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if rerr := cmd.Run(); rerr != nil {
		return 0, fmt.Errorf("recorder en vivo: %w", rerr)
	}

	n := countScreenshots(opts.screenshotsDir)
	if n == 0 {
		return 0, fmt.Errorf("no se capturó ninguna screenshot (¿cerraste sin grabar?)")
	}
	return n, nil
}

// finishLiveRecord deja el flow listo tras una captura en vivo: limpia el spec, guarda
// el meta y avisa. No hay replay ni detección de permisos por replay (las screenshots ya
// están alineadas 1:1 con las acciones; los permisos se toman de la config del proyecto).
func finishLiveRecord(
	e2eDir, slug, specPath string,
	screenshots int,
	huKeys []string,
	usesAuthSession bool,
) error {
	specContent, err := os.ReadFile(specPath)
	if err != nil {
		return ui.Fail("No se pudo leer el spec grabado: %s", err)
	}
	cleaned := stripStorageState(string(specContent))
	if err := os.WriteFile(specPath, []byte(cleaned), 0o644); err != nil {
		return ui.Fail("No se pudo escribir el spec: %s", err)
	}

	ui.PrintStep(fmt.Sprintf(
		"%d screenshot(s) capturada(s) en vivo en %s/",
		screenshots, filepath.Join(e2eDir, ".oraculo", slug),
	))

	if len(huKeys) > 0 || usesAuthSession {
		meta := flowMeta{JiraIssueKeys: huKeys, UsesAuthSession: usesAuthSession}
		if err := saveFlowMeta(e2eDir, slug, meta); err != nil {
			ui.PrintWarning(fmt.Sprintf("No se pudo guardar el meta del flow: %s", err))
		}
	}

	ui.PrintSuccess(fmt.Sprintf("Flow `%s` listo. Súbelo al backoffice con: oraculo push", slug))
	return nil
}
