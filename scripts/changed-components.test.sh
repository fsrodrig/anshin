#!/usr/bin/env bash
set -euo pipefail
script="$(cd "$(dirname "$0")" && pwd)/changed-components.sh"

repo=$(mktemp -d)
trap 'rm -rf "$repo"' EXIT
cd "$repo"
git init -q
git config user.name test
git config user.email test@example.com
git config commit.gpgsign false

mkdir -p agent/cmd android/app docs .github/workflows branding/themes branding/android/res/values deploy/systemd deploy/aur deploy/tailscale deploy/android scripts
echo 'package main' > agent/cmd/main.go
echo '{}' > branding/palette.json
echo '{}' > branding/themes/tokyo-night.json
echo '<resources/>' > branding/android/res/values/anshin_colors.xml
echo '<html></html>' > branding/board.html
echo 'plugins {}' > android/app/build.gradle.kts
echo '# docs' > docs/readme.md
echo 'name: CI' > .github/workflows/ci.yml
echo '[Unit]' > deploy/systemd/anshin-agent.service
echo 'pkgname=anshin-agent-bin' > deploy/aur/PKGBUILD
echo '{}' > deploy/tailscale/policy.example.hujson
echo '# pin' > deploy/android/release-certificate.sha256
echo '#!/bin/sh' > scripts/apk-sign.sh
echo 'version: 2' > .goreleaser.yaml
echo '#!/bin/sh' > scripts/aur-render.sh
echo '#!/bin/sh' > scripts/aur-render.test.sh
echo 'name: Release' > .github/workflows/release.yml
git add -A
git commit -qm base
base=$(git rev-parse HEAD)

failures=0
# expect <want> <case>: commits whatever the case staged on top of base, compares the output (one line per
# component, joined with spaces), then resets to base.
expect() {
  local want=$1 name=$2 got
  git commit -qm "$name" --allow-empty
  got=$("$script" "$base" | paste -sd ' ')
  if [ "$got" != "$want" ]; then
    printf 'FAIL: %s: expected %s, got %s\n' "$name" "$want" "$got" >&2
    failures=$((failures + 1))
  fi
  git reset -q --hard "$base"
}

everything="agent=true android=true"

echo '// change' >> agent/cmd/main.go && git add -A
expect "agent=true android=false" "edit inside agent/"

echo '// change' >> android/app/build.gradle.kts && git add -A
expect "agent=false android=true" "edit inside android/"

echo 'more' >> docs/readme.md && git add -A
expect "agent=false android=false" "docs only"

git mv agent/cmd/main.go docs/main.go
expect "agent=true android=false" "move a file out of agent/ unchanged"

git mv docs/readme.md agent/readme.md
expect "agent=true android=false" "move a file into agent/ unchanged"

git mv android/app/build.gradle.kts docs/build.gradle.kts
expect "agent=false android=true" "move a file out of android/ unchanged"

echo '{"x":1}' > branding/palette.json && git add -A
expect "agent=false android=true" "Ink & bone palette"

echo '{}' > branding/themes/matte-black.json && git add -A
expect "agent=false android=true" "a theme palette"

echo '<resources></resources>' > branding/android/res/values/anshin_colors.xml && git add -A
expect "agent=false android=true" "shared brand resources"

echo '<html><body></body></html>' > branding/board.html && git add -A
expect "agent=false android=false" "other branding files"

echo 'on: push' > .github/workflows/agent.yml && git add -A
expect "agent=true android=false" "agent workflow"

echo 'on: push' > .github/workflows/android.yml && git add -A
expect "agent=false android=true" "android workflow"

echo '[Service]' >> deploy/systemd/anshin-agent.service && git add -A
expect "agent=true android=false" "the systemd unit"

echo 'pkgrel=2' >> deploy/aur/PKGBUILD && git add -A
expect "agent=true android=false" "the AUR template"

echo '// change' >> deploy/tailscale/policy.example.hujson && git add -A
expect "agent=false android=false" "the tailnet policy example, which check-repo checks"

echo 'dist: dist' >> .goreleaser.yaml && git add -A
expect "agent=true android=false" "goreleaser configuration"

echo 'exit 0' >> scripts/aur-render.sh && git add -A
expect "agent=true android=false" "a script check-deploy runs"

echo 'exit 0' >> scripts/apk-sign.sh && git add -A
expect "agent=false android=true" "a release APK script check-android runs"

echo '#!/bin/sh' > scripts/apk-version.test.sh && git add -A
expect "agent=false android=true" "a release APK test check-android runs"

echo '0000' >> deploy/android/release-certificate.sha256 && git add -A
expect "agent=false android=true" "the APK certificate pin"

echo 'exit 0' >> scripts/aur-render.test.sh && git add -A
expect "agent=false android=false" "a script test, which check-repo runs"

echo 'on: workflow_dispatch' >> .github/workflows/release.yml && git add -A
expect "agent=false android=false" "release workflow"

echo 'on: push' >> .github/workflows/ci.yml && git add -A
expect "$everything" "ci workflow"

mkdir -p contract/fixtures && echo '{}' > contract/fixtures/a.json && git add -A
expect "$everything" "contract only"

echo 'x:' > Makefile && git add -A
expect "$everything" "Makefile"

echo '[tools]' > mise.toml && git add -A
expect "$everything" "mise.toml"

expect "agent=false android=false" "no change"

got=$("$script" 0000000000000000000000000000000000000000 | paste -sd ' ')
if [ "$got" != "$everything" ]; then
  printf 'FAIL: unknown base: expected %s, got %s\n' "$everything" "$got" >&2
  failures=$((failures + 1))
fi

got=$("$script" "" | paste -sd ' ')
if [ "$got" != "$everything" ]; then
  printf 'FAIL: empty base (a push to main): expected %s, got %s\n' "$everything" "$got" >&2
  failures=$((failures + 1))
fi

if [ "$failures" -gt 0 ]; then
  exit 1
fi
echo "changed-components: all cases pass"
