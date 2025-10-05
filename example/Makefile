.PHONY: all basic checkpoint streaming error_handling clean help

# Run all examples
all: basic checkpoint streaming error_handling

# Run individual examples
basic:
	@echo "=== Running Basic Example ==="
	@cd basic && go run main.go
	@echo ""

checkpoint:
	@echo "=== Running Checkpoint Example ==="
	@cd checkpoint && go run main.go
	@echo ""

streaming:
	@echo "=== Running Streaming Example ==="
	@cd streaming && go run main.go
	@echo ""

error_handling:
	@echo "=== Running Error Handling Example ==="
	@cd error_handling && go run main.go
	@echo ""

# Build all examples
build:
	@echo "Building all examples..."
	@cd basic && go build -o basic_example
	@cd checkpoint && go build -o checkpoint_example
	@cd streaming && go build -o streaming_example
	@cd error_handling && go build -o error_handling_example
	@echo "✓ All examples built successfully"

# Clean up built binaries and generated WAL files
clean:
	@echo "Cleaning up..."
	@rm -f basic/basic_example
	@rm -f checkpoint/checkpoint_example
	@rm -f streaming/streaming_example
	@rm -f error_handling/error_handling_example
	@find . -name "*.wal" -type f -delete
	@echo "✓ Cleanup complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  all             - Run all examples (default)"
	@echo "  basic           - Run basic usage example"
	@echo "  checkpoint      - Run checkpoint example"
	@echo "  streaming       - Run streaming example"
	@echo "  error_handling  - Run error handling example"
	@echo "  build           - Build all examples"
	@echo "  clean           - Remove built binaries and WAL files"
	@echo "  help            - Show this help message"

