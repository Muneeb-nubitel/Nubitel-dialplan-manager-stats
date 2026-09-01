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

# Keep development tags in separate series:
#   minor -> v0.<minor>.0
#   major -> v<major>.0.0
case "$VERSION" in
  minor)
    TAG_PATTERN='v0.*.0'
    TAG_REGEX='^v0\.([0-9]+)\.0$'
    ;;
  major)
    TAG_PATTERN='v*.0.0'
    TAG_REGEX='^v([0-9]+)\.0\.0$'
    ;;
  *)
    echo "Choose a version type: -v major or -v minor"
    exit 1
    ;;
esac

find_matching_tag() {
  while IFS= read -r tag; do
    if [[ "$tag" =~ $TAG_REGEX ]]; then
      echo "$tag"
      return
    fi
  done < <(git tag "$@" --list "$TAG_PATTERN" --sort=-version:refname)
}

# Reuse a tag from the relevant series when this commit was already tagged.
EXISTING_TAG=$(find_matching_tag --points-at HEAD)
if [[ -n "$EXISTING_TAG" ]]; then
  echo "This commit already has tag: $EXISTING_TAG"
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "git-tag=$EXISTING_TAG" >> "$GITHUB_OUTPUT"
  fi
  exit 0
fi

# Find the highest version in the relevant series.
CURRENT_VERSION=$(find_matching_tag)
if [[ -z "$CURRENT_VERSION" ]]; then
  CURRENT_VERSION="v0.0.0"
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
  *)
    echo "Choose a version type: -v major or -v minor"
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
