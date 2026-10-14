SWAG_VERSION := v2.0.0-rc6

## Swagger tools:
.PHONY: install-swag
install-swag: ## Install pinned swag in bin/ if needed
	@# swag --version omits the release candidate suffix; check the embedded module version.
	@if [ ! -x "$(ROOT_DIR)/bin/swag" ] || \
		[ "$$(go version -m "$(ROOT_DIR)/bin/swag" | \
			awk '$$1 == "mod" && $$2 == "github.com/swaggo/swag/v2" { print $$3 }')" != "$(SWAG_VERSION)" ]; then \
		GOBIN="$(ROOT_DIR)/bin" go install github.com/swaggo/swag/v2/cmd/swag@$(SWAG_VERSION); \
	fi
