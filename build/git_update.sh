#!/bin/bash

set -euo pipefail

VERSION=""

# Read the requested version change.
while getopts "v:" flag; do
  case "${flag}" in
    v) VERSION=${OPTARG} ;;
  esac
done

# Make sure the runner has the latest remote tags.
git fetch --force --prune --tags origin

# Reuse a v1 tag when this commit was already tagged by an earlier run.
EXISTING_TAG=$(git tag --points-at HEAD --list "v1.*" --sort=-version:refname | sed -n '1p')
if [[ -n "$EXISTING_TAG" ]]; then
  echo "This commit already has tag: $EXISTING_TAG"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "git-tag=$EXISTING_TAG" >> "$GITHUB_OUTPUT"
  fi
  exit 0
fi

# Find the highest v1 version, including patch versions.
CURRENT_VERSION=$(git tag -l "v1.*" --sort=-version:refname | sed -n '1p')
if [[ -z "$CURRENT_VERSION" ]]; then
  CURRENT_VERSION="v1.0.0"
fi

# Stop if the selected tag is not a normal semantic version.
if [[ ! "$CURRENT_VERSION" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "Invalid version tag: $CURRENT_VERSION"
  exit 1
fi

MAJOR=${BASH_REMATCH[1]}
MINOR=${BASH_REMATCH[2]}
PATCH=${BASH_REMATCH[3]}

# Increase the selected part and reset the lower parts.
case "$VERSION" in
  major)
    MAJOR=$((MAJOR + 1))
    MINOR=0
    PATCH=0
    ;;
  minor)
    MINOR=$((MINOR + 1))
    PATCH=0
    ;;
  patch)
    PATCH=$((PATCH + 1))
    ;;
  *)
    echo "Choose a version type: -v major, -v minor, or -v patch"
    exit 1
    ;;
esac

NEW_TAG="v$MAJOR.$MINOR.$PATCH"
echo "Updating $CURRENT_VERSION to $NEW_TAG ($VERSION)"

# Create and push only this tag.
git tag "$NEW_TAG"
git push origin "$NEW_TAG"

# Give the new tag to the Docker image step.
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "git-tag=$NEW_TAG" >> "$GITHUB_OUTPUT"
fi

echo "Created tag: $NEW_TAG"
