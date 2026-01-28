# Configuration Validation - Quick Reference

## Quick Start

### Check Configuration

```bash
# Server startup will automatically validate
plandex server

# Look for validation messages
✅ Startup validation passed     # All good
❌ Configuration validation failed  # Fix issues shown
```

### Common Validation Failures

#### 1. Missing Database Config
```bash
export DATABASE_URL="postgres://user:pass@localhost:5432/plandex"
# OR
export DB_HOST="localhost"
export DB_PORT="5432"
export DB_USER="plandex_user"
export DB_PASSWORD="mypassword"
export DB_NAME="plandex"
```

#### 2. Missing Provider Credentials

**Quick option (OpenRouter - supports all models):**
```bash
export OPENROUTER_API_KEY="sk-or-v1-..."
```

**Individual providers:**
```bash
export OPENAI_API_KEY="sk-proj-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export GEMINI_API_KEY="AIza..."
```

#### 3. Database Not Running
```bash
# Linux
sudo systemctl start postgresql

# macOS
brew services start postgresql
```

#### 4. Port 4000 Already In Use
```bash
# Find what's using the port
lsof -i :4000

# Kill the process
kill -9 <PID>
```

## Error Message Format

Every validation error includes:

```
🗄️ CRITICAL: [Problem Summary]

📋 Details:
  [What specifically went wrong]

⚠️  Impact:
  [Why this matters]

✅ Solution:
  [Step-by-step fix instructions]

💡 Example:
  [Working configuration]

🔑 Related variables:
  [Environment variables involved]
```

## Validation Phases

### Phase 1: Startup (Automatic)
- Database connectivity
- Environment variables
- Port availability
- Takes ~100-200ms

### Phase 2: Execution (Before plan runs)
- Provider credentials
- File paths
- LiteLLM health
- Takes ~200-500ms

### Phase 3: Runtime (As needed)
- Feature-specific checks

## Provider Setup

### OpenAI
```bash
export OPENAI_API_KEY="sk-proj-..."
# Optional:
export OPENAI_ORG_ID="org-..."
```

### Anthropic
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
```

### OpenRouter
```bash
export OPENROUTER_API_KEY="sk-or-v1-..."
```

### Google AI Studio
```bash
export GEMINI_API_KEY="AIza..."
```

### Google Vertex AI
```bash
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/key.json"
export VERTEXAI_PROJECT="my-project-id"
export VERTEXAI_LOCATION="us-central1"
```

### Azure OpenAI
```bash
export AZURE_OPENAI_API_KEY="your-key"
export AZURE_API_BASE="https://your-resource.openai.azure.com"
export AZURE_API_VERSION="2025-04-01-preview"  # optional
```

### AWS Bedrock
```bash
# Option 1: Using profile
export PLANDEX_AWS_PROFILE="default"

# Option 2: Using credentials
export AWS_ACCESS_KEY_ID="AKIA..."
export AWS_SECRET_ACCESS_KEY="..."
export AWS_REGION="us-east-1"
```

## Troubleshooting

### Database Issues

**Connection Refused:**
```bash
# Check if PostgreSQL is running
systemctl status postgresql  # Linux
brew services list           # macOS

# Check if port is open
telnet localhost 5432
```

**Authentication Failed:**
```bash
# Check user exists
psql -U postgres -c "\du"

# Create user if needed
psql -U postgres -c "CREATE USER plandex_user WITH PASSWORD 'password';"

# Grant privileges
psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE plandex TO plandex_user;"
```

**Database Doesn't Exist:**
```bash
# Create database
createdb plandex

# Or using psql
psql -U postgres -c "CREATE DATABASE plandex;"
```

### Provider Issues

**API Key Not Working:**
1. Verify key is correct (no extra spaces)
2. Check key hasn't expired
3. Verify account has credits/quota
4. Test key with curl:
```bash
# OpenAI
curl https://api.openai.com/v1/models \
  -H "Authorization: Bearer $OPENAI_API_KEY"

# Anthropic
curl https://api.anthropic.com/v1/messages \
  -H "x-api-key: $ANTHROPIC_API_KEY" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"claude-3-opus-20240229","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}'
```

**File Not Found:**
```bash
# Check file exists
ls -l /path/to/credentials.json

# Check permissions
chmod 644 /path/to/credentials.json

# Verify JSON is valid
cat /path/to/credentials.json | jq .
```

### Environment Issues

**Invalid PORT:**
```bash
export PORT="8099"  # Must be a number 1-65535
```

**Conflicting Variables:**
```bash
# Choose one approach:

# Option 1: DATABASE_URL only
unset DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME

# Option 2: DB_* variables only
unset DATABASE_URL
```

### LiteLLM Issues

**Port Already In Use:**
```bash
# Find and kill process using port 4000
lsof -i :4000
kill -9 <PID>
```

**Proxy Not Starting:**
```bash
# Check logs
tail -f /path/to/litellm.log

# Check Python/LiteLLM installed
pip list | grep litellm

# Try manual start
litellm --port 4000
```

## Environment File Template

Create `.env` file:

```bash
# Database
DATABASE_URL=postgres://plandex:password@localhost:5432/plandex

# AI Providers (set at least one)
OPENROUTER_API_KEY=sk-or-v1-...
OPENAI_API_KEY=sk-proj-...
ANTHROPIC_API_KEY=sk-ant-...
GEMINI_API_KEY=AIza...

# Server (optional)
PORT=8099
GOENV=development

# Debug (optional)
# PLANDEX_DEBUG=1
# PLANDEX_DEBUG_LEVEL=debug
```

Load before starting:
```bash
set -a
source .env
set +a
plandex server
```

## Validation Commands

### Check Database
```bash
# Quick connectivity test
psql -U plandex -h localhost -d plandex -c "SELECT 1;"
```

### Check API Keys
```bash
# List all set API keys
env | grep API_KEY
```

### Check Port Availability
```bash
# Check if port 4000 is available
nc -zv localhost 4000
```

## Getting Help

1. **Read the error message carefully** - it includes specific solutions
2. **Check related variables** - listed in the error
3. **Try the example** - working configuration shown
4. **Follow solution steps** - numbered for clarity
5. **Check documentation**: [VALIDATION_EXAMPLES.md](VALIDATION_EXAMPLES.md)
6. **Still stuck?** Report issue: https://github.com/anthropics/plandex/issues

## Best Practices

### Security
```bash
# Set proper permissions on credential files
chmod 600 ~/.gcp/credentials.json
chmod 600 ~/.aws/credentials

# Don't commit .env files
echo ".env" >> .gitignore
```

### Testing
```bash
# Test configuration before starting
env | grep -E "DATABASE_URL|API_KEY|DB_"

# Verify database connection
psql $DATABASE_URL -c "SELECT 1;"

# Check port availability
lsof -i :4000 || echo "Port 4000 is available"
```

### Monitoring
```bash
# Watch server logs
tail -f plandex.log | grep validation

# Check error registry
cat ~/.plandex/errors.json | jq .
```

## Common Patterns

### Multiple Providers
```bash
# Set multiple providers for fallback
export OPENAI_API_KEY="sk-proj-..."
export ANTHROPIC_API_KEY="sk-ant-..."
export OPENROUTER_API_KEY="sk-or-v1-..."
```

### Development vs Production
```bash
# Development
export GOENV=development
export PLANDEX_DEBUG=1
export DATABASE_URL="postgres://localhost/plandex_dev"

# Production
export GOENV=production
export DATABASE_URL="postgres://prod-host/plandex_prod"
```

### Cloud Deployments
```bash
# Use secrets manager
export DATABASE_URL="$(aws secretsmanager get-secret-value --secret-id plandex-db --query SecretString --output text)"
export OPENAI_API_KEY="$(aws secretsmanager get-secret-value --secret-id openai-key --query SecretString --output text)"
```

## Quick Diagnostics

Run these commands to diagnose issues:

```bash
# Check all environment variables
echo "=== Database ==="
echo "DATABASE_URL: ${DATABASE_URL:+SET}"
echo "DB_HOST: ${DB_HOST:+SET}"
echo "DB_PORT: ${DB_PORT:+SET}"

echo "=== Providers ==="
echo "OPENAI_API_KEY: ${OPENAI_API_KEY:+SET}"
echo "ANTHROPIC_API_KEY: ${ANTHROPIC_API_KEY:+SET}"
echo "OPENROUTER_API_KEY: ${OPENROUTER_API_KEY:+SET}"

echo "=== Services ==="
systemctl status postgresql || brew services list
lsof -i :4000 || echo "Port 4000 available"

echo "=== Database Connectivity ==="
psql -U postgres -c "SELECT version();" || echo "PostgreSQL not accessible"
```

Save as `check-plandex-config.sh` and run before starting server.

## Validation Success Indicators

When everything is configured correctly:

```
✅ Startup validation passed
✅ LiteLLM proxy started successfully
✅ Database connection established
✅ Database initialization complete
Started Plandex server on port 8099
```

You're ready to use Plandex!

---

**More Information:**
- [VALIDATION_EXAMPLES.md](VALIDATION_EXAMPLES.md) - Detailed failure examples
- [VALIDATION_SYSTEM.md](VALIDATION_SYSTEM.md) - Complete system documentation
- [validation/README.md](../app/shared/validation/README.md) - Package documentation
