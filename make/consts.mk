.DEFAULT_GOAL = help

APP_EXT := $(if $(filter Windows_NT,$(OS)),.exe)

# Tooling (linter binary, git hooks) is shared across all git worktrees by
# anchoring it to the common git dir, so a worktree doesn't need its own
# `make install-deps` and `.install-hooks` works where `.git` is a file, not a
# dir. Falls back to the project dir when built outside a git checkout.
GIT_COMMON_DIR := $(abspath $(shell git -C $(CURDIR) rev-parse --git-common-dir 2>/dev/null))
TOOLS_DIR = $(if $(GIT_COMMON_DIR),$(GIT_COMMON_DIR),$(CURDIR))
GIT_HOOKS_DIR = $(if $(GIT_COMMON_DIR),$(GIT_COMMON_DIR)/hooks,$(CURDIR)/.git/hooks)
PROJECT_BIN = $(TOOLS_DIR)/bin
GOLANGCI_LINT ?= $(PROJECT_BIN)/golangci-lint$(APP_EXT)

# Go's buildvcs stamping shells out to git and resolves the wrong repo root
# inside a linked worktree (where .git is a gitfile), failing with exit 128.
# Disable stamping only in a linked worktree; the main checkout and CI keep it.
BUILDVCS_FLAG := $(if $(filter $(CURDIR)/.git,$(GIT_COMMON_DIR)),,-buildvcs=false)
GOLANGCI_LINT_VERSION = v2.9.0
PROTOC_GEN_GO_VERSION = v1.36.11
PROTOC_GEN_GO_GRPC_VERSION = v1.6.2

BUILD_DIR = $(CURDIR)/build
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
