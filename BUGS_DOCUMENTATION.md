# Plandex Bug Documentation

## Project Overview

### What is Plandex?

**Plandex** is a terminal-based AI development tool designed to handle large, complex coding tasks that span multiple files and require multiple steps. It integrates AI models with a professional development workflow, allowing developers to leverage large language models (like Claude, GPT-4, Gemini, etc.) for code generation while maintaining fine-grained control, reviews, and version control.

### What Problem Does It Solve?

Plandex addresses several key challenges in AI-assisted software development:

- **Large Project Scalability**: Handles projects with 20M+ tokens using tree-sitter project maps and smart context management
- **AI-Assisted Development**: Combines planning, implementation, and debugging in a single workflow
- **Review & Safety**: Maintains a "cumulative diff sandbox" keeping AI changes separate until approved
- **Version Control Integration**: Built-in branching, commits, and rollback capabilities
- **Model Flexibility**: Supports multiple AI model providers (Anthropic, OpenAI, Google, DeepSeek, etc.)
- **Autonomy Control**: Ranges from fully manual to fully automated with granular configuration options

---

## System Design Architecture

The system follows a **client-server architecture** with three main components:

```
┌─────────────────────────────────────────────────────────────┐
│                    PLANDEX ARCHITECTURE                      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────────────┐          ┌───────────────────┐    │
│  │   CLI (Go)           │◄────────►│   Server (Go)     │    │
│  │  /app/cli/           │   HTTP   │  /app/server/     │    │
│  │  • Commands          │          │  • HTTP Handlers  │    │
│  │  • REPL              │          │  • Plan Execution │    │
│  │  • Context Loading   │          │  • DB Models      │    │
│  │  • Plan Management   │          │  • LiteLLM Proxy  │    │
│  └──────────────────────┘          └─────────┬─────────┘    │
│                                               │              │
│  ┌────────────────────────────────────────────┼────────────┐│
│  │                                            │            ││
│  │  ┌──────────┐  ┌──────────────────┐  ┌────▼─────┐      ││
│  │  │PostgreSQL│  │  LiteLLM Proxy   │  │Tree-sitter│     ││
│  │  │ Database │  │  (Python FastAPI)│  │ Syntax   │      ││
│  │  └──────────┘  └──────────────────┘  └──────────┘      ││
│  │                                                         ││
│  │  ┌────────────────────────────────────────────────────┐││
│  │  │  SHARED LIBRARY (/app/shared/)                     │││
│  │  │  • Data Models (Plan, Branch, Context, etc.)       │││
│  │  │  • AI Model Providers & Configurations             │││
│  │  │  • API Request/Response Types                      │││
│  │  │  • Autonomy & Configuration Models                 │││
│  │  └────────────────────────────────────────────────────┘││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

### Key Components

1. **CLI Application** (`/app/cli/`): Terminal interface for users
   - Commands for plan management, context loading, diffs, debugging
   - REPL mode with autocomplete
   - Git integration

2. **Server Application** (`/app/server/`): Backend processing
   - HTTP handlers for all operations
   - Plan execution and model orchestration
   - Database operations with PostgreSQL
   - LiteLLM proxy for unified LLM access

3. **Shared Library** (`/app/shared/`): Common data models and types
   - Data models: Plan, Branch, Context, Project, Org, User
   - AI model configurations and provider definitions
   - Autonomy settings and plan configurations

---

## Bugs That Currently Need to Be Updated

### Bug #1: Missing User Name in Conversation Descriptions

**Location**: `app/server/db/convo_helpers.go:140`

**Description**: When a user sends a prompt, the conversation message description shows a generic "User prompt" instead of including the actual user's name for identification.

**Feature Explanation**: The conversation helper creates descriptive log entries for each message in a plan's conversation. These descriptions are shown in the plan log and help users understand the conversation history.

**Current Code**:
```go
var desc string
if message.Role == openai.ChatMessageRoleUser {
    desc = "💬 User prompt"
    // TODO: add user name
} else {
    desc = "🤖 Plandex reply"
    if message.Stopped {
        desc += " | 🛑 " + color.New(color.FgHiRed).Sprint("stopped")
    }
}
```

**Why This Update is Necessary**:
- In multi-user environments (organizations with multiple team members), it's impossible to distinguish which user submitted which prompt
- Makes audit trails and conversation review more difficult
- Reduces accountability and traceability in collaborative plans

**System Design Area Affected**:
- **Conversation Management System**: The convo_helpers module manages all conversation-related database operations
- **Improvement needed**: Pass user information through the message creation flow and include it in the description string

**Changes Required**:
- Retrieve the user's name/email from the session or message context
- Modify the description format to include user identification: `"💬 User prompt (username)"`
- Ensure user information is available when `AddPlanConvoMessage` is called

---

### Bug #2: Eval Framework Provider Template Lacks Flexibility

**Location**: `test/evals/promptfoo-poc/templates/provider.template.yml:1`

**Description**: The evaluation framework's provider template has limited functionality and doesn't support dynamic creation, multiple tools, or different API provider parameters.

**Feature Explanation**: This template is used by the promptfoo evaluation system to configure how different AI model providers are called during testing. It defines the structure for API requests including temperature, max_tokens, tools, and other parameters.

**Current Code**:
```yaml
# TODO: Add support for more dynamic creation, support for multiple tools, different API providers parameters, etc.

id: {{ .provider_id }}
config:
  temperature: {{ .temperature }}
  max_tokens: {{ .max_tokens }}
  response_format: { type: {{ .response_format }} }
  top_p: {{ .top_p }}
  tools:
    [
      {
        "type": "{{ .tool_type }}",
        "function":
          { "name": "{{ .function_name }}", "parameters": {{ .parameters }} },
      },
    ]
  tool_choice:
    type: "{{ .tool_choice_type }}"
    function:
      name: "{{ .tool_choice_function_name }}"
```

**Why This Update is Necessary**:
- Only supports a single tool configuration; real-world scenarios often require multiple tools
- Hardcoded structure doesn't accommodate provider-specific parameters (e.g., Anthropic's different tool format vs OpenAI's)
- Cannot dynamically generate configurations based on test requirements
- Limits the comprehensiveness of model evaluation testing

**System Design Area Affected**:
- **Testing/Evaluation System**: The evals system is used to verify model performance and reliability
- **Improvement needed**: Make the template more flexible to support diverse testing scenarios

**Changes Required**:
- Add support for an array of tools instead of a single tool
- Add conditional logic for provider-specific parameters
- Support dynamic generation of configuration sections
- Add provider-type detection to apply correct parameter formats

---

### Bug #3: Debug Logging Left in Production Git Operations

**Location**: `app/server/db/git.go:695-783`

**Description**: Extensive debug logging is present in the git operations code, suggesting recurring issues with git state management that haven't been fully resolved.

**Feature Explanation**: The git module handles all version control operations for plans, including creating commits, managing branches, and tracking file changes. This is critical for Plandex's versioning and rollback capabilities.

**Current Code Excerpt**:
```go
log.Println("[DEBUG] --- Git Repo State ---")
// ... extensive debug output for:
// - Current branch
// - Recent commits
// - Git status
// - All refs
// - .git directory contents
// - HEAD file contents
// - Lock file detection (HEAD.lock, index.lock)
log.Println("[DEBUG] --- End Git Repo State ---")
```

**Why This Update is Necessary**:
- Debug logging in production code impacts performance
- Suggests underlying git state issues that may cause occasional errors
- The presence of lock file detection indicates historical issues with concurrent git operations
- Log pollution makes it harder to identify actual issues in production

**System Design Area Affected**:
- **Version Control System**: Handles all git operations for plan versioning
- **Distributed Locking**: The server uses locking mechanisms to prevent concurrent modifications
- **Improvement needed**: Resolve underlying git state issues and remove or gate debug logging

**Changes Required**:
- Make debug logging conditional based on a DEBUG environment variable
- Investigate and fix the root causes that necessitated this debugging
- Implement proper git lock handling instead of just detecting lock files
- Add structured logging instead of println statements

---

### Bug #4: Panic Recovery Patterns Indicate Stability Concerns

**Locations**: Multiple files throughout the server codebase
- `app/server/db/convo_helpers.go`
- `app/server/db/plan_helpers.go`
- `app/server/db/context_helpers_*.go`
- `app/server/handlers/*.go`
- `app/server/db/transactions.go`
- `app/server/db/locks.go`

**Description**: The codebase contains extensive panic recovery mechanisms, indicating past goroutine crashes and stability issues.

**Feature Explanation**: Go's panic mechanism causes a goroutine to crash if unhandled. The server uses deferred recover() calls to catch panics and prevent server-wide crashes.

**Examples of Panic-Prone Operations**:
- `GetPlanConvo` - Retrieving plan conversations
- `AddPlanConvoMessage` - Adding messages to conversations
- `SyncPlanTokens` - Synchronizing token counts
- `DeleteDraftPlans` - Cleaning up draft plans
- Context update operations
- File map operations

**Why This Update is Necessary**:
- While panic recovery prevents crashes, the underlying issues should be fixed
- Indicates potential nil pointer dereferences, slice out-of-bounds errors, or race conditions
- Performance impact from constant panic checking
- May mask bugs that should be fixed at the source

**System Design Area Affected**:
- **Database Operations Layer**: All CRUD operations have panic protection
- **Concurrency Handling**: Multiple goroutines may have race conditions
- **Improvement needed**: Identify and fix root causes of panics rather than just recovering

**Changes Required**:
- Add proper nil checks before dereferencing pointers
- Use proper error handling instead of panicking
- Add race condition detection and fix concurrent access issues
- Implement proper mutex locks where needed
- Add comprehensive unit tests to catch these issues

---

### Bug #5: Windows Line Ending Compatibility (Historical - Recently Fixed)

**Location**: File editing system (server-side)

**Description**: Plan replacements failed on Windows due to mismatched line endings between the expected content and actual file content.

**Feature Explanation**: When Plandex edits files, it needs to match existing content to know where to make changes. The file replacement system compares the "old" content with what's in the file before applying the "new" content.

**Why This Was a Bug**:
- Windows uses CRLF (`\r\n`) line endings while Unix/Mac use LF (`\n`)
- When files were loaded on Windows, line endings differed from what the model expected
- Caused "Plan replacement failed" errors during file edits

**Fix Applied (v2.1.0)**:
- Normalize line endings when loading and comparing files
- Handle both CRLF and LF consistently

**System Design Area This Affected**:
- **File Edit System**: The replacement/diff engine for applying AI-generated changes
- **Context Loading**: How files are read into the plan's context

---

### Bug #6: TypeScript File Mapping Incomplete

**Location**: Tree-sitter file mapping system

**Description**: TypeScript files with directly exported symbols were omitted from map files, and certain TypeScript constructs weren't properly supported.

**Feature Explanation**: Plandex uses tree-sitter to parse code and generate "project maps" - structured representations of code files showing functions, classes, exports, etc. These maps help the AI understand codebase structure.

**Issues Found**:
- `export const foo = 'bar'` (directly exported symbols) were not included
- `declare global` blocks were not mapped
- `namespace` declarations were not mapped
- `enum` blocks were not mapped
- Arrow function handling was inconsistent

**Why This Was a Bug**:
- Missing symbols meant the AI couldn't see important parts of the codebase
- Led to incorrect context selection and missed file references
- TypeScript-heavy projects had incomplete project maps

**Fix Applied (v2.2.0)**:
- Enhanced tree-sitter queries for TypeScript
- Added support for all missing constructs
- Improved arrow function mapping

**System Design Area This Affected**:
- **Project Mapping System**: Tree-sitter based code parsing
- **Context Selection**: How the AI chooses which files to include

---

### Bug #7: Model Provider Fallback Errors

**Location**: Model request handling system

**Description**: Context length exceeded errors (400/413 responses) weren't falling back correctly to models with larger context limits.

**Feature Explanation**: Plandex's model system supports fallbacks - if one model fails (due to context limits, rate limits, etc.), it can automatically try another model. This is configured in model packs.

**Issue**:
- When a model returned a 400 or 413 error for context length exceeded
- The fallback system didn't recognize these as "context exceeded" errors
- Instead of trying a larger-context model, it would just fail

**Fix Applied (v2.1.0)**:
- Improved error parsing to detect context length errors from various formats
- Enhanced fallback logic to trigger on these error types
- Added multiple retry layers

**System Design Area This Affected**:
- **Model Request System**: Handles all LLM API calls
- **Fallback/Retry System**: Manages model failures and alternatives
- **Error Handling**: Recognition and categorization of model errors

---

### Bug #8: Custom Models XML Output Format Tool Calls

**Location**: Custom model handling

**Description**: Tool calls were not supported for custom models using the XML output format.

**Feature Explanation**: Plandex supports two output formats for models:
1. **JSON (function calling)**: Standard OpenAI-style tool use
2. **XML**: Alternative format for models that don't support function calling

Some custom models could use XML format but the tool calling code path assumed JSON format.

**Why This Was a Bug**:
- Custom models configured with `outputFormat: "xml"` couldn't use tools
- Limited which custom models could be used for certain roles
- Caused runtime errors when tool calls were attempted

**Fix Applied (v2.1.0)**: Added proper tool call handling for XML output format models.

**System Design Area This Affected**:
- **Custom Model System**: User-defined model configurations
- **Tool Calling**: How the AI invokes structured operations

---

### Bug #9: Conversation Summary Timestamp Error

**Location**: Conversation summarization system

**Description**: Error "conversation summary timestamp not found in conversation" appeared intermittently.

**Feature Explanation**: Plandex automatically summarizes long conversations to fit within token limits. Summaries are tied to specific timestamps to know which messages they cover.

**Issue**:
- Race condition between summary creation and message retrieval
- Summary referenced a timestamp that no longer existed in the conversation
- Caused errors during plan execution

**Fix Applied (v2.1.7)**: Improved timestamp synchronization between summaries and conversations.

**System Design Area This Affected**:
- **Summarization System**: Automatic conversation compression
- **Token Management**: Keeping conversations within context limits

---

### Bug #10: Potential Crash During Plan Stream

**Location**: Plan streaming/execution

**Description**: Potential panic/crash during plan stream operations.

**Feature Explanation**: When Plandex executes a plan, it streams responses in real-time to the CLI. This involves multiple goroutines handling the stream, parsing output, and updating state.

**Issue**:
- Under certain conditions, the streaming code could panic
- Possibly related to nil pointer access or race conditions
- Could crash the server goroutine handling the request

**Fix Applied (v2.1.7)**:
- Added panic recovery in streaming code paths
- Added better protection against panics/crashes in server goroutines across the board

**System Design Area This Affected**:
- **Streaming System**: Real-time response delivery
- **Goroutine Management**: Concurrent execution handling

---

## Summary of Active TODOs Requiring Updates

| Priority | Location | Issue | Status |
|----------|----------|-------|--------|
| Medium | `convo_helpers.go:140` | Add user name to conversation descriptions | Open |
| Low | `provider.template.yml:1` | Enhance eval framework flexibility | Open |
| Medium | `git.go:695-783` | Remove/gate debug logging, fix underlying issues | Ongoing |
| High | Multiple server files | Address root causes of panic-prone code | Partially addressed |

## Historical Fixes Reference

For reference, the following major bug categories have been addressed in recent versions:

- **v2.2.1**: Custom models and providers issue (#291)
- **v2.2.0**: TypeScript file mapping improvements
- **v2.1.7**: Conversation summary timestamp errors, stream crashes
- **v2.1.6**: Lock acquisition errors, garbled error messages
- **v2.1.0**: Windows line endings, XML tool calls, Anthropic system message errors
- **v2.0.x**: Multiple crash fixes, context auto-load issues, race conditions

---

*This documentation is generated based on code analysis and CHANGELOG review. Last updated based on Plandex v2.2.1.*
