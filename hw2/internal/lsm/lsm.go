package lsm

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/RoaringBitmap/roaring"

	"hw2/internal/text"
)

const maxL0Tables = 4

type LSM struct {
	dataDir  string
	memTable *MemTable
	l0       []*SSTable      // сегменты уровня 0
	l1       *SSTable        // скомпакченный сегмент уровня 1
	universe *roaring.Bitmap // ID всех проиндексированных документов (нужен для NOT)
	mu       sync.RWMutex

	flushThreshold int
	nextSSTableID  int
	processor      *text.Processor

	DocPaths  map[uint32]string
	nextDocID uint32
}

func New(dataDir string, processor *text.Processor, flushThreshold int) (*LSM, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dataDir, err)
	}

	l := &LSM{
		dataDir:        dataDir,
		memTable:       newMemTable(),
		universe:       roaring.New(),
		flushThreshold: flushThreshold,
		processor:      processor,
		DocPaths:       make(map[uint32]string),
	}

	if err := l.loadExisting(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *LSM) AddDocument(path, content string) (uint32, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	docID := l.nextDocID
	l.nextDocID++
	l.DocPaths[docID] = path
	l.universe.Add(docID)

	for _, term := range l.processor.Process(content) {
		l.memTable.Add(term, docID)
	}

	if l.memTable.Size() >= l.flushThreshold {
		if err := l.flush(); err != nil {
			return docID, fmt.Errorf("auto-flush: %w", err)
		}
	}
	return docID, nil
}

func (l *LSM) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.flush()
}

func (l *LSM) flush() error {
	if l.memTable.Size() == 0 {
		return l.saveMeta()
	}

	path := filepath.Join(l.dataDir, fmt.Sprintf("l0_%04d.sst", l.nextSSTableID))
	l.nextSSTableID++

	if err := WriteSSTable(path, l.memTable.ToMap()); err != nil {
		return fmt.Errorf("write L0 SSTable: %w", err)
	}
	sst, err := OpenSSTable(path)
	if err != nil {
		return fmt.Errorf("open new L0 SSTable: %w", err)
	}

	// новейший сегмент в начало - при поиске проверяем сначала его
	l.l0 = append([]*SSTable{sst}, l.l0...)
	l.memTable.Clear()

	if err := l.saveMeta(); err != nil {
		return err
	}

	if len(l.l0) >= maxL0Tables {
		return l.compact()
	}
	return nil
}

func (l *LSM) Compact() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.compact()
}

func (l *LSM) compact() error {
	if len(l.l0) == 0 {
		return nil
	}

	tables := make([]*SSTable, 0, len(l.l0)+1)
	tables = append(tables, l.l0...)
	if l.l1 != nil {
		tables = append(tables, l.l1)
	}

	newPath := filepath.Join(l.dataDir, "l1_new.sst")
	if err := MergeSSTables(newPath, tables); err != nil {
		return fmt.Errorf("merge SSTables: %w", err)
	}

	finalPath := filepath.Join(l.dataDir, "l1.sst")
	if err := os.Rename(newPath, finalPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("rename l1: %w", err)
	}

	for _, sst := range l.l0 {
		os.Remove(sst.path)
	}

	newL1, err := OpenSSTable(finalPath)
	if err != nil {
		return fmt.Errorf("open new L1: %w", err)
	}
	l.l1 = newL1
	l.l0 = nil
	return nil
}

func (l *LSM) Lookup(term string) *roaring.Bitmap {
	l.mu.RLock()
	defer l.mu.RUnlock()

	processed := l.processor.ProcessTerm(term)
	if processed == "" {
		return roaring.New()
	}

	result := roaring.New()
	if bm, ok := l.memTable.Get(processed); ok {
		result.Or(bm)
	}
	for _, sst := range l.l0 {
		if bm, ok := sst.Get(processed); ok {
			result.Or(bm)
		}
	}
	if l.l1 != nil {
		if bm, ok := l.l1.Get(processed); ok {
			result.Or(bm)
		}
	}
	return result
}

func (l *LSM) Universe() *roaring.Bitmap {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.universe.Clone()
}

func (l *LSM) Stats() map[string]any {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return map[string]any{
		"documents":       l.universe.GetCardinality(),
		"memtable_terms":  l.memTable.Size(),
		"l0_segments":     len(l.l0),
		"l1_exists":       l.l1 != nil,
		"flush_threshold": l.flushThreshold,
	}
}

func (l *LSM) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.memTable.Size() > 0 {
		return l.flush()
	}
	return l.saveMeta()
}

func (l *LSM) saveMeta() error {
	uData, err := l.universe.ToBytes()
	if err != nil {
		return fmt.Errorf("serialize universe: %w", err)
	}
	if err := os.WriteFile(filepath.Join(l.dataDir, "universe.bin"), uData, 0o644); err != nil {
		return err
	}

	var sb strings.Builder
	for id, path := range l.DocPaths {
		fmt.Fprintf(&sb, "%d\t%s\n", id, path)
	}
	return os.WriteFile(filepath.Join(l.dataDir, "docs.tsv"), []byte(sb.String()), 0o644)
}

func (l *LSM) loadExisting() error {
	l1Path := filepath.Join(l.dataDir, "l1.sst")
	if _, err := os.Stat(l1Path); err == nil {
		sst, err := OpenSSTable(l1Path)
		if err != nil {
			return fmt.Errorf("open L1: %w", err)
		}
		l.l1 = sst
	}

	entries, err := os.ReadDir(l.dataDir)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", l.dataDir, err)
	}

	var l0Names []string
	maxID := -1
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "l0_") && strings.HasSuffix(name, ".sst") {
			l0Names = append(l0Names, name)
			idStr := strings.TrimSuffix(strings.TrimPrefix(name, "l0_"), ".sst")
			if id, err := strconv.Atoi(idStr); err == nil && id > maxID {
				maxID = id
			}
		}
	}
	l.nextSSTableID = maxID + 1

	sort.Sort(sort.Reverse(sort.StringSlice(l0Names))) // newest-first
	l0Slices := make([]*SSTable, 0, len(l0Names))
	for _, name := range l0Names {
		sst, err := OpenSSTable(filepath.Join(l.dataDir, name))
		if err != nil {
			return fmt.Errorf("open L0 %s: %w", name, err)
		}
		l0Slices = append(l0Slices, sst)
	}
	l.l0 = l0Slices

	universePath := filepath.Join(l.dataDir, "universe.bin")
	if data, err := os.ReadFile(universePath); err == nil {
		if _, err := l.universe.FromBuffer(data); err != nil {
			return fmt.Errorf("load universe: %w", err)
		}
	}

	docsPath := filepath.Join(l.dataDir, "docs.tsv")
	if data, err := os.ReadFile(docsPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				continue
			}
			id64, err := strconv.ParseUint(parts[0], 10, 32)
			if err != nil {
				continue
			}
			id := uint32(id64)
			l.DocPaths[id] = parts[1]
			if id >= l.nextDocID {
				l.nextDocID = id + 1
			}
		}
	}

	return nil
}
