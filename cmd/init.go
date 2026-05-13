package cmd

import (
	"encoding/json"
	"fmt"
	"os"

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
		if _, err := os.Stat("oraculo.config.json"); err == nil {
			return fmt.Errorf("ya existe oraculo.config.json — proyecto ya inicializado")
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
			return fmt.Errorf("error al generar config: %w", err)
		}

		if _, err := os.Stat("e2e"); os.IsNotExist(err) {
			if err := os.Mkdir("e2e", 0755); err != nil {
				return fmt.Errorf("no se pudo crear e2e/: %w", err)
			}
		}

		if err := os.WriteFile("oraculo.config.json", data, 0644); err != nil {
			return fmt.Errorf("no se pudo crear oraculo.config.json: %w", err)
		}

		fmt.Println("Proyecto inicializado correctamente")
		fmt.Println("  oraculo.config.json creado")
		fmt.Println("  e2e/ listo")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
