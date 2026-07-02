package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/godicom-dev/godicom"
)

type showOptions struct {
	noMeta      bool
	topLevel    bool
	tagKeywords []string
}

func runShow(args []string) {
	opts := showOptions{}
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	fs.BoolVar(&opts.noMeta, "no-meta", false, "skip file meta information")
	fs.BoolVar(&opts.topLevel, "top", false, "only show top-level elements")
	fs.Func("t", "show only elements with this tag (keyword or hex; repeatable)", func(s string) error {
		opts.tagKeywords = append(opts.tagKeywords, s)
		return nil
	})
	fs.Func("tag", "alias for -t", func(s string) error {
		opts.tagKeywords = append(opts.tagKeywords, s)
		return nil
	})
	fs.Parse(args)
	rest := fs.Args()
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: godicom show [-no-meta] [-top] [-t tag]... <file>")
		os.Exit(1)
	}

	ds, err := godicom.ReadFile(rest[0], nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	filterTags, err := parseShowTags(opts.tagKeywords)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := writeShow(os.Stdout, ds, opts, filterTags); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func parseShowTags(keywords []string) (map[godicom.Tag]struct{}, error) {
	if len(keywords) == 0 {
		return nil, nil
	}
	tags := make(map[godicom.Tag]struct{}, len(keywords))
	for _, keyword := range keywords {
		tag, err := godicom.ParseTag(keyword)
		if err != nil {
			return nil, err
		}
		tags[tag] = struct{}{}
	}
	return tags, nil
}

func writeShow(w io.Writer, ds *godicom.FileDataset, opts showOptions, filterTags map[godicom.Tag]struct{}) error {
	fmt.Fprintf(w, "File: %s\n", ds.Filename)

	if !opts.noMeta && ds.FileMeta != nil && ds.FileMeta.Len() > 0 {
		fmt.Fprintln(w, "--- File Meta ---")
		if len(filterTags) == 0 {
			for _, elem := range ds.FileMeta.Iter() {
				fmt.Fprintln(w, elem)
			}
		} else {
			printMatchingElements(w, ds.FileMeta.Dataset, filterTags, true)
		}
		if ts, ok := ds.FileMeta.GetString(godicom.MustTag("TransferSyntaxUID")); ok && len(filterTags) == 0 {
			fmt.Fprintf(w, "Transfer Syntax: %s\n", ts)
		}
		fmt.Fprintln(w, "--- Dataset ---")
	}

	if len(filterTags) == 0 {
		fmt.Fprintf(w, "Elements: %d\n", ds.Len())
		for _, elem := range ds.Iter() {
			fmt.Fprintln(w, elem)
		}
		return nil
	}

	count := printMatchingElements(w, ds.Dataset, filterTags, !opts.topLevel)
	fmt.Fprintf(w, "Matching elements: %d\n", count)
	return nil
}

func hasTag(tags map[godicom.Tag]struct{}, tag godicom.Tag) bool {
	_, ok := tags[tag]
	return ok
}

func printMatchingElements(w io.Writer, ds *godicom.Dataset, filterTags map[godicom.Tag]struct{}, recursive bool) int {
	count := 0
	visit := func(_ *godicom.Dataset, elem *godicom.Element) {
		if !hasTag(filterTags, elem.Tag) {
			return
		}
		fmt.Fprintln(w, elem)
		count++
	}
	if recursive {
		ds.Walk(visit, true)
		return count
	}
	for _, elem := range ds.Iter() {
		visit(ds, elem)
	}
	return count
}
