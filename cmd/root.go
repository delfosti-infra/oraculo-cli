package cmd

import (
	"fmt"
	"os"

	"github.com/Delfosti-Platform/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "oraculo",
	Short:        "CLI oficial de Oráculo - DelfosTI",
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		ui.PrintBanner()
		_ = cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
