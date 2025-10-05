package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/wizenheimer/wal"
)

func main() {
	// Parse flags
	keepFiles := flag.Bool("keep", false, "Keep WAL files after execution (don't remove)")
	flag.Parse()

	filename := "streaming_example.wal"

	// Write a large number of entries
	fmt.Println("Writing entries to WAL...")
	file, err := os.Create(filename)
	if err != nil {
		log.Fatalf("Failed to create file: %v", err)
	}

	writer := wal.NewBinaryEntryWriter(file)

	const numEntries = 100
	for i := uint64(1); i <= numEntries; i++ {
		entry := wal.NewEntry(i, []byte(fmt.Sprintf("Entry %d with some data payload", i)))
		if err := writer.WriteEntry(entry); err != nil {
			log.Fatalf("Failed to write entry %d: %v", i, err)
		}

		// Periodically flush to demonstrate buffering
		if i%10 == 0 {
			if err := writer.Flush(); err != nil {
				log.Fatalf("Failed to flush: %v", err)
			}
			fmt.Printf("  Flushed after %d entries\n", i)
		}
	}

	if err := writer.Sync(); err != nil {
		log.Fatalf("Failed to sync: %v", err)
	}
	file.Close()
	fmt.Printf("✓ Wrote %d entries\n", numEntries)

	// Stream read entries one by one
	fmt.Println("\nStreaming entries from WAL...")
	file, err = os.Open(filename)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	reader := wal.NewBinaryEntryReader(file)

	count := 0
	var lastLSN uint64
	for {
		entry, err := reader.ReadEntry()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("Failed to read entry: %v", err)
		}

		// Verify CRC at application level
		if err := wal.VerifyEntry(entry); err != nil {
			log.Fatalf("CRC verification failed for entry: %v", err)
		}

		count++
		lastLSN = entry.LogSequenceNumber

		// Print progress every 25 entries
		if count%25 == 0 {
			fmt.Printf("  Read %d entries (current LSN=%d)\n", count, lastLSN)
		}
	}

	fmt.Printf("✓ Successfully read %d entries (last LSN=%d)\n", count, lastLSN)

	// Get file stats
	fileInfo, _ := os.Stat(filename)
	fmt.Printf("✓ WAL file size: %d bytes\n", fileInfo.Size())
	fmt.Printf("✓ Average entry size: %.2f bytes\n", float64(fileInfo.Size())/float64(count))

	// Clean up
	if !*keepFiles {
		os.Remove(filename)
		fmt.Println("\n✓ Streaming example completed successfully (files cleaned up)")
	} else {
		fmt.Printf("\n✓ Streaming example completed successfully (kept file: %s)\n", filename)
	}
}
