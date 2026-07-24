package cmd

import (
	"errors"
	"os"
	"runtime/debug"

	"github.com/delfosti-infra/oraculo-cli/internal/tui"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "oraculo",
	Short:         "CLI oficial de Oráculo - DelfosTI",
	Version:       resolveVersion(),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.PrintBanner(resolveVersion())
		if tui.IsInteractive() {
			return tui.Run(cmd.Root())
		}
		return cmd.Help()
	},
}

func init() {
	rootCmd.SetVersionTemplate("oraculo {{.Version}}\n")
}

var version string

func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}

type exitCodeError struct {
	code int
	err  error
}

func (e *exitCodeError) Error() string {
	return e.err.Error()
}

func (e *exitCodeError) Unwrap() error {
	return e.err
}

func withExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitCodeError{code: code, err: err}
}

func exitCodeFor(err error) int {
	var coded *exitCodeError
	if errors.As(err, &coded) {
		return coded.code
	}
	return 1
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if !errors.Is(err, ui.ErrAlreadyReported) {
			ui.PrintError(err.Error())
		}
		os.Exit(exitCodeFor(err))
	}
}
