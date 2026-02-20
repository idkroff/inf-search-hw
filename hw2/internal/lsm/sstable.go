package lsm

// формат файла: заголовок(12б) | данные(term->bitmap по алфавиту) | индекс(term->offset) | футер(12б)

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/RoaringBitmap/roaring"
)

const (
	sstMagic    uint32 = 0x53535449 // "SSTI"
	sstEndMagic uint32 = 0xDEADBEEF
	sstVersion  uint32 = 1
)

type indexEntry struct {
	term       string
	dataOffset int64
}

type SSTable struct {
	path    string
	index   []indexEntry // весь индекс держим в RAM для бинарного поиска
	dataEnd int64        // байтовое смещение начала индекса (= конец секции данных)
}

func (s *SSTable) Get(term string) (*roaring.Bitmap, bool) {
	i := sort.Search(len(s.index), func(i int) bool {
		return s.index[i].term >= term
	})
	if i >= len(s.index) || s.index[i].term != term {
		return nil, false
	}

	f, err := os.Open(s.path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	if _, err := f.Seek(s.index[i].dataOffset, io.SeekStart); err != nil {
		return nil, false
	}

	var termLen uint16
	if err := binary.Read(f, binary.BigEndian, &termLen); err != nil {
		return nil, false
	}
	// термин пропускаем — смещение уже нашли через индекс
	if _, err := f.Seek(int64(termLen), io.SeekCurrent); err != nil {
		return nil, false
	}

	var bitmapLen uint32
	if err := binary.Read(f, binary.BigEndian, &bitmapLen); err != nil {
		return nil, false
	}
	bitmapBytes := make([]byte, bitmapLen)
	if _, err := io.ReadFull(f, bitmapBytes); err != nil {
		return nil, false
	}

	bm := roaring.New()
	if _, err := bm.FromBuffer(bitmapBytes); err != nil {
		return nil, false
	}
	return bm, true
}

// Scan читает секцию данных последовательно — используется при компакции, чтобы не делать много случайных seek-ов
func (s *SSTable) Scan() (map[string]*roaring.Bitmap, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(12, io.SeekStart); err != nil { // пропускаем заголовок
		return nil, err
	}

	result := make(map[string]*roaring.Bitmap, len(s.index))
	reader := bufio.NewReader(f)
	pos := int64(12)

	for pos < s.dataEnd {
		var termLen uint16
		if err := binary.Read(reader, binary.BigEndian, &termLen); err != nil {
			return nil, fmt.Errorf("scan termLen: %w", err)
		}
		termBytes := make([]byte, termLen)
		if _, err := io.ReadFull(reader, termBytes); err != nil {
			return nil, fmt.Errorf("scan term: %w", err)
		}
		var bitmapLen uint32
		if err := binary.Read(reader, binary.BigEndian, &bitmapLen); err != nil {
			return nil, fmt.Errorf("scan bitmapLen: %w", err)
		}
		bitmapBytes := make([]byte, bitmapLen)
		if _, err := io.ReadFull(reader, bitmapBytes); err != nil {
			return nil, fmt.Errorf("scan bitmap: %w", err)
		}

		bm := roaring.New()
		if _, err := bm.FromBuffer(bitmapBytes); err != nil {
			return nil, fmt.Errorf("scan fromBuffer: %w", err)
		}
		result[string(termBytes)] = bm

		pos += 2 + int64(termLen) + 4 + int64(bitmapLen)
	}
	return result, nil
}

func OpenSSTable(path string) (*SSTable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var magic, version, numTerms uint32
	if err := binary.Read(f, binary.BigEndian, &magic); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}
	if magic != sstMagic {
		return nil, fmt.Errorf("invalid SSTable magic: %#x", magic)
	}
	_ = binary.Read(f, binary.BigEndian, &version)
	_ = binary.Read(f, binary.BigEndian, &numTerms)

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// футер — последние 12 байт файла
	if _, err := f.Seek(stat.Size()-12, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek footer: %w", err)
	}
	var indexOffset int64
	var endMagic uint32
	_ = binary.Read(f, binary.BigEndian, &indexOffset)
	_ = binary.Read(f, binary.BigEndian, &endMagic)
	if endMagic != sstEndMagic {
		return nil, fmt.Errorf("invalid SSTable end magic: %#x", endMagic)
	}

	if _, err := f.Seek(indexOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek index: %w", err)
	}
	index := make([]indexEntry, 0, numTerms)
	for i := uint32(0); i < numTerms; i++ {
		var termLen uint16
		if err := binary.Read(f, binary.BigEndian, &termLen); err != nil {
			return nil, fmt.Errorf("read index termLen[%d]: %w", i, err)
		}
		termBytes := make([]byte, termLen)
		if _, err := io.ReadFull(f, termBytes); err != nil {
			return nil, fmt.Errorf("read index term[%d]: %w", i, err)
		}
		var offset int64
		if err := binary.Read(f, binary.BigEndian, &offset); err != nil {
			return nil, fmt.Errorf("read index offset[%d]: %w", i, err)
		}
		index = append(index, indexEntry{term: string(termBytes), dataOffset: offset})
	}

	return &SSTable{
		path:    path,
		index:   index,
		dataEnd: indexOffset,
	}, nil
}

func WriteSSTable(path string, entries map[string]*roaring.Bitmap) error {
	terms := make([]string, 0, len(entries))
	for t := range entries {
		terms = append(terms, t)
	}
	sort.Strings(terms)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriterSize(f, 1<<20) // 1 MiB

	_ = binary.Write(w, binary.BigEndian, sstMagic)
	_ = binary.Write(w, binary.BigEndian, sstVersion)
	_ = binary.Write(w, binary.BigEndian, uint32(len(terms)))

	type idxRec struct {
		term   string
		offset int64
	}
	offsets := make([]idxRec, 0, len(terms))
	cur := int64(12) // после заголовка

	for _, term := range terms {
		bm := entries[term]
		bmBytes, err := bm.ToBytes()
		if err != nil {
			return fmt.Errorf("serialize bitmap for %q: %w", term, err)
		}
		termBytes := []byte(term)
		offsets = append(offsets, idxRec{term: term, offset: cur})

		_ = binary.Write(w, binary.BigEndian, uint16(len(termBytes)))
		_, _ = w.Write(termBytes)
		_ = binary.Write(w, binary.BigEndian, uint32(len(bmBytes)))
		_, _ = w.Write(bmBytes)

		cur += 2 + int64(len(termBytes)) + 4 + int64(len(bmBytes))
	}

	indexOffset := cur

	for _, rec := range offsets {
		tb := []byte(rec.term)
		_ = binary.Write(w, binary.BigEndian, uint16(len(tb)))
		_, _ = w.Write(tb)
		_ = binary.Write(w, binary.BigEndian, rec.offset)
	}

	_ = binary.Write(w, binary.BigEndian, indexOffset)
	_ = binary.Write(w, binary.BigEndian, sstEndMagic)

	return w.Flush()
}

// сливает несколько SSTable в одну, объединяя битмапы для одинаковых термов
func MergeSSTables(path string, tables []*SSTable) error {
	merged := make(map[string]*roaring.Bitmap)

	for _, tbl := range tables {
		entries, err := tbl.Scan()
		if err != nil {
			return fmt.Errorf("scan %s: %w", tbl.path, err)
		}
		for term, bm := range entries {
			if existing, ok := merged[term]; ok {
				existing.Or(bm)
			} else {
				merged[term] = bm
			}
		}
	}

	return WriteSSTable(path, merged)
}
