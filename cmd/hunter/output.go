package main

import (
	"fmt"
	"io"
	"os"

	"github.com/andreabreu76/harley-hunter/internal/logging"
)

const maxLogBytes = 5 << 20

const drainBuffer = 32 << 10

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

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		drain(reader, terminalOut, file, terminalErr)
	}()

	return func() {
		os.Stdout, os.Stderr = terminalOut, terminalErr
		writer.Close()
		<-drained
		file.Close()
	}, nil
}

func drain(reader io.Reader, terminal, file, complaints io.Writer) {
	buffer := make([]byte, drainBuffer)
	reported := false
	for {
		read, readErr := reader.Read(buffer)
		if read > 0 {
			terminal.Write(buffer[:read])
			if _, err := file.Write(buffer[:read]); err != nil && !reported {
				reported = true
				fmt.Fprintf(complaints, "the log file stopped taking writes, the hunt goes on without it: %v\n", err)
			}
		}
		if readErr != nil {
			return
		}
	}
}
