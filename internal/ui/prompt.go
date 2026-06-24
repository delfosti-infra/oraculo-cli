package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func ReadLine(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return line
}

func PromptYesNo(prompt string, defaultYes bool) bool {
	answer := ReadLine("    " + prompt + " ")
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes" || answer == "s" || answer == "si" || answer == "sí"
}
