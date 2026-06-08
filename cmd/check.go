package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Valida los specs del proyecto",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.PrintHeader(
			"VALIDACIÓN",
			"Comprueba la salud del proyecto",
			"Revisa la configuración, estructura y specs antes de publicar.",
		)

		var config Config

		if _, err := os.Stat("oraculo.config.json"); err != nil {
			ui.PrintError("No se encontró oraculo.config.json. Ejecuta 'oraculo init' primero.")
			return nil
		}
		ui.PrintStep("oraculo.config.json encontrado")

		data, err := os.ReadFile("oraculo.config.json")
		if err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo leer oraculo.config.json: %s", err))
			return nil
		}

		if err := json.Unmarshal(data, &config); err != nil {
			ui.PrintError(fmt.Sprintf("oraculo.config.json tiene formato inválido: %s", err))
			return nil
		}
		ui.PrintStep("Estructura JSON válida")

		if config.Project == "" || config.RefId == "" || config.BaseURL == "" {
			ui.PrintError("oraculo.config.json incompleto. Completa project, refId y base_url (o corré 'oraculo init').")
			return nil
		}
		ui.PrintStep("Campos obligatorios completos")

		if _, err := os.Stat(config.E2EDir); err != nil {
			ui.PrintError(fmt.Sprintf("Directorio %s no encontrado.", config.E2EDir))
			return nil
		}
		ui.PrintStep(fmt.Sprintf("Directorio %s/ encontrado", config.E2EDir))

		if _, err := os.Stat("playwright.config.ts"); err != nil {
			ui.PrintWarning("playwright.config.ts no encontrado en la raíz (puede estar en otro path).")
		} else {
			ui.PrintStep("playwright.config.ts encontrado")
		}

		specs, err := filepath.Glob(config.E2EDir + "/**/*.spec.ts")
		if err != nil {
			ui.PrintError(fmt.Sprintf("Error al buscar specs: %s", err))
			return nil
		}
		if len(specs) == 0 {
			ui.PrintError(fmt.Sprintf("No se encontraron specs en %s/", config.E2EDir))
			return nil
		}
		ui.PrintStep(fmt.Sprintf("%d spec(s) detectado(s)", len(specs)))

		ui.PrintSuccess("Todo listo. El proyecto está saneado y se puede publicar con 'oraculo push'.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
