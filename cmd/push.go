package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use: "push",
	Short: "Publica los specs al servidor de Oráculo",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Publicando specs...")
		return nil
	},
}

func init(){
	rootCmd.AddCommand(pushCmd)
}