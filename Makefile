ROOT_DIR := $(abspath $(dir $(firstword $(MAKEFILE_LIST))))
include $(ROOT_DIR)/make/consts.mk
include $(ROOT_DIR)/make/proto.mk

##
## Tests:
.PHONY: test
test: ## Run library tests
	@cd "$(ROOT_DIR)" && go test ./...

MODULE_DIRS := $(ROOT_DIR)

include $(ROOT_DIR)/make/modules.mk
include $(ROOT_DIR)/make/help.mk
