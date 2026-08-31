package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTeeOutputReachesTheTerminalAndTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "hunter.log")

	reader, terminal, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = terminal
	t.Cleanup(func() { os.Stdout = original })

	stop, err := teeOutput(path)
	if err != nil {
		t.Fatalf("teeOutput: %v", err)
	}
	fmt.Println("dashboard: http://127.0.0.1:8080")
	fmt.Fprintln(os.Stderr, "round failed: no target")
	stop()

	if os.Stdout != terminal {
		t.Error("teeOutput left os.Stdout swapped after stopping")
	}
	terminal.Close()

	shown, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading the terminal: %v", err)
	}
	logged, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the log: %v", err)
	}
	for _, want := range []string{"dashboard: http://127.0.0.1:8080", "round failed: no target"} {
		if !strings.Contains(string(shown), want) {
			t.Errorf("terminal missing %q, got %q", want, shown)
		}
		if !strings.Contains(string(logged), want) {
			t.Errorf("log file missing %q, got %q", want, logged)
		}
	}
}
