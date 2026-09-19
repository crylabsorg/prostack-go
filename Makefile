.DEFAULT_GOAL = help

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
	@grep -h '^[^[:space:]].*##\|^##' $(MAKEFILE_LIST) | \
	sed -e 's/\\$$//' | \
	awk 'BEGIN {FS = ":*[[:space:]]*##[[:space:]]*"}; \
	{ \
		if($$2 == "") \
			next; \
		else if($$0 ~ /^##/) \
			printf "\n\033[33m%s\033[0m\n", $$2; \
		else \
			printf "    \033[34m%-20s\033[0m %s\n", $$1, $$2; \
	}'
	@echo "" # blank line at the end
