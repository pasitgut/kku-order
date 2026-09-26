package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// loadLocalEnvironment loads the project .env.local for direct host runs.
// Values already provided by the process environment take precedence.
func loadLocalEnvironment() error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	projectRoot := findProjectRoot(workingDirectory)
	paths := []string{filepath.Join(projectRoot, ".env.local")}
	if projectRoot != workingDirectory {
		paths = append(paths, filepath.Join(workingDirectory, ".env.local"))
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return loadEnvironmentFile(path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect local environment file: %w", err)
		}
	}
	return nil
}

func findProjectRoot(start string) string {
	for directory := start; ; directory = filepath.Dir(directory) {
		for _, marker := range []string{".git", "docker-compose.yml"} {
			if _, err := os.Stat(filepath.Join(directory, marker)); err == nil {
				return directory
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return start
		}
	}
}

func loadEnvironmentFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open local environment file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, rawValue, found := strings.Cut(line, "=")
		if !found {
			return fmt.Errorf("local environment file has invalid assignment at line %d", lineNumber)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("local environment file has empty key at line %d", lineNumber)
		}
		value, err := parseEnvironmentValue(strings.TrimSpace(rawValue))
		if err != nil {
			return fmt.Errorf("local environment file has invalid value at line %d: %w", lineNumber, err)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set environment variable at line %d: %w", lineNumber, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read local environment file: %w", err)
	}
	return nil
}

func parseEnvironmentValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if raw[0] == '\'' || raw[0] == '"' {
		quote := raw[0]
		for index := 1; index < len(raw); index++ {
			if quote == '"' && raw[index] == '\\' {
				index++
				continue
			}
			if raw[index] == quote {
				remainder := strings.TrimSpace(raw[index+1:])
				if remainder != "" && !strings.HasPrefix(remainder, "#") {
					return "", fmt.Errorf("unexpected characters after quoted value")
				}
				if quote == '"' {
					return strconv.Unquote(raw[:index+1])
				}
				return raw[1:index], nil
			}
		}
		return "", fmt.Errorf("unterminated quoted value")
	}

	if comment := strings.Index(raw, " #"); comment >= 0 {
		raw = raw[:comment]
	}
	return strings.TrimSpace(raw), nil
}
