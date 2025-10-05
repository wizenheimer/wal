# WAL Architecture Deep Dive

This document provides a detailed breakdown of every component of the refactored WAL implementation, starting from the most fundamental concepts and building up to the complete system.

---

## 1. What is a WAL Entry?

**An entry is the atomic unit of data in the Write-Ahead Log.**

### Definition

```protobuf
message WAL_Entry {
    uint64 logSequenceNumber = 1;  // Unique, monotonic ID
    bytes  data = 2;                // Your actual data
    uint32 CRC = 3;                 // Checksum for integrity
    optional bool isCheckpoint = 4; // Special marker
}
```

### Purpose

- Represents ONE logical operation (e.g., "transfer $100", "update user")
- Contains enough information to replay/undo the operation
- Self-contained and verifiable (via CRC)

### Key Properties

**1. Log Sequence Number (LSN)**

- Unique identifier for this entry
- Monotonically increasing (1, 2, 3, 4...)
- Never reused or skipped
- Establishes total ordering of all operations

**2. Data**

- Arbitrary bytes - could be JSON, Protobuf, anything
- The actual payload you want to persist
- WAL doesn't interpret this - it's opaque

**3. CRC (Cyclic Redundancy Check)**

- Detects corruption in storage/transmission
- Calculated from: `CRC32(data + LSN)`
- Verified on read - if mismatch, data is corrupted

**4. IsCheckpoint (Optional)**

- Marks this entry as a recovery point
- Tells recovery: "You can start from here"
- More on this later

### Example Entry

```go
entry := &WAL_Entry{
    LogSequenceNumber: 42,
    Data:              []byte(`{"op":"transfer","from":"A","to":"B","amount":100}`),
    CRC:               0xABCD1234,  // Calculated checksum
    IsCheckpoint:      nil,          // Regular entry
}
```

### Why Entries?

Without entries, the system would have:

- No way to identify individual operations
- No integrity checking
- No recovery points
- Just a blob of bytes

Entries give structure to the log.

---

## 2. What is a Segment?

**A segment is a single file that contains multiple WAL entries.**

### The Problem Segments Solve

Imagine writing ALL entries to one giant file:

```
/var/wal/log.dat  (100 GB and growing...)
```

**Problems:**

- File becomes huge → slow to open, read, seek
- Can't delete old data (file keeps growing forever)
- One corruption corrupts everything
- Hard to manage and backup

### The Solution: Segments

Break the log into multiple files:

```
/var/wal/
  segment-0  (64 MB - oldest)
  segment-1  (64 MB)
  segment-2  (64 MB)
  segment-3  (32 MB - current, still being written)
```

### Segment Properties

**1. Fixed Maximum Size**

- Each segment has a max size (e.g., 64MB)
- When full → create new segment
- Prevents any single file from growing too large

**2. Sequential IDs**

- segment-0, segment-1, segment-2...
- ID tells you the order
- Higher ID = more recent data

**3. Immutable (mostly)**

- Only the CURRENT segment is being written
- Old segments are read-only
- Makes caching, compression, archival easier

**4. Contains Contiguous LSN Range**

```
segment-0: LSN 1    → 1000
segment-1: LSN 1001 → 2000
segment-2: LSN 2001 → 3000
segment-3: LSN 3001 → ...  (current)
```

### Physical Format of a Segment

```
Segment File (segment-3):
┌─────────────────────────────────────┐
│ Entry 1 (LSN: 3001)                 │
│  [4 bytes: size][N bytes: data]     │
├─────────────────────────────────────┤
│ Entry 2 (LSN: 3002)                 │
│  [4 bytes: size][N bytes: data]     │
├─────────────────────────────────────┤
│ Entry 3 (LSN: 3003)                 │
│  [4 bytes: size][N bytes: data]     │
├─────────────────────────────────────┤
│ ...                                 │
└─────────────────────────────────────┘

Each entry is:
1. 4-byte length prefix (little-endian uint32)
2. N bytes of serialized WAL_Entry (protobuf)
```

### Why Segments?

**Benefits:**

- **Bounded file size** - No file grows infinitely
- **Easy cleanup** - Delete segment-0, segment-1 when old
- **Parallel reads** - Read multiple segments concurrently
- **Fault isolation** - Corruption in one segment doesn't affect others
- **Easier backup** - Archive old segments to S3
- **Better performance** - Smaller files = faster I/O

**Trade-offs:**

- More files to manage
- Need logic to handle rotation
- Need to track which segment has which LSN

---

## 3. What is the Segment Manager?

**The SegmentManager is an abstraction that handles all storage operations for segments.**

### Interface Definition

```go
type SegmentManager interface {
    // Create a new segment for writing
    CreateSegment(id int) (io.WriteCloser, error)

    // Open an existing segment for reading
    OpenSegment(id int) (io.ReadCloser, error)

    // List all segment IDs (in order)
    ListSegments() ([]int, error)

    // Delete a segment
    DeleteSegment(id int) error

    // Get current size of a segment
    CurrentSegmentSize(id int) (int64, error)
}
```

### Why an Interface?

**The key insight:** WAL logic doesn't care WHERE segments are stored.

It only cares about:

- "Give me a writer for segment 5"
- "Give me a reader for segment 3"
- "What segments exist?"
- "Delete segment 0"

By making this an interface, we can swap storage backends:

```
           ┌─────────────────────┐
           │   WAL Core Logic    │
           └──────────┬──────────┘
                      │
                      ▼
           ┌──────────────────────┐
           │  SegmentManager      │  ◄── Interface
           │     (interface)      │
           └──────────┬───────────┘
                      │
         ┌────────────┴────────────┬───────────────┐
         ▼                         ▼               ▼
┌────────────────┐      ┌──────────────┐  ┌──────────────┐
│ FileSegment    │      │ S3Segment    │  │ MockSegment  │
│ Manager        │      │ Manager      │  │ Manager      │
└────────────────┘      └──────────────┘  └──────────────┘
   (local disk)           (cloud)          (memory - tests)
```

### FileSegmentManager - The Default Implementation

This is the concrete implementation for filesystem storage.

```go
type FileSegmentManager struct {
    directory string      // Where to store segments
    mu        sync.RWMutex  // Thread-safety
}
```

**What it does:**

1. **CreateSegment(id int)**

   ```go
   // Creates: /var/wal/segment-5
   path := "/var/wal/segment-5"
   file := os.OpenFile(path, O_CREATE|O_WRONLY|O_APPEND, 0644)
   return file, nil
   ```

2. **OpenSegment(id int)**

   ```go
   // Opens: /var/wal/segment-3 for reading
   path := "/var/wal/segment-3"
   file := os.Open(path)
   return file, nil
   ```

3. **ListSegments()**

   ```go
   // Finds all files matching: segment-*
   // Returns: [0, 1, 2, 3, 4] (sorted)
   files := glob("/var/wal/segment-*")
   return parseIDs(files)
   ```

4. **DeleteSegment(id int)**

   ```go
   // Removes: /var/wal/segment-0
   os.Remove("/var/wal/segment-0")
   ```

5. **CurrentSegmentSize(id int)**
   ```go
   // Gets file size of segment-3
   info := os.Stat("/var/wal/segment-3")
   return info.Size()
   ```

### Why This Abstraction Matters

**Without SegmentManager:**

```go
// WAL is tightly coupled to os.File
type WAL struct {
    currentSegment *os.File  // Can ONLY use filesystem
}

func (w *WAL) rotate() {
    // Hard-coded file operations
    w.currentSegment.Close()
    newFile := os.Create("/var/wal/segment-X")
    // ...
}
```

**With SegmentManager:**

```go
// WAL works with ANY storage
type WAL struct {
    segmentMgr SegmentManager  // Could be File, S3, Redis, Mock
}

func (w *WAL) rotate() {
    // Generic operations - works with any backend
    w.currentWriter.Close()
    newWriter := w.segmentMgr.CreateSegment(w.currentSegment + 1)
    // ...
}
```

### Example: MockSegmentManager for Testing

```go
type MockSegmentManager struct {
    segments map[int]*bytes.Buffer  // In-memory storage
}

func (m *MockSegmentManager) CreateSegment(id int) (io.WriteCloser, error) {
    buf := &bytes.Buffer{}
    m.segments[id] = buf
    return &mockWriteCloser{Buffer: buf}, nil
}

// Now tests run in memory - 100x faster!
```

---

## 4. What is an Entry Writer?

**An EntryWriter serializes and writes WAL entries to an underlying writer.**

### Interface Definition

```go
type EntryWriter interface {
    WriteEntry(entry *WAL_Entry) error  // Write one entry
    Flush() error                       // Flush buffer
    Sync() error                        // Persist to disk (fsync)
}
```

### The Problem It Solves

**Question:** How do you write an entry to storage?

**Steps needed:**

1. Serialize `WAL_Entry` struct → bytes (protobuf)
2. Add length prefix (so we know where entry ends)
3. Write to buffer (for performance)
4. Flush buffer when needed
5. Call fsync (for durability)

**Without EntryWriter**, all this logic is scattered in WAL:

```go
func (w *WAL) writeEntry(entry *WAL_Entry) error {
    data := proto.Marshal(entry)  // Serialization
    size := uint32(len(data))
    binary.Write(w.bufWriter, LittleEndian, size)  // Length prefix
    w.bufWriter.Write(data)  // Write
    // ... flush logic
    // ... sync logic
}
```

**With EntryWriter**, it's extracted and reusable:

```go
type BinaryEntryWriter struct {
    w  io.Writer
    bw *bufio.Writer
}

func (bew *BinaryEntryWriter) WriteEntry(entry *WAL_Entry) error {
    data := MustMarshal(entry)
    size := uint32(len(data))
    binary.Write(bew.bw, LittleEndian, size)
    bew.bw.Write(data)
    return nil
}
```

### BinaryEntryWriter - The Default Implementation

```go
type BinaryEntryWriter struct {
    w          io.Writer      // Underlying writer (file, network, etc.)
    bw         *bufio.Writer   // Buffered writer (performance)
    syncWriter syncer          // Optional - for fsync
}
```

**What it does:**

**Format on disk:**

```
┌────────────────────────────────────────┐
│  [size: 4 bytes]  [entry data: N bytes] │  ← Entry 1
├────────────────────────────────────────┤
│  [size: 4 bytes]  [entry data: N bytes] │  ← Entry 2
├────────────────────────────────────────┤
│  [size: 4 bytes]  [entry data: N bytes] │  ← Entry 3
└────────────────────────────────────────┘
```

**Writing process:**

```go
entry := &WAL_Entry{...}

// 1. Serialize to protobuf bytes
data := proto.Marshal(entry)  // e.g., 156 bytes

// 2. Write length prefix
size := uint32(156)
binary.Write(buffer, LittleEndian, size)  // 4 bytes: [0x9C, 0x00, 0x00, 0x00]

// 3. Write entry data
buffer.Write(data)  // 156 bytes

// Result: [4-byte size][156-byte entry]
```

### Why Buffering?

**Without buffering:**

```go
file.Write([]byte{0x9C})  // Syscall 1
file.Write([]byte{0x00})  // Syscall 2
file.Write([]byte{0x00})  // Syscall 3
// ... 100+ syscalls for one entry!
```

**With buffering:**

```go
buffer.Write(...)  // Writes to memory
buffer.Write(...)  // Writes to memory
buffer.Write(...)  // Writes to memory
buffer.Flush()     // ONE syscall for all writes
```

**Performance difference:** 100-1000x faster

### Decorator Pattern for Features

The beauty of the interface: **wrap writers to add features**

```go
// Base: writes binary format
baseWriter := NewBinaryEntryWriter(file)

// Wrap: add compression
compressedWriter := NewCompressingEntryWriter(baseWriter, gzipCompress)

// Wrap: add metrics
metricsWriter := NewMetricsEntryWriter(compressedWriter, metrics)

// Now: metricsWriter has compression + metrics + binary encoding!
```

Each wrapper is approximately 50 lines of code and completely independent.

---

## 5. What is an Entry Reader?

**An EntryReader deserializes and reads WAL entries from an underlying reader.**

### Interface Definition

```go
type EntryReader interface {
    ReadEntry() (*WAL_Entry, error)  // Read next entry (returns io.EOF when done)
}
```

### The Mirror of EntryWriter

If EntryWriter **writes** this format:

```
[size: 4 bytes][entry data: N bytes]
```

Then EntryReader **reads** this format:

```go
func (ber *BinaryEntryReader) ReadEntry() (*WAL_Entry, error) {
    // 1. Read size
    var size uint32
    binary.Read(ber.br, LittleEndian, &size)  // Read 4 bytes

    // 2. Read entry data
    data := make([]byte, size)
    io.ReadFull(ber.br, data)  // Read exactly 'size' bytes

    // 3. Deserialize
    entry := unmarshalAndVerifyEntry(data)

    return entry, nil
}
```

### BinaryEntryReader - The Default Implementation

```go
type BinaryEntryReader struct {
    r  io.Reader
    br *bufio.Reader  // Buffered for performance
}
```

**Reading process:**

```
File bytes: [0x9C, 0x00, 0x00, 0x00, ...156 bytes of data...]
                ↓
1. Read 4 bytes → size = 156
2. Read 156 bytes → entry data
3. Unmarshal protobuf → WAL_Entry struct
4. Verify CRC → ensure not corrupted
5. Return entry
```

### Streaming Pattern

**Key feature:** Returns ONE entry at a time

```go
reader := NewBinaryEntryReader(file)

for {
    entry, err := reader.ReadEntry()
    if err == io.EOF {
        break  // No more entries
    }

    // Process entry
    processEntry(entry)
}

// Memory usage: O(1) - only one entry in memory at a time
```

Compare to loading everything:

```go
// BAD: Loads all entries into memory
entries := readAll()  // If file is 10GB → need 10GB RAM!

for _, entry := range entries {
    processEntry(entry)
}
```

### Verification During Read

```go
func unmarshalAndVerifyEntry(data []byte) (*WAL_Entry, error) {
    // 1. Deserialize
    var entry WAL_Entry
    proto.Unmarshal(data, &entry)

    // 2. Recalculate CRC
    expectedCRC := calculateCRC(entry.Data, entry.LogSequenceNumber)

    // 3. Compare
    if entry.CRC != expectedCRC {
        return nil, fmt.Errorf("CRC mismatch - corrupted entry!")
    }

    return &entry, nil
}
```

**This catches:**

- Disk corruption
- Incomplete writes (crash during write)
- Transmission errors
- Bugs in serialization

---

## 6. How These Components Work Together

Let's trace a complete write operation:

### Writing an Entry

```
Application Code:
    wal.WriteEntry([]byte("transfer $100"))
         ↓
WAL:
    1. Generate LSN = 42
    2. Calculate CRC
    3. Create WAL_Entry{LSN: 42, Data: "...", CRC: 0xABCD}
         ↓
EntryWriter (BinaryEntryWriter):
    4. Serialize to protobuf → 156 bytes
    5. Write [size: 4 bytes][data: 156 bytes] to buffer
         ↓
Buffer (bufio.Writer):
    6. Accumulate in memory
    7. When full or Flush() called → write to underlying writer
         ↓
Underlying Writer (from SegmentManager):
    8. Write bytes to storage (file, S3, etc.)
         ↓
Storage:
    9. Bytes persisted to segment-3
```

### Reading an Entry

```
Application Code:
    entries := wal.ReadAll()
         ↓
WAL:
    1. segments := segmentMgr.ListSegments()  // [0, 1, 2, 3]
    2. For each segment:
         ↓
SegmentManager:
    3. reader := segmentMgr.OpenSegment(segmentID)
         ↓
EntryReader (BinaryEntryReader):
    4. Read [size: 4 bytes]
    5. Read [data: size bytes]
    6. Unmarshal protobuf → WAL_Entry
    7. Verify CRC
    8. Return entry
         ↓
WAL:
    9. Collect all entries from all segments
    10. Return to application
```

### Segment Rotation

```
Writing entry #1000:
    wal.WriteEntry(data)
         ↓
WAL checks size:
    currentSize := segmentMgr.CurrentSegmentSize(currentSegment)
    if currentSize >= maxSegmentSize:
         ↓
Rotation:
    1. entryWriter.Sync()  // Flush current segment
    2. currentWriter.Close()  // Close segment-3
    3. currentSegment++  // Now 4
    4. newWriter := segmentMgr.CreateSegment(4)
    5. entryWriter = NewBinaryEntryWriter(newWriter)
         ↓
Cleanup old segments:
    segments := segmentMgr.ListSegments()  // [0, 1, 2, 3, 4]
    if len(segments) > maxSegments:
        segmentMgr.DeleteSegment(segments[0])  // Delete segment-0
         ↓
Continue writing:
    Write entry to new segment-4
```

---

## Summary: The Component Hierarchy

```
┌──────────────────────────────────────────────────────────┐
│                     Application                          │
│              (Your database, key-value store, etc.)      │
└─────────────────────────┬────────────────────────────────┘
                          │
                          ▼
              ┌───────────────────────┐
              │         WAL           │  ◄─── Orchestrates everything
              │  (Core coordinator)   │       Manages LSN, rotation
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
│    Segments     │  ◄─── Actual files/objects
│  (Physical data)│       segment-0, segment-1, etc.
└─────────────────┘
```

**Each layer has a clear responsibility:**

- **Segments**: Physical storage units
- **SegmentManager**: Where/how to store segments
- **EntryWriter/Reader**: How to serialize/deserialize
- **WAL**: Orchestration, LSN management, rotation logic
- **Application**: Business logic

This separation makes the system flexible, testable, and extensible.
