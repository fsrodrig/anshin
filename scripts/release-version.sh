#!/usr/bin/env bash
# Prints `version=X.Y.Z` and `tag=vX.Y.Z` for the next release and writes its notes to <notes file>, both from
# git-cliff and the Conventional Commit titles merged since the last tag (ADR 0007). Fails when none of them would
# appear in the notes, because that release would ship nothing new.
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <notes file>" >&2
  exit 2
fi
notes=$1
config="$(cd "$(dirname "$0")/.." && pwd)/cliff.toml"

tag=$(git-cliff --config "$config" --bumped-version)
if [[ ! $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "release-version: git-cliff proposed \"$tag\", not vX.Y.Z" >&2
  exit 1
fi
# git-cliff proposes the last tag again when nothing since it bumps the version.
if git rev-parse -q --verify "refs/tags/$tag" > /dev/null; then
  echo "release-version: nothing to release: no feat, fix or breaking change since $tag" >&2
  exit 1
fi
git-cliff --config "$config" --unreleased --tag "$tag" --strip header --output "$notes"
echo "version=${tag#v}"
echo "tag=$tag"
