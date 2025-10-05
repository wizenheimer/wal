package wal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	sync "sync"
)

var segmentPrefix = "segment-"

// SegmentManager handles segment file operations for the WAL.
//
// SegmentManager provides an abstraction for managing the individual segment
// files that make up the WAL, allowing for different storage backends (filesystem,
// cloud storage, etc.).
type SegmentManager interface {
	// CreateSegment creates a new segment with the given ID and returns a writer.
	// If the segment already exists, it is opened in append mode.
	CreateSegment(id int) (io.WriteCloser, error)

	// OpenSegment opens an existing segment for reading.
	OpenSegment(id int) (io.ReadCloser, error)

	// ListSegments returns all segment IDs in ascending order.
	ListSegments() ([]int, error)

	// DeleteSegment removes a segment file.
	DeleteSegment(id int) error

	// CurrentSegmentSize returns the current size in bytes of the segment.
	CurrentSegmentSize(id int) (int64, error)
}

// FileSegmentManager implements SegmentManager for filesystem storage.
//
// Segments are stored as files named "segment-N" where N is the segment ID.
// FileSegmentManager is safe for concurrent use.
type FileSegmentManager struct {
	// directory is the directory to store the segments
	directory string
	// mu is the mutex to protect the segment files
	mu sync.RWMutex
}

// NewFileSegmentManager creates a new FileSegmentManager for the given directory.
//
// The directory is created if it doesn't exist. All segment files will be stored
// in this directory with the naming pattern "segment-N".
func NewFileSegmentManager(directory string) (*FileSegmentManager, error) {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	return &FileSegmentManager{directory: directory}, nil
}

// CreateSegment creates or opens a segment file for writing in append mode.
//
// The file is opened with O_CREATE|O_WRONLY|O_APPEND flags, allowing writes
// to resume from the end of an existing segment.
func (fsm *FileSegmentManager) CreateSegment(id int) (io.WriteCloser, error) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	path := filepath.Join(fsm.directory, fmt.Sprintf("%s%d", segmentPrefix, id))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("create segment %d: %w", id, err)
	}
	return file, nil
}

// OpenSegment opens an existing segment file for reading.
func (fsm *FileSegmentManager) OpenSegment(id int) (io.ReadCloser, error) {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	path := filepath.Join(fsm.directory, fmt.Sprintf("%s%d", segmentPrefix, id))
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open segment %d: %w", id, err)
	}
	return file, nil
}

// ListSegments returns all segment IDs in ascending order.
//
// Segments are discovered by globbing for files matching "segment-*" in the
// directory and extracting the numeric IDs.
func (fsm *FileSegmentManager) ListSegments() ([]int, error) {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	pattern := filepath.Join(fsm.directory, segmentPrefix+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("list segments: %w", err)
	}

	ids := make([]int, 0, len(matches))
	for _, match := range matches {
		var id int
		_, err := fmt.Sscanf(filepath.Base(match), segmentPrefix+"%d", &id)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	// Sort in ascending order
	for i := 0; i < len(ids)-1; i++ {
		for j := i + 1; j < len(ids); j++ {
			if ids[i] > ids[j] {
				ids[i], ids[j] = ids[j], ids[i]
			}
		}
	}

	return ids, nil
}

// DeleteSegment removes a segment file from the filesystem.
func (fsm *FileSegmentManager) DeleteSegment(id int) error {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	path := filepath.Join(fsm.directory, fmt.Sprintf("%s%d", segmentPrefix, id))
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete segment %d: %w", id, err)
	}
	return nil
}

// CurrentSegmentSize returns the current size in bytes of the segment file.
func (fsm *FileSegmentManager) CurrentSegmentSize(id int) (int64, error) {
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	path := filepath.Join(fsm.directory, fmt.Sprintf("%s%d", segmentPrefix, id))
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat segment %d: %w", id, err)
	}
	return info.Size(), nil
}
