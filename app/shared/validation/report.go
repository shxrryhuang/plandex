package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"time"
)

// ValidationReport represents a complete validation report
type ValidationReport struct {
	Timestamp     time.Time          `json:"timestamp"`
	Duration      time.Duration      `json:"duration_ms"`
	TotalChecks   int                `json:"total_checks"`
	ErrorCount    int                `json:"error_count"`
	WarningCount  int                `json:"warning_count"`
	Status        string             `json:"status"` // "pass", "fail", "warn"
	Errors        []*ValidationError `json:"errors,omitempty"`
	Warnings      []*ValidationError `json:"warnings,omitempty"`
	SystemInfo    *SystemInfo        `json:"system_info,omitempty"`
	Configuration *ConfigInfo        `json:"configuration,omitempty"`
}

// SystemInfo contains system information
type SystemInfo struct {
	Hostname    string            `json:"hostname"`
	Environment map[string]string `json:"environment,omitempty"`
	WorkingDir  string            `json:"working_dir"`
}

// ConfigInfo contains configuration information
type ConfigInfo struct {
	Phase           string   `json:"phase"`
	CheckFileAccess bool     `json:"check_file_access"`
	Verbose         bool     `json:"verbose"`
	ProviderNames   []string `json:"provider_names,omitempty"`
}

// NewValidationReport creates a report from validation results
func NewValidationReport(result *ValidationResult, duration time.Duration, opts ValidationOptions) *ValidationReport {
	status := "pass"
	if result.HasErrors() {
		status = "fail"
	} else if result.HasWarnings() {
		status = "warn"
	}

	hostname, _ := os.Hostname()
	workingDir, _ := os.Getwd()

	return &ValidationReport{
		Timestamp:    time.Now(),
		Duration:     duration,
		TotalChecks:  len(result.Errors) + len(result.Warnings),
		ErrorCount:   len(result.Errors),
		WarningCount: len(result.Warnings),
		Status:       status,
		Errors:       result.Errors,
		Warnings:     result.Warnings,
		SystemInfo: &SystemInfo{
			Hostname:   hostname,
			WorkingDir: workingDir,
		},
		Configuration: &ConfigInfo{
			Phase:           string(opts.Phase),
			CheckFileAccess: opts.CheckFileAccess,
			Verbose:         opts.Verbose,
			ProviderNames:   opts.ProviderNames,
		},
	}
}

// ToJSON converts the report to JSON
func (r *ValidationReport) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// SaveJSON saves the report as a JSON file
func (r *ValidationReport) SaveJSON(filePath string) error {
	data, err := r.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	return nil
}

// ToHTML converts the report to HTML
func (r *ValidationReport) ToHTML() (string, error) {
	tmpl := template.Must(template.New("report").Parse(htmlTemplate))

	var buf []byte
	if err := tmpl.Execute(os.Stdout, r); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return string(buf), nil
}

// SaveHTML saves the report as an HTML file
func (r *ValidationReport) SaveHTML(filePath string) error {
	tmpl := template.Must(template.New("report").Parse(htmlTemplate))

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create HTML file: %w", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, r); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return nil
}

// Summary returns a brief text summary
func (r *ValidationReport) Summary() string {
	icon := "✅"
	statusText := "PASSED"

	if r.Status == "fail" {
		icon = "❌"
		statusText = "FAILED"
	} else if r.Status == "warn" {
		icon = "⚠️"
		statusText = "PASSED WITH WARNINGS"
	}

	return fmt.Sprintf(`%s Validation %s

Duration: %dms
Checks:   %d
Errors:   %d
Warnings: %d
Status:   %s
`, icon, statusText, r.Duration.Milliseconds(), r.TotalChecks, r.ErrorCount, r.WarningCount, r.Status)
}

// HTML template for validation report
const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Plandex Validation Report</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif; background: #f5f5f5; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { background: white; padding: 30px; border-radius: 8px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .header h1 { color: #333; margin-bottom: 10px; }
        .status { display: inline-block; padding: 5px 15px; border-radius: 20px; font-weight: bold; margin: 10px 0; }
        .status.pass { background: #d4edda; color: #155724; }
        .status.fail { background: #f8d7da; color: #721c24; }
        .status.warn { background: #fff3cd; color: #856404; }
        .metrics { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin: 20px 0; }
        .metric { background: #f8f9fa; padding: 15px; border-radius: 5px; border-left: 4px solid #007bff; }
        .metric-label { font-size: 12px; color: #666; text-transform: uppercase; }
        .metric-value { font-size: 32px; font-weight: bold; color: #333; margin-top: 5px; }
        .section { background: white; padding: 25px; border-radius: 8px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .section h2 { color: #333; margin-bottom: 15px; border-bottom: 2px solid #007bff; padding-bottom: 10px; }
        .error, .warning { padding: 15px; margin-bottom: 15px; border-radius: 5px; border-left: 4px solid #dc3545; background: #f8d7da; }
        .warning { border-left-color: #ffc107; background: #fff3cd; }
        .error-title { font-weight: bold; color: #721c24; margin-bottom: 5px; }
        .warning-title { font-weight: bold; color: #856404; margin-bottom: 5px; }
        .error-details, .warning-details { color: #666; font-size: 14px; margin: 5px 0; }
        .error-solution, .warning-solution { color: #155724; background: #d4edda; padding: 10px; border-radius: 3px; margin-top: 10px; font-size: 14px; }
        pre { background: #f4f4f4; padding: 10px; border-radius: 3px; overflow-x: auto; }
        .footer { text-align: center; color: #666; margin-top: 30px; font-size: 14px; }
        .timestamp { color: #999; font-size: 14px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔍 Plandex Validation Report</h1>
            <div class="status {{.Status}}">{{if eq .Status "pass"}}✅ PASSED{{else if eq .Status "fail"}}❌ FAILED{{else}}⚠️  PASSED WITH WARNINGS{{end}}</div>
            <p class="timestamp">Generated: {{.Timestamp.Format "2006-01-02 15:04:05 MST"}}</p>
        </div>

        <div class="metrics">
            <div class="metric">
                <div class="metric-label">Duration</div>
                <div class="metric-value">{{.Duration.Milliseconds}}ms</div>
            </div>
            <div class="metric">
                <div class="metric-label">Total Checks</div>
                <div class="metric-value">{{.TotalChecks}}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Errors</div>
                <div class="metric-value" style="color: #dc3545;">{{.ErrorCount}}</div>
            </div>
            <div class="metric">
                <div class="metric-label">Warnings</div>
                <div class="metric-value" style="color: #ffc107;">{{.WarningCount}}</div>
            </div>
        </div>

        {{if .Errors}}
        <div class="section">
            <h2>❌ Errors ({{len .Errors}})</h2>
            {{range .Errors}}
            <div class="error">
                <div class="error-title">{{.Summary}}</div>
                {{if .Details}}<div class="error-details">{{.Details}}</div>{{end}}
                {{if .Impact}}<div class="error-details"><strong>Impact:</strong> {{.Impact}}</div>{{end}}
                {{if .Solution}}<div class="error-solution"><strong>Solution:</strong><br>{{.Solution}}</div>{{end}}
            </div>
            {{end}}
        </div>
        {{end}}

        {{if .Warnings}}
        <div class="section">
            <h2>⚠️  Warnings ({{len .Warnings}})</h2>
            {{range .Warnings}}
            <div class="warning">
                <div class="warning-title">{{.Summary}}</div>
                {{if .Details}}<div class="warning-details">{{.Details}}</div>{{end}}
                {{if .Solution}}<div class="warning-solution"><strong>Suggestion:</strong><br>{{.Solution}}</div>{{end}}
            </div>
            {{end}}
        </div>
        {{end}}

        {{if and (not .Errors) (not .Warnings)}}
        <div class="section">
            <h2>✅ All Checks Passed</h2>
            <p>No errors or warnings found. Your configuration is valid!</p>
        </div>
        {{end}}

        <div class="section">
            <h2>ℹ️  System Information</h2>
            <p><strong>Hostname:</strong> {{.SystemInfo.Hostname}}</p>
            <p><strong>Working Directory:</strong> {{.SystemInfo.WorkingDir}}</p>
            <p><strong>Validation Phase:</strong> {{.Configuration.Phase}}</p>
        </div>

        <div class="footer">
            <p>Generated by Plandex Configuration Validation System</p>
        </div>
    </div>
</body>
</html>`

// GenerateReport validates and generates a report
func GenerateReport(ctx context.Context, opts ValidationOptions, outputPath string) error {
	start := time.Now()

	// Run validation
	validator := NewValidator(opts)
	result := validator.ValidateAll(ctx)

	duration := time.Since(start)

	// Create report
	report := NewValidationReport(result, duration, opts)

	// Save based on file extension
	if outputPath != "" {
		ext := outputPath[len(outputPath)-5:]
		if ext == ".json" {
			return report.SaveJSON(outputPath)
		} else if ext == ".html" {
			return report.SaveHTML(outputPath)
		}
		return fmt.Errorf("unsupported file extension (use .json or .html)")
	}

	// Print to console
	fmt.Println(report.Summary())
	return nil
}
