# Configuration Validation Examples

This document demonstrates common configuration failures and how Plandex's validation system responds with clear, actionable error messages.

## Table of Contents

1. [Database Configuration Errors](#database-configuration-errors)
2. [Provider Credential Errors](#provider-credential-errors)
3. [Environment Variable Errors](#environment-variable-errors)
4. [File Path Errors](#file-path-errors)
5. [Network and Service Errors](#network-and-service-errors)

---

## Database Configuration Errors

### Example 1: No Database Configuration

**Scenario:** User starts Plandex server without setting any database environment variables.

**Error Output:**
```
Running pre-startup validation checks...
❌ Configuration validation failed
================================================================================

🗄️ CRITICAL: No database configuration found

📋 Details:
  Neither DATABASE_URL nor individual DB_* environment variables are set.

⚠️  Impact:
  Plandex server cannot start without a database connection.

✅ Solution:
  Set either DATABASE_URL or all of DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, and DB_NAME.

💡 Example:
  Option 1: Using DATABASE_URL
    export DATABASE_URL="postgres://user:password@localhost:5432/plandex"

  Option 2: Using individual variables
    export DB_HOST="localhost"
    export DB_PORT="5432"
    export DB_USER="plandex_user"
    export DB_PASSWORD="secure_password"
    export DB_NAME="plandex"

🔑 Related variables:
  DATABASE_URL, DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME

================================================================================
Found 1 error(s)
Please fix the errors above before continuing.
```

**How to Fix:**
```bash
# Option 1: Use DATABASE_URL
export DATABASE_URL="postgres://plandex_user:mypassword@localhost:5432/plandex"

# Option 2: Use individual variables
export DB_HOST="localhost"
export DB_PORT="5432"
export DB_USER="plandex_user"
export DB_PASSWORD="mypassword"
export DB_NAME="plandex"
```

---

### Example 2: Incomplete Database Configuration

**Scenario:** User sets some but not all DB_* variables.

**Error Output:**
```
🗄️ CRITICAL: Incomplete database configuration

📋 Details:
  Some DB_* variables are set but these are missing: DB_PASSWORD, DB_NAME

⚠️  Impact:
  Plandex server cannot start with incomplete database configuration.

✅ Solution:
  Set all required DB_* variables or use DATABASE_URL instead.

💡 Example:
  export DB_HOST="localhost"
  export DB_PORT="5432"
  export DB_USER="plandex_user"
  export DB_PASSWORD="secure_password"
  export DB_NAME="plandex"
```

**How to Fix:**
```bash
# Add the missing variables
export DB_PASSWORD="mypassword"
export DB_NAME="plandex"

# Or switch to DATABASE_URL
export DATABASE_URL="postgres://plandex_user:mypassword@localhost:5432/plandex"
```

---

### Example 3: Database Connection Refused

**Scenario:** PostgreSQL is not running or not accessible.

**Error Output:**
```
🗄️ CRITICAL: Cannot connect to database

📋 Details:
  Database server is not accepting connections.

⚠️  Impact:
  Plandex server cannot start without a working database connection.

✅ Solution:
  The database server may not be running or is not accessible:
    1. Check if PostgreSQL is running:
       systemctl status postgresql  # Linux
       brew services list            # macOS
    2. Verify the host and port are correct
    3. Check firewall settings if connecting to a remote host

🐛 Underlying error:
  dial tcp 127.0.0.1:5432: connect: connection refused
```

**How to Fix:**
```bash
# On Linux
sudo systemctl start postgresql
sudo systemctl status postgresql

# On macOS
brew services start postgresql
brew services list

# Check if PostgreSQL is listening
sudo lsof -i :5432
```

---

### Example 4: Database Does Not Exist

**Scenario:** User specifies a database that hasn't been created yet.

**Error Output:**
```
🗄️ CRITICAL: Cannot connect to database

📋 Details:
  The specified database does not exist.

⚠️  Impact:
  Plandex server cannot start without a working database connection.

✅ Solution:
  Create the database:
    1. Using psql:
       psql -U postgres -c "CREATE DATABASE plandex;"
    2. Or using createdb:
       createdb -U postgres plandex
    3. Then restart Plandex
```

**How to Fix:**
```bash
# Create the database
createdb -U postgres plandex

# Or using psql
psql -U postgres -c "CREATE DATABASE plandex;"

# Verify it was created
psql -U postgres -l | grep plandex
```

---

### Example 5: Authentication Failed

**Scenario:** Incorrect database credentials.

**Error Output:**
```
🗄️ CRITICAL: Cannot connect to database

📋 Details:
  Database credentials are invalid.

⚠️  Impact:
  Plandex server cannot start without a working database connection.

✅ Solution:
  Fix the database authentication:
    1. Verify username and password are correct
    2. Check PostgreSQL user exists:
       psql -U postgres -c "\du"
    3. Update pg_hba.conf if needed to allow authentication method
```

**How to Fix:**
```bash
# Verify the user exists
psql -U postgres -c "\du"

# Create the user if needed
psql -U postgres -c "CREATE USER plandex_user WITH PASSWORD 'mypassword';"

# Grant privileges
psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE plandex TO plandex_user;"

# Update DATABASE_URL with correct credentials
export DATABASE_URL="postgres://plandex_user:mypassword@localhost:5432/plandex"
```

---

## Provider Credential Errors

### Example 6: Missing OpenAI API Key

**Scenario:** User tries to use OpenAI models without setting API key.

**Error Output:**
```
🔌 ERROR: Missing required credentials for OpenAI

📋 Details:
  The following required environment variables are not set: OPENAI_API_KEY

⚠️  Impact:
  Cannot use OpenAI models without these credentials.

✅ Solution:
  Configure OpenAI credentials:
    1. Get an API key from https://platform.openai.com/api-keys
    2. Set: export OPENAI_API_KEY=your_key

💡 Example:
  export OPENAI_API_KEY="sk-proj-..."
  export OPENAI_ORG_ID="org-..." # optional

🔑 Related variables:
  OPENAI_API_KEY
```

**How to Fix:**
```bash
# Get your API key from https://platform.openai.com/api-keys
export OPENAI_API_KEY="sk-proj-abcdef123456..."

# Optional: Set organization ID
export OPENAI_ORG_ID="org-xyz123"
```

---

### Example 7: No Provider Credentials Configured

**Scenario:** User hasn't configured any provider credentials.

**Error Output:**
```
🔌 ERROR: No provider credentials configured

📋 Details:
  No valid credentials found for any AI model provider.

⚠️  Impact:
  Cannot execute plans without at least one configured provider.

✅ Solution:
  Configure credentials for at least one provider:

  Quick start with OpenRouter (supports multiple models):
    1. Sign up at https://openrouter.ai
    2. Generate an API key
    3. Set: export OPENROUTER_API_KEY=your_key

  Or configure a specific provider:
    - OpenAI: export OPENAI_API_KEY=your_key
    - Anthropic: export ANTHROPIC_API_KEY=your_key
    - Google: export GEMINI_API_KEY=your_key

  See https://docs.plandex.ai/models/model-providers for full details.

💡 Example:
  export OPENROUTER_API_KEY="sk-or-v1-..."
  export OPENAI_API_KEY="sk-proj-..."
  export ANTHROPIC_API_KEY="sk-ant-..."
```

**How to Fix:**
```bash
# Quick option: Use OpenRouter (supports all models with single API key)
export OPENROUTER_API_KEY="sk-or-v1-your-key-here"

# Or use specific providers
export OPENAI_API_KEY="sk-proj-your-key"
export ANTHROPIC_API_KEY="sk-ant-your-key"
export GEMINI_API_KEY="your-google-key"
```

---

### Example 8: Incomplete Google Vertex AI Configuration

**Scenario:** User sets credentials file but forgets project and location.

**Error Output:**
```
🔌 ERROR: Missing required credentials for Google Vertex AI

📋 Details:
  The following required environment variables are not set: VERTEXAI_PROJECT, VERTEXAI_LOCATION

⚠️  Impact:
  Cannot use Google Vertex AI models without these credentials.

✅ Solution:
  Configure Google Vertex AI credentials:
    1. Create a service account in Google Cloud Console
    2. Download the JSON key file
    3. Set environment variables:
       export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
       export VERTEXAI_PROJECT=your-project-id
       export VERTEXAI_LOCATION=us-central1

💡 Example:
  export GOOGLE_APPLICATION_CREDENTIALS="/path/to/credentials.json"
  export VERTEXAI_PROJECT="my-project-id"
  export VERTEXAI_LOCATION="us-central1"

🔑 Related variables:
  VERTEXAI_PROJECT, VERTEXAI_LOCATION
```

**How to Fix:**
```bash
# Set all required variables
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account.json"
export VERTEXAI_PROJECT="my-gcp-project"
export VERTEXAI_LOCATION="us-central1"
```

---

## Environment Variable Errors

### Example 9: Invalid PORT Number

**Scenario:** User sets PORT to an invalid value.

**Error Output:**
```
⚙️ ERROR: Invalid PORT format

📋 Details:
  PORT must be a number, got: abc123

⚠️  Impact:
  Server will fail to start with invalid port number.

✅ Solution:
  Set PORT to a valid number between 1 and 65535.

💡 Example:
  export PORT="8099"  # default

🔑 Related variables:
  PORT
```

**How to Fix:**
```bash
# Use a valid port number
export PORT="8099"

# Or use another common port
export PORT="3000"
```

---

### Example 10: Conflicting Configuration

**Scenario:** User sets both DATABASE_URL and DB_* variables.

**Error Output:**
```
⚠️  Configuration warnings
================================================================================

⚙️ WARNING: Both DATABASE_URL and DB_* variables are set

📋 Details:
  You have both DATABASE_URL and individual DB_* variables configured. DATABASE_URL will take precedence.

⚠️  Impact:
  DB_* variables will be ignored, which may be confusing.

✅ Solution:
  Use either DATABASE_URL or DB_* variables, but not both. Remove the unused configuration.

💡 Example:
  # Option 1: Use DATABASE_URL only
  export DATABASE_URL="postgres://user:pass@host:5432/db"
  # Remove: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME

  # Option 2: Use DB_* variables only
  export DB_HOST="localhost"
  export DB_PORT="5432"
  # etc.
  # Remove: DATABASE_URL

🔑 Related variables:
  DATABASE_URL, DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME
```

**How to Fix:**
```bash
# Option 1: Keep DATABASE_URL, remove DB_* variables
unset DB_HOST DB_PORT DB_USER DB_PASSWORD DB_NAME

# Option 2: Keep DB_* variables, remove DATABASE_URL
unset DATABASE_URL
```

---

## File Path Errors

### Example 11: Credentials File Not Found

**Scenario:** User specifies a path to credentials file that doesn't exist.

**Error Output:**
```
📁 ERROR: File not found for GOOGLE_APPLICATION_CREDENTIALS

📋 Details:
  File path '/path/to/missing/credentials.json' does not exist

⚠️  Impact:
  Provider cannot load credentials from missing file.

✅ Solution:
  Create the credentials file or fix the path:
    1. Verify the path is correct
    2. Ensure the file exists: ls -l "/path/to/missing/credentials.json"
    3. Check file permissions are readable

  Or provide credentials directly as JSON:
    export GOOGLE_APPLICATION_CREDENTIALS='{"key": "value"}'

🔑 Related variables:
  GOOGLE_APPLICATION_CREDENTIALS
```

**How to Fix:**
```bash
# Verify the file exists
ls -l /path/to/credentials.json

# Fix the path in your environment variable
export GOOGLE_APPLICATION_CREDENTIALS="/correct/path/to/credentials.json"

# Or use the correct path
export GOOGLE_APPLICATION_CREDENTIALS="$HOME/.gcp/service-account.json"
```

---

### Example 12: Malformed JSON in Credentials File

**Scenario:** Credentials file contains invalid JSON.

**Error Output:**
```
📝 ERROR: Invalid JSON in credentials file for GOOGLE_APPLICATION_CREDENTIALS

📋 Details:
  File '/path/to/credentials.json' contains invalid JSON: unexpected end of JSON input

⚠️  Impact:
  Provider cannot parse malformed JSON credentials.

✅ Solution:
  Fix the JSON format in the credentials file.

🔑 Related variables:
  GOOGLE_APPLICATION_CREDENTIALS
```

**How to Fix:**
```bash
# Validate the JSON file
cat /path/to/credentials.json | jq .

# If jq shows errors, fix the JSON format
# Download a fresh credentials file from your provider
```

---

## Network and Service Errors

### Example 13: LiteLLM Port Already in Use

**Scenario:** Port 4000 is already occupied when starting Plandex.

**Error Output:**
```
⚠️  Configuration warnings
================================================================================

🌐 WARNING: Port 4000 is already in use

📋 Details:
  LiteLLM proxy port (4000) is already occupied by another process.

⚠️  Impact:
  May cause LiteLLM proxy startup failure.

✅ Solution:
  Check what's using port 4000:
    lsof -i :4000  # macOS/Linux
    netstat -ano | findstr :4000  # Windows

  If it's a stale LiteLLM process, kill it:
    kill <pid>

  Or configure Plandex to use a different port if supported.
```

**How to Fix:**
```bash
# Find what's using port 4000
lsof -i :4000

# Kill the process if it's stale
kill -9 <PID>

# Or restart your system to clear stale processes
```

---

### Example 14: LiteLLM Proxy Not Responding

**Scenario:** LiteLLM proxy is not accessible during execution.

**Error Output:**
```
🌐 CRITICAL: LiteLLM proxy is not responding

📋 Details:
  Health check failed: dial tcp 127.0.0.1:4000: connect: connection refused

⚠️  Impact:
  Plandex cannot communicate with AI model providers without LiteLLM proxy.

✅ Solution:
  Troubleshoot LiteLLM proxy:
    1. Check if the proxy process is running:
       ps aux | grep litellm
    2. Check proxy logs for errors
    3. Verify no firewall is blocking localhost:4000
    4. Try restarting the Plandex server
```

**How to Fix:**
```bash
# Check if LiteLLM is running
ps aux | grep litellm

# Check the health endpoint manually
curl http://localhost:4000/health

# Restart Plandex server to restart LiteLLM
# (Server startup will automatically start LiteLLM)
```

---

## Complete Validation Success

**Scenario:** All validation checks pass.

**Success Output:**
```
Starting Plandex server...
Running pre-startup validation checks...
✅ Startup validation passed
Starting LiteLLM proxy...
✅ LiteLLM proxy started successfully
Connecting to database...
✅ Database connection established
Running database migrations...
migration state is up to date
✅ Database initialization complete
Started Plandex server on port 8099
```

---

## Best Practices

### 1. Use Environment Files

Create a `.env` file for your configuration:

```bash
# .env
DATABASE_URL=postgres://plandex:password@localhost:5432/plandex
OPENAI_API_KEY=sk-proj-your-key
ANTHROPIC_API_KEY=sk-ant-your-key
PORT=8099
GOENV=development
```

Load it before starting Plandex:

```bash
set -a
source .env
set +a
plandex server
```

### 2. Validate Before Starting

Use the validation helpers to check configuration:

```bash
# Check database connectivity
psql -U plandex -h localhost -d plandex -c "SELECT 1;"

# Verify API keys are set
env | grep API_KEY

# Test LiteLLM port is available
nc -zv localhost 4000
```

### 3. Keep Credentials Secure

```bash
# Set proper permissions on credential files
chmod 600 ~/.gcp/credentials.json
chmod 600 ~/.aws/credentials

# Don't commit .env files
echo ".env" >> .gitignore

# Use secret management for production
# Consider: HashiCorp Vault, AWS Secrets Manager, etc.
```

### 4. Monitor Validation Logs

Plandex logs detailed validation information:

```bash
# Check server logs
tail -f /path/to/plandex.log

# Look for validation errors
grep "validation" /path/to/plandex.log

# Check error registry
cat ~/.plandex/errors.json | jq .
```

---

## Getting Help

If you encounter validation errors not covered here:

1. Check the full error message and related variables
2. Follow the solution steps provided in the error
3. Consult the documentation: https://docs.plandex.ai
4. Ask for help: https://github.com/anthropics/plandex/issues

Remember: Validation errors are designed to help you fix issues quickly. Read the error messages carefully—they include specific solutions for your situation.
