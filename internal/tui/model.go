package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type viewMode int

const (
	modePalette viewMode = iota
	modeArgs
)

type historyEntry struct {
	line string
	ok   bool
}

type model struct {
	root     *cobra.Command
	all      []command
	filtered []command
	input    textinput.Model
	argInput textinput.Model
	selected int
	mode     viewMode
	pending  command
	history  []historyEntry
	width    int
	height   int
	quitting bool
}

func newModel(root *cobra.Command) model {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "escribe / para ver los comandos…"
	input.Focus()

	argInput := textinput.New()
	argInput.Prompt = ""

	commands := collectCommands(root)
	return model{
		root:     root,
		all:      commands,
		filtered: commands,
		input:    input,
		argInput: argInput,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) slashMode() bool {
	return strings.HasPrefix(strings.TrimSpace(m.input.Value()), "/")
}
