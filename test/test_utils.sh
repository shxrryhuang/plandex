#!/bin/bash
# =============================================================================
# test_utils.sh - Common utilities for Plandex test scripts
# =============================================================================
#
# This file provides shared utilities for all Plandex test scripts including:
#   - Color-coded logging (success, error, info)
#   - Command execution with exit code handling
#   - Timeout support for LLM operations
#   - Test environment setup and cleanup
#
# Configuration Variables (can be overridden via environment):
#   PLANDEX_CMD      - Command to run Plandex (default: plandex-dev)
#   PLANDEX_TIMEOUT  - Timeout in seconds for LLM operations (default: 300)
#   PLANDEX_ENV_FILE - Path to environment file (default: ../.env.client-keys)
#
# Usage:
#   source test_utils.sh
#   setup_test_dir "my-test"
#   run_plandex_cmd "new -n test-plan" "Create test plan"
#   run_plandex_cmd_with_timeout "tell 'hello'" "LLM call" 120
#   cleanup_test_dir
#
# Updated: January 2026
#   - Added PLANDEX_TIMEOUT for configurable LLM operation timeouts
#   - Added PLANDEX_ENV_FILE for configurable environment file location
#   - Added run_cmd_with_timeout() and run_plandex_cmd_with_timeout()
#   - setup_test_dir() now handles missing env file gracefully
#
# =============================================================================

export PLANDEX_ENV='development'

# =============================================================================
# ANSI Color Codes
# =============================================================================
export RED='\033[0;31m'
export GREEN='\033[0;32m'
export YELLOW='\033[1;33m'
export NC='\033[0m' # No Color

# =============================================================================
# Configuration Variables
# =============================================================================

# Default command - can be overridden by environment variable
export PLANDEX_CMD="${PLANDEX_CMD:-plandex-dev}"

# Default timeout for LLM operations (in seconds)
# LLM calls can take a while; 300s (5 min) is a safe default
export PLANDEX_TIMEOUT="${PLANDEX_TIMEOUT:-300}"

# Environment file location - can be customized
# This file typically contains API keys and other secrets
export PLANDEX_ENV_FILE="${PLANDEX_ENV_FILE:-../.env.client-keys}"

# =============================================================================
# Logging Functions
# =============================================================================

log() {
    echo -e "$1"
}

success() {
    log "${GREEN}✓ $1${NC}"
}

error() {
    log "${RED}✗ $1${NC}"
    exit 1
}

info() {
    log "${YELLOW}→ $1${NC}"
}

# =============================================================================
# Command Execution Functions
# =============================================================================

# Run command and check for success
run_cmd() {
    local cmd="$1"
    local description="$2"
    
    info "Running: $cmd"
    
    # Run command and capture output and exit code properly
    set +e  # Temporarily disable exit on error
    output=$(eval "$cmd" 2>&1)
    local exit_code=$?
    set -e  # Re-enable exit on error
    
    # Log the output
    echo "$output"
    
    if [ "$exit_code" -eq 0 ]; then
        success "$description"
    else
        error "$description failed (exit code: $exit_code)"
    fi
}

# Run plandex command
run_plandex_cmd() {
    local cmd="$1"
    local description="$2"
    run_cmd "$PLANDEX_CMD $cmd" "$description"
}

# Run plandex command and check if output contains substring
check_plandex_contains() {
    local cmd="$1"
    local expected="$2"
    local description="$3"
    
    info "Running: $PLANDEX_CMD $cmd"
    
    local output=$($PLANDEX_CMD $cmd 2>&1)
    echo "$output"
    
    if echo "$output" | grep -q "$expected"; then
        success "$description"
    else
        error "$description - expected to find '$expected'"
    fi
}

# Check if command fails (expecting failure)
expect_failure() {
    local cmd="$1"
    local description="$2"
    
    info "Running (expecting failure): $cmd"
    
    # Run the command and capture both output and exit code
    set +e  # Temporarily disable exit on error
    output=$(eval "$cmd" 2>&1)
    local exit_code=$?
    set -e  # Re-enable exit on error
    
    echo "$output"
    
    if [ "$exit_code" -ne 0 ]; then
        success "$description (failed as expected with exit code $exit_code)"
    else
        error "$description should have failed but succeeded (exit code: $exit_code)"
    fi
}

# Expect plandex command to fail
expect_plandex_failure() {
    local cmd="$1"
    local description="$2"
    expect_failure "$PLANDEX_CMD $cmd" "$description"
}

# =============================================================================
# Assertion Functions
# =============================================================================

# Check if file exists
check_file() {
    if [ -f "$1" ]; then
        success "File exists: $1"
    else
        error "File missing: $1"
    fi
}

# =============================================================================
# Test Environment Functions
# =============================================================================

# Setup test environment
# Creates a temporary directory and loads environment variables
setup_test_dir() {
    # Source environment file if it exists
    if [ -f "$PLANDEX_ENV_FILE" ]; then
        source "$PLANDEX_ENV_FILE"
        info "Loaded environment from $PLANDEX_ENV_FILE"
    else
        info "Warning: Environment file $PLANDEX_ENV_FILE not found, using defaults"
    fi

    local test_name="$1"
    TEST_DIR="/tmp/plandex-${test_name}-$$"
    TIMESTAMP=$(date +%Y%m%d_%H%M%S)

    info "Setting up test environment in $TEST_DIR"
    mkdir -p "$TEST_DIR"
    cd "$TEST_DIR"

    success "Test environment created"
}

# =============================================================================
# Timeout Functions (for LLM operations)
# =============================================================================

# Run command with timeout support
# Usage: run_cmd_with_timeout "command" "description" [timeout_seconds]
# Exit code 124 indicates timeout
run_cmd_with_timeout() {
    local cmd="$1"
    local description="$2"
    local timeout_secs="${3:-$PLANDEX_TIMEOUT}"

    info "Running (timeout ${timeout_secs}s): $cmd"

    # Run command with timeout and capture output and exit code
    set +e
    output=$(timeout "$timeout_secs" bash -c "$cmd" 2>&1)
    local exit_code=$?
    set -e

    # Log the output
    echo "$output"

    if [ "$exit_code" -eq 124 ]; then
        error "$description timed out after ${timeout_secs}s"
    elif [ "$exit_code" -eq 0 ]; then
        success "$description"
    else
        error "$description failed (exit code: $exit_code)"
    fi
}

# Run plandex command with timeout
run_plandex_cmd_with_timeout() {
    local cmd="$1"
    local description="$2"
    local timeout_secs="${3:-$PLANDEX_TIMEOUT}"
    run_cmd_with_timeout "$PLANDEX_CMD $cmd" "$description" "$timeout_secs"
}

# Cleanup function
cleanup_test_dir() {
    info "Cleaning up test environment"
    cd /
    rm -rf "$TEST_DIR"
    success "Cleanup complete"
}