# Plandex — Concurrency Hardening Fork

This fork contains concurrency bug fixes, resource leak repairs, a new CI pipeline for race/deadlock detection, and comprehensive documentation of the server's concurrency model.

## What Changed

### Bug Fixes (8 fixes across 19 files)

| Fix | Description | Files |
|-----|-------------|-------|
| **C1** SafeMap copy | `Items()` returned internal map reference — callers raced with concurrent writes. Now returns a shallow copy. Added `SetIfAbsent` for atomic check-and-set. | `types/safe_map.go` |
| **C2** Atomic needsRollback | `needsRollback` was a bare `bool` written inside goroutines. Changed to `atomic.Bool`. | `db/queue.go` |
| **C3** Activation TOCTOU | Two concurrent `tell` requests could both register an active plan due to a 100ms sleep + non-atomic check. Replaced with `SetIfAbsent` on the plan registry. | `model/plan/activate.go`, `model/plan/state.go` |
| **C4** Buffered StreamDoneCh | `StreamDoneCh` was unbuffered — senders blocked indefinitely if the receiver wasn't ready. Buffered to capacity 1. | `types/active_plan.go` |
| **C5** Lock error messages | Lock exhaustion errors gave no actionable guidance. Now mention `plandex ps`, `plandex stop`, and 60s auto-expiry. | `db/locks.go`, `db/queue.go` |
| **C6** Context cancel leaks | 22 `context.WithCancel` calls across 8 handler files had no `defer cancel()`, leaking contexts on early return. | `handlers/*.go` |
| **C7** Body close ordering | `defer r.Body.Close()` was placed after `io.ReadAll` — body leaked if read failed. Moved defer before read in 10 locations. | `handlers/*.go`, `handlers/branches.go`, `handlers/plans_versions.go` |
| **C8** Heuristic sleeps | Removed 2 unnecessary `time.Sleep(100ms)` calls that added latency without preventing races. | `handlers/plans_changes.go` |

### CI Pipeline

New workflow at `.github/workflows/concurrency-tests.yml` with three parallel jobs:

- **Race Detection** — `go test -race` on `types`, `db`, and `model/plan` packages
- **Stress Tests** — 10-iteration repeated runs of concurrency tests under the race detector
- **Deadlock Detection** — 30-second timeout tests that catch blocked goroutines in timers, locks, and channels

Triggers: push/PR to concurrency-critical paths, weekly schedule (Sunday 3AM UTC), manual dispatch. Auto-creates a GitHub issue on scheduled failure.

### Documentation

| Document | Description |
|----------|-------------|
| [`docs/CONCURRENCY_PATTERNS.md`](docs/CONCURRENCY_PATTERNS.md) | Rewritten from 417 to 925 lines. Covers shared mutable state map, 4-layer concurrency architecture, failure modes, timer/channel patterns, debugging guide, and testing strategy. |
| [`BUGS_DOCUMENTATION.md`](BUGS_DOCUMENTATION.md) | All 16 fixes documented with root cause, code samples, and resolution. |
| [`docs/SYSTEM_DESIGN.md`](docs/SYSTEM_DESIGN.md) | Updated section 10.4 with 4-layer concurrency architecture overview. |

### Tests

- `types/safe_map_test.go` — Updated `Items()` test to verify copy semantics. Added `SetIfAbsent` tests including a 100-goroutine concurrent race (exactly one winner).

## Verification

All changes compile and pass tests with the race detector:

```bash
cd app/server
go build ./...
go test -race ./types/... ./db/... ./model/plan/...
```

## Files Changed

```
 .github/workflows/concurrency-tests.yml |  364 +++
 BUGS_DOCUMENTATION.md                   |  119 +
 app/server/db/locks.go                  |    2 ~
 app/server/db/queue.go                  |    9 ~
 app/server/handlers/branches.go         |   11 ~
 app/server/handlers/context_helper.go   |    1 +
 app/server/handlers/plans_changes.go    |   14 ~
 app/server/handlers/plans_context.go    |   10 ~
 app/server/handlers/plans_convo.go      |    2 +
 app/server/handlers/plans_exec.go       |   21 ~
 app/server/handlers/plans_versions.go   |    4 ~
 app/server/handlers/settings.go         |    2 +
 app/server/model/plan/activate.go       |   28 ~
 app/server/model/plan/state.go          |   11 ~
 app/server/types/active_plan.go         |   13 ~
 app/server/types/safe_map.go            |   22 ~
 app/server/types/safe_map_test.go       |   75 ~
 docs/CONCURRENCY_PATTERNS.md            |  985 ~
 docs/SYSTEM_DESIGN.md                   |   11 ~
 19 files changed, 1405 insertions(+), 299 deletions(-)
```

---

<h1 align="center">
 <a href="https://plandex.ai">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="images/plandex-logo-dark-v2.png"/>
    <source media="(prefers-color-scheme: light)" srcset="images/plandex-logo-light-v2.png"/>
    <img width="400" src="images/plandex-logo-dark-bg-v2.png"/>
 </a>
 <br />
</h1>
<br />

<div align="center">

<p align="center">
  <!-- Call to Action Links -->
  <a href="#install">
    <b>30-Second Install</b>
  </a>
   ·
  <a href="https://plandex.ai">
    <b>Website</b>
  </a>
   ·
  <a href="https://docs.plandex.ai/">
    <b>Docs</b>
  </a>
   ·
  <a href="#examples-">
    <b>Examples</b>
  </a>
   ·
  <a href="https://docs.plandex.ai/hosting/self-hosting/local-mode-quickstart">
    <b>Local Self-Hosted Mode</b>
  </a>
</p>

<br>

[![Discord](https://img.shields.io/discord/1214825831973785600.svg?style=flat&logo=discord&label=Discord&refresh=1)](https://discord.gg/plandex-ai)
[![GitHub Repo stars](https://img.shields.io/github/stars/plandex-ai/plandex?style=social)](https://github.com/plandex-ai/plandex)
[![Twitter Follow](https://img.shields.io/twitter/follow/PlandexAI?style=social)](https://twitter.com/PlandexAI)

</div>

<p align="center">
  <!-- Badges -->
<a href="https://github.com/plandex-ai/plandex/pulls"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen.svg" alt="PRs Welcome" /></a> <a href="https://github.com/plandex-ai/plandex/releases?q=cli"><img src="https://img.shields.io/github/v/release/plandex-ai/plandex?filter=cli*" alt="Release" /></a>
<a href="https://github.com/plandex-ai/plandex/releases?q=server"><img src="https://img.shields.io/github/v/release/plandex-ai/plandex?filter=server*" alt="Release" /></a>

  <!-- <a href="https://github.com/your_username/your_project/issues">
    <img src="https://img.shields.io/github/issues-closed/your_username/your_project.svg" alt="Issues Closed" />
  </a> -->

</p>

<br />

<div align="center">
<a href="https://trendshift.io/repositories/8994" target="_blank"><img src="https://trendshift.io/api/badge/repositories/8994" alt="plandex-ai%2Fplandex | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<br>

<h1 align="center" >
  An AI coding agent designed for large tasks and real world projects.<br/><br/>
</h1>

<!-- <h2 align="center">
  Designed for large tasks and real world projects.<br/><br/>
  </h2> -->
  <br/>

<div align="center">
  <a href="https://www.youtube.com/watch?v=SFSu2vNmlLk">
    <img src="images/plandex-v2-yt.png" alt="Plandex v2 Demo Video" width="800">
  </a>
</div>

<br/>

💻  Plandex is a terminal-based AI development tool that can **plan and execute** large coding tasks that span many steps and touch dozens of files. It can handle up to 2M tokens of context directly (~100k per file), and can index directories with 20M tokens or more using tree-sitter project maps.

🔬  **A cumulative diff review sandbox** keeps AI-generated changes separate from your project files until they are ready to go. Command execution is controlled so you can easily roll back and debug. Plandex helps you get the most out of AI without leaving behind a mess in your project.

🧠  **Combine the best models** from Anthropic, OpenAI, Google, and open source providers to build entire features and apps with a robust terminal-based workflow.

🚀  Plandex is capable of <strong>full autonomy</strong>—it can load relevant files, plan and implement changes, execute commands, and automatically debug—but it's also highly flexible and configurable, giving developers fine-grained control and a step-by-step review process when needed.

💪  Plandex is designed to be resilient to <strong>large projects and files</strong>. If you've found that others tools struggle once your project gets past a certain size or the changes are too complex, give Plandex a shot.

## Smart context management that works in big projects

- 🐘 **2M token effective context window** with default model pack. Plandex loads only what's needed for each step.

- 🗄️ **Reliable in large projects and files.** Easily generate, review, revise, and apply changes spanning dozens of files.

- 🗺️ **Fast project map generation** and syntax validation with tree-sitter. Supports 30+ languages.

- 💰 **Context caching** is used across the board for OpenAI, Anthropic, and Google models, reducing costs and latency.

## Tight control or full autonomy—it's up to you

- 🚦 **Configurable autonomy:** go from full auto mode to fine-grained control depending on the task.

- 🐞 **Automated debugging** of terminal commands (like builds, linters, tests, deployments, and scripts). If you have Chrome installed, you can also automatically debug browser applications.

## Tools that help you get production-ready results

- 💬 **A project-aware chat mode** that helps you flesh out ideas before moving to implementation. Also great for asking questions and learning about a codebase.

- 🧠 **Easily try + combine models** from multiple providers. Curated model packs offer different tradeoffs of capability, cost, and speed, as well as open source and provider-specific packs.

- 🛡️ **Reliable file edits** that prioritize correctness. While most edits are quick and cheap, Plandex validates both syntax and logic as needed, with multiple fallback layers when there are problems.

- 🔀 **Full-fledged version control** for every update to the plan, including branches for exploring multiple paths or comparing different models.

- 📂 **Git integration** with commit message generation and optional automatic commits.

## Dev-friendly, easy to install

- 🧑‍💻 **REPL mode** with fuzzy auto-complete for commands and file loading. Just run `plandex` in any project to get started.

- 🛠️ **CLI interface** for scripting or piping data into context.

- 📦 **One-line, zero dependency CLI install**. Dockerized local mode for easily self-hosting the server. Cloud-hosting options for extra reliability and convenience.

## Workflow  🔄

<img src="images/plandex-workflow.png" alt="Plandex workflow" width="100%"/>

## Examples  🎥

  <br/>

<div align="center">
  <a href="https://www.youtube.com/watch?v=g-_76U_nK0Y">
    <img src="images/plandex-browser-debug-yt.png" alt="Plandex Browser Debugging Example" width="800">
  </a>
</div>

<br/>

## Install  📥

```bash
curl -sL https://plandex.ai/install.sh | bash
```

**Note:** Windows is supported via [WSL](https://learn.microsoft.com/en-us/windows/wsl/install). Plandex only works correctly on Windows in the WSL shell. It doesn't work in the Windows CMD prompt or PowerShell.

[More installation options.](https://docs.plandex.ai/install)

## Hosting  ⚖️

| Option                     | Description                                                                                                                                                                                                                                                                                                                                                 |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Plandex Cloud**          | Winding down as of 10/3/2025 and no longer accepting new users. <a href="https://plandex.ai/blog/winding-down">Learn more.</a>                                                                                                                                                                                                                              |
| **Self-hosted/Local Mode** | • Run Plandex locally with Docker or host on your own server.<br/>• Use your own [OpenRouter.ai](https://openrouter.ai) key (or [other model provider](https://docs.plandex.ai/models/model-providers) accounts and API keys).<br/>• Follow the [local-mode quickstart](https://docs.plandex.ai/hosting/self-hosting/local-mode-quickstart) to get started. |

## Provider keys  🔑

<!-- If you're going with a 'BYO API Key' option above (whether cloud or self-hosted), you'll need to set API keys for the [model providers](https://docs.plandex.ai/models/model-providers) you're using: -->

```bash
export OPENROUTER_API_KEY=... # if using OpenRouter.ai
```

<br/>

## Claude Pro/Max subscription  🖇️

If you have a Claude Pro or Max subscription, Plandex can use it when calling Anthropic models. You'll be asked if you want to connect a subscription the first time you run Plandex.

<br/>

## Get started  🚀

First, `cd` into a **project directory** where you want to get something done or chat about the project. Make a new directory first with `mkdir your-project-dir` if you're starting on a new project.

```bash
cd your-project-dir
```

For a new project, you might also want to initialize a git repo. Plandex doesn't require that your project is in a git repo, but it does integrate well with git if you use it.

```bash
git init
```

Now start the Plandex REPL in your project:

```bash
plandex
```

or for short:

```bash
pdx
```

<!-- ☁️ _If you're using Plandex Cloud, you'll be prompted at this point to start a trial._

Then just give the REPL help text a quick read, and you're ready go. The REPL starts in _chat mode_ by default, which is good for fleshing out ideas before moving to implementation. Once the task is clear, Plandex will prompt you to switch to _tell mode_ to make a detailed plan and start writing code. -->

<br/>

## Docs  🛠️

### [👉  Full documentation.](https://docs.plandex.ai/)

<br/>

## Discussion and discord  💬

Please feel free to give your feedback, ask questions, report a bug, or just hang out:

- [Discord](https://discord.gg/plandex-ai)
- [Discussions](https://github.com/plandex-ai/plandex/discussions)
- [Issues](https://github.com/plandex-ai/plandex/issues)

## Follow and subscribe

- [Follow @PlandexAI](https://x.com/PlandexAI)
- [Follow @Danenania](https://x.com/Danenania) (Plandex's creator)
- [Subscribe on YouTube](https://x.com/PlandexAI)

<br/>

## Contributors  👥

⭐️  Please star, fork, explore, and contribute to Plandex. There's a lot of work to do and so much that can be improved.

[Here's an overview on setting up a development environment.](https://docs.plandex.ai/development)
