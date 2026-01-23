package syntax

import (
	"strings"
	"testing"
)

// TestLanguageDetection tests file extension to language mapping
func TestLanguageDetection(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		language string
	}{
		{"Go file", "main.go", "go"},
		{"Python file", "script.py", "python"},
		{"JavaScript file", "app.js", "javascript"},
		{"TypeScript file", "app.ts", "typescript"},
		{"TSX file", "component.tsx", "tsx"},
		{"JSX file", "component.jsx", "jsx"},
		{"Rust file", "lib.rs", "rust"},
		{"Java file", "Main.java", "java"},
		{"C file", "main.c", "c"},
		{"C++ file", "main.cpp", "cpp"},
		{"C header", "header.h", "c"},
		{"Ruby file", "app.rb", "ruby"},
		{"PHP file", "index.php", "php"},
		{"Swift file", "app.swift", "swift"},
		{"Kotlin file", "Main.kt", "kotlin"},
		{"Scala file", "App.scala", "scala"},
		{"Shell script", "script.sh", "bash"},
		{"Bash script", "script.bash", "bash"},
		{"Zsh script", "script.zsh", "zsh"},
		{"YAML file", "config.yaml", "yaml"},
		{"YML file", "config.yml", "yaml"},
		{"JSON file", "data.json", "json"},
		{"XML file", "data.xml", "xml"},
		{"HTML file", "index.html", "html"},
		{"CSS file", "style.css", "css"},
		{"SCSS file", "style.scss", "scss"},
		{"Markdown file", "README.md", "markdown"},
		{"SQL file", "query.sql", "sql"},
		{"Dockerfile", "Dockerfile", "dockerfile"},
		{"Makefile", "Makefile", "makefile"},
		{"TOML file", "config.toml", "toml"},
		{"Unknown file", "file.xyz", ""},
	}

	langMap := map[string]string{
		".go":    "go",
		".py":    "python",
		".js":    "javascript",
		".ts":    "typescript",
		".tsx":   "tsx",
		".jsx":   "jsx",
		".rs":    "rust",
		".java":  "java",
		".c":     "c",
		".cpp":   "cpp",
		".h":     "c",
		".rb":    "ruby",
		".php":   "php",
		".swift": "swift",
		".kt":    "kotlin",
		".scala": "scala",
		".sh":    "bash",
		".bash":  "bash",
		".zsh":   "zsh",
		".yaml":  "yaml",
		".yml":   "yaml",
		".json":  "json",
		".xml":   "xml",
		".html":  "html",
		".css":   "css",
		".scss":  "scss",
		".md":    "markdown",
		".sql":   "sql",
		".toml":  "toml",
	}

	specialFiles := map[string]string{
		"Dockerfile": "dockerfile",
		"Makefile":   "makefile",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var detected string

			// Check special files first
			if lang, ok := specialFiles[tt.filename]; ok {
				detected = lang
			} else {
				// Extract extension
				dotIdx := strings.LastIndex(tt.filename, ".")
				if dotIdx != -1 {
					ext := tt.filename[dotIdx:]
					detected = langMap[ext]
				}
			}

			if detected != tt.language {
				t.Errorf("language for %q = %q, want %q", tt.filename, detected, tt.language)
			}
		})
	}
}

// TestCommentStyleDetection tests detection of comment styles by language
func TestCommentStyleDetection(t *testing.T) {
	type commentStyle struct {
		singleLine string
		multiStart string
		multiEnd   string
	}

	tests := []struct {
		name     string
		language string
		style    commentStyle
	}{
		{
			name:     "C-style",
			language: "go",
			style:    commentStyle{"//", "/*", "*/"},
		},
		{
			name:     "Python",
			language: "python",
			style:    commentStyle{"#", `"""`, `"""`},
		},
		{
			name:     "Ruby",
			language: "ruby",
			style:    commentStyle{"#", "=begin", "=end"},
		},
		{
			name:     "HTML",
			language: "html",
			style:    commentStyle{"", "<!--", "-->"},
		},
		{
			name:     "SQL",
			language: "sql",
			style:    commentStyle{"--", "/*", "*/"},
		},
		{
			name:     "Bash",
			language: "bash",
			style:    commentStyle{"#", "", ""},
		},
	}

	commentStyles := map[string]commentStyle{
		"go":         {"//", "/*", "*/"},
		"javascript": {"//", "/*", "*/"},
		"typescript": {"//", "/*", "*/"},
		"java":       {"//", "/*", "*/"},
		"c":          {"//", "/*", "*/"},
		"cpp":        {"//", "/*", "*/"},
		"rust":       {"//", "/*", "*/"},
		"python":     {"#", `"""`, `"""`},
		"ruby":       {"#", "=begin", "=end"},
		"html":       {"", "<!--", "-->"},
		"xml":        {"", "<!--", "-->"},
		"sql":        {"--", "/*", "*/"},
		"bash":       {"#", "", ""},
		"yaml":       {"#", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			style, ok := commentStyles[tt.language]
			if !ok {
				t.Skipf("no comment style defined for %q", tt.language)
			}

			if style.singleLine != tt.style.singleLine {
				t.Errorf("singleLine for %q = %q, want %q", tt.language, style.singleLine, tt.style.singleLine)
			}
			if style.multiStart != tt.style.multiStart {
				t.Errorf("multiStart for %q = %q, want %q", tt.language, style.multiStart, tt.style.multiStart)
			}
			if style.multiEnd != tt.style.multiEnd {
				t.Errorf("multiEnd for %q = %q, want %q", tt.language, style.multiEnd, tt.style.multiEnd)
			}
		})
	}
}

// TestCodeBlockIdentification tests identification of code blocks
func TestCodeBlockIdentification(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		language   string
		blockType  string
		blockCount int
	}{
		{
			name:       "Go function",
			code:       "func main() {\n\tfmt.Println(\"hello\")\n}",
			language:   "go",
			blockType:  "function",
			blockCount: 1,
		},
		{
			name:       "Go struct",
			code:       "type User struct {\n\tName string\n\tAge int\n}",
			language:   "go",
			blockType:  "struct",
			blockCount: 1,
		},
		{
			name:       "JavaScript function",
			code:       "function hello() {\n\treturn 'world';\n}",
			language:   "javascript",
			blockType:  "function",
			blockCount: 1,
		},
		{
			name:       "JavaScript class",
			code:       "class User {\n\tconstructor(name) {\n\t\tthis.name = name;\n\t}\n}",
			language:   "javascript",
			blockType:  "class",
			blockCount: 1,
		},
		{
			name:       "Python function",
			code:       "def hello():\n    return 'world'",
			language:   "python",
			blockType:  "function",
			blockCount: 1,
		},
		{
			name:       "Python class",
			code:       "class User:\n    def __init__(self, name):\n        self.name = name",
			language:   "python",
			blockType:  "class",
			blockCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var count int

			switch tt.blockType {
			case "function":
				switch tt.language {
				case "go":
					count = strings.Count(tt.code, "func ")
				case "javascript", "typescript":
					count = strings.Count(tt.code, "function ")
				case "python":
					count = strings.Count(tt.code, "def ")
				}
			case "struct":
				count = strings.Count(tt.code, "type ") + strings.Count(tt.code, "struct ")
				if count > 1 {
					count = 1 // "type X struct" counts as one struct
				}
			case "class":
				count = strings.Count(tt.code, "class ")
			}

			if count != tt.blockCount {
				t.Errorf("block count for %s = %d, want %d", tt.blockType, count, tt.blockCount)
			}
		})
	}
}

// TestIndentationDetection tests detection of indentation style
func TestIndentationDetection(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		useTabs    bool
		spaceCount int
	}{
		{
			name:       "tabs",
			code:       "func main() {\n\tfmt.Println()\n}",
			useTabs:    true,
			spaceCount: 0,
		},
		{
			name:       "2 spaces",
			code:       "function main() {\n  console.log();\n}",
			useTabs:    false,
			spaceCount: 2,
		},
		{
			name:       "4 spaces",
			code:       "def main():\n    print('hello')",
			useTabs:    false,
			spaceCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.code, "\n")
			var useTabs bool
			var spaceCount int

			for _, line := range lines {
				if len(line) == 0 {
					continue
				}
				if line[0] == '\t' {
					useTabs = true
					break
				}
				if line[0] == ' ' {
					// Count leading spaces
					for i, c := range line {
						if c != ' ' {
							spaceCount = i
							break
						}
					}
					break
				}
			}

			if useTabs != tt.useTabs {
				t.Errorf("useTabs = %v, want %v", useTabs, tt.useTabs)
			}
			if !useTabs && spaceCount != tt.spaceCount {
				t.Errorf("spaceCount = %d, want %d", spaceCount, tt.spaceCount)
			}
		})
	}
}

// TestLineCounting tests line counting utilities
func TestLineCounting(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		lineCount int
		nonEmpty  int
	}{
		{
			name:      "single line",
			code:      "hello",
			lineCount: 1,
			nonEmpty:  1,
		},
		{
			name:      "multiple lines",
			code:      "line1\nline2\nline3",
			lineCount: 3,
			nonEmpty:  3,
		},
		{
			name:      "with empty lines",
			code:      "line1\n\nline3\n\nline5",
			lineCount: 5,
			nonEmpty:  3,
		},
		{
			name:      "trailing newline",
			code:      "line1\nline2\n",
			lineCount: 3,
			nonEmpty:  2,
		},
		{
			name:      "empty string",
			code:      "",
			lineCount: 1,
			nonEmpty:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.code, "\n")
			lineCount := len(lines)

			nonEmpty := 0
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					nonEmpty++
				}
			}

			if lineCount != tt.lineCount {
				t.Errorf("lineCount = %d, want %d", lineCount, tt.lineCount)
			}
			if nonEmpty != tt.nonEmpty {
				t.Errorf("nonEmpty = %d, want %d", nonEmpty, tt.nonEmpty)
			}
		})
	}
}

// TestStringLiteralDetection tests detection of string literals
func TestStringLiteralDetection(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		hasString   bool
		stringCount int
	}{
		{
			name:        "double quoted",
			code:        `fmt.Println("hello")`,
			hasString:   true,
			stringCount: 1,
		},
		{
			name:        "single quoted",
			code:        `console.log('hello')`,
			hasString:   true,
			stringCount: 1,
		},
		{
			name:        "backtick",
			code:        "const s = `template`",
			hasString:   true,
			stringCount: 1,
		},
		{
			name:        "multiple strings",
			code:        `fmt.Printf("%s %s", "hello", "world")`,
			hasString:   true,
			stringCount: 3,
		},
		{
			name:        "no strings",
			code:        "x := 42",
			hasString:   false,
			stringCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasDouble := strings.Contains(tt.code, `"`)
			hasSingle := strings.Contains(tt.code, "'")
			hasBacktick := strings.Contains(tt.code, "`")

			hasString := hasDouble || hasSingle || hasBacktick

			if hasString != tt.hasString {
				t.Errorf("hasString = %v, want %v", hasString, tt.hasString)
			}
		})
	}
}

// TestBraceBalancing tests brace/bracket/paren balancing
func TestBraceBalancing(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		isBalanced bool
	}{
		{
			name:       "balanced braces",
			code:       "func main() { if true { } }",
			isBalanced: true,
		},
		{
			name:       "balanced parens",
			code:       "func(a, b, (c + d))",
			isBalanced: true,
		},
		{
			name:       "balanced brackets",
			code:       "arr[0][1][2]",
			isBalanced: true,
		},
		{
			name:       "unbalanced braces",
			code:       "func main() { if true { }",
			isBalanced: false,
		},
		{
			name:       "unbalanced parens",
			code:       "func(a, b, (c + d)",
			isBalanced: false,
		},
		{
			name:       "mixed balanced",
			code:       "arr[func() { return (1 + 2) }]",
			isBalanced: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			braces := 0
			parens := 0
			brackets := 0

			for _, c := range tt.code {
				switch c {
				case '{':
					braces++
				case '}':
					braces--
				case '(':
					parens++
				case ')':
					parens--
				case '[':
					brackets++
				case ']':
					brackets--
				}
			}

			isBalanced := braces == 0 && parens == 0 && brackets == 0
			if isBalanced != tt.isBalanced {
				t.Errorf("isBalanced = %v, want %v (braces=%d, parens=%d, brackets=%d)",
					isBalanced, tt.isBalanced, braces, parens, brackets)
			}
		})
	}
}
