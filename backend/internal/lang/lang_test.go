package lang

import "testing"

func TestDetect(t *testing.T) {
	tests := map[string]string{
		"main.go":                    Go,
		"internal/orders/service.go": Go,
		"web/App.tsx":                TypeScript,
		"lib/util.ts":                TypeScript,
		"lib/types.d.ts":             TypeScript,
		"README.md":                  "",
		"Makefile":                   "",
		"script.py":                  "python",
		"src/App.java":               "java",
		"web/app.jsx":                "javascript",
		"lib/foo.rs":                 "rust",
		"config.json":                "",
		"styles.css":                 "",
		"schema.sql":                 "",
		"main.go.bak":                "",
		"noextension":                "",
		"":                           "",
	}
	for file, want := range tests {
		if got := Detect(file); got != want {
			t.Errorf("Detect(%q) = %q, want %q", file, got, want)
		}
	}
}
