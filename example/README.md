# WAL Examples

This directory contains examples demonstrating how to use the Write-Ahead Log (WAL) library for reading and writing entries.

## Directory Structure

```
example/
├── README.md                    # This file
├── basic/                       # Basic usage example
│   └── main.go
├── checkpoint/                  # Checkpoint functionality
│   └── main.go
├── streaming/                   # Streaming large datasets
│   └── main.go
├── error_handling/              # Error handling and validation
│   └── main.go
└── segment_manager/             # Segment management operations
    └── main.go
```

## Examples

### 1. Basic Usage (`basic/`)

Demonstrates the fundamental operations:

- Creating a WAL file
- Writing entries using `BinaryEntryWriter`
- Flushing and syncing data
- Reading entries back using `ReadAllEntries`

**Run:**

```bash
cd basic
go run main.go

# Keep WAL files after execution for inspection
go run main.go -keep
```

### 2. Checkpoint Example (`checkpoint/`)

Shows how to use checkpoint functionality:

- Writing multiple entries
- Creating a checkpoint entry
- Reading entries with checkpoint awareness using `ReadEntriesWithCheckpoint`
- Understanding how checkpoints allow you to skip earlier entries

**Run:**

```bash
cd checkpoint
go run main.go
```

**Key Concept:** Checkpoints allow you to mark a point in the WAL where all previous entries can be safely ignored during recovery, as the checkpoint represents a consistent state.

### 3. Streaming Example (`streaming/`)

Demonstrates streaming operations for large datasets:

- Writing many entries efficiently
- Periodic flushing for better performance
- Reading entries one-by-one using `BinaryEntryReader.ReadEntry()`
- File size and performance metrics

**Run:**

```bash
cd streaming
go run main.go
```

### 4. Error Handling (`error_handling/`)

Shows error handling and validation:

- CRC validation on read
- Detecting corrupted entries
- Handling empty files
- Error recovery strategies

**Run:**

```bash
cd error_handling
go run main.go
```

### 5. Segment Manager (`segment_manager/`)

Demonstrates segment management for organizing WAL data across multiple files:

- Creating and managing multiple segments
- Writing entries to different segments
- Listing and reading from segments
- Checking segment sizes
- Appending to existing segments
- Segment rotation (deleting old segments)
- Sequential reading across segments

**Run:**

```bash
cd segment_manager
go run main.go

# Keep segment files after execution for inspection
go run main.go -keep
```

**Key Concept:** Segment management allows you to organize your WAL into multiple files, making it easier to:

- Implement log rotation and cleanup policies
- Reduce memory usage by only loading relevant segments
- Improve performance by distributing I/O across segments
- Archive old data without affecting active segments

## Core Concepts

### WAL Entry Structure

Each WAL entry contains:

- `LogSequenceNumber` (LSN): A monotonically increasing sequence number
- `Data`: The actual payload bytes
- `CRC`: A checksum for data integrity verification
- `IsCheckpoint` (optional): Marks this entry as a checkpoint

### Binary Format

Entries are stored in binary format with:

1. **Length prefix** (4 bytes, little-endian uint32): Size of the marshaled entry
2. **Entry data**: Protocol buffer serialized entry

### CRC Calculation

The CRC is calculated over:

1. The entry data
2. The log sequence number

This ensures both data integrity and correct ordering.

### Writer Operations

- `WriteEntry(entry)`: Writes a single entry to the buffered writer
- `Flush()`: Flushes the buffer to the underlying writer
- `Sync()`: Flushes and ensures data is persisted to disk (if supported)

### Reader Operations

- `ReadEntry()`: Reads the next entry, returns `io.EOF` when done
- `ReadAllEntries(reader)`: Reads all entries into memory
- `ReadEntriesWithCheckpoint(reader)`: Reads entries but only keeps those from the last checkpoint onwards

## Command Line Flags

All examples support the following flag:

- `-keep`: Keep WAL files after execution (don't remove them)

Example usage:

```bash
go run main.go -keep
```

This is useful for:

- Inspecting WAL file contents
- Debugging issues
- Understanding the binary format
- Manual testing

## Best Practices

1. **Always sync after critical writes**: Call `Sync()` to ensure durability
2. **Use checkpoints for long-running systems**: Prevents unbounded WAL growth
3. **Validate CRC on read**: Always check the CRC to detect corruption
4. **Handle errors gracefully**: WAL corruption can happen due to crashes or disk issues
5. **Use buffered operations**: The library uses buffering for better performance

## Running All Examples

```bash
# From the example directory
cd basic && go run main.go && cd ..
cd checkpoint && go run main.go && cd ..
cd streaming && go run main.go && cd ..
cd error_handling && go run main.go && cd ..
cd segment_manager && go run main.go && cd ..

# Or use a loop
for dir in basic checkpoint streaming error_handling segment_manager; do
    echo "Running $dir example..."
    (cd "$dir" && go run main.go)
    echo ""
done

# Keep WAL files for inspection
for dir in basic checkpoint streaming error_handling segment_manager; do
    echo "Running $dir example (keeping files)..."
    (cd "$dir" && go run main.go -keep)
    echo ""
done

# Or use the Makefile
make all
```

## Integration with Your Application

To use the WAL in your application:

```go
import "github.com/wizenheimer/wal"

// Create a writer
file, _ := os.OpenFile("app.wal", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
writer := wal.NewBinaryEntryWriter(file)

// Write an entry
entry := &wal.WAL_Entry{
    LogSequenceNumber: getNextLSN(),
    Data:              serializeYourData(),
}
entry.CRC = calculateCRC(entry.Data, entry.LogSequenceNumber)
writer.WriteEntry(entry)
writer.Sync()

// Read entries on recovery
file, _ := os.Open("app.wal")
entries, checkpointLSN, _ := wal.ReadEntriesWithCheckpoint(file)
// Replay entries to restore state
```

## Troubleshooting

**CRC Mismatch Errors:**

- Indicates data corruption
- Check disk health
- Verify crash recovery procedures

**Performance Issues:**

- Adjust buffer size in the library (defaultBufferSize)
- Use checkpoints more frequently
- Consider batching writes before sync

**File Size Growth:**

- Implement periodic checkpoint creation
- Truncate WAL after successful checkpoint
- Archive old WAL segments

## Building the Examples

Each example is a standalone Go program. To build them:

```bash
# Build all examples using Make
make build

# Or build manually with a loop
for dir in basic checkpoint streaming error_handling segment_manager; do
    (cd "$dir" && go build -o "${dir}_example")
done

# Or build individually
cd basic && go build -o basic_example
cd segment_manager && go build -o segment_manager_example
```
