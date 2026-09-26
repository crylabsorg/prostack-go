# Only app modules with a migrations directory get migration targets.
MIGRATION_APP_NAMES := $(filter $(APP_NAMES),$(patsubst $(APPS_DIR)/%/migrations/,%,$(wildcard $(APPS_DIR)/*/migrations/)))
## Database migrations:
.PHONY: migrate-up migrate-status migrate-hash
migrate-up: $(addprefix migrate-up-,$(MIGRATION_APP_NAMES)) ## Apply migrations for all apps with migrations

migrate-status: $(addprefix migrate-status-,$(MIGRATION_APP_NAMES)) ## Show migration status for all apps with migrations

migrate-hash: $(addprefix migrate-hash-,$(MIGRATION_APP_NAMES)) ## Recompute migration checksums for all apps with migrations

.PHONY: migrate-diff
migrate-diff: $(addprefix migrate-diff-,$(MIGRATION_APP_NAMES)) ## Generate migrations from schema.sql for all apps

ifneq ($(strip $(MIGRATION_APP_NAMES)),)
.PHONY: $(foreach action,up status hash new diff,$(addprefix migrate-$(action)-,$(MIGRATION_APP_NAMES)))

$(addprefix migrate-up-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate apply --env local --var app=$(patsubst migrate-up-%,%,$@)

$(addprefix migrate-status-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate status --env local --var app=$(patsubst migrate-status-%,%,$@)

$(addprefix migrate-hash-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate hash --dir "file://apps/$(patsubst migrate-hash-%,%,$@)/migrations"

$(addprefix migrate-new-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate new --dir "file://apps/$(patsubst migrate-new-%,%,$@)/migrations"

$(addprefix migrate-diff-,$(MIGRATION_APP_NAMES)):
	@cd "$(ROOT_DIR)" && atlas migrate diff $(if $(NAME),"$(NAME)") --env local --var app=$(patsubst migrate-diff-%,%,$@)
endif
