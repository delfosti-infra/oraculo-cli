package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/delfosti-infra/oraculo-cli/internal/api"
	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

type Config struct {
	Project string `json:"project"`
	RefId   string `json:"refId"`
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
			APIURL: defaultAPIURL,
			E2EDir: "e2e",
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

		if selected, err := selectProject(config.APIURL); err != nil {
			ui.PrintWarning(err.Error())
			ui.PrintWarning("Completá `refId` y `base_url` manualmente, o corré 'oraculo login' y reintentá 'oraculo init'.")
		} else {
			config.Project = selected.Name
			config.RefId = selected.RefId
			config.BaseURL = selected.BaseURL
			ui.PrintStep(fmt.Sprintf("Proyecto seleccionado: %s", selected.Name))
		}

		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo generar la configuración: %s", err))
			return nil
		}

		if err := os.WriteFile("oraculo.config.json", data, 0644); err != nil {
			ui.PrintError(fmt.Sprintf("No se pudo crear oraculo.config.json: %s", err))
			return nil
		}
		ui.PrintStep("oraculo.config.json generado")

		if config.RefId != "" {
			ui.PrintSuccess("Proyecto listo. Grabá un flow con 'oraculo record <nombre>' y publicá con 'oraculo push'.")
		} else {
			ui.PrintSuccess("Config generada. Completá los campos faltantes y usá 'oraculo push'.")
		}
		return nil
	},
}

func promptProjectSelection(projects []types.Project) *types.Project {
	ui.PrintStep("Proyectos disponibles:")
	for i, p := range projects {
		fmt.Printf("    %d) %s  ·  %s\n", i+1, p.Name, p.BaseURL)
	}
	fmt.Printf("  Elegí un proyecto [1-%d]: ", len(projects))

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		ui.PrintWarning("No se pudo leer la selección.")
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(projects) {
		ui.PrintWarning("Selección inválida — completá refId manualmente.")
		return nil
	}
	return &projects[n-1]
}

func selectProject(apiURL string) (*types.Project, error) {
	token, err := loadToken()
	if err != nil {
		return nil, err
	}
	apiClient := api.NewClient(strings.TrimRight(apiURL, "/"))
	projects, err := apiClient.ListProjects(token)
	if err != nil {
		return nil, fmt.Errorf("no se pudieron listar los proyectos: %w", err)
	}
	if len(projects) == 0 {
		return nil, fmt.Errorf("tu empresa todavía no tiene proyectos — creá uno en el backoffice")
	}
	selected := promptProjectSelection(projects)
	if selected == nil {
		return nil, fmt.Errorf("selección inválida")
	}
	return selected, nil
}

func init() {
	rootCmd.AddCommand(initCmd)
}
