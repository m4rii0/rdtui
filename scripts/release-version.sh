#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 2 || ! $1 =~ ^(patch|minor|major)$ || ! $2 =~ ^(rc|stable)$ ]]; then
  printf 'Usage: %s {patch|minor|major} {rc|stable}\n' "$0" >&2
  exit 1
fi
bump=$1
channel=$2

# Capture Git's exit status and tolerate repositories with no matching tags.
tags=$(git tag --sort=-v:refname)
stable_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'
major=0 minor=0 patch=0
while IFS= read -r tag; do
  if [[ $tag =~ $stable_pattern ]]; then
    major=${BASH_REMATCH[1]}
    minor=${BASH_REMATCH[2]}
    patch=${BASH_REMATCH[3]}
    break
  fi
done <<< "$tags"

case "$bump" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
esac
version="v${major}.${minor}.${patch}"

if [[ $channel == rc ]]; then
  rc_pattern='^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-rc\.([1-9][0-9]*)$'
  next_rc=1
  while IFS= read -r tag; do
    if [[ $tag =~ $rc_pattern && ${tag%-rc.*} == "$version" ]]; then
      number=${BASH_REMATCH[4]}
      if (( number >= next_rc )); then
        next_rc=$((number + 1))
      fi
    fi
  done <<< "$tags"
  version+="-rc.${next_rc}"
fi

printf '%s\n' "$version"
