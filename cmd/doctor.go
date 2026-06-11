package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/delfosti-infra/oraculo-cli/internal/api"
	appconfig "github.com/delfosti-infra/oraculo-cli/internal/config"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnostica tu entorno: dependencias, conexión y sesión",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor()
	},
}

// runDoctor corre los chequeos de pre-vuelo en orden y termina con exit 1 si
// algún requisito duro (Node, navegadores, conexión) falla, para que sea usable
// en CI. La sesión y el proyecto vinculado son pasos siguientes, no fallos.
func runDoctor() error {
	ui.PrintHeader(
		"DOCTOR",
		"Diagnóstico de tu entorno",
		"Verifica dependencias, conexión al backend y tu sesión antes de grabar.",
	)

	hardFail := false
	pending := false

	if v, ok := commandVersion("node", "--version"); ok {
		ui.PrintStep("Node.js " + v)
	} else {
		hardFail = true
		ui.PrintCheckFail("Node.js no encontrado")
		ui.PrintHint("Instala Node.js 18+ desde https://nodejs.org y reintenta.")
	}

	if v, ok := commandVersion("npm", "--version"); ok {
		ui.PrintStep("npm " + v)
	} else {
		hardFail = true
		ui.PrintCheckFail("npm no encontrado")
		ui.PrintHint("npm viene con Node.js — instala Node.js 18+ y reintenta.")
	}

	if playwrightBrowsersPresent() {
		ui.PrintStep("Navegadores de Playwright instalados")
	} else {
		hardFail = true
		ui.PrintCheckFail("Navegadores de Playwright no instalados")
		ui.PrintHint("Instálalos con: npx playwright install chromium")
	}

	apiURL := appconfig.ResolveAPIURL("", projectConfigAPIURL())
	client := api.NewClient(apiURL)
	if err := client.Ping(); err != nil {
		hardFail = true
		ui.PrintCheckFail("Backend inalcanzable (" + apiURL + ")")
		ui.PrintHint("Verifica tu conexión. Para apuntar a otro backend usa --api-url o ORACULO_API_URL.")
	} else {
		ui.PrintStep("Backend alcanzable (" + apiURL + ")")
		pending = !checkSession(client) || pending
	}

	pending = !checkProjectLinked() || pending

	if hardFail {
		return ui.ErrAlreadyReported
	}
	if pending {
		ui.PrintSuccess("Tu entorno está OK. Resuelve los avisos de arriba para empezar a grabar.")
		return nil
	}
	ui.PrintSuccess("Tu entorno está listo para usar Oráculo.")
	return nil
}

// checkSession devuelve true si hay una sesión válida; false si falta o venció.
func checkSession(client *api.Client) bool {
	token, err := appconfig.LoadToken()
	if err != nil {
		ui.PrintWarning("Sin sesión iniciada")
		ui.PrintHint("Inicia sesión con 'oraculo login'.")
		return false
	}
	if _, err := client.ListProjects(token); err != nil {
		ui.PrintWarning("Sesión vencida o inválida")
		ui.PrintHint("Vuelve a iniciar sesión con 'oraculo login'.")
		return false
	}
	ui.PrintStep("Sesión válida")
	return true
}

// checkProjectLinked devuelve true si el directorio tiene un proyecto vinculado.
func checkProjectLinked() bool {
	config, err := loadOraculoConfig()
	if err != nil || config.RefId == "" {
		ui.PrintWarning("Este directorio no tiene proyecto vinculado")
		ui.PrintHint("Vincula uno con 'oraculo init'.")
		return false
	}
	ui.PrintStep("Proyecto vinculado: " + config.Project)
	return true
}

func commandVersion(bin string, args ...string) (string, bool) {
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// playwrightBrowsersPresent comprueba el cache de navegadores de Playwright
// (respeta PLAYWRIGHT_BROWSERS_PATH) buscando al menos un build de Chromium.
func playwrightBrowsersPresent() bool {
	dir := playwrightBrowsersDir()
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "chromium") {
			return true
		}
	}
	return false
}

func playwrightBrowsersDir() string {
	if custom := strings.TrimSpace(os.Getenv("PLAYWRIGHT_BROWSERS_PATH")); custom != "" && custom != "0" {
		return custom
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Caches", "ms-playwright")
	case "windows":
		if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
			return filepath.Join(local, "ms-playwright")
		}
		return filepath.Join(home, "AppData", "Local", "ms-playwright")
	default:
		return filepath.Join(home, ".cache", "ms-playwright")
	}
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
