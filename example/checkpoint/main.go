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

	filename := "checkpoint_example.wal"

	// Write entries with a checkpoint
	fmt.Println("Writing entries with checkpoint...")
	file, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}

	writer := wal.NewBinaryEntryWriter(file)

	// Write some initial entries
	for i := uint64(1); i <= 3; i++ {
		entry := wal.NewEntry(i, []byte(fmt.Sprintf("Initial entry %d", i)))
		if err := writer.WriteEntry(entry); err != nil {
			log.Fatalf("Failed to write entry %d: %v", i, err)
		}
		fmt.Printf("  Written entry LSN=%d\n", i)
	}

	// Write a checkpoint entry
	checkpointLSN := uint64(4)
	checkpoint := wal.NewCheckpointEntry(checkpointLSN, []byte("Checkpoint entry"))
	if err := writer.WriteEntry(checkpoint); err != nil {
		log.Fatalf("Failed to write checkpoint: %v", err)
	}
	fmt.Printf("  ✓ Written CHECKPOINT at LSN=%d\n", checkpointLSN)

	// Write more entries after checkpoint
	for i := uint64(5); i <= 8; i++ {
		entry := wal.NewEntry(i, []byte(fmt.Sprintf("Post-checkpoint entry %d", i)))
		if err := writer.WriteEntry(entry); err != nil {
			log.Fatalf("Failed to write entry %d: %v", i, err)
		}
		fmt.Printf("  Written entry LSN=%d\n", i)
	}

	if err := writer.Sync(); err != nil {
		log.Fatalf("Failed to sync: %v", err)
	}
	file.Close()

	// Read entries with checkpoint awareness
	fmt.Println("\nReading entries with checkpoint awareness...")
	file, err = os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	entries, checkpointLSNRead, err := wal.ReadEntriesWithCheckpoint(file)
	if err != nil {
		log.Fatalf("Failed to read entries: %v", err)
	}

	if checkpointLSNRead > 0 {
		fmt.Printf("✓ Found checkpoint at LSN=%d\n", checkpointLSNRead)
		fmt.Printf("✓ Entries from checkpoint onwards: %d\n", len(entries))
	} else {
		fmt.Println("No checkpoint found, read all entries")
	}

	fmt.Println("\nEntries after checkpoint:")
	for _, entry := range entries {
		isCheckpoint := entry.IsCheckpoint != nil && *entry.IsCheckpoint
		checkpointMarker := ""
		if isCheckpoint {
			checkpointMarker = " [CHECKPOINT]"
		}
		fmt.Printf("  LSN=%d, Data=%s%s\n",
			entry.LogSequenceNumber,
			string(entry.Data),
			checkpointMarker)
	}

	// Clean up
	if !*keepFiles {
		os.Remove(filename)
		fmt.Println("\n✓ Checkpoint example completed successfully (files cleaned up)")
	} else {
		fmt.Printf("\n✓ Checkpoint example completed successfully (kept file: %s)\n", filename)
	}
}
