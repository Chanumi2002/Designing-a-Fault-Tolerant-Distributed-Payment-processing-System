package utils

import (
	"bufio"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type PersistedLogView struct {
	Time    string
	Message string
}

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

func GetLogFilePath(filename string) string {
	return filepath.Join("log", filename)
}

func ReadPersistedLogs(filename string, limit int) ([]PersistedLogView, error) {
	fullPath := GetLogFilePath(filename)

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []PersistedLogView{}, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]PersistedLogView, 0)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		logLine := parsePersistedLogLine(line)
		if logLine.Message == "" {
			continue
		}

		lines = append(lines, logLine)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	return lines, nil
}

func parsePersistedLogLine(line string) PersistedLogView {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return PersistedLogView{
			Time:    "",
			Message: line,
		}
	}

	timePart := parts[1]
	if len(timePart) >= 8 {
		timePart = timePart[:8]
	}

	return PersistedLogView{
		Time:    timePart,
		Message: parts[2],
	}
}
