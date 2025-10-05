package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/wizenheimer/wal"
)

func main() {
	// Parse flags
	keepFiles := flag.Bool("keep", false, "Keep WAL files after execution (don't remove)")
	flag.Parse()

	filename := "error_handling.wal"

	// Example 1: Corrupted CRC detection
	fmt.Println("Example 1: Demonstrating CRC validation")
	fmt.Println("========================================")

	file, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}

	writer := wal.NewBinaryEntryWriter(file)

	// Write a valid entry
	validEntry := wal.NewEntry(1, []byte("Valid entry data"))
	if err := writer.WriteEntry(validEntry); err != nil {
		log.Fatalf("Failed to write valid entry: %v", err)
	}
	fmt.Println("✓ Written valid entry")

	// Write an entry with corrupted CRC (manually constructed to demonstrate error handling)
	corruptedEntry := &wal.WAL_Entry{
		LogSequenceNumber: 2,
		Data:              []byte("Corrupted entry data"),
		CRC:               0xDEADBEEF, // Wrong CRC
	}
	if err := writer.WriteEntry(corruptedEntry); err != nil {
		log.Fatalf("Failed to write corrupted entry: %v", err)
	}
	fmt.Println("✓ Written entry with incorrect CRC")

	// Write another valid entry
	validEntry2 := wal.NewEntry(3, []byte("Another valid entry"))
	if err := writer.WriteEntry(validEntry2); err != nil {
		log.Fatalf("Failed to write valid entry: %v", err)
	}
	fmt.Println("✓ Written another valid entry")

	if err := writer.Sync(); err != nil {
		log.Fatalf("Failed to sync: %v", err)
	}
	file.Close()

	// Try to read entries - should fail on corrupted entry
	fmt.Println("\nReading entries (will encounter CRC error)...")
	file, err = os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	reader := wal.NewBinaryEntryReader(file)

	for i := 1; ; i++ {
		entry, err := reader.ReadEntry()
		if err != nil {
			fmt.Printf("✗ Error reading entry %d: %v\n", i, err)
			break
		}

		// Verify CRC at application level
		if err := wal.VerifyEntry(entry); err != nil {
			fmt.Printf("✗ CRC verification failed for entry %d: %v\n", i, err)
			break
		}

		fmt.Printf("✓ Successfully read entry LSN=%d, Data=%s\n",
			entry.LogSequenceNumber,
			string(entry.Data))
	}

	// Example 2: Empty file handling
	fmt.Println("\n\nExample 2: Reading from empty file")
	fmt.Println("=====================================")

	emptyFile := "empty.wal"
	f, _ := os.Create(emptyFile)
	f.Close()

	f, _ = os.Open(emptyFile)
	defer f.Close()

	entries, err := wal.ReadAllEntries(f)
	if err != nil {
		fmt.Printf("✗ Error reading empty file: %v\n", err)
	} else {
		fmt.Printf("✓ Successfully handled empty file (read %d entries)\n", len(entries))
	}

	// Clean up
	if !*keepFiles {
		os.Remove(filename)
		os.Remove(emptyFile)
		fmt.Println("\n✓ Error handling examples completed (files cleaned up)")
	} else {
		fmt.Printf("\n✓ Error handling examples completed (kept files: %s, %s)\n", filename, emptyFile)
	}
}
