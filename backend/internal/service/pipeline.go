package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/ai"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/github"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/lang"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/skills"
)

// Error codes stored on failed reviews and returned by the API.
const (
	codePRNotFound      = "pr_not_found"
	codeRateLimited     = "github_rate_limited"
	codeTimeout         = "timeout"
	codeInterrupted     = "interrupted"
	codeAnalysisFailed  = "analysis_failed"
	codeInternal        = "internal_error"
	downloadConcurrency = 5
	recordTimeout       = 10 * time.Second
)

// analysisError marks a failure inside an analyzer.
type analysisError struct{ err error }

func (e *analysisError) Error() string { return "analysis failed: " + e.err.Error() }
func (e *analysisError) Unwrap() error { return e.err }

// run processes one review to completion. It always leaves the review in a
// terminal state, whatever goes wrong, so a panic never kills a worker.
func (s *Service) run(j job) {
	id, ref := j.id, j.ref
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic while processing review", "review", id, "panic", r)
			s.fail(id, codeInternal, "The review failed unexpectedly.")
		}
	}()

	// During a forced shutdown, queued reviews fail fast instead of starting.
	if s.baseCtx.Err() != nil {
		s.fail(id, codeInterrupted, "The server shut down before this review started.")
		return
	}

	ctx, cancel := context.WithTimeout(s.baseCtx, s.cfg.ReviewTimeout)
	defer cancel()
	if j.token != "" {
		ctx = github.WithRequestToken(ctx, j.token)
	}
	ctx = review.WithViewer(ctx, j.viewer)

	findings, message, err := s.execute(ctx, id, ref)
	if err != nil {
		code, msg := classify(err)
		if code == codeInternal {
			slog.Error("review failed", "review", id, "error", err)
		} else {
			slog.Info("review failed", "review", id, "code", code, "error", err)
		}
		s.fail(id, code, msg)
		return
	}
	s.complete(id, findings, message)
}

// execute runs the pipeline stages and returns the findings and a closing message.
func (s *Service) execute(ctx context.Context, id string, ref github.PullRequestRef) ([]review.Finding, string, error) {
	if err := s.transition(ctx, id, review.StatusFetchingPR, "Fetching the pull request and its diff from GitHub"); err != nil {
		return nil, "", err
	}

	pr, err := s.source.FetchPullRequest(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	if err := s.recorder.SavePullRequest(ctx, id, toDomain(pr)); err != nil {
		return nil, "", fmt.Errorf("save pull request: %w", err)
	}

	files, err := s.source.ListFiles(ctx, ref)
	if err != nil {
		return nil, "", err
	}

	reviewable := reviewableFiles(files)
	selected := s.selectSkills(reviewable)
	if len(selected) == 0 {
		return nil, "No changed files in a supported language, so there was nothing to check", nil
	}

	analyzed := reviewable
	skipped := 0
	if len(analyzed) > s.cfg.MaxFilesToAnalyze {
		skipped = len(analyzed) - s.cfg.MaxFilesToAnalyze
		analyzed = analyzed[:s.cfg.MaxFilesToAnalyze]
	}

	inputs, err := s.download(ctx, ref, pr.HeadSHA, analyzed)
	if err != nil {
		return nil, "", err
	}

	msg := fmt.Sprintf("Running static analysis on %d changed %s", len(inputs), plural(len(inputs), "file", "files"))
	if err := s.transition(ctx, id, review.StatusAnalyzing, msg); err != nil {
		return nil, "", err
	}

	var raw []review.Finding
	for _, a := range s.cfg.Analyzers {
		found, err := a.Analyze(ctx, review.AnalysisInput{Files: inputs})
		if err != nil {
			return nil, "", &analysisError{err: err}
		}
		raw = append(raw, found...)
	}
	findings := s.normalize(raw, selected)

	if s.aiProvider != nil && hasAICheck(selected) {
		if err := s.transition(ctx, id, review.StatusReviewing, "Reviewing in context"); err != nil {
			return nil, "", err
		}
		related := s.relatedCode(ctx, id, ref, pr, inputs)
		aiRaw := s.runAI(ctx, selected, inputs, related)
		findings = append(findings, s.normalizeAI(aiRaw, selected)...)
	}
	sortFindings(findings)

	closing := "Review complete"
	if skipped > 0 {
		closing = fmt.Sprintf("Review complete. Analyzed %d of %d source files; the rest were skipped", len(analyzed), len(reviewable))
	}
	return findings, closing, nil
}

// hasAICheck reports whether any selected skill defines a kind: ai check.
func hasAICheck(selected map[string]skills.Skill) bool {
	for _, sk := range selected {
		for _, c := range sk.Checks {
			if c.Kind == skills.KindAI {
				return true
			}
		}
	}
	return false
}

// runAI evaluates every selected skill's kind: ai checks against the files
// that skill applies to. A provider error is logged and treated as "this
// skill's AI pass found nothing" rather than failing the review: the
// deterministic findings are already complete and valuable on their own, and
// one flaky or rate-limited call should not throw them away.
func (s *Service) runAI(ctx context.Context, selected map[string]skills.Skill, files []review.ChangedFile, related []ai.ContextSnippet) []review.Finding {
	var out []review.Finding
	for _, sk := range selected {
		var checks []skills.Check
		for _, c := range sk.Checks {
			if c.Kind == skills.KindAI {
				checks = append(checks, c)
			}
		}
		if len(checks) == 0 {
			continue
		}

		var skillFiles []review.ChangedFile
		for _, f := range files {
			if sk.AppliesTo(f.Language) {
				skillFiles = append(skillFiles, f)
			}
		}
		if len(skillFiles) == 0 {
			continue
		}

		found, err := s.aiProvider.Review(ctx, ai.ReviewInput{Skill: sk, Checks: checks, Files: skillFiles, Context: related})
		if err != nil {
			slog.Warn("ai review pass failed; continuing with deterministic findings only", "skill", sk.ID, "error", err)
			continue
		}
		for _, f := range found {
			out = append(out, review.Finding{
				SkillID: sk.ID, CheckID: f.CheckID, File: f.File, Line: f.Line, EndLine: f.EndLine,
				Evidence: f.Evidence, Found: f.Found, Why: f.Why, Improvement: f.Improvement,
				Confidence: f.Confidence,
			})
		}
	}
	return out
}

// reviewableFiles keeps files in a language we review that still exist after the PR.
func reviewableFiles(files []github.ChangedFile) []github.ChangedFile {
	var out []github.ChangedFile
	for _, f := range files {
		if f.Status != "removed" && lang.Detect(f.Path) != "" {
			out = append(out, f)
		}
	}
	return out
}

// selectSkills returns the skills that cover at least one changed language.
func (s *Service) selectSkills(files []github.ChangedFile) map[string]skills.Skill {
	present := make(map[string]bool)
	for _, f := range files {
		present[lang.Detect(f.Path)] = true
	}

	selected := make(map[string]skills.Skill)
	for _, sk := range s.cfg.Skills {
		for language := range present {
			if sk.AppliesTo(language) {
				selected[sk.ID] = sk
				break
			}
		}
	}
	return selected
}

// download fetches file contents at the PR's head commit, a few at a time.
// Files over the size cap are skipped; any other failure fails the review.
func (s *Service) download(ctx context.Context, ref github.PullRequestRef, sha string, files []github.ChangedFile) ([]review.ChangedFile, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]*review.ChangedFile, len(files))
	var (
		wg       sync.WaitGroup
		once     sync.Once
		firstErr error
		slots    = make(chan struct{}, downloadConcurrency)
	)
	for i, f := range files {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}

			content, err := s.source.FileContent(ctx, ref, f.Path, sha)
			switch {
			case errors.Is(err, github.ErrTooLarge):
				slog.Info("skipping oversized file", "path", f.Path)
			case err != nil:
				once.Do(func() { firstErr = err; cancel() })
			default:
				results[i] = &review.ChangedFile{Path: f.Path, Language: lang.Detect(f.Path), Patch: f.Patch, Content: string(content)}
			}
		}()
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var out []review.ChangedFile
	for _, r := range results {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, nil
}

// normalize turns raw analyzer output into stored findings. Analyzers only say
// what they saw; the skill definition decides severity, source, confidence,
// titles and lesson. Findings that do not match a selected skill's deterministic
// check, or that are invalid, are dropped with a warning: they indicate a bug in
// an analyzer, which should not break the whole review.
func (s *Service) normalize(raw []review.Finding, selected map[string]skills.Skill) []review.Finding {
	var out []review.Finding
	for _, f := range raw {
		sk, ok := selected[f.SkillID]
		if !ok {
			slog.Warn("dropping finding for a skill that does not apply", "skill", f.SkillID, "check", f.CheckID)
			continue
		}
		check, ok := sk.Check(f.CheckID)
		if !ok || check.Kind != skills.KindDeterministic {
			slog.Warn("dropping finding for an unknown or non-deterministic check", "skill", f.SkillID, "check", f.CheckID)
			continue
		}

		f.Source = review.SourceDeterministic
		f.Confidence = 1
		f.Severity = check.Severity
		f.Lesson = check.Lesson
		f = s.enrich(f)
		if err := f.Validate(); err != nil {
			slog.Warn("dropping invalid finding", "error", err)
			continue
		}
		out = append(out, f)
	}
	return out
}

// normalizeAI is normalize's counterpart for the AI pass: it requires a
// kind: ai check (never a deterministic one — an analyzer output claiming an
// AI check, or vice versa, indicates a bug and is dropped) and keeps the
// model's own confidence instead of forcing 1.0.
func (s *Service) normalizeAI(raw []review.Finding, selected map[string]skills.Skill) []review.Finding {
	var out []review.Finding
	for _, f := range raw {
		sk, ok := selected[f.SkillID]
		if !ok {
			slog.Warn("dropping ai finding for a skill that does not apply", "skill", f.SkillID, "check", f.CheckID)
			continue
		}
		check, ok := sk.Check(f.CheckID)
		if !ok || check.Kind != skills.KindAI {
			slog.Warn("dropping ai finding for an unknown or non-ai check", "skill", f.SkillID, "check", f.CheckID)
			continue
		}

		f.Source = review.SourceAI
		f.Severity = check.Severity
		f.Lesson = check.Lesson
		f = s.enrich(f)
		if err := f.Validate(); err != nil {
			slog.Warn("dropping invalid ai finding", "error", err)
			continue
		}
		out = append(out, f)
	}
	return out
}

// sortFindings orders a review's combined findings: most severe first, then
// by file and line, stably so equal-ranked findings keep a predictable order.
func sortFindings(findings []review.Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
			return ra < rb
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
}

func severityRank(s review.Severity) int {
	switch s {
	case review.SeverityCritical:
		return 0
	case review.SeverityHigh:
		return 1
	case review.SeverityMedium:
		return 2
	default:
		return 3
	}
}

// transition records a new status and tells live subscribers.
func (s *Service) transition(ctx context.Context, id string, status review.Status, message string) error {
	if err := s.recorder.SetStatus(ctx, id, status, message); err != nil {
		return fmt.Errorf("set status %s: %w", status, err)
	}
	s.broker.Publish(id, review.Event{Status: status, Message: message, At: time.Now()})
	return nil
}

// complete saves the findings and marks the review done. Saving must not be
// cut short by the review's own timeout or by shutdown, so it has its own budget.
func (s *Service) complete(id string, findings []review.Finding, message string) {
	ctx, cancel := recordContext(s.baseCtx)
	defer cancel()

	if err := s.recorder.CompleteReview(ctx, id, findings, message); err != nil {
		slog.Error("could not save completed review", "review", id, "error", err)
		s.fail(id, codeInternal, "The review finished but its results could not be saved.")
		return
	}
	s.broker.Publish(id, review.Event{Status: review.StatusCompleted, Message: message, At: time.Now()})
}

// fail marks the review failed and tells live subscribers.
func (s *Service) fail(id, code, message string) {
	ctx, cancel := recordContext(s.baseCtx)
	defer cancel()

	if err := s.recorder.FailReview(ctx, id, code, message); err != nil {
		if !errors.Is(err, review.ErrReviewFinished) {
			slog.Error("could not record failed review", "review", id, "error", err)
		}
		return
	}
	s.broker.Publish(id, review.Event{Status: review.StatusFailed, Message: message, At: time.Now()})
}

// recordContext gives database writes their own short deadline, detached from
// the review's cancellation so a timeout or shutdown can still be recorded.
func recordContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), recordTimeout)
}

// classify maps an error to the stable code and the user-facing message stored
// on the review. Internal details stay in the logs.
func classify(err error) (code, message string) {
	var analysis *analysisError
	switch {
	case errors.Is(err, github.ErrNotFound):
		return codePRNotFound, "Pull request not found, or it is in a private repository."
	case errors.Is(err, github.ErrRateLimited):
		return codeRateLimited, "GitHub's rate limit was reached. Please try again in a few minutes."
	case errors.Is(err, context.DeadlineExceeded):
		return codeTimeout, "The review took too long and was stopped."
	case errors.Is(err, context.Canceled):
		return codeInterrupted, "The server shut down while this review was running. Please start it again."
	case errors.As(err, &analysis):
		return codeAnalysisFailed, "The code analysis failed unexpectedly."
	default:
		return codeInternal, "The review failed unexpectedly."
	}
}

func toDomain(pr github.PullRequest) review.PullRequest {
	return review.PullRequest{
		Owner: pr.Ref.Owner, Repo: pr.Ref.Repo, Number: pr.Ref.Number,
		Title: pr.Title, Author: pr.Author, URL: pr.URL, State: pr.State, HeadSHA: pr.HeadSHA,
		FilesChanged: pr.FilesChanged, Additions: pr.Additions, Deletions: pr.Deletions,
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
