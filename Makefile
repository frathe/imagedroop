APP_NAME := Image Drop
APP_ID   := com.frathe.image_drop
BIN_NAME := image_drop
ICON     := assets/appIcon.png
BIN_DIR  := bin
WIN_ARCH := amd64
LINUX_ARCHES := amd64 arm64

.PHONY: all build build-linux-all run fmt vet test golden tidy clean package-mac package-windows package-windows-debug package-linux package-linux-debug build-all install-tools install-linux-tools security security-govulncheck security-github bump-version help

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
	fyne package -os darwin -icon $(ICON) -name "$(APP_NAME)" -appID $(APP_ID) -release
	python3 scripts/patch_macos_document_types.py "$(APP_NAME).app/Contents/Info.plist"
	mkdir -p $(BIN_DIR)
	rm -rf "$(BIN_DIR)/$(APP_NAME).app"
	mv "$(APP_NAME).app" "$(BIN_DIR)/"

package-windows: ## Cross-compile a Windows .exe via fyne-cross (needs Docker) into bin/ (stripped by default)
	fyne-cross windows -arch=$(WIN_ARCH) -icon $(ICON) -name $(BIN_NAME) -app-id $(APP_ID) -env GOTOOLCHAIN=auto
	mkdir -p $(BIN_DIR)
	cp fyne-cross/bin/windows-$(WIN_ARCH)/$(BIN_NAME).exe $(BIN_DIR)/

package-windows-debug: ## Cross-compile a console-subsystem, unstripped Windows .exe for diagnosing startup failures
	fyne-cross windows -arch=$(WIN_ARCH) -icon $(ICON) -name $(BIN_NAME)-debug -app-id $(APP_ID) -env GOTOOLCHAIN=auto -console -no-strip-debug
	mkdir -p $(BIN_DIR)
	cp fyne-cross/bin/windows-$(WIN_ARCH)/$(BIN_NAME)-debug.exe $(BIN_DIR)/

package-linux: ## Cross-compile Linux binaries via fyne-cross (needs Docker) into bin/, one per arch in LINUX_ARCHES (stripped by default)
	mkdir -p $(BIN_DIR)
	for arch in $(LINUX_ARCHES); do \
		fyne-cross linux -arch=$$arch -icon $(ICON) -name $(BIN_NAME) -app-id $(APP_ID) -env GOTOOLCHAIN=auto || exit 1; \
		cp fyne-cross/bin/linux-$$arch/* $(BIN_DIR)/$(BIN_NAME)-linux-$$arch; \
	done

package-linux-debug: ## Cross-compile unstripped Linux binaries for diagnosing startup failures, one per arch in LINUX_ARCHES
	mkdir -p $(BIN_DIR)
	for arch in $(LINUX_ARCHES); do \
		fyne-cross linux -arch=$$arch -icon $(ICON) -name $(BIN_NAME)-debug -app-id $(APP_ID) -env GOTOOLCHAIN=auto -no-strip-debug || exit 1; \
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

bump-version: ## Bump FyneApp.toml version (PART=major|minor|patch, default patch) and tag HEAD as vX.Y.Z
	@version=$$(sed -nE 's/^Version = "(.*)"/\1/p' FyneApp.toml); \
	build=$$(sed -nE 's/^Build = ([0-9]+)/\1/p' FyneApp.toml); \
	part=$${PART:-patch}; \
	major=$$(echo $$version | cut -d. -f1); \
	minor=$$(echo $$version | cut -d. -f2); \
	patch=$$(echo $$version | cut -d. -f3); \
	case $$part in \
		major) major=$$((major+1)); minor=0; patch=0 ;; \
		minor) minor=$$((minor+1)); patch=0 ;; \
		patch) patch=$$((patch+1)) ;; \
		*) echo "Unknown PART=$$part (want major|minor|patch)"; exit 1 ;; \
	esac; \
	new_version=$$major.$$minor.$$patch; \
	new_build=$$((build+1)); \
	tag="v$$new_version"; \
	if git rev-parse "$$tag" >/dev/null 2>&1; then \
		echo "Tag $$tag already exists"; exit 1; \
	fi; \
	sed -i.bak -E "s/^Version = \".*\"/Version = \"$$new_version\"/" FyneApp.toml; \
	sed -i.bak -E "s/^Build = [0-9]+/Build = $$new_build/" FyneApp.toml; \
	rm -f FyneApp.toml.bak; \
	git tag -a "$$tag" -m "Release $$tag" HEAD; \
	echo "Bumped version $$version -> $$new_version (build $$build -> $$new_build)"; \
	echo "Tagged current HEAD ($$(git rev-parse --short HEAD)) as $$tag"; \
	echo; \
	echo "FyneApp.toml was updated but NOT committed (this Makefile never commits)."; \
	echo "Note: $$tag points at HEAD as it was BEFORE this edit, so it does not yet"; \
	echo "include the FyneApp.toml change. Suggested commit message:"; \
	echo "  Bump version to $$new_version"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*##"}; {printf "  %-16s %s\n", $$1, $$2}'
