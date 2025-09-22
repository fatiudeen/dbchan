#!/bin/bash

# Script to bump chart and app versions in Helm Chart.yaml
# Usage: ./scripts/bump-version.sh [chart|app|both] [patch|minor|major]

set -e

CHART_FILE="charts/dbchan/Chart.yaml"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [chart|app|both] [patch|minor|major]"
    echo ""
    echo "Arguments:"
    echo "  chart|app|both  - Which version to bump"
    echo "  patch|minor|major - Version bump type"
    echo ""
    echo "Examples:"
    echo "  $0 chart patch    # Bump chart version patch (0.1.0 -> 0.1.1)"
    echo "  $0 app minor      # Bump app version minor (v.0.1.0 -> v.0.2.0)"
    echo "  $0 both major     # Bump both versions major"
    echo ""
    echo "Current versions:"
    if [ -f "$PROJECT_ROOT/$CHART_FILE" ]; then
        echo "  Chart version: $(grep '^version:' "$PROJECT_ROOT/$CHART_FILE" | awk '{print $2}')"
        echo "  App version: $(grep '^appVersion:' "$PROJECT_ROOT/$CHART_FILE" | awk '{print $2}')"
    else
        echo "  Chart file not found at $PROJECT_ROOT/$CHART_FILE"
    fi
}

# Function to parse version
parse_version() {
    local version="$1"
    # Remove 'v' prefix if present and handle 'v.' format
    version="${version#v}"
    version="${version#.}"
    echo "$version"
}

# Function to bump version
bump_version() {
    local version="$1"
    local bump_type="$2"
    
    # Parse version into parts
    IFS='.' read -r major minor patch <<< "$version"
    
    case "$bump_type" in
        "patch")
            patch=$((patch + 1))
            ;;
        "minor")
            minor=$((minor + 1))
            patch=0
            ;;
        "major")
            major=$((major + 1))
            minor=0
            patch=0
            ;;
        *)
            print_error "Invalid bump type: $bump_type"
            return 1
            ;;
    esac
    
    echo "$major.$minor.$patch"
}

# Function to update chart version
update_chart_version() {
    local new_version="$1"
    local chart_file="$PROJECT_ROOT/$CHART_FILE"
    
    if [ ! -f "$chart_file" ]; then
        print_error "Chart file not found: $chart_file"
        return 1
    fi
    
    # Update version in Chart.yaml
    sed -i.bak "s/^version: .*/version: $new_version/" "$chart_file"
    rm "$chart_file.bak"
    
    print_success "Updated chart version to $new_version"
}

# Function to update app version
update_app_version() {
    local new_version="$1"
    local chart_file="$PROJECT_ROOT/$CHART_FILE"
    
    if [ ! -f "$chart_file" ]; then
        print_error "Chart file not found: $chart_file"
        return 1
    fi
    
    # Add 'v' prefix to app version
    local app_version="v.$new_version"
    
    # Update appVersion in Chart.yaml
    sed -i.bak "s/^appVersion: .*/appVersion: \"$app_version\"/" "$chart_file"
    rm "$chart_file.bak"
    
    print_success "Updated app version to $app_version"
}

# Main function
main() {
    local target="$1"
    local bump_type="$2"
    
    # Change to project root
    cd "$PROJECT_ROOT"
    
    # Validate arguments
    if [ -z "$target" ] || [ -z "$bump_type" ]; then
        print_error "Missing required arguments"
        show_usage
        exit 1
    fi
    
    if [[ ! "$target" =~ ^(chart|app|both)$ ]]; then
        print_error "Invalid target: $target. Must be 'chart', 'app', or 'both'"
        show_usage
        exit 1
    fi
    
    if [[ ! "$bump_type" =~ ^(patch|minor|major)$ ]]; then
        print_error "Invalid bump type: $bump_type. Must be 'patch', 'minor', or 'major'"
        show_usage
        exit 1
    fi
    
    # Check if Chart.yaml exists
    if [ ! -f "$CHART_FILE" ]; then
        print_error "Chart file not found: $CHART_FILE"
        exit 1
    fi
    
    print_info "Bumping $target version ($bump_type)"
    
    # Get current versions
    local current_chart_version=$(grep '^version:' "$CHART_FILE" | awk '{print $2}')
    local current_app_version=$(grep '^appVersion:' "$CHART_FILE" | awk '{print $2}' | tr -d '"')
    
    print_info "Current chart version: $current_chart_version"
    print_info "Current app version: $current_app_version"
    
    # Bump versions based on target
    case "$target" in
        "chart")
            local new_chart_version=$(bump_version "$current_chart_version" "$bump_type")
            print_info "New chart version: $new_chart_version"
            update_chart_version "$new_chart_version"
            ;;
        "app")
            local parsed_app_version=$(parse_version "$current_app_version")
            local new_app_version=$(bump_version "$parsed_app_version" "$bump_type")
            print_info "New app version: v.$new_app_version"
            update_app_version "$new_app_version"
            ;;
        "both")
            local new_chart_version=$(bump_version "$current_chart_version" "$bump_type")
            local parsed_app_version=$(parse_version "$current_app_version")
            local new_app_version=$(bump_version "$parsed_app_version" "$bump_type")
            
            print_info "New chart version: $new_chart_version"
            print_info "New app version: v.$new_app_version"
            
            update_chart_version "$new_chart_version"
            update_app_version "$new_app_version"
            ;;
    esac
    
    print_success "Version bump completed successfully!"
    
    # Show final versions
    local final_chart_version=$(grep '^version:' "$CHART_FILE" | awk '{print $2}')
    local final_app_version=$(grep '^appVersion:' "$CHART_FILE" | awk '{print $2}')
    
    echo ""
    print_info "Final versions:"
    echo "  Chart version: $final_chart_version"
    echo "  App version: $final_app_version"
}

# Run main function with all arguments
main "$@"
