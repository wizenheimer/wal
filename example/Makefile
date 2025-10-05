.PHONY: all basic checkpoint streaming error_handling segment_manager wal_api clean help

# Run all examples
all: basic checkpoint streaming error_handling segment_manager wal_api

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

segment_manager:
	@echo "=== Running Segment Manager Example ==="
	@cd segment_manager && go run main.go
	@echo ""

wal_api:
	@echo "=== Running WAL API Example ==="
	@cd wal_api && go run main.go
	@echo ""

# Build all examples
build:
	@echo "Building all examples..."
	@cd basic && go build -o basic_example
	@cd checkpoint && go build -o checkpoint_example
	@cd streaming && go build -o streaming_example
	@cd error_handling && go build -o error_handling_example
	@cd segment_manager && go build -o segment_manager_example
	@cd wal_api && go build -o wal_api_example
	@echo "✓ All examples built successfully"

# Clean up built binaries and generated WAL files
clean:
	@echo "Cleaning up..."
	@rm -f basic/basic_example
	@rm -f checkpoint/checkpoint_example
	@rm -f streaming/streaming_example
	@rm -f error_handling/error_handling_example
	@rm -f segment_manager/segment_manager_example
	@rm -f wal_api/wal_api_example
	@find . -name "*.wal" -type f -delete
	@rm -rf */example_segments
	@rm -rf */wal_api_example
	@echo "✓ Cleanup complete"

# Show help
help:
	@echo "Available targets:"
	@echo "  all             - Run all examples (default)"
	@echo "  basic           - Run basic usage example"
	@echo "  checkpoint      - Run checkpoint example"
	@echo "  streaming       - Run streaming example"
	@echo "  error_handling  - Run error handling example"
	@echo "  segment_manager - Run segment manager example"
	@echo "  wal_api         - Run WAL API example (high-level API)"
	@echo "  build           - Build all examples"
	@echo "  clean           - Remove built binaries and WAL files"
	@echo "  help            - Show this help message"

