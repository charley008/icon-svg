package app

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

type libraryFileRecord struct {
	Hash           string
	Size, Modified int64
}

type collectionSubset struct {
	fs.FS
	slugs map[string]bool
}

func (s collectionSubset) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(s.FS, name)
	if err != nil || name != "." {
		return entries, err
	}
	out := []fs.DirEntry{}
	for _, entry := range entries {
		if s.slugs[entry.Name()] {
			out = append(out, entry)
		}
	}
	return out, nil
}

func readLibraryRecords(q queryer) (map[string]libraryFileRecord, error) {
	rows, err := q.Query(`SELECT path,hash,size,modified FROM library_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]libraryFileRecord{}
	for rows.Next() {
		var name string
		var r libraryFileRecord
		if err = rows.Scan(&name, &r.Hash, &r.Size, &r.Modified); err != nil {
			return nil, err
		}
		result[name] = r
	}
	return result, rows.Err()
}

func recordHashes(records map[string]libraryFileRecord) map[string]string {
	result := make(map[string]string, len(records))
	for name, r := range records {
		result[name] = r.Hash
	}
	return result
}

// Metadata is always reread. Unchanged SVGs need only a stat; changed SVGs are
// read and validated one at a time. Nil SVG entries are filename placeholders
// for metadata normalization, never material that may be written to disk.
func (l *Library) scanFileTree(old map[string]libraryFileRecord, full bool) (map[string][]byte, map[string]libraryFileRecord, scanIOStats, error) {
	files := map[string][]byte{}
	next := map[string]libraryFileRecord{}
	stats := scanIOStats{}
	var total int64
	visit := func(name string) error {
		if err := l.check(name); err != nil {
			return err
		}
		info, err := l.root.Stat(name)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 16<<20 {
			return errors.New("素材文件类型或大小无效")
		}
		total += info.Size()
		if total > 256<<20 || len(next) >= 50000 {
			return errors.New("素材库超出扫描限制")
		}
		r := libraryFileRecord{Size: info.Size(), Modified: info.ModTime().UnixNano()}
		if previous, ok := old[name]; !full && ok && strings.HasSuffix(name, ".svg") && previous.Size == r.Size && previous.Modified == r.Modified {
			r.Hash = previous.Hash
			stats.Reused++
		} else {
			raw, err := l.read(name)
			if err != nil {
				return err
			}
			after, err := l.root.Stat(name)
			if err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) || int64(len(raw)) != info.Size() {
				return fmt.Errorf("%s 在扫描期间发生变化", name)
			}
			if strings.HasSuffix(name, ".svg") {
				if _, err = validateSVG(raw); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			} else {
				files[name] = raw
			}
			r.Hash = digest(raw)
			stats.Read++
		}
		next[name] = r
		if strings.HasSuffix(name, ".svg") {
			files[name] = nil
		}
		return nil
	}
	entries, err := fs.ReadDir(l.root.FS(), ".")
	if err != nil {
		return nil, nil, stats, err
	}
	for _, entry := range entries {
		if err = l.check(entry.Name()); err != nil {
			return nil, nil, stats, err
		}
		if entry.Name() == "licenses" || strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
			continue
		}
		if !validSlug(entry.Name()) {
			return nil, nil, stats, errors.New("无效合集目录")
		}
		children, err := fs.ReadDir(l.root.FS(), entry.Name())
		if err != nil {
			return nil, nil, stats, err
		}
		for _, child := range children {
			name := entry.Name() + "/" + child.Name()
			if err = l.check(name); err != nil {
				return nil, nil, stats, err
			}
			if child.IsDir() || strings.HasPrefix(child.Name(), ".") || (child.Name() != "meta.json" && !strings.HasSuffix(child.Name(), ".svg")) {
				continue
			}
			if !libraryTarget(name) {
				return nil, nil, stats, errors.New("无效素材文件名")
			}
			if err = visit(name); err != nil {
				return nil, nil, stats, err
			}
		}
		if _, ok := next[entry.Name()+"/meta.json"]; !ok {
			return nil, nil, stats, errors.New("合集缺少 meta.json")
		}
	}
	if err = visit("categories.json"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, stats, err
	}
	return files, next, stats, nil
}

func (s *Store) saveLibraryRecords(tx *sql.Tx, records map[string]libraryFileRecord) error {
	old, err := readLibraryRecords(tx)
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO library_files VALUES(?,?,?,?) ON CONFLICT(path) DO UPDATE SET hash=excluded.hash,size=excluded.size,modified=excluded.modified`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for name, r := range records {
		if previous, ok := old[name]; ok && previous == r {
			continue
		}
		if _, err = stmt.Exec(name, r.Hash, r.Size, r.Modified); err != nil {
			return err
		}
	}
	for name := range old {
		if _, ok := records[name]; !ok {
			if _, err = tx.Exec(`DELETE FROM library_files WHERE path=?`, name); err != nil {
				return err
			}
		}
	}
	_, err = tx.Exec(`INSERT INTO library_state VALUES(1,?,?) ON CONFLICT(id) DO UPDATE SET root=excluded.root,fingerprint=excluded.fingerprint`, s.library.root.Name(), fingerprintHashes(recordHashes(records)))
	return err
}

func (s *Store) persistLibraryFiles(tx *sql.Tx) error {
	old, err := readLibraryRecords(tx)
	if err != nil {
		return err
	}
	s.library.mu.Lock()
	defer s.library.mu.Unlock()
	_, next, _, err := s.library.scanFileTree(old, false)
	if err != nil {
		return err
	}
	if err = s.saveLibraryRecords(tx, next); err != nil {
		return err
	}
	s.library.records = next
	return nil
}

// Import already holds the library lock.
func (s *Store) saveLibraryRecordsAfterImport(tx *sql.Tx) error {
	old, err := readLibraryRecords(tx)
	if err != nil {
		return err
	}
	_, records, _, err := s.library.scanFileTree(old, false)
	if err != nil {
		return err
	}
	if err = s.saveLibraryRecords(tx, records); err != nil {
		return err
	}
	s.library.records = records
	return nil
}
