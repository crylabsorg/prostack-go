.DEFAULT_GOAL := help

BUILD_DIR ?= $(ROOT_DIR)/build
BUILD_PLATFORMS ?= $(shell go env GOOS)/$(shell go env GOARCH)
LINUX_PLATFORMS := linux/amd64 linux/arm64
MACOS_PLATFORMS := darwin/amd64 darwin/arm64
