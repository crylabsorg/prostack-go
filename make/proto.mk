## Protocol buffers:
.PHONY: proto-tools
proto-tools: ## Install pinned protobuf generators into bin/
	@GOBIN="$(ROOT_DIR)/bin" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
	@GOBIN="$(ROOT_DIR)/bin" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2

.PHONY: proto-gen
proto-gen: proto-tools ## Generate shared gRPC code in pkg/proto
	@cd "$(ROOT_DIR)" && buf generate
