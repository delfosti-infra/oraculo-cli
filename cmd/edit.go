package cmd

import (
	"fmt"
	"os"

	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var editCmd = &cobra.Command{
	Use:   "edit <nombre>",
	Short: "Regraba un flow existente para reemplazar su spec (el diff se muestra al pushear)",
	Args:  cobra.ExactArgs(1),
	Long: "Vuelve a abrir Playwright codegen sobre un flow que ya grabaste y reemplaza su\n" +
		"spec local. Al correr 'oraculo push' verás un diff contra lo publicado en el\n" +
		"backoffice y se te pedirá confirmación antes de reemplazarlo.",
	Example: "  oraculo edit login\n  oraculo edit checkout --fresh",
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := slugify(args[0])
		if slug == "" {
			return ui.Fail("El nombre tiene que tener al menos un caracter alfanumérico.")
		}

		config, err := loadOraculoConfig()
		if err != nil {
			return ui.Fail("%s", err)
		}
		e2eDir := config.E2EDir
		if e2eDir == "" {
			e2eDir = "e2e"
		}

		if !flowSpecExists(e2eDir, slug) {
			ui.PrintWarning(fmt.Sprintf("No encontré un flow local llamado '%s' en %s/.", slug, e2eDir))
			ui.PrintHint(fmt.Sprintf("Si es nuevo, grábalo con: oraculo record %s", slug))
			return ui.ErrAlreadyReported
		}

		ui.PrintHint(fmt.Sprintf("Vas a regrabar '%s'. Al pushear verás el diff y se pedirá confirmar el reemplazo.", slug))
		return recordCmd.RunE(cmd, args)
	},
}

func flowSpecExists(e2eDir, slug string) bool {
	_, err := os.Stat(resolveSpecPath(e2eDir, slug))
	return err == nil
}

func init() {
	registerRecordFlags(editCmd)
	rootCmd.AddCommand(editCmd)
}
