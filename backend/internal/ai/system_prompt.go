package ai

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// systemPromptFile is looked up directly inside the skills directory
// (alongside the per-skill folders, not inside one), the same place
// skill.yaml files live, so the reviewer's voice and rules are a plain
// editable file like everything else the product teaches from — no Go code
// to touch, no rebuild needed beyond restarting the server.
const systemPromptFile = "system_prompt.md"

// defaultSystemPrompt is used until a system_prompt.md exists, and is what
// skills/system_prompt.md starts out containing — edit that file to change
// the reviewer's behavior.
const defaultSystemPrompt = `You are a code reviewer. You review pull requests against a specific
software-engineering skill and report only what the evidence in the diff supports.

Rules:
- Report a finding only when you can point to the exact lines that show the problem. Evidence,
  not opinion: if you are not confident there is a real issue, do not report it.
- One finding per genuine issue. Do not repeat the same issue across nearby lines.
- "found" states what you observed, in one or two sentences, plainly.
- "why" explains the concrete consequence — what breaks, what becomes harder, what risk this
  creates — not a restatement of the principle.
- "improvement" is a specific, actionable change, not general advice.
- confidence is your own honest estimate (0 to 1) that this is a real, worthwhile finding.
- Only report issues covered by the checks listed below. Do not invent new checks.
- If you find nothing worth reporting, call the tool with an empty findings list.`

// LoadSystemPrompt reads system_prompt.md from fsys (typically the skills
// directory). A missing, empty, or whitespace-only file is not an error — it
// just means defaultSystemPrompt is used, so the product works unedited.
func LoadSystemPrompt(fsys fs.FS) (string, error) {
	data, err := fs.ReadFile(fsys, systemPromptFile)
	if errors.Is(err, fs.ErrNotExist) {
		return defaultSystemPrompt, nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", systemPromptFile, err)
	}

	text := strings.TrimSpace(string(data))
	if text == "" {
		return defaultSystemPrompt, nil
	}
	return text, nil
}
