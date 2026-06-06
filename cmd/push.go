package cmd

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Delfosti-Platform/oraculo-cli/internal/api"
	"github.com/Delfosti-Platform/oraculo-cli/internal/api/types"
	"github.com/Delfosti-Platform/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var pushCoreFlag bool

type pushResult struct {
	slug   string
	ok     bool
	detail string
}

var pushCmd = &cobra.Command{
	Use:   "push [flow-slug]",
	Short: "Sube los flows del proyecto al backoffice",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ui.PrintHeader(
			"PUSH",
			"Sube tus flows al backoffice",
			"Publica el spec + screenshots capturadas con oraculo record.",
		)

		config, err := loadOraculoConfig()
		if err != nil {
			ui.PrintError(err.Error())
			return nil
		}
		if config.RefId == "" {
			ui.PrintError("Falta el campo `refId` en oraculo.config.json. Corré 'oraculo init' para seleccionar el proyecto.")
			return nil
		}
		if config.APIURL == "" {
			ui.PrintError("Falta el campo `api_url` en oraculo.config.json.")
			return nil
		}

		token, err := loadToken()
		if err != nil {
			ui.PrintError(err.Error())
			return nil
		}

		e2eDir := config.E2EDir
		if e2eDir == "" {
			e2eDir = "e2e"
		}

		var targetSlugs []string
		if len(args) > 0 {
			targetSlugs = []string{slugify(args[0])}
		} else {
			targetSlugs, err = discoverFlows(e2eDir)
			if err != nil {
				ui.PrintError(err.Error())
				return nil
			}
			if len(targetSlugs) == 0 {
				ui.PrintError("No se encontraron specs en " + e2eDir + "/. Corré 'oraculo record <nombre>' primero.")
				return nil
			}
		}

		ui.PrintStep(fmt.Sprintf("Publicando %d flow(s) al proyecto '%s'", len(targetSlugs), config.Project))

		client := &http.Client{Timeout: 60 * time.Second}
		apiClient := api.NewClient(strings.TrimRight(config.APIURL, "/"))
		var results []pushResult

		for _, slug := range targetSlugs {
			spinner := ui.NewSpinner(fmt.Sprintf("Subiendo '%s'...", slug))
			spinner.Start()
			flowRefId, detail, err := pushFlow(client, config, token, e2eDir, slug)
			spinner.Stop()
			if err != nil {
				results = append(results, pushResult{slug: slug, ok: false, detail: err.Error()})
				ui.PrintWarning(fmt.Sprintf("'%s' · %s", slug, err.Error()))
				continue
			}

			if pushCoreFlag {
				if err := apiClient.ToggleFlowCore(token, config.RefId, flowRefId, true); err != nil {
					ui.PrintWarning(fmt.Sprintf("'%s' subido pero falló al marcar como core: %s", slug, err.Error()))
				} else {
					detail = detail + " · marcado como ★ Core"
				}
			}

			results = append(results, pushResult{slug: slug, ok: true, detail: detail})
			ui.PrintStep(fmt.Sprintf("'%s' · %s", slug, detail))
		}

		ok, fail := 0, 0
		for _, r := range results {
			if r.ok {
				ok++
			} else {
				fail++
			}
		}

		if fail == 0 {
			ui.PrintSuccess(fmt.Sprintf("%d flow(s) subido(s) exitosamente", ok))
		} else {
			ui.PrintError(fmt.Sprintf("%d ok · %d con error", ok, fail))
		}
		return nil
	},
}

func loadToken() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo obtener el directorio home: %w", err)
	}
	tokenPath := filepath.Join(home, ".oraculo", "token")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", fmt.Errorf("no se encontró token en ~/.oraculo/token. Corré 'oraculo login' primero")
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token vacío. Corré 'oraculo login' de nuevo")
	}
	return token, nil
}

func discoverFlows(e2eDir string) ([]string, error) {
	entries, err := os.ReadDir(e2eDir)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s/: %w", e2eDir, err)
	}
	var slugs []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".spec.ts") {
			continue
		}
		if strings.Contains(name, ".oraculo.") {
			continue
		}
		slugs = append(slugs, strings.TrimSuffix(name, ".spec.ts"))
	}
	sort.Strings(slugs)
	return slugs, nil
}

func pushFlow(client *http.Client, config *Config, token, e2eDir, slug string) (string, string, error) {
	specPath := filepath.Join(e2eDir, slug+".spec.ts")
	specContent, err := os.ReadFile(specPath)
	if err != nil {
		return "", "", fmt.Errorf("no se pudo leer %s: %w", specPath, err)
	}

	screenshotsDir := filepath.Join(e2eDir, ".oraculo", slug)
	screenshots, err := collectScreenshots(screenshotsDir)
	if err != nil {
		return "", "", err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	_ = writer.WriteField("name", slugToName(slug))
	_ = writer.WriteField("slug", slug)
	_ = writer.WriteField("specContent", string(specContent))
	_ = writer.WriteField("specPath", specPath)

	// HUs guardadas en el sidecar al hacer `oraculo record --HU=...` o vía auto-detect.
	meta := loadFlowMeta(e2eDir, slug)
	for _, key := range meta.JiraIssueKeys {
		_ = writer.WriteField("jiraIssueKeys", key)
	}
	// El flow se grabó con la sesión del proyecto → el worker debe inyectarla al replayar.
	if meta.UsesAuthSession {
		_ = writer.WriteField("usesAuthSession", "true")
	}

	for _, sp := range screenshots {
		file, err := os.Open(sp)
		if err != nil {
			return "", "", fmt.Errorf("no se pudo abrir %s: %w", sp, err)
		}
		part, err := writer.CreateFormFile("screenshots", filepath.Base(sp))
		if err != nil {
			file.Close()
			return "", "", err
		}
		if _, err := io.Copy(part, file); err != nil {
			file.Close()
			return "", "", err
		}
		file.Close()
	}
	writer.Close()

	url := fmt.Sprintf("%s/projects/%s/flows", strings.TrimRight(config.APIURL, "/"), config.RefId)
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("no se pudo conectar al API: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("no se pudo leer la respuesta: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("%s", api.ErrorMessage(respBody, resp.StatusCode))
	}

	flow, err := types.UnwrapJSON[types.FlowRef](respBody)
	if err != nil {
		return "", "", fmt.Errorf("push flow: %w", err)
	}

	detail := fmt.Sprintf("%d screenshots subidas", len(screenshots))
	if len(meta.JiraIssueKeys) > 0 {
		detail = fmt.Sprintf("%s · %d HU(s) linkeadas", detail, len(meta.JiraIssueKeys))
	}
	return flow.RefId, detail, nil
}

func collectScreenshots(dir string) ([]string, error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type indexed struct {
		path string
		n    int
	}
	var items []indexed
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "step-") || !strings.HasSuffix(name, ".png") {
			continue
		}
		numStr := strings.TrimPrefix(strings.TrimSuffix(name, ".png"), "step-")
		n, err := strconv.Atoi(numStr)
		if err != nil {
			continue
		}
		items = append(items, indexed{path: filepath.Join(dir, name), n: n})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].n < items[j].n })
	paths := make([]string, len(items))
	for i, it := range items {
		paths[i] = it.path
	}
	return paths, nil
}

func slugToName(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

func init() {
	pushCmd.Flags().BoolVar(&pushCoreFlag, "core", false, "Marca el flow como parte del Core Suite del proyecto")
	rootCmd.AddCommand(pushCmd)
}
