#!/usr/bin/env bash
# Prints `<component>=true|false` for each component whose CI job is path-filtered, comparing <base> with <head>
# (default HEAD). The CI gate accepts a skipped component job only when this says false, so any doubt means true.
set -euo pipefail

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
  echo "usage: $0 <base> [<head>]" >&2
  exit 2
fi
base=$1 head=${2-HEAD}

# An empty base (a push to main, which CI never filters) or one git can't diff against marks everything changed.
# --no-renames lists a moved file under both its old and new path, so moving a file out of a component still counts
# as changing it.
if [ -z "$base" ] || ! changed=$(git diff --no-renames --name-only "$base" "$head" 2>/dev/null); then
  changed=ALL
fi

touches() { [ "$changed" = ALL ] || grep -qE "$1" <<< "$changed"; }

# component <name> [<regex>]: changed if <name>/, its workflow <name>.yml, anything every component shares, or the
# other paths it reads (<regex>) changed.
component() {
  local result=false
  if touches "^($1/|contract/|Makefile\$|mise\\.toml\$|scripts/changed-components\\.sh\$|\\.github/workflows/(ci|$1)\\.yml\$)${2:+|$2}"; then
    result=true
  fi
  echo "$1=$result"
}

component agent
# The Android tests read the palettes and the shared brand resources.
component android '^branding/(palette\.json$|themes/|android/res/)'
