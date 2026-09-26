package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ADDR", "")
	t.Setenv("SKILLS_DIR", "")

	got := Load()
	if got.Addr != ":8080" || got.SkillsDir != "skills" {
		t.Fatalf("got %+v, want defaults", got)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("ADDR", ":9090")
	t.Setenv("SKILLS_DIR", "/opt/skills")

	got := Load()
	if got.Addr != ":9090" || got.SkillsDir != "/opt/skills" {
		t.Fatalf("got %+v, want values from the environment", got)
	}
}
