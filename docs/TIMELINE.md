# WAL Visual Guide - How Everything Works Together

## Visual Timeline: WAL Operations

### Scenario: Writing 15 Entries with Rotation

```
Configuration:
- MaxSegmentSize: 5 entries worth of data
- MaxSegments: 3
- SyncInterval: Every 3 writes

Timeline:
════════════════════════════════════════════════════════════════

t=0: WAL Opens
     ┌─────────────┐
     │ segment-0   │ (empty, LSN will start at 1)
     │ (current)   │
     └─────────────┘
     lastLSN = 0

─────────────────────────────────────────────────────────────

t=1: Write entry "A"
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1: "A"  │ ← In buffer (not synced yet)
     └─────────────┘
     lastLSN = 1

t=2: Write entry "B"
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1: "A"  │
     │ LSN 2: "B"  │ ← In buffer
     └─────────────┘
     lastLSN = 2

t=3: Write entry "C" → AUTO-SYNC (3 writes reached)
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1: "A"  │ ✓ Flushed to disk
     │ LSN 2: "B"  │ ✓ Flushed to disk
     │ LSN 3: "C"  │ ✓ Flushed to disk
     └─────────────┘
     lastLSN = 3

─────────────────────────────────────────────────────────────

t=4: Write entries "D", "E"
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1: "A"  │ (on disk)
     │ LSN 2: "B"  │ (on disk)
     │ LSN 3: "C"  │ (on disk)
     │ LSN 4: "D"  │ ← In buffer
     │ LSN 5: "E"  │ ← In buffer
     └─────────────┘
     lastLSN = 5
     Size: 5 entries → At limit!

t=5: Write entry "F" → ROTATION TRIGGERED

     Step 1: Sync segment-0
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1-5     │ ✓ All synced
     └─────────────┘

     Step 2: Close segment-0, Create segment-1
     ┌─────────────┐  ┌─────────────┐
     │ segment-0   │  │ segment-1   │
     │ LSN 1-5     │  │ (new)       │
     │ (closed)    │  │ (current)   │
     └─────────────┘  └─────────────┘

     Step 3: Write "F" to new segment
     ┌─────────────┐  ┌─────────────┐
     │ segment-0   │  │ segment-1   │
     │ LSN 1-5     │  │ LSN 6: "F"  │
     └─────────────┘  └─────────────┘
     lastLSN = 6

─────────────────────────────────────────────────────────────

t=6-8: Write entries "G" through "K" (5 more entries)
       segment-1 fills up, rotates to segment-2

     ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
     │ segment-0   │  │ segment-1   │  │ segment-2   │
     │ LSN 1-5     │  │ LSN 6-10    │  │ LSN 11: "K" │
     └─────────────┘  └─────────────┘  └─────────────┘
     lastLSN = 11

─────────────────────────────────────────────────────────────

t=9-10: Write entries "L" through "P" (5 more entries)
        segment-2 fills up, rotates to segment-3

        BUT: MaxSegments = 3, we already have 3 segments!
        → Delete segment-0 (oldest)

     ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
     │ segment-0   │  │ segment-1   │  │ segment-2   │
     │ DELETED! ×  │  │ LSN 6-10    │  │ LSN 11-15   │
     └─────────────┘  └─────────────┘  └─────────────┘

                      ┌─────────────┐
                      │ segment-3   │
                      │ LSN 16: "P" │
                      └─────────────┘

     Final state: 3 segments (1, 2, 3)
     LSN range: 6-16
     Lost LSNs 1-5 (segment-0 deleted)

════════════════════════════════════════════════════════════════
```

---

## State Machine: WAL Entry Lifecycle

```
┌──────────┐
│ Created  │  Application creates data
└────┬─────┘
     │
     │ wal.WriteEntry(data)
     ▼
┌──────────┐
│ Assigned │  LSN assigned (e.g., LSN 42)
│   LSN    │  CRC calculated
└────┬─────┘
     │
     │ Serialized to protobuf
     ▼
┌──────────┐
│ Buffered │  In application buffer (bufio.Writer)
│ (Memory) │  NOT safe from crash!
└────┬─────┘
     │
     │ Flush() called
     ▼
┌──────────┐
│  Kernel  │  In OS page cache
│  Buffer  │  Visible to reads, NOT safe from crash!
└────┬─────┘
     │
     │ Sync() / fsync() called
     ▼
┌──────────┐
│   Disk   │  Persisted to physical storage
│ (Durable)│  ✓ Safe from crash
└────┬─────┘
     │
     │ Time passes...
     ▼
┌──────────┐
│  Rotated │  Segment becomes read-only
│  (Old)   │  Part of segment-N
└────┬─────┘
     │
     │ More time...
     ▼
┌──────────┐
│ Deleted  │  Segment removed when exceeding MaxSegments
│    ×     │  OR archived to S3/backup
└──────────┘
```

---

## Example: Database Using WAL

### Initial State

```
Database:
  users table: (empty)

WAL:
  lastLSN = 0
  segments = []
```

### Operation Sequence

**1. Insert User "Alice"**

```
Application:
  db.Execute("INSERT INTO users VALUES ('alice', 25)")
       ↓
WAL Write:
  entry = {
    LSN: 1,
    Data: {"op":"INSERT","table":"users","data":{"name":"alice","age":25}},
    CRC: 0xABCD
  }
       ↓
State After:
  Database (in-memory):
    users: [{"name":"alice","age":25}]

  WAL (segment-0):
    LSN 1: INSERT alice
```

**2. Insert User "Bob"**

```
WAL Write:
  LSN 2: INSERT bob

State:
  Database: [alice, bob]
  WAL: LSN 1-2
```

**3. Update Alice's Age**

```
Application:
  db.Execute("UPDATE users SET age=26 WHERE name='alice'")

WAL Write:
  LSN 3: UPDATE alice age=26

State:
  Database: [alice(26), bob(30)]
  WAL: LSN 1-3
```

**4. Create Checkpoint**

```
Application:
  db.CreateCheckpoint()

WAL Write:
  LSN 4: [CHECKPOINT] {"users": [{"name":"alice","age":26}, {"name":"bob","age":30}]}

State:
  Database: [alice(26), bob(30)]
  WAL: LSN 1-4 (LSN 4 is checkpoint)
```

**5. Delete Bob**

```
WAL Write:
  LSN 5: DELETE bob

State:
  Database: [alice(26)]
  WAL: LSN 1-5
```

**6. CRASH! Power Loss**

```
  Database: Lost (was in memory)
  WAL: LSN 1-5 safely on disk
```

**7. Recovery**

```
Step 1: Read from last checkpoint
  entries = [
    LSN 4: [CHECKPOINT] {"users": [alice(26), bob(30)]},
    LSN 5: DELETE bob
  ]

Step 2: Restore from checkpoint
  Database: [alice(26), bob(30)]

Step 3: Replay operations after checkpoint
  Apply LSN 5: DELETE bob
  Database: [alice(26)]

Step 4: Recovery complete!
  Database: [alice(26)] Correct state
```

**Without checkpoint, would need to replay LSN 1-5:**

```
Start: Database empty
LSN 1: INSERT alice(25)
LSN**Without checkpoint, would need to replay LSN 1-5:**
```

Start: Database empty
LSN 1: INSERT alice(25)
LSN 2: INSERT bob(30)
LSN 3: UPDATE alice age=26
LSN 4: (skip - it's just the checkpoint)
LSN 5: DELETE bob
Result: [alice(26)] - Same result, but more work

```

---

## Memory vs Disk States

### During Normal Operation

```

TIME: t=0 (just after writing 3 entries)

Application Buffer (bufio.Writer):
┌─────────────────────────────────────┐
│ LSN 1: "data1" (48 bytes) │
│ LSN 2: "data2" (52 bytes) │
│ LSN 3: "data3" (45 bytes) │
│ │
│ Total buffered: 145 bytes │
└─────────────────────────────────────┘
↓ Not yet flushed
OS Kernel Buffer:
┌─────────────────────────────────────┐
│ (empty) │
└─────────────────────────────────────┘

Physical Disk (segment-0):
┌─────────────────────────────────────┐
│ (empty or old data) │
└─────────────────────────────────────┘

WARNING: CRASH HERE = LOSE ALL 3 ENTRIES!

════════════════════════════════════════════════

TIME: t=1 (after Flush(), before Sync())

Application Buffer:
┌─────────────────────────────────────┐
│ (empty - flushed) │
└─────────────────────────────────────┘
↓ Flushed
OS Kernel Buffer:
┌─────────────────────────────────────┐
│ LSN 1: "data1" │
│ LSN 2: "data2" │
│ LSN 3: "data3" │
└─────────────────────────────────────┘
↓ Not yet synced
Physical Disk:
┌─────────────────────────────────────┐
│ (empty or old data) │
└─────────────────────────────────────┘

WARNING: CRASH HERE = LOSE ALL 3 ENTRIES!
(kernel buffer is lost on power loss)

════════════════════════════════════════════════

TIME: t=2 (after Sync() / fsync())

Application Buffer:
┌─────────────────────────────────────┐
│ (empty) │
└─────────────────────────────────────┘

OS Kernel Buffer:
┌─────────────────────────────────────┐
│ (empty - synced) │
└─────────────────────────────────────┘
↓ Synced
Physical Disk:
┌─────────────────────────────────────┐
│ LSN 1: "data1" │
│ LSN 2: "data2" │
│ LSN 3: "data3" │
└─────────────────────────────────────┘

CRASH HERE = DATA IS SAFE!
(on physical media, survives power loss)

```

---

## Read Path Visualization

### Scenario: Reading from Checkpoint with Multiple Segments

```

WAL State:
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ segment-0 │ │ segment-1 │ │ segment-2 │
├──────────────┤ ├──────────────┤ ├──────────────┤
│ LSN 1: op1 │ │ LSN 6: op6 │ │ LSN 11: op11 │
│ LSN 2: op2 │ │ LSN 7: op7 │ │ LSN 12: [CP] │ ← Checkpoint!
│ LSN 3: [CP] │ │ LSN 8: op8 │ │ LSN 13: op13 │
│ LSN 4: op4 │ │ LSN 9: [CP] │ │ LSN 14: op14 │
│ LSN 5: op5 │ │ LSN 10: op10 │ │ │
└──────────────┘ └──────────────┘ └──────────────┘

Reading Process:
═══════════════════════════════════════════════════

Step 1: Read segment-0
entries = []

Read LSN 1: op1
Not checkpoint → entries = [op1]

Read LSN 2: op2
Not checkpoint → entries = [op1, op2]

Read LSN 3: [CP]
IS CHECKPOINT! → entries = [[CP at LSN 3]] (Reset)

Read LSN 4: op4
Not checkpoint → entries = [[CP], op4]

Read LSN 5: op5
Not checkpoint → entries = [[CP], op4, op5]

Step 2: Read segment-1
Read LSN 6: op6
Not checkpoint → entries = [[CP], op4, op5, op6]

Read LSN 7: op7
Not checkpoint → entries = [[CP], op4, op5, op6, op7]

Read LSN 8: op8
Not checkpoint → entries = [[CP], op4, op5, op6, op7, op8]

Read LSN 9: [CP]
IS CHECKPOINT! → entries = [[CP at LSN 9]] (Reset again)

Read LSN 10: op10
Not checkpoint → entries = [[CP], op10]

Step 3: Read segment-2
Read LSN 11: op11
Not checkpoint → entries = [[CP], op10, op11]

Read LSN 12: [CP]
IS CHECKPOINT! → entries = [[CP at LSN 12]] (Final reset)

Read LSN 13: op13
Not checkpoint → entries = [[CP], op13]

Read LSN 14: op14
Not checkpoint → entries = [[CP], op13, op14]

Final Result:
entries = [
[CP at LSN 12], // Checkpoint entry
op13, // Operations after checkpoint
op14
]

Recovery only needs to:

1. Restore state from checkpoint at LSN 12
2. Replay op13 and op14
3. DONE! (instead of replaying all 14 operations)

```

---

## Concurrent Operations

### Multiple Writers (Thread-Safe)

```

Thread 1 Thread 2 WAL State
════════════════════════════════════════════════════════

Write("A")
├─ Lock()
├─ LSN = 1 lastLSN = 1
├─ Write entry
└─ Unlock()
Write("B")
├─ Lock() [BLOCKED - waiting for Thread 1]

                        ├─ LSN = 2         lastLSN = 2
                        ├─ Write entry
                        └─ Unlock()

Write("C")
├─ Lock()
├─ LSN = 3 lastLSN = 3
└─ Unlock()
Write("D")
├─ Lock()
├─ LSN = 4 lastLSN = 4
└─ Unlock()

Final WAL:
LSN 1: "A"
LSN 2: "B"
LSN 3: "C"
LSN 4: "D"

LSNs are sequential and unique
No race conditions
Order is preserved

```

---

## Error Scenarios

### Scenario 1: Corruption Detected

```

Reading segment-0:

Entry 1 (LSN 1):
Data: "operation1"
CRC (stored): 0x12345678
CRC (calculated): 0x12345678
Match! Entry is valid

Entry 2 (LSN 2):
Data: "operation2"
CRC (stored): 0xABCDEF00
CRC (calculated): 0x99999999
MISMATCH! Entry is corrupted!

Action: Return error, stop reading
Result: Application knows data is corrupted

```

### Scenario 2: Incomplete Write (Crash During Write)

```

Before crash:
Writing entry with size = 150 bytes

Bytes written so far:
[size: 4 bytes] [data: 80 bytes] (CRASH)

After restart:
Reading:
[size = 150] (Expects 150 bytes)
[data: 80 bytes available] (Only 80 bytes)

io.ReadFull() returns error: "unexpected EOF"

Result: Truncate at last valid entry
(repair operation)

```

### Scenario 3: Disk Full During Write

```

Segment state:
Size: 63.9 MB

Write operation:
Entry size: 200 KB
Expected total: 64.1 MB > 64 MB max

Step 1: rotateIfNeeded() called
Step 2: Tries to create segment-4
Step 3: os.Create() fails - disk full!

Error handling:
Return error to application
Application decides: - Retry later? - Delete old segments? - Alert administrator? - Reject write?

```

---

## Performance Characteristics

### Write Throughput vs Sync Strategy

```

Strategy 1: Sync Every Write
────────────────────────────
for i := 0; i < 1000; i++ {
wal.WriteEntry(data)
wal.Sync() // 1000 fsync calls
}

Performance: ~1,000 writes/sec
Latency: ~1ms per write
Data loss risk: 0 entries (max)

Strategy 2: Sync Every 100 Writes
──────────────────────────────────
for i := 0; i < 1000; i++ {
wal.WriteEntry(data)
if i%100 == 0 {
wal.Sync() // 10 fsync calls
}
}

Performance: ~50,000 writes/sec
Latency: ~20μs per write (averaged)
Data loss risk: 100 entries (max)

Strategy 3: Sync on Timer (200ms)
──────────────────────────────────
for i := 0; i < 1000; i++ {
wal.WriteEntry(data)
}
// Background: sync every 200ms

Performance: ~100,000 writes/sec
Latency: ~10μs per write
Data loss risk: 200ms worth of data

Graph:
Throughput
▲
100K│ ● Strategy 3 (timer)
│  
 50K│ ● Strategy 2 (batch)
│  
 1K│ ● Strategy 1 (every write)
│
└─────────────────────────────► Durability
Low Med High

```

---

## Size Calculations

### Example Configuration

```

Configuration:
MaxSegmentSize: 64 MB
MaxSegments: 10
Average entry size: 1 KB

Calculations:
─────────────────────────────────────────────

Entries per segment:
64 MB / 1 KB = 64,000 entries

Total entries stored:
64,000 entries × 10 segments = 640,000 entries

Total disk usage:
64 MB × 10 segments = 640 MB

LSN range:
Current LSN: 1,000,000
Oldest available: 1,000,000 - 640,000 = 360,000
Available range: LSN 360,000 to 1,000,000

Time to fill (at 1000 writes/sec):
640,000 entries / 1000 per sec = 640 seconds
= ~10.6 minutes of history

If crash occurs:
Can recover up to 10.6 minutes of history
Older data is deleted (if no archive)

````

---

## Real-World Example: E-commerce Order System

### Setup
```go
type OrderSystem struct {
    orders map[string]*Order
    wal    *WAL
}

type Order struct {
    ID       string
    Items    []Item
    Total    float64
    Status   string
}
````

### Operation Flow

```
═══════════════════════════════════════════════════════

Event: Customer places order

1. Create order in memory:
   order = Order{
       ID: "ORD-123",
       Items: [{"laptop", 999.99}],
       Total: 999.99,
       Status: "pending"
   }

2. Write to WAL:
   data = {"type":"CREATE_ORDER", "order": order}
   lsn, _ = wal.WriteEntry(marshal(data))

   WAL now has:
   LSN 1001: CREATE_ORDER ORD-123

3. Update in-memory state:
   orders["ORD-123"] = order

4. Return success to customer

═══════════════════════════════════════════════════════

Event: Payment processed

1. Update order:
   order.Status = "paid"

2. Write to WAL:
   data = {"type":"UPDATE_ORDER", "id":"ORD-123", "status":"paid"}
   lsn, _ = wal.WriteEntry(marshal(data))

   WAL now has:
   LSN 1001: CREATE_ORDER ORD-123
   LSN 1002: UPDATE_ORDER ORD-123 → paid

3. Update in-memory:
   orders["ORD-123"].Status = "paid"

═══════════════════════════════════════════════════════

Event: Checkpoint (every 1000 orders)

1. Serialize all orders:
   state = {"orders": orders}  // All current orders

2. Write checkpoint:
   lsn, _ = wal.WriteCheckpoint(marshal(state))

   WAL now has:
   LSN 1001: CREATE_ORDER ORD-123
   LSN 1002: UPDATE_ORDER ORD-123 → paid
   ...
   LSN 2000: [CHECKPOINT] All orders snapshot
   LSN 2001: CREATE_ORDER ORD-124

═══════════════════════════════════════════════════════

Event: CRASH! Server goes down

WAL on disk:
  LSN 1-2000: Various operations
  LSN 2000: [CHECKPOINT]
  LSN 2001-2500: Operations after checkpoint

In-memory state: LOST

═══════════════════════════════════════════════════════

Event: Recovery on restart

1. Read from checkpoint:
   entries = wal.ReadFromCheckpoint()

   Result:
   [
     LSN 2000: [CHECKPOINT] {orders: {...}},
     LSN 2001: CREATE_ORDER ORD-124,
     LSN 2002: UPDATE_ORDER ORD-124,
     ...
     LSN 2500: CREATE_ORDER ORD-150
   ]

2. Restore from checkpoint:
   checkpoint = entries[0]
   orders = unmarshal(checkpoint.Data).orders

   State restored to LSN 2000

3. Replay 500 operations:
   for entry in entries[1:]:
       applyOperation(entry)

   Final state: LSN 2500

4. Resume normal operation:
   System is now consistent!
   Can accept new orders starting at LSN 2501

Recovery time: ~2 seconds
Without checkpoint: Would need to replay 2500 operations (~25 seconds)
```

---

## Summary: The Big Picture

```
Component Interaction Map:

┌─────────────────────────────────────────────────────┐
│                   Application                        │
│              (Your Business Logic)                   │
└───────────────────┬─────────────────────────────────┘
                    │
                    │ WriteEntry(data)
                    ▼
            ┌───────────────┐
            │      WAL      │
            └───────┬───────┘
                    │
         ┏━━━━━━━━━━┻━━━━━━━━━━┓
         ▼                      ▼
┌─────────────────┐    ┌──────────────────┐
│  LSN Generator  │    │  Entry Creation  │
│  (lastLSN++)    │    │  (Data + CRC)    │
└─────────────────┘    └────────┬─────────┘
                                │
                                ▼
                    ┌───────────────────────┐
                    │  Rotation Check       │
                    │  (Size >= Max?)       │
                    └───────┬───────────────┘
                            │
                    ┌───────┴───────┐
                    │ No            │ Yes
                    ▼               ▼
            ┌──────────────┐  ┌─────────────┐
            │ Write Entry  │  │   Rotate    │
            └──────┬───────┘  └──────┬──────┘
                   │                 │
                   │                 ▼
                   │         ┌───────────────┐
                   │         │ Close Old     │
                   │         │ Create New    │
                   │         │ Delete Oldest?│
                   │         └───────┬───────┘
                   │                 │
                   └─────────┬───────┘
                             ▼
                    ┌─────────────────┐
                    │  EntryWriter    │
                    │  (Serialize)    │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  Buffer         │
                    │  (Memory)       │
                    └────────┬────────┘
                             │
                   ┌─────────┴─────────┐
                   │                   │
         Background Timer          Manual Sync()
                   │                   │
                   └─────────┬─────────┘
                             ▼
                    ┌─────────────────┐
                    │  Flush + Fsync  │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │  Physical Disk  │
                    │  DURABLE        │
                    └─────────────────┘
```

**Key Takeaways:**

1. **Entries** are the atomic units of data
2. **Segments** are the physical storage containers
3. **LSN** provides ordering and uniqueness
4. **SegmentManager** abstracts storage backend
5. **EntryWriter/Reader** handle serialization
6. **Buffering** improves performance
7. **Syncing** ensures durability
8. **Rotation** manages segment lifecycle
9. **Checkpoints** optimize recovery
10. **WAL** orchestrates everything together

Each component has a single, clear responsibility, making the system modular, testable, and extensible!# WAL Visual Guide - How Everything Works Together

## Visual Timeline: WAL Operations

### Scenario: Writing 15 Entries with Rotation

```
Configuration:
- MaxSegmentSize: 5 entries worth of data
- MaxSegments: 3
- SyncInterval: Every 3 writes

Timeline:
════════════════════════════════════════════════════════════════

t=0: WAL Opens
     ┌─────────────┐
     │ segment-0   │ (empty, LSN will start at 1)
     │ (current)   │
     └─────────────┘
     lastLSN = 0

─────────────────────────────────────────────────────────────

t=1: Write entry "A"
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1: "A"  │ ← In buffer (not synced yet)
     └─────────────┘
     lastLSN = 1

t=2: Write entry "B"
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1: "A"  │
     │ LSN 2: "B"  │ ← In buffer
     └─────────────┘
     lastLSN = 2

t=3: Write entry "C" → AUTO-SYNC (3 writes reached)
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1: "A"  │ ✓ Flushed to disk
     │ LSN 2: "B"  │ ✓ Flushed to disk
     │ LSN 3: "C"  │ ✓ Flushed to disk
     └─────────────┘
     lastLSN = 3

─────────────────────────────────────────────────────────────

t=4: Write entries "D", "E"
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1: "A"  │ (on disk)
     │ LSN 2: "B"  │ (on disk)
     │ LSN 3: "C"  │ (on disk)
     │ LSN 4: "D"  │ ← In buffer
     │ LSN 5: "E"  │ ← In buffer
     └─────────────┘
     lastLSN = 5
     Size: 5 entries → At limit!

t=5: Write entry "F" → ROTATION TRIGGERED

     Step 1: Sync segment-0
     ┌─────────────┐
     │ segment-0   │
     │ LSN 1-5     │ ✓ All synced
     └─────────────┘

     Step 2: Close segment-0, Create segment-1
     ┌─────────────┐  ┌─────────────┐
     │ segment-0   │  │ segment-1   │
     │ LSN 1-5     │  │ (new)       │
     │ (closed)    │  │ (current)   │
     └─────────────┘  └─────────────┘

     Step 3: Write "F" to new segment
     ┌─────────────┐  ┌─────────────┐
     │ segment-0   │  │ segment-1   │
     │ LSN 1-5     │  │ LSN 6: "F"  │
     └─────────────┘  └─────────────┘
     lastLSN = 6

─────────────────────────────────────────────────────────────

t=6-8: Write entries "G" through "K" (5 more entries)
       segment-1 fills up, rotates to segment-2

     ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
     │ segment-0   │  │ segment-1   │  │ segment-2   │
     │ LSN 1-5     │  │ LSN 6-10    │  │ LSN 11: "K" │
     └─────────────┘  └─────────────┘  └─────────────┘
     lastLSN = 11

─────────────────────────────────────────────────────────────

t=9-10: Write entries "L" through "P" (5 more entries)
        segment-2 fills up, rotates to segment-3

        BUT: MaxSegments = 3, we already have 3 segments!
        → Delete segment-0 (oldest)

     ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
     │ segment-0   │  │ segment-1   │  │ segment-2   │
     │ DELETED! ×  │  │ LSN 6-10    │  │ LSN 11-15   │
     └─────────────┘  └─────────────┘  └─────────────┘

                      ┌─────────────┐
                      │ segment-3   │
                      │ LSN 16: "P" │
                      └─────────────┘

     Final state: 3 segments (1, 2, 3)
     LSN range: 6-16
     Lost LSNs 1-5 (segment-0 deleted)

════════════════════════════════════════════════════════════════
```

---

## State Machine: WAL Entry Lifecycle

```
┌──────────┐
│ Created  │  Application creates data
└────┬─────┘
     │
     │ wal.WriteEntry(data)
     ▼
┌──────────┐
│ Assigned │  LSN assigned (e.g., LSN 42)
│   LSN    │  CRC calculated
└────┬─────┘
     │
     │ Serialized to protobuf
     ▼
┌──────────┐
│ Buffered │  In application buffer (bufio.Writer)
│ (Memory) │  NOT safe from crash!
└────┬─────┘
     │
     │ Flush() called
     ▼
┌──────────┐
│  Kernel  │  In OS page cache
│  Buffer  │  Visible to reads, NOT safe from crash!
└────┬─────┘
     │
     │ Sync() / fsync() called
     ▼
┌──────────┐
│   Disk   │  Persisted to physical storage
│ (Durable)│  ✓ Safe from crash
└────┬─────┘
     │
     │ Time passes...
     ▼
┌──────────┐
│  Rotated │  Segment becomes read-only
│  (Old)   │  Part of segment-N
└────┬─────┘
     │
     │ More time...
     ▼
┌──────────┐
│ Deleted  │  Segment removed when exceeding MaxSegments
│    ×     │  OR archived to S3/backup
└──────────┘
```

---

## Example: Database Using WAL

### Initial State

```
Database:
  users table: (empty)

WAL:
  lastLSN = 0
  segments = []
```

### Operation Sequence

**1. Insert User "Alice"**

```
Application:
  db.Execute("INSERT INTO users VALUES ('alice', 25)")
       ↓
WAL Write:
  entry = {
    LSN: 1,
    Data: {"op":"INSERT","table":"users","data":{"name":"alice","age":25}},
    CRC: 0xABCD
  }
       ↓
State After:
  Database (in-memory):
    users: [{"name":"alice","age":25}]

  WAL (segment-0):
    LSN 1: INSERT alice
```

**2. Insert User "Bob"**

```
WAL Write:
  LSN 2: INSERT bob

State:
  Database: [alice, bob]
  WAL: LSN 1-2
```

**3. Update Alice's Age**

```
Application:
  db.Execute("UPDATE users SET age=26 WHERE name='alice'")

WAL Write:
  LSN 3: UPDATE alice age=26

State:
  Database: [alice(26), bob(30)]
  WAL: LSN 1-3
```

**4. Create Checkpoint**

```
Application:
  db.CreateCheckpoint()

WAL Write:
  LSN 4: [CHECKPOINT] {"users": [{"name":"alice","age":26}, {"name":"bob","age":30}]}

State:
  Database: [alice(26), bob(30)]
  WAL: LSN 1-4 (LSN 4 is checkpoint)
```

**5. Delete Bob**

```
WAL Write:
  LSN 5: DELETE bob

State:
  Database: [alice(26)]
  WAL: LSN 1-5
```

**6. CRASH! Power Loss**

```
  Database: Lost (was in memory)
  WAL: LSN 1-5 safely on disk
```

**7. Recovery**

```
Step 1: Read from last checkpoint
  entries = [
    LSN 4: [CHECKPOINT] {"users": [alice(26), bob(30)]},
    LSN 5: DELETE bob
  ]

Step 2: Restore from checkpoint
  Database: [alice(26), bob(30)]

Step 3: Replay operations after checkpoint
  Apply LSN 5: DELETE bob
  Database: [alice(26)]

Step 4: Recovery complete!
  Database: [alice(26)] Correct state
```

**Without checkpoint, would need to replay LSN 1-5:**

```
Start: Database empty
LSN 1: INSERT alice(25)
LSN
```
