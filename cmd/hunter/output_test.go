package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

type brokenFile struct {
	acceptedWrites int
	writes         int
	err            error
}

func (b *brokenFile) Write(p []byte) (int, error) {
	b.writes++
	if b.writes > b.acceptedWrites {
		return 0, b.err
	}
	return len(p), nil
}

func TestDrainKeepsTheTerminalFedWhenTheLogFileFails(t *testing.T) {
	cases := []struct {
		name           string
		acceptedWrites int
	}{
		{"the file fails on the very first line", 0},
		{"the file fails halfway through the round", 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}
			file := &brokenFile{acceptedWrites: c.acceptedWrites, err: errors.New("no space left on device")}
			var terminal, complaints bytes.Buffer

			drained := make(chan struct{})
			go func() {
				defer close(drained)
				drain(reader, &terminal, file, &complaints)
			}()

			const rounds = 400
			printed := make(chan struct{})
			go func() {
				defer close(printed)
				for i := 0; i < rounds; i++ {
					fmt.Fprintf(writer, "round %03d: %s\n", i, strings.Repeat("olx ok: 12 items ", 10))
				}
				writer.Close()
			}()

			select {
			case <-printed:
			case <-time.After(5 * time.Second):
				t.Fatal("printing blocked: the drain stopped reading the pipe after the log file failed")
			}
			<-drained

			shown := terminal.String()
			for _, want := range []string{"round 000:", fmt.Sprintf("round %03d:", rounds-1)} {
				if !strings.Contains(shown, want) {
					t.Errorf("the terminal lost %q after the log file failed", want)
				}
			}
			if got := strings.Count(complaints.String(), "\n"); got != 1 {
				t.Errorf("the log failure was reported %d times, want exactly 1:\n%s", got, complaints.String())
			}
			if !strings.Contains(complaints.String(), "no space left on device") {
				t.Errorf("the report does not say why the log file failed: %q", complaints.String())
			}
		})
	}
}
