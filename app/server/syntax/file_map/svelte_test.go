package file_map

import (
	"testing"
)

func TestGetSvelteScriptAndStyle(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantScript     string
		wantScriptLang string
		wantStyle      string
	}{
		{
			name:           "empty content",
			content:        "",
			wantScript:     "",
			wantScriptLang: "",
			wantStyle:      "",
		},
		{
			name: "script only",
			content: `<script>
				let count = 0;
			</script>`,
			wantScript:     "let count = 0;",
			wantScriptLang: "",
			wantStyle:      "",
		},
		{
			name: "typescript script",
			content: `<script lang="ts">
				let count: number = 0;
			</script>`,
			wantScript:     "let count: number = 0;",
			wantScriptLang: "ts",
			wantStyle:      "",
		},
		{
			name: "style only",
			content: `<style>
				.container { color: red; }
			</style>`,
			wantScript:     "",
			wantScriptLang: "",
			wantStyle:      ".container { color: red; }",
		},
		{
			name: "script and style",
			content: `<script>
				let name = 'world';
			</script>
			<style>
				h1 { color: blue; }
			</style>`,
			wantScript:     "let name = 'world';",
			wantScriptLang: "",
			wantStyle:      "h1 { color: blue; }",
		},
		{
			name: "complete svelte component",
			content: `<script lang="ts">
				export let name: string;
			</script>

			<h1>Hello {name}!</h1>

			<style>
				h1 { font-size: 2em; }
			</style>`,
			wantScript:     "export let name: string;",
			wantScriptLang: "ts",
			wantStyle:      "h1 { font-size: 2em; }",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script, lang, style := getSvelteScriptAndStyle([]byte(tt.content))

			if script != tt.wantScript {
				t.Errorf("script = %q, want %q", script, tt.wantScript)
			}
			if lang != tt.wantScriptLang {
				t.Errorf("scriptLang = %q, want %q", lang, tt.wantScriptLang)
			}
			if style != tt.wantStyle {
				t.Errorf("style = %q, want %q", style, tt.wantStyle)
			}
		})
	}
}

func TestMapSvelte(t *testing.T) {
	tests := []struct {
		name             string
		content          string
		minDefCount      int
		checkTypes       []string
	}{
		{
			name:        "empty content",
			content:     "",
			minDefCount: 0,
		},
		{
			name: "script only component",
			content: `<script>
				let count = 0;
				function increment() { count++; }
			</script>`,
			minDefCount: 1,
			checkTypes:  []string{"svelte-script"},
		},
		{
			name: "markup only component",
			content: `<div class="container">
				<h1>Hello</h1>
			</div>`,
			minDefCount: 1,
			checkTypes:  []string{"tag"},
		},
		{
			name: "script and markup",
			content: `<script>
				let name = 'world';
			</script>
			<div>Hello {name}</div>`,
			minDefCount: 2,
			checkTypes:  []string{"svelte-script", "tag"},
		},
		{
			name: "full component with style",
			content: `<script lang="ts">
				export let name: string = 'world';
			</script>
			<main>
				<h1>Hello {name}!</h1>
			</main>
			<style>
				main { padding: 1em; }
			</style>`,
			minDefCount: 3,
			checkTypes:  []string{"svelte-script", "tag", "svelte-style"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defs := mapSvelte([]byte(tt.content))

			if len(defs) < tt.minDefCount {
				t.Errorf("mapSvelte() count = %d, want at least %d", len(defs), tt.minDefCount)
				return
			}

			// Check that expected types are present
			typeMap := make(map[string]bool)
			for _, def := range defs {
				typeMap[def.Type] = true
			}

			for _, expectedType := range tt.checkTypes {
				if !typeMap[expectedType] {
					t.Errorf("missing expected type %q in definitions", expectedType)
				}
			}
		})
	}
}

func TestMapSvelteWithTypescript(t *testing.T) {
	content := `<script lang="ts">
		interface User {
			name: string;
			age: number;
		}

		let user: User = { name: 'John', age: 30 };

		function greet(u: User): string {
			return 'Hello ' + u.name;
		}
	</script>
	<div>{greet(user)}</div>`

	defs := mapSvelte([]byte(content))

	if len(defs) == 0 {
		t.Fatal("mapSvelte() returned no definitions")
	}

	// First def should be svelte-script
	if defs[0].Type != "svelte-script" {
		t.Errorf("first def Type = %q, want %q", defs[0].Type, "svelte-script")
	}

	// Should have TypeScript signature
	if defs[0].Signature != `<script lang="ts">` {
		t.Errorf("Signature = %q, want %q", defs[0].Signature, `<script lang="ts">`)
	}
}

func TestMapSvelteWithJavaScript(t *testing.T) {
	content := `<script>
		let count = 0;

		function increment() {
			count++;
		}

		const double = (n) => n * 2;
	</script>
	<button on:click={increment}>Count: {count}</button>`

	defs := mapSvelte([]byte(content))

	if len(defs) == 0 {
		t.Fatal("mapSvelte() returned no definitions")
	}

	// First def should be svelte-script
	if defs[0].Type != "svelte-script" {
		t.Errorf("first def Type = %q, want %q", defs[0].Type, "svelte-script")
	}

	// Should have JavaScript signature (empty lang)
	if defs[0].Signature != `<script lang="">` {
		t.Errorf("Signature = %q, want %q", defs[0].Signature, `<script lang="">`)
	}
}

func TestMapSvelteStyleDefinition(t *testing.T) {
	content := `<style>
		.container {
			display: flex;
			flex-direction: column;
		}

		.item {
			padding: 1rem;
		}
	</style>`

	defs := mapSvelte([]byte(content))

	// Should have style definition
	hasStyle := false
	for _, def := range defs {
		if def.Type == "svelte-style" {
			hasStyle = true
			if def.Signature != "<style>" {
				t.Errorf("style Signature = %q, want %q", def.Signature, "<style>")
			}
		}
	}

	if !hasStyle {
		t.Error("mapSvelte() did not include style definition")
	}
}

func TestMapSvelteComplexComponent(t *testing.T) {
	content := `<script lang="ts">
		import { onMount } from 'svelte';
		import type { User } from './types';

		export let users: User[] = [];
		let loading = true;

		onMount(async () => {
			const res = await fetch('/api/users');
			users = await res.json();
			loading = false;
		});

		function formatName(user: User): string {
			return user.firstName + ' ' + user.lastName;
		}
	</script>

	<main>
		<header>
			<nav>
				<ul>
					<li><a href="/">Home</a></li>
					<li><a href="/about">About</a></li>
				</ul>
			</nav>
		</header>

		<section>
			{#if loading}
				<p>Loading...</p>
			{:else}
				{#each users as user}
					<div class="user-card">
						<h2>{formatName(user)}</h2>
					</div>
				{/each}
			{/if}
		</section>
	</main>

	<style>
		main {
			max-width: 800px;
			margin: 0 auto;
		}

		.user-card {
			border: 1px solid #ccc;
			padding: 1rem;
			margin: 0.5rem 0;
		}
	</style>`

	defs := mapSvelte([]byte(content))

	// Should have at least 3 definitions (script, markup, style)
	if len(defs) < 3 {
		t.Errorf("mapSvelte() returned %d definitions, want at least 3", len(defs))
	}

	// Check for all three types
	types := make(map[string]bool)
	for _, def := range defs {
		types[def.Type] = true
	}

	expectedTypes := []string{"svelte-script", "tag", "svelte-style"}
	for _, et := range expectedTypes {
		if !types[et] {
			t.Errorf("missing definition type: %s", et)
		}
	}
}

func TestMapSvelteMalformedContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "unclosed script tag",
			content: "<script>let x = 1;",
		},
		{
			name:    "unclosed style tag",
			content: "<style>.foo { color: red; }",
		},
		{
			name:    "invalid html",
			content: "<div><script>x</div></script>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			defs := mapSvelte([]byte(tt.content))
			// Just verify it returns something
			_ = defs
		})
	}
}
