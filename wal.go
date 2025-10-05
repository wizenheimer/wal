// Package wal provides a high-performance Write-Ahead Log (WAL) implementation for Go.
//
// This package implements the Write-Ahead Logging pattern, which ensures durability
// and atomicity by writing all changes to a log before applying them. The WAL supports
// segment rotation, checkpointing, and configurable sync intervals for optimal performance.
//
// Basic usage:
//
//	// Create a segment manager
//	segMgr, err := wal.NewFileSegmentManager("./wal-data")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Open or create the WAL
//	w, err := wal.Open(segMgr, wal.DefaultWALOptions())
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer w.Close()
//
//	// Write entries
//	lsn, err := w.WriteEntry([]byte("data"))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Create checkpoints
//	cpLsn, err := w.WriteCheckpoint([]byte("checkpoint data"))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Read entries back
//	entries, err := w.ReadAll()
//	if err != nil {
//		log.Fatal(err)
//	}
package wal

import (
	"context"
	"fmt"
	"io"
	"log"
	sync "sync"
	"time"
)

var defaultSyncInterval = 3 * time.Second

// WALOptions are the options for the WAL
type WALOptions struct {
	// MaxSegmentSize is the maximum size of a segment
	// in bytes
	MaxSegmentSize int64
	// MaxSegments is the maximum number of segments
	// to keep in memory
	MaxSegments int
	// SyncInterval is the interval at which to sync the WAL
	// to disk
	SyncInterval time.Duration
	// EnableFsync is whether to enable fsync
	// for the WAL
	EnableFsync bool
}

// DefaultWALOptions returns the default WAL options
func DefaultWALOptions() WALOptions {
	return WALOptions{
		MaxSegmentSize: 4 * 1024 * 1024, // 4MB
		MaxSegments:    10,
		SyncInterval:   defaultSyncInterval,
		EnableFsync:    true,
	}
}

// WAL implements the Write-Ahead Logging (WAL) pattern.
//
// WAL provides durable, ordered logging with support for:
//   - Automatic segment rotation based on size limits
//   - Background syncing at configurable intervals
//   - Checkpointing for faster recovery
//   - Thread-safe concurrent writes
//   - Sequential reads from all segments or from last checkpoint
//
// The WAL is safe for concurrent use by multiple goroutines.
type WAL struct {
	// segmentMgr is the segment manager for the WAL
	// it is used to create and manage the segment files
	segmentMgr SegmentManager
	// options are the options for the WAL
	// it is used to configure the WAL
	options WALOptions

	// mu is the mutex for the WAL
	// it is used to protect the WAL
	mu sync.Mutex
	// currentSegment is the current segment for the WAL
	// it is used to write the entries to the current segment
	currentSegment int
	// currentWriter is the current writer for the WAL
	// it is used to write the entries to the current segment
	currentWriter io.WriteCloser
	// entryWriter is the entry writer for the WAL
	// it is used to write the entries to the current segment
	entryWriter *BinaryEntryWriter
	// lastLSN is the last LSN for the WAL
	// it is used to write the entries to the current segment
	lastLSN uint64

	// syncTimer is the timer for the WAL
	// it is used to sync the WAL to disk
	syncTimer *time.Timer

	// ctx is the context for the WAL
	// it is used to cancel the WAL
	ctx context.Context
	// cancel is the cancel function for the WAL
	// it is used to cancel the WAL
	cancel context.CancelFunc
	// wg is the wait group for the WAL
	// it is used to wait for the WAL to finish
	wg sync.WaitGroup
}

// Open opens or creates a WAL with the specified segment manager and options.
//
// If existing segments are found, Open resumes from the last segment and loads
// the last LSN. If no segments exist, it creates a new one starting at segment 0.
//
// Open also starts a background goroutine that periodically syncs the WAL to disk
// based on the configured SyncInterval.
//
// The returned WAL must be closed with Close() to ensure all data is flushed.
func Open(segmentMgr SegmentManager, opts WALOptions) (*WAL, error) {
	segments, err := segmentMgr.ListSegments()
	if err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}

	var currentSegment int
	if len(segments) > 0 {
		currentSegment = segments[len(segments)-1]
	} else {
		currentSegment = 0
	}

	// Open current segment
	writer, err := segmentMgr.CreateSegment(currentSegment)
	if err != nil {
		return nil, fmt.Errorf("open current segment: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	wal := &WAL{
		segmentMgr:     segmentMgr,
		options:        opts,
		currentSegment: currentSegment,
		currentWriter:  writer,
		entryWriter:    NewBinaryEntryWriter(writer),
		syncTimer:      time.NewTimer(opts.SyncInterval),
		ctx:            ctx,
		cancel:         cancel,
	}

	// Read last LSN from current segment
	if err := wal.loadLastLSN(); err != nil {
		writer.Close()
		cancel()
		return nil, fmt.Errorf("load last LSN: %w", err)
	}

	// Start background sync
	wal.wg.Add(1)
	go wal.syncLoop()

	return wal, nil
}

// loadLastLSN loads the last LSN from the current segment
func (w *WAL) loadLastLSN() error {
	reader, err := w.segmentMgr.OpenSegment(w.currentSegment)
	if err != nil {
		return err
	}
	defer reader.Close()

	entryReader := NewBinaryEntryReader(reader)
	var lastEntry *WAL_Entry

	for {
		entry, err := entryReader.ReadEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read entry: %w", err)
		}
		lastEntry = entry
	}

	if lastEntry != nil {
		w.lastLSN = lastEntry.LogSequenceNumber
	}

	return nil
}

// WriteEntry writes a new entry to the WAL and returns its Log Sequence Number (LSN).
//
// The LSN is a monotonically increasing identifier that uniquely identifies this entry.
// WriteEntry automatically handles segment rotation when the current segment exceeds
// MaxSegmentSize.
//
// This method is thread-safe and can be called concurrently from multiple goroutines.
func (w *WAL) WriteEntry(data []byte) (uint64, error) {
	return w.writeEntry(data, false)
}

// WriteCheckpoint writes a checkpoint entry to the WAL and returns its LSN.
//
// A checkpoint marks a known good state in the log. When reading with ReadFromCheckpoint,
// only entries from the most recent checkpoint onwards are returned, allowing for faster
// recovery by skipping earlier entries.
//
// WriteCheckpoint forces a sync before writing the checkpoint to ensure all prior entries
// are durably stored.
//
// This method is thread-safe and can be called concurrently from multiple goroutines.
func (w *WAL) WriteCheckpoint(data []byte) (uint64, error) {
	return w.writeEntry(data, true)
}

// writeEntry writes a new entry to the WAL
// it is used to write a new entry to the WAL
// and rotates the segment if needed
func (w *WAL) writeEntry(data []byte, isCheckpoint bool) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if rotation needed
	if err := w.rotateIfNeeded(); err != nil {
		return 0, fmt.Errorf("rotate: %w", err)
	}

	// Generate LSN
	w.lastLSN++
	lsn := w.lastLSN

	// Create entry
	entry := NewEntry(lsn, data)

	if isCheckpoint {
		// Sync before checkpoint
		if err := w.entryWriter.Sync(); err != nil {
			return 0, fmt.Errorf("sync before checkpoint: %w", err)
		}
		isCP := true
		entry.IsCheckpoint = &isCP
	}

	// Write entry
	if err := w.entryWriter.WriteEntry(entry); err != nil {
		return 0, fmt.Errorf("write entry: %w", err)
	}

	return lsn, nil
}

// rotateIfNeeded checks if the current segment is full
// and rotates the segment if needed
func (w *WAL) rotateIfNeeded() error {
	size, err := w.segmentMgr.CurrentSegmentSize(w.currentSegment)
	if err != nil {
		return err
	}

	buffered := int64(w.entryWriter.BufferedBytes())
	if size+buffered < w.options.MaxSegmentSize {
		return nil
	}

	return w.rotate()
}

// rotate rotates the current segment
// and creates a new segment
// it cleans up old segments if needed
func (w *WAL) rotate() error {
	// Sync and close current segment
	if err := w.entryWriter.Sync(); err != nil {
		return fmt.Errorf("sync before rotation: %w", err)
	}

	if err := w.currentWriter.Close(); err != nil {
		return fmt.Errorf("close current segment: %w", err)
	}

	// Cleanup old segments if needed
	segments, err := w.segmentMgr.ListSegments()
	if err != nil {
		return err
	}

	if len(segments) >= w.options.MaxSegments {
		if err := w.segmentMgr.DeleteSegment(segments[0]); err != nil {
			log.Printf("Warning: failed to delete old segment: %v", err)
		}
	}

	// Create new segment
	w.currentSegment++
	writer, err := w.segmentMgr.CreateSegment(w.currentSegment)
	if err != nil {
		return fmt.Errorf("create new segment: %w", err)
	}

	w.currentWriter = writer
	w.entryWriter = NewBinaryEntryWriter(writer)

	return nil
}

// Sync flushes buffered writes and syncs to disk if fsync is enabled.
//
// Sync is called automatically by the background sync loop at the configured
// SyncInterval, but can also be called manually to ensure durability of recent writes.
//
// This method is thread-safe.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.entryWriter.Sync(); err != nil {
		return err
	}

	w.syncTimer.Reset(w.options.SyncInterval)
	return nil
}

// syncLoop is the loop for the WAL
// it is used to sync the WAL to disk
// at the specified interval
// this ensures that failures are isolated to a single sync
func (w *WAL) syncLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.syncTimer.C:
			if err := w.Sync(); err != nil {
				log.Printf("WAL sync error: %v", err)
			}
		case <-w.ctx.Done():
			return
		}
	}
}

// Close gracefully shuts down the WAL, stopping the background sync loop
// and flushing all buffered data to disk.
//
// Close must be called to ensure all data is durably stored. After Close is called,
// the WAL should not be used.
func (w *WAL) Close() error {
	w.cancel()
	w.wg.Wait()

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.entryWriter.Sync(); err != nil {
		return err
	}

	return w.currentWriter.Close()
}

// ReadAll reads all entries from all segments in order.
//
// This method reads every entry across all segment files, verifying CRC checksums
// and returning them in LSN order. Use ReadFromCheckpoint for faster recovery that
// skips entries before the last checkpoint.
//
// This method is safe to call while the WAL is actively being written to.
func (w *WAL) ReadAll() ([]*WAL_Entry, error) {
	segments, err := w.segmentMgr.ListSegments()
	if err != nil {
		return nil, err
	}

	var entries []*WAL_Entry

	for _, segID := range segments {
		reader, err := w.segmentMgr.OpenSegment(segID)
		if err != nil {
			return nil, fmt.Errorf("open segment %d: %w", segID, err)
		}

		segEntries, err := ReadAllEntries(reader)
		reader.Close()

		if err != nil {
			return nil, fmt.Errorf("read segment %d: %w", segID, err)
		}

		entries = append(entries, segEntries...)
	}

	return entries, nil
}

// ReadFromCheckpoint reads all entries from the last checkpoint onwards.
//
// This method scans all segments and returns only entries from the most recent
// checkpoint forward, discarding earlier entries. This enables faster recovery
// by replaying only the necessary entries to restore state.
//
// If no checkpoint is found, all entries are returned (equivalent to ReadAll).
//
// This method is safe to call while the WAL is actively being written to.
func (w *WAL) ReadFromCheckpoint() ([]*WAL_Entry, error) {
	segments, err := w.segmentMgr.ListSegments()
	if err != nil {
		return nil, err
	}

	var entries []*WAL_Entry
	var checkpointLSN uint64

	for _, segID := range segments {
		reader, err := w.segmentMgr.OpenSegment(segID)
		if err != nil {
			return nil, fmt.Errorf("open segment %d: %w", segID, err)
		}

		segEntries, cpLSN, err := ReadEntriesWithCheckpoint(reader)
		reader.Close()

		if err != nil {
			return nil, fmt.Errorf("read segment %d: %w", segID, err)
		}

		// If we found a checkpoint, reset entries
		if cpLSN > checkpointLSN {
			entries = segEntries
			checkpointLSN = cpLSN
		} else {
			entries = append(entries, segEntries...)
		}
	}

	return entries, nil
}
