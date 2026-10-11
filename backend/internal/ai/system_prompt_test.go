package ai

import (
	"testing"
	"testing/fstest"
)

func TestLoadSystemPromptFallsBackWhenMissing(t *testing.T) {
	got, err := LoadSystemPrompt(fstest.MapFS{"go/skill.yaml": {Data: []byte("x")}})
	if err != nil || got != defaultSystemPrompt {
		t.Fatalf("got %q, %v; want the default", got, err)
	}
}

func TestLoadSystemPromptUsesTheFileWhenPresent(t *testing.T) {
	custom := "Be an unreasonably harsh reviewer."
	got, err := LoadSystemPrompt(fstest.MapFS{"system_prompt.md": {Data: []byte(custom)}})
	if err != nil || got != custom {
		t.Fatalf("got %q, %v; want %q", got, err, custom)
	}
}

func TestLoadSystemPromptTrimsWhitespace(t *testing.T) {
	got, err := LoadSystemPrompt(fstest.MapFS{"system_prompt.md": {Data: []byte("\n  Be concise.  \n\n")}})
	if err != nil || got != "Be concise." {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestLoadSystemPromptFallsBackWhenFileIsBlank(t *testing.T) {
	got, err := LoadSystemPrompt(fstest.MapFS{"system_prompt.md": {Data: []byte("   \n\t\n")}})
	if err != nil || got != defaultSystemPrompt {
		t.Fatalf("got %q, %v; want the default for a blank file", got, err)
	}
}
