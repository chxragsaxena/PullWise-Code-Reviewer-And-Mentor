package httpapi

import (
	"time"

	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/review"
	"github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor/internal/service"
)

// The types below are the JSON shapes of docs/api.md. Keep them in sync.

type pullRequestDTO struct {
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	URL          string `json:"url"`
	HeadSHA      string `json:"headSha"`
	FilesChanged int    `json:"filesChanged"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
}

type countsDTO struct {
	Critical   int `json:"critical"`
	High       int `json:"high"`
	Medium     int `json:"medium"`
	Suggestion int `json:"suggestion"`
}

type reviewSummaryDTO struct {
	ID          string         `json:"id"`
	Status      string         `json:"status"`
	PullRequest pullRequestDTO `json:"pullRequest"`
	Counts      countsDTO      `json:"counts"`
	CreatedAt   string         `json:"createdAt"`
	CompletedAt *string        `json:"completedAt"`
}

type reviewDTO struct {
	reviewSummaryDTO
	Error    *errorDetail `json:"error"`
	Findings []findingDTO `json:"findings"`
}

type findingDTO struct {
	ID          string  `json:"id"`
	SkillID     string  `json:"skillId"`
	SkillName   string  `json:"skillName"`
	CheckID     string  `json:"checkId"`
	CheckTitle  string  `json:"checkTitle"`
	Source      string  `json:"source"`
	Severity    string  `json:"severity"`
	Confidence  float64 `json:"confidence"`
	File        string  `json:"file"`
	Line        *int    `json:"line"`
	EndLine     *int    `json:"endLine"`
	Evidence    string  `json:"evidence"`
	Found       string  `json:"found"`
	Why         string  `json:"why"`
	Improvement string  `json:"improvement"`
	LessonID    *string `json:"lessonId"`
}

type lessonDTO struct {
	ID       string `json:"id"`
	Markdown string `json:"markdown"`
}

type findingDetailDTO struct {
	findingDTO
	Lesson *lessonDTO `json:"lesson"`
}

type eventDTO struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	At      string `json:"at"`
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func toSummaryDTO(r review.Review) reviewSummaryDTO {
	dto := reviewSummaryDTO{
		ID:     r.ID,
		Status: string(r.Status),
		PullRequest: pullRequestDTO{
			Owner: r.PullRequest.Owner, Repo: r.PullRequest.Repo, Number: r.PullRequest.Number,
			Title: r.PullRequest.Title, Author: r.PullRequest.Author, URL: r.PullRequest.URL,
			HeadSHA: r.PullRequest.HeadSHA, FilesChanged: r.PullRequest.FilesChanged,
			Additions: r.PullRequest.Additions, Deletions: r.PullRequest.Deletions,
		},
		Counts: countsDTO{
			Critical: r.Counts.Critical, High: r.Counts.High, Medium: r.Counts.Medium, Suggestion: r.Counts.Suggestion,
		},
		CreatedAt: formatTime(r.CreatedAt),
	}
	if r.CompletedAt != nil {
		s := formatTime(*r.CompletedAt)
		dto.CompletedAt = &s
	}
	return dto
}

func toReviewDTO(r review.Review) reviewDTO {
	dto := reviewDTO{reviewSummaryDTO: toSummaryDTO(r), Findings: make([]findingDTO, 0, len(r.Findings))}
	if r.Error != nil {
		dto.Error = &errorDetail{Code: r.Error.Code, Message: r.Error.Message}
	}
	for _, f := range r.Findings {
		dto.Findings = append(dto.Findings, toFindingDTO(f))
	}
	return dto
}

func toFindingDTO(f review.Finding) findingDTO {
	dto := findingDTO{
		ID: f.ID, SkillID: f.SkillID, SkillName: f.SkillName, CheckID: f.CheckID, CheckTitle: f.CheckTitle,
		Source: string(f.Source), Severity: string(f.Severity), Confidence: f.Confidence,
		File: f.File, Evidence: f.Evidence, Found: f.Found, Why: f.Why, Improvement: f.Improvement,
	}
	if f.Line > 0 {
		dto.Line = &f.Line
	}
	if f.EndLine > 0 {
		dto.EndLine = &f.EndLine
	}
	if f.Lesson != "" {
		dto.LessonID = &f.Lesson
	}
	return dto
}

func toFindingDetailDTO(d service.FindingDetail) findingDetailDTO {
	dto := findingDetailDTO{findingDTO: toFindingDTO(d.Finding)}
	if d.Lesson != nil {
		dto.Lesson = &lessonDTO{ID: d.Lesson.ID, Markdown: d.Lesson.Markdown}
	}
	return dto
}

func toEventDTO(e review.Event) eventDTO {
	return eventDTO{Status: string(e.Status), Message: e.Message, At: formatTime(e.At)}
}
