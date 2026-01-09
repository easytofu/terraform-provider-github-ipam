# Copyright (c) EasyTofu
# SPDX-License-Identifier: MPL-2.0

default: build

BINARY_NAME=terraform-provider-gitipam
VERSION?=0.1.0
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

# Build the provider
build:
	go build -o $(BINARY_NAME)

# Install the provider locally for development
install: build
	mkdir -p ~/.terraform.d/plugins/registry.opentofu.org/easytofu/gitipam/$(VERSION)/$(GOOS)_$(GOARCH)
	cp $(BINARY_NAME) ~/.terraform.d/plugins/registry.opentofu.org/easytofu/gitipam/$(VERSION)/$(GOOS)_$(GOARCH)/

# Run unit tests
test:
	go test -v -cover -timeout 120s ./...

# Run acceptance tests
testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./internal/...

# Run linting
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...
	gofumpt -w .

# Generate documentation
docs:
	go generate ./...

# Tidy dependencies
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

# Run all checks (lint, test, build)
check: fmt lint test build

.PHONY: default build install test testacc lint fmt docs tidy clean check
