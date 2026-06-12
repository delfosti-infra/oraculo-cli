package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	updateModulePath = "github.com/delfosti-infra/oraculo-cli"
	updateTagsAPIURL = "https://api.github.com/repos/delfosti-infra/oraculo-cli/tags?per_page=100"
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
		"Busca la última versión publicada y la instala con la versión exacta.",
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

	if _, ok := commandVersion("go", "version"); !ok {
		ui.PrintError("Necesitas Go para actualizar (la CLI se instala con go install).")
		ui.PrintHint("Instala Go desde https://go.dev/dl/ o pídele a tu contacto de DelfosTI el binario precompilado.")
		return ui.ErrAlreadyReported
	}

	if err := goInstallCLI(latest); err != nil {
		return ui.Fail("No se pudo instalar %s: %s", latest, err)
	}

	if err := ensureOraculoSymlink(); err != nil {
		ui.PrintWarning("Se instaló, pero no pude actualizar el comando 'oraculo': " + err.Error())
	}

	ui.PrintSuccess(fmt.Sprintf("Actualizado a %s. Verifica con: oraculo --version", latest))
	return nil
}

// fetchLatestCLIVersion devuelve el tag de versión más alto publicado en GitHub.
// Va directo a GitHub (no al proxy de Go) para esquivar el lag del endpoint @latest.
func fetchLatestCLIVersion() (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, updateTagsAPIURL, nil)
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

	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return "", fmt.Errorf("respuesta inválida de GitHub: %w", err)
	}

	best := ""
	for _, t := range tags {
		if _, _, _, ok := parseSemver(t.Name); !ok {
			continue
		}
		if best == "" || semverLess(best, t.Name) {
			best = t.Name
		}
	}
	if best == "" {
		return "", fmt.Errorf("no encontré tags de versión")
	}
	return best, nil
}

// goInstallCLI corre `go install module@version` heredando stdout/stderr.
func goInstallCLI(version string) error {
	cmd := exec.Command("go", "install", updateModulePath+"@"+version)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ensureOraculoSymlink deja el comando `oraculo` apuntando al binario recién
// instalado (`oraculo-cli`). En Windows copia el .exe; en el resto, symlink.
func ensureOraculoSymlink() error {
	gobin := goBinDir()
	if gobin == "" {
		return fmt.Errorf("no pude resolver el directorio de binarios de Go")
	}
	src := filepath.Join(gobin, "oraculo-cli")
	dst := filepath.Join(gobin, "oraculo")
	if runtime.GOOS == "windows" {
		src += ".exe"
		dst += ".exe"
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o755)
	}
	_ = os.Remove(dst)
	return os.Symlink(src, dst)
}

func goBinDir() string {
	if out, err := exec.Command("go", "env", "GOBIN").Output(); err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" {
			return dir
		}
	}
	if out, err := exec.Command("go", "env", "GOPATH").Output(); err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" {
			return filepath.Join(dir, "bin")
		}
	}
	return ""
}

// parseSemver descompone "vX.Y.Z" (ignora pre-release). ok=false si no matchea.
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

// semverLess reporta si a < b. Versiones no parseables se tratan como no-menores.
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
