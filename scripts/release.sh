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

# Update the default binary version.
sed -i.bak "s/^var version = \".*\"/var version = \"$VERSION\"/" main.go
rm main.go.bak

echo "Running checks..."
make check

# Update CHANGELOG
echo "Please update CHANGELOG.md for version $VERSION"
echo "Press Enter when done..."
read

# Commit version bump
git add main.go CHANGELOG.md
git commit -m "chore: bump version to $VERSION"

# Create and push tag
git tag -a "$TAG" -m "Release $TAG"

echo ""
echo "Release $TAG prepared!"
echo ""
echo "To publish the release:"
echo "  git push origin main"
echo "  git push origin $TAG"
echo ""
echo "This will trigger:"
echo "- GitHub Release creation"
echo "- Docker multi-arch builds"
echo "- Binary artifacts upload"