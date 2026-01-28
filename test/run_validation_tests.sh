#!/bin/bash
# =============================================================================
# run_validation_tests.sh — Local test runner for the validation phase pipeline
# =============================================================================
#
# Mirrors the CI pipeline (validation-tests.yml) so developers can run the
# full validation test suite locally before pushing.
#
# Usage:
#   ./test/run_validation_tests.sh            # run all checks
#   ./test/run_validation_tests.sh unit       # unit tests only
#   ./test/run_validation_tests.sh build      # CLI build check only
#   ./test/run_validation_tests.sh format     # gofmt check only
#   ./test/run_validation_tests.sh vet        # go vet only
#
# Exit codes:
#   0 — all checks passed
#   1 — one or more checks failed (details printed above exit)
#
# Updated: January 2026
# =============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Source shared test utilities for colour logging
source "$SCRIPT_DIR/test_utils.sh"

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
SHARED_DIR="$REPO_ROOT/app/shared"
CLI_DIR="$REPO_ROOT/app/cli"

# Track job results via a temp file (portable, no associative arrays needed)
RESULTS_FILE=$(mktemp)
trap "rm -f $RESULTS_FILE" EXIT

# ---------------------------------------------------------------------------
# Helper: run a named check and record pass/fail
# ---------------------------------------------------------------------------
run_check() {
    local name="$1"
    shift

    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    info "[$name] Starting"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

    set +e
    "$@"
    local rc=$?
    set -e

    if [ $rc -eq 0 ]; then
        echo "$name PASS" >> "$RESULTS_FILE"
        success "[$name] Passed"
    else
        echo "$name FAIL" >> "$RESULTS_FILE"
        echo -e "${RED}✗ [$name] Failed${NC}"
    fi
}

# ---------------------------------------------------------------------------
# CHECK 1: gofmt — formatting
# ---------------------------------------------------------------------------
check_format() {
    info "Checking gofmt on validation source files..."

    local bad=""

    # shared
    local shared_unformatted
    shared_unformatted=$(cd "$SHARED_DIR" && gofmt -l validation.go validation_test.go 2>/dev/null) || true
    if [ -n "$shared_unformatted" ]; then
        echo -e "${RED}Unformatted files in app/shared:${NC}"
        echo "$shared_unformatted"
        bad=1
    fi

    # cli
    local cli_unformatted
    cli_unformatted=$(cd "$CLI_DIR" && gofmt -l lib/startup_validation.go 2>/dev/null) || true
    if [ -n "$cli_unformatted" ]; then
        echo -e "${RED}Unformatted files in app/cli:${NC}"
        echo "$cli_unformatted"
        bad=1
    fi

    if [ -n "$bad" ]; then
        info "Fix with: gofmt -w app/shared/ app/cli/lib/startup_validation.go"
        return 1
    fi

    info "All validation files are properly formatted."
}

# ---------------------------------------------------------------------------
# CHECK 2: go vet — static analysis
# ---------------------------------------------------------------------------
check_vet() {
    info "Running go vet on shared module..."
    (cd "$SHARED_DIR" && go vet ./...)

    info "Running go vet on cli module..."
    (cd "$CLI_DIR" && go vet ./...)

    info "go vet passed on both modules."
}

# ---------------------------------------------------------------------------
# CHECK 3: unit tests — race-safe with coverage
# ---------------------------------------------------------------------------
check_unit_tests() {
    info "Running validation framework unit tests (race detector enabled)..."

    cd "$SHARED_DIR"

    go test -v -race -coverprofile=validation-coverage.out -covermode=atomic \
        -run "TestValidationResult|TestValidationError|TestValidateEnvVarSet|TestValidateProviderCompatibility|TestValidateFilePath|TestValidationResult_Timestamp|TestNewValidationResult" \
        ./... 2>&1 | tee /tmp/validation-unit-tests.txt

    local passed failed races
    passed=$(grep -c "^--- PASS:" /tmp/validation-unit-tests.txt 2>/dev/null | tr -d '[:space:]') || passed=0
    failed=$(grep -c "^--- FAIL:" /tmp/validation-unit-tests.txt 2>/dev/null | tr -d '[:space:]') || failed=0
    races=$(grep -c "WARNING: DATA RACE" /tmp/validation-unit-tests.txt 2>/dev/null | tr -d '[:space:]') || races=0

    echo ""
    echo "┌──────────────┬───────┐"
    echo "│ Metric       │ Count │"
    echo "├──────────────┼───────┤"
    printf "│ Passed       │ %5s │\n" "$passed"
    printf "│ Failed       │ %5s │\n" "$failed"
    printf "│ Data Races   │ %5s │\n" "$races"
    echo "└──────────────┴───────┘"

    # Coverage summary
    echo ""
    info "Coverage for validation files:"
    go tool cover -func=validation-coverage.out 2>/dev/null | grep -E "validation|TOTAL" || true

    if [ "$failed" != "0" ]; then
        echo -e "${RED}$failed test(s) failed.${NC}"
        return 1
    fi
    if [ "$races" != "0" ]; then
        echo -e "${RED}Data race(s) detected.${NC}"
        return 1
    fi

    cd "$REPO_ROOT"
}

# ---------------------------------------------------------------------------
# CHECK 4: CLI build — integration compilation
# ---------------------------------------------------------------------------
check_build() {
    info "Building CLI module to verify validation integration points..."

    (cd "$CLI_DIR" && go build ./...)

    info "Verifying integration entry points..."

    local missing=""
    for cmd in tell build continue; do
        if grep -q "MustRunDeferredValidation" "$CLI_DIR/cmd/${cmd}.go"; then
            success "  cmd/${cmd}.go — MustRunDeferredValidation present"
        else
            echo -e "${RED}  cmd/${cmd}.go — MustRunDeferredValidation MISSING${NC}"
            missing=1
        fi
    done

    if grep -q "RunStartupValidation" "$CLI_DIR/main.go"; then
        success "  main.go — RunStartupValidation present"
    else
        echo -e "${RED}  main.go — RunStartupValidation MISSING${NC}"
        missing=1
    fi

    if [ -n "$missing" ]; then
        return 1
    fi

    info "CLI builds cleanly with all validation hooks wired."
}

# ---------------------------------------------------------------------------
# Main dispatch
# ---------------------------------------------------------------------------
TARGET="${1:-all}"

case "$TARGET" in
    format)
        run_check "format" check_format
        ;;
    vet)
        run_check "vet" check_vet
        ;;
    unit)
        run_check "unit-tests" check_unit_tests
        ;;
    build)
        run_check "build" check_build
        ;;
    all)
        run_check "format"     check_format
        run_check "vet"        check_vet
        run_check "unit-tests" check_unit_tests
        run_check "build"      check_build
        ;;
    *)
        echo "Usage: $0 {all|format|vet|unit|build}"
        exit 1
        ;;
esac

# ---------------------------------------------------------------------------
# Final summary
# ---------------------------------------------------------------------------
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  VALIDATION PIPELINE SUMMARY"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

any_failed=0
while read -r job status; do
    if [ "$status" = "PASS" ]; then
        printf "  ${GREEN}%-16s %s${NC}\n" "$job" "PASS"
    else
        printf "  ${RED}%-16s %s${NC}\n" "$job" "FAIL"
        any_failed=1
    fi
done < "$RESULTS_FILE"

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $any_failed -eq 1 ]; then
    echo -e "${RED}Some checks failed. Review the output above for details.${NC}"
    exit 1
fi

echo -e "${GREEN}All validation pipeline checks passed.${NC}"
exit 0
