package main

import (
	"fmt"
	"os"

	"github.com/godicom-dev/godicom"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: godicom <command> [args...]")
		fmt.Println("Commands:")
		fmt.Println("  read <file>          - Read and display DICOM file")
		fmt.Println("  readcopy <src> <dst> - Read then write DICOM file")
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "read":
		if len(os.Args) < 3 {
			fmt.Println("Usage: godicom read <file>")
			os.Exit(1)
		}
		force := false
		filename := os.Args[2]
		ds, err := godicom.ReadFile(filename, &godicom.ReadOptions{Force: force})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("File: %s\n", ds.Filename)
		fmt.Printf("Elements: %d\n", ds.Len())
		fmt.Println("---")
		for _, elem := range ds.Iter() {
			fmt.Println(elem)
		}

	case "readcopy":
		if len(os.Args) < 4 {
			fmt.Println("Usage: godicom readcopy <src> <dst>")
			os.Exit(1)
		}
		src := os.Args[2]
		dst := os.Args[3]

		ds, err := godicom.ReadFile(src, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Read %d elements from %s\n", ds.Len(), src)

		err = ds.SaveAs(dst, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Written to %s\n", dst)

		// Re-read and compare
		ds2, err := godicom.ReadFile(dst, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Re-read error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Re-read %d elements from %s\n", ds2.Len(), dst)
	}
}
