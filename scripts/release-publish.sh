#!/usr/bin/env bash
# Publishing creates the tag, so all assets must be in the draft first. Reuse failed drafts;
# published releases are immutable, and retries must prove their commit and assets match.
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
fi

want=$(for asset in "$@"; do basename "$asset"; done | LC_ALL=C sort | paste -sd ' ')
got=$(gh release view "$tag" --json assets --jq '.assets[].name' | LC_ALL=C sort | paste -sd ' ')
[ "$got" = "$want" ] || fail "$tag holds $got, want $want"

if [ "${draft-}" = false ]; then
  pointed=$(gh api "repos/{owner}/{repo}/git/ref/tags/$tag" --jq .object.sha)
  [ "$pointed" = "$commit" ] || fail "$tag points at $pointed, want $commit"
  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT
  gh release download "$tag" --dir "$work"
  for asset in "$@"; do
    cmp -s "$asset" "$work/$(basename "$asset")" || fail "$tag has different content for $(basename "$asset")"
  done
  echo "release-publish: $tag is already published with the expected commit and assets"
  exit 0
fi

gh release edit "$tag" --draft=false --latest
pointed=$(gh api "repos/{owner}/{repo}/git/ref/tags/$tag" --jq .object.sha)
[ "$pointed" = "$commit" ] || fail "$tag points at $pointed, want $commit"
