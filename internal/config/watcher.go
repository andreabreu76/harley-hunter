package config

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Watcher struct {
	path    string
	mu      sync.RWMutex
	current Config
	modTime time.Time
	warn    io.Writer
}

func NewWatcher(path string) (*Watcher, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	w := &Watcher{path: path, current: cfg, warn: os.Stderr}
	if info, err := os.Stat(path); err == nil {
		w.modTime = info.ModTime()
	}
	return w, nil
}

func (w *Watcher) Path() string { return w.path }

func (w *Watcher) Current() Config {
	info, err := os.Stat(w.path)
	if err != nil {
		return w.snapshot()
	}
	w.mu.RLock()
	unchanged := info.ModTime().Equal(w.modTime)
	w.mu.RUnlock()
	if unchanged {
		return w.snapshot()
	}

	cfg, err := Load(w.path)
	if err != nil {
		fmt.Fprintf(w.warn, "config at %s is unreadable, keeping the last good one: %v\n", w.path, err)
		w.mu.Lock()
		w.modTime = info.ModTime()
		w.mu.Unlock()
		return w.snapshot()
	}

	w.mu.Lock()
	w.current = cfg
	w.modTime = info.ModTime()
	w.mu.Unlock()
	return cfg
}

func (w *Watcher) snapshot() Config {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.current
}
