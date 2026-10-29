package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

func LoadDotEnv(paths ...string) ([]string, error) {
	var loaded []string
	for _, path := range paths {
		pairs, err := parseDotEnv(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return loaded, err
		}
		for key, value := range pairs {
			if _, set := os.LookupEnv(key); !set {
				if err := os.Setenv(key, value); err != nil {
					return loaded, fmt.Errorf("set %s from %s: %w", key, path, err)
				}
			}
		}
		loaded = append(loaded, path)
	}
	return loaded, nil
}

func parseDotEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pairs := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))

		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("%s:%d: expected KEY=VALUE", path, lineNo)
		}
		pairs[key] = unquote(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return pairs, nil
}

// unquote strips one matching pair of surrounding quotes.
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
