package main

import (
	"io"
	"os"

	"github.com/andreabreu76/harley-hunter/internal/logging"
)

const maxLogBytes = 5 << 20

func teeOutput(path string) (func(), error) {
	file, err := logging.Open(path, maxLogBytes)
	if err != nil {
		return nil, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		file.Close()
		return nil, err
	}

	terminalOut, terminalErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = writer, writer

	copied := make(chan struct{})
	go func() {
		io.Copy(io.MultiWriter(terminalOut, file), reader)
		close(copied)
	}()

	return func() {
		os.Stdout, os.Stderr = terminalOut, terminalErr
		writer.Close()
		<-copied
		file.Close()
	}, nil
}
