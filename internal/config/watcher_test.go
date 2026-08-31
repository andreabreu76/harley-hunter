package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfigFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(time.Duration(len(body)) * time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherPicksUpAnEditedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfigFile(t, path, "sources:\n  - olx\n")

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	if got := w.Current().Sources; len(got) != 1 || got[0] != "olx" {
		t.Fatalf("Sources = %v, want [olx]", got)
	}

	writeConfigFile(t, path, "sources:\n  - olx\n  - webmotors\n")
	if got := w.Current().Sources; len(got) != 2 {
		t.Errorf("Sources = %v, want the two from the edited file", got)
	}
}

func TestWatcherKeepsTheLastGoodConfigWhenTheFileBreaks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	writeConfigFile(t, path, "sources:\n  - olx\n")

	w, err := NewWatcher(path)
	if err != nil {
		t.Fatalf("NewWatcher: %v", err)
	}
	var warned bytes.Buffer
	w.warn = &warned

	writeConfigFile(t, path, "sources: [unclosed\n")
	if got := w.Current().Sources; len(got) != 1 || got[0] != "olx" {
		t.Errorf("Sources = %v, want the last good config", got)
	}
	if warned.Len() == 0 {
		t.Error("a broken config was swallowed without a warning")
	}
}

func TestWatcherRefusesAMissingFileUpFront(t *testing.T) {
	if _, err := NewWatcher(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("NewWatcher accepted a path with no file")
	}
}
