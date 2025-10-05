package wal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	sync "sync"
)

var segmentPrefix = "segment-"

// SegmentManager handles segment file operations
type SegmentManager interface {
	// CreateSegment creates a new segment with the given ID
	CreateSegment(id int) (io.WriteCloser, error)
	// OpenSegment opens an existing segment for reading
	OpenSegment(id int) (io.ReadCloser, error)
	// ListSegments returns all segment IDs in order
	ListSegments() ([]int, error)
	// DeleteSegment removes a segment
	DeleteSegment(id int) error
	// CurrentSegmentSize returns the size of the current segment
	CurrentSegmentSize(id int) (int64, error)
}

// FileSegmentManager implements SegmentManager for filesystem
type FileSegmentManager struct {
	// directory is the directory to store the segments
	directory string
	// mu is the mutex to protect the segment files
	mu sync.RWMutex
}

func NewFileSegmentManager(directory string) (*FileSegmentManager, error) {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}
	return &FileSegmentManager{directory: directory}, nil
}

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

func (fsm *FileSegmentManager) DeleteSegment(id int) error {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	path := filepath.Join(fsm.directory, fmt.Sprintf("%s%d", segmentPrefix, id))
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete segment %d: %w", id, err)
	}
	return nil
}

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
