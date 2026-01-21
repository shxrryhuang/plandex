package lib

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"plandex-cli/fs"

	shared "plandex-shared"

	"gopkg.in/yaml.v3"
)

const (
	// TemplatesDir is the name of the templates directory in plandex home
	TemplatesDir = "templates"
	// BuiltInTemplatesDir is the subdirectory for built-in templates
	BuiltInTemplatesDir = "builtin"
	// UserTemplatesDir is the subdirectory for user-created templates
	UserTemplatesDir = "user"
)

// GetTemplatesBaseDir returns the base directory for all templates
func GetTemplatesBaseDir() string {
	return filepath.Join(fs.HomePlandexDir, TemplatesDir)
}

// GetBuiltInTemplatesDir returns the directory for built-in templates
func GetBuiltInTemplatesDir() string {
	return filepath.Join(GetTemplatesBaseDir(), BuiltInTemplatesDir)
}

// GetUserTemplatesDir returns the directory for user-created templates
func GetUserTemplatesDir() string {
	return filepath.Join(GetTemplatesBaseDir(), UserTemplatesDir)
}

// EnsureTemplatesDirs creates the templates directories if they don't exist
func EnsureTemplatesDirs() error {
	builtInDir := GetBuiltInTemplatesDir()
	userDir := GetUserTemplatesDir()

	if err := os.MkdirAll(builtInDir, 0755); err != nil {
		return fmt.Errorf("error creating built-in templates dir: %v", err)
	}
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return fmt.Errorf("error creating user templates dir: %v", err)
	}

	// Initialize built-in templates if they don't exist
	if err := initBuiltInTemplates(builtInDir); err != nil {
		return fmt.Errorf("error initializing built-in templates: %v", err)
	}

	return nil
}

// LoadTemplate loads a template by name from the templates directories
func LoadTemplate(name string) (*shared.PlanTemplate, error) {
	// First check user templates
	userDir := GetUserTemplatesDir()
	userPath := filepath.Join(userDir, name+".yaml")
	if _, err := os.Stat(userPath); err == nil {
		return loadTemplateFromFile(userPath, false)
	}

	// Then check built-in templates
	builtInDir := GetBuiltInTemplatesDir()
	builtInPath := filepath.Join(builtInDir, name+".yaml")
	if _, err := os.Stat(builtInPath); err == nil {
		return loadTemplateFromFile(builtInPath, true)
	}

	return nil, fmt.Errorf("template '%s' not found", name)
}

// ListTemplates returns all available templates
func ListTemplates() ([]*shared.PlanTemplate, error) {
	var templates []*shared.PlanTemplate

	// Load built-in templates
	builtInDir := GetBuiltInTemplatesDir()
	builtInTemplates, err := loadTemplatesFromDir(builtInDir, true)
	if err != nil {
		return nil, fmt.Errorf("error loading built-in templates: %v", err)
	}
	templates = append(templates, builtInTemplates...)

	// Load user templates
	userDir := GetUserTemplatesDir()
	userTemplates, err := loadTemplatesFromDir(userDir, false)
	if err != nil {
		return nil, fmt.Errorf("error loading user templates: %v", err)
	}
	templates = append(templates, userTemplates...)

	// Sort by category then name
	sort.Slice(templates, func(i, j int) bool {
		if templates[i].Category != templates[j].Category {
			return templates[i].Category < templates[j].Category
		}
		return templates[i].Name < templates[j].Name
	})

	return templates, nil
}

// SaveTemplate saves a template to the user templates directory
func SaveTemplate(tmpl *shared.PlanTemplate) error {
	userDir := GetUserTemplatesDir()

	if err := os.MkdirAll(userDir, 0755); err != nil {
		return fmt.Errorf("error creating user templates dir: %v", err)
	}

	// Generate ID from name if not set
	if tmpl.Id == "" {
		tmpl.Id = generateTemplateId(tmpl.Name)
	}

	filePath := filepath.Join(userDir, tmpl.Id+".yaml")

	data, err := yaml.Marshal(tmpl)
	if err != nil {
		return fmt.Errorf("error marshaling template: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("error writing template file: %v", err)
	}

	return nil
}

// DeleteTemplate deletes a user template
func DeleteTemplate(name string) error {
	userDir := GetUserTemplatesDir()
	filePath := filepath.Join(userDir, name+".yaml")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("template '%s' not found in user templates", name)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("error deleting template: %v", err)
	}

	return nil
}

// ExecuteTemplate renders a template with the provided variables
func ExecuteTemplate(tmpl *shared.PlanTemplate, variables map[string]string) (*shared.TemplateExecution, error) {
	// Validate required variables
	for _, v := range tmpl.Variables {
		if v.Required {
			if val, ok := variables[v.Name]; !ok || val == "" {
				return nil, fmt.Errorf("required variable '%s' not provided", v.Name)
			}
		}
	}

	// Apply defaults for missing variables
	for _, v := range tmpl.Variables {
		if _, ok := variables[v.Name]; !ok && v.Default != "" {
			variables[v.Name] = v.Default
		}
	}

	// Render the initial prompt
	renderedPrompt, err := renderTemplate(tmpl.InitialPrompt, variables)
	if err != nil {
		return nil, fmt.Errorf("error rendering template: %v", err)
	}

	// Generate plan name from template name and first variable
	planName := tmpl.Name
	if len(variables) > 0 {
		for _, v := range tmpl.Variables {
			if val, ok := variables[v.Name]; ok && val != "" {
				planName = fmt.Sprintf("%s - %s", tmpl.Name, val)
				break
			}
		}
	}

	return &shared.TemplateExecution{
		Template:       tmpl,
		Variables:      variables,
		RenderedPrompt: renderedPrompt,
		PlanName:       planName,
	}, nil
}

// loadTemplateFromFile loads a single template from a YAML file
func loadTemplateFromFile(path string, builtIn bool) (*shared.PlanTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading template file: %v", err)
	}

	var tmpl shared.PlanTemplate
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("error parsing template file: %v", err)
	}

	tmpl.BuiltIn = builtIn

	// Set ID from filename if not specified
	if tmpl.Id == "" {
		tmpl.Id = strings.TrimSuffix(filepath.Base(path), ".yaml")
	}

	return &tmpl, nil
}

// loadTemplatesFromDir loads all templates from a directory
func loadTemplatesFromDir(dir string, builtIn bool) ([]*shared.PlanTemplate, error) {
	var templates []*shared.PlanTemplate

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return templates, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("error reading templates directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		tmpl, err := loadTemplateFromFile(path, builtIn)
		if err != nil {
			// Log error but continue loading other templates
			fmt.Printf("Warning: error loading template %s: %v\n", entry.Name(), err)
			continue
		}

		templates = append(templates, tmpl)
	}

	return templates, nil
}

// renderTemplate renders a template string with the provided variables
func renderTemplate(templateStr string, variables map[string]string) (string, error) {
	// Create a new template with custom delimiters to avoid conflicts
	tmpl, err := template.New("prompt").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("error parsing template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, variables); err != nil {
		return "", fmt.Errorf("error executing template: %v", err)
	}

	return buf.String(), nil
}

// generateTemplateId generates a URL-safe ID from a template name
func generateTemplateId(name string) string {
	// Convert to lowercase and replace spaces with hyphens
	id := strings.ToLower(name)
	id = strings.ReplaceAll(id, " ", "-")

	// Remove any characters that aren't alphanumeric or hyphens
	reg := regexp.MustCompile("[^a-z0-9-]+")
	id = reg.ReplaceAllString(id, "")

	// Remove consecutive hyphens
	reg = regexp.MustCompile("-+")
	id = reg.ReplaceAllString(id, "-")

	// Trim hyphens from start and end
	id = strings.Trim(id, "-")

	return id
}

// initBuiltInTemplates creates the default built-in templates if they don't exist
func initBuiltInTemplates(dir string) error {
	templates := getBuiltInTemplates()

	for _, tmpl := range templates {
		filePath := filepath.Join(dir, tmpl.Id+".yaml")
		if _, err := os.Stat(filePath); err == nil {
			// Template already exists, skip
			continue
		}

		data, err := yaml.Marshal(tmpl)
		if err != nil {
			return fmt.Errorf("error marshaling template %s: %v", tmpl.Id, err)
		}

		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return fmt.Errorf("error writing template %s: %v", tmpl.Id, err)
		}
	}

	return nil
}

// getBuiltInTemplates returns the list of built-in templates
func getBuiltInTemplates() []*shared.PlanTemplate {
	return []*shared.PlanTemplate{
		{
			Id:          "new-api-endpoint",
			Name:        "New REST API Endpoint",
			Description: "Create a new REST API endpoint with validation, error handling, and tests",
			Category:    "backend",
			Tags:        []string{"api", "rest", "endpoint"},
			Variables: []shared.TemplateVariable{
				{
					Name:     "EndpointName",
					Prompt:   "What should the endpoint be called?",
					Type:     "string",
					Required: true,
				},
				{
					Name:    "HttpMethod",
					Prompt:  "Which HTTP method?",
					Type:    "select",
					Options: []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
					Default: "GET",
				},
				{
					Name:    "AuthRequired",
					Prompt:  "Require authentication?",
					Type:    "boolean",
					Default: "true",
				},
			},
			InitialPrompt: `Create a new {{.HttpMethod}} endpoint at /api/{{.EndpointName}}.

Requirements:
- Input validation for all parameters
- Proper error handling with appropriate HTTP status codes
- Follow existing code patterns and conventions
{{if eq .AuthRequired "true"}}- Authentication required - use existing auth middleware{{end}}

Please also create:
1. Unit tests for the endpoint
2. Update API documentation if it exists
3. Add any necessary types/models

Look at existing endpoints for reference on coding style and patterns.`,
			AutoContext: []string{
				"src/routes/**/*.ts",
				"src/controllers/**/*.ts",
				"src/middleware/*.ts",
			},
			Author:  "Plandex",
			Version: "1.0.0",
			BuiltIn: true,
		},
		{
			Id:          "new-react-component",
			Name:        "New React Component",
			Description: "Create a new React component with TypeScript, styling, and tests",
			Category:    "frontend",
			Tags:        []string{"react", "component", "typescript"},
			Variables: []shared.TemplateVariable{
				{
					Name:     "ComponentName",
					Prompt:   "What should the component be called?",
					Type:     "string",
					Required: true,
				},
				{
					Name:    "ComponentType",
					Prompt:  "What type of component?",
					Type:    "select",
					Options: []string{"functional", "page", "layout", "form"},
					Default: "functional",
				},
				{
					Name:    "WithStyles",
					Prompt:  "Include styling?",
					Type:    "boolean",
					Default: "true",
				},
				{
					Name:    "WithTests",
					Prompt:  "Include tests?",
					Type:    "boolean",
					Default: "true",
				},
			},
			InitialPrompt: `Create a new React {{.ComponentType}} component called {{.ComponentName}}.

Requirements:
- Use TypeScript with proper type definitions
- Follow existing component patterns in the codebase
- Use functional component with hooks
{{if eq .WithStyles "true"}}- Include styling using the project's existing styling approach (CSS modules, styled-components, or Tailwind){{end}}
{{if eq .WithTests "true"}}- Create unit tests using the project's testing framework{{end}}

The component should:
1. Be well-documented with JSDoc comments
2. Handle loading and error states if applicable
3. Be accessible (proper ARIA attributes)
4. Be responsive

Look at existing components for reference on coding style and patterns.`,
			AutoContext: []string{
				"src/components/**/*.tsx",
				"src/components/**/*.css",
				"src/styles/*.css",
			},
			Author:  "Plandex",
			Version: "1.0.0",
			BuiltIn: true,
		},
		{
			Id:          "add-feature",
			Name:        "Add New Feature",
			Description: "Plan and implement a new feature with full-stack considerations",
			Category:    "general",
			Tags:        []string{"feature", "fullstack", "planning"},
			Variables: []shared.TemplateVariable{
				{
					Name:     "FeatureName",
					Prompt:   "What is the feature called?",
					Type:     "string",
					Required: true,
				},
				{
					Name:     "FeatureDescription",
					Prompt:   "Describe what the feature should do:",
					Type:     "string",
					Required: true,
				},
				{
					Name:    "IncludeTests",
					Prompt:  "Include tests?",
					Type:    "boolean",
					Default: "true",
				},
			},
			InitialPrompt: `I want to add a new feature: {{.FeatureName}}

Feature Description:
{{.FeatureDescription}}

Please:
1. First, analyze the existing codebase to understand the architecture and patterns
2. Create a detailed implementation plan
3. Implement the feature following existing patterns and conventions
4. Handle edge cases and error scenarios
{{if eq .IncludeTests "true"}}5. Write comprehensive tests for the new functionality{{end}}

Consider:
- Database changes if needed
- API changes if needed
- Frontend changes if needed
- Security implications
- Performance implications`,
			Author:  "Plandex",
			Version: "1.0.0",
			BuiltIn: true,
		},
		{
			Id:          "fix-bug",
			Name:        "Fix Bug",
			Description: "Investigate and fix a bug with proper testing",
			Category:    "maintenance",
			Tags:        []string{"bug", "fix", "debugging"},
			Variables: []shared.TemplateVariable{
				{
					Name:     "BugDescription",
					Prompt:   "Describe the bug:",
					Type:     "string",
					Required: true,
				},
				{
					Name:   "StepsToReproduce",
					Prompt: "Steps to reproduce (optional):",
					Type:   "string",
				},
				{
					Name:   "ExpectedBehavior",
					Prompt: "What should happen instead?",
					Type:   "string",
				},
			},
			InitialPrompt: `I need to fix a bug.

Bug Description:
{{.BugDescription}}

{{if .StepsToReproduce}}Steps to Reproduce:
{{.StepsToReproduce}}{{end}}

{{if .ExpectedBehavior}}Expected Behavior:
{{.ExpectedBehavior}}{{end}}

Please:
1. First, investigate the codebase to understand the root cause
2. Identify all affected code paths
3. Implement a fix that addresses the root cause
4. Add a regression test to prevent this bug from recurring
5. Check for similar issues elsewhere in the codebase`,
			Author:  "Plandex",
			Version: "1.0.0",
			BuiltIn: true,
		},
		{
			Id:          "refactor-code",
			Name:        "Refactor Code",
			Description: "Refactor code for better maintainability and performance",
			Category:    "maintenance",
			Tags:        []string{"refactor", "cleanup", "improvement"},
			Variables: []shared.TemplateVariable{
				{
					Name:     "TargetArea",
					Prompt:   "What code area do you want to refactor?",
					Type:     "string",
					Required: true,
				},
				{
					Name:    "RefactorGoal",
					Prompt:  "What's the goal of this refactoring?",
					Type:    "select",
					Options: []string{"improve readability", "improve performance", "reduce duplication", "better separation of concerns", "update to new patterns"},
					Default: "improve readability",
				},
			},
			InitialPrompt: `I want to refactor: {{.TargetArea}}

Goal: {{.RefactorGoal}}

Please:
1. Analyze the current implementation
2. Identify specific improvements to make
3. Refactor the code while maintaining the same functionality
4. Ensure all existing tests still pass
5. Update any affected documentation

Important:
- Make incremental changes that are easy to review
- Don't change functionality unless explicitly needed
- Follow existing code patterns and conventions
- Consider backward compatibility`,
			Author:  "Plandex",
			Version: "1.0.0",
			BuiltIn: true,
		},
		{
			Id:          "write-tests",
			Name:        "Write Tests",
			Description: "Add tests for existing code",
			Category:    "testing",
			Tags:        []string{"testing", "unit-tests", "coverage"},
			Variables: []shared.TemplateVariable{
				{
					Name:     "TargetCode",
					Prompt:   "What code do you want to test?",
					Type:     "string",
					Required: true,
				},
				{
					Name:    "TestType",
					Prompt:  "What type of tests?",
					Type:    "select",
					Options: []string{"unit tests", "integration tests", "e2e tests", "all"},
					Default: "unit tests",
				},
			},
			InitialPrompt: `I want to add {{.TestType}} for: {{.TargetCode}}

Please:
1. Analyze the code to understand all code paths and edge cases
2. Write comprehensive tests covering:
   - Happy path scenarios
   - Edge cases
   - Error handling
   - Boundary conditions
3. Follow the existing testing patterns in the codebase
4. Use descriptive test names that explain what is being tested
5. Add any necessary test utilities or fixtures

Aim for high code coverage while focusing on meaningful tests.`,
			AutoContext: []string{
				"**/*.test.*",
				"**/*.spec.*",
				"**/test/**/*",
				"**/tests/**/*",
			},
			Author:  "Plandex",
			Version: "1.0.0",
			BuiltIn: true,
		},
	}
}
