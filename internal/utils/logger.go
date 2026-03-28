package utils

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

func SetupGlobalFileLogger(filename string) (*os.File, error) {
	if err := os.MkdirAll("log", 0755); err != nil {
		return nil, err
	}

	fullPath := filepath.Join("log", filename)

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	writer := io.MultiWriter(os.Stdout, file)
	log.SetOutput(writer)
	log.SetFlags(log.Ldate | log.Ltime)

	return file, nil
}
