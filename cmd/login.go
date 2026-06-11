package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/delfosti-infra/oraculo-cli/internal/api"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

const defaultAPIURL = "http://localhost:3000"

var loginAPIURLFlag string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Autentica tu cuenta de Oráculo",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogin()
	},
}

// resolveLoginAPIURL elige el API contra el cual loguear, en orden de prioridad:
// el flag --api-url, luego api_url del oraculo.config.json del directorio actual,
// y por último el default local.
func resolveLoginAPIURL() string {
	if loginAPIURLFlag != "" {
		return loginAPIURLFlag
	}
	if config, err := loadOraculoConfig(); err == nil && config.APIURL != "" {
		return config.APIURL
	}
	return defaultAPIURL
}

func runLogin() error {
	ui.PrintHeader(
		"ACCESO",
		"Inicia sesión en Oráculo",
		"Conéctate con tu cuenta de DelfosTI para empezar.",
	)

	apiURL := strings.TrimRight(resolveLoginAPIURL(), "/")
	ui.PrintStep(fmt.Sprintf("API: %s", apiURL))

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
		return ui.Fail("%s", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("no se pudo obtener el directorio home: %w", err)
	}

	oraculoDir := filepath.Join(homeDir, ".oraculo")
	if err := os.MkdirAll(oraculoDir, 0700); err != nil {
		return fmt.Errorf("no se pudo crear ~/.oraculo: %w", err)
	}

	if err := os.WriteFile(filepath.Join(oraculoDir, "token"), []byte(session.AccessToken), 0600); err != nil {
		return fmt.Errorf("no se pudo guardar el token: %w", err)
	}

	ui.PrintSuccess(fmt.Sprintf(
		"Bienvenido %s · %s",
		session.User.Email,
		session.User.Role,
	))

	return nil
}

func init() {
	loginCmd.Flags().StringVar(&loginAPIURLFlag, "api-url", "", "URL del API de Oráculo (default: api_url del oraculo.config.json o http://localhost:3000)")
	rootCmd.AddCommand(loginCmd)
}
