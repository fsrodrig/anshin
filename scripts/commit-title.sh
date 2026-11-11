#!/usr/bin/env bash
# Checks that a PR title is a Conventional Commit. The title becomes the squash commit on main and its
# release-note line (cliff.toml), so this is the only commit message that has to follow the rules.
set -euo pipefail

types='build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test'
# Scopes are optional but closed, so release notes tell an Obtainium user whether the phone changed.
scopes='agent|android|contract|deploy|brand|docs|ci|deps|security'
max_length=100

title=${1-}
if [ -z "$title" ]; then
  echo "usage: $0 \"<type>(<scope>)!: <subject>\"" >&2
  exit 2
fi

fail() {
  printf 'PR title "%s": %s\n' "$title" "$1" >&2
  printf 'Expected "<type>[(<scope>)][!]: <subject>", type one of %s, scope one of %s.\n' \
    "${types//|/, }" "${scopes//|/, }" >&2
  exit 1
}

[ "${#title}" -le "$max_length" ] || fail "longer than $max_length characters"

if [[ ! $title =~ ^([a-z]+)(\(([a-z-]+)\))?(!)?:\ (.+)$ ]]; then
  fail "not in Conventional Commit form"
fi
type=${BASH_REMATCH[1]}
scope=${BASH_REMATCH[3]}
subject=${BASH_REMATCH[5]}

[[ $type =~ ^($types)$ ]] || fail "unknown type \"$type\""
[ -z "$scope" ] || [[ $scope =~ ^($scopes)$ ]] || fail "unknown scope \"$scope\""
[[ ! $subject =~ ^[[:space:]] ]] || fail "subject starts with a space"
[[ ! $subject =~ ^[A-Z][a-z] ]] || fail "subject starts with a capitalised word; start lower case"
[[ ! $subject =~ \.$ ]] || fail "subject ends with a period"
