.PHONY: all build test lint clean check

# Default target
all: lint test build

build:
	@echo "==> Building mei CLI..."
	@mkdir -p bin
	@go build -o bin/mei ./cmd/mei

test:
	@echo "==> Running tests..."
	@go test -v -race ./...

lint:
	@echo "==> Running golangci-lint..."
	@golangci-lint run

clean:
	@echo "==> Cleaning..."
	@rm -rf bin/
	@go clean

check: lint test
	@echo "==> All checks passed!"
