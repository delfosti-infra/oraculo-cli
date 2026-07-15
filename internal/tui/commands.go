package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type command struct {
	name     string
	argHint  string
	short    string
	required bool
}

var excludedCommands = map[string]bool{
	"help":       true,
	"completion": true,
	"shell":      true,
}

func collectCommands(root *cobra.Command) []command {
	commands := make([]command, 0, len(root.Commands()))
	for _, c := range root.Commands() {
		if c.Hidden || excludedCommands[c.Name()] {
			continue
		}
		name, hint, _ := strings.Cut(c.Use, " ")
		commands = append(commands, command{
			name:     name,
			argHint:  strings.TrimSpace(hint),
			short:    c.Short,
			required: strings.Contains(hint, "<"),
		})
	}
	return commands
}

func filterCommands(commands []command, query string) []command {
	q := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(query), "/"))
	if q == "" {
		return commands
	}
	matches := make([]command, 0, len(commands))
	for _, c := range commands {
		if strings.Contains(strings.ToLower(c.name), q) {
			matches = append(matches, c)
		}
	}
	return matches
}

type commandFinishedMsg struct {
	line string
	err  error
}

func runCommand(name string, args []string) tea.Cmd {
	full := append([]string{name}, args...)
	proc := exec.Command(os.Args[0], full...)
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	line := "/" + strings.TrimSpace(name+" "+strings.Join(args, " "))
	return tea.ExecProcess(proc, func(err error) tea.Msg {
		return commandFinishedMsg{line: line, err: err}
	})
}
