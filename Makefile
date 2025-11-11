.PHONY: help build test lint fmt clean install

# Default target
help:
	@echo "Available targets:"
	@echo "  build    - Build the binary"
	@echo "  test     - Run tests"
	@echo "  lint     - Run linters"
	@echo "  fmt      - Format code"
	@echo "  clean    - Clean build artifacts"
	@echo "  install  - Install binary to GOPATH/bin"

# Build the binary
build:
	go build -o work main.go

# Run tests
test:
	go test ./... -v

# Run linters (requires golangci-lint)
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

# Format code
fmt:
	go fmt ./...
	gofmt -s -w .

# Clean build artifacts
clean:
	rm -f work
	rm -rf dist/
	rm -rf completions/

# Install binary
install:
	go install

# Run tests with coverage
test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run tests quickly
test-quick:
	go test ./... -short
