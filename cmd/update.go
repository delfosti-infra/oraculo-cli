package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	updateLatestAPIURL   = "https://api.github.com/repos/delfosti-infra/oraculo-cli/releases/latest"
	updateReleaseBaseURL = "https://github.com/delfosti-infra/oraculo-cli/releases/download"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Actualiza la CLI de Oráculo a la última versión publicada",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUpdate(cmd.Root().Version)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(current string) error {
	ui.PrintHeader(
		"UPDATE",
		"Actualiza la CLI de Oráculo",
		"Descarga el binario de la última versión publicada y reemplaza el actual.",
	)

	ui.PrintStep("Buscando la última versión publicada…")
	latest, err := fetchLatestCLIVersion()
	if err != nil {
		return ui.Fail("No se pudo consultar la última versión: %s", err)
	}

	if current != "dev" && !semverLess(current, latest) {
		ui.PrintSuccess(fmt.Sprintf("Ya tienes la última versión (%s).", current))
		return nil
	}

	if current == "dev" {
		ui.PrintStep(fmt.Sprintf("Estás en un build local (dev) — instalando %s", latest))
	} else {
		ui.PrintStep(fmt.Sprintf("Nueva versión disponible: %s → %s", current, latest))
	}

	exePath, err := currentExecutablePath()
	if err != nil {
		return ui.Fail("No pude resolver la ruta del ejecutable actual: %s", err)
	}

	asset := releaseAssetName()
	url := fmt.Sprintf("%s/%s/%s", updateReleaseBaseURL, latest, asset)

	ui.PrintStep("Descargando " + asset + "…")
	deferred, err := downloadAndReplace(url, exePath)
	if err != nil {
		return ui.Fail("No se pudo instalar %s: %s", latest, err)
	}

	if deferred {
		ui.PrintSuccess(fmt.Sprintf(
			"%s descargada. El binario estaba en uso: cierra esta terminal para completar la instalación y luego verifica con: oraculo --version",
			latest,
		))
		return nil
	}

	ui.PrintSuccess(fmt.Sprintf("Actualizado a %s. Verifica con: oraculo --version", latest))
	return nil
}

func currentExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved, nil
	}
	return exe, nil
}

func releaseAssetName() string {
	name := fmt.Sprintf("oraculo_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func downloadAndReplace(url, exePath string) (bool, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("descarga falló (HTTP %d) en %s", resp.StatusCode, url)
	}

	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".oraculo-update-*")
	if err != nil {
		return false, fmt.Errorf("no pude crear archivo temporal en %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return false, fmt.Errorf("no pude escribir la descarga: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return false, err
	}

	if runtime.GOOS == "windows" {
		return replaceWindowsBinary(exePath, tmpPath)
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		return false, fmt.Errorf("no pude reemplazar el binario: %w", err)
	}
	return false, nil
}

func replaceWindowsBinary(exePath, tmpPath string) (bool, error) {
	old := exePath + ".old"
	_ = os.Remove(old)

	var renameErr error
	for attempt := 0; attempt < 5; attempt++ {
		if renameErr = os.Rename(exePath, old); renameErr == nil {
			break
		}
		time.Sleep(time.Duration(200*(attempt+1)) * time.Millisecond)
	}

	if renameErr != nil {
		if deferErr := deferWindowsSwap(exePath, tmpPath); deferErr != nil {
			return false, fmt.Errorf("binario en uso (%v) y falló el reemplazo diferido: %w", renameErr, deferErr)
		}
		return true, nil
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		_ = os.Rename(old, exePath)
		return false, fmt.Errorf("no pude colocar el binario nuevo: %w", err)
	}
	_ = os.Remove(old)
	return false, nil
}

func fetchLatestCLIVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, updateLatestAPIURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub respondió %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}
	if release.TagName == "" {
		return "", fmt.Errorf("no encontré un release publicado")
	}
	return release.TagName, nil
}

func parseSemver(v string) (int, int, int, bool) {
	core := strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(core, '-'); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

func semverLess(a, b string) bool {
	aMaj, aMin, aPat, aOK := parseSemver(a)
	bMaj, bMin, bPat, bOK := parseSemver(b)
	if !aOK || !bOK {
		return false
	}
	if aMaj != bMaj {
		return aMaj < bMaj
	}
	if aMin != bMin {
		return aMin < bMin
	}
	return aPat < bPat
}
