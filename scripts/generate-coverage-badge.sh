#!/bin/bash

# Generate coverage badge script for test coverage reporting
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COVERAGE_FILE="${1:-coverage-combined.out}"
BADGE_OUTPUT="${2:-coverage-badge.svg}"
README_FILE="${3:-README.md}"

print_status() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${NC}"
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

# Function to determine badge color based on coverage
get_badge_color() {
    local coverage=$1
    
    if (( $(echo "$coverage >= 80" | bc -l 2>/dev/null || echo "0") )); then
        echo "brightgreen"
    elif (( $(echo "$coverage >= 70" | bc -l 2>/dev/null || echo "0") )); then
        echo "green"
    elif (( $(echo "$coverage >= 60" | bc -l 2>/dev/null || echo "0") )); then
        echo "yellow"
    elif (( $(echo "$coverage >= 50" | bc -l 2>/dev/null || echo "0") )); then
        echo "orange"
    else
        echo "red"
    fi
}

# Function to generate SVG badge
generate_svg_badge() {
    local coverage=$1
    local color=$2
    local output_file=$3
    
    # Simple SVG badge template
    cat > "$output_file" << EOF
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="104" height="20">
<linearGradient id="b" x2="0" y2="100%">
<stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
<stop offset="1" stop-opacity=".1"/>
</linearGradient>
<clipPath id="a">
<rect width="104" height="20" rx="3" fill="#fff"/>
</clipPath>
<g clip-path="url(#a)">
<path fill="#555" d="M0 0h63v20H0z"/>
<path fill="$(get_shield_color $color)" d="M63 0h41v20H63z"/>
<path fill="url(#b)" d="M0 0h104v20H0z"/>
</g>
<g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="110">
<text x="325" y="15" fill="#010101" fill-opacity=".3" transform="scale(.1)" textLength="530">coverage</text>
<text x="325" y="14" transform="scale(.1)" textLength="530">coverage</text>
<text x="825" y="15" fill="#010101" fill-opacity=".3" transform="scale(.1)" textLength="310">${coverage}%</text>
<text x="825" y="14" transform="scale(.1)" textLength="310">${coverage}%</text>
</g>
</svg>
EOF
}

# Function to get shield color code
get_shield_color() {
    local color=$1
    case $color in
        "brightgreen") echo "#4c1" ;;
        "green") echo "#97ca00" ;;
        "yellow") echo "#dfb317" ;;
        "orange") echo "#fe7d37" ;;
        "red") echo "#e05d44" ;;
        *) echo "#9f9f9f" ;;
    esac
}

# Function to generate shields.io URL
generate_shields_url() {
    local coverage=$1
    local color=$2
    
    # URL encode the coverage percentage
    local encoded_coverage=$(echo "$coverage" | sed 's/%/%25/g')
    
    echo "https://img.shields.io/badge/coverage-${encoded_coverage}%25-${color}"
}

# Function to update README with coverage badge
update_readme_badge() {
    local readme_file=$1
    local badge_url=$2
    
    if [[ -f "$readme_file" ]]; then
        print_status $BLUE "Updating coverage badge in $readme_file..."
        
        # Check if coverage badge already exists
        if grep -q "coverage-.*-.*" "$readme_file"; then
            # Replace existing badge
            sed -i.bak "s|!\[Coverage\](https://img\.shields\.io/badge/coverage-[^)]*)|![Coverage]($badge_url)|g" "$readme_file"
            print_status $GREEN "✓ Updated existing coverage badge"
        elif grep -q "!\[.*\].*shields\.io.*coverage" "$readme_file"; then
            # Replace any existing coverage badge format
            sed -i.bak "s|!\[[^]]*\]([^)]*shields\.io[^)]*coverage[^)]*)|![Coverage]($badge_url)|g" "$readme_file"
            print_status $GREEN "✓ Replaced existing coverage badge"
        else
            # Add new badge after main title
            if grep -q "^# " "$readme_file"; then
                # Find first main heading and add badge after it
                awk -v badge="![Coverage]($badge_url)" '
                /^# / && !badge_added {
                    print $0
                    print ""
                    print badge
                    print ""
                    badge_added = 1
                    next
                }
                { print }
                ' "$readme_file" > "$readme_file.tmp" && mv "$readme_file.tmp" "$readme_file"
                print_status $GREEN "✓ Added new coverage badge"
            else
                # Prepend badge to file
                echo "![Coverage]($badge_url)" > "$readme_file.tmp"
                echo "" >> "$readme_file.tmp"
                cat "$readme_file" >> "$readme_file.tmp"
                mv "$readme_file.tmp" "$readme_file"
                print_status $GREEN "✓ Added coverage badge at top"
            fi
        fi
        
        # Clean up backup file if it exists
        [[ -f "$readme_file.bak" ]] && rm "$readme_file.bak"
    else
        print_status $BLUE "README file $readme_file not found, creating badge info file"
        echo "![Coverage]($badge_url)" > "coverage-badge.md"
        print_status $GREEN "✓ Created coverage-badge.md with badge"
    fi
}

# Main function
main() {
    print_status $BLUE "=== Coverage Badge Generator ==="
    
    # Check if coverage file exists
    if [[ ! -f "$COVERAGE_FILE" ]]; then
        print_status $RED "✗ Coverage file not found: $COVERAGE_FILE"
        exit 1
    fi
    
    # Calculate coverage
    print_status $BLUE "Calculating coverage from $COVERAGE_FILE..."
    coverage=$(calculate_coverage "$COVERAGE_FILE")
    
    if [[ -z "$coverage" || "$coverage" == "" ]]; then
        print_status $RED "✗ Could not determine coverage percentage"
        exit 1
    fi
    
    print_status $GREEN "✓ Coverage: ${coverage}%"
    
    # Determine badge color
    color=$(get_badge_color "$coverage")
    print_status $BLUE "Badge color: $color"
    
    # Generate shields.io URL
    badge_url=$(generate_shields_url "$coverage" "$color")
    print_status $BLUE "Badge URL: $badge_url"
    
    # Generate SVG badge (optional)
    if command -v curl >/dev/null 2>&1; then
        print_status $BLUE "Downloading SVG badge..."
        if curl -s "$badge_url" -o "$BADGE_OUTPUT"; then
            print_status $GREEN "✓ SVG badge saved: $BADGE_OUTPUT"
        else
            print_status $BLUE "Could not download SVG badge, generating simple version"
            generate_svg_badge "$coverage" "$color" "$BADGE_OUTPUT"
            print_status $GREEN "✓ Simple SVG badge generated: $BADGE_OUTPUT"
        fi
    else
        generate_svg_badge "$coverage" "$color" "$BADGE_OUTPUT"
        print_status $GREEN "✓ Simple SVG badge generated: $BADGE_OUTPUT"
    fi
    
    # Create badge information file
    cat > "coverage-badge-info.txt" << EOF
Coverage: ${coverage}%
Color: $color
SVG File: $BADGE_OUTPUT
Shields.io URL: $badge_url
Markdown: ![Coverage]($badge_url)
HTML: <img src="$badge_url" alt="Coverage ${coverage}%" />
EOF
    
    print_status $GREEN "✓ Badge info saved: coverage-badge-info.txt"
    
    # Update README if requested and file exists
    if [[ "$README_FILE" != "" ]]; then
        update_readme_badge "$README_FILE" "$badge_url"
    fi
    
    # Output for CI/CD use
    echo "COVERAGE_PERCENT=$coverage" >> "${GITHUB_OUTPUT:-/dev/null}" 2>/dev/null || true
    echo "COVERAGE_COLOR=$color" >> "${GITHUB_OUTPUT:-/dev/null}" 2>/dev/null || true
    echo "COVERAGE_BADGE_URL=$badge_url" >> "${GITHUB_OUTPUT:-/dev/null}" 2>/dev/null || true
    
    print_status $GREEN "=== Coverage badge generation complete ==="
}

# Show help if requested
if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
    echo "Usage: $0 [COVERAGE_FILE] [BADGE_OUTPUT] [README_FILE]"
    echo ""
    echo "Generate coverage badge from Go coverage profile"
    echo ""
    echo "Arguments:"
    echo "  COVERAGE_FILE   Coverage profile file (default: coverage-combined.out)"
    echo "  BADGE_OUTPUT    SVG badge output file (default: coverage-badge.svg)"
    echo "  README_FILE     README file to update with badge (default: README.md)"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Use defaults"
    echo "  $0 coverage.out                      # Custom coverage file"
    echo "  $0 coverage.out badge.svg           # Custom output file"
    echo "  $0 coverage.out badge.svg README.md # Update README"
    echo ""
    echo "Environment variables for CI/CD:"
    echo "  GITHUB_OUTPUT   Set coverage info for GitHub Actions"
    exit 0
fi

# Run main function
main "$@"