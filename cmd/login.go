package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/Delfosti-Platform/oraculo-cli/internal/api"
	"github.com/Delfosti-Platform/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

const defaultAPIURL = "http://localhost:3000"

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Autentica tu cuenta de Oráculo",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.PrintHeader(
			"ACCESO",
			"Inicia sesión en Oráculo",
			"Conéctate con tu cuenta de DelfosTI para empezar.",
		)

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

		client := api.NewClient(defaultAPIURL)
		session, err := client.Login(email, password)
		spinner.Stop()

		if err != nil {
			ui.PrintError(err.Error())
			return nil
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("no se pudo obtener el directorio home: %w", err)
		}

		oraculoDir := homeDir + "/.oraculo"
		if err := os.MkdirAll(oraculoDir, 0700); err != nil {
			return fmt.Errorf("no se pudo crear ~/.oraculo: %w", err)
		}

		if err := os.WriteFile(oraculoDir+"/token", []byte(session.AccessToken), 0600); err != nil {
			return fmt.Errorf("no se pudo guardar el token: %w", err)
		}

		ui.PrintSuccess(fmt.Sprintf(
			"Bienvenido %s · %s",
			session.User.Email,
			session.User.Role,
		))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
