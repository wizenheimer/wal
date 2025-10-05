package wal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// EntryReader reads WAL entries from an underlying reader
type EntryReader interface {
	// ReadEntry reads the next entry, returns io.EOF when done
	ReadEntry() (*WAL_Entry, error)
}

// BinaryEntryReader reads entries in binary format with length prefix
type BinaryEntryReader struct {
	// r is the underlying reader
	r io.Reader
	// br is the buffered reader
	br *bufio.Reader
}

func NewBinaryEntryReader(r io.Reader) *BinaryEntryReader {
	return &BinaryEntryReader{
		r:  r,
		br: bufio.NewReaderSize(r, defaultBufferSize),
	}
}

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
