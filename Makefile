# `make check` is the Done gate, and CI runs the same targets. Tools are pinned in mise.toml; `mise install` gets
# them, or install the same versions yourself. Each component adds its own target (`check-agent`,
# `check-android`) to `check` in the pull request that creates it.

.PHONY: check check-repo check-agent check-android setup

check: check-repo check-agent check-android

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

AGENT_ARCHES := amd64 arm64
# docs/design.md § Budgets.
AGENT_MAX_BYTES := 10000000

# -race needs cgo; the shipped binaries are built without it. Go's test cache doesn't notice changes outside the
# module, so the tests that read contract/ run again uncached. The -update line and the one after it regenerate
# contract/fixtures and fail on any difference, untracked files included, so commit fixture changes before running
# this locally. The two `!` lines keep "never samples the host" checkable: live adapters live only in host.go, and
# no test-only replay code reaches the binary.
check-agent:
	cd agent && unformatted=$$(gofmt -l .) && [ -z "$$unformatted" ] || { echo "gofmt needed: $$unformatted" >&2; exit 1; }
	cd agent && go vet ./...
	cd agent && go tool staticcheck ./...
	cd agent && go test -race ./...
	cd agent && go test -count=1 -run '^(TestGolden|TestProbesOnCaptures)' ./internal/server ./internal/recorder/probe
	cd agent && go test -count=1 -run '^TestGolden' ./internal/server -update
	@changed=$$(git status --porcelain -- contract/fixtures) && [ -z "$$changed" ] || { printf '%s\n' "$$changed" \
	  "contract/fixtures differs from what the Agent emits from contract/captures: review git diff, then commit" >&2; exit 1; }
	! git grep -nE 'os\.DirFS\("/"\)|unix\.(Statfs|Statx|ClockGettime)\(' -- agent ':!agent/cmd/anshin-agent/host.go'
	! (cd agent && go list -deps ./cmd/anshin-agent) | grep -xE 'testing(/.*)?|.*/internal/capture/(replay|capturetest)'
	cd agent && for arch in $(AGENT_ARCHES); do \
	  out=dist/anshin-agent-linux-$$arch; \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags='-s -w' -o $$out ./cmd/anshin-agent || exit 1; \
	  size=$$(stat -c %s $$out); echo "$$out: $$size bytes"; \
	  [ $$size -le $(AGENT_MAX_BYTES) ] || { echo "$$out is over the $(AGENT_MAX_BYTES)-byte budget" >&2; exit 1; }; \
	done

# Gradle runs on the JDK 21 pinned in android/gradle/gradle-daemon-jvm.properties, downloading it if the host has
# another JDK, so any JDK can launch the wrapper. --warning-mode=fail keeps deprecations from piling up unseen.
check-android:
	cd android && ./gradlew --warning-mode=fail lintDebug testDebugUnitTest assembleDebug

# Rerun after a lefthook version bump: the hook calls the installed lefthook binary.
setup:
	lefthook install
