package tui

import (
	"fmt"
	"strings"
)

func (m model) View() string {
	if m.quitting {
		return ""
	}
	// Frame mínimo mientras el subcomando tiene la terminal: el flush previo a
	// ceder borra el prompt/footer y no quedan frames duplicados en el scrollback.
	// El resultado se registra como línea persistente (tea.Println) al terminar.
	if m.executing {
		return "\n"
	}

	var b strings.Builder
	b.WriteString("\n")

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
