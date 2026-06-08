package ui

import (
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().
			Foreground(colorInk).
			Bold(true)

	eyebrowStyle = lipgloss.NewStyle().
			Foreground(colorGold).
			Bold(true)

	hintStyle = lipgloss.NewStyle().
			Foreground(colorInkMute).
			Italic(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorInkSoft)

	arrowStyle = lipgloss.NewStyle().
			Foreground(colorGold).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess).
			Bold(true)

	errorMarkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B5443A")).
			Bold(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(colorGold)
)

func PrintHeader(eyebrow, title, hint string) {
	fmt.Println()
	fmt.Printf("  %s %s\n", arrowStyle.Render("❯"), eyebrowStyle.Render(eyebrow))
	fmt.Printf("  %s\n", headerStyle.Render(title))
	if hint != "" {
		fmt.Printf("  %s\n", hintStyle.Render(hint))
	}
	fmt.Println()
}

func PromptLabel(label string) {
	fmt.Printf("  %s %s  ", arrowStyle.Render("·"), labelStyle.Render(padRight(label, 10)))
}

func PrintSuccess(message string) {
	fmt.Printf("\n  %s %s\n\n", successStyle.Render("✓"), message)
}

func PrintError(message string) {
	fmt.Printf("\n  %s %s\n\n", errorMarkStyle.Render("✗"), message)
}

func PrintStep(message string) {
	fmt.Printf("  %s %s\n", successStyle.Render("✓"), labelStyle.Render(message))
}

func PrintHint(message string) {
	fmt.Printf("  %s %s\n", arrowStyle.Render("→"), hintStyle.Render(message))
}

func PrintWarning(message string) {
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#C2902B")).Bold(true)
	fmt.Printf("  %s %s\n", warningStyle.Render("!"), labelStyle.Render(message))
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + repeat(" ", width-len(s))
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

type Spinner struct {
	message string
	stop    chan struct{}
	wg      sync.WaitGroup
}

func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		stop:    make(chan struct{}),
	}
}

func (s *Spinner) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-s.stop:
				fmt.Print("\r\033[2K")
				return
			case <-ticker.C:
				fmt.Printf("\r  %s %s", spinnerStyle.Render(frames[i]), s.message)
				i = (i + 1) % len(frames)
			}
		}
	}()
}

func (s *Spinner) Stop() {
	close(s.stop)
	s.wg.Wait()
}
