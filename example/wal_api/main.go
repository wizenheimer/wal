package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/wizenheimer/wal"
)

func main() {
	// Parse flags
	keepFiles := flag.Bool("keep", false, "Keep WAL files after execution (don't remove)")
	flag.Parse()

	// Create directory for WAL segments
	walDir := "wal_api_example"

	fmt.Println("=== WAL API Example ===")
	fmt.Println("This example demonstrates the high-level WAL API")
	fmt.Println()

	// Example 1: Create and Open a WAL
	fmt.Println("1. Creating WAL with segment manager...")
	segmentMgr, err := wal.NewFileSegmentManager(walDir)
	if err != nil {
		log.Fatalf("Failed to create segment manager: %v", err)
	}

	// Configure WAL options
	opts := wal.DefaultWALOptions()
	opts.MaxSegmentSize = 1024 // Small size to demonstrate rotation
	opts.MaxSegments = 3       // Keep only 3 segments
	opts.SyncInterval = 2 * time.Second
	opts.EnableFsync = true

	fmt.Printf("  WAL Options:\n")
	fmt.Printf("    Max Segment Size: %d bytes\n", opts.MaxSegmentSize)
	fmt.Printf("    Max Segments: %d\n", opts.MaxSegments)
	fmt.Printf("    Sync Interval: %v\n", opts.SyncInterval)
	fmt.Printf("    Enable Fsync: %v\n", opts.EnableFsync)

	walInstance, err := wal.Open(segmentMgr, opts)
	if err != nil {
		log.Fatalf("Failed to open WAL: %v", err)
	}
	defer walInstance.Close()

	fmt.Println("  ✓ WAL opened successfully")

	// Example 2: Write regular entries
	fmt.Println("\n2. Writing regular entries...")
	for i := 1; i <= 10; i++ {
		data := []byte(fmt.Sprintf("Regular entry %d with some payload data", i))
		lsn, err := walInstance.WriteEntry(data)
		if err != nil {
			log.Fatalf("Failed to write entry %d: %v", i, err)
		}
		fmt.Printf("  Written entry %d, LSN=%d\n", i, lsn)

		// Add a small delay to demonstrate background sync
		if i == 5 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Example 3: Manual sync
	fmt.Println("\n3. Manually syncing WAL...")
	if err := walInstance.Sync(); err != nil {
		log.Fatalf("Failed to sync: %v", err)
	}
	fmt.Println("  ✓ WAL synced to disk")

	// Example 4: Write checkpoint
	fmt.Println("\n4. Writing checkpoint entry...")
	checkpointData := []byte("Application state snapshot at this point")
	checkpointLSN, err := walInstance.WriteCheckpoint(checkpointData)
	if err != nil {
		log.Fatalf("Failed to write checkpoint: %v", err)
	}
	fmt.Printf("  ✓ Checkpoint written at LSN=%d\n", checkpointLSN)

	// Example 5: Write more entries after checkpoint
	fmt.Println("\n5. Writing entries after checkpoint...")
	for i := 11; i <= 20; i++ {
		data := []byte(fmt.Sprintf("Post-checkpoint entry %d", i))
		lsn, err := walInstance.WriteEntry(data)
		if err != nil {
			log.Fatalf("Failed to write entry %d: %v", i, err)
		}
		if i%5 == 0 {
			fmt.Printf("  Written entry %d, LSN=%d\n", i, lsn)
		}
	}

	// Example 6: Write many entries to trigger segment rotation
	fmt.Println("\n6. Writing many entries to trigger segment rotation...")
	additionalEntries := 30
	for i := 1; i <= additionalEntries; i++ {
		data := []byte(fmt.Sprintf("Rotation test entry %d with padding to increase size XXXXXXXXXXXXXXXXXXXX", i))
		lsn, err := walInstance.WriteEntry(data)
		if err != nil {
			log.Fatalf("Failed to write entry: %v", err)
		}
		if i%10 == 0 {
			fmt.Printf("  Written %d additional entries (last LSN=%d)\n", i, lsn)
		}
	}
	fmt.Printf("  ✓ Wrote %d additional entries to trigger rotation\n", additionalEntries)

	// Example 7: Read all entries
	fmt.Println("\n7. Reading all entries from WAL...")
	entries, err := walInstance.ReadAll()
	if err != nil {
		log.Fatalf("Failed to read all entries: %v", err)
	}
	fmt.Printf("  Total entries in WAL: %d\n", len(entries))
	fmt.Println("  First 5 entries:")
	for i := 0; i < 5 && i < len(entries); i++ {
		entry := entries[i]
		isCheckpoint := entry.IsCheckpoint != nil && *entry.IsCheckpoint
		checkpointMarker := ""
		if isCheckpoint {
			checkpointMarker = " [CHECKPOINT]"
		}
		dataPreview := string(entry.Data)
		if len(dataPreview) > 40 {
			dataPreview = dataPreview[:40] + "..."
		}
		fmt.Printf("    LSN=%d, Data=%s%s\n", entry.LogSequenceNumber, dataPreview, checkpointMarker)
	}

	// Example 8: Read from last checkpoint
	fmt.Println("\n8. Reading entries from last checkpoint...")
	checkpointEntries, err := walInstance.ReadFromCheckpoint()
	if err != nil {
		log.Fatalf("Failed to read from checkpoint: %v", err)
	}
	fmt.Printf("  Entries from checkpoint onwards: %d\n", len(checkpointEntries))
	fmt.Println("  Showing entries around checkpoint:")

	// Find checkpoint and show context
	checkpointIdx := -1
	for i, entry := range checkpointEntries {
		if entry.IsCheckpoint != nil && *entry.IsCheckpoint {
			checkpointIdx = i
			break
		}
	}

	if checkpointIdx >= 0 {
		start := checkpointIdx
		if start > 2 {
			start = checkpointIdx - 2
		}
		end := checkpointIdx + 3
		if end > len(checkpointEntries) {
			end = len(checkpointEntries)
		}

		for i := start; i < end; i++ {
			entry := checkpointEntries[i]
			isCheckpoint := entry.IsCheckpoint != nil && *entry.IsCheckpoint
			marker := "  "
			if isCheckpoint {
				marker = "→ [CHECKPOINT]"
			}
			dataPreview := string(entry.Data)
			if len(dataPreview) > 35 {
				dataPreview = dataPreview[:35] + "..."
			}
			fmt.Printf("    %s LSN=%d, Data=%s\n", marker, entry.LogSequenceNumber, dataPreview)
		}
	}

	// Example 9: Check segment information
	fmt.Println("\n9. Checking segment information...")
	segments, err := segmentMgr.ListSegments()
	if err != nil {
		log.Fatalf("Failed to list segments: %v", err)
	}
	fmt.Printf("  Active segments: %v\n", segments)
	fmt.Println("  Segment sizes:")
	totalSize := int64(0)
	for _, segID := range segments {
		size, err := segmentMgr.CurrentSegmentSize(segID)
		if err != nil {
			log.Printf("  Warning: couldn't get size for segment %d: %v", segID, err)
			continue
		}
		fmt.Printf("    Segment %d: %d bytes\n", segID, size)
		totalSize += size
	}
	fmt.Printf("  Total WAL size: %d bytes\n", totalSize)

	// Example 10: Recovery simulation
	fmt.Println("\n10. Simulating recovery process...")
	fmt.Println("  Closing current WAL...")
	if err := walInstance.Close(); err != nil {
		log.Fatalf("Failed to close WAL: %v", err)
	}

	fmt.Println("  Reopening WAL (simulating crash recovery)...")
	recoveredWAL, err := wal.Open(segmentMgr, opts)
	if err != nil {
		log.Fatalf("Failed to reopen WAL: %v", err)
	}
	defer recoveredWAL.Close()

	// Read and verify
	recoveredEntries, err := recoveredWAL.ReadFromCheckpoint()
	if err != nil {
		log.Fatalf("Failed to read from recovered WAL: %v", err)
	}
	fmt.Printf("  ✓ Recovery successful!\n")
	fmt.Printf("  ✓ Recovered %d entries from checkpoint\n", len(recoveredEntries))

	// Write new entry to verify WAL is functional
	newData := []byte("Entry after recovery")
	newLSN, err := recoveredWAL.WriteEntry(newData)
	if err != nil {
		log.Fatalf("Failed to write after recovery: %v", err)
	}
	fmt.Printf("  ✓ Successfully wrote new entry after recovery (LSN=%d)\n", newLSN)

	// Final summary
	fmt.Println("\n=== Summary ===")
	allEntriesAfterRecovery, _ := recoveredWAL.ReadAll()
	fmt.Printf("Total entries in WAL: %d\n", len(allEntriesAfterRecovery))
	fmt.Printf("Active segments: %d\n", len(segments))
	fmt.Printf("Total storage used: %d bytes\n", totalSize)
	fmt.Println("\nKey features demonstrated:")
	fmt.Println("  ✓ High-level WAL API")
	fmt.Println("  ✓ Automatic segment rotation")
	fmt.Println("  ✓ Background syncing")
	fmt.Println("  ✓ Checkpoint creation and recovery")
	fmt.Println("  ✓ Segment management (max segments limit)")
	fmt.Println("  ✓ Crash recovery simulation")

	// Clean up
	recoveredWAL.Close()
	if !*keepFiles {
		os.RemoveAll(walDir)
		fmt.Println("\n✓ Example completed successfully (files cleaned up)")
	} else {
		fmt.Printf("\n✓ Example completed successfully (kept directory: %s)\n", walDir)
	}
}
