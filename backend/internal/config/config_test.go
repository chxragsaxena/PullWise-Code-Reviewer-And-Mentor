package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// clearEnv blanks every variable Load reads, so tests start from a known state.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ADDR", "SKILLS_DIR", "DATABASE_URL", "GITHUB_TOKEN", "MAX_CONCURRENT_REVIEWS",
		"MAX_QUEUED_REVIEWS", "CREATE_RATE_PER_MINUTE", "CREATE_BURST", "REVIEW_TIMEOUT",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		Addr: ":8080", SkillsDir: "skills", DatabaseURL: "postgres://x",
		MaxConcurrentReviews: 2, MaxQueuedReviews: 50, ReviewTimeout: 5 * time.Minute,
		CreateRatePerMinute: 20, CreateBurst: 10,
	}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestLoadFromEnv(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("ADDR", ":9090")
	t.Setenv("SKILLS_DIR", "/opt/skills")
	t.Setenv("GITHUB_TOKEN", "ghp_x")
	t.Setenv("MAX_CONCURRENT_REVIEWS", "4")
	t.Setenv("MAX_QUEUED_REVIEWS", "8")
	t.Setenv("CREATE_RATE_PER_MINUTE", "0") // turns the limit off
	t.Setenv("CREATE_BURST", "3")
	t.Setenv("REVIEW_TIMEOUT", "90s")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		Addr: ":9090", SkillsDir: "/opt/skills", DatabaseURL: "postgres://db", GitHubToken: "ghp_x",
		MaxConcurrentReviews: 4, MaxQueuedReviews: 8, ReviewTimeout: 90 * time.Second,
		CreateRatePerMinute: 0, CreateBurst: 3,
	}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	clearEnv(t) // no DATABASE_URL
	t.Setenv("MAX_CONCURRENT_REVIEWS", "0")
	t.Setenv("CREATE_BURST", "lots")
	t.Setenv("REVIEW_TIMEOUT", "soon")

	_, err := Load()
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"DATABASE_URL is required", "MAX_CONCURRENT_REVIEWS", "CREATE_BURST", "REVIEW_TIMEOUT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error is missing %q:\n%v", want, err)
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".env", `
# a comment
DOTENV_PLAIN=plain
DOTENV_QUOTED="with spaces"
DOTENV_SINGLE='single'
export DOTENV_EXPORTED=yes
DOTENV_EQUALS=a=b=c
DOTENV_EMPTY=
DOTENV_ALREADY_SET=from-file
`)
	t.Setenv("DOTENV_ALREADY_SET", "from-environment")
	for _, k := range []string{"DOTENV_PLAIN", "DOTENV_QUOTED", "DOTENV_SINGLE", "DOTENV_EXPORTED", "DOTENV_EQUALS", "DOTENV_EMPTY"} {
		os.Unsetenv(k)
		t.Cleanup(func() { os.Unsetenv(k) })
	}

	loaded, err := LoadDotEnv(filepath.Join(dir, "missing.env"), path)
	if err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if len(loaded) != 1 || loaded[0] != path {
		t.Fatalf("loaded = %v, want only the file that exists", loaded)
	}

	want := map[string]string{
		"DOTENV_PLAIN": "plain", "DOTENV_QUOTED": "with spaces", "DOTENV_SINGLE": "single",
		"DOTENV_EXPORTED": "yes", "DOTENV_EQUALS": "a=b=c", "DOTENV_EMPTY": "",
		"DOTENV_ALREADY_SET": "from-environment", // real environment wins over the file
	}
	for key, value := range want {
		if got := os.Getenv(key); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
}

func TestLoadDotEnvRejectsMalformedLines(t *testing.T) {
	path := writeFile(t, t.TempDir(), ".env", "GOOD=1\nthis line has no equals sign\n")
	_, err := LoadDotEnv(path)
	if err == nil || !strings.Contains(err.Error(), ":2:") {
		t.Fatalf("error = %v, want it to point at line 2", err)
	}
}
