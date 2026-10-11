package ai

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
)

// maxPromptChars bounds how much diff text goes into one request. GitHub
// truncates very large patches on its own, but a PR with many changed files
// can still add up to more tokens than is sensible to send in one call; this
// is a coarse budget until a smarter per-file or chunked strategy is needed
// (see the tracker's known risks).
const maxPromptChars = 40_000

const systemPrompt = `You are a code reviewer. You review pull requests against a specific
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

// buildPrompt turns a skill's AI checks and a set of changed files into the
// user message sent to the model. It embeds each check's lesson text, so the
// model reviews using the same material the product teaches from.
func buildPrompt(input ReviewInput) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Skill: %s\n", input.Skill.Name)
	if input.Skill.Description != "" {
		fmt.Fprintf(&b, "%s\n", input.Skill.Description)
	}

	b.WriteString("\n# Checks to apply\n")
	for _, check := range input.Checks {
		fmt.Fprintf(&b, "\n## %s (checkId: %s)\n", check.Title, check.ID)
		if check.Lesson == "" {
			continue
		}
		lesson, err := input.Skill.Lesson(check.Lesson)
		if err != nil {
			slog.Warn("ai: lesson unavailable for prompt", "skill", input.Skill.ID, "check", check.ID, "error", err)
			continue
		}
		b.WriteString(lesson)
		b.WriteString("\n")
	}

	b.WriteString("\n# Changed files\n")
	writeFiles(&b, input.Files)

	return b.String()
}

// writeFiles appends each file's diff, stopping once the shared budget is
// spent so one large PR cannot blow out the request. Diffs are sent rather
// than full file content: they are what changed, they are already a fraction
// of the size, and they are what a human reviewer would actually read first.
func writeFiles(b *strings.Builder, files []review.ChangedFile) {
	budget := maxPromptChars
	for i, f := range files {
		if f.Patch == "" {
			continue
		}
		entry := fmt.Sprintf("\n## %s\n```diff\n%s\n```\n", f.Path, f.Patch)
		if len(entry) > budget {
			fmt.Fprintf(b, "\n(%d more changed file(s) omitted: the diff budget for this review was reached)\n", len(files)-i)
			return
		}
		b.WriteString(entry)
		budget -= len(entry)
	}
}
