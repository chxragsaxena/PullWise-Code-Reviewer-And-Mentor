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
