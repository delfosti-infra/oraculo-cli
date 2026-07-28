package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
	appconfig "github.com/delfosti-infra/oraculo-cli/internal/config"
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

		apiURL := appconfig.ResolveAPIURL("", config.APIURL)

		selected, err := selectProject(apiURL)
		if err != nil {
			return ui.Fail("%s", err)
		}

		prev := config.Project
		prevRefId := config.RefId
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

		warnAboutFlowsFromOtherProject(config, prevRefId, prev)
		return nil
	},
}

func warnAboutFlowsFromOtherProject(config *types.Config, prevRefId, prevName string) {
	if prevRefId == "" || prevRefId == config.RefId {
		return
	}

	e2eDir := config.E2EDir
	if e2eDir == "" {
		e2eDir = "e2e"
	}
	slugs, err := discoverFlows(e2eDir)
	if err != nil {
		return
	}

	stale := 0
	for _, slug := range slugs {
		if classifyFlowOwnership(loadFlowMeta(e2eDir, slug), config.RefId) == ownershipForeign {
			stale++
		}
	}
	if stale == 0 {
		return
	}

	owner := prevName
	if owner == "" {
		owner = "otro proyecto"
	}
	ui.PrintWarning(fmt.Sprintf(
		"En %s/ quedan %d flow(s) de '%s'. 'oraculo push' NO los va a subir a '%s'.",
		e2eDir, stale, owner, config.Project,
	))
	ui.PrintHint("Lo recomendado es una carpeta por proyecto. Si necesitas subir uno puntual igual: 'oraculo push <slug> --force'.")
}

func init() {
	rootCmd.AddCommand(switchCmd)
}
