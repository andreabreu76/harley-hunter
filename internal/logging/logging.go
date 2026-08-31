package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type File struct {
	path string
	max  int64
	mu   sync.Mutex
	file *os.File
	size int64
}

func Open(path string, maxBytes int64) (*File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating the log directory: %w", err)
	}
	handle, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening the log file: %w", err)
	}
	info, err := handle.Stat()
	if err != nil {
		handle.Close()
		return nil, fmt.Errorf("sizing the log file: %w", err)
	}
	return &File{path: path, max: maxBytes, file: handle, size: info.Size()}, nil
}

func (f *File) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.max > 0 && f.size+int64(len(p)) > f.max {
		if err := f.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := f.file.Write(p)
	f.size += int64(n)
	return n, err
}

func (f *File) rotate() error {
	if err := f.file.Close(); err != nil {
		return fmt.Errorf("closing the log file before rotating: %w", err)
	}
	if err := os.Rename(f.path, f.path+".1"); err != nil {
		return fmt.Errorf("rotating the log file: %w", err)
	}
	handle, err := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening the log file after rotating: %w", err)
	}
	f.file = handle
	f.size = 0
	return nil
}

func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Close()
}
