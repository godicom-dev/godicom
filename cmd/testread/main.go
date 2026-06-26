package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/godicom-dev/godicom"
)

func main() {
	testDir := `pydicom\src\pydicom\data\test_files`
	if len(os.Args) > 1 {
		testDir = os.Args[1]
	}

	entries, err := os.ReadDir(testDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading directory: %v\n", err)
		os.Exit(1)
	}

	passed := 0
	failed := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".dcm" {
			continue
		}
		path := filepath.Join(testDir, entry.Name())
		ds, err := godicom.ReadFile(path, &godicom.ReadOptions{Force: true})
		if err != nil {
			fmt.Printf("FAIL %s: %v\n", entry.Name(), err)
			failed++
		} else {
			fmt.Printf("OK   %s: %d elements\n", entry.Name(), ds.Len())
			passed++
		}
	}
	fmt.Printf("\n%d passed, %d failed\n", passed, failed)
}
