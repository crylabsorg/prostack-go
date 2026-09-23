# The entry point selects modules: app modules at the root, library in prostack-go/.
## Modules:
.PHONY: mod-download mod-tidy mod-update
mod-download: ## Download dependencies for selected modules
	@set -e; for dir in $(MODULE_DIRS); do \
		echo "Downloading dependencies in $$dir"; \
		(cd "$$dir" && go mod download); \
	done

mod-tidy: ## Tidy selected go.mod files
	@set -e; for dir in $(MODULE_DIRS); do \
		echo "Tidying $$dir"; \
		(cd "$$dir" && go mod tidy); \
	done

mod-update: ## Update dependencies and tidy selected modules
	@set -e; for dir in $(MODULE_DIRS); do \
		echo "Updating dependencies in $$dir"; \
		(cd "$$dir" && go get -u ./... && go mod tidy); \
	done
