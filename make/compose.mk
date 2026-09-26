## Local dependencies:
.PHONY: dev-up dev-down
dev-up: ## Start local dependencies
	@docker compose -f "$(ROOT_DIR)/devops/docker-compose.yaml" up -d

dev-down: ## Stop local dependencies
	@docker compose -f "$(ROOT_DIR)/devops/docker-compose.yaml" down
