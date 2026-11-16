package workspaceexport

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Row is one exported record, keyed by column name.
type Row map[string]any

// Source yields the rows of one table of one workspace. The database source
// filters by workspace id in SQL; BuildArchive checks the id again, so a source
// that returns another workspace's row cannot put it in the archive.
type Source interface {
	Rows(ctx context.Context, table, workspaceID string, emit func(Row) error) error
}

// Manifest is manifest.json: what the archive holds and when it was made.
type Manifest struct {
	WorkspaceID string         `json:"workspace_id"`
	ExportedAt  time.Time      `json:"exported_at"`
	Counts      map[string]int `json:"counts"`
}

// BuildArchive reads every table from src and returns the zip holding one
// NDJSON file per table and manifest.json. Each row is cut down to the table's
// allowlist, and a row of another workspace is skipped.
func BuildArchive(ctx context.Context, src Source, workspaceID string, now time.Time) ([]byte, Manifest, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	manifest := Manifest{WorkspaceID: workspaceID, ExportedAt: now.UTC(), Counts: map[string]int{}}

	for _, table := range Tables {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: table + ".ndjson", Method: zip.Deflate, Modified: now})
		if err != nil {
			return nil, Manifest{}, err
		}
		cols := allowed[table]
		key := workspaceKey(table)
		count := 0
		err = src.Rows(ctx, table, workspaceID, func(row Row) error {
			if fmt.Sprint(row[key]) != workspaceID {
				return nil
			}
			out := make(Row, len(cols))
			for _, c := range cols {
				if v, ok := row[c]; ok {
					out[c] = v
				}
			}
			line, err := json.Marshal(out)
			if err != nil {
				return fmt.Errorf("encode %s row: %w", table, err)
			}
			if _, err := w.Write(append(line, '\n')); err != nil {
				return err
			}
			count++
			return nil
		})
		if err != nil {
			return nil, Manifest{}, fmt.Errorf("read %s: %w", table, err)
		}
		manifest.Counts[table] = count
	}

	w, err := zw.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate, Modified: now})
	if err != nil {
		return nil, Manifest{}, err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, Manifest{}, err
	}
	if _, err := w.Write(append(body, '\n')); err != nil {
		return nil, Manifest{}, err
	}
	if err := zw.Close(); err != nil {
		return nil, Manifest{}, err
	}
	return buf.Bytes(), manifest, nil
}
