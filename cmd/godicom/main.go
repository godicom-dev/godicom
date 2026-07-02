package main

import (
	"fmt"
	"os"

	"github.com/godicom-dev/godicom"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "read", "show":
		runShow(os.Args[2:])
	case "readcopy":
		runReadCopy(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: godicom <command> [args...]")
	fmt.Println("Commands:")
	fmt.Println("  show <file>          - Display DICOM file (file meta + dataset)")
	fmt.Println("  read <file>          - Alias for show")
	fmt.Println("  readcopy <src> <dst> - Read then write DICOM file")
}

func runReadCopy(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: godicom readcopy <src> <dst>")
		os.Exit(1)
	}
	src := args[0]
	dst := args[1]

	ds, err := godicom.ReadFile(src, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Read %d elements from %s\n", ds.Len(), src)

	if err := ds.SaveAs(dst, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Written to %s\n", dst)

	ds2, err := godicom.ReadFile(dst, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Re-read error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Re-read %d elements from %s\n", ds2.Len(), dst)
	if ds.Len() != ds2.Len() {
		fmt.Fprintf(os.Stderr, "Warning: element count changed %d -> %d\n", ds.Len(), ds2.Len())
		os.Exit(1)
	}
}
