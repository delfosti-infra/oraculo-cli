//go:build windows

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const windowsDetachedProcess = 0x00000008

func deferWindowsSwap(exePath, tmpPath string) error {
	newPath := exePath + ".new"
	_ = os.Remove(newPath)
	if err := os.Rename(tmpPath, newPath); err != nil {
		return fmt.Errorf("no pude preparar el binario nuevo: %w", err)
	}

	pid := os.Getpid()
	escapedNew := strings.ReplaceAll(newPath, "'", "''")
	escapedExe := strings.ReplaceAll(exePath, "'", "''")
	script := fmt.Sprintf(
		"Wait-Process -Id %d -ErrorAction SilentlyContinue; "+
			"Start-Sleep -Milliseconds 400; "+
			"for ($i=0; $i -lt 20; $i++) { try { Move-Item -Force -LiteralPath '%s' -Destination '%s'; break } catch { Start-Sleep -Milliseconds 300 } }",
		pid, escapedNew, escapedExe,
	)

	helper := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", script)
	helper.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windowsDetachedProcess}
	if err := helper.Start(); err != nil {
		return fmt.Errorf("no pude programar el reemplazo diferido: %w", err)
	}
	_ = helper.Process.Release()
	return nil
}
