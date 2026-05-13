package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Valida los specs del proyecto",
	RunE: func(cmd *cobra.Command, args []string) error {
		var config Config

		// 1. Verificar que oraculo.config.json existe
		if _, err := os.Stat("oraculo.config.json"); err != nil {
			return fmt.Errorf("corré oraculo init primero")
		}

		// 2. Leer y parsear el config
		data, err := os.ReadFile("oraculo.config.json")
		if err != nil {
			return fmt.Errorf("no se pudo leer oraculo.config.json: %w", err)
		}
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("oraculo.config.json tiene formato inválido: %w", err)
		}

		// 3. Validar campos obligatorios
		if config.Project == "" || config.Slug == "" || config.BaseURL == "" {
			return fmt.Errorf("oraculo.config.json incompleto — completá project, slug y base_url")
		}

		// 4. Verificar que e2e/ existe
		if _, err := os.Stat(config.E2EDir); err != nil {
			return fmt.Errorf("directorio %s no encontrado", config.E2EDir)
		}

		// 5. Verificar playwright.config.ts (warning, no error)
		if _, err := os.Stat("playwright.config.ts"); err != nil {
			fmt.Println("  advertencia: no se encontró playwright.config.ts")
		}

		// 6. Contar specs
		specs, err := filepath.Glob(config.E2EDir + "/**/*.spec.ts")
		if err != nil {
			return fmt.Errorf("error al buscar specs: %w", err)
		}
		if len(specs) == 0 {
			return fmt.Errorf("no se encontraron specs en %s", config.E2EDir)
		}

		fmt.Printf("todo ok — %d spec(s) encontrado(s)\n", len(specs))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
