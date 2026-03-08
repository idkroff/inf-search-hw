package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hw2/internal/kgram"
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
	nostem  := flag.Bool("nostem", false, "disable stemming/stop-words (нужно для prefix/wildcard поиска)")
	flag.Parse()

	if *reindex {
		os.RemoveAll(*dataDir)
		fmt.Println("Existing index removed.")
	}

	proc := text.NewProcessor(*lang)
	proc.NoStem = *nostem

	idx, err := lsm.New(*dataDir, proc, *flush)
	if err != nil {
		fatalf("create index: %v", err)
	}
	defer idx.Close()

	kg := kgram.New(kgram.DefaultK)

	if idx.Universe().GetCardinality() == 0 {
		fmt.Printf("Indexing documents from %q ...\n", *docsDir)
		if err := indexDirectory(idx, kg, proc, *docsDir); err != nil {
			fatalf("indexing: %v", err)
		}
		if err := idx.Flush(); err != nil {
			fatalf("flush: %v", err)
		}
		fmt.Println()
	} else {
		fmt.Printf("Loaded existing index (%d documents).\n", idx.Universe().GetCardinality())
		fmt.Println("Rebuilding k-gram index in memory...")
		if terms, err := idx.AllTerms(); err != nil {
			fatalf("rebuild kgram: %v", err)
		} else {
			for _, t := range terms {
				kg.AddTerm(t)
			}
		}
		fmt.Println()
	}

	printStats(idx)
	runREPL(idx, kg)
}

func indexDirectory(idx *lsm.LSM, kg *kgram.Index, proc *text.Processor, dir string) error {
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
		for _, term := range proc.Process(string(content)) {
			kg.AddTerm(term)
		}
		fmt.Printf("  [%3d] %s\n", docID, path)
		return nil
	})
}

const help = `Boolean + Prefix + Wildcard query REPL
  Operators : AND  OR  NOT  ( )
  Implicit AND: "fox hound" is the same as "fox AND hound"
  Prefix search:   "comput*"   — все термины с данным префиксом
  Wildcard search: "c*t"       — k-gram поиск по паттерну (рекомендуется -nostem)
  Terms are stemmed unless -nostem is set.

Commands:
  stats    — show index statistics
  compact  — force LSM compaction
  help     — show this message
  quit     — exit
`

func runREPL(idx *lsm.LSM, kg *kgram.Index) {
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

		results, err := query.Evaluate(line, idx, kg)
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
