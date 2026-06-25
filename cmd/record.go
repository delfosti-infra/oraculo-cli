package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/delfosti-infra/oraculo-cli/internal/api"
	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
	appconfig "github.com/delfosti-infra/oraculo-cli/internal/config"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

const stepFailMarker = "__ORACULO_STEP_FAIL__"

var recordHUFlag []string
var recordFreshFlag bool
var recordPlatformFlag string
var recordAppFlag string
var recordAppActivityFlag string

var recordCmd = &cobra.Command{
	Use:   "record <nombre>",
	Short: "Graba un flow con Playwright codegen y captura screenshots + trace",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawName := args[0]
		slug := slugify(rawName)
		if slug == "" {
			return ui.Fail("El nombre tiene que tener al menos un caracter alfanumérico.")
		}

		ui.PrintHeader(
			"RECORD",
			"Graba un flow con Playwright",
			"Abre codegen, hace los clicks y cierra el browser. Oráculo captura el trace y screenshots después.",
		)

		config, err := loadOraculoConfig()
		if err != nil {
			return ui.Fail("%s", err)
		}
		if config.BaseURL == "" {
			return ui.Fail("Falta el campo `base_url` en oraculo.config.json.")
		}

		if !playwrightTestResolvable() {
			ui.PrintWarning("Tu proyecto no tiene @playwright/test, necesario para grabar el flow.")
			if !ui.PromptYesNo("¿Lo instalo ahora con npm install -D @playwright/test? [S/n]", true) {
				printPlaywrightTestModuleHint()
				return ui.ErrAlreadyReported
			}
			if err := installPlaywrightTestPackage(); err != nil || !playwrightTestResolvable() {
				printPlaywrightTestModuleHint()
				return ui.ErrAlreadyReported
			}
			ui.PrintStep("@playwright/test instalado — sigo con la grabación")
		}

		e2eDir := config.E2EDir
		if e2eDir == "" {
			e2eDir = "e2e"
		}
		if err := os.MkdirAll(e2eDir, 0755); err != nil {
			return ui.Fail("No se pudo crear %s/: %s", e2eDir, err)
		}

		if mp := normalizePlatform(recordPlatformFlag); mp == "IOS" || mp == "ANDROID" {
			return recordMobile(e2eDir, slug, mp)
		}

		specPath := filepath.Join(e2eDir, slug+".spec.ts")
		tracePath := filepath.Join(e2eDir, slug+".trace.zip")
		screenshotsDir := filepath.Join(e2eDir, ".oraculo", slug)

		if _, err := os.Stat(specPath); err == nil {
			ui.PrintWarning(fmt.Sprintf("Ya existe un flow llamado `%s`.", slug))
			if !ui.PromptYesNo("¿Lo regrabas y reemplazas el actual? [s/N]", false) {
				ui.PrintHint("Sin cambios. Graba con otro nombre, o vuelve a correr y confirma para reemplazar.")
				return nil
			}
			removeFlowArtifacts(e2eDir, slug)
			ui.PrintStep(fmt.Sprintf("Reemplazando `%s` — se borraron spec, trace, screenshots y meta anteriores.", slug))
			ui.PrintHint("Si este flow ya está en el backoffice, 'oraculo push' te mostrará el diff y pedirá confirmar antes de reemplazarlo.")
		}

		huKeys := resolveHUsForRecord(recordHUFlag)

		if !recordFreshFlag && looksLikeLoginFlow(slug) {
			ui.PrintWarning("Parece que vas a grabar el flow de login. Con la sesión guardada el browser abre ya autenticado y el login no queda grabado.")
			if ui.PromptYesNo("¿Grabar con navegador limpio, sin la sesión guardada (--fresh)? [S/n]", true) {
				recordFreshFlag = true
			}
		}

		authStatePath, usesAuthSession := "", false
		if !recordFreshFlag {
			authStatePath, usesAuthSession = loadAuthSessionForRecord(config)
			if authStatePath != "" {
				defer os.Remove(authStatePath)
			}
		}

		ui.PrintStep(fmt.Sprintf("Abriendo codegen contra %s", config.BaseURL))

		codegenArgs := []string{
			"codegen",
			config.BaseURL,
			"--output", specPath,
			"--target", "playwright-test",
		}
		if authStatePath != "" {
			codegenArgs = append(codegenArgs, "--load-storage="+authStatePath)
		}
		runCodegen := func() (string, error) {
			var stderr bytes.Buffer
			codegen := exec.Command("npx", playwrightNpxArgs(codegenArgs...)...)
			codegen.Stdin = os.Stdin
			codegen.Stdout = os.Stdout
			codegen.Stderr = io.MultiWriter(os.Stderr, &stderr)
			err := codegen.Run()
			return stderr.String(), err
		}

		codegenStderr, codegenErr := runCodegen()
		if codegenErr != nil && playwrightBrowsersMissing(codegenStderr) {
			ui.PrintStep("Faltan los navegadores de Playwright — descargando chromium (una sola vez)…")
			if installErr := installPlaywrightBrowsers(); installErr != nil {
				printPlaywrightInstallHint("oraculo record")
				return ui.ErrAlreadyReported
			}
			codegenStderr, codegenErr = runCodegen()
		}
		if codegenErr != nil {
			if playwrightBrowsersMissing(codegenStderr) {
				printPlaywrightInstallHint("oraculo record")
				return ui.ErrAlreadyReported
			}
			return ui.Fail("Codegen no terminó bien: %s", codegenErr)
		}

		if _, err := os.Stat(specPath); err != nil {
			return ui.Fail("Codegen no generó spec. ¿Cerraste el browser sin grabar?")
		}
		ui.PrintStep("Spec generado: " + specPath)

		specContent, err := os.ReadFile(specPath)
		if err != nil {
			return ui.Fail("No se pudo leer el spec: %s", err)
		}

		specContent = []byte(cleanRecordedSpec(stripStorageState(string(specContent))))

		if err := os.MkdirAll(screenshotsDir, 0755); err != nil {
			return ui.Fail("No se pudo crear %s: %s", screenshotsDir, err)
		}
		_ = os.RemoveAll(screenshotsDir)
		_ = os.MkdirAll(screenshotsDir, 0755)

		absScreenshotsDir, err := filepath.Abs(screenshotsDir)
		if err != nil {
			absScreenshotsDir = screenshotsDir
		}

		instrumented, stepCount := instrumentSpec(string(specContent), absScreenshotsDir)

		if err := os.WriteFile(specPath, []byte(instrumented), 0644); err != nil {
			return ui.Fail("No se pudo escribir spec instrumentado: %s", err)
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
		configContent := buildPlaywrightConfig(absSpecPath, authStatePath, stepCount)
		if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
			spinner.Stop()
			return ui.Fail("No se pudo escribir config temporal: %s", err)
		}

		runTest := exec.Command(
			"npx",
			playwrightNpxArgs(
				"test",
				"--config", configPath,
				"--trace=on",
				"--reporter=line",
				"--workers=1",
			)...,
		)
		testOutput, testErr := runTest.CombinedOutput()
		spinner.Stop()

		if testErr != nil && playwrightBrowsersMissing(string(testOutput)) {
			printPlaywrightInstallHint("oraculo record")
			return ui.ErrAlreadyReported
		}

		screenshots := countScreenshots(screenshotsDir)
		if screenshots == 0 {
			ui.PrintError("No se capturaron screenshots. Output de Playwright:")
			fmt.Println(string(testOutput))
			return ui.ErrAlreadyReported
		}
		ui.PrintStep(fmt.Sprintf("%d screenshot(s) en %s/", screenshots, screenshotsDir))

		traces := findTraceFiles(outputDir)
		if len(traces) > 0 {
			if err := moveFile(traces[0], tracePath); err == nil {
				ui.PrintStep("Trace guardado: " + tracePath)
			}
		}

		if failedSteps := parseFailedSteps(string(testOutput)); len(failedSteps) > 0 {
			ui.PrintWarning(fmt.Sprintf(
				"%d paso(s) fallaron durante el replay. Sus screenshots muestran dónde quedó:",
				len(failedSteps),
			))
			for _, f := range failedSteps {
				ui.PrintHint(f)
			}
		} else if testErr != nil {
			ui.PrintWarning("El replay terminó con errores. Output de Playwright:")
			fmt.Println(string(testOutput))
		}

		platform := normalizePlatform(recordPlatformFlag)
		if recordPlatformFlag != "" && platform == "" {
			ui.PrintWarning(fmt.Sprintf(
				"Plataforma inválida '%s' (usá web, ios o android). Se ignora.",
				recordPlatformFlag,
			))
		}

		if len(huKeys) > 0 || usesAuthSession || platform != "" {
			meta := flowMeta{
				JiraIssueKeys:   huKeys,
				UsesAuthSession: usesAuthSession,
				Platform:        platform,
			}
			if err := saveFlowMeta(e2eDir, slug, meta); err != nil {
				ui.PrintWarning(fmt.Sprintf("No se pudo guardar el meta del flow: %s", err))
			}
		}

		ui.PrintSuccess(fmt.Sprintf("Flow `%s` listo. Súbelo al backoffice con: oraculo push", slug))
		return nil
	},
}

func removeFlowArtifacts(e2eDir, slug string) {
	_ = os.Remove(filepath.Join(e2eDir, slug+".spec.ts"))
	_ = os.Remove(filepath.Join(e2eDir, slug+".trace.zip"))
	_ = os.RemoveAll(filepath.Join(e2eDir, ".oraculo", slug))
	_ = os.Remove(metaPath(e2eDir, slug))
}

func recorderCommand() []string {
	if custom := strings.TrimSpace(os.Getenv("ORACULO_RECORDER_CMD")); custom != "" {
		return strings.Fields(custom)
	}
	return []string{"npx", "--yes", "oraculo-mobile-recorder@latest"}
}

func recordMobile(e2eDir, slug, platform string) error {
	app := strings.TrimSpace(recordAppFlag)
	if app == "" {
		return ui.Fail("Para grabar un flow móvil necesitás --app (appPackage/bundleId o ruta al .apk/.ipa).")
	}

	outPath := filepath.Join(e2eDir, slug+".mobile.json")
	if _, err := os.Stat(outPath); err == nil {
		ui.PrintWarning(fmt.Sprintf("Ya existe un flow móvil `%s`.", slug))
		if !ui.PromptYesNo("¿Lo regrabás y reemplazás el actual? [s/N]", false) {
			ui.PrintHint("Sin cambios. Graba con otro nombre o confirma para reemplazar.")
			return nil
		}
	}

	recorder := recorderCommand()
	args := append([]string{}, recorder[1:]...)
	args = append(args,
		"--platform", strings.ToLower(platform),
		"--app", app,
		"--out", outPath,
	)
	if recordAppActivityFlag != "" {
		args = append(args, "--app-activity", recordAppActivityFlag)
	}
	if device := os.Getenv("APPIUM_DEVICE_NAME"); device != "" {
		args = append(args, "--device", device)
	}
	if appium := os.Getenv("APPIUM_URL"); appium != "" {
		args = append(args, "--appium", appium)
	}

	ui.PrintStep("Abriendo el grabador móvil — tocá elementos en la pantalla del device para armar el flow")
	rec := exec.Command(recorder[0], args...)
	rec.Stdin = os.Stdin
	rec.Stdout = os.Stdout
	rec.Stderr = os.Stderr
	if err := rec.Run(); err != nil {
		ui.PrintHint("¿El grabador no está instalado? Definí ORACULO_RECORDER_CMD apuntando al recorder del worker.")
		return ui.Fail("El grabador móvil terminó con error: %s", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		return ui.Fail("El grabador no generó el spec móvil (¿cerraste sin guardar?).")
	}

	meta := flowMeta{JiraIssueKeys: resolveHUsForRecord(recordHUFlag), Platform: platform}
	if err := saveFlowMeta(e2eDir, slug, meta); err != nil {
		ui.PrintWarning(fmt.Sprintf("No se pudo guardar el meta del flow: %s", err))
	}

	ui.PrintSuccess(fmt.Sprintf("Flow móvil `%s` listo. Súbelo al backoffice con: oraculo push", slug))
	return nil
}

func instrumentSpec(content, screenshotsDir string) (string, int) {
	lines := strings.Split(content, "\n")
	output := make([]string, 0, len(lines)*3)
	stepCounter := 0

	gotoRegex := regexp.MustCompile(`page\.goto\(`)
	submitRegex := regexp.MustCompile(`getByRole\(['"` + "`" + `]button['"` + "`" + `]`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !isWrappableAction(trimmed) {
			output = append(output, line)
			continue
		}

		stepCounter++
		indent := leadingWhitespace(line)

		failLog := func(errVar string) string {
			return fmt.Sprintf(
				"console.log('%s %d: ' + (%s instanceof Error ? %s.message : String(%s)).split('\\n')[0]);",
				stepFailMarker, stepCounter, errVar, errVar, errVar,
			)
		}

		output = append(output, indent+"try {", indent+"  "+trimmed)
		if retry := retryFirstExpr(trimmed); retry != "" {
			output = append(output,
				indent+"} catch (__oraculoErr) {",
				indent+"  if (/strict mode violation/i.test(String(__oraculoErr))) {",
				indent+"    try { "+retry+" } catch (__oraculoErr2) { "+failLog("__oraculoErr2")+" }",
				indent+"  } else { "+failLog("__oraculoErr")+" }",
				indent+"}",
			)
		} else {
			output = append(output, fmt.Sprintf("%s} catch (__oraculoErr) { %s }", indent, failLog("__oraculoErr")))
		}

		if gotoRegex.MatchString(line) {
			output = append(output,
				indent+"await page.waitForLoadState('domcontentloaded').catch(() => {});",
				indent+"await page.waitForLoadState('networkidle', { timeout: 3000 }).catch(() => {});",
			)
		} else if submitRegex.MatchString(line) {
			output = append(output,
				indent+"await page.waitForLoadState('networkidle', { timeout: 3000 }).catch(() => {});",
			)
		}

		output = append(output, indent+"await page.waitForTimeout(400);")

		screenshotPath := filepath.Join(screenshotsDir, fmt.Sprintf("step-%d.png", stepCounter))
		output = append(output, fmt.Sprintf(
			"%sawait page.screenshot({ path: %q, fullPage: false });",
			indent, screenshotPath,
		))
	}
	return strings.Join(output, "\n"), stepCounter
}

func isWrappableAction(trimmed string) bool {
	return strings.HasSuffix(trimmed, ";") && strings.HasPrefix(trimmed, "await ")
}

func retryFirstExpr(trimmed string) string {
	m := recordedActionRegex.FindStringSubmatch(trimmed)
	if m == nil {
		return ""
	}
	locator, method, args := m[2], m[3], m[4]
	if strings.Contains(locator, ".first(") || strings.Contains(locator, ".last(") || strings.Contains(locator, ".nth(") {
		return ""
	}
	return fmt.Sprintf("%s.first().%s(%s);", locator, method, args)
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

func parseFailedSteps(output string) []string {
	var failed []string
	for _, line := range strings.Split(output, "\n") {
		idx := strings.Index(line, stepFailMarker)
		if idx < 0 {
			continue
		}
		if strings.Contains(line, "console.log(") || strings.Contains(line, "__oraculoErr") {
			continue
		}
		rest := strings.TrimSpace(line[idx+len(stepFailMarker):])
		if rest == "" {
			continue
		}
		failed = append(failed, "Paso "+rest)
	}
	return failed
}

var playwrightMissingBrowserRegex = regexp.MustCompile(
	`(?i)Executable doesn't exist|playwright install`,
)

func playwrightBrowsersMissing(output string) bool {
	return playwrightMissingBrowserRegex.MatchString(output)
}

func installPlaywrightBrowsers() error {
	cmd := exec.Command("npx", playwrightNpxArgs("install", "chromium")...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printPlaywrightInstallHint(retryCmd string) {
	ui.PrintError("Los navegadores de Playwright no están instalados.")
	ui.PrintHint("Instálalos con:  npx playwright install chromium")
	ui.PrintHint("Después vuelve a correr:  " + retryCmd)
}

func playwrightTestResolvable() bool {
	cmd := exec.Command("node", "-e", "require.resolve('@playwright/test')")
	return cmd.Run() == nil
}

var loginFlowSlugRegex = regexp.MustCompile(`(^|-)(log-?in|sign-?in|inicio-de-sesion|iniciar-sesion|inicio-sesion)($|-)`)

func looksLikeLoginFlow(slug string) bool {
	return loginFlowSlugRegex.MatchString(slug)
}

func installPlaywrightTestPackage() error {
	cmd := exec.Command("npm", "install", "-D", "@playwright/test")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printPlaywrightTestModuleHint() {
	ui.PrintError("Falta el paquete @playwright/test en tu proyecto.")
	ui.PrintHint("Instálalo en la raíz del proyecto (donde está oraculo.config.json) con:")
	ui.PrintHint("  npm install -D @playwright/test")
	ui.PrintHint("Después vuelve a correr:  oraculo record")
}

func playwrightNpxArgs(sub ...string) []string {
	return append([]string{"--yes", "--package", "@playwright/test", "playwright"}, sub...)
}

func loadOraculoConfig() (*types.Config, error) {
	data, err := os.ReadFile("oraculo.config.json")
	if err != nil {
		return nil, fmt.Errorf("no se encontró oraculo.config.json. Corre 'oraculo init' primero")
	}
	var c types.Config
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

var recordedActionRegex = regexp.MustCompile(
	`^(\s*)(await\s+.+?)\.(fill|click|dblclick|press|selectOption|check|uncheck)\((.*)\);\s*$`,
)

func cleanRecordedSpec(spec string) string {
	lines := strings.Split(spec, "\n")
	out := make([]string, 0, len(lines))

	lastFillIdx := -1
	lastFillLocator := ""

	for _, line := range lines {
		m := recordedActionRegex.FindStringSubmatch(line)
		if m != nil {
			locator, method, args := m[2], m[3], m[4]

			if method == "press" && strings.Contains(args, "CapsLock") {
				continue
			}

			if method == "fill" && lastFillIdx >= 0 && locator == lastFillLocator {
				out[lastFillIdx] = line
				continue
			}
			if method == "fill" {
				out = append(out, line)
				lastFillIdx = len(out) - 1
				lastFillLocator = locator
				continue
			}
		}

		lastFillIdx = -1
		lastFillLocator = ""
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

const (
	replayBaseTimeoutMs    = 60_000
	replayPerStepTimeoutMs = 4_000
)

func buildPlaywrightConfig(absSpecPath, authStatePath string, stepCount int) string {
	specDir := filepath.Dir(absSpecPath)
	specName := filepath.Base(absSpecPath)
	timeoutMs := replayBaseTimeoutMs + stepCount*replayPerStepTimeoutMs
	storageLine := ""
	if authStatePath != "" {
		storageLine = fmt.Sprintf("\n    storageState: %q,", authStatePath)
	}
	return fmt.Sprintf(`import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: %q,
  testMatch: %q,
  timeout: %d,
  retries: 0,
  workers: 1,
  use: {
    headless: true,
    actionTimeout: 10_000,
    navigationTimeout: 20_000,
    ignoreHTTPSErrors: true,%s
  },
});
`, specDir, specName, timeoutMs, storageLine)
}

func loadAuthSessionForRecord(config *types.Config) (string, bool) {
	if config.RefId == "" || config.APIURL == "" {
		ui.PrintWarning("Sin `refId`/`api_url` en config — grabando sin sesión. Usa --fresh para silenciar.")
		return "", false
	}

	token, err := appconfig.LoadToken()
	if err != nil {
		ui.PrintWarning("No estás logueado — grabando sin sesión. Corre `oraculo login` + `oraculo auth`, o usa --fresh.")
		return "", false
	}

	apiClient := api.NewClient(strings.TrimRight(config.APIURL, "/"))
	state, err := apiClient.GetAuthSessionState(token, config.RefId)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("No se pudo obtener la sesión (%s) — grabando sin sesión.", err))
		return "", false
	}
	if state == nil {
		ui.PrintWarning("El proyecto no tiene sesión guardada — grabando sin login. Captúrala con `oraculo auth`.")
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

	ui.PrintStep("Sesión del proyecto cargada — grabas ya autenticado (sin login)")
	ui.PrintHint("¿Este flow ES el login? Córtalo (Ctrl+C) y vuelve a correr con --fresh para grabar con navegador limpio.")
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

func normalizePlatform(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "web":
		return "WEB"
	case "ios":
		return "IOS"
	case "android":
		return "ANDROID"
	default:
		return ""
	}
}

func registerRecordFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(
		&recordHUFlag,
		"HU",
		nil,
		"HU(s) de Jira asociadas a este flow (ej. --HU=KDP0-6 --HU=KDP0-7). Si no se especifica, intenta detectar desde la branch.",
	)
	cmd.Flags().BoolVar(
		&recordFreshFlag,
		"fresh",
		false,
		"Graba sin la sesión guardada del proyecto (navegador limpio). Úsalo para el flow de login.",
	)
	cmd.Flags().StringVar(
		&recordPlatformFlag,
		"platform",
		"",
		"Plataforma del flow: web (default), ios o android.",
	)
	cmd.Flags().StringVar(
		&recordAppFlag,
		"app",
		"",
		"(móvil) appPackage/bundleId o ruta al .apk/.ipa a grabar.",
	)
	cmd.Flags().StringVar(
		&recordAppActivityFlag,
		"app-activity",
		"",
		"(móvil, Android) activity de arranque, si la app la requiere.",
	)
}

func init() {
	registerRecordFlags(recordCmd)
	rootCmd.AddCommand(recordCmd)
}
