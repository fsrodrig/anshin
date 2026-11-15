#!/usr/bin/env bash
# Publishes GitHub Release <tag> at <commit> with <notes file> and every <asset>, through gh. The draft holds every
# asset before it is published, and publishing it is what creates the tag, so a tag never exists without its
# assets. A draft left by a failed run is reused, so the job can be re-run; a published release is never changed.
set -euo pipefail

if [ $# -lt 4 ]; then
  echo "usage: $0 <tag> <commit> <notes file> <asset>..." >&2
  exit 2
fi
tag=$1 commit=$2 notes=$3
shift 3

fail() {
  echo "release-publish: $1" >&2
  exit 1
}

if ! draft=$(gh release view "$tag" --json isDraft --jq .isDraft 2> /dev/null); then
  gh release create "$tag" --draft --target "$commit" --title "$tag" --notes-file "$notes" "$@"
elif [ "$draft" = true ]; then
  gh release edit "$tag" --target "$commit" --title "$tag" --notes-file "$notes"
  gh release upload "$tag" --clobber "$@"
else
  fail "$tag is already published"
fi

want=$(for asset in "$@"; do basename "$asset"; done | LC_ALL=C sort | paste -sd ' ')
got=$(gh release view "$tag" --json assets --jq '.assets[].name' | LC_ALL=C sort | paste -sd ' ')
[ "$got" = "$want" ] || fail "the draft of $tag holds $got, want $want"

gh release edit "$tag" --draft=false --latest
pointed=$(gh api "repos/{owner}/{repo}/git/ref/tags/$tag" --jq .object.sha)
[ "$pointed" = "$commit" ] || fail "$tag points at $pointed, want $commit"
