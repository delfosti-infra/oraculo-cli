package cmd

import (
	"bytes"
	"errors"
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

	"github.com/delfosti-infra/oraculo-cli/internal/api"
	"github.com/delfosti-infra/oraculo-cli/internal/api/types"
	appconfig "github.com/delfosti-infra/oraculo-cli/internal/config"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
	"github.com/spf13/cobra"
)

var pushCoreFlag bool
var pushYesFlag bool

var errFlowUnchanged = errors.New("el flow ya existe sin cambios")
var errReplaceDeclined = errors.New("reemplazo cancelado por el usuario")

type pushResult struct {
	slug      string
	ok        bool
	replaced  bool
	unchanged bool
	detail    string
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
			return ui.Fail("%s", err)
		}
		if config.RefId == "" {
			return ui.Fail("Falta el campo `refId` en oraculo.config.json. Corre 'oraculo init' para seleccionar el proyecto.")
		}
		if config.APIURL == "" {
			return ui.Fail("Falta el campo `api_url` en oraculo.config.json.")
		}

		token, err := appconfig.LoadToken()
		if err != nil {
			return ui.Fail("%s", err)
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
				return ui.Fail("%s", err)
			}
			if len(targetSlugs) == 0 {
				return ui.Fail("No se encontraron specs en %s/. Corre 'oraculo record <nombre>' primero.", e2eDir)
			}
		}

		ui.PrintStep(fmt.Sprintf("Publicando %d flow(s) al proyecto '%s'", len(targetSlugs), config.Project))

		client := &http.Client{Timeout: 60 * time.Second}
		apiClient := api.NewClient(strings.TrimRight(config.APIURL, "/"))
		var results []pushResult

		for _, slug := range targetSlugs {
			result := pushOne(client, apiClient, config, token, e2eDir, slug)
			results = append(results, result)
		}

		ok, replaced, unchanged, fail := 0, 0, 0, 0
		for _, r := range results {
			switch {
			case r.replaced:
				replaced++
			case r.ok:
				ok++
			case r.unchanged:
				unchanged++
			default:
				fail++
			}
		}

		if fail > 0 {
			return ui.Fail("%d nuevo(s) · %d reemplazado(s) · %d sin cambios · %d con error", ok, replaced, unchanged, fail)
		}
		ui.PrintSuccess(pushSummary(ok, replaced, unchanged))
		return nil
	},
}

func pushOne(
	client *http.Client,
	apiClient *api.Client,
	config *types.Config,
	token, e2eDir, slug string,
) pushResult {
	existing, err := apiClient.FindFlowBySlug(token, config.RefId, slug)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("'%s' · no se pudo consultar el backoffice: %s", slug, err.Error()))
		return pushResult{slug: slug, ok: false, detail: err.Error()}
	}

	if existing != nil {
		return replaceExistingFlow(apiClient, config, token, e2eDir, slug, existing)
	}

	return createNewFlow(client, apiClient, config, token, e2eDir, slug)
}

func createNewFlow(
	client *http.Client,
	apiClient *api.Client,
	config *types.Config,
	token, e2eDir, slug string,
) pushResult {
	spinner := ui.NewSpinner(fmt.Sprintf("Subiendo '%s'...", slug))
	spinner.Start()
	flowRefId, detail, err := pushFlow(client, config, token, e2eDir, slug)
	spinner.Stop()

	if errors.Is(err, errFlowUnchanged) {
		ui.PrintStep(fmt.Sprintf("'%s' · ya existe en el backoffice; corré 'oraculo push %s' de nuevo para ver el diff", slug, slug))
		return pushResult{slug: slug, unchanged: true, detail: "ya existía"}
	}
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("'%s' · %s", slug, err.Error()))
		return pushResult{slug: slug, ok: false, detail: err.Error()}
	}

	if pushCoreFlag {
		if err := apiClient.ToggleFlowCore(token, config.RefId, flowRefId, true); err != nil {
			ui.PrintWarning(fmt.Sprintf("'%s' subido pero falló al marcar como core: %s", slug, err.Error()))
		} else {
			detail = detail + " · marcado como ★ Core"
		}
	}

	ui.PrintStep(fmt.Sprintf("'%s' · %s", slug, detail))
	return pushResult{slug: slug, ok: true, detail: detail}
}

func replaceExistingFlow(
	apiClient *api.Client,
	config *types.Config,
	token, e2eDir, slug string,
	existing *types.FlowSummary,
) pushResult {
	localSpec, err := readLocalSpec(e2eDir, slug)
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("'%s' · %s", slug, err.Error()))
		return pushResult{slug: slug, ok: false, detail: err.Error()}
	}

	if ui.SpecsEqual(existing.SpecContent, localSpec) {
		ui.PrintStep(fmt.Sprintf("'%s' · sin cambios, ya estaba al día en el backoffice", slug))
		return pushResult{slug: slug, unchanged: true, detail: "sin cambios"}
	}

	if err := confirmReplace(slug, existing, localSpec); err != nil {
		if errors.Is(err, errReplaceDeclined) {
			ui.PrintStep(fmt.Sprintf("'%s' · sin cambios (reemplazo cancelado)", slug))
			return pushResult{slug: slug, unchanged: true, detail: "reemplazo cancelado"}
		}
		ui.PrintWarning(fmt.Sprintf("'%s' · %s", slug, err.Error()))
		return pushResult{slug: slug, ok: false, detail: err.Error()}
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Reemplazando '%s' en el backoffice...", slug))
	spinner.Start()
	updateErr := apiClient.UpdateFlowSpec(token, config.RefId, existing.RefId, localSpec)
	spinner.Stop()
	if updateErr != nil {
		ui.PrintWarning(fmt.Sprintf("'%s' · %s", slug, updateErr.Error()))
		return pushResult{slug: slug, ok: false, detail: updateErr.Error()}
	}

	detail := "spec reemplazado"
	if pushCoreFlag {
		if err := apiClient.ToggleFlowCore(token, config.RefId, existing.RefId, true); err != nil {
			ui.PrintWarning(fmt.Sprintf("'%s' reemplazado pero falló al marcar como core: %s", slug, err.Error()))
		} else {
			detail = detail + " · marcado como ★ Core"
		}
	}

	ui.PrintStep(fmt.Sprintf("'%s' · %s", slug, detail))
	ui.PrintHint("Las screenshots del flow quedaron marcadas como desactualizadas. Hoy el endpoint de update solo reemplaza el spec; re-subir las nuevas capturas necesita soporte en el core (pendiente).")
	return pushResult{slug: slug, replaced: true, detail: detail}
}

func confirmReplace(slug string, existing *types.FlowSummary, localSpec string) error {
	remoteLabel := fmt.Sprintf("backoffice: %s", slug)
	if existing.UpdatedAt != "" {
		remoteLabel = fmt.Sprintf("%s (actualizado %s)", remoteLabel, existing.UpdatedAt)
	}
	ui.PrintSpecDiff(remoteLabel, "local: "+slug+".spec.ts", existing.SpecContent, localSpec)

	ui.PrintWarning(fmt.Sprintf("Esto REEMPLAZA el flow '%s' en el backoffice con tu spec local.", slug))
	if pushYesFlag {
		return nil
	}
	if !ui.PromptYesNo(fmt.Sprintf("¿Reemplazar '%s' en el backoffice? [s/N]", slug), false) {
		return errReplaceDeclined
	}
	return nil
}

func resolveSpecPath(e2eDir, slug string) string {
	mobilePath := filepath.Join(e2eDir, slug+".mobile.json")
	if _, statErr := os.Stat(mobilePath); statErr == nil {
		return mobilePath
	}
	return filepath.Join(e2eDir, slug+".spec.ts")
}

func readLocalSpec(e2eDir, slug string) (string, error) {
	specPath := resolveSpecPath(e2eDir, slug)
	specContent, err := os.ReadFile(specPath)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer %s: %w", specPath, err)
	}
	return string(specContent), nil
}

func pushSummary(uploaded, replaced, unchanged int) string {
	changed := uploaded + replaced
	if changed == 0 && unchanged > 0 {
		return fmt.Sprintf("Sin cambios para subir (%d flow(s) ya estaban al día)", unchanged)
	}

	var parts []string
	if uploaded > 0 {
		parts = append(parts, fmt.Sprintf("%d nuevo(s)", uploaded))
	}
	if replaced > 0 {
		parts = append(parts, fmt.Sprintf("%d reemplazado(s)", replaced))
	}
	if unchanged > 0 {
		parts = append(parts, fmt.Sprintf("%d sin cambios", unchanged))
	}
	return strings.Join(parts, " · ")
}

func discoverFlows(e2eDir string) ([]string, error) {
	entries, err := os.ReadDir(e2eDir)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s/: %w", e2eDir, err)
	}
	seen := map[string]bool{}
	var slugs []string
	add := func(slug string) {
		if !seen[slug] {
			seen[slug] = true
			slugs = append(slugs, slug)
		}
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.Contains(name, ".oraculo.") {
			continue
		}
		switch {
		case strings.HasSuffix(name, ".spec.ts"):
			add(strings.TrimSuffix(name, ".spec.ts"))
		case strings.HasSuffix(name, ".mobile.json"):
			add(strings.TrimSuffix(name, ".mobile.json"))
		}
	}
	sort.Strings(slugs)
	return slugs, nil
}

func pushFlow(client *http.Client, config *types.Config, token, e2eDir, slug string) (string, string, error) {
	specPath := resolveSpecPath(e2eDir, slug)
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

	meta := loadFlowMeta(e2eDir, slug)
	for _, key := range meta.JiraIssueKeys {
		_ = writer.WriteField("boardIssueKeys", key)
	}
	if meta.UsesAuthSession {
		_ = writer.WriteField("usesAuthSession", "true")
	}
	if meta.Platform != "" {
		_ = writer.WriteField("platform", meta.Platform)
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

	if resp.StatusCode == http.StatusConflict {
		return "", "", errFlowUnchanged
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
	pushCmd.Flags().BoolVarP(&pushYesFlag, "yes", "y", false, "No pide confirmación al reemplazar un flow que ya existe en el backoffice")
	rootCmd.AddCommand(pushCmd)
}
