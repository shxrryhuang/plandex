#!/bin/bash

# Plandex Validation System - Enable/Test Script
# This script helps you enable and test the validation system safely

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Emojis
CHECK="✅"
CROSS="❌"
GEAR="⚙️"
ROCKET="🚀"
WARNING="⚠️"

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  ${GEAR}  Plandex Configuration Validation System"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Function to print colored output
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

# Function to check if validation is currently enabled
check_current_state() {
    if [[ "${PLANDEX_ENABLE_VALIDATION}" == "true" ]]; then
        print_info "Current state: Validation ENABLED"
        return 0
    else
        print_info "Current state: Validation DISABLED (original behavior)"
        return 1
    fi
}

# Function to show help
show_help() {
    cat << EOF
Usage: $0 [COMMAND]

Commands:
    enable              Enable validation system (all features)
    disable             Disable validation system (original behavior)
    enable-dev          Enable validation with development settings
    enable-staging      Enable validation with staging settings
    enable-prod         Enable validation with production settings
    test                Test validation with current settings
    status              Show current validation status
    rollback            Disable validation (alias for disable)
    help                Show this help message

Examples:
    # Enable validation (all features)
    $0 enable

    # Enable with development settings (verbose, strict)
    $0 enable-dev

    # Test current configuration
    $0 test

    # Disable validation
    $0 disable

Environment Variables:
    PLANDEX_ENABLE_VALIDATION      Master switch (true/false)
    PLANDEX_VALIDATION_VERBOSE     Verbose error messages (true/false)
    PLANDEX_VALIDATION_STRICT      Strict mode - warnings block (true/false)
    PLANDEX_VALIDATION_FILE_CHECKS File access checks (true/false)

For more information, see docs/VALIDATION_ROLLOUT.md
EOF
}

# Function to enable validation
enable_validation() {
    local profile="$1"

    echo ""
    print_info "Enabling validation system..."
    echo ""

    case "$profile" in
        "dev")
            print_info "Profile: Development (verbose, strict, thorough)"
            cat > .env.validation << EOF
# Plandex Validation System - Development Profile
PLANDEX_ENABLE_VALIDATION=true
PLANDEX_VALIDATION_VERBOSE=true
PLANDEX_VALIDATION_STRICT=true
PLANDEX_VALIDATION_FILE_CHECKS=true
EOF
            ;;
        "staging")
            print_info "Profile: Staging (verbose, non-strict, fast)"
            cat > .env.validation << EOF
# Plandex Validation System - Staging Profile
PLANDEX_ENABLE_VALIDATION=true
PLANDEX_VALIDATION_VERBOSE=true
PLANDEX_VALIDATION_STRICT=false
PLANDEX_VALIDATION_FILE_CHECKS=false
EOF
            ;;
        "prod")
            print_info "Profile: Production (concise, non-strict, fast)"
            cat > .env.validation << EOF
# Plandex Validation System - Production Profile
PLANDEX_ENABLE_VALIDATION=true
PLANDEX_VALIDATION_VERBOSE=false
PLANDEX_VALIDATION_STRICT=false
PLANDEX_VALIDATION_FILE_CHECKS=false
EOF
            ;;
        *)
            print_info "Profile: Standard (all features enabled)"
            cat > .env.validation << EOF
# Plandex Validation System - Standard Profile
PLANDEX_ENABLE_VALIDATION=true
PLANDEX_VALIDATION_VERBOSE=true
PLANDEX_VALIDATION_STRICT=false
PLANDEX_VALIDATION_FILE_CHECKS=false
EOF
            ;;
    esac

    # Export variables
    export PLANDEX_ENABLE_VALIDATION=true

    case "$profile" in
        "dev")
            export PLANDEX_VALIDATION_VERBOSE=true
            export PLANDEX_VALIDATION_STRICT=true
            export PLANDEX_VALIDATION_FILE_CHECKS=true
            ;;
        "staging")
            export PLANDEX_VALIDATION_VERBOSE=true
            export PLANDEX_VALIDATION_STRICT=false
            export PLANDEX_VALIDATION_FILE_CHECKS=false
            ;;
        "prod")
            export PLANDEX_VALIDATION_VERBOSE=false
            export PLANDEX_VALIDATION_STRICT=false
            export PLANDEX_VALIDATION_FILE_CHECKS=false
            ;;
        *)
            export PLANDEX_VALIDATION_VERBOSE=true
            export PLANDEX_VALIDATION_STRICT=false
            export PLANDEX_VALIDATION_FILE_CHECKS=false
            ;;
    esac

    print_success "Validation system enabled"
    echo ""
    print_info "Configuration saved to: .env.validation"
    echo ""
    print_info "To make this permanent, add to your environment:"
    echo ""
    echo "    source .env.validation"
    echo ""
    print_info "Or add to your shell profile (~/.bashrc, ~/.zshrc):"
    echo ""
    cat .env.validation | sed 's/^/    export /'
    echo ""
}

# Function to disable validation
disable_validation() {
    echo ""
    print_info "Disabling validation system..."
    echo ""

    # Create disabled config
    cat > .env.validation << EOF
# Plandex Validation System - DISABLED
PLANDEX_ENABLE_VALIDATION=false
EOF

    # Unset variables
    unset PLANDEX_ENABLE_VALIDATION
    unset PLANDEX_VALIDATION_VERBOSE
    unset PLANDEX_VALIDATION_STRICT
    unset PLANDEX_VALIDATION_FILE_CHECKS

    print_success "Validation system disabled (original behavior restored)"
    echo ""
    print_info "To make this permanent, unset from your environment:"
    echo ""
    echo "    unset PLANDEX_ENABLE_VALIDATION"
    echo "    unset PLANDEX_VALIDATION_VERBOSE"
    echo "    unset PLANDEX_VALIDATION_STRICT"
    echo "    unset PLANDEX_VALIDATION_FILE_CHECKS"
    echo ""
}

# Function to show current status
show_status() {
    echo ""
    print_info "Current Validation Status:"
    echo ""

    if [[ "${PLANDEX_ENABLE_VALIDATION}" == "true" ]]; then
        print_success "Validation System: ENABLED"
    else
        print_warning "Validation System: DISABLED (original behavior)"
    fi

    echo ""
    print_info "Feature Flags:"

    # Master switch
    if [[ "${PLANDEX_ENABLE_VALIDATION}" == "true" ]]; then
        echo "  ● Master switch: enabled"
    else
        echo "  ○ Master switch: disabled"
    fi

    # Individual flags
    if [[ "${PLANDEX_VALIDATION_VERBOSE}" == "true" ]]; then
        echo "  ● Verbose errors: enabled"
    else
        echo "  ○ Verbose errors: disabled"
    fi

    if [[ "${PLANDEX_VALIDATION_STRICT}" == "true" ]]; then
        echo "  ● Strict mode: enabled"
    else
        echo "  ○ Strict mode: disabled"
    fi

    if [[ "${PLANDEX_VALIDATION_FILE_CHECKS}" == "true" ]]; then
        echo "  ● File checks: enabled"
    else
        echo "  ○ File checks: disabled"
    fi

    echo ""

    # Show environment variables
    print_info "Environment Variables:"
    echo ""
    env | grep "PLANDEX_" | grep -i "validation" || echo "  (none set)"
    echo ""
}

# Function to test validation
test_validation() {
    echo ""
    print_info "Testing validation system..."
    echo ""

    # Check if server binary exists
    if [[ ! -f "./plandex-server" ]]; then
        print_error "plandex-server not found in current directory"
        print_info "Please run this script from the directory containing plandex-server"
        echo ""
        return 1
    fi

    # Show current settings
    if [[ "${PLANDEX_ENABLE_VALIDATION}" == "true" ]]; then
        print_info "Validation is ENABLED - will test with validation"
    else
        print_warning "Validation is DISABLED - will test original behavior"
    fi

    echo ""
    print_info "Starting server test..."
    echo ""
    print_info "Press Ctrl+C to stop the server"
    echo ""
    echo "─────────────────────────────────────────────────────────────"
    echo ""

    # Run server
    ./plandex-server
}

# Main script logic
case "${1:-}" in
    "enable")
        enable_validation "standard"
        ;;
    "enable-dev")
        enable_validation "dev"
        ;;
    "enable-staging")
        enable_validation "staging"
        ;;
    "enable-prod")
        enable_validation "prod"
        ;;
    "disable"|"rollback")
        disable_validation
        ;;
    "test")
        test_validation
        ;;
    "status")
        show_status
        ;;
    "help"|"--help"|"-h")
        show_help
        ;;
    "")
        print_error "No command specified"
        echo ""
        show_help
        exit 1
        ;;
    *)
        print_error "Unknown command: $1"
        echo ""
        show_help
        exit 1
        ;;
esac

echo ""
echo "═══════════════════════════════════════════════════════════════"
echo ""
