#!/bin/bash

# Script to test the CI/CD workflow locally
# This simulates the version management logic without pushing to remote

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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
    echo "Usage: $0 [patch|minor|major] [--dry-run]"
    echo ""
    echo "Arguments:"
    echo "  patch|minor|major  - Version bump type"
    echo "  --dry-run          - Show what would be done without making changes"
    echo ""
    echo "Examples:"
    echo "  $0 patch           # Test patch version bump"
    echo "  $0 minor --dry-run # Show what minor bump would do"
    echo "  $0 major           # Test major version bump"
}

# Parse arguments
BUMP_TYPE=""
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        patch|minor|major)
            BUMP_TYPE="$1"
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            show_usage
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

if [ -z "$BUMP_TYPE" ]; then
    print_error "Version bump type is required"
    show_usage
    exit 1
fi

# Change to project root
cd "$PROJECT_ROOT"

print_info "Testing workflow for $BUMP_TYPE version bump"
if [ "$DRY_RUN" = true ]; then
    print_warning "DRY RUN MODE - No changes will be made"
fi

# Get current versions
CHART_VERSION=$(grep '^version:' charts/dbchan/Chart.yaml | awk '{print $2}')
APP_VERSION=$(grep '^appVersion:' charts/dbchan/Chart.yaml | awk '{print $2}' | tr -d '"')

print_info "Current chart version: $CHART_VERSION"
print_info "Current app version: $APP_VERSION"

# Check if tag exists
TAG_EXISTS=$(git tag -l "v$CHART_VERSION" | wc -l)
if [ "$TAG_EXISTS" -gt 0 ]; then
    print_warning "Tag v$CHART_VERSION already exists"
else
    print_info "Tag v$CHART_VERSION does not exist"
fi

# Calculate new version
IFS='.' read -r major minor patch <<< "$CHART_VERSION"

case "$BUMP_TYPE" in
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
esac

NEW_CHART_VERSION="$major.$minor.$patch"
NEW_APP_VERSION="v$NEW_CHART_VERSION"
NEW_TAG="v$NEW_CHART_VERSION"

print_info "New chart version: $NEW_CHART_VERSION"
print_info "New app version: $NEW_APP_VERSION"
print_info "New tag: $NEW_TAG"

if [ "$DRY_RUN" = true ]; then
    print_info "DRY RUN - Would update Chart.yaml:"
    echo "  version: $CHART_VERSION -> $NEW_CHART_VERSION"
    echo "  appVersion: \"$APP_VERSION\" -> \"$NEW_APP_VERSION\""
    print_info "DRY RUN - Would update values.yaml:"
    echo "  image.tag: \"\" -> \"$NEW_APP_VERSION\""
    print_info "DRY RUN - Would create and push tag: $NEW_TAG"
    print_info "DRY RUN - Would build image with tag: $NEW_APP_VERSION"
    print_info "DRY RUN - Would build and push chart to OCI registry: oci://registry-1.docker.io/fatiudeen/dbchan-chart:$NEW_CHART_VERSION"
else
    # Make actual changes
    print_info "Updating Chart.yaml..."
    sed -i.bak "s/^version: .*/version: $NEW_CHART_VERSION/" charts/dbchan/Chart.yaml
    sed -i.bak "s/^appVersion: .*/appVersion: \"$NEW_APP_VERSION\"/" charts/dbchan/Chart.yaml
    rm charts/dbchan/Chart.yaml.bak
    
    print_info "Updating values.yaml..."
    sed -i.bak "s/^      tag: .*/      tag: \"$NEW_APP_VERSION\"/" charts/dbchan/values.yaml
    rm charts/dbchan/values.yaml.bak
    
    print_success "Chart.yaml updated successfully"
    
    # Show git status
    print_info "Git status:"
    git status --porcelain
    
    # Ask for confirmation before committing
    echo ""
    read -p "Do you want to commit these changes? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        git add charts/dbchan/Chart.yaml
        git commit -m "Bump version to $NEW_CHART_VERSION ($BUMP_TYPE)"
        print_success "Changes committed successfully"
        
        # Ask about creating tag
        read -p "Do you want to create and push tag $NEW_TAG? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            git tag $NEW_TAG
            print_success "Tag $NEW_TAG created"
            print_info "To push the tag, run: git push origin $NEW_TAG"
        fi
    else
        print_info "Changes not committed. You can review them with: git diff"
    fi
fi

print_success "Workflow test completed!"
