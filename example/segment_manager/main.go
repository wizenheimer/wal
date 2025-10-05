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
	keepFiles := flag.Bool("keep", false, "Keep segment files after execution (don't remove)")
	flag.Parse()

	// Create a temporary directory for segments
	segmentDir := "example_segments"

	// Create a segment manager
	fmt.Println("=== Segment Manager Example ===")
	fmt.Printf("Creating segment manager in directory: %s\n", segmentDir)

	manager, err := wal.NewFileSegmentManager(segmentDir)
	if err != nil {
		log.Fatalf("Failed to create segment manager: %v", err)
	}

	// Example 1: Create and write to multiple segments
	fmt.Println("\n1. Creating and writing to multiple segments...")
	for segmentID := 1; segmentID <= 3; segmentID++ {
		segment, err := manager.CreateSegment(segmentID)
		if err != nil {
			log.Fatalf("Failed to create segment %d: %v", segmentID, err)
		}

		// Write entries to the segment
		writer := wal.NewBinaryEntryWriter(segment)
		for i := 0; i < 3; i++ {
			lsn := uint64((segmentID-1)*3 + i + 1)
			entry := wal.NewEntry(lsn, []byte(fmt.Sprintf("Segment %d, Entry %d", segmentID, i+1)))
			if err := writer.WriteEntry(entry); err != nil {
				log.Fatalf("Failed to write entry to segment %d: %v", segmentID, err)
			}
		}

		if err := writer.Sync(); err != nil {
			log.Fatalf("Failed to sync segment %d: %v", segmentID, err)
		}

		segment.Close()
		fmt.Printf("  ✓ Created segment %d with 3 entries\n", segmentID)
	}

	// Example 2: List all segments
	fmt.Println("\n2. Listing all segments...")
	segments, err := manager.ListSegments()
	if err != nil {
		log.Fatalf("Failed to list segments: %v", err)
	}
	fmt.Printf("  Found %d segments: %v\n", len(segments), segments)

	// Example 3: Read from segments
	fmt.Println("\n3. Reading entries from all segments...")
	totalEntries := 0
	for _, segmentID := range segments {
		segment, err := manager.OpenSegment(segmentID)
		if err != nil {
			log.Fatalf("Failed to open segment %d: %v", segmentID, err)
		}

		entries, err := wal.ReadAllEntries(segment)
		if err != nil {
			log.Fatalf("Failed to read from segment %d: %v", segmentID, err)
		}

		fmt.Printf("  Segment %d (%d entries):\n", segmentID, len(entries))
		for _, entry := range entries {
			fmt.Printf("    LSN=%d, Data=%s\n", entry.LogSequenceNumber, string(entry.Data))
			totalEntries++
		}

		segment.Close()
	}
	fmt.Printf("  Total entries read: %d\n", totalEntries)

	// Example 4: Check segment sizes
	fmt.Println("\n4. Checking segment sizes...")
	for _, segmentID := range segments {
		size, err := manager.CurrentSegmentSize(segmentID)
		if err != nil {
			log.Fatalf("Failed to get size of segment %d: %v", segmentID, err)
		}
		fmt.Printf("  Segment %d: %d bytes\n", segmentID, size)
	}

	// Example 5: Append to an existing segment
	fmt.Println("\n5. Appending to existing segment 2...")
	segment2, err := manager.CreateSegment(2)
	if err != nil {
		log.Fatalf("Failed to open segment 2: %v", err)
	}

	writer := wal.NewBinaryEntryWriter(segment2)
	newEntry := wal.NewEntry(100, []byte("Appended entry"))
	if err := writer.WriteEntry(newEntry); err != nil {
		log.Fatalf("Failed to append entry: %v", err)
	}
	if err := writer.Sync(); err != nil {
		log.Fatalf("Failed to sync: %v", err)
	}
	segment2.Close()

	// Verify the append
	size, err := manager.CurrentSegmentSize(2)
	if err != nil {
		log.Fatalf("Failed to get size: %v", err)
	}
	fmt.Printf("  ✓ Segment 2 new size: %d bytes\n", size)

	// Example 6: Simulate segment rotation (delete old segments)
	fmt.Println("\n6. Simulating segment rotation (keeping only last 2 segments)...")
	segments, _ = manager.ListSegments()
	if len(segments) > 2 {
		for i := 0; i < len(segments)-2; i++ {
			segmentID := segments[i]
			if err := manager.DeleteSegment(segmentID); err != nil {
				log.Fatalf("Failed to delete segment %d: %v", segmentID, err)
			}
			fmt.Printf("  ✓ Deleted old segment %d\n", segmentID)
		}
	}

	// List remaining segments
	segments, _ = manager.ListSegments()
	fmt.Printf("  Remaining segments: %v\n", segments)

	// Example 7: Read across multiple segments sequentially
	fmt.Println("\n7. Sequential read across all remaining segments...")
	allEntries := make([]*wal.WAL_Entry, 0)
	for _, segmentID := range segments {
		segment, err := manager.OpenSegment(segmentID)
		if err != nil {
			log.Fatalf("Failed to open segment %d: %v", segmentID, err)
		}

		reader := wal.NewBinaryEntryReader(segment)
		for {
			entry, err := reader.ReadEntry()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("Failed to read entry: %v", err)
			}
			allEntries = append(allEntries, entry)
		}
		segment.Close()
	}

	fmt.Printf("  Read %d total entries across all segments:\n", len(allEntries))
	for _, entry := range allEntries {
		fmt.Printf("    LSN=%d, Data=%s\n", entry.LogSequenceNumber, string(entry.Data))
	}

	// Clean up
	if !*keepFiles {
		os.RemoveAll(segmentDir)
		fmt.Println("\n✓ Example completed successfully (files cleaned up)")
	} else {
		fmt.Printf("\n✓ Example completed successfully (kept directory: %s)\n", segmentDir)
	}
}
