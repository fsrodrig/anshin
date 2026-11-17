# `make check` is the Done gate, and CI runs the same targets. Tools are pinned in mise.toml; `mise install` gets
# them, or install the same versions yourself. Each component adds its own target (`check-agent`,
# `check-android`) to `check` in the pull request that creates it.

.PHONY: check check-repo check-brand check-agent check-deploy check-android setup

check: check-repo check-brand check-agent check-deploy check-android

check-repo:
	scripts/commit-title.test.sh
	scripts/changed-components.test.sh
	scripts/identifier-scan.test.sh
	scripts/identifier-scan.sh
	scripts/hujson-check.sh
	scripts/unit-check.test.sh
	scripts/aur-render.test.sh
	scripts/archives-check.test.sh
	scripts/release-version.test.sh
	scripts/release-publish.test.sh
	scripts/aur-push.test.sh
	shellcheck scripts/*.sh scripts/qa deploy/aur/PKGBUILD
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
	! git grep -nE 'os\.DirFS\("/"\)|unix\.(Statfs|Fstatfs|Statx|ClockGettime|Uname)\(' -- agent ':!agent/cmd/anshin-agent/host.go'
	! (cd agent && go list -deps ./cmd/anshin-agent) | grep -xE 'testing(/.*)?|.*/internal/capture/(replay|capturetest)'

	@# The QA Agent's raw mode answers without authorization, so the QA tooling must never reach the shipped binary.
	cd agent && deps=$$(go list -deps ./cmd/anshin-agent) && ! printf '%s\n' "$$deps" | grep -E '/internal/qa$$|/cmd/anshin-qa' \
	  || { echo "anshin-agent depends on the QA tooling" >&2; exit 1; }
	scripts/qa help > /dev/null
	cd agent && for arch in $(AGENT_ARCHES); do \
	  out=dist/anshin-agent-linux-$$arch; \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags='-s -w' -o $$out ./cmd/anshin-agent || exit 1; \
	  size=$$(stat -c %s $$out); echo "$$out: $$size bytes"; \
	  [ $$size -le $(AGENT_MAX_BYTES) ] || { echo "$$out is over the $(AGENT_MAX_BYTES)-byte budget" >&2; exit 1; }; \
	done

# docs/design.md § Packaging and release. scripts/unit-check.sh pins the unit's directives; the threshold is the 5.3
# exposure that set scores in `systemd-analyze security` on systemd 255 to 262, in tenths, so a weaker unit fails here
# even if the pin is edited with it. The snapshot is built and checked exactly as the Release workflow builds and
# checks a release's archives, so the PKGBUILD and goreleaser's archive names cannot drift apart unnoticed.
UNIT := deploy/systemd/anshin-agent.service
UNIT_MAX_EXPOSURE := 53

check-deploy:
	scripts/unit-check.sh $(UNIT)
	@out=$$(systemd-analyze security --offline=true --threshold=$(UNIT_MAX_EXPOSURE) --no-pager $(UNIT) 2>&1) \
	  || { printf '%s\n' "$$out" >&2; echo "$(UNIT) exposes more than $(UNIT_MAX_EXPOSURE)/100" >&2; exit 1; }; \
	  printf '%s\n' "$$out" | tail -n 1
	goreleaser check
	goreleaser release --snapshot --clean
	scripts/archives-check.sh 0.0.0.snapshot dist

# docs/design.md § Budgets, checked on the unsigned APK; signing adds a few tens of KB, which the sign job prints.
APK_MAX_BYTES := 5000000
APK := android/app/build/outputs/apk/release/app-release-unsigned.apk
APK_MAPPING := android/app/build/outputs/mapping/release/mapping.txt
# Empty for pull requests and local builds, which makes a development build. The Release workflow passes the version
# it cuts. Recipes read it from the environment, so the value never becomes shell syntax.
RELEASE_VERSION ?=
export RELEASE_VERSION

# Gradle runs on the JDK 21 pinned in android/gradle/gradle-daemon-jvm.properties, downloading it if the host has
# another JDK, so any JDK can launch the wrapper. --warning-mode=fail keeps deprecations from piling up unseen.
# verifyRoborazziDebug runs every unit test and fails on any screenshot that differs from its committed golden.
# The 1.2.3 build proves the version wiring on every Android change, so a release run is never its first test. The
# RELEASE_VERSION build runs last, so the APK left behind (for QA, a developer or the Release workflow) is the one the
# checks after it just passed.
check-android:
	cd android && ./gradlew --warning-mode=fail lintDebug verifyRoborazziDebug assembleDebug
	scripts/apk-version.test.sh
	cd android && ./gradlew --warning-mode=fail assembleRelease -Panshin.version=1.2.3
	scripts/apk-check.sh $(APK) $(APK_MAPPING) $(APK_MAX_BYTES) 1.2.3
	cd android && ./gradlew --warning-mode=fail assembleRelease $${RELEASE_VERSION:+"-Panshin.version=$$RELEASE_VERSION"}
	scripts/apk-check.sh $(APK) $(APK_MAPPING) $(APK_MAX_BYTES) $${RELEASE_VERSION:+"$$RELEASE_VERSION"}
	scripts/apk-check.test.sh $(APK) $(APK_MAPPING) $${RELEASE_VERSION:+"$$RELEASE_VERSION"}
	scripts/apk-sign.test.sh $(APK)

# Rerun after a lefthook version bump: the hook calls the installed lefthook binary.
setup:
	lefthook install
