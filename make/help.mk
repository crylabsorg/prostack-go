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
