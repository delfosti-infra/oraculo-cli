package tui

import (
	"fmt"
	"strings"
)

const maxHistoryLines = 6

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")

	if len(m.history) > 0 {
		start := 0
		if len(m.history) > maxHistoryLines {
			start = len(m.history) - maxHistoryLines
		}
		for _, h := range m.history[start:] {
			mark := successStyle.Render("✓")
			if !h.ok {
				mark = errorStyle.Render("✗")
			}
			b.WriteString(fmt.Sprintf("  %s %s\n", mark, historyStyle.Render(h.line)))
		}
		b.WriteString("\n")
	}

	if m.mode == modeArgs {
		b.WriteString(fmt.Sprintf("  %s %s\n", promptStyle.Render(m.pending.name), m.argInput.View()))
		b.WriteString("\n  " + footerStyle.Render("Enter ejecutar · Esc cancelar") + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("  %s %s\n", cursorStyle.Render(">"), m.input.View()))

	if !m.slashMode() {
		b.WriteString("\n  " + footerStyle.Render("/ comandos · Esc salir") + "\n")
		return b.String()
	}

	b.WriteString("\n")
	if len(m.filtered) == 0 {
		b.WriteString("  " + hintStyle.Render("Sin coincidencias") + "\n")
	}
	for i, c := range m.filtered {
		b.WriteString(m.renderRow(i, c) + "\n")
	}

	b.WriteString("\n  " + footerStyle.Render("↑↓ navegar · Enter ejecutar · Tab completar · Esc salir") + "\n")
	return b.String()
}

func (m model) renderRow(i int, c command) string {
	marker := "  "
	name := nameStyle.Render("/" + c.name)
	if i == m.selected {
		marker = cursorStyle.Render("›") + " "
		name = nameSelectedStyle.Render("/" + c.name)
	}

	row := marker + name
	if c.argHint != "" {
		row += " " + hintStyle.Render(c.argHint)
	}
	if c.short != "" {
		row += "  " + shortStyle.Render(c.short)
	}
	return row
}
