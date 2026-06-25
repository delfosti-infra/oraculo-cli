package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const diffContextLines = 3

var (
	diffAddStyle  = lipgloss.NewStyle().Foreground(Success)
	diffDelStyle  = lipgloss.NewStyle().Foreground(Danger)
	diffMetaStyle = lipgloss.NewStyle().Foreground(InkMute).Italic(true)
)

type diffOp int

const (
	diffEqual diffOp = iota
	diffAdd
	diffDelete
)

type diffLine struct {
	op   diffOp
	text string
}

func SpecsEqual(a, b string) bool {
	return normalizeSpec(a) == normalizeSpec(b)
}

func normalizeSpec(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimRight(s, "\n") + "\n"
}

func PrintSpecDiff(remoteLabel, localLabel, remote, local string) {
	fmt.Println()
	fmt.Printf("  %s %s\n", diffDelStyle.Render("---"), diffMetaStyle.Render(remoteLabel))
	fmt.Printf("  %s %s\n", diffAddStyle.Render("+++"), diffMetaStyle.Render(localLabel))

	remoteLines := strings.Split(normalizeSpec(remote), "\n")
	localLines := strings.Split(normalizeSpec(local), "\n")
	lines := diffLines(remoteLines, localLines)
	printCollapsedDiff(lines)
	fmt.Println()
}

func printCollapsedDiff(lines []diffLine) {
	visible := markVisible(lines)
	skipping := false
	for i, line := range lines {
		if !visible[i] {
			if !skipping {
				fmt.Printf("  %s\n", diffMetaStyle.Render("⋮"))
				skipping = true
			}
			continue
		}
		skipping = false
		switch line.op {
		case diffAdd:
			fmt.Printf("  %s\n", diffAddStyle.Render("+ "+line.text))
		case diffDelete:
			fmt.Printf("  %s\n", diffDelStyle.Render("- "+line.text))
		default:
			fmt.Printf("  %s\n", diffMetaStyle.Render("  "+line.text))
		}
	}
}

func markVisible(lines []diffLine) []bool {
	visible := make([]bool, len(lines))
	for i, line := range lines {
		if line.op == diffEqual {
			continue
		}
		lo := i - diffContextLines
		if lo < 0 {
			lo = 0
		}
		hi := i + diffContextLines
		if hi >= len(lines) {
			hi = len(lines) - 1
		}
		for j := lo; j <= hi; j++ {
			visible[j] = true
		}
	}
	return visible
}

func diffLines(a, b []string) []diffLine {
	lcs := lcsTable(a, b)
	var out []diffLine
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			out = append(out, diffLine{op: diffEqual, text: a[i]})
			i++
			j++
			continue
		}
		if lcs[i+1][j] >= lcs[i][j+1] {
			out = append(out, diffLine{op: diffDelete, text: a[i]})
			i++
		} else {
			out = append(out, diffLine{op: diffAdd, text: b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		out = append(out, diffLine{op: diffDelete, text: a[i]})
	}
	for ; j < len(b); j++ {
		out = append(out, diffLine{op: diffAdd, text: b[j]})
	}
	return out
}

func lcsTable(a, b []string) [][]int {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}
	return table
}
