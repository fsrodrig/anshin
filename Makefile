# `make check` is the Done gate, and CI runs the same targets. Tools are pinned in mise.toml; `mise install` gets
# them, or install the same versions yourself. Each component adds its own target (`check-agent`,
# `check-android`) to `check` in the pull request that creates it.

.PHONY: check check-repo check-brand check-agent check-android setup

check: check-repo check-brand check-agent check-android

check-repo:
	scripts/commit-title.test.sh
	scripts/changed-components.test.sh
	scripts/identifier-scan.test.sh
	scripts/identifier-scan.sh
	shellcheck scripts/*.sh
	actionlint
	zizmor --offline --min-severity low .github
	lychee $$(git ls-files '*.md')
	git-cliff --unreleased --bump > /dev/null

check-brand:
	scripts/brand-check.test.sh
	scripts/brand-check.sh

AGENT_ARCHES := amd64 arm64
# docs/design.md § Budgets.
AGENT_MAX_BYTES := 10000000

# -race needs cgo; the shipped binaries are built without it.
check-agent:
	cd agent && unformatted=$$(gofmt -l .) && [ -z "$$unformatted" ] || { echo "gofmt needed: $$unformatted" >&2; exit 1; }
	cd agent && go vet ./...
	cd agent && go tool staticcheck ./...
	cd agent && go test -race ./...
	cd agent && for arch in $(AGENT_ARCHES); do \
	  out=dist/anshin-agent-linux-$$arch; \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags='-s -w' -o $$out ./cmd/anshin-agent || exit 1; \
	  size=$$(stat -c %s $$out); echo "$$out: $$size bytes"; \
	  [ $$size -le $(AGENT_MAX_BYTES) ] || { echo "$$out is over the $(AGENT_MAX_BYTES)-byte budget" >&2; exit 1; }; \
	done

# Gradle runs on the JDK 21 pinned in android/gradle/gradle-daemon-jvm.properties, downloading it if the host has
# another JDK, so any JDK can launch the wrapper. --warning-mode=fail keeps deprecations from piling up unseen.
# verifyRoborazziDebug runs every unit test and fails on any screenshot that differs from its committed golden.
check-android:
	cd android && ./gradlew --warning-mode=fail lintDebug verifyRoborazziDebug assembleDebug

# Rerun after a lefthook version bump: the hook calls the installed lefthook binary.
setup:
	lefthook install
