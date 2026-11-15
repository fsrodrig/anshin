#!/usr/bin/env bash
# Runs release-publish.sh against a stand-in gh that keeps one release's state in files.
set -euo pipefail
script="$(cd "$(dirname "$0")" && pwd)/release-publish.sh"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin" "$tmp/dist"
cat > "$tmp/bin/gh" <<'GH'
#!/usr/bin/env bash
set -euo pipefail
state=$GH_STUB_STATE
echo "$*" >> "$state/log"
cmd="$1 $2"
shift 2
case $cmd in
  "release view")
    [ -f "$state/draft" ] || { echo "release not found" >&2; exit 1; }
    if [ "$3" = isDraft ]; then cat "$state/draft"; else cat "$state/assets" 2> /dev/null || true; fi ;;
  "release create" | "release edit" | "release upload")
    shift
    while [ $# -gt 0 ]; do
      case $1 in
        --target) echo "$2" > "$state/target"; shift 2 ;;
        --title | --notes-file) shift 2 ;;
        --draft) echo true > "$state/draft"; shift ;;
        --draft=false) echo false > "$state/draft"; shift ;;
        --clobber | --latest) shift ;;
        *) cp "$1" "$state/$(basename "$1")"; grep -qxF "$(basename "$1")" "$state/assets" 2> /dev/null || basename "$1" >> "$state/assets"; shift ;;
      esac
    done ;;
  "release download")
    shift
    [ "$1" = --dir ]
    cp "$state/checksums.txt" "$state/a.tar.gz" "$2/" ;;
  "api repos/{owner}/{repo}/git/ref/tags/v1.2.3")
    if [ -f "$state/fail-api" ]; then rm "$state/fail-api"; exit 1; fi
    cat "$state/target" ;;
  *) echo "unexpected: gh $cmd $*" >&2; exit 1 ;;
esac
GH
chmod +x "$tmp/bin/gh"
touch "$tmp/notes.md" "$tmp/dist/checksums.txt" "$tmp/dist/a.tar.gz"
export PATH="$tmp/bin:$PATH"
commit=0123456789abcdef0123456789abcdef01234567

failures=0
fail() {
  printf 'FAIL: %s: %s\n' "$case" "$1" >&2
  failures=$((failures + 1))
}
# publish <case> <want exit>: one run of the script against the state the case set up in $GH_STUB_STATE.
publish() {
  local got=0
  "$script" v1.2.3 "$commit" "$tmp/notes.md" "$tmp/dist/checksums.txt" "$tmp/dist/a.tar.gz" > /dev/null 2>&1 || got=$?
  [ "$got" = "$1" ] || fail "exit $got, want $1"
}
state() {
  case=$1
  export GH_STUB_STATE="$tmp/$case"
  mkdir "$GH_STUB_STATE"
}
draft() { cat "$GH_STUB_STATE/draft"; }
logged() { grep -qE "$1" "$GH_STUB_STATE/log"; }

state "no release yet"
publish 0
[ "$(draft)" = false ] || fail "not published"
logged "^release create v1.2.3 --draft --target $commit " || fail "no draft created at $commit"

state "a draft from a failed run"
echo true > "$GH_STUB_STATE/draft"
echo a.tar.gz > "$GH_STUB_STATE/assets"
publish 0
[ "$(draft)" = false ] || fail "not published"
logged '^release upload v1.2.3 --clobber ' || fail "assets not uploaded again"
! logged '^release create' || fail "a second release created"

state "a draft holding an unexpected asset"
echo true > "$GH_STUB_STATE/draft"
printf 'a.tar.gz\nold.tar.gz\n' > "$GH_STUB_STATE/assets"
publish 1
[ "$(draft)" = true ] || fail "published anyway"

state "failure after publication"
touch "$GH_STUB_STATE/fail-api"
publish 1
[ "$(draft)" = false ] || fail "not published before failure"
: > "$GH_STUB_STATE/log"
publish 0
! logged '^release (create|upload|edit)' || fail "a published release changed"

for mismatch in commit names content; do
  state "published with different $mismatch"
  publish 0
  case $mismatch in
    commit) echo another-commit > "$GH_STUB_STATE/target" ;;
    names) echo extra.tar.gz >> "$GH_STUB_STATE/assets" ;;
    content) echo changed > "$GH_STUB_STATE/a.tar.gz" ;;
  esac
  : > "$GH_STUB_STATE/log"
  publish 1
  ! logged '^release (create|upload|edit)' || fail "a published release changed"
done

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "release-publish: all cases pass"
