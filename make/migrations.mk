# Only app modules with a migrations directory get migration targets.
MIGRATION_APP_NAMES := $(filter $(APP_NAMES),$(patsubst $(APPS_DIR)/%/migrations/,%,$(wildcard $(APPS_DIR)/*/migrations/)))
DEFAULT_MIGRATION_APP := $(if $(filter 1,$(words $(MIGRATION_APP_NAMES))),$(firstword $(MIGRATION_APP_NAMES)))

## Database migrations:
.PHONY: migrate-up migrate-status migrate-hash migrate-new
migrate-up: $(addprefix migrate-up-,$(MIGRATION_APP_NAMES)) ## Apply migrations for all apps with migrations

migrate-status: $(addprefix migrate-status-,$(MIGRATION_APP_NAMES)) ## Show migration status for all apps with migrations

migrate-hash: $(addprefix migrate-hash-,$(MIGRATION_APP_NAMES)) ## Recompute migration checksums for all apps with migrations

migrate-new: ## Create a dated migration: make migrate-new [APP=app]
	@test -n "$(or $(APP),$(DEFAULT_MIGRATION_APP))" || { echo "APP is required unless exactly one app has migrations"; exit 1; }
	@$(MAKE) migrate-new-$(or $(APP),$(DEFAULT_MIGRATION_APP))

ifneq ($(strip $(MIGRATION_APP_NAMES)),)
.PHONY: $(foreach action,up status hash new,$(addprefix migrate-$(action)-,$(MIGRATION_APP_NAMES)))

$(addprefix migrate-up-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate apply --env local --var app=$(patsubst migrate-up-%,%,$@)

$(addprefix migrate-status-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate status --env local --var app=$(patsubst migrate-status-%,%,$@)

$(addprefix migrate-hash-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate hash --dir "file://apps/$(patsubst migrate-hash-%,%,$@)/migrations"

$(addprefix migrate-new-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate new --dir "file://apps/$(patsubst migrate-new-%,%,$@)/migrations"
endif
