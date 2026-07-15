package cmd

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	duplicateCheckMinScreenshots = 4
	duplicateDistinctMinRatio    = 0.5
)

func screenshotDistinctCount(dir string) (total int, distinct int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, fmt.Errorf("leer screenshots en %s: %w", dir, err)
	}
	hashes := make(map[[sha256.Size]byte]bool)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "step-") || !strings.HasSuffix(name, ".png") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			return 0, 0, fmt.Errorf("leer %s: %w", name, readErr)
		}
		total++
		hashes[sha256.Sum256(data)] = true
	}
	return total, len(hashes), nil
}

func replayScreenshotsLookBroken(total, distinct int) bool {
	if total < duplicateCheckMinScreenshots {
		return false
	}
	return float64(distinct)/float64(total) <= duplicateDistinctMinRatio
}
