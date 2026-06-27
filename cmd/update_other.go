//go:build !windows

package cmd

import "fmt"

func deferWindowsSwap(_, _ string) error {
	return fmt.Errorf("reemplazo diferido solo disponible en Windows")
}
