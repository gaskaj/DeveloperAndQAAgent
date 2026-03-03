#!/bin/bash

# Coverage collection script for comprehensive test coverage reporting
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
UNIT_COVERAGE_FILE="coverage-unit.out"
INTEGRATION_COVERAGE_FILE="coverage-integration.out"
COMBINED_COVERAGE_FILE="coverage-combined.out"
COVERAGE_HTML_FILE="coverage.html"
COVERAGE_REPORT_FILE="coverage-report.txt"
MIN_COVERAGE_OVERALL=80
COVERAGE_DIR="coverage-reports"

# Package-specific minimum coverage thresholds
declare -A PACKAGE_THRESHOLDS=(
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/claude"]=85
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/ghub"]=85
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/developer"]=85
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/config"]=80
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/state"]=80
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/workspace"]=80
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/errors"]=75
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/observability"]=75
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/agent"]=80
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/orchestrator"]=80
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/creativity"]=75
    ["github.com/gaskaj/DeveloperAndQAAgent/internal/gitops"]=75
)

# Function to print colored output
print_status() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${NC}"
}

# Function to setup test environment
setup_test_env() {
    print_status $BLUE "Setting up test environment..."
    
    # Create coverage directory
    mkdir -p "$COVERAGE_DIR"
    
    # Create temporary test directories
    mkdir -p /tmp/test-workspaces
    mkdir -p /tmp/test-state
    
    # Set environment variables for integration tests
    export INTEGRATION_TEST_MODE=true
    export TEST_WORKSPACE_DIR=/tmp/test-workspaces
    export TEST_STATE_DIR=/tmp/test-state
}

# Function to run unit tests with coverage
run_unit_tests() {
    print_status $BLUE "Running unit tests with coverage..."
    
    # Run unit tests (excluding integration tests)
    if go test -short -race -coverprofile="$UNIT_COVERAGE_FILE" -covermode=atomic ./internal/...; then
        print_status $GREEN "✓ Unit tests passed"
    else
        print_status $RED "✗ Unit tests failed"
        return 1
    fi
}

# Function to run integration tests with coverage
run_integration_tests() {
    print_status $BLUE "Running integration tests with coverage..."
    
    # Run integration tests only
    if go test -race -coverprofile="$INTEGRATION_COVERAGE_FILE" -covermode=atomic -timeout=30m ./internal/integration/...; then
        print_status $GREEN "✓ Integration tests passed"
    else
        print_status $RED "✗ Integration tests failed"
        return 1
    fi
}

# Function to combine coverage profiles
combine_coverage() {
    print_status $BLUE "Combining coverage profiles..."
    
    # Start with atomic mode header
    echo "mode: atomic" > "$COMBINED_COVERAGE_FILE"
    
    # Combine unit and integration coverage
    if [[ -f "$UNIT_COVERAGE_FILE" ]]; then
        tail -n +2 "$UNIT_COVERAGE_FILE" >> "$COMBINED_COVERAGE_FILE"
    fi
    
    if [[ -f "$INTEGRATION_COVERAGE_FILE" ]]; then
        tail -n +2 "$INTEGRATION_COVERAGE_FILE" >> "$COMBINED_COVERAGE_FILE"
    fi
    
    print_status $GREEN "✓ Coverage profiles combined"
}

# Function to generate HTML coverage report
generate_html_report() {
    print_status $BLUE "Generating HTML coverage report..."
    
    if go tool cover -html="$COMBINED_COVERAGE_FILE" -o "$COVERAGE_HTML_FILE"; then
        print_status $GREEN "✓ HTML coverage report generated: $COVERAGE_HTML_FILE"
    else
        print_status $RED "✗ Failed to generate HTML coverage report"
        return 1
    fi
}

# Function to calculate coverage percentage
calculate_coverage() {
    local coverage_file=$1
    local coverage_percent=""
    
    if [[ -f "$coverage_file" ]]; then
        coverage_percent=$(go tool cover -func="$coverage_file" | grep "total:" | awk '{print $3}' | sed 's/%//')
    fi
    
    echo "$coverage_percent"
}

# Function to analyze package coverage
analyze_package_coverage() {
    print_status $BLUE "Analyzing per-package coverage..."
    
    {
        echo "# Package Coverage Analysis"
        echo "Generated on: $(date)"
        echo ""
        echo "## Overall Coverage"
        
        local overall_coverage
        overall_coverage=$(calculate_coverage "$COMBINED_COVERAGE_FILE")
        echo "Total coverage: ${overall_coverage}%"
        echo ""
        
        echo "## Per-Package Coverage"
        echo "| Package | Coverage | Threshold | Status |"
        echo "|---------|----------|-----------|--------|"
        
        # Analyze each package
        go tool cover -func="$COMBINED_COVERAGE_FILE" | grep -v "total:" | while read -r line; do
            if [[ $line == *".go:"* ]]; then
                local file=$(echo "$line" | awk '{print $1}')
                local package_path=$(dirname "$file")
                local coverage=$(echo "$line" | awk '{print $3}' | sed 's/%//')
                
                # Find threshold for this package
                local threshold=70  # default threshold
                for pkg_pattern in "${!PACKAGE_THRESHOLDS[@]}"; do
                    if [[ $package_path == *"${pkg_pattern#*internal/}"* ]]; then
                        threshold=${PACKAGE_THRESHOLDS[$pkg_pattern]}
                        break
                    fi
                done
                
                # Determine status
                local status="✓ PASS"
                if (( $(echo "$coverage < $threshold" | bc -l) )); then
                    status="✗ FAIL"
                fi
                
                echo "| $package_path | ${coverage}% | ${threshold}% | $status |"
            fi
        done
        
        echo ""
        echo "## Critical Paths Analysis"
        echo "Identifying uncovered critical code paths:"
        echo ""
        
        # Find uncovered lines in critical packages
        for critical_pkg in "${!PACKAGE_THRESHOLDS[@]}"; do
            if (( ${PACKAGE_THRESHOLDS[$critical_pkg]} >= 85 )); then
                local pkg_short="${critical_pkg#*internal/}"
                echo "### $pkg_short (Critical - ${PACKAGE_THRESHOLDS[$critical_pkg]}% required)"
                
                # Find Go files in this package
                find "./internal/$pkg_short" -name "*.go" -not -name "*_test.go" 2>/dev/null | head -5 | while read -r file; do
                    if [[ -f "$file" ]]; then
                        echo "- $file"
                    fi
                done
                echo ""
            fi
        done
        
    } > "$COVERAGE_REPORT_FILE"
    
    print_status $GREEN "✓ Package coverage analysis completed: $COVERAGE_REPORT_FILE"
}

# Function to check quality gates
check_quality_gates() {
    print_status $BLUE "Checking quality gates..."
    
    local overall_coverage
    overall_coverage=$(calculate_coverage "$COMBINED_COVERAGE_FILE")
    
    if [[ -z "$overall_coverage" ]]; then
        print_status $RED "✗ Could not determine coverage percentage"
        return 1
    fi
    
    local pass=true
    
    # Check overall coverage threshold
    print_status $YELLOW "Overall coverage: ${overall_coverage}% (minimum: ${MIN_COVERAGE_OVERALL}%)"
    if (( $(echo "$overall_coverage < $MIN_COVERAGE_OVERALL" | bc -l) )); then
        print_status $RED "✗ Overall coverage below minimum threshold"
        pass=false
    else
        print_status $GREEN "✓ Overall coverage meets minimum threshold"
    fi
    
    # Check critical package coverage (simplified check)
    print_status $YELLOW "Checking critical package coverage..."
    local critical_failures=0
    
    # For now, we'll do a basic check - can be enhanced with per-package analysis
    go tool cover -func="$COMBINED_COVERAGE_FILE" | grep -E "(claude|ghub|developer)/" | while read -r line; do
        if [[ $line == *".go:"* ]]; then
            local coverage=$(echo "$line" | awk '{print $3}' | sed 's/%//')
            if (( $(echo "$coverage < 85" | bc -l) )); then
                print_status $YELLOW "Warning: Critical package file below 85%: $line"
            fi
        fi
    done
    
    if [[ "$pass" == true ]]; then
        print_status $GREEN "✓ All quality gates passed"
        return 0
    else
        print_status $RED "✗ Quality gates failed"
        return 1
    fi
}

# Function to generate coverage badge
generate_coverage_badge() {
    print_status $BLUE "Generating coverage badge..."
    
    local coverage
    coverage=$(calculate_coverage "$COMBINED_COVERAGE_FILE")
    
    if [[ -n "$coverage" ]]; then
        # Determine badge color based on coverage
        local color="red"
        if (( $(echo "$coverage >= 80" | bc -l) )); then
            color="green"
        elif (( $(echo "$coverage >= 70" | bc -l) )); then
            color="yellow"
        elif (( $(echo "$coverage >= 60" | bc -l) )); then
            color="orange"
        fi
        
        # Generate simple badge info
        echo "Coverage: ${coverage}%" > coverage-badge.txt
        echo "Color: $color" >> coverage-badge.txt
        echo "URL: https://img.shields.io/badge/coverage-${coverage}%25-${color}" >> coverage-badge.txt
        
        print_status $GREEN "✓ Coverage badge info generated: coverage-badge.txt"
    else
        print_status $RED "✗ Could not generate coverage badge"
    fi
}

# Function to cleanup
cleanup() {
    print_status $BLUE "Cleaning up temporary files..."
    rm -rf /tmp/test-workspaces /tmp/test-state
}

# Main execution function
main() {
    local mode="${1:-all}"
    
    print_status $BLUE "=== Coverage Analysis Script ==="
    print_status $BLUE "Mode: $mode"
    
    case "$mode" in
        "unit")
            setup_test_env
            run_unit_tests
            generate_html_report
            ;;
        "integration") 
            setup_test_env
            run_integration_tests
            generate_html_report
            ;;
        "analyze")
            if [[ ! -f "$COMBINED_COVERAGE_FILE" ]]; then
                print_status $RED "No combined coverage file found. Run 'coverage.sh all' first."
                exit 1
            fi
            analyze_package_coverage
            check_quality_gates
            generate_coverage_badge
            ;;
        "all"|"")
            setup_test_env
            
            # Run tests
            if run_unit_tests && run_integration_tests; then
                combine_coverage
                generate_html_report
                analyze_package_coverage
                check_quality_gates
                generate_coverage_badge
            else
                print_status $RED "Tests failed, skipping coverage analysis"
                cleanup
                exit 1
            fi
            ;;
        "help")
            echo "Usage: $0 [mode]"
            echo "Modes:"
            echo "  unit        - Run unit tests with coverage only"
            echo "  integration - Run integration tests with coverage only"
            echo "  analyze     - Analyze existing coverage data"
            echo "  all         - Run all tests and generate full coverage report (default)"
            echo "  help        - Show this help message"
            exit 0
            ;;
        *)
            print_status $RED "Unknown mode: $mode"
            echo "Use '$0 help' for usage information"
            exit 1
            ;;
    esac
    
    cleanup
    print_status $GREEN "=== Coverage analysis complete ==="
}

# Check dependencies
if ! command -v go &> /dev/null; then
    print_status $RED "Error: Go is not installed or not in PATH"
    exit 1
fi

if ! command -v bc &> /dev/null; then
    print_status $YELLOW "Warning: bc calculator not found. Some numeric comparisons may not work."
fi

# Execute main function with all arguments
main "$@"