package lsm

import (
	"sort"
	"strings"

	"github.com/RoaringBitmap/roaring"
)

type MemTable struct {
	entries map[string]*roaring.Bitmap
}

func newMemTable() *MemTable {
	return &MemTable{
		entries: make(map[string]*roaring.Bitmap),
	}
}

func (m *MemTable) Add(term string, docID uint32) {
	bm, ok := m.entries[term]
	if !ok {
		bm = roaring.New()
		m.entries[term] = bm
	}
	bm.Add(docID)
}

func (m *MemTable) Get(term string) (*roaring.Bitmap, bool) {
	bm, ok := m.entries[term]
	return bm, ok
}

func (m *MemTable) Size() int {
	return len(m.entries)
}

func (m *MemTable) Terms() []string {
	terms := make([]string, 0, len(m.entries))
	for t := range m.entries {
		terms = append(terms, t)
	}
	sort.Strings(terms)
	return terms
}

func (m *MemTable) ToMap() map[string]*roaring.Bitmap {
	out := make(map[string]*roaring.Bitmap, len(m.entries))
	for term, bm := range m.entries {
		out[term] = bm.Clone()
	}
	return out
}

func (m *MemTable) Clear() {
	m.entries = make(map[string]*roaring.Bitmap)
}

func (m *MemTable) GetByPrefix(prefix string) map[string]*roaring.Bitmap {
	result := make(map[string]*roaring.Bitmap)
	for term, bm := range m.entries {
		if strings.HasPrefix(term, prefix) {
			result[term] = bm
		}
	}
	return result
}
