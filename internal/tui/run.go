package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// IsInteractive indica si hay una terminal real para correr el TUI.
func IsInteractive() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

// Run abre el modo interactivo, derivando la paleta de los subcomandos de root.
func Run(root *cobra.Command) error {
	if !IsInteractive() {
		return fmt.Errorf("el modo interactivo requiere una terminal")
	}
	_, err := tea.NewProgram(newModel(root)).Run()
	return err
}
