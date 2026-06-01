package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/delfosti/oraculo-cli/internal/api"
	"github.com/delfosti/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var actionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`page\.goto\(`),
	regexp.MustCompile(`getByLabel\([^)]+\)\.fill\(`),
	regexp.MustCompile(`getByRole\(['"` + "`" + `]\w+['"` + "`" + `],\s*\{[^}]+\}\)\.fill\(`),
	regexp.MustCompile(`getByPlaceholder\(['"` + "`" + `][^'"` + "`" + `]+['"` + "`" + `]\)\.fill\(`),
	regexp.MustCompile(`page\.fill\(['"` + "`" + `]`),
	regexp.MustCompile(`getByRole\(['"` + "`" + `]\w+['"` + "`" + `],\s*\{[^}]+\}\)\.click\(\)`),
	regexp.MustCompile(`getByText\(['"` + "`" + `][^'"` + "`" + `]+['"` + "`" + `]\)\.click\(\)`),
	regexp.MustCompile(`page\.click\(['"` + "`" + `]`),
	regexp.MustCompile(`\.press\(['"` + "`" + `]`),
	regexp.MustCompile(`getByLabel\([^)]+\)\.selectOption\(`),
	regexp.MustCompile(`page\.waitForSelector\(['"` + "`" + `]`),
}

var recordHUFlag []string
var recordFreshFlag bool

var recordCmd = &cobra.Command{
	Use:   "record <nombre>",
	Short: "Graba un flow con Playwright codegen y captura screenshots + trace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawName := args[0]
		slug := slugify(rawName)
		if slug == "" {
			ui.PrintError("El nombre tiene que tener al menos un caracter alfanumérico.")
			return nil
		}

		ui.PrintHeader(
			"RECORD",
			"Graba un flow con Playwright",
			"Abre codegen, hace los clicks y cierra el browser. Oráculo captura el trace y screenshots después.",
		)

		// Resolver HUs asociadas al flow (--HU explícito o auto-detect desde branch).
		// Lo hacemos ANTES del codegen así no esperás 5 minutos para enterarte que la branch no tenía key.
		huKeys := resolveHUsForRecord(recordHUFlag)

		config, err := loadOraculoConfig()
		if err != nil {
			ui.PrintError(err.Error())
			return nil
		}
		if config.BaseURL == "" {
			ui.PrintError("Falta el campo `base_url` en oraculo.config.json.")
			return nil
		}

		e2eDir := config.E2EDir
		if e2eDir == "" {
			e2eDir = "e2e"
		}
		if err := os.MkdirAll(e2eDir, 0755); err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo crear %s/: %s", e2eDir, err))
			return nil
		}

		specPath := filepath.Join(e2eDir, slug+".spec.ts")
		tracePath := filepath.Join(e2eDir, slug+".trace.zip")
		screenshotsDir := filepath.Join(e2eDir, ".oraculo", slug)

		if _, err := os.Stat(specPath); err == nil {
			ui.PrintError(fmt.Sprintf("Ya existe %s. Borralo o usa otro nombre.", specPath))
			return nil
		}

		// Cargar la sesión guardada del proyecto para grabar ya autenticado (sin
		// grabar el login). Con --fresh se omite (ej. para grabar el flow de login).
		authStatePath, usesAuthSession := "", false
		if !recordFreshFlag {
			authStatePath, usesAuthSession = loadAuthSessionForRecord(config)
			if authStatePath != "" {
				defer os.Remove(authStatePath)
			}
		}

		ui.PrintStep(fmt.Sprintf("Abriendo codegen contra %s", config.BaseURL))

		codegenArgs := []string{
			"playwright",
			"codegen",
			config.BaseURL,
			"--output", specPath,
			"--target", "playwright-test",
		}
		if authStatePath != "" {
			codegenArgs = append(codegenArgs, "--load-storage="+authStatePath)
		}
		codegen := exec.Command("npx", codegenArgs...)
		codegen.Stdin = os.Stdin
		codegen.Stdout = os.Stdout
		codegen.Stderr = os.Stderr
		if err := codegen.Run(); err != nil {
			ui.PrintError(fmt.Sprintf("Codegen no terminó bien: %s", err))
			return nil
		}

		if _, err := os.Stat(specPath); err != nil {
			ui.PrintError("Codegen no generó spec. ¿Cerraste el browser sin grabar?")
			return nil
		}
		ui.PrintStep("Spec generado: " + specPath)

		specContent, err := os.ReadFile(specPath)
		if err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo leer el spec: %s", err))
			return nil
		}

		specContent = []byte(stripStorageState(string(specContent)))

		if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo crear %s: %s", screenshotsDir, err))
			return nil
		}
		_ = os.RemoveAll(screenshotsDir)
		_ = os.MkdirAll(screenshotsDir, 0755)

		absScreenshotsDir, err := filepath.Abs(screenshotsDir)
		if err != nil {
			absScreenshotsDir = screenshotsDir
		}

		instrumented := instrumentSpec(string(specContent), absScreenshotsDir)

		if err := os.WriteFile(specPath, []byte(instrumented), 0644); err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo escribir spec instrumentado: %s", err))
			return nil
		}
		defer func() {
			_ = os.WriteFile(specPath, specContent, 0644)
		}()

		spinner := ui.NewSpinner("Reproduciendo el flow para capturar screenshots y trace...")
		spinner.Start()

		outputDir := filepath.Join(e2eDir, ".oraculo", "runner-"+slug)
		defer os.RemoveAll(outputDir)
		_ = os.MkdirAll(outputDir, 0700)

		absSpecPath, _ := filepath.Abs(specPath)
		configPath := filepath.Join(outputDir, "playwright.config.ts")
		configContent := buildPlaywrightConfig(absSpecPath, authStatePath)
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			spinner.Stop()
			ui.PrintError(fmt.Sprintf("No se pudo escribir config temporal: %s", err))
			return nil
		}

		runTest := exec.Command(
			"npx",
			"playwright",
			"test",
			"--config", configPath,
			"--trace=on",
			"--reporter=line",
			"--workers=1",
		)
		testOutput, testErr := runTest.CombinedOutput()
		spinner.Stop()

		screenshots := countScreenshots(screenshotsDir)
		if screenshots == 0 {
			ui.PrintError("No se capturaron screenshots. Output de Playwright:")
			fmt.Println(string(testOutput))
			return nil
		}
		ui.PrintStep(fmt.Sprintf("%d screenshot(s) en %s/", screenshots, screenshotsDir))

		traces := findTraceFiles(outputDir)
		if len(traces) > 0 {
			if err := moveFile(traces[0], tracePath); err == nil {
				ui.PrintStep("Trace guardado: " + tracePath)
			}
		}

		if testErr != nil {
			ui.PrintWarning("El spec corrió con errores. Los artefactos tienen los pasos hasta donde llegó.")
		}

		if len(huKeys) > 0 || usesAuthSession {
			meta := flowMeta{JiraIssueKeys: huKeys, UsesAuthSession: usesAuthSession}
			if err := saveFlowMeta(e2eDir, slug, meta); err != nil {
				ui.PrintWarning(fmt.Sprintf("No se pudo guardar el meta del flow: %s", err))
			}
		}

		ui.PrintSuccess(fmt.Sprintf("Flow `%s` listo. Subilo al backoffice con: oraculo push", slug))
		return nil
	},
}

func instrumentSpec(content, screenshotsDir string) string {
	lines := strings.Split(content, "\n")
	output := make([]string, 0, len(lines)*3)
	stepCounter := 0

	gotoRegex := regexp.MustCompile(`page\.goto\(`)
	submitRegex := regexp.MustCompile(`getByRole\(['"` + "`" + `]button['"` + "`" + `]`)

	for _, line := range lines {
		output = append(output, line)

		for _, p := range actionPatterns {
			if !p.MatchString(line) {
				continue
			}
			stepCounter++
			indent := leadingWhitespace(line)

			if gotoRegex.MatchString(line) {
				output = append(output, fmt.Sprintf(
					"%sawait page.waitForLoadState('domcontentloaded').catch(() => {});",
					indent,
				))
				output = append(output, fmt.Sprintf(
					"%sawait page.waitForLoadState('networkidle', { timeout: 3000 }).catch(() => {});",
					indent,
				))
			} else if submitRegex.MatchString(line) {
				output = append(output, fmt.Sprintf(
					"%sawait page.waitForLoadState('networkidle', { timeout: 3000 }).catch(() => {});",
					indent,
				))
			}

			output = append(output, fmt.Sprintf(
				"%sawait page.waitForTimeout(400);",
				indent,
			))

			screenshotPath := filepath.Join(screenshotsDir, fmt.Sprintf("step-%d.png", stepCounter))
			output = append(output, fmt.Sprintf(
				"%sawait page.screenshot({ path: %q, fullPage: false });",
				indent, screenshotPath,
			))
			break
		}
	}
	return strings.Join(output, "\n")
}

func leadingWhitespace(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}

func countScreenshots(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "step-") && strings.HasSuffix(e.Name(), ".png") {
			count++
		}
	}
	return count
}

func loadOraculoConfig() (*Config, error) {
	data, err := os.ReadFile("oraculo.config.json")
	if err != nil {
		return nil, fmt.Errorf("no se encontró oraculo.config.json. Corré 'oraculo init' primero")
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("oraculo.config.json inválido: %w", err)
	}
	return &c, nil
}

var slugifyRegex = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	out := strings.ToLower(s)
	out = slugifyRegex.ReplaceAllString(out, "-")
	out = strings.Trim(out, "-")
	return out
}

func findTraceFiles(dir string) []string {
	var found []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), "trace.zip") {
			found = append(found, path)
		}
		return nil
	})
	return found
}

var storageStateUseRegex = regexp.MustCompile(`\n?[ \t]*test\.use\(\{[^{}]*storageState[^{}]*\}\);[ \t]*\n?`)

func stripStorageState(spec string) string {
	return storageStateUseRegex.ReplaceAllString(spec, "\n")
}

func buildPlaywrightConfig(absSpecPath, authStatePath string) string {
	specDir := filepath.Dir(absSpecPath)
	specName := filepath.Base(absSpecPath)
	storageLine := ""
	if authStatePath != "" {
		storageLine = fmt.Sprintf("\n    storageState: %q,", authStatePath)
	}
	return fmt.Sprintf(`import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: %q,
  testMatch: %q,
  timeout: 60_000,
  retries: 0,
  workers: 1,
  use: {
    headless: true,
    actionTimeout: 10_000,
    navigationTimeout: 20_000,
    ignoreHTTPSErrors: true,%s
  },
});
`, specDir, specName, storageLine)
}

// loadAuthSessionForRecord baja el storageState guardado del proyecto a un archivo
// temporal para grabar y reproducir ya autenticado. Devuelve ("", false) y avisa
// si no estás logueado, el proyecto no tiene sesión, o falla la descarga — en ese
// caso se graba sin sesión (best-effort, no rompe el record).
func loadAuthSessionForRecord(config *Config) (string, bool) {
	if config.RefId == "" || config.APIURL == "" {
		ui.PrintWarning("Sin `refId`/`api_url` en config — grabando sin sesión. Usá --fresh para silenciar.")
		return "", false
	}

	token, err := loadToken()
	if err != nil {
		ui.PrintWarning("No estás logueado — grabando sin sesión. Corré `oraculo login` + `oraculo auth`, o usá --fresh.")
		return "", false
	}

	apiClient := api.NewClient(strings.TrimRight(config.APIURL, "/"))
	state, err := apiClient.GetAuthSessionState(token, config.RefId)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("No se pudo obtener la sesión (%s) — grabando sin sesión.", err))
		return "", false
	}
	if state == nil {
		ui.PrintWarning("El proyecto no tiene sesión guardada — grabando sin login. Capturala con `oraculo auth`.")
		return "", false
	}

	tmp, err := os.CreateTemp("", "oraculo-record-auth-*.json")
	if err != nil {
		ui.PrintWarning("No se pudo preparar la sesión — grabando sin sesión.")
		return "", false
	}
	if _, err := tmp.Write(state); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		ui.PrintWarning("No se pudo escribir la sesión — grabando sin sesión.")
		return "", false
	}
	tmp.Close()

	ui.PrintStep("Sesión del proyecto cargada — grabás ya autenticado (sin login)")
	return tmp.Name(), true
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func init() {
	recordCmd.Flags().StringSliceVar(
		&recordHUFlag,
		"HU",
		nil,
		"HU(s) de Jira asociadas a este flow (ej. --HU=KDP0-6 --HU=KDP0-7). Si no se especifica, intenta detectar desde la branch.",
	)
	recordCmd.Flags().BoolVar(
		&recordFreshFlag,
		"fresh",
		false,
		"Graba sin la sesión guardada del proyecto (navegador limpio). Usalo para el flow de login.",
	)
	rootCmd.AddCommand(recordCmd)
}
