// Package lang detects the programming language of a file from its path.
package lang

import "path"

// Language ids match the "languages" listed in skill.yaml files.
const (
	Go         = "go"
	TypeScript = "typescript"
	Python     = "python"
)

// byExtension covers source code only; data and markup are not reviewed.
var byExtension = map[string]string{
	".go":  Go,
	".ts":  TypeScript,
	".tsx": TypeScript,
	".mts": TypeScript,
	".cts": TypeScript,

	".js":    "javascript",
	".jsx":   "javascript",
	".mjs":   "javascript",
	".cjs":   "javascript",
	".py":    Python,
	".java":  "java",
	".kt":    "kotlin",
	".scala": "scala",
	".rb":    "ruby",
	".php":   "php",
	".rs":    "rust",
	".c":     "c",
	".cc":    "cpp",
	".cpp":   "cpp",
	".cxx":   "cpp",
	".hpp":   "cpp",
	".cs":    "csharp",
	".swift": "swift",
}

// Detect returns the language of a file, or "" if it is not one we review.
func Detect(filePath string) string {
	return byExtension[path.Ext(filePath)]
}
