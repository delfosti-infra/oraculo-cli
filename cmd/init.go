package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/delfosti/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

type Config struct {
	Project string `json:"project"`
	Slug    string `json:"slug"`
	BaseURL string `json:"base_url"`
	APIURL  string `json:"api_url"`
	E2EDir  string `json:"e2e_dir"`
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Inicializa Oráculo en el proyecto actual",
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.PrintHeader(
			"INICIO",
			"Inicializa Oráculo en tu proyecto",
			"Crea el archivo de configuración y la estructura base.",
		)

		if _, err := os.Stat("oraculo.config.json"); err == nil {
			ui.PrintError("Ya existe oraculo.config.json — el proyecto ya está inicializado.")
			return nil
		}

		config := Config{
			Project: "",
			Slug:    "",
			BaseURL: "",
			APIURL:  "https://api.oraculo.delfosti.com",
			E2EDir:  "e2e",
		}
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo generar la configuración: %s", err))
			return nil
		}

		if _, err := os.Stat("e2e"); os.IsNotExist(err) {
			if err := os.Mkdir("e2e", 0755); err != nil {
				ui.PrintError(fmt.Sprintf("No se pudo crear e2e/: %s", err))
				return nil
			}
			ui.PrintStep("Carpeta e2e/ creada")
		} else {
			ui.PrintStep("Carpeta e2e/ ya existía")
		}

		if err := os.WriteFile("oraculo.config.json", data, 0644); err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo crear oraculo.config.json: %s", err))
			return nil
		}
		ui.PrintStep("oraculo.config.json generado")

		ui.PrintSuccess("Proyecto listo. Completa los campos del config y ejecuta 'oraculo login' si todavía no lo hiciste.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
