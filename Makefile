.DEFAULT_GOAL = help

APP_EXT := $(if $(filter Windows_NT,$(OS)),.exe)

PROJECT_DIR = $(CURDIR)

# Tooling (linter binary, git hooks) is shared across all git worktrees by
# anchoring it to the common git dir, so a worktree doesn't need its own
# `make install-deps` and `.install-hooks` works where `.git` is a file, not a
# dir. Falls back to the project dir when built outside a git checkout.
GIT_COMMON_DIR := $(abspath $(shell git -C $(CURDIR) rev-parse --git-common-dir 2>/dev/null))
TOOLS_DIR = $(if $(GIT_COMMON_DIR),$(GIT_COMMON_DIR),$(PROJECT_DIR))
GIT_HOOKS_DIR = $(if $(GIT_COMMON_DIR),$(GIT_COMMON_DIR)/hooks,$(PROJECT_DIR)/.git/hooks)
PROJECT_BIN = $(TOOLS_DIR)/bin
GOLANGCI_LINT ?= $(PROJECT_BIN)/golangci-lint$(APP_EXT)

# Go's buildvcs stamping shells out to git and resolves the wrong repo root
# inside a linked worktree (where .git is a gitfile), failing with exit 128.
# Disable stamping only in a linked worktree; the main checkout and CI keep it.
BUILDVCS_FLAG := $(if $(filter $(PROJECT_DIR)/.git,$(GIT_COMMON_DIR)),,-buildvcs=false)
GOLANGCI_LINT_VERSION = v2.9.0
PROTOC_GEN_GO_VERSION = v1.36.11
PROTOC_GEN_GO_GRPC_VERSION = v1.6.2

BUILD_DIR = $(PROJECT_DIR)/build
LINUX_PLATFORMS = linux/amd64 linux/arm64
MACOS_PLATFORMS = darwin/arm64

HOST_UNAME := $(shell uname -s)

ifeq ($(HOST_UNAME),Darwin)
DEFAULT_BUILD_TARGET = build-macos
DEFAULT_PLATFORMS = $(MACOS_PLATFORMS)
else
DEFAULT_BUILD_TARGET = build-linux
DEFAULT_PLATFORMS = $(LINUX_PLATFORMS)
endif

BUILD_PLATFORMS ?= $(DEFAULT_PLATFORMS)

##
## Build:
.PHONY: build
build: ## - Build all binaries for the current host platform set
	$(MAKE) $(DEFAULT_BUILD_TARGET)

.PHONY: .build-all
.build-all: build-app1

.PHONY: build-linux
build-linux: ## - Build all binaries for Linux
	@$(MAKE) .build-all BUILD_PLATFORMS="$(LINUX_PLATFORMS)"

.PHONY: build-macos
build-macos: ## - Build all binaries for macOS
	@$(MAKE) .build-all BUILD_PLATFORMS="$(MACOS_PLATFORMS)"

.PHONY: build-app1
build-app1: ## - Build app1 for configured platforms
build-app1: clean ensure-build-dir
	@$(foreach platform,$(BUILD_PLATFORMS),\
		$(eval OS := $(word 1,$(subst /, ,$(platform)))) \
		$(eval ARCH := $(word 2,$(subst /, ,$(platform)))) \
		echo "Building app1 for $(OS)/$(ARCH)..." && \
		(cd ./apps/app1 && CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build $(BUILDVCS_FLAG) -o $(BUILD_DIR)/app1-$(OS)-$(ARCH) .);)

.PHONY: clean
clean:
	@echo "Cleaning build artifacts at $(BUILD_DIR)..."
	@rm -rf $(BUILD_DIR)

.PHONY: ensure-build-dir
ensure-build-dir:
	@echo "Ensuring build directory exists at $(BUILD_DIR)..."
	@mkdir -p $(BUILD_DIR)

##
## Tests:
.PHONY: test
test: ## - Run tests for all services
	@echo "Running tests for root module..."
	@go test -v ./...

##
## Modules:
.PHONY: mod-download
mod-download: ## - Run go mod download in all modules
	@echo "Running go mod download on all modules..."
	@find . -name "go.mod" -exec dirname {} \; | while read dir; do \
		echo "go mod download in $$dir"; \
		(cd $$dir && go mod download) || exit 1; \
	done
	@echo "All modules downloaded successfully."

.PHONY: mod-tidy
mod-tidy: ## - Run go mod tidy in all modules
	@echo "Running go mod tidy on all modules..."
	@find . -name "go.mod" -exec dirname {} \; | while read dir; do \
		echo "go mod tidy in $$dir"; \
		(cd $$dir && go mod tidy) || exit 1; \
	done
	@echo "All modules tidied successfully."

.PHONY: mod-update
mod-update: ## - Update all dependencies (go get -u) and tidy in all modules
	@echo "Updating dependencies in all modules..."
	@find . -name "go.mod" -exec dirname {} \; | while read dir; do \
		echo "go get -u ./... in $$dir"; \
		(cd $$dir && go get -u ./... && go mod tidy) || exit 1; \
	done
	@echo "All modules updated successfully."

##
## Other:
.PHONY: help
help: ## - Show help message
	@awk '\
		BEGIN { \
			blue   = "\033[34m"; \
			yellow = "\033[33m"; \
			reset  = "\033[0m"; \
		} \
		/^##[[:space:]]+/ { \
			title = $$0; \
			sub(/^##[[:space:]]+/, "", title); \
			printf "\n%s%s%s\n", yellow, title, reset; \
			next; \
		} \
		/^[^[:space:]#][^:]*:.*##[[:space:]]+/ { \
			target = desc = $$0; \
			sub(/:.*/, "", target); \
			sub(/^.*##[[:space:]]+/, "", desc); \
			printf "    %s%-20s%s %s\n", blue, target, reset, desc; \
		} \
	' $(MAKEFILE_LIST)
	@printf '\n'
