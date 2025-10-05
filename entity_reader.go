package wal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// EntryReader reads WAL entries from an underlying reader.
//
// EntryReader implementations handle the deserialization of WAL entries
// from their binary format.
type EntryReader interface {
	// ReadEntry reads the next entry, returns io.EOF when done.
	ReadEntry() (*WAL_Entry, error)
}

// BinaryEntryReader reads entries in binary format with a length prefix.
//
// The binary format consists of:
//   - 4 bytes: uint32 length of the protobuf-encoded entry (little-endian)
//   - N bytes: protobuf-encoded WAL_Entry
//
// BinaryEntryReader uses buffering for efficient reading of sequential entries.
type BinaryEntryReader struct {
	// r is the underlying reader
	r io.Reader
	// br is the buffered reader
	br *bufio.Reader
}

// NewBinaryEntryReader creates a new BinaryEntryReader that reads from r.
//
// The reader is buffered with a 4KB buffer for optimal performance.
func NewBinaryEntryReader(r io.Reader) *BinaryEntryReader {
	return &BinaryEntryReader{
		r:  r,
		br: bufio.NewReaderSize(r, defaultBufferSize),
	}
}

// ReadEntry reads the next WAL entry from the reader.
//
// ReadEntry first reads a 4-byte length prefix, then reads that many bytes
// and unmarshals them as a protobuf-encoded WAL_Entry.
//
// Returns io.EOF when no more entries are available.
func (ber *BinaryEntryReader) ReadEntry() (*WAL_Entry, error) {
	// Read length prefix
	var size uint32
	if err := binary.Read(ber.br, binary.LittleEndian, &size); err != nil {
		return nil, err // Will be io.EOF at end of file
	}

	// Read entry data
	data := make([]byte, size)
	if _, err := io.ReadFull(ber.br, data); err != nil {
		return nil, fmt.Errorf("read entry data: %w", err)
	}

	// Unmarshal entry
	var entry WAL_Entry
	MustUnmarshal(data, &entry)

	return &entry, nil
}
