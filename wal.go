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

// WAL implements the Write-Ahead Logging (WAL) pattern
// It manages the writing of entries to a segment file
// and provides a way to read the entries back in order
// It also manages the segment file rotation and cleanup
// It also provides a way to checkpoint the WAL
// It also provides a way to read the entries back in order
// It also provides a way to checkpoint the WAL
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

// Open opens or creates a WAL
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

// WriteEntry writes a new entry to the WAL
func (w *WAL) WriteEntry(data []byte) (uint64, error) {
	return w.writeEntry(data, false)
}

// WriteCheckpoint writes a checkpoint entry
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

// Sync flushes buffered writes and syncs to disk
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

// Close closes the WAL
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

// ReadAll reads all entries from all segments
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

// ReadFromCheckpoint reads all entries from the last checkpoint
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
