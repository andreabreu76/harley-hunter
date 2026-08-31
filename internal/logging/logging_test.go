package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenCreatesTheDirectoryAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "hunter.log")
	f, err := Open(path, 1024)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := f.Write([]byte("first round\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	again, err := Open(path, 1024)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	if _, err := again.Write([]byte("second round\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	again.Close()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "first round") || !strings.Contains(string(body), "second round") {
		t.Errorf("log lost a round:\n%s", body)
	}
}

func TestWriteRotatesAndKeepsTheLineThatOverflowed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hunter.log")
	f, err := Open(path, 32)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	if _, err := f.Write([]byte(strings.Repeat("a", 30) + "\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := f.Write([]byte("the line that overflowed\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current), "the line that overflowed") {
		t.Errorf("the overflowing line was dropped:\n%s", current)
	}

	rotated, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("reading the rotated file: %v", err)
	}
	if !strings.Contains(string(rotated), "aaa") {
		t.Errorf("the rotated file lost the earlier lines:\n%s", rotated)
	}
}

func TestWriteKeepsWorkingAfterARotationThatFailed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hunter.log")
	f, err := Open(path, 32)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if err := os.Mkdir(path+".1", 0o755); err != nil {
		t.Fatalf("blocking the rotation target: %v", err)
	}

	if _, err := f.Write([]byte(strings.Repeat("a", 30) + "\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := f.Write([]byte("the line that could not rotate\n")); err == nil {
		t.Error("a rotation that failed was not reported to the caller")
	}

	if _, err := f.Write([]byte("the round after the failed rotation\n")); err != nil {
		t.Fatalf("the log stopped taking writes after a failed rotation: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "the round after the failed rotation") {
		t.Errorf("the log lost every round after a failed rotation:\n%s", body)
	}
}
