package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/spf13/cobra"
)


var loginCmd = &cobra.Command{
	Use: "login",
	Short: "Autentica tu cuenta de Oráculo",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

		// Request email
		fmt.Print("Email: ")
		email, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("Error al leer el email: %w", err)
		}
		email = strings.TrimSpace(email)

		// Request password
		fmt.Print("Password: ")
		passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("Error al leer el password: %w", err)
		}
		fmt.Println()
		password:= string(passwordBytes)
		_ = password
		
		// Save token after validate in BD
		token := "<token-simulator>" + email

		homeDir, err := os.UserHomeDir()

		if err != nil {
    		return fmt.Errorf("No se pudo obtener el directorio home: %w", err)
		}

		oraculoDir := homeDir + "/.oraculo"

		if err := os.MkdirAll(oraculoDir, 0700); err != nil {
			return fmt.Errorf("No se pudo crear ~/.oraculo: %w", err)
		}

		if err := os.WriteFile(oraculoDir+"/token", []byte(token), 0600); err != nil {
			return fmt.Errorf("No se pudo guardar el token: %w", err)
		}

		fmt.Println("Sesión iniciada correctamente")

		return nil
	},
}

func init(){
	rootCmd.AddCommand(loginCmd)
}