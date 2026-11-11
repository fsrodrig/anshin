# `make check` is the Done gate, and CI runs the same targets. Tools are pinned in mise.toml; `mise install` gets
# them, or install the same versions yourself. Each component adds its own target (`check-agent`,
# `check-android`) to `check` in the pull request that creates it.

.PHONY: check check-repo setup

check: check-repo

check-repo:
	scripts/commit-title.test.sh
	scripts/identifier-scan.test.sh
	scripts/identifier-scan.sh
	shellcheck scripts/*.sh
	actionlint
	zizmor --offline --min-severity low .github
	lychee $$(git ls-files '*.md')
	git-cliff --unreleased --bump > /dev/null

# Rerun after a lefthook version bump: the hook calls the installed lefthook binary.
setup:
	lefthook install
