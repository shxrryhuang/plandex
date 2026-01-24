# Plandex Replay Mode

Replay mode allows you to step through a previously recorded plan execution, inspect what happened at each stage, view file diffs, and optionally re-apply changes in a controlled manner.

## Overview

When you run `plandex tell`, Plandex automatically records the execution session including:
- Model requests and responses
- File changes and diffs
- Build operations
- Context loading
- Subtask execution
- Errors encountered

This recorded session can later be replayed for debugging, understanding, or controlled re-execution.

## Safety Guarantees

**Replay is safe by default.** Here's what this means:

### Read-Only Mode (Default)
- No files are modified
- No API calls are made to AI models
- You can freely inspect and navigate without side effects
- Use this to understand what happened during a run

### Simulate Mode
- Shows what would happen without making changes
- Validates file states against recorded states
- Reports divergences from the original run
- Useful for checking if replay would succeed before applying

### Apply Mode (Explicit Opt-In Required)
- Actually applies file changes
- Requires explicit confirmation
- Still uses recorded model responses (does not re-call AI)
- Use with caution in production environments

## Quick Start

```bash
# List all replay sessions for the current plan
plandex replay list

# Show details of a specific session
plandex replay show abc123

# Start interactive replay (read-only by default)
plandex replay start abc123

# Inspect a specific step
plandex replay inspect abc123 5
```

## CLI Commands

### `plandex replay list`
Lists all recorded sessions for the current plan.

```
📼 Replay Sessions for Plan: my-feature

ID       Branch  Status      Steps  Duration  Started           Prompt
abc123   main    ✓ Completed 42     2m15s     2024-01-15 14:30  Add user authentication...
def456   main    ✗ Failed    28     1m45s     2024-01-15 13:00  Fix login bug...
```

### `plandex replay show <sessionId>`
Shows detailed information about a session including:
- Statistics (steps, tokens, model calls)
- Errors encountered
- Step-by-step summary

### `plandex replay start <sessionId> [flags]`
Starts an interactive replay session.

**Flags:**
- `--mode=<mode>`: Replay mode (read_only, simulate, apply)
- `--auto`: Automatically advance through steps
- `--delay=<ms>`: Delay between steps in auto mode
- `--from=<step>`: Start from specific step number
- `--to=<step>`: End at specific step number
- `--stop-on-diverge`: Stop if file state diverges

### `plandex replay inspect <sessionId> <stepNumber>`
Shows detailed information about a specific step including:
- Step type and status
- Model request/response details
- File diffs
- Context changes

### `plandex replay delete <sessionId>`
Deletes a replay session (with confirmation).

## Interactive Replay Commands

During interactive replay, use these commands:

| Command | Description |
|---------|-------------|
| `n`, `next` | Execute next step |
| `p`, `prev` | Go to previous step (doesn't undo) |
| `j <n>`, `jump <n>` | Jump to step number n |
| `s`, `skip` | Skip current step |
| `i [n]`, `inspect [n]` | Inspect step (current or specified) |
| `d`, `diff` | Show file diff for current step |
| `r`, `run` | Run all remaining steps |
| `status` | Show replay status |
| `q`, `quit` | Exit replay |
| `h`, `help` | Show help |

## Step Types

Replay tracks these step types:

| Type | Icon | Description |
|------|------|-------------|
| model_request | 📤 | Request sent to AI model |
| model_response | 📥 | Response received from AI |
| file_diff | 📝 | Diff computed for file |
| file_write | ✏️ | New file written |
| file_remove | 🗑️ | File deleted |
| file_move | 📦 | File moved/renamed |
| build_start | 🔨 | Build operation started |
| build_complete | ✅ | Build completed successfully |
| build_error | ❌ | Build failed with error |
| context_load | 📚 | Context files loaded |
| subtask_start | ▶️ | Subtask execution started |
| subtask_complete | ✔️ | Subtask completed |
| user_prompt | 💬 | User prompt recorded |
| error | ⚠️ | Error occurred |

## Divergence Detection

When replaying, Plandex can detect divergences between the current file state and the recorded state:

```
⚠️  Divergence: File content differs from recorded state: src/auth.go
   Step 15: content_mismatch
   Expected: (first 200 chars of expected content)
   Actual: (first 200 chars of actual content)
```

Divergences can occur when:
- Files were modified after the original run
- External processes changed files
- Git operations modified the working directory

Use `--stop-on-diverge` to halt replay when divergence is detected.

## Limitations and Caveats

### What Replay Does NOT Do

1. **Re-execute AI model calls**: Replay uses recorded responses, not live AI calls
2. **Undo changes**: Going "back" in replay doesn't undo file changes
3. **Handle external state**: Database changes, network requests, etc. are not replayed
4. **Guarantee identical results**: File state may have changed since recording

### When Replay May Diverge

1. **File modifications**: If files were changed after the original run
2. **Context drift**: If loaded context files changed
3. **Time-sensitive code**: If code depends on timestamps
4. **External dependencies**: If behavior depends on external services
5. **Random/non-deterministic**: If model responses vary (recorded responses are fixed)

### Best Practices

1. **Use read-only mode first** to understand what happened
2. **Check for divergences** in simulate mode before applying
3. **Review diffs carefully** before using apply mode
4. **Back up important files** before applying in production
5. **Use specific step ranges** (`--from`, `--to`) for targeted replay

## Configuration

### Environment Variables

```bash
# Disable automatic recording (not recommended)
export PLANDEX_REPLAY_RECORDING=false

# Enable verbose replay logging
export PLANDEX_REPLAY_DEBUG=true
```

### Recording Settings

Recording is enabled by default for all `plandex tell` executions. Sessions are stored in the plan's data directory and can be managed with the replay commands.

## Examples

### Debugging a Failed Run

```bash
# List sessions to find the failed one
plandex replay list

# Show details to see where it failed
plandex replay show abc123

# Start replay and step through to the error
plandex replay start abc123

# In interactive mode:
replay> jump 25  # Jump near the error
replay> inspect  # See what happened
replay> diff     # View file diff
```

### Re-applying Changes Safely

```bash
# First, simulate to check for divergences
plandex replay start abc123 --mode=simulate
replay> run

# If no divergences, apply with confirmation
plandex replay start abc123 --mode=apply
```

### Inspecting Model Interactions

```bash
# Start replay focused on model steps
plandex replay start abc123

# In interactive mode:
replay> next     # Step through model request
replay> inspect  # See request details (tokens, prompt)
replay> next     # Step to response
replay> inspect  # See response content
```

### Skipping Problematic Steps

```bash
# Skip steps 10-15 that had issues
plandex replay start abc123

replay> jump 10
replay> skip     # Skip step 10
replay> skip     # Skip step 11
# ... or use --skip flag when starting
```

## API Reference

For programmatic access, replay operations are available via the API:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/plans/{planId}/replay/sessions` | GET | List sessions |
| `/plans/{planId}/replay/sessions/{id}` | GET | Get session |
| `/plans/{planId}/replay/sessions/{id}` | DELETE | Delete session |
| `/plans/{planId}/{branch}/replay/start` | POST | Start replay |
| `/plans/{planId}/replay/step` | POST | Execute step |
| `/plans/{planId}/replay/inspect` | GET | Inspect step |
| `/plans/{planId}/replay/pause` | POST | Pause/resume |
| `/plans/{planId}/replay/stop` | POST | Stop replay |
| `/plans/{planId}/replay/status` | GET | Get status |

## Troubleshooting

### "No replay sessions found"
- Recording may be disabled
- The plan hasn't been executed yet
- Sessions may have been deleted

### "Session not found"
- Use the full session ID or ensure the short ID is unique
- The session may have been deleted

### "Divergence detected"
- Files changed since the original run
- Use read-only mode to inspect without concerns
- Consider starting fresh if divergence is significant

### "Failed to execute step"
- Check the error message for details
- Some steps require specific file states
- Consider skipping problematic steps

## Data Storage

Replay sessions are stored in:
```
<plan_dir>/replay/<session_id>.json
```

Each session file contains:
- Session metadata
- All recorded steps
- Initial file snapshots
- Token usage statistics
- Error information
