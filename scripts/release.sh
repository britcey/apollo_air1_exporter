#!/bin/bash
set -e

# Release script for Apollo Air-1 Exporter

if [ -z "$1" ]; then
    echo "Usage: $0 <version>"
    echo "Example: $0 0.1.1"
    exit 1
fi

VERSION="${1#v}"
TAG="v$VERSION"

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Expected a version such as 0.1.1 or v0.1.1"
    exit 1
fi

echo "Preparing release $TAG..."

# Check if on main branch
BRANCH=$(git branch --show-current)
if [ "$BRANCH" != "main" ]; then
    echo "Error: Must be on main branch to release (currently on $BRANCH)"
    exit 1
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo "Error: There are uncommitted changes"
    exit 1
fi

# Update the release baseline used by Makefile release helpers.
printf '%s\n' "$VERSION" > VERSION

echo "Running checks..."
make check

# Update CHANGELOG
echo "Please update CHANGELOG.md with a heading in this format:"
echo "## $VERSION / $(date -u +%F)"
echo "Press Enter when done..."
read

# Commit release metadata
git add VERSION CHANGELOG.md
git commit -m "chore: prepare release $TAG"

# Create and push tag
git tag -a "$TAG" -m "Release $TAG"

echo ""
echo "Release $TAG prepared!"
echo ""
echo "Push the commit and tag to start the GitHub Actions release workflow:"
echo "  git push origin main"
echo "  git push origin $TAG"
echo ""
echo "The workflow creates the GitHub Release and multi-architecture images."