package ai

const reportToolName = "report_findings"

const maxTokens = 8000

var findingProperties = map[string]any{
	"checkId":     map[string]any{"type": "string", "description": "The checkId this finding belongs to, exactly as given above."},
	"file":        map[string]any{"type": "string"},
	"line":        map[string]any{"type": "integer", "description": "1-based line number, or 0 if the finding applies to the whole file."},
	"endLine":     map[string]any{"type": "integer", "description": "0 for a single line or the whole file."},
	"evidence":    map[string]any{"type": "string", "description": "The exact code that shows the problem."},
	"found":       map[string]any{"type": "string"},
	"why":         map[string]any{"type": "string"},
	"improvement": map[string]any{"type": "string"},
	"confidence":  map[string]any{"type": "number", "description": "Your confidence, 0 to 1, that this is a real, worthwhile finding."},
}

var findingRequired = []string{"checkId", "file", "line", "endLine", "evidence", "found", "why", "improvement", "confidence"}

func findingsObjectSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"findings": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"properties":           findingProperties,
					"required":             findingRequired,
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"findings"},
		"additionalProperties": false,
	}
}

type toolInput struct {
	Findings []struct {
		CheckID     string  `json:"checkId"`
		File        string  `json:"file"`
		Line        int     `json:"line"`
		EndLine     int     `json:"endLine"`
		Evidence    string  `json:"evidence"`
		Found       string  `json:"found"`
		Why         string  `json:"why"`
		Improvement string  `json:"improvement"`
		Confidence  float64 `json:"confidence"`
	} `json:"findings"`
}

func toFindings(parsed toolInput) []Finding {
	out := make([]Finding, 0, len(parsed.Findings))
	for _, f := range parsed.Findings {
		confidence := f.Confidence
		if confidence < 0 {
			confidence = 0
		} else if confidence > 1 {
			confidence = 1
		}
		out = append(out, Finding{
			CheckID: f.CheckID, File: f.File, Line: f.Line, EndLine: f.EndLine,
			Evidence: f.Evidence, Found: f.Found, Why: f.Why, Improvement: f.Improvement,
			Confidence: confidence,
		})
	}
	return out
}
