// Package lang detects the programming language of a file from its path.
package lang

import "path"

// Language ids match the "languages" listed in skill.yaml files.
const (
	Go         = "go"
	TypeScript = "typescript"
)

var byExtension = map[string]string{
	".go":  Go,
	".ts":  TypeScript,
	".tsx": TypeScript,
	".mts": TypeScript,
	".cts": TypeScript,
}

// Detect returns the language of a file, or "" if it is not one we review.
func Detect(filePath string) string {
	return byExtension[path.Ext(filePath)]
}
