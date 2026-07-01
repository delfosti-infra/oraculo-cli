package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/delfosti-infra/oraculo-cli/internal/api"
	appconfig "github.com/delfosti-infra/oraculo-cli/internal/config"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	authLabelFlag         string
	authExpiresInDaysFlag int
)

type capturedStorageState struct {
	Cookies []json.RawMessage `json:"cookies"`
	Origins []struct {
		LocalStorage []json.RawMessage `json:"localStorage"`
	} `json:"origins"`
}

const authCaptureScript = `const readline = require('readline');

(async () => {
  const url = process.argv[2];
  const out = process.argv[3];
  if (!url || !out) { console.error('faltan argumentos url/out'); process.exit(1); }

  let chromium;
  try {
    ({ chromium } = require(require.resolve('@playwright/test', { paths: [process.cwd()] })));
  } catch (e) {
    console.error('MODULE_NOT_FOUND @playwright/test');
    process.exit(3);
  }

  const launchOpts = {
    headless: false,
    ignoreDefaultArgs: ['--enable-automation'],
    args: ['--disable-blink-features=AutomationControlled'],
  };

  let browser;
  try {
    browser = await chromium.launch(Object.assign({ channel: 'chrome' }, launchOpts));
  } catch (e) {
    browser = await chromium.launch(launchOpts);
  }

  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(url, { waitUntil: 'domcontentloaded' }).catch(() => {});

  console.log('\n  >> Inicia sesion en el navegador. Cuando termines, vuelve a esta terminal y presiona ENTER (no cierres el navegador).\n');

  const rl = readline.createInterface({ input: process.stdin });
  await Promise.race([
    new Promise((resolve) => rl.once('line', resolve)),
    new Promise((resolve) => browser.once('disconnected', resolve)),
  ]);
  rl.close();

  if (browser.isConnected()) {
    await context.storageState({ path: out }).catch(() => {});
    await browser.close().catch(() => {});
  }
})().catch((e) => { console.error(String((e && e.message) || e)); process.exit(1); });
`

func writeAuthCaptureScript() (string, func(), error) {
	noop := func() {}
	f, err := os.CreateTemp("", "oraculo-auth-capture-*.cjs")
	if err != nil {
		return "", noop, fmt.Errorf("crear script de captura: %w", err)
	}
	if _, err := f.WriteString(authCaptureScript); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", noop, fmt.Errorf("escribir script de captura: %w", err)
	}
	f.Close()
	return f.Name(), func() { os.Remove(f.Name()) }, nil
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Captura y guarda la sesión autenticada del proyecto (storageState)",
	Args:  cobra.NoArgs,
	Long: "Abre un navegador contra el base_url del proyecto: inicia sesión y vuelve\n" +
		"a la terminal a presionar ENTER. Oráculo guarda la sesión (cookies +\n" +
		"localStorage) cifrada en el core para grabar/correr autenticado, sin login.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthCapture()
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Muestra el estado de la sesión guardada del proyecto",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthStatus()
	},
}

var authClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Borra la sesión guardada del proyecto",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthClear()
	},
}

func runAuthCapture() error {
	ui.PrintHeader(
		"AUTH",
		"Captura la sesión del proyecto",
		"Abre un navegador; inicia sesión y presiona ENTER en la terminal. La sesión queda cifrada para grabar/correr autenticado.",
	)

	config, err := loadOraculoConfig()
	if err != nil {
		return ui.Fail("%s", err)
	}
	if config.BaseURL == "" {
		return ui.Fail("Falta el campo `base_url` en oraculo.config.json.")
	}
	if config.RefId == "" {
		return ui.Fail("Falta el campo `refId` en oraculo.config.json. Corre 'oraculo init'.")
	}
	if config.APIURL == "" {
		return ui.Fail("Falta el campo `api_url` en oraculo.config.json.")
	}

	token, err := appconfig.LoadToken()
	if err != nil {
		return ui.Fail("%s", err)
	}

	tmp, err := os.CreateTemp("", "oraculo-auth-*.json")
	if err != nil {
		return ui.Fail("No se pudo crear archivo temporal: %s", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if !playwrightTestResolvable() {
		ui.PrintWarning("Tu proyecto no tiene @playwright/test, necesario para capturar la sesión.")
		if !ui.PromptYesNo("¿Lo instalo ahora con npm install -D @playwright/test? [S/n]", true) {
			printPlaywrightTestModuleHint()
			return ui.ErrAlreadyReported
		}
		if err := installPlaywrightTestPackage(); err != nil || !playwrightTestResolvable() {
			printPlaywrightTestModuleHint()
			return ui.ErrAlreadyReported
		}
		ui.PrintStep("@playwright/test instalado — sigo con la captura")
	}

	scriptPath, cleanup, err := writeAuthCaptureScript()
	if err != nil {
		return ui.Fail("%s", err)
	}
	defer cleanup()

	ui.PrintStep(fmt.Sprintf("Abriendo Chrome contra %s — inicia sesión y luego presiona ENTER aquí", config.BaseURL))
	ui.PrintHint("Se abre tu Chrome real, sin marcas de automatización, para que el login de Google/GitHub no lo bloquee.")

	runOpen := func() (string, error) {
		var stderr bytes.Buffer
		open := exec.Command("node", scriptPath, config.BaseURL, tmpPath)
		open.Stdin = os.Stdin
		open.Stdout = os.Stdout
		open.Stderr = io.MultiWriter(os.Stderr, &stderr)
		err := open.Run()
		return stderr.String(), err
	}

	openStderr, openErr := runOpen()
	if openErr != nil && playwrightBrowsersMissing(openStderr) {
		ui.PrintStep("Faltan los navegadores de Playwright — descargando chromium (una sola vez)…")
		if installErr := installPlaywrightBrowsers(); installErr != nil {
			printPlaywrightInstallHint("oraculo auth")
			return ui.ErrAlreadyReported
		}
		openStderr, openErr = runOpen()
	}
	if openErr != nil {
		if playwrightBrowsersMissing(openStderr) {
			printPlaywrightInstallHint("oraculo auth")
			return ui.ErrAlreadyReported
		}
		if strings.Contains(openStderr, "MODULE_NOT_FOUND") {
			printPlaywrightTestModuleHint()
			return ui.ErrAlreadyReported
		}
		return ui.Fail("El navegador de captura no terminó bien: %s", openErr)
	}

	stateData, err := os.ReadFile(tmpPath)
	if err != nil || len(stateData) == 0 {
		return ui.Fail("No se capturó la sesión. ¿Presionaste ENTER sin iniciar sesión, o cerraste el navegador antes de tiempo?")
	}

	var state capturedStorageState
	if err := json.Unmarshal(stateData, &state); err != nil {
		return ui.Fail("El storageState capturado no es JSON válido.")
	}

	localStorageCount := 0
	for _, origin := range state.Origins {
		localStorageCount += len(origin.LocalStorage)
	}
	if len(state.Cookies) == 0 && localStorageCount == 0 {
		return ui.Fail(
			"No se detectó ninguna sesión (0 cookies, 0 localStorage). " +
				"¿Iniciaste sesión antes de presionar ENTER? Corre `oraculo auth` de nuevo, inicia sesión y luego presiona ENTER.",
		)
	}

	apiClient := api.NewClient(strings.TrimRight(config.APIURL, "/"))
	meta, err := apiClient.PutAuthSession(
		token,
		config.RefId,
		json.RawMessage(stateData),
		authLabelFlag,
		authExpiresInDaysFlag,
	)
	if err != nil {
		return ui.Fail("No se pudo guardar la sesión: %s", err)
	}

	expiry := "sin expiración"
	if meta.ExpiresAt != nil {
		expiry = "expira " + *meta.ExpiresAt
	}
	ui.PrintSuccess(fmt.Sprintf(
		"Sesión '%s' guardada para '%s' (%s) · %d cookies, %d entradas de localStorage. "+
			"Graba con `oraculo record <nombre>` sin el login.",
		meta.Label, config.Project, expiry, len(state.Cookies), localStorageCount,
	))
	return nil
}

func runAuthStatus() error {
	config, err := loadOraculoConfig()
	if err != nil {
		return ui.Fail("%s", err)
	}
	if config.RefId == "" || config.APIURL == "" {
		return ui.Fail("Faltan `refId` o `api_url` en oraculo.config.json. Corre 'oraculo init'.")
	}

	token, err := appconfig.LoadToken()
	if err != nil {
		return ui.Fail("%s", err)
	}

	apiClient := api.NewClient(strings.TrimRight(config.APIURL, "/"))
	meta, err := apiClient.GetAuthSessionStatus(token, config.RefId)
	if err != nil {
		return ui.Fail("%s", err)
	}
	if meta == nil {
		ui.PrintStep(fmt.Sprintf(
			"El proyecto '%s' no tiene sesión guardada. Captúrala con `oraculo auth`.",
			config.Project,
		))
		return nil
	}

	expiry := "sin expiración"
	if meta.ExpiresAt != nil {
		expiry = "expira " + *meta.ExpiresAt
	}
	if meta.Expired {
		ui.PrintWarning(fmt.Sprintf(
			"Sesión '%s' (capturada %s) EXPIRADA — refréscala con `oraculo auth`.",
			meta.Label, meta.CapturedAt,
		))
		return nil
	}
	ui.PrintStep(fmt.Sprintf(
		"Sesión '%s' activa · capturada %s · %s",
		meta.Label, meta.CapturedAt, expiry,
	))
	return nil
}

func runAuthClear() error {
	config, err := loadOraculoConfig()
	if err != nil {
		return ui.Fail("%s", err)
	}
	if config.RefId == "" || config.APIURL == "" {
		return ui.Fail("Faltan `refId` o `api_url` en oraculo.config.json. Corre 'oraculo init'.")
	}

	token, err := appconfig.LoadToken()
	if err != nil {
		return ui.Fail("%s", err)
	}

	apiClient := api.NewClient(strings.TrimRight(config.APIURL, "/"))
	if err := apiClient.DeleteAuthSession(token, config.RefId); err != nil {
		return ui.Fail("%s", err)
	}
	ui.PrintSuccess(fmt.Sprintf("Sesión del proyecto '%s' borrada.", config.Project))
	return nil
}

func init() {
	authCmd.Flags().StringVar(
		&authLabelFlag,
		"label",
		"",
		"Etiqueta para la sesión (ej. --label=admin)",
	)
	authCmd.Flags().IntVar(
		&authExpiresInDaysFlag,
		"expires-in-days",
		0,
		"Días hasta que la sesión expire (1-365; sin flag = no expira)",
	)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authClearCmd)
	rootCmd.AddCommand(authCmd)
}
