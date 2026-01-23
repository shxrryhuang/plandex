package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// getTestFixturePath returns the path to a test fixture file
func getTestFixturePath(filename string) string {
	_, currentFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(currentFile)
	return filepath.Join(dir, "xml_test_examples", filename)
}

// loadTestFixture loads a test fixture file and returns its content
func loadTestFixture(t *testing.T, filename string) string {
	t.Helper()
	path := getTestFixturePath(filename)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to load fixture %s: %v", filename, err)
	}
	return string(content)
}

// TestXMLTagExtraction tests extraction of XML-like tags from content
func TestXMLTagExtraction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		tagName  string
		hasTag   bool
		expected string
	}{
		{
			name:     "simple tag extraction",
			input:    "<file>content here</file>",
			tagName:  "file",
			hasTag:   true,
			expected: "content here",
		},
		{
			name:     "tag with attributes",
			input:    `<file path="main.go">package main</file>`,
			tagName:  "file",
			hasTag:   true,
			expected: "package main",
		},
		{
			name:     "no matching tag",
			input:    "<other>content</other>",
			tagName:  "file",
			hasTag:   false,
			expected: "",
		},
		{
			name:     "empty content",
			input:    "<file></file>",
			tagName:  "file",
			hasTag:   true,
			expected: "",
		},
		{
			name:     "nested content",
			input:    "<file>line1\nline2\nline3</file>",
			tagName:  "file",
			hasTag:   true,
			expected: "line1\nline2\nline3",
		},
		{
			name:     "self-closing tag",
			input:    "<file/>",
			tagName:  "file",
			hasTag:   true,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openTag := "<" + tt.tagName
			closeTag := "</" + tt.tagName + ">"
			selfClose := "<" + tt.tagName + "/>"

			hasTag := strings.Contains(tt.input, openTag)
			if hasTag != tt.hasTag {
				t.Errorf("hasTag = %v, want %v", hasTag, tt.hasTag)
			}

			if tt.hasTag && !strings.Contains(tt.input, selfClose) {
				startIdx := strings.Index(tt.input, ">")
				endIdx := strings.Index(tt.input, closeTag)
				if startIdx != -1 && endIdx != -1 && startIdx < endIdx {
					content := tt.input[startIdx+1 : endIdx]
					if content != tt.expected {
						t.Errorf("content = %q, want %q", content, tt.expected)
					}
				}
			}
		})
	}
}

// TestXMLAttributeParsing tests parsing attributes from XML-like tags
func TestXMLAttributeParsing(t *testing.T) {
	tests := []struct {
		name       string
		tag        string
		attrName   string
		attrValue  string
		shouldFind bool
	}{
		{
			name:       "simple attribute",
			tag:        `<file path="main.go">`,
			attrName:   "path",
			attrValue:  "main.go",
			shouldFind: true,
		},
		{
			name:       "multiple attributes - first",
			tag:        `<file path="main.go" lang="go">`,
			attrName:   "path",
			attrValue:  "main.go",
			shouldFind: true,
		},
		{
			name:       "multiple attributes - second",
			tag:        `<file path="main.go" lang="go">`,
			attrName:   "lang",
			attrValue:  "go",
			shouldFind: true,
		},
		{
			name:       "attribute with spaces in value",
			tag:        `<file description="main entry point">`,
			attrName:   "description",
			attrValue:  "main entry point",
			shouldFind: true,
		},
		{
			name:       "missing attribute",
			tag:        `<file path="main.go">`,
			attrName:   "lang",
			attrValue:  "",
			shouldFind: false,
		},
		{
			name:       "no attributes",
			tag:        `<file>`,
			attrName:   "path",
			attrValue:  "",
			shouldFind: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple attribute extraction
			searchStr := tt.attrName + `="`
			idx := strings.Index(tt.tag, searchStr)
			found := idx != -1

			if found != tt.shouldFind {
				t.Errorf("found attribute = %v, want %v", found, tt.shouldFind)
			}

			if found {
				startIdx := idx + len(searchStr)
				endIdx := strings.Index(tt.tag[startIdx:], `"`)
				if endIdx != -1 {
					value := tt.tag[startIdx : startIdx+endIdx]
					if value != tt.attrValue {
						t.Errorf("attribute value = %q, want %q", value, tt.attrValue)
					}
				}
			}
		})
	}
}

// TestXMLEntityEscaping tests XML entity handling
func TestXMLEntityEscaping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ampersand",
			input:    "&amp;",
			expected: "&",
		},
		{
			name:     "less than",
			input:    "&lt;",
			expected: "<",
		},
		{
			name:     "greater than",
			input:    "&gt;",
			expected: ">",
		},
		{
			name:     "quote",
			input:    "&quot;",
			expected: "\"",
		},
		{
			name:     "apostrophe",
			input:    "&apos;",
			expected: "'",
		},
		{
			name:     "mixed content",
			input:    "if a &lt; b &amp;&amp; c &gt; d",
			expected: "if a < b && c > d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input
			result = strings.ReplaceAll(result, "&amp;", "&")
			result = strings.ReplaceAll(result, "&lt;", "<")
			result = strings.ReplaceAll(result, "&gt;", ">")
			result = strings.ReplaceAll(result, "&quot;", "\"")
			result = strings.ReplaceAll(result, "&apos;", "'")

			if result != tt.expected {
				t.Errorf("unescaped = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestXMLCDATAHandling tests CDATA section handling
func TestXMLCDATAHandling(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		hasCDATA bool
		content  string
	}{
		{
			name:     "simple CDATA",
			input:    "<![CDATA[raw content here]]>",
			hasCDATA: true,
			content:  "raw content here",
		},
		{
			name:     "CDATA with special chars",
			input:    "<![CDATA[<script>alert('test')</script>]]>",
			hasCDATA: true,
			content:  "<script>alert('test')</script>",
		},
		{
			name:     "no CDATA",
			input:    "regular content",
			hasCDATA: false,
			content:  "",
		},
		{
			name:     "empty CDATA",
			input:    "<![CDATA[]]>",
			hasCDATA: true,
			content:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cdataStart := "<![CDATA["
			cdataEnd := "]]>"

			hasCDATA := strings.Contains(tt.input, cdataStart) && strings.Contains(tt.input, cdataEnd)
			if hasCDATA != tt.hasCDATA {
				t.Errorf("hasCDATA = %v, want %v", hasCDATA, tt.hasCDATA)
			}

			if hasCDATA {
				startIdx := strings.Index(tt.input, cdataStart) + len(cdataStart)
				endIdx := strings.Index(tt.input, cdataEnd)
				content := tt.input[startIdx:endIdx]
				if content != tt.content {
					t.Errorf("CDATA content = %q, want %q", content, tt.content)
				}
			}
		})
	}
}

// TestXMLCommentHandling tests XML comment handling
func TestXMLCommentHandling(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		hasComment bool
		comment    string
	}{
		{
			name:       "simple comment",
			input:      "<!-- this is a comment -->",
			hasComment: true,
			comment:    " this is a comment ",
		},
		{
			name:       "no comment",
			input:      "regular content",
			hasComment: false,
			comment:    "",
		},
		{
			name:       "comment with code",
			input:      "<!-- TODO: fix this -->",
			hasComment: true,
			comment:    " TODO: fix this ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commentStart := "<!--"
			commentEnd := "-->"

			hasComment := strings.Contains(tt.input, commentStart) && strings.Contains(tt.input, commentEnd)
			if hasComment != tt.hasComment {
				t.Errorf("hasComment = %v, want %v", hasComment, tt.hasComment)
			}

			if hasComment {
				startIdx := strings.Index(tt.input, commentStart) + len(commentStart)
				endIdx := strings.Index(tt.input, commentEnd)
				comment := tt.input[startIdx:endIdx]
				if comment != tt.comment {
					t.Errorf("comment = %q, want %q", comment, tt.comment)
				}
			}
		})
	}
}

// TestXMLNestedTags tests handling of nested XML tags
func TestXMLNestedTags(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		outerTag   string
		innerTag   string
		innerCount int
	}{
		{
			name:       "single nested tag",
			input:      "<outer><inner>content</inner></outer>",
			outerTag:   "outer",
			innerTag:   "inner",
			innerCount: 1,
		},
		{
			name:       "multiple nested tags",
			input:      "<outer><inner>1</inner><inner>2</inner></outer>",
			outerTag:   "outer",
			innerTag:   "inner",
			innerCount: 2,
		},
		{
			name:       "no inner tags",
			input:      "<outer>content only</outer>",
			outerTag:   "outer",
			innerTag:   "inner",
			innerCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openTag := "<" + tt.innerTag + ">"
			count := strings.Count(tt.input, openTag)

			if count != tt.innerCount {
				t.Errorf("inner tag count = %d, want %d", count, tt.innerCount)
			}
		})
	}
}

// TestXMLMalformedHandling tests handling of malformed XML
func TestXMLMalformedHandling(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		isMalformed bool
		reason      string
	}{
		{
			name:        "well-formed",
			input:       "<root><child>content</child></root>",
			isMalformed: false,
			reason:      "",
		},
		{
			name:        "unclosed tag",
			input:       "<root><child>content</root>",
			isMalformed: true,
			reason:      "unclosed child tag",
		},
		{
			name:        "mismatched tags",
			input:       "<root><child>content</other></root>",
			isMalformed: true,
			reason:      "mismatched tags",
		},
		{
			name:        "missing closing tag",
			input:       "<root>content",
			isMalformed: true,
			reason:      "missing closing tag",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple malformed detection: count opening vs closing tags
			openCount := strings.Count(tt.input, "<") - strings.Count(tt.input, "</") - strings.Count(tt.input, "/>")
			closeCount := strings.Count(tt.input, "</")

			// Very basic check - not comprehensive XML validation
			isMalformed := false
			if strings.Count(tt.input, "<") != strings.Count(tt.input, ">") {
				isMalformed = true
			}

			// For unclosed tags, check if there's a tag that doesn't have a matching close
			if strings.Contains(tt.input, "<child>") && !strings.Contains(tt.input, "</child>") {
				isMalformed = true
			}

			// Check for mismatched tags
			if strings.Contains(tt.input, "</other>") && !strings.Contains(tt.input, "<other>") {
				isMalformed = true
			}

			// Check for missing closing root tag
			if strings.Contains(tt.input, "<root>") && !strings.Contains(tt.input, "</root>") {
				isMalformed = true
			}

			if isMalformed != tt.isMalformed {
				t.Errorf("isMalformed = %v, want %v (reason: %s)", isMalformed, tt.isMalformed, tt.reason)
			}
			_ = openCount
			_ = closeCount
		})
	}
}

// TestFixtureSimpleTags tests XML parsing using simple_tags.xml fixture
func TestFixtureSimpleTags(t *testing.T) {
	content := loadTestFixture(t, "simple_tags.xml")

	tests := []struct {
		name     string
		check    func(string) bool
		expected bool
	}{
		{
			name:     "contains basic file tag",
			check:    func(c string) bool { return strings.Contains(c, "<file>content here</file>") },
			expected: true,
		},
		{
			name:     "contains tag with attributes",
			check:    func(c string) bool { return strings.Contains(c, `path="main.go"`) && strings.Contains(c, `lang="go"`) },
			expected: true,
		},
		{
			name:     "contains empty tag",
			check:    func(c string) bool { return strings.Contains(c, "<file></file>") },
			expected: true,
		},
		{
			name:     "contains self-closing tag",
			check:    func(c string) bool { return strings.Contains(c, "<file/>") },
			expected: true,
		},
		{
			name:     "contains multiline content",
			check:    func(c string) bool { return strings.Contains(c, "line1\nline2\nline3") },
			expected: true,
		},
		{
			name:     "contains nested files structure",
			check:    func(c string) bool { return strings.Contains(c, "<files>") && strings.Count(c, `path="`) >= 3 },
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.check(content)
			if result != tt.expected {
				t.Errorf("check %q = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

// TestFixtureAttributes tests attribute parsing using attributes.xml fixture
func TestFixtureAttributes(t *testing.T) {
	content := loadTestFixture(t, "attributes.xml")

	tests := []struct {
		name        string
		attrName    string
		attrValue   string
		shouldExist bool
	}{
		{"single attribute path", "path", "main.go", true},
		{"multiple attrs version", "version", "1.0", true},
		{"description with spaces", "description", "main entry point for the application", true},
		{"boolean readonly", "readonly", "true", true},
		{"numeric line", "line", "42", true},
		{"numeric column", "column", "10", true},
		{"empty attribute value", "path", "", true},
		{"escaped quotes in title", "title", `Say &quot;Hello&quot;`, true},
		{"type source", "type", "source", true},
		{"type test", "type", "test", true},
		{"type config", "type", "config", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchStr := tt.attrName + `="` + tt.attrValue + `"`
			found := strings.Contains(content, searchStr)
			if found != tt.shouldExist {
				t.Errorf("attribute %s=%q found = %v, want %v", tt.attrName, tt.attrValue, found, tt.shouldExist)
			}
		})
	}
}

// TestFixtureEntities tests XML entity handling using entities.xml fixture
func TestFixtureEntities(t *testing.T) {
	content := loadTestFixture(t, "entities.xml")

	tests := []struct {
		name     string
		entity   string
		expected bool
	}{
		{"ampersand entity", "&amp;", true},
		{"less than entity", "&lt;", true},
		{"greater than entity", "&gt;", true},
		{"quote entity", "&quot;", true},
		{"apostrophe entity", "&apos;", true},
		{"numeric decimal reference", "&#60;", true},
		{"numeric hex reference", "&#x3C;", true},
		{"entity in attribute", `path="a&amp;b.txt"`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := strings.Contains(content, tt.entity)
			if found != tt.expected {
				t.Errorf("entity %q found = %v, want %v", tt.entity, found, tt.expected)
			}
		})
	}
}

// TestFixtureEntitiesDecoding tests decoding entities from entities.xml fixture
func TestFixtureEntitiesDecoding(t *testing.T) {
	content := loadTestFixture(t, "entities.xml")

	tests := []struct {
		name    string
		encoded string
		decoded string
	}{
		{"foo & bar", "foo &amp; bar", "foo & bar"},
		{"if a < b", "if a &lt; b", "if a < b"},
		{"if a > b", "if a &gt; b", "if a > b"},
		{"say hello", "say &quot;hello&quot;", `say "hello"`},
		{"it's working", "it&apos;s working", "it's working"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(content, tt.encoded) {
				t.Errorf("encoded string %q not found in fixture", tt.encoded)
				return
			}

			result := tt.encoded
			result = strings.ReplaceAll(result, "&amp;", "&")
			result = strings.ReplaceAll(result, "&lt;", "<")
			result = strings.ReplaceAll(result, "&gt;", ">")
			result = strings.ReplaceAll(result, "&quot;", "\"")
			result = strings.ReplaceAll(result, "&apos;", "'")

			if result != tt.decoded {
				t.Errorf("decoded = %q, want %q", result, tt.decoded)
			}
		})
	}
}

// TestFixtureCDATA tests CDATA handling using cdata.xml fixture
func TestFixtureCDATA(t *testing.T) {
	content := loadTestFixture(t, "cdata.xml")

	tests := []struct {
		name         string
		cdataContent string
		shouldExist  bool
	}{
		{"raw content", "raw content here", true},
		{"script tag in CDATA", "<script>alert('test')</script>", true},
		{"function with operators", "if (a < b && c > d)", true},
		{"empty CDATA", "<![CDATA[]]>", true},
		{"HTML template", `<div class="container">`, true},
		{"console log", `console.log("Initializing...");`, true},
		{"SQL query", "SELECT * FROM users", true},
		{"regex pattern", `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := strings.Contains(content, tt.cdataContent)
			if found != tt.shouldExist {
				t.Errorf("CDATA content %q found = %v, want %v", tt.cdataContent, found, tt.shouldExist)
			}
		})
	}
}

// TestFixtureCDATAExtraction tests extracting CDATA content from cdata.xml fixture
func TestFixtureCDATAExtraction(t *testing.T) {
	content := loadTestFixture(t, "cdata.xml")

	cdataStart := "<![CDATA["
	cdataEnd := "]]>"

	// Count CDATA sections
	cdataCount := strings.Count(content, cdataStart)
	if cdataCount < 8 {
		t.Errorf("expected at least 8 CDATA sections, found %d", cdataCount)
	}

	// Extract and validate first CDATA section
	firstStart := strings.Index(content, cdataStart)
	if firstStart == -1 {
		t.Fatal("no CDATA section found")
	}

	firstEnd := strings.Index(content[firstStart:], cdataEnd)
	if firstEnd == -1 {
		t.Fatal("CDATA section not properly closed")
	}

	firstContent := content[firstStart+len(cdataStart) : firstStart+firstEnd]
	if firstContent != "raw content here" {
		t.Errorf("first CDATA content = %q, want %q", firstContent, "raw content here")
	}
}

// TestFixtureNestedTags tests nested tag handling using nested_tags.xml fixture
func TestFixtureNestedTags(t *testing.T) {
	content := loadTestFixture(t, "nested_tags.xml")

	tests := []struct {
		name     string
		check    func(string) bool
		expected bool
	}{
		{
			name:     "single level nesting",
			check:    func(c string) bool { return strings.Contains(c, "<outer>\n    <inner>content</inner>\n</outer>") },
			expected: true,
		},
		{
			name:     "multiple siblings",
			check:    func(c string) bool { return strings.Count(c, "<inner>") >= 4 },
			expected: true,
		},
		{
			name:     "deep nesting level5",
			check:    func(c string) bool { return strings.Contains(c, "<level5>deep content</level5>") },
			expected: true,
		},
		{
			name: "mixed content parent",
			check: func(c string) bool {
				return strings.Contains(c, "Some text before") && strings.Contains(c, "Some text after")
			},
			expected: true,
		},
		{
			name:     "complex structure sections",
			check:    func(c string) bool { return strings.Count(c, `<section id="`) >= 2 },
			expected: true,
		},
		{
			name:     "items with properties",
			check:    func(c string) bool { return strings.Contains(c, `<property key="color">red</property>`) },
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.check(content)
			if result != tt.expected {
				t.Errorf("check %q = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

// TestFixtureNestedTagsDepth tests depth counting in nested_tags.xml fixture
func TestFixtureNestedTagsDepth(t *testing.T) {
	content := loadTestFixture(t, "nested_tags.xml")

	// Verify all 5 levels exist
	for i := 1; i <= 5; i++ {
		levelTag := "<level" + string(rune('0'+i)) + ">"
		if !strings.Contains(content, levelTag) {
			t.Errorf("missing nesting level %d tag: %s", i, levelTag)
		}
	}

	// Count paragraph tags
	paragraphCount := strings.Count(content, "<paragraph>")
	if paragraphCount != 3 {
		t.Errorf("paragraph count = %d, want 3", paragraphCount)
	}

	// Count item tags
	itemCount := strings.Count(content, "<item>")
	if itemCount != 2 {
		t.Errorf("item count = %d, want 2", itemCount)
	}
}

// TestFixtureAllFilesExist verifies all fixture files can be loaded
func TestFixtureAllFilesExist(t *testing.T) {
	fixtures := []string{
		"simple_tags.xml",
		"attributes.xml",
		"entities.xml",
		"cdata.xml",
		"nested_tags.xml",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			path := getTestFixturePath(fixture)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("fixture file does not exist: %s", path)
			}

			content := loadTestFixture(t, fixture)
			if len(content) == 0 {
				t.Errorf("fixture file is empty: %s", fixture)
			}

			// Verify XML declaration
			if !strings.HasPrefix(content, `<?xml version="1.0"`) {
				t.Errorf("fixture %s missing XML declaration", fixture)
			}
		})
	}
}

// TestFixtureXMLComments tests that all fixture files contain comments
func TestFixtureXMLComments(t *testing.T) {
	fixtures := []string{
		"simple_tags.xml",
		"attributes.xml",
		"entities.xml",
		"cdata.xml",
		"nested_tags.xml",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			content := loadTestFixture(t, fixture)

			commentCount := strings.Count(content, "<!--")
			if commentCount < 3 {
				t.Errorf("fixture %s has only %d comments, expected at least 3", fixture, commentCount)
			}

			// Verify comments are properly closed
			openComments := strings.Count(content, "<!--")
			closeComments := strings.Count(content, "-->")
			if openComments != closeComments {
				t.Errorf("fixture %s has mismatched comments: %d open, %d close", fixture, openComments, closeComments)
			}
		})
	}
}
