# Plandex System Design Document

**Version:** 1.0
**Last Updated:** January 2026

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
├── data_models.go          # Core API types
├── ai_models_available.go  # Available models
├── ai_models_providers.go  # Provider configs
├── plan_config.go          # Plan settings
├── context.go              # Context types
└── req_res.go              # Request/response wrappers
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
- Heartbeat system for active plans
- Graceful shutdown with 60s timeout for active operations

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
