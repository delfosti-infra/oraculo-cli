package cmd

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/delfosti-infra/oraculo-cli/internal/tui"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "oraculo",
	Short:        "CLI oficial de Oráculo - DelfosTI",
	Version:      resolveVersion(),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.PrintBanner()
		if tui.IsInteractive() {
			return tui.Run(cmd.Root())
		}
		return cmd.Help()
	},
}

func init() {
	rootCmd.SetVersionTemplate("oraculo {{.Version}}\n")
}

// resolveVersion devuelve la versión del módulo embebida por `go install`
// (ej. "v1.0.4"). Para builds locales (`go build`/`go run`) devuelve "dev".
func resolveVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
