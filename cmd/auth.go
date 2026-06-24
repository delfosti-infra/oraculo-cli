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

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Captura y guarda la sesión autenticada del proyecto (storageState)",
	Args:  cobra.NoArgs,
	Long: "Abre un browser contra el base_url del proyecto: inicia sesión y ciérralo.\n" +
		"Oráculo guarda la sesión (cookies + localStorage) cifrada en el core para\n" +
		"que puedas grabar y correr flows ya autenticado, sin grabar el login.",
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
		"Abre un browser; inicia sesión y ciérralo. La sesión queda cifrada para grabar/correr autenticado.",
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

	ui.PrintStep(fmt.Sprintf("Abriendo browser contra %s — inicia sesión y ciérralo", config.BaseURL))

	runOpen := func() (string, error) {
		var stderr bytes.Buffer
		open := exec.Command("npx", playwrightNpxArgs("open", "--save-storage="+tmpPath, config.BaseURL)...)
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
		return ui.Fail("Playwright no terminó bien: %s", openErr)
	}

	stateData, err := os.ReadFile(tmpPath)
	if err != nil || len(stateData) == 0 {
		return ui.Fail("No se capturó la sesión. ¿Cerraste el browser sin loguearte?")
	}

	var probe map[string]any
	if err := json.Unmarshal(stateData, &probe); err != nil {
		return ui.Fail("El storageState capturado no es JSON válido.")
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
		"Sesión '%s' guardada para '%s' (%s). Graba con `oraculo record <nombre>` sin el login.",
		meta.Label, config.Project, expiry,
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
			"Sesión '%s' (capturada %s) EXPIRADA — refrescala con `oraculo auth`.",
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
