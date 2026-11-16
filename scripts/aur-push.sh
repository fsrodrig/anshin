#!/usr/bin/env bash
# Identical files need no new commit on retry; older releases must never replace newer packages.
set -euo pipefail

if [ $# -lt 2 ] || [ $# -gt 3 ]; then
  echo "usage: $0 <version> <dir with PKGBUILD and .SRCINFO> [<remote>]" >&2
  exit 2
fi
version=$1 dir=$2 remote=${3-ssh://aur@aur.archlinux.org/anshin-agent-bin.git}
root=$(cd "$(dirname "$0")/.." && pwd)
: "${AUR_SSH_PRIVATE_KEY:?is not set}"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
(umask 077 && printf '%s\n' "$AUR_SSH_PRIVATE_KEY" > "$work/key")
export GIT_SSH_COMMAND="ssh -i '$work/key' -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile='$root/deploy/aur/known_hosts' -o GlobalKnownHostsFile=/dev/null"

git clone -q "$remote" "$work/aur"
if [ -f "$work/aur/PKGBUILD" ]; then
  remote_version=$(sed -n 's/^pkgver=//p' "$work/aur/PKGBUILD")
  newest=$(printf '%s\n' "$version" "$remote_version" | sort -V | tail -n 1)
  if [ "$remote_version" != "$version" ] && [ "$newest" = "$remote_version" ]; then
    echo "aur-push: the AUR has newer version $remote_version; skipping $version"
    exit 0
  fi
fi
cp "$dir/PKGBUILD" "$dir/.SRCINFO" "$work/aur/"
cd "$work/aur"
git add PKGBUILD .SRCINFO
if git diff --cached --quiet; then
  echo "aur-push: the AUR already has these files for $version"
  exit 0
fi
git -c user.name="Anshin release" -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
  -c commit.gpgsign=false commit -q -m "Update to $version"
git push -q origin HEAD:master
