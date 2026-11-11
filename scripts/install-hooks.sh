#!/bin/sh
# Git hooks are shared by every worktree, and `lefthook install` writes the installing checkout's path into
# them. Installing only from the main checkout keeps that path valid after a worktree is removed.
set -eu
git_dir=$(git rev-parse --path-format=absolute --git-dir)
common_dir=$(git rev-parse --path-format=absolute --git-common-dir)
if [ "$git_dir" != "$common_dir" ]; then
  echo "Run \`make setup\` from the main checkout; hooks installed there cover every worktree." >&2
  exit 0
fi
exec lefthook install
