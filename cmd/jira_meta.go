package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
)

type flowMeta struct {
	JiraIssueKeys   []string `json:"jiraIssueKeys,omitempty"`
	UsesAuthSession bool     `json:"usesAuthSession,omitempty"`
	Platform        string   `json:"platform,omitempty"`
}

func metaPath(e2eDir, slug string) string {
	return filepath.Join(e2eDir, ".oraculo", slug+".meta.json")
}

func loadFlowMeta(e2eDir, slug string) flowMeta {
	var m flowMeta
	data, err := os.ReadFile(metaPath(e2eDir, slug))
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, &m)
	return m
}

func saveFlowMeta(e2eDir, slug string, m flowMeta) error {
	path := metaPath(e2eDir, slug)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("crear directorio de meta '%s': %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar meta del flow '%s': %w", slug, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("escribir meta del flow '%s': %w", path, err)
	}
	return nil
}

var jiraIssueKeyRegex = regexp.MustCompile(`[A-Z][A-Z0-9_]*-\d+`)

func validateJiraIssueKeys(raw []string) ([]string, []string) {
	seen := make(map[string]bool)
	var valid, invalid []string
	for _, item := range raw {
		for _, k := range strings.Split(item, ",") {
			k = strings.TrimSpace(strings.ToUpper(k))
			if k == "" {
				continue
			}
			if !jiraIssueKeyRegex.MatchString(k) || jiraIssueKeyRegex.FindString(k) != k {
				invalid = append(invalid, k)
				continue
			}
			if seen[k] {
				continue
			}
			seen[k] = true
			valid = append(valid, k)
		}
	}
	return valid, invalid
}

// detectHUsFromGitBranch ejecuta `git rev-parse --abbrev-ref HEAD` y extrae
// todas las HU keys que matcheen el patrón "ABC-123" en el nombre de la branch.
//
// Soporta patrones comunes:
//
// Si la branch no tiene ningún match o git no está disponible, devuelve nil.
func detectHUsFromGitBranch() (branch string, keys []string) {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", nil
	}
	branch = strings.TrimSpace(string(out))
	if branch == "" || branch == "HEAD" {
		return branch, nil
	}
	matches := jiraIssueKeyRegex.FindAllString(branch, -1)
	seen := make(map[string]bool)
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			keys = append(keys, m)
		}
	}
	return branch, keys
}

var (
	wandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D4B373")).
			Bold(true)
	branchStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)
	keyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B89968")).
			Bold(true)
)

// promptForHUsFromBranch muestra los keys detectados y pregunta al usuario
// si quiere usarlos. Devuelve la lista final (puede ser vacía si el usuario rechaza).
func promptForHUsFromBranch(branch string, detected []string) []string {
	if len(detected) == 0 {
		return nil
	}

	fmt.Println()
	fmt.Printf("  %s %s\n",
		wandStyle.Render("🪄"),
		wandStyle.Render("HU detectada en branch"),
	)
	fmt.Printf("    %s %s\n", branchStyle.Render("branch:"), branchStyle.Render(branch))

	if len(detected) == 1 {
		fmt.Printf("    %s %s\n", branchStyle.Render("HU:    "), keyStyle.Render(detected[0]))
		fmt.Println()
		if askYesNo(fmt.Sprintf("Linkear %s a este flow? [Y/n]", keyStyle.Render(detected[0])), true) {
			return detected
		}
		return nil
	}

	// Múltiples HUs en la branch
	fmt.Printf("    %s\n", branchStyle.Render("HUs:"))
	for i, k := range detected {
		fmt.Printf("      %s %s\n",
			branchStyle.Render(fmt.Sprintf("[%d]", i+1)),
			keyStyle.Render(k),
		)
	}
	fmt.Println()
	fmt.Printf("    %s\n", branchStyle.Render("Linkear: [a] todas · [1-N] una específica · [n] ninguna"))
	answer := readLine("    > ")
	answer = strings.TrimSpace(strings.ToLower(answer))
	switch {
	case answer == "" || answer == "a" || answer == "all":
		return detected
	case answer == "n" || answer == "no":
		return nil
	default:
		// número o lista de números separados por comas
		var picked []string
		for _, idxStr := range strings.Split(answer, ",") {
			idxStr = strings.TrimSpace(idxStr)
			var idx int
			_, err := fmt.Sscanf(idxStr, "%d", &idx)
			if err == nil && idx >= 1 && idx <= len(detected) {
				picked = append(picked, detected[idx-1])
			}
		}
		return picked
	}
}

func askYesNo(prompt string, defaultYes bool) bool {
	answer := readLine("    " + prompt + " ")
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes" || answer == "s" || answer == "si" || answer == "sí"
}

func readLine(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return line
}

// printLinkedHUs imprime la confirmación de HUs que quedan asociadas al flow.
func printLinkedHUs(keys []string) {
	if len(keys) == 0 {
		return
	}
	rendered := make([]string, len(keys))
	for i, k := range keys {
		rendered[i] = keyStyle.Render(k)
	}
	ui.PrintStep(fmt.Sprintf(
		"%d HU(s) asociada(s): %s",
		len(keys),
		strings.Join(rendered, ", "),
	))
}

// resolveHUsForRecord decide qué HUs van a quedar asociadas al flow.
// Prioridad:
//  1. Si el user pasó --HU explícito → solo esos (validados + dedupeados)
//  2. Si no, intenta auto-detect desde la branch de git y pregunta confirmación
//  3. Si no hay nada → devuelve nil (flow sin HU asociada)
func resolveHUsForRecord(flagValues []string) []string {
	if len(flagValues) > 0 {
		valid, invalid := validateJiraIssueKeys(flagValues)
		if len(invalid) > 0 {
			ui.PrintWarning(fmt.Sprintf(
				"HU(s) con formato inválido descartadas: %s",
				strings.Join(invalid, ", "),
			))
		}
		if len(valid) > 0 {
			printLinkedHUs(valid)
		}
		return valid
	}

	branch, detected := detectHUsFromGitBranch()
	if len(detected) == 0 {
		return nil
	}
	picked := promptForHUsFromBranch(branch, detected)
	if len(picked) > 0 {
		printLinkedHUs(picked)
	}
	return picked
}
