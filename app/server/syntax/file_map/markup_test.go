package file_map

import (
	"testing"
)

func TestMapMarkup(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		minCount     int
		containsSig  string
	}{
		{
			name:     "empty content",
			content:  "",
			minCount: 0,
		},
		{
			name:        "simple div",
			content:     "<div>content</div>",
			minCount:    1,
			containsSig: "div",
		},
		{
			name:        "nested structure",
			content:     "<html><body><div>content</div></body></html>",
			minCount:    1,
			containsSig: "html",
		},
		{
			name:        "nav element",
			content:     "<nav><a href='/'>Home</a></nav>",
			minCount:    1,
			containsSig: "nav",
		},
		{
			name:        "section element",
			content:     "<section><h1>Title</h1><p>Content</p></section>",
			minCount:    1,
			containsSig: "section",
		},
		{
			name:        "form element",
			content:     "<form><input type='text'/></form>",
			minCount:    1,
			containsSig: "form",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defs := mapMarkup([]byte(tt.content))

			if len(defs) < tt.minCount {
				t.Errorf("mapMarkup() count = %d, want at least %d", len(defs), tt.minCount)
			}

			if tt.containsSig != "" {
				found := containsSignature(defs, tt.containsSig)
				if !found {
					t.Errorf("expected to find signature %q in definitions", tt.containsSig)
				}
			}
		})
	}
}

// Helper to recursively check if any definition contains the signature
func containsSignature(defs []Definition, sig string) bool {
	for _, def := range defs {
		if def.Signature == sig {
			return true
		}
		if containsSignature(def.Children, sig) {
			return true
		}
	}
	return false
}

func TestMapMarkupWithAttributes(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectedAttrs []string
		targetSig     string
	}{
		{
			name:          "div with id",
			content:       `<div id="main">content</div>`,
			expectedAttrs: []string{"#main"},
			targetSig:     "div",
		},
		{
			name:          "div with class",
			content:       `<div class="container">content</div>`,
			expectedAttrs: []string{".container"},
			targetSig:     "div",
		},
		{
			name:          "div with id and class",
			content:       `<div id="app" class="wrapper">content</div>`,
			expectedAttrs: []string{"#app", ".wrapper"},
			targetSig:     "div",
		},
		{
			name:          "multiple classes",
			content:       `<div class="class1 class2 class3">content</div>`,
			expectedAttrs: []string{".class1.class2.class3"},
			targetSig:     "div",
		},
		{
			name:          "more than 3 classes truncated",
			content:       `<div class="a b c d e">content</div>`,
			expectedAttrs: []string{".a.b.c"},
			targetSig:     "div",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defs := mapMarkup([]byte(tt.content))

			if len(defs) == 0 {
				t.Fatal("mapMarkup() returned no definitions")
			}

			// Find the target definition recursively
			def := findDefinitionBySignature(defs, tt.targetSig)
			if def == nil {
				t.Fatalf("could not find definition with signature %q", tt.targetSig)
			}

			attrs := def.TagAttrs
			if len(attrs) != len(tt.expectedAttrs) {
				t.Errorf("attrs count = %d, want %d", len(attrs), len(tt.expectedAttrs))
				return
			}

			for i, expected := range tt.expectedAttrs {
				if attrs[i] != expected {
					t.Errorf("attr[%d] = %q, want %q", i, attrs[i], expected)
				}
			}
		})
	}
}

// Helper to find a definition by signature recursively
func findDefinitionBySignature(defs []Definition, sig string) *Definition {
	for i := range defs {
		if defs[i].Signature == sig {
			return &defs[i]
		}
		if found := findDefinitionBySignature(defs[i].Children, sig); found != nil {
			return found
		}
	}
	return nil
}

func TestIsSignificantTag(t *testing.T) {
	significantTags := []string{
		"html", "head", "body", "main", "nav", "header", "footer",
		"article", "section", "form", "dialog", "template", "table",
		"div", "ul", "aside",
	}

	nonSignificantTags := []string{
		"span", "p", "a", "img", "input", "button", "label",
		"h1", "h2", "h3", "li", "tr", "td", "th",
	}

	for _, tag := range significantTags {
		t.Run("significant_"+tag, func(t *testing.T) {
			if !isSignificantTag(tag) {
				t.Errorf("isSignificantTag(%q) = false, want true", tag)
			}
		})
	}

	for _, tag := range nonSignificantTags {
		t.Run("non_significant_"+tag, func(t *testing.T) {
			if isSignificantTag(tag) {
				t.Errorf("isSignificantTag(%q) = true, want false", tag)
			}
		})
	}
}

func TestAreMarkupDefinitionsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        Definition
		b        Definition
		expected bool
	}{
		{
			name:     "identical definitions",
			a:        Definition{Type: "tag", Signature: "div", TagAttrs: []string{"#id"}},
			b:        Definition{Type: "tag", Signature: "div", TagAttrs: []string{"#id"}},
			expected: true,
		},
		{
			name:     "different types",
			a:        Definition{Type: "tag", Signature: "div"},
			b:        Definition{Type: "function", Signature: "div"},
			expected: false,
		},
		{
			name:     "different signatures",
			a:        Definition{Type: "tag", Signature: "div"},
			b:        Definition{Type: "tag", Signature: "span"},
			expected: false,
		},
		{
			name:     "different attr counts",
			a:        Definition{Type: "tag", Signature: "div", TagAttrs: []string{"#id"}},
			b:        Definition{Type: "tag", Signature: "div", TagAttrs: []string{"#id", ".class"}},
			expected: false,
		},
		{
			name:     "different attr values",
			a:        Definition{Type: "tag", Signature: "div", TagAttrs: []string{"#id1"}},
			b:        Definition{Type: "tag", Signature: "div", TagAttrs: []string{"#id2"}},
			expected: false,
		},
		{
			name:     "both empty attrs",
			a:        Definition{Type: "tag", Signature: "div", TagAttrs: []string{}},
			b:        Definition{Type: "tag", Signature: "div", TagAttrs: []string{}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := areMarkupDefinitionsEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("areMarkupDefinitionsEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConsolidateRepeatedTags(t *testing.T) {
	tests := []struct {
		name        string
		defs        []Definition
		wantCount   int
		wantTagReps int
	}{
		{
			name: "single definition",
			defs: []Definition{
				{Type: "tag", Signature: "div"},
			},
			wantCount:   1,
			wantTagReps: 0,
		},
		{
			name: "two identical definitions",
			defs: []Definition{
				{Type: "tag", Signature: "div"},
				{Type: "tag", Signature: "div"},
			},
			wantCount:   1,
			wantTagReps: 2,
		},
		{
			name: "three identical definitions",
			defs: []Definition{
				{Type: "tag", Signature: "li"},
				{Type: "tag", Signature: "li"},
				{Type: "tag", Signature: "li"},
			},
			wantCount:   1,
			wantTagReps: 3,
		},
		{
			name: "different definitions not consolidated",
			defs: []Definition{
				{Type: "tag", Signature: "div"},
				{Type: "tag", Signature: "span"},
			},
			wantCount:   2,
			wantTagReps: 0,
		},
		{
			name: "definitions with children not consolidated",
			defs: []Definition{
				{Type: "tag", Signature: "div", Children: []Definition{{Type: "tag", Signature: "span"}}},
				{Type: "tag", Signature: "div"},
			},
			wantCount:   2,
			wantTagReps: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := consolidateRepeatedTags(tt.defs)

			if len(result) != tt.wantCount {
				t.Errorf("consolidateRepeatedTags() count = %d, want %d", len(result), tt.wantCount)
			}

			if tt.wantTagReps > 0 && len(result) > 0 {
				if result[0].TagReps != tt.wantTagReps {
					t.Errorf("TagReps = %d, want %d", result[0].TagReps, tt.wantTagReps)
				}
			}
		})
	}
}

func TestMapMarkupComplexStructure(t *testing.T) {
	html := `
	<!DOCTYPE html>
	<html>
		<head>
			<title>Test</title>
		</head>
		<body>
			<header>
				<nav>
					<ul>
						<li><a href="/">Home</a></li>
						<li><a href="/about">About</a></li>
					</ul>
				</nav>
			</header>
			<main>
				<article>
					<section>
						<h1>Title</h1>
						<p>Content</p>
					</section>
				</article>
			</main>
			<footer>
				<p>Copyright 2024</p>
			</footer>
		</body>
	</html>
	`

	defs := mapMarkup([]byte(html))

	if len(defs) == 0 {
		t.Fatal("mapMarkup() returned no definitions for complex HTML")
	}

	// Should have html as root
	if defs[0].Signature != "html" {
		t.Errorf("root Signature = %q, want %q", defs[0].Signature, "html")
	}

	// Should have children
	if len(defs[0].Children) == 0 {
		t.Error("html element has no children")
	}
}

func TestMapMarkupMalformedHTML(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "unclosed tag",
			content: "<div>content",
		},
		{
			name:    "mismatched tags",
			content: "<div><span></div></span>",
		},
		{
			name:    "missing close tag",
			content: "<div><p>text</div>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			defs := mapMarkup([]byte(tt.content))
			// Just verify it returns something (even if empty)
			_ = defs
		})
	}
}

func TestDefinitionStructure(t *testing.T) {
	def := Definition{
		Type:      "tag",
		Signature: "div",
		TagAttrs:  []string{"#main", ".container"},
		TagReps:   3,
		Children: []Definition{
			{Type: "tag", Signature: "span"},
		},
	}

	if def.Type != "tag" {
		t.Errorf("Type = %q, want %q", def.Type, "tag")
	}
	if def.Signature != "div" {
		t.Errorf("Signature = %q, want %q", def.Signature, "div")
	}
	if len(def.TagAttrs) != 2 {
		t.Errorf("TagAttrs len = %d, want 2", len(def.TagAttrs))
	}
	if def.TagReps != 3 {
		t.Errorf("TagReps = %d, want 3", def.TagReps)
	}
	if len(def.Children) != 1 {
		t.Errorf("Children len = %d, want 1", len(def.Children))
	}
}
