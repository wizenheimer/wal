# Color definitions
RED     := \033[0;31m
GREEN   := \033[0;32m
YELLOW  := \033[0;33m
BLUE    := \033[0;34m
MAGENTA := \033[0;35m
CYAN    := \033[0;36m
RESET   := \033[0m

# Project configuration
BINARY_NAME := wal
GO_FILES := $(shell find . -type f -name '*.go' -not -path "./vendor/*")
PROTO_FILES := $(shell find . -type f -name '*.proto')

.PHONY: all proto build run test clean fmt lint vet help init deps

# Default target
all: help

# Generate Go code from proto files
proto:
	@echo "$(CYAN)Generating protobuf code...$(RESET)"
	@protoc --go_out=. --go_opt=paths=source_relative types.proto
	@echo "$(GREEN)✓ Protobuf generation complete$(RESET)"

# Build the application
build: proto
	@echo "$(CYAN)Building $(BINARY_NAME)...$(RESET)"
	@go build -o bin/$(BINARY_NAME) .
	@echo "$(GREEN)✓ Build complete: bin/$(BINARY_NAME)$(RESET)"

# Run the application
run: build
	@echo "$(CYAN)Running $(BINARY_NAME)...$(RESET)"
	@./bin/$(BINARY_NAME)

# Run tests
test:
	@echo "$(CYAN)Running tests...$(RESET)"
	@go test -v -race -coverprofile=coverage.out ./...
	@echo "$(GREEN)✓ Tests complete$(RESET)"

# Run tests with coverage report
test-coverage: test
	@echo "$(CYAN)Generating coverage report...$(RESET)"
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ Coverage report: coverage.html$(RESET)"

# Run benchmarks
bench:
	@echo "$(CYAN)Running benchmarks...$(RESET)"
	@go test -bench=. -benchmem ./...
	@echo "$(GREEN)✓ Benchmarks complete$(RESET)"

# Format code
fmt:
	@echo "$(CYAN)Formatting code...$(RESET)"
	@gofmt -s -w $(GO_FILES)
	@echo "$(GREEN)✓ Code formatted$(RESET)"

# Lint code
lint:
	@echo "$(CYAN)Running linter...$(RESET)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
		echo "$(GREEN)✓ Linting complete$(RESET)"; \
	else \
		echo "$(YELLOW)⚠ golangci-lint not installed. Run: brew install golangci-lint$(RESET)"; \
	fi

# Run go vet
vet:
	@echo "$(CYAN)Running go vet...$(RESET)"
	@go vet ./...
	@echo "$(GREEN)✓ Vet complete$(RESET)"

# Check for common issues
check: fmt vet lint test
	@echo "$(GREEN)✓ All checks passed$(RESET)"

# Initialize project (setup tools and dependencies)
init:
	@echo "$(CYAN)Initializing project...$(RESET)"
	@echo "$(YELLOW)Checking for required tools...$(RESET)"
	@command -v go >/dev/null 2>&1 || { echo "$(RED)✗ Go is not installed$(RESET)"; exit 1; }
	@command -v protoc >/dev/null 2>&1 || { echo "$(RED)✗ protoc is not installed. Run: brew install protobuf$(RESET)"; exit 1; }
	@echo "$(GREEN)✓ Go found$(RESET)"
	@echo "$(GREEN)✓ protoc found$(RESET)"
	@echo "$(YELLOW)Installing Go tools...$(RESET)"
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@echo "$(GREEN)✓ protoc-gen-go installed$(RESET)"
	@echo "$(YELLOW)Downloading dependencies...$(RESET)"
	@go mod download
	@go mod verify
	@echo "$(GREEN)✓ Dependencies ready$(RESET)"
	@echo "$(YELLOW)Generating protobuf code...$(RESET)"
	@$(MAKE) proto
	@echo "$(GREEN)✓ Project initialized successfully!$(RESET)"
	@echo ""
	@echo "$(CYAN)Next steps:$(RESET)"
	@echo "  $(GREEN)make build$(RESET)  - Build the application"
	@echo "  $(GREEN)make test$(RESET)   - Run tests"

# Download dependencies
deps:
	@echo "$(CYAN)Downloading dependencies...$(RESET)"
	@go mod download
	@go mod verify
	@echo "$(GREEN)✓ Dependencies downloaded$(RESET)"

# Tidy dependencies
tidy:
	@echo "$(CYAN)Tidying dependencies...$(RESET)"
	@go mod tidy
	@echo "$(GREEN)✓ Dependencies tidied$(RESET)"

# Clean generated files and build artifacts
clean:
	@echo "$(CYAN)Cleaning...$(RESET)"
	@rm -f types.pb.go
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "$(GREEN)✓ Clean complete$(RESET)"

# Display help
help:
	@echo "$(CYAN)"
	@echo "╦ ╦╔═╗╦  "
	@echo "║║║╠═╣║  "
	@echo "╚╩╝╩ ╩╩═╝"
	@echo "$(RESET)"
	@echo "$(MAGENTA)Write-Ahead Log$(RESET)"
	@echo ""
	@echo "$(MAGENTA)═══════════════════════════════════════════════$(RESET)"
	@echo ""
	@echo "$(YELLOW)Build Commands:$(RESET)"
	@echo "  $(GREEN)make init$(RESET)           - Initialize project (first time setup)"
	@echo "  $(GREEN)make build$(RESET)          - Build the application"
	@echo "  $(GREEN)make run$(RESET)            - Build and run the application"
	@echo ""
	@echo "$(YELLOW)Development Commands:$(RESET)"
	@echo "  $(GREEN)make proto$(RESET)          - Generate Go code from protobuf"
	@echo "  $(GREEN)make fmt$(RESET)            - Format Go code"
	@echo "  $(GREEN)make vet$(RESET)            - Run go vet"
	@echo "  $(GREEN)make lint$(RESET)           - Run golangci-lint"
	@echo "  $(GREEN)make check$(RESET)          - Run fmt, vet, lint, and test"
	@echo ""
	@echo "$(YELLOW)Testing Commands:$(RESET)"
	@echo "  $(GREEN)make test$(RESET)           - Run tests with race detector"
	@echo "  $(GREEN)make test-coverage$(RESET)  - Run tests and generate coverage report"
	@echo "  $(GREEN)make bench$(RESET)          - Run benchmarks"
	@echo ""
	@echo "$(YELLOW)Dependency Commands:$(RESET)"
	@echo "  $(GREEN)make deps$(RESET)           - Download dependencies"
	@echo "  $(GREEN)make tidy$(RESET)           - Tidy dependencies"
	@echo ""
	@echo "$(YELLOW)Utility Commands:$(RESET)"
	@echo "  $(GREEN)make clean$(RESET)          - Remove generated files and artifacts"
	@echo "  $(GREEN)make help$(RESET)           - Display this help message"
	@echo ""
	@echo "$(MAGENTA)═══════════════════════════════════════════════$(RESET)"