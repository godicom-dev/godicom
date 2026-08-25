package main

import (
	"fmt"
	"io"
	"os"

	"github.com/godicom-dev/godicom"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stdout)
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
		printUsage(os.Stdout)
		os.Exit(1)
	}
}

// printUsage writes the command and flag summary. Every flag listed under "Flags
// for show and read" has to exist in newShowFlagSet, and every flag registered
// there has to be listed here; TestPrintUsageMatchesShowFlags checks both ways.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: godicom <command> [flags] [args...]")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  show [flags] <file>  - Display DICOM file (file meta + dataset)")
	fmt.Fprintln(w, "  read [flags] <file>  - Alias for show")
	fmt.Fprintln(w, "  readcopy <src> <dst> - Read then write DICOM file")
	fmt.Fprintln(w, "Flags for show and read:")
	fmt.Fprintln(w, "  -no-meta             - Skip file meta information")
	fmt.Fprintln(w, "  -top                 - Restrict -t to the top level, skipping sequences")
	fmt.Fprintln(w, "  -debug               - Emit reader debug logs to stderr")
	fmt.Fprintln(w, "  -t <tag>             - Show only this tag; keyword or hex (repeatable)")
	fmt.Fprintln(w, "  -tag <tag>           - Alias for -t")
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
