package wal

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// calculateCRC calculates the CRC32 of the data and the log sequence number
func calculateCRC(data []byte, lsn uint64) uint32 {
	h := crc32.NewIEEE()
	h.Write(data)
	binary.Write(h, binary.LittleEndian, lsn)
	return h.Sum32()
}

// NewEntry creates a new WAL entry with the given LSN and data
// It automatically calculates and sets the CRC
func NewEntry(lsn uint64, data []byte) *WAL_Entry {
	entry := &WAL_Entry{
		LogSequenceNumber: lsn,
		Data:              data,
	}
	entry.CRC = calculateCRC(data, lsn)
	return entry
}

// NewCheckpointEntry creates a new checkpoint WAL entry with the given LSN and data
// It automatically calculates and sets the CRC and marks it as a checkpoint
func NewCheckpointEntry(lsn uint64, data []byte) *WAL_Entry {
	entry := &WAL_Entry{
		LogSequenceNumber: lsn,
		Data:              data,
	}
	checkpoint := true
	entry.IsCheckpoint = &checkpoint
	entry.CRC = calculateCRC(data, lsn)
	return entry
}

// VerifyEntry verifies the CRC32 of an entry
// It returns an error if the CRC32 mismatch
func VerifyEntry(entry *WAL_Entry) error {
	expectedCRC := calculateCRC(entry.Data, entry.LogSequenceNumber)
	if entry.CRC != expectedCRC {
		return fmt.Errorf("CRC mismatch: expected %d, got %d", expectedCRC, entry.CRC)
	}
	return nil
}

// ReadAllEntries reads all the entries from the reader and verifies their CRC
// It returns the entries and an error if the reading or verification fails
func ReadAllEntries(r io.Reader) ([]*WAL_Entry, error) {
	reader := NewBinaryEntryReader(r)
	var entries []*WAL_Entry

	for {
		entry, err := reader.ReadEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			return entries, err
		}

		// Verify CRC at application level, not transport level
		if err := VerifyEntry(entry); err != nil {
			return entries, err
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// ReadEntriesWithCheckpoint reads all the entries from the reader and verifies their CRC
// It returns the entries and the checkpoint LSN and an error if the reading or verification fails
func ReadEntriesWithCheckpoint(r io.Reader) ([]*WAL_Entry, uint64, error) {
	reader := NewBinaryEntryReader(r)
	var entries []*WAL_Entry
	var checkpointLSN uint64

	for {
		entry, err := reader.ReadEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			return entries, checkpointLSN, err
		}

		// Verify CRC at application level, not transport level
		if err := VerifyEntry(entry); err != nil {
			return entries, checkpointLSN, err
		}

		if entry.IsCheckpoint != nil && *entry.IsCheckpoint {
			// Reset entries from checkpoint
			entries = []*WAL_Entry{entry}
			checkpointLSN = entry.LogSequenceNumber
		} else {
			entries = append(entries, entry)
		}
	}

	return entries, checkpointLSN, nil
}
