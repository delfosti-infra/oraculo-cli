package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case commandFinishedMsg:
		m.history = append(m.history, historyEntry{line: msg.line, ok: msg.err == nil})
		m.input.SetValue("")
		m.filtered = m.all
		m.selected = 0
		return m, nil

	case tea.KeyMsg:
		if m.mode == modeArgs {
			return m.updateArgs(msg)
		}
		return m.updatePalette(msg)
	}

	return m, nil
}

func (m model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc":
		m.quitting = true
		return m, tea.Quit

	case "up", "ctrl+p":
		if m.slashMode() && m.selected > 0 {
			m.selected--
		}
		return m, nil

	case "down", "ctrl+n":
		if m.slashMode() && m.selected < len(m.filtered)-1 {
			m.selected++
		}
		return m, nil

	case "tab":
		if m.slashMode() && len(m.filtered) > 0 {
			m.input.SetValue("/" + m.filtered[m.selected].name + " ")
			m.input.CursorEnd()
		}
		return m, nil

	case "enter":
		if !m.slashMode() {
			return m, nil
		}
		return m.submitPalette()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.filtered = filterCommands(m.all, m.input.Value())
	if m.selected >= len(m.filtered) {
		m.selected = max(0, len(m.filtered)-1)
	}
	return m, cmd
}

func (m model) submitPalette() (tea.Model, tea.Cmd) {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(m.input.Value()), "/"))

	if len(fields) > 1 {
		if c, ok := m.findByName(fields[0]); ok {
			return m.dispatch(c, fields[1:])
		}
	}

	if len(m.filtered) == 0 {
		return m, nil
	}
	selected := m.filtered[m.selected]
	if len(fields) == 1 {
		if c, ok := m.findByName(fields[0]); ok {
			selected = c
		}
	}

	if selected.required {
		m.mode = modeArgs
		m.pending = selected
		m.argInput.SetValue("")
		m.argInput.Placeholder = selected.argHint
		m.argInput.Focus()
		return m, textinput.Blink
	}
	return m.dispatch(selected, nil)
}

func (m model) updateArgs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "esc":
		m.mode = modePalette
		return m, nil

	case "enter":
		args := strings.Fields(m.argInput.Value())
		if m.pending.required && len(args) == 0 {
			return m, nil
		}
		m.mode = modePalette
		return m.dispatch(m.pending, args)
	}

	var cmd tea.Cmd
	m.argInput, cmd = m.argInput.Update(msg)
	return m, cmd
}

func (m model) dispatch(c command, args []string) (tea.Model, tea.Cmd) {
	m.input.SetValue("")
	m.filtered = m.all
	m.selected = 0
	m.mode = modePalette
	return m, runCommand(c.name, args)
}

func (m model) findByName(name string) (command, bool) {
	for _, c := range m.all {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}
