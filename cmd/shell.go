package cmd

import (
	"github.com/delfosti-infra/oraculo-cli/internal/tui"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Abre el modo interactivo con la paleta de comandos",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run(rootCmd)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
}
