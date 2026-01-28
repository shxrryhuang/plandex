# Plandex System Design Document

**Version:** 1.5
**Last Updated:** January 2026 (Added Preflight Validation Phase — execution-readiness gate; resolved all high-priority error-scan gaps)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture Diagram](#2-architecture-diagram)
3. [Component Architecture](#3-component-architecture)
4. [Data Models](#4-data-models)
5. [API Specification](#5-api-specification)
6. [Data Flows](#6-data-flows)
7. [Database Design](#7-database-design)
8. [Security Architecture](#8-security-architecture)
9. [Deployment Architecture](#9-deployment-architecture)
10. [Performance Considerations](#10-performance-considerations)
11. [Recovery & Resilience System](#11-recovery--resilience-system)
12. [Startup & Provider Validation](#12-startup--provider-validation)
13. [Progress Reporting System](#13-progress-reporting-system)
14. [Error Handling & Retry Strategy](#14-error-handling--retry-strategy)
15. [Atomic Patch Application](#15-atomic-patch-application)
16. [Preflight Validation Phase](#16-preflight-validation-phase)

---

## 1. Overview

### 1.1 What is Plandex?

Plandex is an AI-powered coding assistant that helps developers plan and implement code changes through natural language conversations. It provides a CLI interface that communicates with a backend server, which orchestrates AI model interactions and manages plan state.

### 1.2 Technology Stack

| Layer | Technology |
|-------|------------|
| CLI | Go + Cobra Framework |
| Server | Go + Chi Router |
| Database | PostgreSQL 16 |
| AI Proxy | Python + LiteLLM |
| Container | Docker + Docker Compose |

### 1.3 Supported AI Providers

- OpenAI (GPT-4, GPT-4o, o1, o3)
- Anthropic (Claude 3.5, Claude 4)
- Google (Gemini 2.0, Vertex AI)
- Azure OpenAI
- AWS Bedrock
- Ollama (local models)
- OpenRouter

---

## 2. Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                USER LAYER                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│    ┌──────────────┐         ┌──────────────┐         ┌──────────────┐      │
│    │   Terminal   │         │     IDE      │         │   Scripts    │      │
│    └──────┬───────┘         └──────┬───────┘         └──────┬───────┘      │
│           │                        │                        │               │
│           └────────────────────────┼────────────────────────┘               │
│                                    ▼                                        │
│                          ┌─────────────────┐                                │
│                          │   Plandex CLI   │                                │
│                          │    (Go/Cobra)   │                                │
│                          └────────┬────────┘                                │
│                                   │                                         │
└───────────────────────────────────┼─────────────────────────────────────────┘
                                    │ HTTPS/SSE
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              SERVER LAYER                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│    ┌────────────────────────────────────────────────────────────────────┐   │
│    │                        Plandex Server (Go)                         │   │
│    ├────────────────────────────────────────────────────────────────────┤   │
│    │                                                                    │   │
│    │  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐ │   │
│    │  │   Handlers   │  │   Middleware │  │     Route Management     │ │   │
│    │  │  (28 files)  │  │  Auth/CORS   │  │    (Chi Router)          │ │   │
│    │  └──────┬───────┘  └──────────────┘  └──────────────────────────┘ │   │
│    │         │                                                          │   │
│    │  ┌──────┴───────────────────────────────────────────────────────┐ │   │
│    │  │                    Plan Execution Engine                      │ │   │
│    │  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────────┐ │ │   │
│    │  │  │ Tell Exec   │ │ Build Exec  │ │ Stream Processor        │ │ │   │
│    │  │  │ (messages)  │ │ (changes)   │ │ (real-time output)      │ │ │   │
│    │  │  └─────────────┘ └─────────────┘ └─────────────────────────┘ │ │   │
│    │  └──────────────────────────┬───────────────────────────────────┘ │   │
│    │                             │                                      │   │
│    │  ┌──────────────────────────┴───────────────────────────────────┐ │   │
│    │  │                    Model Client Layer                         │ │   │
│    │  │  ┌─────────────────────────────────────────────────────────┐ │ │   │
│    │  │  │           Retry & Safety Guard                           │ │ │   │
│    │  │  │  RetryConfig · OperationSafety · RetryContext            │ │ │   │
│    │  │  │  withStreamingRetries → backoff → journal → registry     │ │ │   │
│    │  │  └──────────────────────────┬──────────────────────────────┘ │ │   │
│    │  │                             │                                 │ │   │
│    │  │  ┌──────────────────────────┴──────────────────────────────┐ │ │   │
│    │  │  │              LiteLLM Proxy (Python)                     │ │ │   │
│    │  │  │  Unified interface to multiple AI providers             │ │ │   │
│    │  │  └─────────────────────────────────────────────────────────┘ │ │   │
│    │  └──────────────────────────────────────────────────────────────┘ │   │
│    │                                                                    │   │
│    └────────────────────────────────┬───────────────────────────────────┘   │
│                                     │                                       │
└─────────────────────────────────────┼───────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              DATA LAYER                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│    ┌────────────────────────┐         ┌────────────────────────┐           │
│    │   PostgreSQL 16        │         │   File Storage         │           │
│    │                        │         │                        │           │
│    │  - Users & Orgs        │         │  - Plan files          │           │
│    │  - Plans & Branches    │         │  - Context cache       │           │
│    │  - Conversations       │         │  - Build artifacts     │           │
│    │  - Context metadata    │         │                        │           │
│    │  - Build results       │         │                        │           │
│    └────────────────────────┘         └────────────────────────┘           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
                                      │
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           AI PROVIDER LAYER                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐         │
│  │  OpenAI  │ │Anthropic │ │  Google  │ │  Azure   │ │  Ollama  │         │
│  │  GPT-4o  │ │  Claude  │ │  Gemini  │ │ OpenAI   │ │  Local   │         │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 3. Component Architecture

### 3.1 CLI (`/app/cli/`)

The CLI provides the user-facing interface for interacting with Plandex.

```
app/cli/
├── main.go                 # Entry point
├── cmd/                    # 55 command implementations
│   ├── plans.go           # Plan management
│   ├── tell.go            # Send messages
│   ├── build.go           # Build changes
│   ├── apply.go           # Apply to files
│   ├── context_load.go    # Load context files
│   └── ...
├── api/                    # HTTP client
│   ├── clients.go         # Client configuration
│   └── methods.go         # 100+ API methods
├── lib/                    # Shared CLI logic
│   ├── apply.go           # File application
│   ├── context_load.go    # Context loading
│   └── ...
├── auth/                   # Authentication
├── fs/                     # File system operations
└── term/                   # Terminal UI utilities
```

**Key Commands:**

| Command | Description |
|---------|-------------|
| `plandex new` | Create a new plan |
| `plandex tell` | Send a message to the AI |
| `plandex build` | Build pending changes |
| `plandex apply` | Apply changes to project files |
| `plandex load` | Load context files |
| `plandex diffs` | View pending changes |
| `plandex rewind` | Revert to previous state |

### 3.2 Server (`/app/server/`)

The server handles API requests, manages state, and orchestrates AI interactions.

```
app/server/
├── main.go                 # Entry point
├── routes/
│   └── routes.go          # Route definitions (40+ endpoints)
├── handlers/               # 28 handler files
│   ├── accounts.go        # Authentication
│   ├── plans_crud.go      # Plan CRUD
│   ├── plans_exec.go      # Plan execution
│   ├── plans_context.go   # Context management
│   └── ...
├── db/                     # Database layer
│   ├── db.go              # Connection management
│   ├── data_models.go     # Schema models
│   ├── plan_helpers.go    # Plan operations
│   └── migrations/        # 50+ SQL migrations
├── model/                  # AI model integration & resilience
│   ├── circuit_breaker.go # Provider health state machine
│   ├── dead_letter_queue.go # Failed operation storage
│   ├── graceful_degradation.go # Adaptive quality reduction
│   ├── health_check.go    # Proactive provider monitoring
│   ├── stream_recovery.go # Partial stream tracking
│   ├── client.go          # Client management
│   ├── client_stream.go   # Streaming responses
│   └── plan/              # Plan execution engine (36 files)
│       ├── tell_exec.go
│       ├── build_exec.go
│       └── tell_stream_processor.go
├── syntax/                 # Code analysis (30+ languages)
└── utils/                  # Utilities
```

### 3.3 Shared Library (`/app/shared/`)

Common types shared between CLI and server.

```
app/shared/
├── data_models.go            # Core API types
├── ai_models_available.go    # Available models
├── ai_models_providers.go    # Provider configs
├── ai_models_errors.go       # Model error types (+ ProviderFailureType field)
├── plan_config.go            # Plan settings
├── context.go                # Context types
├── req_res.go                # Request/response wrappers
│
├── # Startup & Provider Validation
├── validation.go             # Validation framework (types, FormatCLI, helpers)
├── validation_test.go        # 18 unit tests
│
├── # Recovery & Resilience System
├── provider_failures.go      # Provider failure classification & per-type RetryStrategy
├── retry_config.go           # Configurable retry policy (env overrides, backoff, jitter)
├── operation_safety.go       # OperationSafety enum + IsOperationSafe() idempotency guard
├── retry_context.go          # RetryContext — per-attempt tracking, journal bridge
├── file_transaction.go       # Transactional file operations (ACID-like)
├── resume_algorithm.go       # Safe resume from checkpoints
├── error_report.go           # Comprehensive error reporting
├── error_registry.go         # Persistent error store (StoreWithContext bridge)
├── unrecoverable_errors.go   # Unrecoverable edge cases + DetectUnrecoverableCondition
├── run_journal.go            # Execution journal (retry/circuit/fallback events, checkpoints)
├── replay_types.go           # Replay mode types
│
├── # Progress Reporting
├── progress.go               # Core types: Step, StepState, StepKind, ProgressReport
│
├── # Error Handling & Retry
├── retry_policy.go           # Configurable retry with exponential backoff
├── idempotency.go            # Deduplication across retries
└── ai_models_errors.go       # ModelError ↔ ProviderFailure bridge + fallback routing
```

---

## 4. Data Models

### 4.1 Core Entities

```go
// Organization - top-level tenant
type Org struct {
    Id                 string
    Name               string
    Domain             string
    OwnerId            string
    IsTrial            bool
    AutoAddDomainUsers bool
}

// User - authenticated user
type User struct {
    Id                string
    Name              string
    Email             string
    DefaultPlanConfig *PlanConfig
    NumNonDraftPlans  int
}

// Plan - container for AI-assisted coding session
type Plan struct {
    Id             string
    OrgId          string
    OwnerId        string
    ProjectId      string
    Name           string
    Status         PlanStatus
    TotalReplies   int
    ActiveBranches int
    PlanConfig     *PlanConfig
}

// Branch - represents a plan branch (like git branches)
type Branch struct {
    Id             string
    PlanId         string
    ParentBranchId *string
    Name           string
    Status         PlanStatus
    ContextTokens  int
    ConvoTokens    int
}

// Context - file or data loaded into plan
type Context struct {
    Id        string
    BranchId  string
    Type      ContextType  // file, url, note, directory, image
    Name      string
    Body      string
    NumTokens int
}

// ConvoMessage - conversation message
type ConvoMessage struct {
    Id          string
    BranchId    string
    Role        string  // user, assistant, system
    Message     string
    BuildStatus *string
}
```

### 4.2 Plan Status Lifecycle

```
┌─────────────────────────────────────────────────────────┐
│                    Plan Status Flow                      │
├─────────────────────────────────────────────────────────┤
│                                                         │
│   ┌──────────┐     tell      ┌──────────┐              │
│   │  draft   │ ───────────▶  │ replying │              │
│   └──────────┘               └────┬─────┘              │
│        │                          │                     │
│        │                          ▼                     │
│        │                    ┌──────────┐                │
│        │                    │ building │                │
│        │                    └────┬─────┘                │
│        │                         │                      │
│        │            ┌────────────┼────────────┐         │
│        │            ▼            ▼            ▼         │
│        │      ┌──────────┐ ┌──────────┐ ┌──────────┐   │
│        │      │  ready   │ │  error   │ │ stopped  │   │
│        │      └────┬─────┘ └──────────┘ └──────────┘   │
│        │           │                                    │
│        │           ▼                                    │
│        │      ┌──────────┐                              │
│        └─────▶│ finished │                              │
│               └──────────┘                              │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 4.3 Context Types

| Type | Description |
|------|-------------|
| `file` | Source code file |
| `url` | Web page content |
| `note` | User-provided text |
| `directory` | Directory tree structure |
| `image` | Image for vision models |
| `piped` | Piped input data |

---

## 5. API Specification

### 5.1 Authentication Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/accounts/email_verifications` | Start email verification |
| POST | `/accounts/email_verifications/check_pin` | Verify PIN code |
| POST | `/accounts/sign_in` | Sign in user |
| POST | `/accounts/sign_out` | Sign out user |

### 5.2 Plan Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/plans` | List all plans |
| POST | `/projects/{projectId}/plans` | Create new plan |
| GET | `/plans/{planId}` | Get plan details |
| DELETE | `/plans/{planId}` | Delete plan |
| PATCH | `/plans/{planId}/rename` | Rename plan |
| PATCH | `/plans/{planId}/archive` | Archive plan |

### 5.3 Plan Execution (Streaming)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/plans/{planId}/{branch}/tell` | Send message (SSE) |
| PATCH | `/plans/{planId}/{branch}/build` | Build changes (SSE) |
| PATCH | `/plans/{planId}/{branch}/connect` | Resume stream (SSE) |
| DELETE | `/plans/{planId}/{branch}/stop` | Stop execution |

### 5.4 Context Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/plans/{planId}/{branch}/context` | List context |
| POST | `/plans/{planId}/{branch}/context` | Load context |
| PUT | `/plans/{planId}/{branch}/context` | Update context |
| DELETE | `/plans/{planId}/{branch}/context` | Remove context |

### 5.5 Changes & History

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/plans/{planId}/{branch}/diffs` | Get pending diffs |
| GET | `/plans/{planId}/{branch}/convo` | Get conversation |
| PATCH | `/plans/{planId}/{branch}/apply` | Apply changes |
| PATCH | `/plans/{planId}/{branch}/rewind` | Rewind to state |
| PATCH | `/plans/{planId}/{branch}/reject_all` | Reject all changes |

---

## 6. Data Flows

### 6.1 Tell & Build Flow

```
┌────────────────────────────────────────────────────────────────────────┐
│                         Tell & Build Flow                               │
└────────────────────────────────────────────────────────────────────────┘

User: plandex tell "add login feature"
         │
         ▼
┌─────────────────┐
│   CLI: tell     │
│   command       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Startup        │  ← Synchronous checks: home dir, auth files,
│  Validation     │    PLANDEX_ENV, debug level, trace file path
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Provider       │  ← Deferred checks: API keys, credential files,
│  Validation     │    provider compatibility, AWS profile reachability
└────────┬────────┘
         │ POST /plans/{id}/{branch}/tell
         ▼
┌─────────────────────────────────────────────────────────┐
│                    Server                                │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  1. Load plan state from database                       │
│  2. Load context (files, URLs, notes)                   │
│  3. Load conversation history                           │
│  4. Build system prompt                                 │
│                                                         │
│  ┌───────────────────────────────────────────────────┐ │
│  │          Prepare AI Request                        │ │
│  │  ┌─────────────────────────────────────────────┐  │ │
│  │  │ System: "You are Plandex, an AI assistant"  │  │ │
│  │  │ Context: [file1.ts, file2.ts, ...]          │  │ │
│  │  │ History: [prev messages...]                 │  │ │
│  │  │ User: "add login feature"                   │  │ │
│  │  └─────────────────────────────────────────────┘  │ │
│  └───────────────────────────────────────────────────┘ │
│                         │                               │
│                         ▼                               │
│  ┌───────────────────────────────────────────────────┐ │
│  │       Retry & Safety Guard                         │ │
│  │  - Check OperationSafety (Safe/Conditional/       │ │
│  │    Irreversible) before each attempt               │ │
│  │  - withStreamingRetries[T] wraps the call         │ │
│  │  - On failure: classify → GetStrategy →           │ │
│  │    ComputeBackoffDelay → RecordAttempt             │ │
│  │  - On give-up: DetectUnrecoverableCondition →     │ │
│  │    StoreWithContext → RecordRetryOutcome           │ │
│  └───────────────────────────────────────────────────┘ │
│                         │                               │
│                         ▼                               │
│  ┌───────────────────────────────────────────────────┐ │
│  │              LiteLLM Proxy                         │ │
│  │  Routes to appropriate AI provider                 │ │
│  └───────────────────────────────────────────────────┘ │
│                         │                               │
│             ◄───────────┘  (retry on failure)           │
│             │                                           │
│                         ▼                               │
│  ┌───────────────────────────────────────────────────┐ │
│  │          Stream Processor                          │ │
│  │  - Parse streaming response                        │ │
│  │  - Extract structured edits (XML)                  │ │
│  │  - Identify file changes                           │ │
│  │  - Store pending changes                           │ │
│  └───────────────────────────────────────────────────┘ │
│                         │                               │
│                         │ SSE (Server-Sent Events)      │
└─────────────────────────┼───────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                    CLI Display                           │
│  - Real-time streaming output                           │
│  - Shows AI response as it generates                    │
│  - Highlights proposed code changes                     │
└─────────────────────────────────────────────────────────┘
```

### 6.2 Apply Changes Flow

```
User: plandex apply
         │
         ▼
┌─────────────────┐
│  CLI: apply     │
│  command        │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────┐
│           Apply Process (Atomic Transaction)             │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  1. Get pending changes from server                     │
│     GET /plans/{id}/{branch}/diffs                      │
│                                                         │
│  2. FileTransaction.Begin()                             │
│     ┌─────────────────────────────────────────────────┐│
│     │ For each file change:                           ││
│     │  a. Snapshot original content (rollback source) ││
│     │  b. Stage operation (Create/Modify/Delete)      ││
│     │  c. WAL entry written (crash safety)            ││
│     └─────────────────────────────────────────────────┘│
│                                                         │
│  3. FileTransaction.ApplyAll()                          │
│     ┌─────────────────────────────────────────────────┐│
│     │  • Operations applied sequentially              ││
│     │  • PatchStatusReporter fires per-file events    ││
│     │  • On failure → Rollback() restores all files   ││
│     └─────────────────────────────────────────────────┘│
│                                                         │
│  4. (Optional) Workspace isolation path:                │
│     ┌─────────────────────────────────────────────────┐│
│     │  • ApplyFilesToWorkspace() redirects to          ││
│     │    isolated workspace directory                  ││
│     │  • Manager.Commit() pushes to project           ││
│     │    atomically after all writes succeed           ││
│     └─────────────────────────────────────────────────┘│
│                                                         │
│  5. Post-apply script (optional):                       │
│     ┌─────────────────────────────────────────────────┐│
│     │  • Execute _apply.sh with signal handling       ││
│     │  • Script failure does NOT roll back files      ││
│     └─────────────────────────────────────────────────┘│
│                                                         │
│  6. Git integration (optional):                         │
│     ┌─────────────────────────────────────────────────┐│
│     │ a. Stage changed files                          ││
│     │ b. Generate commit message (AI)                 ││
│     │ c. Create commit                                ││
│     └─────────────────────────────────────────────────┘│
│                                                         │
│  7. Notify server of applied changes                    │
│     PATCH /plans/{id}/{branch}/apply                    │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## 7. Database Design

### 7.1 Entity Relationship Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Database Entity Relationships                         │
└─────────────────────────────────────────────────────────────────────────┘

┌──────────┐       ┌──────────┐       ┌──────────┐
│   Org    │ 1───* │ OrgUser  │ *───1 │   User   │
│          │       │          │       │          │
│ id       │       │ org_id   │       │ id       │
│ name     │       │ user_id  │       │ name     │
│ owner_id │───────│ role_id  │       │ email    │
└────┬─────┘       └──────────┘       └──────────┘
     │
     │ 1
     │
     │ *
┌────┴─────┐       ┌──────────┐
│ Project  │ 1───* │   Plan   │
│          │       │          │
│ id       │       │ id       │
│ org_id   │       │ project_ │
│ name     │       │ owner_id │
└──────────┘       │ name     │
                   │ status   │
                   └────┬─────┘
                        │
                        │ 1
                        │
                        │ *
                   ┌────┴─────┐
                   │  Branch  │
                   │          │
                   │ id       │
                   │ plan_id  │
                   │ parent_  │
                   │ status   │
                   └────┬─────┘
                        │
          ┌─────────────┼─────────────┐
          │             │             │
          ▼ *           ▼ *           ▼ *
    ┌──────────┐  ┌──────────┐  ┌──────────┐
    │ Context  │  │  Convo   │  │ PlanBuild│
    │          │  │ Message  │  │          │
    │ id       │  │          │  │ id       │
    │ branch_id│  │ id       │  │ branch_id│
    │ type     │  │ branch_id│  │ file_path│
    │ body     │  │ role     │  │ error    │
    └──────────┘  │ message  │  └──────────┘
                  └──────────┘
```

### 7.2 Key Tables

```sql
-- Core authentication
CREATE TABLE users (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    domain VARCHAR(255),
    default_plan_config JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Multi-tenancy
CREATE TABLE orgs (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    domain VARCHAR(255),
    owner_id UUID REFERENCES users(id),
    is_trial BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Plans and branches
CREATE TABLE plans (
    id UUID PRIMARY KEY,
    org_id UUID REFERENCES orgs(id),
    project_id UUID REFERENCES projects(id),
    owner_id UUID REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'draft',
    total_replies INTEGER DEFAULT 0,
    plan_config JSONB,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE branches (
    id UUID PRIMARY KEY,
    plan_id UUID REFERENCES plans(id),
    parent_branch_id UUID REFERENCES branches(id),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'draft',
    context_tokens INTEGER DEFAULT 0,
    convo_tokens INTEGER DEFAULT 0
);

-- Context and conversation
CREATE TABLE contexts (
    id UUID PRIMARY KEY,
    branch_id UUID REFERENCES branches(id),
    context_type VARCHAR(50) NOT NULL,
    name VARCHAR(255),
    body TEXT,
    num_tokens INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE convo_messages (
    id UUID PRIMARY KEY,
    branch_id UUID REFERENCES branches(id),
    role VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    build_status VARCHAR(50),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Build results
CREATE TABLE plan_builds (
    id UUID PRIMARY KEY,
    plan_id UUID REFERENCES plans(id),
    convo_message_id UUID REFERENCES convo_messages(id),
    file_path VARCHAR(500),
    error TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
```

### 7.3 Connection Configuration

| Setting | Production | Development |
|---------|------------|-------------|
| Max Open Connections | 50 | 10 |
| Max Idle Connections | 20 | 5 |
| Query Timeout | 30s | 30s |
| Lock Timeout | 4s | 4s |
| Idle Session Timeout | 90s | 90s |

---

## 8. Security Architecture

### 8.1 Authentication Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Authentication Flow                               │
└─────────────────────────────────────────────────────────────────────┘

1. Email Verification
   User ──▶ POST /accounts/email_verifications ──▶ Email sent with PIN

2. PIN Verification
   User ──▶ POST /accounts/email_verifications/check_pin ──▶ PIN validated

3. Sign In
   User ──▶ POST /accounts/sign_in ──▶ JWT token returned

4. Authenticated Requests
   User ──▶ Request with "Authorization: Bearer {token}" ──▶ Server validates
```

### 8.2 Authorization Model

```
┌─────────────────────────────────────────────────────────────────────┐
│                    RBAC Structure                                    │
└─────────────────────────────────────────────────────────────────────┘

Org Owner
    │
    ├── Full org management
    ├── User management
    ├── Billing management
    └── All plan access

Org Admin
    │
    ├── User management
    ├── Invite users
    └── All plan access

Org Member
    │
    ├── Create own plans
    ├── Access shared plans
    └── Limited org visibility
```

### 8.3 Security Measures

| Layer | Measure |
|-------|---------|
| Transport | HTTPS/TLS encryption |
| Authentication | JWT tokens with expiration |
| Authorization | RBAC per organization |
| Database | Hashed tokens, parameterized queries |
| API | Rate limiting, request size limits |
| Input | Validation, sanitization |

---

## 9. Deployment Architecture

### 9.1 Docker Compose (Local/Self-Hosted)

```yaml
services:
  plandex-postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: plandex
      POSTGRES_USER: plandex
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - plandex-db:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U plandex"]
    deploy:
      resources:
        limits:
          memory: 512M

  plandex-server:
    build: ./app/server
    ports:
      - "8099:8099"
    environment:
      DATABASE_URL: postgres://plandex:${DB_PASSWORD}@plandex-postgres:5432/plandex
      GOENV: production
    depends_on:
      plandex-postgres:
        condition: service_healthy
    deploy:
      resources:
        limits:
          memory: 2G
```

### 9.2 Environment Variables

**Server:**
```bash
DATABASE_URL=postgres://user:pass@host:5432/plandex
GOENV=production
PORT=8099
IS_CLOUD=false
LOCAL_MODE=true
PLANDEX_BASE_DIR=/var/plandex
```

**CLI:**
```bash
PLANDEX_API_HOST=https://api.plandex.ai
PLANDEX_ENV=production
```

**AI Providers:**
```bash
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
GOOGLE_APPLICATION_CREDENTIALS=/path/to/creds.json
```

### 9.3 Deployment Modes

| Mode | Description | Use Case |
|------|-------------|----------|
| Cloud | Multi-tenant SaaS | Teams, collaboration |
| Self-Hosted | Single instance | Enterprise, privacy |
| Local | Development | Testing, development |

---

## 10. Performance Considerations

### 10.1 Streaming Architecture

- **Server-Sent Events (SSE)** for real-time AI responses
- Non-blocking I/O for concurrent plan execution
- Connection keep-alive for long-running operations

### 10.2 Caching Strategy

| Cache Type | Purpose | TTL |
|------------|---------|-----|
| File map cache | Tree-sitter analysis results | Session |
| Context cache | Loaded file contents | Plan lifetime |
| Token count cache | Avoid re-counting | Until modified |

### 10.3 Token Management

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Context Window Management                         │
└─────────────────────────────────────────────────────────────────────┘

Model Context Limit: ~128K tokens (varies by model)

Allocation:
├── System Prompt:     ~2K tokens
├── Context Files:     ~80K tokens (configurable)
├── Conversation:      ~40K tokens (with summarization)
└── Response Buffer:   ~6K tokens

Summarization triggered when conversation exceeds threshold
```

### 10.4 Concurrency Control

- Database locks for plan execution
- Heartbeat system for active plans (3s interval, 60s timeout)
- Graceful shutdown with 60s timeout for active operations
- Queue-based operation batching (same-branch reads batched, writes exclusive)
- DB-lock exponential backoff retry (6 attempts, 300ms initial, 2x factor, ±30% jitter)
  *(distinct from provider retry — see §11 for AI provider retry policy via `RetryConfig`)*

**Diagnostic Tools:**
- `plandex doctor` - Check for stale locks and system health
- `plandex doctor --fix` - Automatically clean up stale locks
- `plandex ps` - View active operations

> **For detailed concurrency documentation, see:**
> - [CONCURRENCY_SAFETY.md](./CONCURRENCY_SAFETY.md) - Failure modes, debugging, testing
> - [CONCURRENCY_PATTERNS.md](./CONCURRENCY_PATTERNS.md) - Code-level patterns

---

## 11. Recovery & Resilience System

### 11.1 Overview

The recovery system provides fault tolerance for AI-assisted coding operations through:
- **Provider Failure Classification** - Distinguishing retryable vs non-retryable errors
- **Configurable Retry Policy** - Per-failure-type exponential backoff with jitter, provider Retry-After respect, and env-driven overrides (`RetryConfig`)
- **Operation Safety Guard** - Prevents retrying irreversible side effects; classifies every operation as Safe / Conditional / Irreversible (`OperationSafety`)
- **Structured Retry Tracking** - Every attempt recorded with timing, failure type, strategy, and fallback info (`RetryContext`)
- **Transactional File Operations** - ACID-like guarantees with rollback support
- **Resume Algorithm** - Safe continuation from checkpoints
- **Comprehensive Error Reporting** - Root cause, context, and recovery guidance; unrecoverable detection now invoked from the live retry loop

### 11.2 Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       RECOVERY & RESILIENCE SYSTEM                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Provider Failure Classification                    │   │
│  │                      (provider_failures.go)                          │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────┐  │   │
│  │  │  Retryable  │  │Non-Retryable│  │     Retry Strategy          │  │   │
│  │  │ rate_limit  │  │quota_exhaust│  │ • Exponential backoff       │  │   │
│  │  │ overloaded  │  │auth_invalid │  │ • Respect Retry-After       │  │   │
│  │  │server_error │  │content_policy│  │ • Provider fallback        │  │   │
│  │  │  timeout    │  │context_long │  │ • Max attempts              │  │   │
│  │  └─────────────┘  └─────────────┘  └─────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │              Configurable Retry Policy + Safety Guard                 │   │
│  │          (retry_config.go, operation_safety.go, retry_context.go)    │   │
│  │  ┌──────────────┐  ┌────────────────┐  ┌──────────────────────────┐ │   │
│  │  │ RetryConfig  │  │OperationSafety│  │     RetryContext         │ │   │
│  │  │ • Per-type   │  │ • Safe         │  │ • []RetryAttempt         │ │   │
│  │  │   overrides  │  │ • Conditional  │  │ • Attempt timing/delay   │ │   │
│  │  │ • Env vars   │  │ • Irreversible │  │ • Fallback tracking      │ │   │
│  │  │ • Backoff    │  │ • IsOp Safe()  │  │ • FinalizeWithError()    │ │   │
│  │  │   math       │  │ • ClassifyOp() │  │ • Journal + Registry     │ │   │
│  │  └──────────────┘  └────────────────┘  └──────────────────────────┘ │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                   Transactional File Operations                       │   │
│  │                      (file_transaction.go)                           │   │
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌─────────────────┐   │   │
│  │  │ Snapshots │  │Operations │  │Checkpoints│  │   WAL/Journal   │   │   │
│  │  │ Original  │  │ Create    │  │ Named     │  │ Crash recovery  │   │   │
│  │  │ content   │  │ Modify    │  │ restore   │  │ Write-ahead log │   │   │
│  │  │ captured  │  │ Delete    │  │ points    │  │                 │   │   │
│  │  └───────────┘  │ Rename    │  └───────────┘  └─────────────────┘   │   │
│  │                 └───────────┘                                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                        Resume Algorithm                               │   │
│  │                     (resume_algorithm.go)                            │   │
│  │                                                                       │   │
│  │  1. Select Checkpoint ──▶ 2. Validate Journal ──▶ 3. Validate Files │   │
│  │          │                                                │          │   │
│  │          ▼                                                ▼          │   │
│  │  4. Handle Divergences ◀── 5. Dry Run Check ◀── 6. Create Backup   │   │
│  │          │                                                           │   │
│  │          ▼                                                           │   │
│  │  7. Restore Files ──────▶ 8. Update Journal State                   │   │
│  │                                                                       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      Error Reporting System                           │   │
│  │              (error_report.go, unrecoverable_errors.go)              │   │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────────┐  │   │
│  │  │   Root Cause    │  │  Step Context   │  │  Recovery Action    │  │   │
│  │  │ • Category      │  │ • Plan/Entry    │  │ • Auto-recoverable? │  │   │
│  │  │ • Type/Code     │  │ • Phase         │  │ • Retry strategy    │  │   │
│  │  │ • HTTP code     │  │ • Transaction   │  │ • Manual actions    │  │   │
│  │  │ • Provider      │  │ • Model context │  │ • Alternatives      │  │   │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 11.3 Provider Failure Classification

Every classified error now carries a `ProviderFailureType` field that maps
directly into `GetRetryStrategy()`, so the retry loop can look up the
correct backoff parameters without additional logic.

**Retryable Failures:**
| Type | HTTP Code | Strategy | Default Attempts |
|------|-----------|----------|-----------------|
| Rate Limit | 429 | Exponential backoff ×2, respect Retry-After | 5 |
| Overloaded | 503, 529 | Backoff ×2 + jitter, tries fallback | 5 |
| Server Error | 500, 502 | Backoff ×2 + jitter, tries fallback | 3 |
| Timeout | 504 | Immediate retry | 2 |
| Connection Error | — | Short backoff ×1.5 | 3 |
| Stream Interrupted | — | Backoff ×1.5 | 2 |
| Cache Error | — | Single retry, no delay | 1 |
| Provider Unavailable | 502 | Backoff ×2, tries fallback | 3 |

**Non-Retryable Failures:**
| Type | HTTP Code | Required Action |
|------|-----------|-----------------|
| Auth Invalid | 401 | Fix API credentials |
| Quota Exhausted | 402, 429* | Add credits/upgrade |
| Context Too Long | 400, 413 | Reduce input size or switch model |
| Content Policy | 400 | Modify content |
| Model Not Found | 404 | Use valid model ID |
| Unsupported Feature | 501 | Change approach |

*Note: HTTP 429 is rate-limited (retryable) when the message indicates per-minute throttling, and quota-exhausted (non-retryable) when it indicates a billing cap. The classifier checks message keywords to distinguish them.*

### 11.4 Transactional File Operations

```go
// Transaction lifecycle
tx := NewFileTransaction(planId, branch, workDir)
tx.Begin()

tx.ModifyFile("src/auth.go", newContent)  // Snapshot captured
tx.CreateFile("src/login.go", content)    // Tracked for rollback

checkpoint := tx.CreateCheckpoint("after_auth", "Completed auth changes")

// On provider failure:
if err := tx.RollbackOnProviderFailure(failure); err != nil {
    // Files restored to pre-transaction state
}

// Or on success:
tx.Commit()
```

### 11.5 Resume Algorithm

The 8-step safe resume process:

1. **Select Checkpoint** - By name, latest, or last known good
2. **Validate Journal** - Verify integrity hash matches
3. **Validate Files** - Compare current state vs checkpoint
4. **Handle Divergences** - Report or repair (file_missing, hash_mismatch, file_extra)
5. **Dry Run Check** - Optional validation-only mode
6. **Create Backup** - Optional safety backup before resume
7. **Restore Files** - From checkpoint.FileContents if requested
8. **Update State** - Mark entries for replay

### 11.6 Unrecoverable Edge Cases

The system explicitly identifies scenarios where automatic recovery is impossible:

| Category | Examples | User Communication |
|----------|----------|-------------------|
| Provider Limit | Quota exhausted, context too long | Manual action required with specific commands |
| Authentication | Invalid API key, permission denied | Credential update instructions |
| Data Loss | Checkpoint lost, journal corrupted | Partial recovery options, backup restoration |
| External State | Concurrent modification, file conflicts | Merge guidance, divergence resolution |
| System Resource | Disk full, permission errors | System administration actions |

### 11.7 Idempotency & Safety Guarantees

Retries are guaranteed to be safe and produce equivalent results through:

1. **Operation Safety Classification** - Every operation is tagged Safe, Conditional, or Irreversible by `ClassifyOperation()`.  Irreversible operations (shell exec, external API writes) are blocked from retry unless `PLANDEX_RETRY_IRREVERSIBLE=true` is set.
2. **Snapshot-based rollback** - Original content captured once, restored before retry.  Conditional operations (file writes) can be rolled back to a checkpoint before re-execution.
3. **Journal entry deduplication** - Status tracking prevents re-execution of completed steps
4. **Hash-based verification** - File states validated against expected hashes
5. **Operation sequencing** - Strict ordering with rollback in reverse order
6. **Retry attempt tracing** - `RetryContext` records every attempt with timing, strategy, and fallback info so the system can distinguish "retried and succeeded" from "retried and exhausted"

### 11.8 Key Files

```
app/shared/
├── provider_failures.go          # Provider failure classification & GetRetryStrategy()
├── provider_failures_test.go     # 18 test cases
├── retry_config.go               # Configurable retry policy (RetryConfig, ComputeBackoffDelay)
├── retry_config_test.go          # 12 test cases — backoff math, jitter, env loading
├── operation_safety.go           # Idempotency guard (OperationSafety, ClassifyOperation)
├── operation_safety_test.go      # 3 test cases — safety levels, classification
├── retry_context.go              # Structured retry tracking (RetryContext, RetryAttempt)
├── retry_context_test.go         # 13 test cases — attempt lifecycle, CanRetry, fallback
├── ai_models_errors.go           # ModelError type (now includes ProviderFailureType)
├── file_transaction.go           # Transactional file operations
├── file_transaction_test.go      # 21 test cases
├── resume_algorithm.go           # Safe resume from checkpoint
├── resume_algorithm_test.go      # 18 test cases
├── error_report.go               # Comprehensive error reporting
├── error_report_test.go          # 16 test cases
├── error_registry.go             # Persistent error storage (StoreWithContext added)
├── unrecoverable_errors.go       # Edge case documentation & detection
├── unrecoverable_errors_test.go  # 16 test cases
└── run_journal.go                # Execution journal (RecordRetryAttempt/Outcome added)

app/server/model/
├── model_error.go                # HTTP classifier — populates ProviderFailureType
├── client.go                     # Constants + defaultRetryConfig init
└── client_stream.go              # withStreamingRetries — wired to RetryConfig + RetryContext
```

### 11.9 Test Coverage

| Component | Tests | Coverage |
|-----------|-------|----------|
| Provider Failures | 18 | Classification, retry strategies, real-world scenarios |
| Retry Config | 12 | Backoff growth, jitter bounds, max clamping, env loading, Retry-After cap |
| Operation Safety | 3 | Safety strings, IsOperationSafe for all combos, ClassifyOperation |
| Retry Context | 13 | Attempt lifecycle, CanRetry caps, fallback recording, timing, Summary |
| File Transaction | 21 | CRUD, rollback, checkpoints, provider failure handling |
| Resume Algorithm | 18 | Checkpoint selection, validation, repair actions |
| Error Reporting | 16 | Formatting, context, recovery actions |
| Unrecoverable Errors | 16 | Edge cases, user communication |
| **Total** | **117** | Full recovery system coverage |

---

## 12. Startup & Provider Validation

### 12.1 Overview

Plandex performs three-phase configuration validation before any plan execution begins. This catches common misconfigurations (missing API keys, invalid paths, incompatible provider combinations, unwritable project roots) early and surfaces clear, actionable error messages — both in CLI output and in the error registry for run journals. The third phase (Preflight) is documented in detail in [§16](#16-preflight-validation-phase).

### 12.2 Three-Phase Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       VALIDATION PIPELINE                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                  Phase 1: Synchronous (Startup)                       │   │
│  │                  startup_validation.go — RunStartupValidation()       │   │
│  │                                                                       │   │
│  │  Runs once per CLI invocation, before any network call:               │   │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌─────────────────┐    │   │
│  │  │  Home Directory  │  │   Auth/Config    │  │  Environment    │    │   │
│  │  │  • Exists        │  │   Files          │  │  Variables      │    │   │
│  │  │  • Is directory  │  │   • Valid JSON   │  │  • PLANDEX_ENV  │    │   │
│  │  │  • Is writable   │  │   • Not empty    │  │  • DEBUG_LEVEL  │    │   │
│  │  └──────────────────┘  └──────────────────┘  │  • TRACE_FILE   │    │   │
│  │                                               └─────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                     │                                        │
│                                     ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                  Phase 2: Deferred (Provider-Scoped)                  │   │
│  │                  startup_validation.go — RunProviderValidation()       │   │
│  │                                                                       │   │
│  │  Runs after plan settings are loaded, before the first API call:      │   │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌─────────────────┐    │   │
│  │  │  API Key Vars    │  │  Credential      │  │  Provider       │    │   │
│  │  │  • Required keys │  │  Files           │  │  Compatibility  │    │   │
│  │  │  • Extra auth    │  │  • File paths    │  │  • Dual         │    │   │
│  │  │    vars          │  │    exist         │  │    Anthropic    │    │   │
│  │  │  • AWS profile   │  │  • Claude Max    │  │    warning      │    │   │
│  │  │    reachability  │  │    creds         │  │                 │    │   │
│  │  └──────────────────┘  └──────────────────┘  └─────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                     │                                        │
│                                     ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                  Phase 3: Preflight (Execution Readiness)             │   │
│  │                  preflight_validation.go — MustRunPreflightChecks()   │   │
│  │                                                                       │   │
│  │  Runs after provider validation, before any LLM call or file write:  │   │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌─────────────────┐    │   │
│  │  │  Project Root    │  │   Shell &        │  │  Config Files   │    │   │
│  │  │  • Exists        │  │   Output Dirs    │  │  • projects-v2  │    │   │
│  │  │  • Is directory  │  │   • SHELL set    │  │  • settings-v2  │    │   │
│  │  │  • Is writable   │  │   • REPL dir     │  │  • Valid JSON   │    │   │
│  │  └──────────────────┘  │   • API host URL │  └─────────────────┘    │   │
│  │                         └──────────────────┘                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 12.3 Validation Severity

| Severity | Behaviour |
|----------|-----------|
| `fatal` | Halts execution immediately; user must fix before proceeding |
| `warning` | Printed to stderr; does not block execution |

### 12.4 Validation Categories

Errors are grouped by category in CLI output for clarity:

| Category | What it covers |
|----------|----------------|
| `filesystem` | Home directory, auth files, credential file paths, trace file parent |
| `environment` | `PLANDEX_ENV`, `PLANDEX_DEBUG_LEVEL`, `PLANDEX_TRACE_FILE` |
| `authentication` | Claude Max sign-in state, credential file presence |
| `provider` | API keys, extra auth vars, provider compatibility |
| `configuration` | General config issues |

### 12.5 Integration Points

- **`main.go`** — calls `RunStartupValidation()` for all commands except `version`, `browser`, `help`, `sign-in`, `connect`.
- **`cmd/tell.go`, `cmd/build.go`, `cmd/continue.go`** — each calls `MustRunDeferredValidation()` immediately before `MustVerifyAuthVars`.
- **Error Registry** — `ToErrorReport()` persists failed validation results so they appear in the run journal for later diagnosis.

### 12.6 Key Files

```
app/shared/
├── validation.go             # Framework: types, FormatCLI, ToErrorReport, helpers
└── validation_test.go        # 18 unit tests

app/cli/lib/
└── startup_validation.go     # RunStartupValidation, RunProviderValidation,
                              #   MustRunDeferredValidation, per-provider checks
```

### 12.7 Test Coverage

| Component | Tests | What is covered |
|-----------|-------|-----------------|
| ValidationResult ops | 6 | New, Add (fatal/warn/nil), Merge, MergeNil |
| ValidationError interface | 1 | Error() string format |
| FormatCLI output | 4 | Empty, fatal header, warning header, category grouping |
| ToErrorReport | 2 | Passed (nil), with fatals (full report) |
| ValidateEnvVarSet | 3 | Present, whitespace-only, empty |
| ValidateProviderCompatibility | 4 | No conflict, dual Anthropic, single Claude Max, empty |
| ValidateFilePath | 2 | Empty path, non-empty path |
| Timestamp freshness | 1 | Timestamp set between before/after creation |
| **Total** | **18** | Full validation framework coverage |

### 12.8 CI Pipeline

A dedicated GitHub Actions workflow (`.github/workflows/validation-tests.yml`) runs independently of the main CI suite. It triggers only on changes to validation source files or on a daily 2:30 AM UTC schedule.

| Job | What it does |
|-----|--------------|
| `format` | `gofmt` on validation source files |
| `vet` | `go vet` on `shared` and `cli` modules |
| `unit-tests` | All 23 tests with `-race`, coverage profile uploaded to Codecov |
| `build` | Full CLI compile + grep-verification that all integration entry points exist |
| `summary` | Aggregated pass/fail status table |
| `notify-on-failure` | Auto-opens a labeled GitHub issue on scheduled-run failure |

A local mirror (`test/run_validation_tests.sh`) sources the existing `test/test_utils.sh` conventions and supports running all checks or individual targets (`format`, `vet`, `unit`, `build`).

### 12.9 Error Message Scan — Known Gaps

A post-implementation scan of the CLI identified error conditions not yet surfaced by the validation system. The five high-priority items and the project-root writability check have since been resolved by the Preflight Validation Phase (§16). Remaining gaps are listed below.

**Resolved by Preflight Phase (§16):**

| Error condition | Preflight check | Severity |
|-----------------|-----------------|----------|
| `SHELL` env var empty and `/bin/bash` fallback missing | `checkShellAvailable` | Fatal |
| `PLANDEX_API_HOST` set to an invalid URL/hostname | `checkAPIHostValid` | Warn |
| `PLANDEX_REPL_OUTPUT_FILE` parent directory does not exist | `checkReplOutputDir` | Warn |
| `projects-v2.json` contains malformed JSON | `checkProjectsFileValid` | Fatal |
| `settings-v2.json` contains malformed JSON | `checkSettingsFileValid` | Fatal |
| Project root directory is not writable | `checkProjectRootWritable` | Fatal |

**Remaining — deferred validation candidates (medium priority):**

| Error condition | Source location | Why it matters |
|-----------------|-----------------|----------------|
| `GOOGLE_APPLICATION_CREDENTIALS` file contains invalid JSON | `lib/startup_validation.go:372` | File exists but content is unparseable |
| `PLANDEX_AWS_PROFILE` names a profile not present in config files | `lib/startup_validation.go:342` | Profile reachability check stops at file existence |
| `PLANDEX_COLUMNS` is not a valid integer | `term/utils.go:88` | Silently ignored — user never learns their value was rejected |
| `PLANDEX_STREAM_FOREGROUND_COLOR` is an invalid ANSI code | `term/utils.go:125` | Silently ignored until rendering |

**Requires runtime context (lower priority):**

| Error condition | Source location | Why deferred |
|-----------------|-----------------|--------------|
| Custom editor command not on PATH | `lib/editor.go:75` | Only known after user selects editor |
| `less` pager command not available | `term/utils.go:49` | Only needed during output display |

---

## 13. Progress Reporting System

### 13.1 Overview

The progress reporting system provides real-time, step-level visibility into plan execution. It tracks every phase (initializing → planning → describing → building → applying → validating → completed) and every step within those phases (LLM calls, file reads/writes, tool executions, context loading). Output adapts automatically to the environment: animated progress bars and spinners in a TTY, structured log lines otherwise.

### 13.2 Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       PROGRESS REPORTING SYSTEM                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    Core Data Model (shared/progress.go)               │   │
│  │                                                                       │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌──────────────┐  │   │
│  │  │  StepState │  │  StepKind  │  │    Step    │  │ProgressReport│  │   │
│  │  │ running    │  │ llm_call   │  │ id, kind   │  │ phase        │  │   │
│  │  │ completed  │  │ file_read  │  │ state      │  │ steps[]      │  │   │
│  │  │ failed     │  │ file_write │  │ progress   │  │ counts       │  │   │
│  │  │ waiting    │  │ tool_exec  │  │ children[] │  │ metrics      │  │   │
│  │  │ stalled    │  │ validation │  │ timing     │  │ warnings     │  │   │
│  │  └────────────┘  └────────────┘  └────────────┘  └──────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                     │                                        │
│              ┌──────────────────────┼──────────────────────┐                │
│              ▼                      ▼                      ▼                │
│  ┌───────────────────┐  ┌───────────────────┐  ┌─────────────────────┐    │
│  │   Tracker         │  │ StreamAdapter     │  │  Pipeline           │    │
│  │ (tracker.go)      │  │ (stream_adapter)  │  │ (pipeline/pipeline) │    │
│  │                   │  │                   │  │                     │    │
│  │ • Event loop      │  │ • Bridges legacy  │  │ • Standalone        │    │
│  │ • 100ms batching  │  │   StreamMessage   │  │   orchestrator      │    │
│  │ • Stall detection │  │   → Tracker       │  │ • Callback-driven   │    │
│  │ • Phase callbacks │  │ • Maps events to  │  │ • UUID step IDs     │    │
│  │ • Async channel   │  │   phases/steps    │  │ • Per-kind stall    │    │
│  └────────┬──────────┘  └───────────────────┘  │   thresholds        │    │
│           │                                     └─────────────────────┘    │
│           ▼                                                                  │
│  ┌───────────────────┐                                                      │
│  │   Renderer        │                                                      │
│  │ (renderer.go)     │                                                      │
│  │                   │                                                      │
│  │ • TTY: ANSI       │                                                      │
│  │   progress bars,  │                                                      │
│  │   spinners, color │                                                      │
│  │ • Non-TTY:        │                                                      │
│  │   structured logs │                                                      │
│  └───────────────────┘                                                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 13.3 Step Lifecycle

Steps progress through states split into two guarantee tiers:

| Tier | States | Meaning |
|------|--------|---------|
| Guaranteed | `completed`, `failed`, `skipped` | Terminal — once set, never changes |
| Best-effort | `running`, `waiting`, `stalled` | Live indicators; may transition to a guaranteed state |

### 13.4 Stall Detection

Each `StepKind` has an expected-duration threshold. A background ticker compares elapsed time and marks the step as stalled when exceeded:

| Kind | Threshold |
|------|-----------|
| `llm_call` | 60 s |
| `tool_exec` | 60 s |
| `file_build` | 120 s |
| `validation` | 30 s |
| `context` | 10 s |
| `user_input` | 0 s (immediate) |

### 13.5 Rendering Strategy

| Environment | Output style |
|-------------|--------------|
| TTY | Phase header with elapsed time; animated progress bar (█/░); spinning current step; last 3 completed steps; warning indicators |
| Non-TTY | One structured log line per event: `[TIME] [STATE] PHASE > ICON STEP (detail) [progress] ERROR` |

### 13.6 StreamAdapter Bridge

The `StreamAdapter` translates the backend's `StreamMessage` protocol into `Tracker` calls without modifying either side:

| StreamMessage type | Progress action |
|--------------------|--------------------|
| `Start` | Phase → initializing or building |
| `Reply` | Phase → planning; start/update LLM step |
| `Describing` | Phase → describing; complete LLM step |
| `BuildInfo` | Track per-path file build; complete/skip on finish |
| `LoadContext` | Context loading step with file count |
| `Finished` | Phase → completed; complete pending steps |
| `Error` | Phase → failed; fail running steps |
| `Aborted` | Phase → stopped; skip running steps |

### 13.7 Key Files

```
app/shared/
└── progress.go                  # Core types: ProgressStep (Kind, State, CompletedAt),
                                 #   StepState, StepKind, ProgressPhase, ProgressReport,
                                 #   ProgressMessage. Also contains a simpler Step type
                                 #   used by the newer Progress snapshot API.

app/cli/progress/
├── tracker.go                   # State coordinator, event loop, stall detection
├── renderer.go                  # TTY/non-TTY output formatting
├── stream_adapter.go            # StreamMessage → Tracker bridge
├── examples_test.go             # Rendered-output examples (uses ProgressStep literals)
└── pipeline/
    ├── pipeline.go              # Standalone callback-driven orchestrator
    ├── pipeline_test.go         # Pipeline unit tests (callbacks typed *ProgressStep)
    └── runner.go                # Visual executor with spinner animation
```

> **Note:** `progress.go` exports two step-like types. The progress reporting
> system (Tracker, Pipeline, StreamAdapter) uses `ProgressStep` — the struct
> with `Kind`, `State`, `CompletedAt`, and `TokensProcessed`. The newer
> `Progress` snapshot API uses the simpler `Step` struct with `Phase`,
> `Status`, and `Confidence`. Tests and callbacks must match the type expected
> by the API they target.

---

## 14. Error Handling & Retry Strategy

### 14.1 Overview

The error handling system provides a multi-layered resilience stack for AI provider interactions. It spans from initial failure classification through adaptive retries, circuit breaking, graceful degradation, dead-lettering, health monitoring, and stream recovery — all coordinated through a unified run journal.

### 14.2 Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     ERROR HANDLING & RETRY STRATEGY                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Layer 1: Failure Classification & Retry                             │   │
│  │                                                                       │   │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐  │   │
│  │  │  RetryPolicy    │    │  RetryState     │    │ IdempotencyMgr  │  │   │
│  │  │ (retry_policy)  │    │  (retry_policy) │    │ (idempotency)   │  │   │
│  │  │                 │    │                 │    │                 │  │   │
│  │  │ • Per-type      │    │ • Attempt log   │    │ • Dedup keys    │  │   │
│  │  │   policies      │    │ • Delay calc    │    │ • File hashes   │  │   │
│  │  │ • Exp. backoff  │    │ • ShouldRetry   │    │ • TTL cleanup   │  │   │
│  │  │ • Jitter        │    │   decision      │    │ • Rollback info │  │   │
│  │  └─────────────────┘    └─────────────────┘    └─────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                     │                                        │
│                                     ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Layer 2: Provider Health & Routing                                   │   │
│  │                                                                       │   │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐  │   │
│  │  │ CircuitBreaker  │    │  HealthCheck    │    │  Degradation    │  │   │
│  │  │ (circuit_br.)   │    │  Manager        │    │  Manager        │  │   │
│  │  │                 │    │ (health_check)  │    │ (graceful_deg.) │  │   │
│  │  │ • Closed →      │    │                 │    │                 │  │   │
│  │  │   Open →        │    │ • Latency       │    │ • 5 levels:     │  │   │
│  │  │   HalfOpen      │    │   tracking      │    │   None→Critical │  │   │
│  │  │ • Per-provider  │    │ • Health score  │    │ • Auto-trigger  │  │   │
│  │  │   state         │    │ • Best provider │    │   from error    │  │   │
│  │  │ • Failure window│    │   selection     │    │   rate          │  │   │
│  │  └─────────────────┘    └─────────────────┘    └─────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                     │                                        │
│                                     ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Layer 3: Recovery & Audit                                            │   │
│  │                                                                       │   │
│  │  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐  │   │
│  │  │  DeadLetter     │    │   RunJournal    │    │  StreamRecovery │  │   │
│  │  │  Queue          │    │  (run_journal)  │    │  Manager        │  │   │
│  │  │ (dead_letter_q) │    │                 │    │ (stream_recov.) │  │   │
│  │  │                 │    │ • Full audit    │    │                 │  │   │
│  │  │ • Stores failed │    │   trail         │    │ • Partial       │  │   │
│  │  │   ops after all │    │ • Retry events  │    │   content       │  │   │
│  │  │   retries done  │    │ • Circuit events│    │   buffering     │  │   │
│  │  │ • Auto-retry    │    │ • Checkpoints   │    │ • Token-based   │  │   │
│  │  │   scheduling    │    │ • Pause/resume  │    │   checkpoints   │  │   │
│  │  └─────────────────┘    └─────────────────┘    └─────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 14.3 Retry Policies

Pre-defined policies for each retryable failure type:

| Policy | Trigger | Max Attempts | Initial Delay | Backoff |
|--------|---------|--------------|---------------|---------|
| `PolicyRateLimit` | HTTP 429 | Provider-specific | Retry-After header | Exponential + jitter |
| `PolicyOverloaded` | HTTP 503, 529 | 5 | 2 s | 2x |
| `PolicyServerError` | HTTP 500, 502 | 3 | 1 s | 2x |
| `PolicyTimeout` | HTTP 504 | 3 | 500 ms | 1.5x |
| `PolicyConnectionError` | Network failure | 4 | 1 s | 2x |
| `PolicyStreamInterrupted` | Mid-stream drop | 3 | 500 ms | 1.5x |

### 14.4 Circuit Breaker State Machine

```
  ┌──────────┐  failures ≥ threshold   ┌──────────┐
  │  Closed  │ ──────────────────────▶ │   Open   │
  │          │ ◀───────────────────── │          │
  └──────────┘   success in HalfOpen  └────┬─────┘
                                           │ timeout
                                           ▼
                                     ┌──────────┐
                                     │ HalfOpen │
                                     │ (probe)  │
                                     └──────────┘
```

Each provider gets its own circuit. Configurable thresholds: failure count, failure rate within a sliding window, and excluded failure types (e.g., auth errors bypass the circuit).

### 14.5 Graceful Degradation Levels

When error rates climb, the system automatically reduces scope to maintain functionality:

| Level | Context | Timeout | Max Retries | Model Preference |
|-------|---------|---------|-------------|------------------|
| None | Full | 1x | Full | Best |
| Light | 90% | 1.2x | Reduced | Best |
| Moderate | 70% | 1.5x | 2 | Faster |
| Heavy | 50% | 2x | 1 | Cheapest |
| Critical | 30% | 3x | 0 | Fastest |

### 14.6 Idempotency Guarantees

The `IdempotencyManager` prevents duplicate side effects across retries:

1. **Key-based tracking** — each operation is assigned a stable idempotency key before the first attempt
2. **Status lifecycle** — Pending → InProgress → Completed/Failed/RolledBack
3. **File change records** — every file modification is logged with before/after hashes for verification
4. **TTL cleanup** — stale records expire automatically; manual `Remove()` for explicit cleanup

### 14.7 Key Files

```
app/shared/
├── retry_policy.go           # Policy definitions, delay calculation, RetryState
├── retry_policy_test.go      # Test coverage
├── retry_config.go           # Per-type policy config, env overrides, backoff math
├── retry_context.go          # RetryContext: per-attempt tracking, FinalizeWithError()
│                             #   calls ErrorReportFromProviderFailure() + DetectUnrecoverableCondition()
├── operation_safety.go       # OperationSafety enum + IsOperationSafe() idempotency guard
├── idempotency.go            # Deduplication, file change records, stats
├── idempotency_test.go       # Test coverage
├── run_journal.go            # Execution log: entries, checkpoints, retry events
└── ai_models_errors.go       # ModelError bridge + fallback routing

app/server/model/
├── circuit_breaker.go        # Per-provider state machine
├── circuit_breaker_test.go
├── dead_letter_queue.go      # Failed operation storage + auto-retry
├── dead_letter_queue_test.go
├── graceful_degradation.go   # Adaptive quality reduction
├── graceful_degradation_test.go
├── health_check.go           # Proactive monitoring + best-provider selection
├── health_check_test.go
├── stream_recovery.go        # Partial stream buffering + checkpoints
└── ERROR_HANDLING.md         # Subsystem design notes
```

---

## 15. Atomic Patch Application

### 15.1 Overview

The patch application system provides two complementary paths for writing plan changes to disk:

1. **Concurrent goroutine path** (`ApplyFiles`) — the current production entry point. Each file write runs in its own goroutine; errors are collected via a buffered channel. A per-file rollback plan (`ToRevert` / `ToRemove`) is returned so the caller can undo changes if a downstream step (e.g., a post-apply script) fails.

2. **Transactional path** (`FileTransaction`) — a stricter, ACID-like engine available for callers that need all-or-nothing semantics. It captures snapshots at `Begin()`, applies operations sequentially, and rolls back the entire set on any failure. A write-ahead log (WAL) provides crash recovery, and named checkpoints allow partial restore.

The workspace isolation layer (`ApplyFilesToWorkspace` / `Manager.Commit`) sits above either path and redirects writes to an isolated directory before committing atomically to the project.

### 15.2 Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       ATOMIC PATCH APPLICATION                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Orchestration Layer                                                  │   │
│  │                                                                       │   │
│  │  ┌────────────────────────┐    ┌────────────────────────────────┐  │   │
│  │  │  apply.go              │    │  workspace_apply.go            │  │   │
│  │  │  (ApplyFiles)          │    │  (ApplyFilesToWorkspace)       │  │   │
│  │  │                        │    │                                │  │   │
│  │  │  • Main apply path     │    │  • Workspace-aware adapter     │  │   │
│  │  │  • Script execution    │    │  • Redirects to isolated dir   │  │   │
│  │  │  • Git commit          │    │  • Atomic metadata updates     │  │   │
│  │  └────────────────────────┘    └────────────────────────────────┘  │   │
│  └──────────────────────────┬──────────────────────────────────────────┘   │
│                              │                                               │
│                              ▼                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Transaction Engine (file_transaction.go)                             │   │
│  │                                                                       │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────────────┐  │   │
│  │  │Snapshots │  │Operations│  │Checkpoints│  │  WAL/Journal       │  │   │
│  │  │Original  │  │Create    │  │Named      │  │  Crash recovery    │  │   │
│  │  │content   │  │Modify    │  │restore    │  │  Write-ahead log   │  │   │
│  │  │captured  │  │Delete    │  │points     │  │  State tracking    │  │   │
│  │  │at Begin()│  │Rename    │  │           │  │                    │  │   │
│  │  └──────────┘  └──────────┘  └──────────┘  └────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                              │                                               │
│                              ▼                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Status & Error Layer                                                 │   │
│  │                                                                       │   │
│  │  ┌────────────────────────┐    ┌────────────────────────────────┐  │   │
│  │  │  PatchStatusReporter   │    │  UnrecoverableErrors           │  │   │
│  │  │  (patch_status.go)     │    │  (unrecoverable_errors.go)     │  │   │
│  │  │                        │    │                                │  │   │
│  │  │  • Per-file events     │    │  • Classifies fatal failures   │  │   │
│  │  │  • Phase transitions   │    │  • User action guidance        │  │   │
│  │  │  • Summary counts      │    │  • Partial recovery options    │  │   │
│  │  └────────────────────────┘    └────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                              │                                               │
│                              ▼                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Workspace Isolation (workspace/manager.go)                           │   │
│  │                                                                       │   │
│  │  Create → Activate → [Apply changes] → Commit / Discard             │   │
│  │                                                                       │   │
│  │  • Isolated directory tree per plan/branch                            │   │
│  │  • Commit uses its own transaction for project writes                 │   │
│  │  • Stale workspace cleanup                                           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 15.3 Transaction Lifecycle

```go
tx := NewFileTransaction(planId, branch, workDir)
tx.Begin()                                         // Lock + WAL open

tx.ModifyFile("src/auth.go", newContent)           // Snapshot captured
tx.CreateFile("src/login.go", content)             // Tracked for rollback
checkpoint := tx.CreateCheckpoint("after_auth")    // Named restore point

tx.ApplyAll()                                      // Sequential apply
// On failure anywhere:
tx.Rollback()                                      // All files restored

// On success:
tx.Commit()                                        // WAL sealed
```

### 15.4 Workspace Isolation Flow

1. **Create** — allocates workspace ID; initializes `files/`, `checkpoints/`, `logs/` directories
2. **Activate** — marks workspace in-use; updates last-accessed time
3. **Apply** — `ApplyFilesToWorkspace()` writes to workspace directory; metadata updates held pending
4. **Commit** — `Manager.Commit()` snapshots project files, applies workspace modifications to project sequentially, rolls back on failure, then updates workspace state
5. **Discard** — marks workspace discarded; optionally deletes directory
6. **Cleanup** — stale workspaces identified by age and state, removed in batches

### 15.5 Key Files

```
app/shared/
├── file_transaction.go          # Transaction engine: snapshots, WAL, checkpoints, rollback
├── patch_status.go              # PatchStatusReporter interface + LoggingReporter
├── patch_apply_test.go          # Transaction integration tests
└── unrecoverable_errors.go      # Fatal error classification with user guidance

app/cli/lib/
├── apply.go                     # ApplyFiles() — concurrent goroutine path with errCh drain
│                                #   and per-file rollback plan; Rollback() restores from plan;
│                                #   script execution, git commit helpers
└── workspace_apply.go           # Workspace-aware apply/rollback adapter

app/cli/workspace/
└── manager.go                   # Workspace lifecycle: create, activate, commit, discard, cleanup
```

> **Integration note:** `ApplyFiles()` currently uses the concurrent goroutine
> path. The `FileTransaction` engine is available for stricter transactional
> semantics (e.g., workspace commits via `Manager.Commit()`). Wiring
> `ApplyFiles` directly into `FileTransaction` is a candidate for a future
> iteration once the transactional path has broader test coverage.

---

## 16. Preflight Validation Phase

### 16.1 Overview

The Preflight phase is the third and final validation gate before plan execution begins. It sits immediately after the Deferred (provider-scoped) checks and immediately before any LLM call, file write, or network request. Its purpose is to surface all execution-readiness failures together so the user can fix everything in one pass — rather than hitting one cryptic mid-execution error, fixing it, and then hitting the next.

All preflight checks are intentionally cheap: no API calls, no file scanning. They target conditions that the earlier error-message scan (§12.9) flagged as high or medium priority.

### 16.2 Execution Position

```
main()
  └─ runStartupChecks()                    ← Phase 1: env / fs / auth
       cmd.Execute()
         └─ tell / build / continue
              ├─ auth.MustResolveAuthWithOrg()
              ├─ lib.MustResolveProject()
              ├─ mustSetPlanExecFlags()
              ├─ lib.MustRunDeferredValidation()   ← Phase 2: provider keys
              ├─ lib.MustRunPreflightChecks()      ← Phase 3: execution readiness
              └─ plan_exec.TellPlan / Build        ← real work begins here
```

`MustRunPreflightChecks` is called identically in `tell.go`, `build.go`, and `continue.go`.

### 16.3 Check Registry

| # | Check function | Category | Severity | What it validates |
|---|----------------|----------|----------|-------------------|
| 1 | `checkProjectRootWritable` | Filesystem | Fatal | `fs.ProjectRoot` exists, is a directory, is writable (temp-file probe) |
| 2 | `checkShellAvailable` | Config | Fatal | `SHELL` env var is set or `/bin/bash` exists as fallback |
| 3 | `checkReplOutputDir` | Filesystem | Warn | `PLANDEX_REPL_OUTPUT_FILE` parent directory exists if env var is set |
| 4 | `checkAPIHostValid` | Config | Warn | `PLANDEX_API_HOST` is a valid URL with host component if set |
| 5 | `checkProjectsFileValid` | Filesystem | Fatal | `projects-v2.json` is valid JSON if present |
| 6 | `checkSettingsFileValid` | Filesystem | Fatal | `settings-v2.json` is valid JSON if present |

### 16.4 Error Reporting

Preflight failures use a dedicated `buildPreflightErrorReport()` helper that tags errors with the `PREFLIGHT_VALIDATION` label — distinct from `STARTUP_VALIDATION` — so the global error registry can distinguish the two phases. The `FormatCLI()` renderer groups errors by category and renders severity badges identically to the other phases.

### 16.5 Key Files

| File | Role |
|------|------|
| `app/cli/lib/preflight_validation.go` | Check registry, `MustRunPreflightChecks()` entry point, `buildPreflightErrorReport()` helper, all 6 check implementations |
| `app/cli/lib/preflight_validation_test.go` | 21 subtests covering every check and edge case |
| `app/shared/validation.go:52` | `PhasePreflight` constant |
| `app/cli/cmd/tell.go:74` | Wire call |
| `app/cli/cmd/build.go:45` | Wire call |
| `app/cli/cmd/continue.go:55` | Wire call |

### 16.6 Test Coverage

| Test | Subtests | Status |
|------|----------|--------|
| `TestCheckProjectRootWritable` | writable passes / missing fails / empty path fails / file-not-dir fails | PASS |
| `TestCheckShellAvailable` | SHELL set passes / unset+bash passes / both missing (skip — requires sandbox) | PASS |
| `TestCheckReplOutputDir` | unset passes / valid parent passes / missing parent fails | PASS |
| `TestCheckAPIHostValid` | unset passes / valid URL passes / invalid string fails / scheme-only fails | PASS |
| `TestCheckProjectsFileValid` | missing passes / valid JSON passes / malformed fails / empty warns | PASS |
| `TestCheckSettingsFileValid` | missing passes / valid JSON passes / malformed fails / empty warns | PASS |
| **Total** | **21 subtests** | **All passing** |

---

## Appendix A: File Reference

```
Key Server Files:
  /app/server/main.go                     Entry point
  /app/server/routes/routes.go            Route definitions
  /app/server/handlers/plans_exec.go      Plan execution (16.7 KB)
  /app/server/db/data_models.go           Database models (28.9 KB)
  /app/server/model/plan/tell_exec.go     Tell execution

Key CLI Files:
  /app/cli/main.go                        Entry point
  /app/cli/api/methods.go                 API methods (76 KB)
  /app/cli/cmd/repl.go                    REPL mode (39 KB)
  /app/cli/lib/context_update.go          Context handling (27.7 KB)

Key Shared Files:
  /app/shared/data_models.go              Core types (15.2 KB)
  /app/shared/ai_models_available.go      Model definitions (29 KB)
  /app/shared/validation.go               Validation framework (types, helpers)
  /app/shared/progress.go                 Progress types (StepState, StepKind, ProgressReport)
  /app/shared/ai_models_errors.go         ModelError + ProviderFailureType + fallback routing
  /app/shared/provider_failures.go        Failure classification + RetryStrategy defs
  /app/shared/retry_policy.go             Retry policies + exponential backoff
  /app/shared/retry_config.go             RetryConfig: per-type policy, env loading, backoff math
  /app/shared/operation_safety.go         OperationSafety enum + IsOperationSafe() guard
  /app/shared/retry_context.go            RetryContext: per-attempt tracking; FinalizeWithError()
                                          calls ErrorReportFromProviderFailure() + journal bridge
  /app/shared/idempotency.go              Deduplication across retries
  /app/shared/file_transaction.go         ACID-like transactional file operations
  /app/shared/patch_status.go             Per-file status event reporting
  /app/shared/error_registry.go           Persistent error store + StoreWithContext()
  /app/shared/run_journal.go              Execution journal: retry/circuit/fallback events, checkpoints
  /app/shared/error_report.go             ErrorReport: root cause + recovery actions
  /app/shared/unrecoverable_errors.go     DetectUnrecoverableCondition() + user templates

Key CLI Validation Files:
  /app/cli/lib/startup_validation.go      Startup + provider validation logic
  /app/cli/lib/preflight_validation.go    Preflight checks + MustRunPreflightChecks() entry point
  /app/cli/lib/preflight_validation_test.go  21 subtests for all 6 preflight checks

Key Progress Files:
  /app/cli/progress/tracker.go            State coordinator + stall detection
  /app/cli/progress/renderer.go           TTY/non-TTY output formatting
  /app/cli/progress/stream_adapter.go     StreamMessage → Tracker bridge

Key Model / Error Handling Files (Server):
  /app/server/model/model_error.go        ClassifyModelError() → ProviderFailureType
  /app/server/model/client_stream.go      withStreamingRetries[T] — live retry loop
  /app/server/model/circuit_breaker.go    Per-provider health state machine
  /app/server/model/dead_letter_queue.go  Failed operation storage + auto-retry
  /app/server/model/graceful_degradation.go  Adaptive quality reduction
  /app/server/model/health_check.go       Proactive monitoring + routing

Key Apply Files:
  /app/cli/lib/apply.go                   ApplyFiles() — concurrent goroutine path + Rollback()
  /app/cli/lib/workspace_apply.go         Workspace-aware apply adapter
  /app/cli/workspace/manager.go           Workspace lifecycle management
```

---

## Appendix B: Glossary

| Term | Definition |
|------|------------|
| Plan | A container for an AI-assisted coding session |
| Branch | A version of a plan (like git branches) |
| Context | Files, URLs, or notes loaded into a plan |
| Tell | Send a message to the AI |
| Build | Execute pending code changes |
| Apply | Write changes to actual project files |
| Rewind | Revert plan to a previous state |

---

*Document generated: January 2026 · Last updated: January 28, 2026 (v1.5)*
