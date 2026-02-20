package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hw2/internal/lsm"
	"hw2/internal/query"
	"hw2/internal/text"
)

func main() {
	docsDir := flag.String("docs", "docs", "directory with .txt documents to index")
	dataDir := flag.String("data", "data", "directory for on-disk index storage")
	lang    := flag.String("lang", "english", "stemming language: english | russian")
	flush   := flag.Int("flush", 200, "flush MemTable after this many (term,docID) pairs")
	reindex := flag.Bool("reindex", false, "delete existing index and rebuild from scratch")
	flag.Parse()

	if *reindex {
		os.RemoveAll(*dataDir)
		fmt.Println("Existing index removed.")
	}

	proc := text.NewProcessor(*lang)

	idx, err := lsm.New(*dataDir, proc, *flush)
	if err != nil {
		fatalf("create index: %v", err)
	}
	defer idx.Close()

	// индексируем только при первом запуске (пустой universe) или после -reindex
	if idx.Universe().GetCardinality() == 0 {
		fmt.Printf("Indexing documents from %q ...\n", *docsDir)
		if err := indexDirectory(idx, *docsDir); err != nil {
			fatalf("indexing: %v", err)
		}
		if err := idx.Flush(); err != nil {
			fatalf("flush: %v", err)
		}
		fmt.Println()
	} else {
		fmt.Printf("Loaded existing index (%d documents).\n\n",
			idx.Universe().GetCardinality())
	}

	printStats(idx)
	runREPL(idx)
}

func indexDirectory(idx *lsm.LSM, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".txt") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		docID, err := idx.AddDocument(path, string(content))
		if err != nil {
			return fmt.Errorf("index %s: %w", path, err)
		}
		fmt.Printf("  [%3d] %s\n", docID, path)
		return nil
	})
}

const help = `Boolean query REPL
  Operators : AND  OR  NOT  ( )
  Implicit AND: "fox hound" is the same as "fox AND hound"
  Terms are stemmed and stop-words are ignored automatically.

Commands:
  stats    — show index statistics
  compact  — force LSM compaction
  help     — show this message
  quit     — exit
`

func runREPL(idx *lsm.LSM) {
	fmt.Print(help)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		switch strings.ToLower(line) {
		case "quit", "exit", "q":
			return
		case "stats":
			printStats(idx)
			continue
		case "compact":
			if err := idx.Compact(); err != nil {
				fmt.Printf("compact error: %v\n", err)
			} else {
				fmt.Println("Compaction done.")
				printStats(idx)
			}
			continue
		case "help":
			fmt.Print(help)
			continue
		}

		results, err := query.Evaluate(line, idx)
		if err != nil {
			fmt.Printf("Parse error: %v\n", err)
			continue
		}

		ids := results.ToArray()

		if len(ids) == 0 {
			fmt.Println("No documents found.")
			continue
		}

		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		fmt.Printf("Found %d document(s)  [query: %s]\n", len(ids), line)
		for _, id := range ids {
			path := idx.DocPaths[id]
			snippet := readSnippet(path, 80)
			fmt.Printf("  [%3d] %-35s  %s\n", id, filepath.Base(path), snippet)
		}
		fmt.Println()
	}
}

func printStats(idx *lsm.LSM) {
	stats := idx.Stats()
	fmt.Println("─── Index stats ───────────────────────")
	keys := []string{
		"documents", "memtable_terms",
		"l0_segments", "l1_exists", "flush_threshold",
	}
	for _, k := range keys {
		fmt.Printf("  %-20s %v\n", k, stats[k])
	}
	fmt.Println("───────────────────────────────────────")
	fmt.Println()
}

func readSnippet(path string, maxLen int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.Join(strings.Fields(string(data)), " ")
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}
