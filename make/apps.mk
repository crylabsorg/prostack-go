# Every apps/<name>/go.mod adds build-<name>, run-<name>, and test-<name>.
APPS_DIR := $(ROOT_DIR)/apps
APP_DIRS := $(patsubst %/,%,$(dir $(wildcard $(APPS_DIR)/*/go.mod)))
APP_NAMES := $(notdir $(APP_DIRS))

# Go's VCS stamping can fail when the system is checked out as a linked worktree.
GIT_COMMON_DIR := $(shell git -C "$(ROOT_DIR)" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)
BUILDVCS_FLAG := $(if $(filter $(ROOT_DIR)/.git,$(GIT_COMMON_DIR)),,-buildvcs=false)

## Build:
.PHONY: build build-linux build-macos clean
build: $(addprefix build-,$(APP_NAMES)) ## Build all apps for BUILD_PLATFORMS (host by default)

build-linux: BUILD_PLATFORMS := $(LINUX_PLATFORMS)
build-linux: build ## Build all apps for Linux amd64 and arm64

build-macos: BUILD_PLATFORMS := $(MACOS_PLATFORMS)
build-macos: build ## Build all apps for macOS amd64 and arm64

clean: ## Remove build artifacts
	@rm -rf "$(BUILD_DIR)"

## Tests:
.PHONY: test
test: $(addprefix test-,$(APP_NAMES)) ## Test all apps

ifneq ($(strip $(APP_NAMES)),)
.PHONY: $(addprefix build-,$(APP_NAMES)) $(addprefix run-,$(APP_NAMES)) $(addprefix test-,$(APP_NAMES))

$(addprefix build-,$(APP_NAMES)):
	@mkdir -p "$(BUILD_DIR)"
	@app="$(patsubst build-%,%,$@)"; for platform in $(BUILD_PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "Building $$app for $$os/$$arch..."; \
		(cd "$(APPS_DIR)/$$app" && CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build $(BUILDVCS_FLAG) -o "$(BUILD_DIR)/$$app-$$os-$$arch" .) || exit $$?; \
	done

$(addprefix run-,$(APP_NAMES)):
	@cd "$(APPS_DIR)/$(patsubst run-%,%,$@)" && go run .

$(addprefix test-,$(APP_NAMES)):
	@cd "$(APPS_DIR)/$(patsubst test-%,%,$@)" && go test ./...
endif
