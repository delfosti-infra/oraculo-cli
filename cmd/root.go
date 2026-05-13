package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use: "oraculo",
	Short: "CLI oficial de Oráculo - DelfosTI",
	Long:  "Oráculo CLI — graba, valida y publica tus specs de Playwright desde la terminal.",
	SilenceUsage: true,
}

func Execute(){
	if err:= rootCmd.Execute(); err != nil {
		fmt.Fprint(os.Stderr, err)
		os.Exit(1)
	}
}