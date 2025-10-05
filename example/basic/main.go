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

	// Create a WAL file
	filename := "example.wal"
	file, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}

	// Create a writer
	writer := wal.NewBinaryEntryWriter(file)

	// Write some entries
	fmt.Println("Writing entries to WAL...")
	for i := uint64(1); i <= 5; i++ {
		entry := wal.NewEntry(i, []byte(fmt.Sprintf("Entry %d data", i)))

		if err := writer.WriteEntry(entry); err != nil {
			log.Fatalf("Failed to write entry %d: %v", i, err)
		}
		fmt.Printf("  Written entry LSN=%d\n", i)
	}

	// Flush and sync the data
	if err := writer.Sync(); err != nil {
		log.Fatalf("Failed to sync: %v", err)
	}

	file.Close()
	fmt.Println("✓ All entries written successfully")

	// Now read the entries back
	fmt.Println("\nReading entries from WAL...")
	file, err = os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	entries, err := wal.ReadAllEntries(file)
	if err != nil {
		log.Fatalf("Failed to read entries: %v", err)
	}

	fmt.Printf("Read %d entries:\n", len(entries))
	for _, entry := range entries {
		fmt.Printf("  LSN=%d, Data=%s, CRC=%d\n",
			entry.LogSequenceNumber,
			string(entry.Data),
			entry.CRC)
	}

	// Clean up
	if !*keepFiles {
		os.Remove(filename)
		fmt.Println("\n✓ Example completed successfully (files cleaned up)")
	} else {
		fmt.Printf("\n✓ Example completed successfully (kept file: %s)\n", filename)
	}
}
