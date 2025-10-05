# Quick Start Guide

## Running Examples

### Using Make (Recommended)

```bash
# Show all available commands
make help

# Run all examples
make

# Run individual examples
make basic
make checkpoint
make streaming
make error_handling
make segment_manager
make wal_api

# Build all examples
make build

# Clean up
make clean
```

### Command Line Options

All examples support the `-keep` flag to preserve WAL files after execution:

```bash
# Run and keep the WAL files for inspection
cd basic && go run main.go -keep
cd checkpoint && go run main.go -keep
cd streaming && go run main.go -keep
cd error_handling && go run main.go -keep
cd segment_manager && go run main.go -keep
cd wal_api && go run main.go -keep
```

### Manual Execution

```bash
# Run examples individually
cd basic && go run main.go
cd checkpoint && go run main.go
cd streaming && go run main.go
cd error_handling && go run main.go
cd segment_manager && go run main.go
cd wal_api && go run main.go
```

## Example Overview

| Example              | Purpose                   | Key Features                                                                               |
| -------------------- | ------------------------- | ------------------------------------------------------------------------------------------ |
| **basic/**           | Introduction to WAL       | • Write entries<br>• Flush & sync<br>• Read all entries                                    |
| **checkpoint/**      | Checkpoint functionality  | • Create checkpoints<br>• Recovery from checkpoint<br>• Skip old entries                   |
| **streaming/**       | Large dataset handling    | • Write 100 entries<br>• Periodic flushing<br>• Stream reading<br>• Performance metrics    |
| **error_handling/**  | Error validation          | • CRC validation<br>• Corrupt entry detection<br>• Empty file handling                     |
| **segment_manager/** | Segment management        | • Multiple segments<br>• Segment rotation<br>• Size tracking<br>• Sequential reads         |
| **wal_api/**         | High-level production API | • Automatic LSN management<br>• Background sync<br>• Auto rotation<br>• Complete lifecycle |

## What You'll Learn

### 1. Basic Usage (5 minutes)

- How to create a WAL file
- Writing entries with CRC checksums
- Reading entries back
- Basic file operations

### 2. Checkpoints (10 minutes)

- Why checkpoints matter
- Creating checkpoint entries
- Recovery optimization
- Reducing replay time

### 3. Streaming (10 minutes)

- Handling large datasets
- Buffer management
- Memory-efficient reading
- Performance monitoring

### 4. Error Handling (15 minutes)

- CRC validation
- Corruption detection
- Graceful error handling
- Recovery strategies

### 5. Segment Management (15 minutes)

- Organizing WAL into multiple files
- Segment creation and deletion
- Implementing rotation policies
- Reading across segments

### 6. WAL API (15 minutes)

- High-level production-ready API
- Automatic LSN management
- Background syncing
- Automatic segment rotation
- Crash recovery handling

## Quick Code Examples

### High-Level WAL API (Recommended)

```go
package main

import (
    "github.com/wizenheimer/wal"
)

func main() {
    // Create segment manager
    segmentMgr, _ := wal.NewFileSegmentManager("./wal_data")

    // Open WAL with options
    opts := wal.DefaultWALOptions()
    w, _ := wal.Open(segmentMgr, opts)
    defer w.Close()

    // Write entry (LSN auto-managed)
    lsn, _ := w.WriteEntry([]byte("Hello WAL"))

    // Recovery
    entries, _ := w.ReadFromCheckpoint()
}
```

### Low-Level Binary Entry API

```go
package main

import (
    "github.com/wizenheimer/wal"
    "os"
)

func main() {
    // Create writer
    file, _ := os.Create("app.wal")
    writer := wal.NewBinaryEntryWriter(file)

    // Write entry
    entry := wal.NewEntry(1, []byte("Hello WAL"))
    writer.WriteEntry(entry)
    writer.Sync()
    file.Close()

    // Read entries
    file, _ = os.Open("app.wal")
    entries, _ := wal.ReadAllEntries(file)
    file.Close()
}
```

## Next Steps

1. **Run all examples**: `make all`
2. **Read the detailed README.md**
3. **Experiment with the code**
4. **Integrate into your application**

## Troubleshooting

**Import errors?**

```bash
go mod tidy
```

**Can't run examples?**

```bash
# Make sure you're in the example directory
cd /path/to/wal/example
make basic
```

**Want to see the code?**

```bash
# Each example is in its own directory
cat basic/main.go
cat checkpoint/main.go
```
