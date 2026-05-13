package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var recordCmd = &cobra.Command{
	Use: "record [nombre]",
	Short: "Graba un nuevo spec con Playwright",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Iniciando grabación.....")
		return nil
	},
}

func init(){
	rootCmd.AddCommand(recordCmd)
}