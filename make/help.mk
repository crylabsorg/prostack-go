## Help:
.PHONY: help
help: ## Show available commands
	@awk '\
		BEGIN { \
			blue = "\033[34m"; \
			yellow = "\033[33m"; \
			reset = "\033[0m"; \
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
	@if [ -n "$(strip $(APP_NAMES))" ]; then \
		printf '\n\033[33mApps:\033[0m\n'; \
		for app in $(APP_NAMES); do \
			printf '    \033[34m%-20s\033[0m %s\n' "build-$$app" "Build $$app"; \
			printf '    \033[34m%-20s\033[0m %s\n' "run-$$app" "Run $$app"; \
			printf '    \033[34m%-20s\033[0m %s\n' "test-$$app" "Test $$app"; \
		done; \
	fi
	@if [ -n "$(strip $(MIGRATION_APP_NAMES))" ]; then \
		printf '\n\033[33mMigration apps:\033[0m\n'; \
		for app in $(MIGRATION_APP_NAMES); do \
			printf '    \033[34m%-20s\033[0m %s\n' "migrate-up-$$app" "Apply $$app migrations"; \
			printf '    \033[34m%-20s\033[0m %s\n' "migrate-status-$$app" "Show $$app migration status"; \
			printf '    \033[34m%-20s\033[0m %s\n' "migrate-new-$$app" "Create dated $$app migration"; \
			printf '    \033[34m%-20s\033[0m %s\n' "migrate-hash-$$app" "Recompute $$app migration checksum"; \
		done; \
	fi
	@printf '\n'
