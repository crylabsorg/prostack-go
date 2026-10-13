GOLANGCI_LINT_VERSION := v2.14.0

## Lint:
lint-tools: SHELL := /bin/bash
.PHONY: lint-tools
lint-tools: ## Install pinned golangci-lint into bin/
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@set -o pipefail; \
		curl -sSfL "https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh" | \
		sh -s -- -b "$(ROOT_DIR)/bin" "$(GOLANGCI_LINT_VERSION)"

.PHONY: lint
lint: lint-tools ## Lint selected Go modules
	@set -e; for dir in $(MODULE_DIRS); do \
		echo "Linting $$dir"; \
		(cd "$$dir" && "$(ROOT_DIR)/bin/golangci-lint" run --config "$(ROOT_DIR)/.golangci.yml" ./...); \
	done

.PHONY: lint-fix
lint-fix: lint-tools ## Lint selected Go modules and apply fixes
	@set -e; for dir in $(MODULE_DIRS); do \
		echo "Linting and fixing $$dir"; \
		(cd "$$dir" && "$(ROOT_DIR)/bin/golangci-lint" run --fix --config "$(ROOT_DIR)/.golangci.yml" ./...); \
	done

ifneq ($(strip $(APP_NAMES)),)
.PHONY: $(addprefix lint-,$(APP_NAMES)) $(addprefix lint-fix-,$(APP_NAMES))
$(addprefix lint-,$(APP_NAMES)) $(addprefix lint-fix-,$(APP_NAMES)): lint-tools
	@cd "$(APPS_DIR)/$(patsubst lint-%,%,$(patsubst lint-fix-%,%,$@))" && \
		"$(ROOT_DIR)/bin/golangci-lint" run $(if $(filter lint-fix-%,$@),--fix) --config "$(ROOT_DIR)/.golangci.yml" ./...
endif
