package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplayScreenshotsLookBroken(t *testing.T) {
	cases := []struct {
		name     string
		total    int
		distinct int
		want     bool
	}{
		{name: "pocas capturas nunca marca", total: 3, distinct: 1, want: false},
		{name: "todas distintas OK", total: 10, distinct: 10, want: false},
		{name: "mitad idénticas marca", total: 10, distinct: 5, want: true},
		{name: "todas idénticas marca", total: 8, distinct: 1, want: true},
		{name: "mayoría distintas OK", total: 10, distinct: 6, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := replayScreenshotsLookBroken(tc.total, tc.distinct); got != tc.want {
				t.Errorf("replayScreenshotsLookBroken(%d, %d) = %v, want %v", tc.total, tc.distinct, got, tc.want)
			}
		})
	}
}

func TestScreenshotDistinctCount(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("step-1.png", "aaa")
	write("step-2.png", "aaa")
	write("step-3.png", "bbb")
	write("nota.txt", "ignorado")

	total, distinct, err := screenshotDistinctCount(dir)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || distinct != 2 {
		t.Errorf("got total=%d distinct=%d, want 3 y 2", total, distinct)
	}
}
