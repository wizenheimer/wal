# WAL Core Concepts Deep Dive

This document covers the key operational concepts that make the WAL work.

---

## 7. Log Sequence Number (LSN)

**The LSN is the backbone of the entire WAL system.**

### What is an LSN?

A Log Sequence Number is:

- A **unique, monotonically increasing** integer
- Assigned to **every entry** in the WAL
- **Never reused** or skipped
- Establishes a **total ordering** of all operations

```go
Entry 1: LSN = 1
Entry 2: LSN = 2
Entry 3: LSN = 3
...
Entry 1000: LSN = 1000
```

### Why LSN Matters

**1. Total Ordering**

```
If LSN(A) < LSN(B), then operation A happened before operation B
```

This is critical for:

- **Recovery**: Replay operations in exact order
- **Replication**: Slaves know what order to apply changes
- **Consistency**: Maintain causality of operations

**2. Uniqueness**

```
Each operation has exactly one LSN
No two operations share an LSN
```

Allows you to:

- **Reference** specific operations ("replay from LSN 5000")
- **Track progress** ("slave is at LSN 3000, master at LSN 5000")
- **Detect missing** operations (LSN 100, 101, 103 → missing 102!)

**3. Gap Detection**

```
Expected: 1, 2, 3, 4, 5
Found:    1, 2,    4, 5
                ↑ Gap! Entry 3 is missing
```

### How LSN is Generated

```go
type WAL struct {
    lastLSN uint64  // Tracks the last assigned LSN
    mu      sync.Mutex
}

func (w *WAL) WriteEntry(data []byte) (uint64, error) {
    w.mu.Lock()
    defer w.mu.Unlock()

    // Generate new LSN
    w.lastLSN++           // Increment: 41 → 42
    lsn := w.lastLSN      // Assign: lsn = 42

    entry := &WAL_Entry{
        LogSequenceNumber: lsn,  // Use new LSN
        Data:              data,
        // ...
    }

    return lsn, w.entryWriter.WriteEntry(entry)
}
```

**Key properties:**

- **Thread-safe**: Mutex ensures no duplicate LSNs
- **Sequential**: Always increments by 1
- **In-memory counter**: Doesn't require disk read

### LSN Across Restarts

**Problem:** After a crash, what's the next LSN?

**Solution:** Read the last entry from the log

```go
func (w *WAL) loadLastLSN() error {
    // Open current segment
    reader := w.segmentMgr.OpenSegment(w.currentSegment)

    // Read all entries
    entryReader := NewBinaryEntryReader(reader)
    var lastEntry *WAL_Entry

    for {
        entry, err := entryReader.ReadEntry()
        if err == io.EOF {
            break
        }
        lastEntry = entry  // Keep updating to find last
    }

    if lastEntry != nil {
        w.lastLSN = lastEntry.LogSequenceNumber  // Resume from here
    }

    return nil
}
```

**After restart:**

```
Before crash: Last LSN = 1000
After restart: Read log → find LSN 1000
Next write: LSN = 1001 ✓
```

### LSN and Segment Mapping

LSNs span across segments:

```
segment-0: LSN 1    → 500   (500 entries)
segment-1: LSN 501  → 1000  (500 entries)
segment-2: LSN 1001 → 1500  (500 entries)
segment-3: LSN 1501 → ...   (current, growing)
```

**To find entry with LSN 750:**

1. Check segment-0: LSN 1-500 → Not here
2. Check segment-1: LSN 501-1000 → Found! (750 is in this range)
3. Scan segment-1 until you find entry with LSN 750

---

## 8. What is Syncing (Flush vs Sync)?

**Understanding the write path is critical for durability.**

### The Write Journey

When you write data, it goes through multiple layers:

```
Application
    ↓ WriteEntry()
WAL (in-memory)
    ↓ entryWriter.WriteEntry()
Buffer (bufio.Writer) ← Data sits here
    ↓ Flush()
Kernel buffer (OS page cache)
    ↓ Sync() / fsync()
Disk (physical storage)
```

### Flush() - Memory to Kernel

```go
func (bew *BinaryEntryWriter) Flush() error {
    return bew.bw.Flush()  // bufio → kernel
}
```

**What it does:**

- Moves data from **application buffer** → **OS kernel buffer**
- Data is now in kernel memory (still not on disk!)
- Fast operation (~microseconds)

**After Flush():**

```
+ Data visible to other processes reading the file
- Data NOT safe from crash (still in RAM)
```

### Sync() - Kernel to Disk

```go
func (bew *BinaryEntryWriter) Sync() error {
    bew.Flush()  // First flush to kernel

    if bew.syncWriter != nil {
        return bew.syncWriter.Sync()  // Then fsync to disk
    }
    return nil
}
```

**What it does:**

- Calls `fsync()` system call
- Forces kernel to write **kernel buffer** → **physical disk**
- Waits until disk confirms write
- Slow operation (~milliseconds)

**After Sync():**

```
+ Data visible to other processes
+ Data safe from crash (on physical media)
```

### Why Not Sync Every Write?

**Performance impact:**

```go
// BAD: Sync after every write
for i := 0; i < 1000; i++ {
    wal.WriteEntry(data)
    wal.Sync()  // 1000 fsync calls → ~1000ms
}

// GOOD: Batch writes, sync periodically
for i := 0; i < 1000; i++ {
    wal.WriteEntry(data)  // Buffered in memory
}
wal.Sync()  // 1 fsync call → ~1ms
```

**Throughput difference:**

- Sync every write: ~1,000 writes/second
- Sync every 100 writes: ~100,000 writes/second
- **100x faster!**

### Sync Strategies

**1. Sync on Timer (Default)**

```go
// Sync every 200ms automatically
opts.SyncInterval = 200 * time.Millisecond

// Background goroutine
func (w *WAL) syncLoop() {
    for {
        select {
        case <-w.syncTimer.C:
            w.Sync()  // Auto-sync
        }
    }
}
```

**Trade-off:**

- Good throughput
- May lose up to 200ms of data on crash

**2. Sync on Every Write**

```go
func (w *WAL) WriteEntry(data []byte) error {
    // ... write entry
    return w.Sync()  // Force sync
}
```

**Trade-off:**

- Maximum durability (no data loss)
- Poor throughput

**3. Sync on Batch**

```go
for i := 0; i < 100; i++ {
    wal.WriteEntry(data)
}
wal.Sync()  // Sync after 100 writes
```

**Trade-off:**

- Balanced throughput/durability
- May lose up to 100 entries on crash

### The Durability Triangle

```
         Fast
          ▲
         / \
        /   \
       /     \
      / Sync  \
     /  Every  \
    /   Timer   \
   /             \
  ◄───────────────►
Durable         Throughput
(Sync          (Never sync)
 every
 write)
```

Pick two!

---

## 9. What are Checkpoints?

**A checkpoint is a snapshot of your application state saved in the WAL.**

### The Recovery Problem

Imagine a database WAL with 1 million operations:

```
LSN 1: CREATE TABLE users
LSN 2: INSERT user1
LSN 3: INSERT user2
...
LSN 1,000,000: UPDATE user500
```

**On crash recovery:**

- Must replay ALL 1 million operations
- Takes hours!
- Most operations are redundant (insert then delete same row)

### The Checkpoint Solution

Periodically save a **full snapshot** of state:

```
LSN 1: CREATE TABLE users
LSN 2: INSERT user1
...
LSN 500,000: UPDATE user250

[CHECKPOINT at LSN 500,001]
Data: Serialized state of entire database
      (all tables, all rows, all indexes)

LSN 500,002: INSERT user251
LSN 500,003: UPDATE user100
...
LSN 1,000,000: DELETE user50

[CHECKPOINT at LSN 1,000,001]
Data: New serialized state

LSN 1,000,002: ...
```

**On crash recovery:**

1. Find last checkpoint (LSN 1,000,001)
2. Restore state from checkpoint
3. Replay only operations AFTER checkpoint (LSN 1,000,002+)
4. Recovery time: **seconds instead of hours**

### How Checkpoints Work

**Creating a checkpoint:**

```go
func (w *WAL) WriteCheckpoint(data []byte) (uint64, error) {
    w.mu.Lock()
    defer w.mu.Unlock()

    // CRITICAL: Sync before checkpoint
    // Ensures all previous operations are persisted
    if err := w.entryWriter.Sync(); err != nil {
        return 0, err
    }

    // Create checkpoint entry
    w.lastLSN++
    isCP := true
    entry := &WAL_Entry{
        LogSequenceNumber: w.lastLSN,
        Data:              data,  // Snapshot of application state
        CRC:               calculateCRC(data, w.lastLSN),
        IsCheckpoint:      &isCP,  // Marked as checkpoint
    }

    return w.lastLSN, w.entryWriter.WriteEntry(entry)
}
```

**Key steps:**

1. **Sync first** - Ensure WAL is consistent
2. **Serialize state** - Convert app state to bytes
3. **Mark specially** - Set `IsCheckpoint = true`
4. **Write to WAL** - Just like any other entry

**Reading from checkpoint:**

```go
func (w *WAL) ReadFromCheckpoint() ([]*WAL_Entry, error) {
    var entries []*WAL_Entry
    var checkpointLSN uint64

    // Read all segments
    for _, segID := range segments {
        reader := w.segmentMgr.OpenSegment(segID)
        entryReader := NewBinaryEntryReader(reader)

        for {
            entry, err := entryReader.ReadEntry()
            if err == io.EOF {
                break
            }

            // Found a checkpoint?
            if entry.IsCheckpoint != nil && *entry.IsCheckpoint {
                // Discard all previous entries
                entries = []*WAL_Entry{entry}
                checkpointLSN = entry.LogSequenceNumber
            } else {
                // Accumulate entries after checkpoint
                entries = append(entries, entry)
            }
        }
        reader.Close()
    }

    return entries, nil
}
```

**Logic:**

- Scan all entries
- When checkpoint found → **discard everything before**
- Keep only checkpoint + entries after
- If multiple checkpoints → use the **last one**

### Example: Database Checkpoint

```go
type Database struct {
    tables map[string]*Table
    wal    *WAL
}

func (db *Database) CreateCheckpoint() error {
    // 1. Serialize current database state
    state := &DatabaseState{
        Tables: db.tables,
        // ... all data structures
    }
    data, _ := json.Marshal(state)

    // 2. Write to WAL
    lsn, err := db.wal.WriteCheckpoint(data)
    if err != nil {
        return err
    }

    log.Printf("Checkpoint created at LSN %d", lsn)
    return nil
}

func (db *Database) Recover() error {
    // 1. Read from last checkpoint
    entries, err := db.wal.ReadFromCheckpoint()
    if err != nil {
        return err
    }

    if len(entries) == 0 {
        // No checkpoint - full replay needed
        return db.replayAll()
    }

    // 2. Restore state from checkpoint
    checkpointEntry := entries[0]
    var state DatabaseState
    json.Unmarshal(checkpointEntry.Data, &state)
    db.tables = state.Tables

    log.Printf("Restored from checkpoint at LSN %d",
        checkpointEntry.LogSequenceNumber)

    // 3. Replay operations after checkpoint
    for i := 1; i < len(entries); i++ {
        db.applyOperation(entries[i])
    }

    log.Printf("Replayed %d operations", len(entries)-1)
    return nil
}
```

### Checkpoint Frequency

**Too frequent:**

- Expensive (serializing large state)
- Wastes disk space
- Fast recovery

**Too infrequent:**

- Low overhead
- Saves disk space
- Slow recovery

**Common strategies:**

1. **Time-based**: Every 5 minutes
2. **Size-based**: Every 10,000 operations
3. **Hybrid**: Every 5 min OR 10,000 ops (whichever comes first)

```go
// Checkpoint every 5 minutes or 10,000 ops
ticker := time.NewTicker(5 * time.Minute)
opCount := 0

for {
    select {
    case <-ticker.C:
        db.CreateCheckpoint()
        opCount = 0
    default:
        db.WriteOperation(...)
        opCount++
        if opCount >= 10000 {
            db.CreateCheckpoint()
            opCount = 0
        }
    }
}
```

---

## 10. Segment Rotation

**Rotation is the process of closing the current segment and starting a new one.**

### Why Rotate?

**Problem:** If we write to one file forever:

- File grows infinitely large
- Slow to open/read
- Can't delete old data
- One corruption affects everything

**Solution:** Rotate to new segments periodically

### When to Rotate?

**Trigger: Current segment reaches max size**

```go
func (w *WAL) rotateIfNeeded() error {
    // Get current segment size
    size, _ := w.segmentMgr.CurrentSegmentSize(w.currentSegment)

    // Add buffered bytes not yet flushed
    buffered := int64(w.entryWriter.BufferedBytes())

    // Check if rotation needed
    if size + buffered >= w.options.MaxSegmentSize {
        return w.rotate()
    }

    return nil
}
```

**Example:**

```
MaxSegmentSize = 64 MB

segment-3:
  Current size on disk: 63.8 MB
  Buffered in memory: 0.3 MB
  Total: 64.1 MB ≥ 64 MB → ROTATE!
```

### Rotation Process

```go
func (w *WAL) rotate() error {
    // STEP 1: Sync current segment
    // Ensures all buffered data is written
    if err := w.entryWriter.Sync(); err != nil {
        return err
    }

    // STEP 2: Close current segment
    if err := w.currentWriter.Close(); err != nil {
        return err
    }

    // STEP 3: Delete old segments if needed
    segments, _ := w.segmentMgr.ListSegments()
    if len(segments) >= w.options.MaxSegments {
        // Remove oldest segment
        w.segmentMgr.DeleteSegment(segments[0])
    }

    // STEP 4: Create new segment
    w.currentSegment++  // Increment: 3 → 4
    newWriter, err := w.segmentMgr.CreateSegment(w.currentSegment)
    if err != nil {
        return err
    }

    // STEP 5: Setup new writer
    w.currentWriter = newWriter
    w.entryWriter = NewBinaryEntryWriter(newWriter)

    return nil
}
```

### Rotation Timeline

```
Time    Segment    Size        Action
──────────────────────────────────────────
t=0     seg-0      0 MB        Writing...
t=1     seg-0      32 MB       Writing...
t=2     seg-0      64 MB       ROTATE!
t=2     seg-1      0 MB        New segment
t=3     seg-1      32 MB       Writing...
t=4     seg-1      64 MB       ROTATE!
t=4     seg-2      0 MB        New segment
...
```

### Max Segments Management

**Problem:** If we keep rotating, we'll have infinite segments

**Solution:** Delete oldest when exceeding max

```
MaxSegments = 5

Segments: [0, 1, 2, 3, 4]  ← At limit
Rotate → Create segment-5
Check: 6 segments > 5 max
Delete: segment-0  ← Oldest removed
Result: [1, 2, 3, 4, 5]  ← Back to limit
```

**Code:**

```go
segments, _ := w.segmentMgr.ListSegments()

if len(segments) >= w.options.MaxSegments {
    oldestSegment := segments[0]  // First in list
    w.segmentMgr.DeleteSegment(oldestSegment)
}
```

### Rotation and LSN

**Important:** LSN continues across rotations

```
segment-2 (being rotated):
  Last entry: LSN 1500

Rotate → segment-3 created

segment-3 (new):
  First entry: LSN 1501  ← Continues from previous
```

LSN is **global** across all segments, not per-segment.

---

## 11. WAL Options

**Configuration that controls WAL behavior**

### Options Structure

```go
type WALOptions struct {
    MaxSegmentSize int64          // Max bytes per segment
    MaxSegments    int             // Max number of segments to keep
    SyncInterval   time.Duration   // How often to auto-sync
    EnableFsync    bool            // Whether to call fsync
}
```

### Default Options

```go
func DefaultWALOptions() WALOptions {
    return WALOptions{
        MaxSegmentSize: 64 * 1024 * 1024,    // 64 MB
        MaxSegments:    10,                   // Keep 10 segments
        SyncInterval:   200 * time.Millisecond, // Sync every 200ms
        EnableFsync:    true,                 // Call fsync
    }
}
```

### MaxSegmentSize

**What it controls:** Size of each segment file

**Small values (e.g., 1 MB):**

- More frequent rotation
- Smaller individual files
- Easier to manage/backup
- More overhead (opening/closing files)
- More segments to scan during recovery

**Large values (e.g., 1 GB):**

- Less rotation overhead
- Fewer segments to manage
- Large individual files
- Harder to backup/transfer
- More disk space needed

**Recommendation:**

- **SSD**: 64-128 MB (fast I/O, rotation is cheap)
- **HDD**: 256-512 MB (minimize seeking)
- **Cloud (S3)**: 100-500 MB (minimize API calls)

### MaxSegments

**What it controls:** Maximum number of segments to keep

**Trade-off:**

```
MaxSegments = 5, MaxSegmentSize = 64 MB
Maximum WAL size = 5 × 64 MB = 320 MB

Old data beyond 320 MB is deleted!
```

**Small values (e.g., 3):**

- Bounded disk usage
- Less history available
- Frequent deletions

**Large values (e.g., 100):**

- Long history available
- Unbounded disk growth
- Infrequent deletions

**Recommendation:**

- **Development**: 3-5 segments (limited disk)
- **Production**: 20-50 segments (retain history)
- **Archival**: Infinite + archive old segments to S3

### SyncInterval

**What it controls:** How often the background sync happens

**Fast sync (e.g., 50ms):**

- High durability (lose max 50ms of data)
- More fsync calls, more disk I/O
- Lower throughput

**Slow sync (e.g., 1s):**

- Better throughput
- Lower durability (lose up to 1s of data)
- Fewer fsync calls

**Recommendation:**

- **Financial systems**: 10-50ms (high durability)
- **General apps**: 100-500ms (balanced)
- **Analytics**: 1-5s (throughput > durability)

### EnableFsync

**What it controls:** Whether to call `fsync()` at all

**Enabled (true):**

- Data survives crashes/power loss
- Slower writes (disk I/O)

**Disabled (false):**

- Much faster writes
- Data may be lost on crash
- Only safe if durability is not required

**Recommendation:**

- **Production**: Always `true`
- **Testing**: `false` for speed
- **Development**: `false` unless testing recovery

---

## 12. The Complete WAL Struct

Now let's understand the main WAL struct:

```go
type WAL struct {
    // Storage abstraction
    segmentMgr      SegmentManager  // Where segments are stored

    // Configuration
    options         WALOptions      // Behavior settings

    // Current state (protected by mutex)
    mu              sync.Mutex      // Protects below fields
    currentSegment  int             // Current segment ID (e.g., 5)
    currentWriter   io.WriteCloser  // Writer for current segment
    entryWriter     *BinaryEntryWriter  // Serializer
    lastLSN         uint64          // Last assigned LSN

    // Background sync
    syncTimer       *time.Timer     // Timer for auto-sync
    ctx             context.Context // For graceful shutdown
    cancel          context.CancelFunc
    wg              sync.WaitGroup  // Wait for goroutines
}
```

### Field Breakdown

**segmentMgr**: Storage backend

- File system, S3, Redis, etc.
- Provides segment create/open/delete/list

**options**: Configuration

- MaxSegmentSize, MaxSegments, etc.
- Controls behavior

**mu**: Mutex for thread-safety

- Protects all mutable state
- Ensures LSN uniqueness
- Prevents race conditions

**currentSegment**: Which segment we're writing to

- Example: `3` means writing to `segment-3`
- Increments during rotation

**currentWriter**: Open file handle

- Points to the current segment's file
- Closed during rotation, new one opened

**entryWriter**: Serialization layer

- Wraps `currentWriter`
- Handles protobuf encoding
- Manages buffering

**lastLSN**: Last assigned LSN

- Increments with each write
- Used to generate next LSN
- Persists across restarts

**syncTimer**: Background sync timer

- Fires every `SyncInterval`
- Triggers automatic sync
- Reduces data loss window

**ctx/cancel**: Graceful shutdown

- Signals background goroutine to stop
- Ensures clean termination

**wg**: Wait group

- Waits for background goroutine
- Ensures all work completes before Close()

---

## 13. Putting It All Together

### Complete Write Flow

```
1. Application calls: wal.WriteEntry(data)
                         ↓
2. WAL locks mutex (thread-safety)
                         ↓
3. Check if rotation needed:
   - Get current segment size
   - If >= MaxSegmentSize → rotate()
                         ↓
4. Generate LSN:
   - lastLSN++
   - lsn = lastLSN (e.g., 42)
                         ↓
5. Calculate CRC:
   - crc = CRC32(data + lsn)
                         ↓
6. Create entry:
   - entry = WAL_Entry{LSN: 42, Data: data, CRC: crc}
                         ↓
7. Serialize:
   - bytes = protobuf.Marshal(entry)
                         ↓
8. EntryWriter writes:
   - Write [size: 4 bytes][bytes: N bytes]
   - Buffer in memory (bufio)
                         ↓
9. Unlock mutex
                         ↓
10. Background timer fires (every 200ms)
                         ↓
11. Auto-sync:
    - Flush buffer → kernel
    - fsync → disk
                         ↓
12. Data now durable!
```

### Complete Read Flow

```
1. Application calls: wal.ReadFromCheckpoint()
                         ↓
2. Get all segment IDs:
   - segments = [0, 1, 2, 3]
                         ↓
3. For each segment:
                         ↓
4. Open segment:
   - reader = segmentMgr.OpenSegment(segID)
                         ↓
5. Create EntryReader:
   - entryReader = NewBinaryEntryReader(reader)
                         ↓
6. Read entries one by one:
   - Read [size: 4 bytes]
   - Read [data: size bytes]
   - Unmarshal protobuf
   - Verify CRC
                         ↓
7. Check for checkpoint:
   - If entry.IsCheckpoint == true
     → Discard all previous entries
     → Start fresh from checkpoint
                         ↓
8. Accumulate entries
                         ↓
9. Close segment reader
                         ↓
10. Move to next segment
                         ↓
11. Return all entries from last checkpoint
```

### Complete Lifecycle

```
1. OPEN
   wal, _ := Open(segmentMgr, opts)
   - List existing segments
   - Open last segment
   - Read last LSN
   - Start background sync goroutine

2. WRITE
   for i := 0; i < 1000; i++ {
       wal.WriteEntry(data)
   }
   - Generate LSNs: 1, 2, 3, ...
   - Write to current segment
   - Auto-rotate when segment full
   - Background sync every 200ms

3. CHECKPOINT
   wal.WriteCheckpoint(stateSnapshot)
   - Sync first
   - Write checkpoint entry
   - Mark with IsCheckpoint = true

4. CLOSE
   wal.Close()
   - Stop background goroutine
   - Final sync
   - Close current segment
   - Clean shutdown

5. RECOVER (next startup)
   entries, _ := wal.ReadFromCheckpoint()
   - Read from last checkpoint
   - Replay operations
   - Resume normal operation
```

---

## Summary: How Everything Fits

```
┌─────────────────────────────────────────┐
│         Application Layer               │
│   (Database, KV Store, etc.)            │
└─────────────┬───────────────────────────┘
              │
              ▼
    ┌─────────────────────┐
    │     WAL Struct      │ ← Orchestrator
    ├─────────────────────┤
    │ • LSN management    │
    │ • Rotation logic    │
    │ • Thread safety     │
    │ • Background sync   │
    └──┬──────────────┬───┘
       │              │
       ▼              ▼
┌──────────────┐  ┌────────────────┐
│ SegmentMgr   │  │ EntryWriter/   │
│              │  │ Reader         │
├──────────────┤  ├────────────────┤
│ Storage      │  │ Serialization  │
│ abstraction  │  │ Buffering      │
└──────┬───────┘  └────────┬───────┘
       │                   │
       ▼                   ▼
┌──────────────┐  ┌────────────────┐
│  Segments    │  │  Binary        │
│  (Files)     │  │  Format        │
└──────────────┘  └────────────────┘
```

**Every component has one job:**

- **Segments**: Physical storage containers
- **LSN**: Ordering and uniqueness
- **SegmentManager**: Storage abstraction
- **EntryWriter/Reader**: Serialization
- **Sync**: Durability control
- **Checkpoints**: Recovery optimization
- **Rotation**: Segment lifecycle
- **WAL**: Orchestrate everything

This separation makes the system modular, testable, and extensible!
