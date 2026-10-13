PROTOC_GEN_GO_VERSION := v1.36.12
PROTOC_GEN_GO_GRPC_VERSION := v1.6.2

## Protocol buffers:
.PHONY: install-protobuf-generators
install-protobuf-generators: ## Install pinned protoc-gen-go and protoc-gen-go-grpc in bin/ if needed
	@if [ ! -x "$(ROOT_DIR)/bin/protoc-gen-go" ] || \
		! version="$$("$(ROOT_DIR)/bin/protoc-gen-go" --version)" || \
		[ "$$version" != "protoc-gen-go $(PROTOC_GEN_GO_VERSION)" ]; then \
		GOBIN="$(ROOT_DIR)/bin" go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION); \
	fi
	@if [ ! -x "$(ROOT_DIR)/bin/protoc-gen-go-grpc" ] || \
		! version="$$("$(ROOT_DIR)/bin/protoc-gen-go-grpc" --version)" || \
		[ "$$version" != "protoc-gen-go-grpc $(patsubst v%,%,$(PROTOC_GEN_GO_GRPC_VERSION))" ]; then \
		GOBIN="$(ROOT_DIR)/bin" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION); \
	fi

.PHONY: proto-gen
proto-gen: install-protobuf-generators ## Generate shared gRPC code in pkg/proto
	@cd "$(ROOT_DIR)" && buf generate
