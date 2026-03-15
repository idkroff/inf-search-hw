package main

import (
	"encoding/csv"
	"io"
	"os"
	"path/filepath"
	"time"
)

// loadMetadata читает docs/metadata.csv и возвращает map basename → (start, *end).
func loadMetadata(dir string) map[string][2]*time.Time {
	result := make(map[string][2]*time.Time)
	f, err := os.Open(filepath.Join(dir, "metadata.csv"))
	if err != nil {
		return result // нет файла — не страшно
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Read() // пропустить заголовок
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) < 3 {
			continue
		}
		name := rec[0]
		var pair [2]*time.Time
		if t, err := time.Parse(metaDateFmt, rec[1]); err == nil {
			pair[0] = &t
		}
		if rec[2] != "" {
			if t, err := time.Parse(metaDateFmt, rec[2]); err == nil {
				pair[1] = &t
			}
		}
		result[name] = pair
	}
	return result
}
