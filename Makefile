GOOS=js
GOARCH=wasm

export GOOS
export GOARCH

# Format all files
fmt:
	@echo "==> Formatting source"
	@gofmt -s -w $(shell find . -type f -name '*.go' -not -path "./vendor/*")
	@echo "==> Done"
.PHONY: fmt

# Tidy the go.mod file
tidy:
	@echo "==> Cleaning go.mod"
	@go mod tidy
	@echo "==> Done"
.PHONY: tidy

# Lint the project
lint:
	@echo "==> Linting Go files"
	@golangci-lint run ./...
.PHONY: lint

# Run all tests
test:
	@go test -cover ./...
.PHONY: test

# Build the commands
build:
	@goreleaser release --clean --snapshot
.PHONY: build

