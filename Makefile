##
## Tests:
.PHONY: test
test: ## - Run tests for all services
	@echo "Running tests..."
	@go test -v ./...

include make/modules.mk
include make/help.mk
