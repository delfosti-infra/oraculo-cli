package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/delfosti-infra/oraculo-cli/internal/api"
	"github.com/delfosti-infra/oraculo-cli/internal/config"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var loginAPIURLFlag string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Autentica tu cuenta de Oráculo",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogin()
	},
}

func projectConfigAPIURL() string {
	if c, err := loadOraculoConfig(); err == nil {
		return c.APIURL
	}
	return ""
}

func runLogin() error {
	ui.PrintHeader(
		"ACCESO",
		"Inicia sesión en Oráculo",
		"Conéctate con tu cuenta de DelfosTI para empezar.",
	)

	apiURL := config.ResolveAPIURL(loginAPIURLFlag, projectConfigAPIURL())
	ui.PrintStep(fmt.Sprintf("API: %s", config.DisplayAPIURL(apiURL)))

	reader := bufio.NewReader(os.Stdin)

	ui.PromptLabel("Email")
	email, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("no se pudo leer el email: %w", err)
	}
	email = strings.TrimSpace(email)

	ui.PromptLabel("Password")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("no se pudo leer el password: %w", err)
	}
	fmt.Println()
	password := string(passwordBytes)

	spinner := ui.NewSpinner("Consultando al oráculo...")
	spinner.Start()

	client := api.NewClient(apiURL)
	session, err := client.Login(email, password)
	spinner.Stop()

	if err != nil {
		if api.IsConnectionError(err) {
			ui.PrintError(fmt.Sprintf("No se pudo conectar con el backend de Oráculo en %s", apiURL))
			ui.PrintHint("Verifica tu conexión. Para apuntar a otro backend usa --api-url o la variable ORACULO_API_URL.")
			return ui.ErrAlreadyReported
		}
		return ui.Fail("%s", err)
	}

	if err := config.SaveToken(session.AccessToken); err != nil {
		return err
	}
	if err := config.SaveGlobalAPIURL(apiURL); err != nil {
		return err
	}

	ui.PrintSuccess(fmt.Sprintf(
		"Bienvenido %s · %s",
		session.User.Email,
		session.User.Role,
	))
	ui.PrintHint("Siguiente: 'oraculo init' para vincular tu proyecto.")

	return nil
}

func init() {
	loginCmd.Flags().StringVar(&loginAPIURLFlag, "api-url", "", "URL del backend de Oráculo (default: ORACULO_API_URL, api_url del proyecto, o el backend hosteado)")
	rootCmd.AddCommand(loginCmd)
}
