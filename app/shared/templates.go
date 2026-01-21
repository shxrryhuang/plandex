package shared

// PlanTemplate represents a reusable template for creating plans
type PlanTemplate struct {
	// Unique identifier for the template
	Id string `json:"id" yaml:"id"`

	// Human-readable name of the template
	Name string `json:"name" yaml:"name"`

	// Description of what this template does
	Description string `json:"description" yaml:"description"`

	// Category for organizing templates (e.g., "api", "frontend", "testing")
	Category string `json:"category,omitempty" yaml:"category,omitempty"`

	// Tags for searching and filtering
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Variables that users must provide when using the template
	Variables []TemplateVariable `json:"variables,omitempty" yaml:"variables,omitempty"`

	// The initial prompt to send after creating the plan
	// Supports variable interpolation using {{.VariableName}} syntax
	InitialPrompt string `json:"initialPrompt" yaml:"initialPrompt"`

	// Files or patterns to auto-load into context
	AutoContext []string `json:"autoContext,omitempty" yaml:"autoContext,omitempty"`

	// Optional model pack to use for this template
	ModelPack string `json:"modelPack,omitempty" yaml:"modelPack,omitempty"`

	// Optional plan configuration overrides
	PlanConfig *PlanConfig `json:"planConfig,omitempty" yaml:"planConfig,omitempty"`

	// Author of the template (for shared templates)
	Author string `json:"author,omitempty" yaml:"author,omitempty"`

	// Version of the template
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// Whether this is a built-in template
	BuiltIn bool `json:"builtIn,omitempty" yaml:"builtIn,omitempty"`

	// Organization ID for org-shared templates (empty for user templates)
	OrgId string `json:"orgId,omitempty" yaml:"orgId,omitempty"`
}

// TemplateVariable represents a variable that users must provide
type TemplateVariable struct {
	// Name of the variable (used in template interpolation)
	Name string `json:"name" yaml:"name"`

	// Human-readable prompt to show the user
	Prompt string `json:"prompt" yaml:"prompt"`

	// Type of the variable: "string", "boolean", "select"
	Type string `json:"type,omitempty" yaml:"type,omitempty"`

	// Default value if user doesn't provide one
	Default string `json:"default,omitempty" yaml:"default,omitempty"`

	// Options for "select" type variables
	Options []string `json:"options,omitempty" yaml:"options,omitempty"`

	// Whether this variable is required
	Required bool `json:"required,omitempty" yaml:"required,omitempty"`

	// Validation pattern (regex) for the variable
	Validation string `json:"validation,omitempty" yaml:"validation,omitempty"`
}

// TemplateExecution represents the result of executing a template
type TemplateExecution struct {
	// The template that was executed
	Template *PlanTemplate `json:"template"`

	// Variable values provided by the user
	Variables map[string]string `json:"variables"`

	// The rendered initial prompt
	RenderedPrompt string `json:"renderedPrompt"`

	// Plan name to use
	PlanName string `json:"planName"`
}

// ListTemplatesRequest is the request for listing templates
type ListTemplatesRequest struct {
	Category  string `json:"category,omitempty"`
	Tag       string `json:"tag,omitempty"`
	BuiltIn   *bool  `json:"builtIn,omitempty"`
	OrgShared *bool  `json:"orgShared,omitempty"`
}

// ListTemplatesResponse is the response for listing templates
type ListTemplatesResponse struct {
	Templates []*PlanTemplate `json:"templates"`
}

// CreateTemplateRequest is the request for creating a new template
type CreateTemplateRequest struct {
	Template *PlanTemplate `json:"template"`
}

// CreateTemplateResponse is the response for creating a template
type CreateTemplateResponse struct {
	Template *PlanTemplate `json:"template"`
}
