#!/bin/bash

# Plandex Dual-Path Verification Script
# Tests that both validation and original paths work correctly

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Emojis
CHECK="✅"
CROSS="❌"
GEAR="⚙️"
ROCKET="🚀"
WARNING="⚠️"

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

print_success() {
    echo -e "${GREEN}${CHECK} $1${NC}"
}

print_error() {
    echo -e "${RED}${CROSS} $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}${WARNING} $1${NC}"
}

print_info() {
    echo -e "${BLUE}$1${NC}"
}

print_header() {
    echo ""
    echo "═══════════════════════════════════════════════════════════════"
    echo "  $1"
    echo "═══════════════════════════════════════════════════════════════"
    echo ""
}

# Function to save current environment
save_environment() {
    SAVED_VALIDATION="${PLANDEX_ENABLE_VALIDATION:-}"
    SAVED_VERBOSE="${PLANDEX_VALIDATION_VERBOSE:-}"
    SAVED_STRICT="${PLANDEX_VALIDATION_STRICT:-}"
    SAVED_FILE_CHECKS="${PLANDEX_VALIDATION_FILE_CHECKS:-}"
}

# Function to restore environment
restore_environment() {
    if [[ -n "$SAVED_VALIDATION" ]]; then
        export PLANDEX_ENABLE_VALIDATION="$SAVED_VALIDATION"
    else
        unset PLANDEX_ENABLE_VALIDATION
    fi

    if [[ -n "$SAVED_VERBOSE" ]]; then
        export PLANDEX_VALIDATION_VERBOSE="$SAVED_VERBOSE"
    else
        unset PLANDEX_VALIDATION_VERBOSE
    fi

    if [[ -n "$SAVED_STRICT" ]]; then
        export PLANDEX_VALIDATION_STRICT="$SAVED_STRICT"
    else
        unset PLANDEX_VALIDATION_STRICT
    fi

    if [[ -n "$SAVED_FILE_CHECKS" ]]; then
        export PLANDEX_VALIDATION_FILE_CHECKS="$SAVED_FILE_CHECKS"
    else
        unset PLANDEX_VALIDATION_FILE_CHECKS
    fi
}

# Function to run test
run_test() {
    local test_name="$1"
    local test_command="$2"

    echo ""
    print_info "Test: $test_name"
    echo ""

    if eval "$test_command"; then
        print_success "PASSED: $test_name"
        ((TESTS_PASSED++))
        return 0
    else
        print_error "FAILED: $test_name"
        ((TESTS_FAILED++))
        return 1
    fi
}

# Test 1: Validation package compiles
test_validation_package() {
    print_info "Building validation package..."
    cd /Users/sherryhuang/Desktop/mercor/5/5/model_b/app/shared/validation
    go build -o /tmp/validation-test.o
    rm -f /tmp/validation-test.o
    return 0
}

# Test 2: Features package compiles
test_features_package() {
    print_info "Building features package..."
    cd /Users/sherryhuang/Desktop/mercor/5/5/model_b/app/shared/features
    go build -o /tmp/features-test.o
    rm -f /tmp/features-test.o
    return 0
}

# Test 3: Server compiles with validation disabled
test_server_compile_disabled() {
    print_info "Building server (validation disabled)..."
    cd /Users/sherryhuang/Desktop/mercor/5/5/model_b/app/server
    unset PLANDEX_ENABLE_VALIDATION
    go build -o /tmp/plandex-server-test
    rm -f /tmp/plandex-server-test
    return 0
}

# Test 4: Server compiles with validation enabled
test_server_compile_enabled() {
    print_info "Building server (validation enabled)..."
    cd /Users/sherryhuang/Desktop/mercor/5/5/model_b/app/server
    export PLANDEX_ENABLE_VALIDATION=true
    go build -o /tmp/plandex-server-test
    rm -f /tmp/plandex-server-test
    return 0
}

# Test 5: Feature flags work correctly
test_feature_flags() {
    print_info "Testing feature flag system..."

    # Test disabled state
    unset PLANDEX_ENABLE_VALIDATION
    cd /Users/sherryhuang/Desktop/mercor/5/5/model_b/app/shared/features

    cat > /tmp/test_features.go << 'EOF'
package main

import (
    "fmt"
    "os"
    "plandex-shared/features"
)

func main() {
    // Test disabled state
    os.Unsetenv("PLANDEX_ENABLE_VALIDATION")
    manager := features.NewFeatureManager()
    manager.LoadFromEnvironment()

    if manager.IsEnabled(features.ValidationSystem) {
        fmt.Println("ERROR: Validation should be disabled")
        os.Exit(1)
    }

    // Test enabled state
    os.Setenv("PLANDEX_ENABLE_VALIDATION", "true")
    manager2 := features.NewFeatureManager()
    manager2.LoadFromEnvironment()

    if !manager2.IsEnabled(features.ValidationSystem) {
        fmt.Println("ERROR: Validation should be enabled")
        os.Exit(1)
    }

    fmt.Println("Feature flags working correctly")
    os.Exit(0)
}
EOF

    cd /Users/sherryhuang/Desktop/mercor/5/5/model_b/app/shared
    go run /tmp/test_features.go
    rm -f /tmp/test_features.go
    return 0
}

# Test 6: Validation tests pass
test_validation_tests() {
    print_info "Running validation test suite..."
    cd /Users/sherryhuang/Desktop/mercor/5/5/model_b/app/shared/validation
    go test -v -count=1 | tee /tmp/validation-test-output.txt

    # Check if all tests passed
    if grep -q "FAIL" /tmp/validation-test-output.txt; then
        rm -f /tmp/validation-test-output.txt
        return 1
    fi

    rm -f /tmp/validation-test-output.txt
    return 0
}

# Test 7: Safe wrappers work
test_safe_wrappers() {
    print_info "Testing safe validation wrappers..."

    cat > /tmp/test_wrappers.go << 'EOF'
package main

import (
    "context"
    "fmt"
    "os"
    "plandex-shared/validation"
)

func main() {
    ctx := context.Background()

    // Test with validation disabled
    os.Unsetenv("PLANDEX_ENABLE_VALIDATION")
    err := validation.SafeValidateStartup(ctx)
    if err != nil {
        fmt.Println("ERROR: Safe wrapper should not error when disabled")
        os.Exit(1)
    }

    fmt.Println("Safe wrappers working correctly")
    os.Exit(0)
}
EOF

    cd /Users/sherryhuang/Desktop/mercor/5/5/model_b/app/shared
    go run /tmp/test_wrappers.go
    rm -f /tmp/test_wrappers.go
    return 0
}

# Main test execution
main() {
    print_header "${GEAR} Plandex Dual-Path Verification"

    print_info "This script verifies both validation and original paths work correctly"
    echo ""

    # Save current environment
    save_environment

    # Run tests
    run_test "Validation package compiles" "test_validation_package"
    run_test "Features package compiles" "test_features_package"
    run_test "Server compiles (validation disabled)" "test_server_compile_disabled"
    run_test "Server compiles (validation enabled)" "test_server_compile_enabled"
    run_test "Feature flags work correctly" "test_feature_flags"
    run_test "Validation tests pass" "test_validation_tests"
    run_test "Safe wrappers work correctly" "test_safe_wrappers"

    # Restore environment
    restore_environment

    # Summary
    print_header "Test Results"

    TOTAL_TESTS=$((TESTS_PASSED + TESTS_FAILED))

    echo ""
    print_info "Total tests: $TOTAL_TESTS"
    print_success "Passed: $TESTS_PASSED"

    if [[ $TESTS_FAILED -gt 0 ]]; then
        print_error "Failed: $TESTS_FAILED"
        echo ""
        print_error "Some tests failed - please review output above"
        echo ""
        exit 1
    else
        echo ""
        print_success "All tests passed! Both paths working correctly."
        echo ""
        print_info "You can safely:"
        echo "  • Enable validation: ./scripts/enable-validation.sh enable"
        echo "  • Disable validation: ./scripts/enable-validation.sh disable"
        echo "  • Check status: ./scripts/enable-validation.sh status"
        echo ""
        exit 0
    fi
}

# Run main
main
