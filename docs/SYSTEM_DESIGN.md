# Plandex System Design Document

**Version:** 1.2
**Last Updated:** January 2026 (Added Startup & Provider Validation, CI Pipeline, Error Scan)

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
├── model/                  # AI model integration
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
├── ai_models_errors.go       # Model error types
├── plan_config.go            # Plan settings
├── context.go                # Context types
├── req_res.go                # Request/response wrappers
│
├── # Startup & Provider Validation
├── validation.go             # Validation framework (types, FormatCLI, helpers)
├── validation_test.go        # 18 unit tests
│
├── # Recovery & Resilience System
├── provider_failures.go      # Provider failure classification
├── file_transaction.go       # Transactional file operations
├── resume_algorithm.go       # Safe resume from checkpoints
├── error_report.go           # Comprehensive error reporting
├── unrecoverable_errors.go   # Unrecoverable edge cases
├── run_journal.go            # Execution journal & checkpoints
└── replay_types.go           # Replay mode types
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
│  │              LiteLLM Proxy                         │ │
│  │  Routes to appropriate AI provider                 │ │
│  └───────────────────────────────────────────────────┘ │
│                         │                               │
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
│                 Apply Process                            │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  1. Get pending changes from server                     │
│     GET /plans/{id}/{branch}/diffs                      │
│                                                         │
│  2. For each file change:                               │
│     ┌─────────────────────────────────────────────────┐│
│     │ a. Read current file from disk                  ││
│     │ b. Apply structured edit                        ││
│     │ c. Write updated file                           ││
│     │ d. Validate syntax (optional)                   ││
│     └─────────────────────────────────────────────────┘│
│                                                         │
│  3. Git integration (optional):                         │
│     ┌─────────────────────────────────────────────────┐│
│     │ a. Stage changed files                          ││
│     │ b. Generate commit message (AI)                 ││
│     │ c. Create commit                                ││
│     └─────────────────────────────────────────────────┘│
│                                                         │
│  4. Notify server of applied changes                    │
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
- Exponential backoff retry (6 attempts, 300ms initial, 2x factor, ±30% jitter)

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
- **Transactional File Operations** - ACID-like guarantees with rollback support
- **Resume Algorithm** - Safe continuation from checkpoints
- **Comprehensive Error Reporting** - Root cause, context, and recovery guidance

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

**Retryable Failures:**
| Type | HTTP Code | Strategy |
|------|-----------|----------|
| Rate Limit | 429 | Exponential backoff, respect Retry-After |
| Overloaded | 503, 529 | Backoff + provider fallback |
| Server Error | 500, 502 | Immediate retry with backoff |
| Timeout | 504 | Immediate retry |

**Non-Retryable Failures:**
| Type | HTTP Code | Required Action |
|------|-----------|-----------------|
| Auth Invalid | 401 | Fix API credentials |
| Quota Exhausted | 402, 429* | Add credits/upgrade |
| Context Too Long | 400, 413 | Reduce input size |
| Content Policy | 400 | Modify content |

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

### 11.7 Idempotency Guarantees

Retries are guaranteed to produce identical results through:

1. **Snapshot-based rollback** - Original content captured once, restored before retry
2. **Journal entry deduplication** - Status tracking prevents re-execution of completed steps
3. **Hash-based verification** - File states validated against expected hashes
4. **Operation sequencing** - Strict ordering with rollback in reverse order

### 11.8 Key Files

```
app/shared/
├── provider_failures.go       # Provider failure classification (900+ lines)
├── provider_failures_test.go  # 18 test cases
├── file_transaction.go        # Transactional file operations (600+ lines)
├── file_transaction_test.go   # 21 test cases
├── resume_algorithm.go        # Safe resume from checkpoint (600+ lines)
├── resume_algorithm_test.go   # 18 test cases
├── error_report.go            # Comprehensive error reporting (500+ lines)
├── error_report_test.go       # 16 test cases
├── unrecoverable_errors.go    # Edge case documentation (600+ lines)
├── unrecoverable_errors_test.go # 16 test cases
└── run_journal.go             # Execution journal with checkpoints
```

### 11.9 Test Coverage

| Component | Tests | Coverage |
|-----------|-------|----------|
| Provider Failures | 18 | Classification, retry strategies, real-world scenarios |
| File Transaction | 21 | CRUD, rollback, checkpoints, provider failure handling |
| Resume Algorithm | 18 | Checkpoint selection, validation, repair actions |
| Error Reporting | 16 | Formatting, context, recovery actions |
| Unrecoverable Errors | 16 | Edge cases, user communication |
| **Total** | **89** | Full recovery system coverage |

---

## 12. Startup & Provider Validation

### 12.1 Overview

Plandex performs two-phase configuration validation before any plan execution begins. This catches common misconfigurations (missing API keys, invalid paths, incompatible provider combinations) early and surfaces clear, actionable error messages — both in CLI output and in the error registry for run journals.

### 12.2 Two-Phase Architecture

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

A post-implementation scan of the CLI identified error conditions not yet surfaced by the two-phase validation system. They are grouped below by whether the framework could catch them now or whether runtime context is required.

**Catchable at startup (high priority):**

| Error condition | Source location | Why it matters |
|-----------------|-----------------|----------------|
| `SHELL` env var empty and `/bin/bash` fallback missing | `lib/apply.go:375` | Apply script execution fails silently |
| `PLANDEX_API_HOST` set to an invalid URL/hostname | `api/clients.go:25` | Cryptic network errors instead of clear config feedback |
| `PLANDEX_REPL_OUTPUT_FILE` parent directory does not exist | `stream_tui/run.go:102` | `WriteFile` fails at runtime with no pre-check |
| `projects-v2.json` contains malformed JSON | `fs/projects.go:31`, `lib/current.go:96` | Same pattern as `auth.json` — framework already supports this |
| `settings-v2.json` contains malformed JSON | `lib/current.go:198` | Bare unmarshal error on exit |

**Catchable in deferred validation (medium priority):**

| Error condition | Source location | Why it matters |
|-----------------|-----------------|----------------|
| `GOOGLE_APPLICATION_CREDENTIALS` file contains invalid JSON | `lib/startup_validation.go:372` | File exists but content is unparseable |
| `PLANDEX_AWS_PROFILE` names a profile not present in config files | `lib/startup_validation.go:342` | Profile reachability check stops at file existence |
| `PLANDEX_COLUMNS` is not a valid integer | `term/utils.go:88` | Silently ignored — user never learns their value was rejected |
| `PLANDEX_STREAM_FOREGROUND_COLOR` is an invalid ANSI code | `term/utils.go:125` | Silently ignored until rendering |

**Requires runtime context (lower priority):**

| Error condition | Source location | Why deferred |
|-----------------|-----------------|--------------|
| Project root directory is not writable | `lib/apply.go:356` | Project root only known after plan resolution |
| Custom editor command not on PATH | `lib/editor.go:75` | Only known after user selects editor |
| `less` pager command not available | `term/utils.go:49` | Only needed during output display |

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

Key CLI Validation Files:
  /app/cli/lib/startup_validation.go      Startup + provider validation logic
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

*Document generated: January 2026*
