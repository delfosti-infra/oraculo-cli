package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Cambia el proyecto vinculado a este directorio",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.PrintHeader(
			"SWITCH",
			"Cambia el proyecto vinculado",
			"Reselecciona el proyecto del backoffice y actualiza oraculo.config.json.",
		)

		config, err := loadOraculoConfig()
		if err != nil {
			return ui.Fail("%s", err)
		}

		apiURL := config.APIURL
		if apiURL == "" {
			apiURL = defaultAPIURL
		}

		selected, err := selectProject(apiURL)
		if err != nil {
			return ui.Fail("%s", err)
		}

		prev := config.Project
		config.Project = selected.Name
		config.RefId = selected.RefId
		config.BaseURL = selected.BaseURL

		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return ui.Fail("No se pudo serializar la configuración: %s", err)
		}
		if err := os.WriteFile("oraculo.config.json", data, 0644); err != nil {
			return ui.Fail("No se pudo escribir oraculo.config.json: %s", err)
		}

		if prev == "" {
			ui.PrintSuccess(fmt.Sprintf("Proyecto vinculado: %s", selected.Name))
		} else {
			ui.PrintSuccess(fmt.Sprintf("Proyecto cambiado: %s → %s", prev, selected.Name))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}
