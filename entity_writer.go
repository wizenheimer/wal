package wal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

var defaultBufferSize = 1024 * 4 // 4KB

// EntryWriter writes WAL entries to an underlying writer
type EntryWriter interface {
	// WriteEntry writes a single entry
	WriteEntry(entry *WAL_Entry) error
	// Flush flushes any buffered data
	Flush() error
	// Sync ensures data is persisted (if supported)
	Sync() error
}

// syncer is a helper interface for types that support Sync
type syncer interface {
	Sync() error
}

// BinaryEntryWriter writes entries in binary format with length prefix
type BinaryEntryWriter struct {
	// w is the underlying writer
	w io.Writer
	// bw is the buffered writer
	bw *bufio.Writer
	// syncWriter is only used if the writer supports Sync
	syncWriter syncer
}

func NewBinaryEntryWriter(w io.Writer) *BinaryEntryWriter {
	bw := bufio.NewWriterSize(w, defaultBufferSize)
	syncWriter, _ := w.(syncer)
	return &BinaryEntryWriter{
		w:          w,
		bw:         bw,
		syncWriter: syncWriter,
	}
}

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

// Flush flushes any buffered data
func (bew *BinaryEntryWriter) Flush() error {
	if err := bew.bw.Flush(); err != nil {
		return fmt.Errorf("flush buffer: %w", err)
	}
	return nil
}

// Sync ensures data is persisted (if supported)
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

// BufferedBytes returns the number of bytes in the buffer
func (bew *BinaryEntryWriter) BufferedBytes() int {
	return bew.bw.Buffered()
}
