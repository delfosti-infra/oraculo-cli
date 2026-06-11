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
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.PrintHeader(
			"VALIDACIÓN",
			"Comprueba la salud del proyecto",
			"Revisa la configuración, estructura y specs antes de publicar.",
		)

		var config Config

		if _, err := os.Stat("oraculo.config.json"); err != nil {
			return ui.Fail("No se encontró oraculo.config.json. Ejecuta 'oraculo init' primero.")
		}
		ui.PrintStep("oraculo.config.json encontrado")

		data, err := os.ReadFile("oraculo.config.json")
		if err != nil {
			return ui.Fail("No se pudo leer oraculo.config.json: %s", err)
		}

		if err := json.Unmarshal(data, &config); err != nil {
			return ui.Fail("oraculo.config.json tiene formato inválido: %s", err)
		}
		ui.PrintStep("Estructura JSON válida")

		if config.Project == "" || config.RefId == "" || config.BaseURL == "" {
			return ui.Fail("oraculo.config.json incompleto. Completa project, refId y base_url (o corre 'oraculo init').")
		}
		ui.PrintStep("Campos obligatorios completos")

		if _, err := os.Stat(config.E2EDir); err != nil {
			return ui.Fail("Directorio %s no encontrado.", config.E2EDir)
		}
		ui.PrintStep(fmt.Sprintf("Directorio %s/ encontrado", config.E2EDir))

		if _, err := os.Stat("playwright.config.ts"); err != nil {
			ui.PrintWarning("playwright.config.ts no encontrado en la raíz (puede estar en otro path).")
		} else {
			ui.PrintStep("playwright.config.ts encontrado")
		}

		specs, err := filepath.Glob(config.E2EDir + "/**/*.spec.ts")
		if err != nil {
			return ui.Fail("Error al buscar specs: %s", err)
		}
		if len(specs) == 0 {
			return ui.Fail("No se encontraron specs en %s/", config.E2EDir)
		}
		ui.PrintStep(fmt.Sprintf("%d spec(s) detectado(s)", len(specs)))

		ui.PrintSuccess("Todo listo. El proyecto está saneado y se puede publicar con 'oraculo push'.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
