# WAL - Write-Ahead Log Implementation in Go

A high-performance Write-Ahead Log (WAL) library for Go, designed for building reliable and durable data systems.

[![Go Version](https://img.shields.io/badge/go-1.24.2+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

## Overview

This WAL implementation provides durability and crash recovery for your applications by ensuring all changes are written to persistent storage before being applied. It's designed with a modular architecture that supports multiple storage backends, efficient segment management, and checkpoint-based recovery.

## Features

- **Durable Writes**: All entries are persisted to disk with configurable fsync behavior
- **Segment Management**: Automatic log rotation and cleanup with configurable size limits
- **Checkpoint Support**: Fast recovery by resuming from last checkpoint instead of replaying entire log
- **LSN Tracking**: Monotonic log sequence numbers for total ordering of operations
- **CRC Validation**: Built-in data integrity checking
- **Thread-Safe**: Concurrent reads and writes with proper locking
- **Pluggable Storage**: Interface-based design supports multiple storage backends
- **Background Syncing**: Automatic periodic fsync with configurable intervals
- **Streaming API**: Memory-efficient entry-by-entry reading

## Installation

```bash
go get github.com/wizenheimer/wal
```

## Quick Start

```go
package main

import (
    "log"
    "github.com/wizenheimer/wal"
)

func main() {
    // Create segment manager for file-based storage
    segmentMgr, err := wal.NewFileSegmentManager("./wal_data")
    if err != nil {
        log.Fatal(err)
    }

    // Configure WAL options
    opts := wal.DefaultWALOptions()
    opts.MaxSegmentSize = 64 * 1024 * 1024  // 64MB per segment
    opts.MaxSegments = 10                    // Keep 10 segments max
    opts.SyncInterval = 200 * time.Millisecond

    // Open WAL
    w, err := wal.Open(segmentMgr, opts)
    if err != nil {
        log.Fatal(err)
    }
    defer w.Close()

    // Write entries (LSN is managed automatically)
    lsn, err := w.WriteEntry([]byte("operation data"))
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Written entry with LSN: %d", lsn)

    // Create checkpoint periodically
    checkpointLSN, err := w.WriteCheckpoint([]byte("state snapshot"))
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Created checkpoint at LSN: %d", checkpointLSN)

    // Read entries from last checkpoint (for recovery)
    entries, err := w.ReadFromCheckpoint()
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Read %d entries from checkpoint", len(entries))
}
```

## Architecture

### Component Hierarchy

```
┌──────────────────────────────────────────────────────────┐
│                     Application                          │
│              (Your database, key-value store, etc.)      │
└─────────────────────────┬────────────────────────────────┘
                          │
                          ▼
              ┌───────────────────────┐
              │         WAL           │  ← Orchestrates everything
              │  (Core coordinator)   │    Manages LSN, rotation
              └──────────┬────────────┘
                         │
         ┌───────────────┴───────────────┐
         ▼                               ▼
┌────────────────────┐         ┌─────────────────────┐
│  SegmentManager    │         │   EntryWriter       │
│  (Where to store)  │         │   (How to serialize)│
└────────┬───────────┘         └──────────┬──────────┘
         │                                 │
         │                                 ▼
         │                     ┌──────────────────────┐
         │                     │   EntryReader        │
         │                     │   (How to read back) │
         │                     └──────────────────────┘
         │
         ▼
┌─────────────────┐
│    Segments     │  ← Actual files/objects
│  (Physical data)│     segment-0, segment-1, etc.
└─────────────────┘
```

### Core Components

#### 1. WAL Entry

The atomic unit of data in the log:

```protobuf
message WAL_Entry {
    uint64 logSequenceNumber = 1;  // Unique, monotonic ID
    bytes  data = 2;                // Your actual data
    uint32 CRC = 3;                 // Checksum for integrity
    optional bool isCheckpoint = 4; // Special marker
}
```

#### 2. Segments

Segments are individual files that contain multiple entries:

```
/var/wal/
  segment-0  (64 MB - oldest)
  segment-1  (64 MB)
  segment-2  (64 MB)
  segment-3  (32 MB - current, still being written)
```

**Benefits:**

- Bounded file size
- Easy cleanup and archival
- Parallel reads
- Fault isolation

#### 3. Log Sequence Number (LSN)

A unique, monotonically increasing identifier for each entry:

```
segment-0: LSN 1    → 1000
segment-1: LSN 1001 → 2000
segment-2: LSN 2001 → 3000
segment-3: LSN 3001 → ...  (current)
```

**Purpose:**

- Establishes total ordering of operations
- Enables tracking and referencing specific operations
- Allows gap detection for missing entries

#### 4. Segment Manager

Interface for storage abstraction:

```go
type SegmentManager interface {
    CreateSegment(id int) (io.WriteCloser, error)
    OpenSegment(id int) (io.ReadCloser, error)
    ListSegments() ([]int, error)
    DeleteSegment(id int) error
    CurrentSegmentSize(id int) (int64, error)
}
```

Implementations:

- `FileSegmentManager` - Local filesystem storage (default)
- Custom implementations for S3, Redis, etc.

### Write Flow

```
1. Application calls: wal.WriteEntry(data)
                         ↓
2. WAL locks mutex (thread-safety)
                         ↓
3. Check if rotation needed
                         ↓
4. Generate LSN (lastLSN++)
                         ↓
5. Calculate CRC
                         ↓
6. Create WAL_Entry
                         ↓
7. Serialize to protobuf
                         ↓
8. Write to buffer (bufio)
                         ↓
9. Background sync (every N ms)
   - Flush buffer → kernel
   - fsync → disk
                         ↓
10. Data is durable!
```

### Segment Rotation

When the current segment reaches the maximum size:

```
Step 1: Sync current segment (flush all buffered data)
Step 2: Close current segment
Step 3: Delete oldest segment if at max count
Step 4: Create new segment (ID++)
Step 5: Setup new writer
```

### Checkpoints

Checkpoints allow fast recovery by saving application state:

```
Regular Entries:
LSN 1: INSERT user1
LSN 2: INSERT user2
...
LSN 500,000: UPDATE user250

[CHECKPOINT at LSN 500,001]
Data: Serialized state of entire application

LSN 500,002: INSERT user251
...
LSN 1,000,000: DELETE user50
```

**Recovery Process:**

1. Find last checkpoint
2. Restore state from checkpoint
3. Replay only operations after checkpoint
4. Recovery time: seconds instead of hours

## Configuration

### WAL Options

```go
type WALOptions struct {
    MaxSegmentSize int64          // Max bytes per segment (default: 4MB)
    MaxSegments    int             // Max segments to keep (default: 10)
    SyncInterval   time.Duration   // Auto-sync interval (default: 3s)
    EnableFsync    bool            // Whether to fsync (default: true)
}
```

### Tuning Recommendations

**MaxSegmentSize:**

- SSD: 64-128 MB (fast I/O, rotation is cheap)
- HDD: 256-512 MB (minimize seeking)
- Cloud (S3): 100-500 MB (minimize API calls)

**MaxSegments:**

- Development: 3-5 segments (limited disk)
- Deployment: 20-50 segments (retain history)
- Archival: Larger number + archive old segments

**SyncInterval:**

- Financial systems: 10-50ms (high durability)
- General apps: 100-500ms (balanced)
- Analytics: 1-5s (throughput over durability)

**EnableFsync:**

- Deployment: Always `true`
- Testing: `false` for speed
- Development: `false` unless testing recovery

## API Reference

### High-Level WAL API

#### Open

```go
func Open(segmentMgr SegmentManager, opts WALOptions) (*WAL, error)
```

Opens or creates a WAL with the given segment manager and options.

#### WriteEntry

```go
func (w *WAL) WriteEntry(data []byte) (uint64, error)
```

Writes a regular entry to the WAL. Returns the assigned LSN.

#### WriteCheckpoint

```go
func (w *WAL) WriteCheckpoint(data []byte) (uint64, error)
```

Writes a checkpoint entry containing application state. Syncs before writing.

#### ReadAll

```go
func (w *WAL) ReadAll() ([]*WAL_Entry, error)
```

Reads all entries from all segments.

#### ReadFromCheckpoint

```go
func (w *WAL) ReadFromCheckpoint() ([]*WAL_Entry, error)
```

Reads entries starting from the last checkpoint. Discards all entries before the checkpoint.

#### Sync

```go
func (w *WAL) Sync() error
```

Manually flushes buffers and syncs to disk.

#### Close

```go
func (w *WAL) Close() error
```

Closes the WAL, syncing all data and stopping background goroutines.

### Low-Level Entry API

For fine-grained control:

```go
// Create writer
writer := wal.NewBinaryEntryWriter(file)

// Write entry
entry := &wal.WAL_Entry{
    LogSequenceNumber: lsn,
    Data:              data,
    CRC:               calculateCRC(data, lsn),
}
writer.WriteEntry(entry)
writer.Sync()

// Create reader
reader := wal.NewBinaryEntryReader(file)

// Read entries
for {
    entry, err := reader.ReadEntry()
    if err == io.EOF {
        break
    }
    // Process entry
}
```

## Examples

See the [example/](example/) directory for comprehensive examples:

- **basic/** - Fundamental read/write operations
- **checkpoint/** - Checkpoint creation and recovery
- **streaming/** - Efficient streaming for large datasets
- **error_handling/** - CRC validation and error recovery
- **segment_manager/** - Multi-segment management
- **wal_api/** - Complete example

Run examples:

```bash
cd example/wal_api
go run main.go
```

## Performance Characteristics

### Write Throughput vs Sync Strategy

| Strategy              | Throughput          | Latency | Data Loss Risk  |
| --------------------- | ------------------- | ------- | --------------- |
| Sync every write      | ~1,000 writes/sec   | ~1ms    | 0 entries       |
| Sync every 100 writes | ~50,000 writes/sec  | ~20μs   | 100 entries max |
| Sync on timer (200ms) | ~100,000 writes/sec | ~10μs   | 200ms of data   |

### Memory Usage

The streaming API ensures O(1) memory usage:

- Only one entry in memory at a time during reads
- Buffer size is configurable (default: 4KB)
- No need to load entire WAL into memory

## Use Cases

### Database Systems

```go
// Write operation to WAL before applying to database
func (db *Database) ExecuteQuery(query string) error {
    // Write to WAL first
    lsn, err := db.wal.WriteEntry([]byte(query))
    if err != nil {
        return err
    }

    // Apply to database
    err = db.applyQuery(query)
    if err != nil {
        // Can rollback using WAL
        return err
    }

    return nil
}

// Recovery on startup
func (db *Database) Recover() error {
    entries, err := db.wal.ReadFromCheckpoint()
    if err != nil {
        return err
    }

    for _, entry := range entries {
        if entry.IsCheckpoint != nil && *entry.IsCheckpoint {
            // Restore from checkpoint
            db.restoreState(entry.Data)
        } else {
            // Replay operation
            db.applyQuery(string(entry.Data))
        }
    }

    return nil
}
```

### Message Queue

```go
type Queue struct {
    wal      *wal.WAL
    messages []Message
}

func (q *Queue) Enqueue(msg Message) error {
    data, _ := json.Marshal(msg)
    _, err := q.wal.WriteEntry(data)
    if err != nil {
        return err
    }
    q.messages = append(q.messages, msg)
    return nil
}

func (q *Queue) Checkpoint() error {
    state, _ := json.Marshal(q.messages)
    _, err := q.wal.WriteCheckpoint(state)
    return err
}
```

### State Machine Replication

```go
type StateMachine struct {
    wal   *wal.WAL
    state map[string]string
}

func (sm *StateMachine) Apply(command Command) error {
    // Log command
    data, _ := json.Marshal(command)
    _, err := sm.wal.WriteEntry(data)
    if err != nil {
        return err
    }

    // Execute command
    sm.executeCommand(command)
    return nil
}
```

## Error Handling

### CRC Mismatch

```go
entries, err := wal.ReadAll()
if err != nil {
    if errors.Is(err, wal.ErrCRCMismatch) {
        // Data corruption detected
        log.Error("WAL corruption detected, attempting recovery...")
        // Truncate at last valid entry or restore from backup
    }
}
```

### Disk Full

```go
_, err := w.WriteEntry(data)
if err != nil {
    if errors.Is(err, os.ErrNoSpace) {
        // Disk full - cleanup old segments
        // or alert administrator
    }
}
```

### Incomplete Write After Crash

The library automatically handles incomplete writes:

- CRC validation detects partial entries
- Recovery truncates at last valid entry
- Ensures WAL integrity after crash

## Best Practices

1. **Use Checkpoints**: Create checkpoints periodically to bound recovery time

   ```go
   // Every 10,000 operations or 5 minutes
   if operationCount >= 10000 || time.Since(lastCheckpoint) > 5*time.Minute {
       w.WriteCheckpoint(serializeState())
   }
   ```

2. **Tune Sync Strategy**: Balance durability vs throughput based on requirements

   ```go
   // Financial: High durability
   opts.SyncInterval = 50 * time.Millisecond

   // Analytics: High throughput
   opts.SyncInterval = 5 * time.Second
   ```

3. **Handle Errors Gracefully**: Always check errors and implement recovery logic

   ```go
   if _, err := w.WriteEntry(data); err != nil {
       log.Error("WAL write failed", "error", err)
       // Implement retry logic or alert
   }
   ```

4. **Monitor Disk Usage**: Set appropriate segment limits

   ```go
   // Max WAL size = MaxSegmentSize * MaxSegments
   // Example: 64MB * 10 = 640MB max
   ```

5. **Clean Shutdown**: Always close WAL properly
   ```go
   defer w.Close()  // Ensures final sync
   ```

## Testing

Run tests:

```bash
# Unit tests
go test ./...

# Run with race detector
go test -race ./...

# Run benchmarks
go test -bench=. -benchmem

# Run examples
cd example && make all
```

## Documentation

- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - Detailed component architecture
- [CONCEPT.md](docs/CONCEPT.md) - Core concepts and operations
- [TIMELINE.md](docs/TIMELINE.md) - Visual timelines and examples
- [Examples](example/README.md) - Comprehensive usage examples

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file for details
