package cmd

import (
	"fmt"
	"os"
	"strings"

	"plandex-cli/api"
	"plandex-cli/auth"
	"plandex-cli/lib"
	"plandex-cli/plan_exec"
	"plandex-cli/term"
	"plandex-cli/types"

	shared "plandex-shared"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/plandex-ai/survey/v2"
	"github.com/spf13/cobra"
)

var templateCategory string

func init() {
	RootCmd.AddCommand(templatesCmd)
	templatesCmd.AddCommand(templatesListCmd)
	templatesCmd.AddCommand(templatesUseCmd)
	templatesCmd.AddCommand(templatesShowCmd)
	templatesCmd.AddCommand(templatesCreateCmd)
	templatesCmd.AddCommand(templatesDeleteCmd)

	templatesListCmd.Flags().StringVarP(&templateCategory, "category", "c", "", "Filter by category")
}

// templatesCmd is the parent command for template operations
var templatesCmd = &cobra.Command{
	Use:     "templates",
	Aliases: []string{"tmpl", "template"},
	Short:   "Manage plan templates",
	Long:    `Plan templates allow you to quickly start new plans with pre-defined prompts, variables, and context.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to listing templates
		templatesList(cmd, args)
	},
}

// templatesListCmd lists all available templates
var templatesListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List available templates",
	Run:     templatesList,
}

func templatesList(cmd *cobra.Command, args []string) {
	// Ensure templates directory exists
	if err := lib.EnsureTemplatesDirs(); err != nil {
		term.OutputErrorAndExit("Error initializing templates: %v", err)
	}

	templates, err := lib.ListTemplates()
	if err != nil {
		term.OutputErrorAndExit("Error loading templates: %v", err)
	}

	if len(templates) == 0 {
		fmt.Println("No templates found.")
		fmt.Println()
		term.PrintCmds("", "templates create")
		return
	}

	// Filter by category if specified
	if templateCategory != "" {
		var filtered []*shared.PlanTemplate
		for _, t := range templates {
			if strings.EqualFold(t.Category, templateCategory) {
				filtered = append(filtered, t)
			}
		}
		templates = filtered

		if len(templates) == 0 {
			fmt.Printf("No templates found in category '%s'.\n", templateCategory)
			return
		}
	}

	fmt.Println()
	color.New(color.Bold).Println("📋 Available Templates")
	fmt.Println()

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Category", "Description", "Type"})
	table.SetAutoWrapText(false)
	table.SetBorder(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetTablePadding("  ")
	table.SetNoWhiteSpace(true)

	for _, t := range templates {
		typeStr := "user"
		if t.BuiltIn {
			typeStr = "built-in"
		}

		desc := t.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}

		table.Append([]string{
			color.New(color.FgCyan).Sprint(t.Id),
			t.Category,
			desc,
			typeStr,
		})
	}

	table.Render()
	fmt.Println()
	term.PrintCmds("", "templates use <name>", "templates show <name>")
}

// templatesUseCmd uses a template to create a new plan
var templatesUseCmd = &cobra.Command{
	Use:   "use [template-name]",
	Short: "Create a new plan from a template",
	Long:  `Start a new plan using the specified template. You'll be prompted to fill in any required variables.`,
	Args:  cobra.MaximumNArgs(1),
	Run:   templatesUse,
}

func templatesUse(cmd *cobra.Command, args []string) {
	auth.MustResolveAuthWithOrg()
	lib.MustResolveOrCreateProject()

	// Ensure templates directory exists
	if err := lib.EnsureTemplatesDirs(); err != nil {
		term.OutputErrorAndExit("Error initializing templates: %v", err)
	}

	var templateName string
	if len(args) > 0 {
		templateName = args[0]
	} else {
		// Show selection menu
		templates, err := lib.ListTemplates()
		if err != nil {
			term.OutputErrorAndExit("Error loading templates: %v", err)
		}

		if len(templates) == 0 {
			fmt.Println("No templates available. Create one with 'plandex templates create'")
			return
		}

		var options []string
		for _, t := range templates {
			label := fmt.Sprintf("%s - %s", t.Id, t.Description)
			if len(label) > 70 {
				label = label[:67] + "..."
			}
			options = append(options, label)
		}

		var selectedIdx int
		prompt := &survey.Select{
			Message: "Select a template:",
			Options: options,
		}
		if err := survey.AskOne(prompt, &selectedIdx); err != nil {
			term.OutputErrorAndExit("Error selecting template: %v", err)
		}

		templateName = templates[selectedIdx].Id
	}

	// Load the template
	tmpl, err := lib.LoadTemplate(templateName)
	if err != nil {
		term.OutputErrorAndExit("Error loading template: %v", err)
	}

	fmt.Println()
	color.New(color.Bold).Printf("📋 Using template: %s\n", tmpl.Name)
	fmt.Println()

	// Collect variables from user
	variables := make(map[string]string)
	for _, v := range tmpl.Variables {
		var value string

		switch v.Type {
		case "select":
			var selectedIdx int
			prompt := &survey.Select{
				Message: v.Prompt,
				Options: v.Options,
			}
			if v.Default != "" {
				for i, opt := range v.Options {
					if opt == v.Default {
						prompt.Default = v.Options[i]
						break
					}
				}
			}
			if err := survey.AskOne(prompt, &selectedIdx); err != nil {
				term.OutputErrorAndExit("Error getting input: %v", err)
			}
			value = v.Options[selectedIdx]

		case "boolean":
			var confirmed bool
			prompt := &survey.Confirm{
				Message: v.Prompt,
				Default: v.Default == "true",
			}
			if err := survey.AskOne(prompt, &confirmed); err != nil {
				term.OutputErrorAndExit("Error getting input: %v", err)
			}
			if confirmed {
				value = "true"
			} else {
				value = "false"
			}

		default: // string
			prompt := &survey.Input{
				Message: v.Prompt,
				Default: v.Default,
			}
			if err := survey.AskOne(prompt, &value); err != nil {
				term.OutputErrorAndExit("Error getting input: %v", err)
			}
		}

		if value == "" && v.Required {
			term.OutputErrorAndExit("Variable '%s' is required", v.Name)
		}

		variables[v.Name] = value
	}

	// Execute the template
	execution, err := lib.ExecuteTemplate(tmpl, variables)
	if err != nil {
		term.OutputErrorAndExit("Error executing template: %v", err)
	}

	// Create the plan
	term.StartSpinner("")

	res, apiErr := api.Client.CreatePlan(lib.CurrentProjectId, shared.CreatePlanRequest{Name: execution.PlanName})
	if apiErr != nil {
		term.OutputErrorAndExit("Error creating plan: %v", apiErr.Msg)
	}

	if err := lib.WriteCurrentPlan(res.Id); err != nil {
		term.OutputErrorAndExit("Error setting current plan: %v", err)
	}

	if err := lib.WriteCurrentBranch("main"); err != nil {
		term.OutputErrorAndExit("Error setting current branch: %v", err)
	}

	term.StopSpinner()

	fmt.Printf("✅ Created plan '%s' from template '%s'\n", color.New(color.Bold, term.ColorHiGreen).Sprint(execution.PlanName), tmpl.Name)
	fmt.Println()

	// Load auto-context if specified
	if len(tmpl.AutoContext) > 0 {
		fmt.Println("📥 Loading template context...")
		lib.MustLoadContext(tmpl.AutoContext, &types.LoadContextParams{
			DefsOnly:          true,
			SkipIgnoreWarning: true,
			AutoLoaded:        true,
		})
	}

	// Show the rendered prompt
	fmt.Println()
	color.New(color.Bold).Println("📝 Template prompt ready:")
	fmt.Println()
	fmt.Println(color.New(color.FgHiBlack).Sprint("─────────────────────────────────────────────"))
	fmt.Println(execution.RenderedPrompt)
	fmt.Println(color.New(color.FgHiBlack).Sprint("─────────────────────────────────────────────"))
	fmt.Println()

	// Ask if they want to send the prompt
	var sendPrompt bool
	surveyPrompt := &survey.Confirm{
		Message: "Send this prompt to start the plan?",
		Default: true,
	}
	if err := survey.AskOne(surveyPrompt, &sendPrompt); err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}

	if sendPrompt {
		fmt.Println()
		fmt.Println("💬 Starting plan with template prompt...")
		fmt.Println()

		// Use the tell command functionality
		tellFlags := types.TellFlags{
			ExecEnabled: true,
		}

		plan_exec.TellPlan(plan_exec.ExecParams{
			CurrentPlanId: lib.CurrentPlanId,
			CurrentBranch: lib.CurrentBranch,
			AuthVars:      lib.MustVerifyAuthVars(auth.Current.IntegratedModelsMode),
			CheckOutdatedContext: func(maybeContexts []*shared.Context, projectPaths *types.ProjectPaths) (bool, bool, error) {
				return lib.CheckOutdatedContextWithOutput(false, false, maybeContexts, projectPaths)
			},
		}, execution.RenderedPrompt, tellFlags)
	} else {
		fmt.Println()
		fmt.Println("The prompt has been prepared. Use 'plandex tell' to send it when ready.")
		fmt.Println()
		term.PrintCmds("", "tell", "load", "context")
	}
}

// templatesShowCmd shows details of a template
var templatesShowCmd = &cobra.Command{
	Use:   "show <template-name>",
	Short: "Show template details",
	Args:  cobra.ExactArgs(1),
	Run:   templatesShow,
}

func templatesShow(cmd *cobra.Command, args []string) {
	// Ensure templates directory exists
	if err := lib.EnsureTemplatesDirs(); err != nil {
		term.OutputErrorAndExit("Error initializing templates: %v", err)
	}

	tmpl, err := lib.LoadTemplate(args[0])
	if err != nil {
		term.OutputErrorAndExit("Error loading template: %v", err)
	}

	fmt.Println()
	color.New(color.Bold).Printf("📋 Template: %s\n", tmpl.Name)
	fmt.Println()

	fmt.Printf("  %s: %s\n", color.New(color.Bold).Sprint("ID"), tmpl.Id)
	fmt.Printf("  %s: %s\n", color.New(color.Bold).Sprint("Description"), tmpl.Description)
	fmt.Printf("  %s: %s\n", color.New(color.Bold).Sprint("Category"), tmpl.Category)

	if tmpl.BuiltIn {
		fmt.Printf("  %s: built-in\n", color.New(color.Bold).Sprint("Type"))
	} else {
		fmt.Printf("  %s: user\n", color.New(color.Bold).Sprint("Type"))
	}

	if len(tmpl.Tags) > 0 {
		fmt.Printf("  %s: %s\n", color.New(color.Bold).Sprint("Tags"), strings.Join(tmpl.Tags, ", "))
	}

	if tmpl.Author != "" {
		fmt.Printf("  %s: %s\n", color.New(color.Bold).Sprint("Author"), tmpl.Author)
	}

	if tmpl.Version != "" {
		fmt.Printf("  %s: %s\n", color.New(color.Bold).Sprint("Version"), tmpl.Version)
	}

	if len(tmpl.Variables) > 0 {
		fmt.Println()
		color.New(color.Bold).Println("  Variables:")
		for _, v := range tmpl.Variables {
			reqStr := ""
			if v.Required {
				reqStr = color.New(color.FgRed).Sprint(" (required)")
			}
			fmt.Printf("    • %s%s: %s\n", color.New(color.FgCyan).Sprint(v.Name), reqStr, v.Prompt)
			if v.Type == "select" && len(v.Options) > 0 {
				fmt.Printf("      Options: %s\n", strings.Join(v.Options, ", "))
			}
			if v.Default != "" {
				fmt.Printf("      Default: %s\n", v.Default)
			}
		}
	}

	if len(tmpl.AutoContext) > 0 {
		fmt.Println()
		color.New(color.Bold).Println("  Auto-load context:")
		for _, ctx := range tmpl.AutoContext {
			fmt.Printf("    • %s\n", ctx)
		}
	}

	fmt.Println()
	color.New(color.Bold).Println("  Initial Prompt Template:")
	fmt.Println(color.New(color.FgHiBlack).Sprint("  ─────────────────────────────────────────────"))
	for _, line := range strings.Split(tmpl.InitialPrompt, "\n") {
		fmt.Printf("  %s\n", line)
	}
	fmt.Println(color.New(color.FgHiBlack).Sprint("  ─────────────────────────────────────────────"))
	fmt.Println()

	term.PrintCmds("", "templates use "+tmpl.Id)
}

// templatesCreateCmd creates a new template
var templatesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new template",
	Long:  `Create a new plan template interactively.`,
	Run:   templatesCreate,
}

func templatesCreate(cmd *cobra.Command, args []string) {
	// Ensure templates directory exists
	if err := lib.EnsureTemplatesDirs(); err != nil {
		term.OutputErrorAndExit("Error initializing templates: %v", err)
	}

	tmpl := &shared.PlanTemplate{}

	// Get basic info
	namePrompt := &survey.Input{Message: "Template name:"}
	if err := survey.AskOne(namePrompt, &tmpl.Name); err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}
	if tmpl.Name == "" {
		term.OutputErrorAndExit("Template name is required")
	}

	descPrompt := &survey.Input{Message: "Description:"}
	if err := survey.AskOne(descPrompt, &tmpl.Description); err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}

	categoryPrompt := &survey.Select{
		Message: "Category:",
		Options: []string{"general", "backend", "frontend", "testing", "maintenance", "other"},
		Default: "general",
	}
	if err := survey.AskOne(categoryPrompt, &tmpl.Category); err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}

	// Get variables
	fmt.Println()
	var addVariables bool
	addVarsPrompt := &survey.Confirm{Message: "Add variables?", Default: false}
	if err := survey.AskOne(addVarsPrompt, &addVariables); err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}

	for addVariables {
		variable := shared.TemplateVariable{}

		varNamePrompt := &survey.Input{Message: "Variable name (e.g., FeatureName):"}
		if err := survey.AskOne(varNamePrompt, &variable.Name); err != nil {
			term.OutputErrorAndExit("Error: %v", err)
		}

		varPromptPrompt := &survey.Input{Message: "Prompt to show user:"}
		if err := survey.AskOne(varPromptPrompt, &variable.Prompt); err != nil {
			term.OutputErrorAndExit("Error: %v", err)
		}

		varTypePrompt := &survey.Select{
			Message: "Variable type:",
			Options: []string{"string", "boolean", "select"},
			Default: "string",
		}
		if err := survey.AskOne(varTypePrompt, &variable.Type); err != nil {
			term.OutputErrorAndExit("Error: %v", err)
		}

		if variable.Type == "select" {
			var optionsStr string
			optionsPrompt := &survey.Input{Message: "Options (comma-separated):"}
			if err := survey.AskOne(optionsPrompt, &optionsStr); err != nil {
				term.OutputErrorAndExit("Error: %v", err)
			}
			for _, opt := range strings.Split(optionsStr, ",") {
				opt = strings.TrimSpace(opt)
				if opt != "" {
					variable.Options = append(variable.Options, opt)
				}
			}
		}

		var requiredInput bool
		requiredPrompt := &survey.Confirm{Message: "Required?", Default: true}
		if err := survey.AskOne(requiredPrompt, &requiredInput); err != nil {
			term.OutputErrorAndExit("Error: %v", err)
		}
		variable.Required = requiredInput

		tmpl.Variables = append(tmpl.Variables, variable)

		addMorePrompt := &survey.Confirm{Message: "Add another variable?", Default: false}
		if err := survey.AskOne(addMorePrompt, &addVariables); err != nil {
			term.OutputErrorAndExit("Error: %v", err)
		}
	}

	// Get initial prompt
	fmt.Println()
	fmt.Println("Enter the initial prompt template.")
	fmt.Println("Use {{.VariableName}} to insert variables.")
	fmt.Println("Press Enter twice to finish.")
	fmt.Println()

	promptPrompt := &survey.Multiline{Message: "Initial prompt:"}
	if err := survey.AskOne(promptPrompt, &tmpl.InitialPrompt); err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}

	// Get auto-context
	fmt.Println()
	var autoContextStr string
	autoContextPrompt := &survey.Input{Message: "Auto-load context patterns (comma-separated, e.g., src/**/*.ts):"}
	if err := survey.AskOne(autoContextPrompt, &autoContextStr); err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}
	if autoContextStr != "" {
		for _, pattern := range strings.Split(autoContextStr, ",") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				tmpl.AutoContext = append(tmpl.AutoContext, pattern)
			}
		}
	}

	// Save the template
	if err := lib.SaveTemplate(tmpl); err != nil {
		term.OutputErrorAndExit("Error saving template: %v", err)
	}

	fmt.Println()
	fmt.Printf("✅ Template '%s' created successfully!\n", color.New(color.Bold, term.ColorHiGreen).Sprint(tmpl.Name))
	fmt.Println()
	term.PrintCmds("", "templates use "+tmpl.Id, "templates show "+tmpl.Id)
}

// templatesDeleteCmd deletes a user template
var templatesDeleteCmd = &cobra.Command{
	Use:     "delete <template-name>",
	Aliases: []string{"rm"},
	Short:   "Delete a user template",
	Args:    cobra.ExactArgs(1),
	Run:     templatesDelete,
}

func templatesDelete(cmd *cobra.Command, args []string) {
	// Ensure templates directory exists
	if err := lib.EnsureTemplatesDirs(); err != nil {
		term.OutputErrorAndExit("Error initializing templates: %v", err)
	}

	templateName := args[0]

	// Check if it's a built-in template
	tmpl, err := lib.LoadTemplate(templateName)
	if err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}

	if tmpl.BuiltIn {
		term.OutputErrorAndExit("Cannot delete built-in templates")
	}

	// Confirm deletion
	var confirm bool
	confirmPrompt := &survey.Confirm{
		Message: fmt.Sprintf("Delete template '%s'?", templateName),
		Default: false,
	}
	if err := survey.AskOne(confirmPrompt, &confirm); err != nil {
		term.OutputErrorAndExit("Error: %v", err)
	}

	if !confirm {
		fmt.Println("Cancelled.")
		return
	}

	if err := lib.DeleteTemplate(templateName); err != nil {
		term.OutputErrorAndExit("Error deleting template: %v", err)
	}

	fmt.Printf("✅ Template '%s' deleted.\n", templateName)
}
