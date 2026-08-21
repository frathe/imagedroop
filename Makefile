APP_NAME := PicFetch
PACKAGE_ID := io.github.frathe.picfetch
BIN_NAME := picfetch
ICON     := assets/appIcon.png
BIN_DIR  := bin
WIN_ARCHES := amd64 arm64
LINUX_ARCHES := amd64 arm64

RELEASE_BRANCH := main

.PHONY: all build build-linux-all run fmt vet test verify golden tidy clean package-mac package-windows package-windows-debug package-linux package-linux-debug build-all install-tools install-linux-tools security security-govulncheck security-github bump-version release help

all: build

build: ## Build a native binary for the current OS/arch into bin/ (stripped, no debug symbols)
	mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/$(BIN_NAME) .

run: ## Run the app directly (go run .)
	go run .

fmt: ## Format all Go source files
	gofmt -l -w .

vet: ## Run go vet
	go vet ./...

test: ## Run tests
	go test ./...

verify: ## Run the same checks CI does (gofmt, vet, build, race tests)
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files need gofmt (run 'make fmt'):"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...
	go build ./...
	go test -race ./...

golden: ## Regenerate the e2e golden-master screenshots via Docker (linux/amd64, matching CI exactly - needs Docker)
	@# Fyne's software rasterizer renders slightly different anti-aliased
	@# pixels depending on CPU architecture - fyne.io/fyne/v2's own test
	@# harness even special-cases darwin/arm64 for it. A master captured by
	@# running `go test` directly on a non-amd64-Linux machine can pass there
	@# and still fail in CI, which runs on ubuntu-latest/amd64 with no such
	@# leniency - this target renders in the same environment CI does so the
	@# result is never machine-dependent. See CONTRIBUTING.md for the full
	@# accept-a-new-master workflow.
	docker run --rm --platform linux/amd64 \
		-v "$(CURDIR):/work" -w /work \
		-e HOST_UID=$$(id -u) -e HOST_GID=$$(id -g) \
		ubuntu:24.04 bash -c '\
			set -e; \
			apt-get update -qq; \
			apt-get install -y -qq gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev golang-go ca-certificates >/dev/null; \
			go test -run TestE2E ./internal/ui/... -v || true; \
			if [ -d internal/ui/testdata/failed ]; then chown -R "$$HOST_UID:$$HOST_GID" internal/ui/testdata/failed; fi \
		'
	@echo "Inspect internal/ui/testdata/failed/*.png (if any), and if they look right, copy the ones you want over the matching internal/ui/testdata/*.png to accept them as the new baseline."

tidy: ## Tidy go.mod / go.sum
	go mod tidy

security-govulncheck: ## Scan dependencies for known Go vulnerabilities (govulncheck)
	govulncheck ./...

security-github: ## List open GitHub Dependabot alerts for this repo (needs `gh auth login`)
	gh api "repos/$$(gh repo view --json nameWithOwner -q .nameWithOwner)/dependabot/alerts" \
		--jq '.[] | select(.state=="open") | "\(.security_advisory.severity)\t\(.dependency.package.name)\t\(.security_advisory.summary)"'

security: security-govulncheck security-github ## Run all security checks (govulncheck + GitHub Dependabot alerts)

clean: ## Remove all build artifacts
	rm -rf $(BIN_DIR) fyne-cross "$(APP_NAME).app" "$(BIN_NAME).zip"

package-mac: ## Package a macOS .app bundle (native, no Docker) into bin/
	fyne package -os darwin -icon $(ICON) -name "$(APP_NAME)" -appID $(PACKAGE_ID) -release
	python3 scripts/patch_macos_document_types.py "$(APP_NAME).app/Contents/Info.plist"
	mkdir -p $(BIN_DIR)
	rm -rf "$(BIN_DIR)/$(APP_NAME).app"
	mv "$(APP_NAME).app" "$(BIN_DIR)/"

package-windows: ## Cross-compile Windows .exe files via fyne-cross (needs Docker) into bin/, one per arch in WIN_ARCHES (stripped by default)
	mkdir -p $(BIN_DIR)
	for arch in $(WIN_ARCHES); do \
		fyne-cross windows -arch=$$arch -icon $(ICON) -name $(BIN_NAME) -app-id $(PACKAGE_ID) -env GOTOOLCHAIN=auto || exit 1; \
		cp fyne-cross/bin/windows-$$arch/$(BIN_NAME).exe $(BIN_DIR)/$(BIN_NAME)-windows-$$arch.exe; \
	done

package-windows-debug: ## Cross-compile console-subsystem, unstripped Windows .exe files for diagnosing startup failures, one per arch in WIN_ARCHES
	mkdir -p $(BIN_DIR)
	for arch in $(WIN_ARCHES); do \
		fyne-cross windows -arch=$$arch -icon $(ICON) -name $(BIN_NAME)-debug -app-id $(PACKAGE_ID) -env GOTOOLCHAIN=auto -console -no-strip-debug || exit 1; \
		cp fyne-cross/bin/windows-$$arch/$(BIN_NAME)-debug.exe $(BIN_DIR)/$(BIN_NAME)-debug-windows-$$arch.exe; \
	done

package-linux: ## Cross-compile Linux binaries via fyne-cross (needs Docker) into bin/, one per arch in LINUX_ARCHES (stripped by default)
	mkdir -p $(BIN_DIR)
	for arch in $(LINUX_ARCHES); do \
		fyne-cross linux -arch=$$arch -icon $(ICON) -name $(BIN_NAME) -app-id $(PACKAGE_ID) -env GOTOOLCHAIN=auto || exit 1; \
		cp fyne-cross/bin/linux-$$arch/* $(BIN_DIR)/$(BIN_NAME)-linux-$$arch; \
	done

package-linux-debug: ## Cross-compile unstripped Linux binaries for diagnosing startup failures, one per arch in LINUX_ARCHES
	mkdir -p $(BIN_DIR)
	for arch in $(LINUX_ARCHES); do \
		fyne-cross linux -arch=$$arch -icon $(ICON) -name $(BIN_NAME)-debug -app-id $(PACKAGE_ID) -env GOTOOLCHAIN=auto -no-strip-debug || exit 1; \
		cp fyne-cross/bin/linux-$$arch/* $(BIN_DIR)/$(BIN_NAME)-debug-linux-$$arch; \
	done

build-linux-all: package-linux ## Alias for package-linux: cross-compile Linux binaries for all LINUX_ARCHES via fyne-cross (needs Docker)

build-all: package-mac package-windows package-linux ## Build release artifacts for macOS, Windows, and Linux

install-tools: ## Install the fyne, fyne-cross, and govulncheck CLI tools
	go install fyne.io/fyne/v2/cmd/fyne@latest
	go install github.com/fyne-io/fyne-cross@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

install-linux-tools: ## Install apt dev headers needed to build natively on Linux (OpenGL, X11, Wayland; needs sudo)
	sudo apt-get update
	sudo apt-get install -y gcc libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev

bump-version: ## Bump FyneApp.toml's Version/Build only (PART=major|minor|patch, default patch); no commit, no tag
	@scripts/bump_version.sh $${PART:-patch} >/dev/null
	@echo "FyneApp.toml was updated but NOT committed. Use 'make release' for the full flow."

release: ## Full release: verify, bump version, commit, tag, push (PART=major|minor|patch, default patch; YES=1 skips the prompt)
	@# The tag must contain its own version bump, so this target commits the
	@# FyneApp.toml edit before tagging - the one place the Makefile writes to
	@# git history. Publishing happens in .github/workflows/release.yml, which
	@# is triggered by the tag push and re-runs CI as a gate, so a red run
	@# leaves the tag orphaned rather than shipping a broken build.
	@set -e; \
	part=$${PART:-patch}; \
	branch=$$(git rev-parse --abbrev-ref HEAD); \
	if [ "$$branch" != "$(RELEASE_BRANCH)" ]; then \
		echo "On branch '$$branch', expected '$(RELEASE_BRANCH)' (override with RELEASE_BRANCH=)"; exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Working tree is dirty - commit or stash first:"; git status --short; exit 1; \
	fi; \
	git fetch --quiet origin "$(RELEASE_BRANCH)"; \
	if [ "$$(git rev-parse HEAD)" != "$$(git rev-parse origin/$(RELEASE_BRANCH))" ]; then \
		echo "HEAD and origin/$(RELEASE_BRANCH) have diverged - pull/push first"; exit 1; \
	fi; \
	new_version=$$(scripts/bump_version.sh $$part --dry-run); \
	tag="v$$new_version"; \
	if git rev-parse -q --verify "refs/tags/$$tag" >/dev/null || \
	   [ -n "$$(git ls-remote --tags origin "refs/tags/$$tag")" ]; then \
		echo "Tag $$tag already exists"; exit 1; \
	fi; \
	if [ -z "$$YES" ]; then \
		printf "Release %s from %s (%s)? [y/N] " "$$tag" "$(RELEASE_BRANCH)" "$$(git rev-parse --short HEAD)"; \
		read answer; \
		case $$answer in y|Y|yes|YES) ;; *) echo "Aborted."; exit 1 ;; esac; \
	fi; \
	$(MAKE) verify; \
	scripts/bump_version.sh $$part >/dev/null; \
	git add FyneApp.toml; \
	git commit -m "Release $$tag"; \
	git tag -a "$$tag" -m "Release $$tag"; \
	git push origin "$(RELEASE_BRANCH)"; \
	git push origin "$$tag"; \
	echo "Pushed $$tag - .github/workflows/release.yml now builds and publishes the artifacts."; \
	if command -v gh >/dev/null 2>&1; then \
		gh run watch --exit-status; \
	fi

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  %-16s %s\n", $$1, $$2}'
