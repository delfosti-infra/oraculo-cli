package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/delfosti-infra/oraculo-cli/internal/ui"
)

var (
	promptStyle = lipgloss.NewStyle().Foreground(ui.Gold).Bold(true)

	titleStyle = lipgloss.NewStyle().Foreground(ui.Gold).Italic(true).Bold(true)

	nameStyle = lipgloss.NewStyle().Foreground(ui.Gold)

	nameSelectedStyle = lipgloss.NewStyle().Foreground(ui.Gold).Bold(true)

	shortStyle = lipgloss.NewStyle().Foreground(ui.InkSoft)

	hintStyle = lipgloss.NewStyle().Foreground(ui.InkMute)

	footerStyle = lipgloss.NewStyle().Foreground(ui.InkMute)

	cursorStyle = lipgloss.NewStyle().Foreground(ui.Gold).Bold(true)

	historyStyle = lipgloss.NewStyle().Foreground(ui.InkMute)

	errorStyle = lipgloss.NewStyle().Foreground(ui.Danger)

	successStyle = lipgloss.NewStyle().Foreground(ui.Success)
)
