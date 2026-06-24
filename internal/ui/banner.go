package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

const version = "v1.1.0"

const asciiLogo = ` ██████╗ ██████╗  █████╗  ██████╗██╗   ██╗██╗      ██████╗
██╔═══██╗██╔══██╗██╔══██╗██╔════╝██║   ██║██║     ██╔═══██╗
██║   ██║██████╔╝███████║██║     ██║   ██║██║     ██║   ██║
██║   ██║██╔══██╗██╔══██║██║     ██║   ██║██║     ██║   ██║
╚██████╔╝██║  ██║██║  ██║╚██████╗╚██████╔╝███████╗╚██████╔╝
 ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝ ╚═════╝ `

var (
	colorGold    = Gold
	colorInk     = Ink
	colorInkSoft = InkSoft
	colorInkMute = InkMute
	colorSuccess = Success
	colorDarkBg  = DarkBg
)

var (
	logoStyle = lipgloss.NewStyle().
			Foreground(colorGold).
			Bold(true)

	logoDarkStyle = lipgloss.NewStyle().
			Foreground(colorDarkBg).
			Bold(true)

	deltaStyle = lipgloss.NewStyle().
			Foreground(colorGold).
			Italic(true).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorInkSoft)

	versionStyle = lipgloss.NewStyle().
			Foreground(colorInkMute).
			Italic(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(colorInkMute)
)

const (
	sweepStepDelay = 45 * time.Millisecond
	sweepStepWidth = 2
	finalPause     = 250 * time.Millisecond
)

const (
	ansiCursorUp  = "\033[%dA"
	ansiClearLine = "\033[2K"
	ansiLineStart = "\r"
)

func isTTY() bool {
	return isatty.IsTerminal(os.Stdout.Fd())
}

func PrintBanner() {
	animated := isTTY()
	lines := strings.Split(asciiLogo, "\n")

	fmt.Println()

	if animated {
		renderLogoSweep(lines)
	} else {
		for _, l := range lines {
			fmt.Println(logoStyle.Render(l))
		}
	}

	fmt.Println()

	delta := deltaStyle.Render("Δ")
	subtitle := subtitleStyle.Render("DelfosTI · Plataforma para grabar, refactorizar y ejecutar flujos Playwright")
	separator := separatorStyle.Render("·")
	versionText := versionStyle.Render(version)

	fmt.Printf("  %s  %s  %s  %s\n\n", delta, subtitle, separator, versionText)

	if animated {
		time.Sleep(finalPause)
	}
}

func renderLogoSweep(lines []string) {
	runeLines := make([][]rune, len(lines))
	maxWidth := 0
	for i, l := range lines {
		runeLines[i] = []rune(l)
		if len(runeLines[i]) > maxWidth {
			maxWidth = len(runeLines[i])
		}
	}

	for reveal := 0; reveal <= maxWidth; reveal += sweepStepWidth {
		if reveal > 0 {
			fmt.Printf(ansiCursorUp, len(lines))
		}

		for _, runes := range runeLines {
			cut := reveal
			if cut > len(runes) {
				cut = len(runes)
			}

			gold := logoStyle.Render(string(runes[:cut]))
			dark := logoDarkStyle.Render(string(runes[cut:]))

			fmt.Print(ansiClearLine + ansiLineStart)
			fmt.Println(gold + dark)
		}

		time.Sleep(sweepStepDelay)
	}

	fmt.Printf(ansiCursorUp, len(lines))
	for _, runes := range runeLines {
		fmt.Print(ansiClearLine + ansiLineStart)
		fmt.Println(logoStyle.Render(string(runes)))
	}
}
