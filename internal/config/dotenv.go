package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		value = strings.ReplaceAll(value, `\n`, string([]byte{10}))
		if existing, exists := os.LookupEnv(key); !exists || strings.TrimSpace(existing) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func loadConfigDotEnv() {
	if path := strings.TrimSpace(os.Getenv("ENV_FILE")); path != "" {
		loadDotEnv(path)
		return
	}

	cwd, _ := os.Getwd()
	paths := []string{
		filepath.Join(executableDir(), ".env"),
		filepath.Join(cwd, ".env"),
		filepath.Join(cwd, "backend", ".env"),
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		loadDotEnv(clean)
	}
}

func executableDir() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(path)
}
