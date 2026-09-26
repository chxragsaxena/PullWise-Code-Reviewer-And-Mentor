// Package config reads server settings from the environment.
package config

import "os"

// Config holds process-wide settings.
type Config struct {
	// Addr is the address the HTTP server listens on.
	Addr string
	// SkillsDir is the directory containing skill folders.
	SkillsDir string
}

// Load reads configuration from environment variables, applying defaults.
func Load() Config {
	return Config{
		Addr:      getenv("ADDR", ":8080"),
		SkillsDir: getenv("SKILLS_DIR", "skills"),
	}
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
