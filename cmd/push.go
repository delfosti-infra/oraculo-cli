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
var pushForceFlag bool
var pushAllFlag bool

var errFlowUnchanged = errors.New("el flow ya existe sin cambios")
var errReplaceDeclined = errors.New("cancelado por el usuario")

type pushResult struct {
	slug      string
	ok        bool
	replaced  bool
	unchanged bool
	synced    bool
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

		var discovered []string
		explicit := len(args) > 0
		if explicit {
			discovered = []string{slugify(args[0])}
		} else {
			discovered, err = discoverFlows(e2eDir)
			if err != nil {
				return ui.Fail("%s", err)
			}
			if len(discovered) == 0 {
				return ui.Fail("No se encontraron specs en %s/. Corre 'oraculo record <nombre>' primero.", e2eDir)
			}
		}

		selection := selectFlowsToPush(e2eDir, config, discovered, explicit)
		if explicit && len(selection.Foreign) > 0 {
			foreign := selection.Foreign[0]
			return ui.Fail(
				"'%s' fue grabado para «%s», no para «%s». Corre 'oraculo switch' para cambiar de proyecto, o 'oraculo push %s --force' para subirlo igual.",
				foreign.Slug, foreign.ProjectName, config.Project, foreign.Slug,
			)
		}
		reportSkippedFlows(selection, config.Project)

		targetSlugs := selection.Push
		if len(targetSlugs) == 0 {
			ui.PrintSuccess(pushSummary(0, 0, len(selection.Unchanged)))
			return nil
		}

		ui.PrintStep(fmt.Sprintf("Publicando %d flow(s) al proyecto '%s'", len(targetSlugs), config.Project))

		client := &http.Client{Timeout: 60 * time.Second}
		apiClient := api.NewClient(strings.TrimRight(config.APIURL, "/"))
		var results []pushResult

		for _, slug := range targetSlugs {
			result := pushOne(client, apiClient, config, token, e2eDir, slug)
			results = append(results, result)
		}

		ok, replaced, fail := 0, 0, 0
		unchanged := len(selection.Unchanged)
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
			return ui.Fail("%d nuevo(s) · %d con versión nueva · %d sin cambios · %d con error", ok, replaced, unchanged, fail)
		}
		ui.PrintSuccess(pushSummary(ok, replaced, unchanged))
		return nil
	},
}

func selectFlowsToPush(
	e2eDir string,
	config *types.Config,
	slugs []string,
	explicit bool,
) flowSelection {
	var selection flowSelection
	for _, slug := range slugs {
		meta := loadFlowMeta(e2eDir, slug)

		switch classifyFlowOwnership(meta, config.RefId) {
		case ownershipForeign:
			if !pushForceFlag {
				selection.Foreign = append(selection.Foreign, foreignFlow{
					Slug:        slug,
					ProjectName: describeProjectOwner(meta),
				})
				continue
			}
		case ownershipOrphan:
			selection.Adopted = append(selection.Adopted, slug)
		}

		if !explicit && !pushAllFlag && isFlowUnchangedSinceLastPush(e2eDir, slug, meta) {
			selection.Unchanged = append(selection.Unchanged, unchangedFlow{
				Slug:     slug,
				PushedAt: parsePushedAt(meta.PushedAt),
			})
			continue
		}

		selection.Push = append(selection.Push, slug)
	}
	return selection
}

func isFlowUnchangedSinceLastPush(e2eDir, slug string, meta flowMeta) bool {
	spec, err := readLocalSpec(e2eDir, slug)
	if err != nil {
		return false
	}
	return isAlreadyPushed(meta, spec)
}

func parsePushedAt(raw string) time.Time {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func reportSkippedFlows(selection flowSelection, projectName string) {
	if len(selection.Foreign) > 0 {
		ui.PrintWarning(fmt.Sprintf(
			"%d flow(s) omitido(s) — fueron grabados para otro proyecto:",
			len(selection.Foreign),
		))
		for _, line := range groupForeignByProject(selection.Foreign) {
			ui.PrintStep(line)
		}
		ui.PrintHint("Cambia de proyecto con 'oraculo switch', o fuerza uno puntual con 'oraculo push <slug> --force'.")
	}

	if len(selection.Unchanged) > 0 {
		ui.PrintStep(fmt.Sprintf(
			"%d flow(s) sin cambios desde la última subida — omitidos. Usa '--all' para revisarlos igual contra el backoffice.",
			len(selection.Unchanged),
		))
	}

	if len(selection.Adopted) > 0 {
		ui.PrintStep(fmt.Sprintf(
			"%d flow(s) sin proyecto asignado — quedan vinculados a '%s' al subirlos.",
			len(selection.Adopted), projectName,
		))
	}
}

func stampPushedMeta(e2eDir, slug string, config *types.Config) {
	spec, err := readLocalSpec(e2eDir, slug)
	if err != nil {
		return
	}
	meta := loadFlowMeta(e2eDir, slug)
	meta.ProjectRefId = config.RefId
	meta.ProjectName = config.Project
	meta.PushedAt = time.Now().UTC().Format(time.RFC3339)
	meta.SpecHash = specFingerprint(spec)
	if err := saveFlowMeta(e2eDir, slug, meta); err != nil {
		ui.PrintWarning(fmt.Sprintf("'%s' subido, pero no se pudo guardar el meta local: %s", slug, err))
	}
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

	var result pushResult
	if existing != nil {
		result = replaceExistingFlow(apiClient, config, token, e2eDir, slug, existing)
	} else {
		result = createNewFlow(client, apiClient, config, token, e2eDir, slug)
	}

	if result.synced {
		stampPushedMeta(e2eDir, slug, config)
	}
	return result
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
		ui.PrintStep(fmt.Sprintf("'%s' · ya existe en el backoffice; corre 'oraculo push %s' de nuevo para ver el diff", slug, slug))
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
	return pushResult{slug: slug, ok: true, synced: true, detail: detail}
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
		return pushResult{slug: slug, unchanged: true, synced: true, detail: "sin cambios"}
	}

	if err := confirmReplace(slug, existing, localSpec); err != nil {
		if errors.Is(err, errReplaceDeclined) {
			ui.PrintStep(fmt.Sprintf("'%s' · sin cambios (cancelado)", slug))
			return pushResult{slug: slug, unchanged: true, detail: "cancelado"}
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

	detail := "versión nueva subida"
	if existing.SpecVersion > 0 {
		detail = fmt.Sprintf("versión v%d creada", existing.SpecVersion+1)
	}
	if pushCoreFlag {
		if err := apiClient.ToggleFlowCore(token, config.RefId, existing.RefId, true); err != nil {
			ui.PrintWarning(fmt.Sprintf("'%s' subido pero falló al marcar como core: %s", slug, err.Error()))
		} else {
			detail = detail + " · marcado como ★ Core"
		}
	}

	ui.PrintStep(fmt.Sprintf("'%s' · %s", slug, detail))
	ui.PrintHint("Las screenshots del flow quedaron marcadas como desactualizadas. Hoy el update solo actualiza el spec; re-subir las nuevas capturas necesita soporte en el core (pendiente).")
	return pushResult{slug: slug, replaced: true, synced: true, detail: detail}
}

func confirmReplace(slug string, existing *types.FlowSummary, localSpec string) error {
	remoteLabel := fmt.Sprintf("backoffice: %s", slug)
	if existing.SpecVersion > 0 {
		remoteLabel = fmt.Sprintf("%s v%d", remoteLabel, existing.SpecVersion)
	}
	if existing.UpdatedAt != "" {
		remoteLabel = fmt.Sprintf("%s (actualizado %s)", remoteLabel, existing.UpdatedAt)
	}
	ui.PrintSpecDiff(remoteLabel, "local: "+slug+".spec.ts", existing.SpecContent, localSpec)

	prompt := fmt.Sprintf("¿Subir '%s' como versión nueva? [s/N]", slug)
	if existing.SpecVersion > 0 {
		ui.PrintStep(fmt.Sprintf(
			"Se creará la versión v%d de '%s'. La v%d queda en el historial.",
			existing.SpecVersion+1, slug, existing.SpecVersion,
		))
		prompt = fmt.Sprintf("¿Crear la versión v%d de '%s'? [s/N]", existing.SpecVersion+1, slug)
	}
	if pushYesFlag {
		return nil
	}
	if !ui.PromptYesNo(prompt, false) {
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
		parts = append(parts, fmt.Sprintf("%d con versión nueva", replaced))
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
	for _, p := range meta.GrantedPermissions {
		_ = writer.WriteField("grantedPermissions", p)
	}
	if meta.Geolocation != nil {
		_ = writer.WriteField("geolocationLat", strconv.FormatFloat(meta.Geolocation.Lat, 'f', -1, 64))
		_ = writer.WriteField("geolocationLng", strconv.FormatFloat(meta.Geolocation.Lng, 'f', -1, 64))
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
	if len(meta.GrantedPermissions) > 0 {
		detail = fmt.Sprintf("%s · %d permiso(s)", detail, len(meta.GrantedPermissions))
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
	pushCmd.Flags().BoolVarP(&pushYesFlag, "yes", "y", false, "No pide confirmación al crear una versión nueva de un flow existente")
	pushCmd.Flags().BoolVar(&pushForceFlag, "force", false, "Sube el flow aunque haya sido grabado para otro proyecto")
	pushCmd.Flags().BoolVar(&pushAllFlag, "all", false, "Revisa todos los flows contra el backoffice, incluso los que no cambiaron desde la última subida")
	rootCmd.AddCommand(pushCmd)
}
