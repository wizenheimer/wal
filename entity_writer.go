package wal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

var defaultBufferSize = 1024 * 4 // 4KB

// EntryWriter writes WAL entries to an underlying writer.
//
// EntryWriter implementations handle the serialization and buffering of
// WAL entries, providing methods to flush and sync data to ensure durability.
type EntryWriter interface {
	// WriteEntry writes a single entry to the underlying writer.
	WriteEntry(entry *WAL_Entry) error
	// Flush flushes any buffered data to the underlying writer.
	Flush() error
	// Sync ensures data is persisted to disk (if the underlying writer supports it).
	Sync() error
}

// syncer is a helper interface for types that support Sync
type syncer interface {
	Sync() error
}

// BinaryEntryWriter writes entries in binary format with a length prefix.
//
// The binary format consists of:
//   - 4 bytes: uint32 length of the protobuf-encoded entry (little-endian)
//   - N bytes: protobuf-encoded WAL_Entry
//
// BinaryEntryWriter uses buffering for performance and supports syncing to
// ensure durability when the underlying writer implements the Sync() method.
type BinaryEntryWriter struct {
	// w is the underlying writer
	w io.Writer
	// bw is the buffered writer
	bw *bufio.Writer
	// syncWriter is only used if the writer supports Sync
	syncWriter syncer
}

// NewBinaryEntryWriter creates a new BinaryEntryWriter that writes to w.
//
// The writer is buffered with a 4KB buffer for optimal performance.
// If w supports Sync() (such as *os.File), syncing will be available.
func NewBinaryEntryWriter(w io.Writer) *BinaryEntryWriter {
	bw := bufio.NewWriterSize(w, defaultBufferSize)
	syncWriter, _ := w.(syncer)
	return &BinaryEntryWriter{
		w:          w,
		bw:         bw,
		syncWriter: syncWriter,
	}
}

// WriteEntry writes a WAL entry in binary format.
//
// The entry is marshaled to protobuf, prefixed with its length as a 4-byte
// little-endian uint32, and written to the buffered writer.
func (bew *BinaryEntryWriter) WriteEntry(entry *WAL_Entry) error {
	data := MustMarshal(entry)

	// Write length prefix
	size := uint32(len(data))
	if err := binary.Write(bew.bw, binary.LittleEndian, size); err != nil {
		return fmt.Errorf("failed to write size: %w", err)
	}

	// Write entry data
	if _, err := bew.bw.Write(data); err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}

	return nil
}

// Flush flushes any buffered data to the underlying writer.
//
// This writes all buffered data but does not guarantee persistence to disk.
// Use Sync for durability guarantees.
func (bew *BinaryEntryWriter) Flush() error {
	if err := bew.bw.Flush(); err != nil {
		return fmt.Errorf("flush buffer: %w", err)
	}
	return nil
}

// Sync flushes buffered data and syncs to disk if the underlying writer supports it.
//
// For writers like *os.File, Sync calls fsync to ensure data is persisted.
// If the underlying writer does not support Sync, only the flush is performed.
func (bew *BinaryEntryWriter) Sync() error {
	if err := bew.Flush(); err != nil {
		return err
	}

	if bew.syncWriter != nil {
		if err := bew.syncWriter.Sync(); err != nil {
			return fmt.Errorf("sync: %w", err)
		}
	}
	return nil
}

// BufferedBytes returns the number of bytes currently buffered but not yet flushed.
//
// This is useful for determining when to rotate segments based on buffered data size.
func (bew *BinaryEntryWriter) BufferedBytes() int {
	return bew.bw.Buffered()
}
