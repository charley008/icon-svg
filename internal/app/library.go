package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Library owns the file tree. Root confines operations even if a directory is
// replaced by a symlink between validation and opening the file.
type Library struct {
	root        *os.Root
	lock        *os.File
	mu          sync.Mutex
	fingerprint string
	records     map[string]libraryFileRecord
	loadStats   scanIOStats
	scanMu      sync.Mutex
	statusMu    sync.RWMutex
	status      LibraryScanStatus
	scanned     map[string]string
}

func reservedName(s string) bool {
	s = strings.ToLower(strings.Split(s, ".")[0])
	switch s {
	case "con", "prn", "aux", "nul", "licenses":
		return true
	}
	return len(s) == 4 && (strings.HasPrefix(s, "com") || strings.HasPrefix(s, "lpt")) && s[3] >= '0' && s[3] <= '9'
}
func recordID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
func isPublished(v *bool) bool { return v == nil || *v }
func publication(v bool) *bool {
	if v {
		return nil
	}
	return &v
}

// libraryInput exposes only collection directories to the index loader.
type libraryInput struct{ fs.FS }

func (l libraryInput) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, e := fs.ReadDir(l.FS, name)
	if e != nil || name != "." {
		return entries, e
	}
	out := []fs.DirEntry{}
	for _, entry := range entries {
		if entry.Name() == "licenses" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("素材库不允许符号链接")
		}
		if entry.IsDir() {
			out = append(out, entry)
		}
	}
	return out, nil
}

func openLibrary(dir string) (*Library, error) {
	info, e := os.Lstat(dir)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("library 必须是真实目录，不能是符号链接")
	}
	root, e := os.OpenRoot(dir)
	if e != nil {
		return nil, e
	}
	if e = rejectReparsePoint(root, "."); e != nil {
		root.Close()
		return nil, e
	}
	l := &Library{root: root}
	if e = l.check(".library.lock"); e != nil {
		root.Close()
		return nil, e
	}
	l.lock, e = root.OpenFile(".library.lock", os.O_CREATE|os.O_RDWR, 0600)
	if e == nil {
		e = lockLibrary(l.lock)
	}
	if e != nil {
		l.Close()
		return nil, fmt.Errorf("素材库已被其他进程使用或无法加锁: %w", e)
	}
	if e = l.recover(); e != nil {
		l.Close()
		return nil, e
	}
	return l, nil
}
func (l *Library) Close() {
	if l.lock != nil {
		l.lock.Close()
	}
	if l.root != nil {
		l.root.Close()
	}
}

func (l *Library) check(name string) error {
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || strings.HasPrefix(name, "/") {
		return errors.New("素材路径无效")
	}
	parts := strings.Split(name, "/")
	for n := range parts {
		component := strings.Join(parts[:n+1], "/")
		info, e := l.root.Lstat(component)
		if errors.Is(e, fs.ErrNotExist) {
			return nil
		}
		if e != nil {
			return e
		}
		if e = rejectReparsePoint(l.root, component); e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("素材库不允许链接或特殊文件")
		}
	}
	return nil
}
func (l *Library) read(name string) ([]byte, error) {
	if e := l.check(name); e != nil {
		return nil, e
	}
	f, e := l.root.Open(name)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return nil, e
	}
	limit := int64(16 << 20)
	if name == ".pending.json" {
		// The durable journal contains base64 data for an entire bounded library.
		limit = 384 << 20
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("素材文件类型或大小无效")
	}
	content, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(content)) > limit {
		return nil, errors.New("素材文件过大")
	}
	return content, e
}

// atomicFile never truncates a user-supplied target. Rename replaces a link
// instead of following it; the temporary file is exclusively created in Root.
func (l *Library) atomicFile(name string, content []byte) error {
	if e := l.check(name); e != nil {
		return e
	}
	parent := path.Dir(name)
	if parent != "." {
		if e := l.root.MkdirAll(parent, 0755); e != nil {
			return e
		}
	}
	tmp := path.Join(parent, ".tmp-"+randomToken())
	f, e := l.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer l.root.Remove(tmp)
	if _, e = f.Write(content); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return l.root.Rename(tmp, name)
}

func libraryTarget(name string) bool {
	if libraryLicenseTarget(name) {
		return true
	}
	if name == "categories.json" {
		return true
	}
	parts := strings.Split(name, "/")
	return len(parts) == 2 && validSlug(parts[0]) && (parts[1] == "meta.json" || validSVGName(parts[1]))
}
func validSVGName(name string) bool {
	return strings.HasSuffix(name, ".svg") && len(name) <= 180 && slugRE.MatchString(strings.TrimSuffix(name, ".svg")) && !reservedName(name)
}

// A durable redo journal makes multi-file changes recoverable after a crash.
// It contains only validated library-relative targets and is removed last.
// A JSON null value means remove the file; non-null values replace its bytes.
func (l *Library) recover() error {
	raw, e := l.read(".pending.json")
	if errors.Is(e, fs.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	var files map[string][]byte
	if e = json.Unmarshal(raw, &files); e != nil {
		return fmt.Errorf("素材写入日志损坏: %w", e)
	}
	keys := make([]string, 0, len(files))
	for name := range files {
		if !libraryTarget(name) {
			return errors.New("素材写入日志包含无效路径")
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		if files[name] == nil {
			if e = l.check(name); e == nil {
				e = l.root.Remove(name)
				if errors.Is(e, fs.ErrNotExist) {
					e = nil
				}
			}
		} else {
			e = l.atomicFile(name, files[name])
		}
		if e != nil {
			return e
		}
	}
	// A deleted metadata file denotes deletion of the entire collection.
	// Remove only its now-empty directory, never recursively follow contents.
	for _, name := range keys {
		if files[name] == nil && strings.HasSuffix(name, "/meta.json") {
			if e = l.check(path.Dir(name)); e != nil {
				return e
			}
			if e = l.root.Remove(path.Dir(name)); e != nil && !errors.Is(e, fs.ErrNotExist) {
				return e
			}
		}
	}
	return l.root.Remove(".pending.json")
}
func (l *Library) save(files map[string][]byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.saveLocked(files)
}

func (l *Library) saveLocked(files map[string][]byte) error {
	if _, e := l.root.Lstat(".pending.json"); e == nil {
		return errors.New("素材库存在未完成写入，请先刷新素材库恢复")
	}
	current, e := l.currentFingerprint()
	if e != nil {
		return e
	}
	if l.fingerprint != "" && current != l.fingerprint {
		return errors.New("素材文件已被外部修改，请先刷新素材库后再保存")
	}
	changes := map[string][]byte{}
	for name, content := range files {
		if !libraryTarget(name) {
			return errors.New("拒绝写入无效素材路径")
		}
		old, e := l.read(name)
		if e != nil && !errors.Is(e, fs.ErrNotExist) {
			return e
		}
		if (content == nil && e == nil) || (content != nil && !bytes.Equal(old, content)) {
			changes[name] = content
		}
	}
	if len(changes) > 0 {
		raw, e := json.Marshal(changes)
		if e != nil {
			return e
		}
		if e = l.atomicFile(".pending.json", raw); e != nil {
			return e
		}
		if e = l.recover(); e != nil {
			return e
		}
	}
	updated, e := l.currentFingerprint()
	if e != nil {
		return e
	}
	l.fingerprint = updated
	return nil
}

// Load a bounded, validated snapshot before touching the live index. A bad
// external edit leaves the last good index available and never partially loads.
func (l *Library) load() (map[string][]byte, error) { return l.loadFiles(false) }
func (l *Library) loadFiles(useCache bool) (map[string][]byte, error) {
	files := map[string][]byte{}
	e := l.walkFiles(useCache, func(name string, content []byte) { files[name] = content })
	return files, e
}

// Saving only needs a consistency fingerprint. Hash one file at a time rather
// than allocating another complete SVG snapshot beside the pending changes.
func (l *Library) currentFingerprint() (string, error) {
	_, records, _, err := l.scanFileTree(l.records, false)
	if err != nil {
		return "", err
	}
	return fingerprintHashes(recordHashes(records)), nil
}

func (l *Library) walkFiles(useCache bool, visit func(string, []byte)) error {
	l.loadStats = scanIOStats{}
	entries, e := fs.ReadDir(l.root.FS(), ".")
	if e != nil {
		return e
	}
	files := map[string]bool{}
	total := 0
	for _, entry := range entries {
		if e = l.check(entry.Name()); e != nil {
			return e
		}
		if entry.Name() == "licenses" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("素材库不允许符号链接")
		}
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		if !validSlug(slug) {
			return fmt.Errorf("无效合集目录: %s", slug)
		}
		children, e := fs.ReadDir(l.root.FS(), slug)
		if e != nil {
			return e
		}
		for _, child := range children {
			if e = l.check(slug + "/" + child.Name()); e != nil {
				return e
			}
			if child.IsDir() || strings.HasPrefix(child.Name(), ".") {
				continue
			}
			if child.Name() != "meta.json" && !strings.HasSuffix(child.Name(), ".svg") {
				continue
			}
			name := slug + "/" + child.Name()
			if !libraryTarget(name) {
				return fmt.Errorf("无效素材文件名: %s", name)
			}
			content, _, reused, e := l.readScannedFile(name, useCache)
			if e != nil {
				return e
			}
			if reused {
				l.loadStats.Reused++
			} else {
				l.loadStats.Read++
			}

			total += len(content)
			if total > 256<<20 || len(files) > 50000 {
				return errors.New("素材库超出扫描限制")
			}
			files[name] = true
			visit(name, content)
		}
		if _, ok := files[slug+"/meta.json"]; !ok {
			return fmt.Errorf("合集缺少 meta.json: %s", slug)
		}
	}
	categories, e := l.read("categories.json")
	if e == nil {
		l.loadStats.Read++
		visit("categories.json", categories)
	} else if !errors.Is(e, fs.ErrNotExist) {
		return e
	}
	return nil
}
func fingerprint(files map[string][]byte) string {
	hashes := make(map[string]string, len(files))
	for name, content := range files {
		hashes[name] = digest(content)
	}
	return fingerprintHashes(hashes)
}
func fingerprintHashes(hashes map[string]string) string {
	keys := make([]string, 0, len(hashes))
	for k := range hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteByte(0)
		buf.WriteString(hashes[k])
		buf.WriteByte(0)
	}
	return digest(buf.Bytes())
}

func (s *Store) metadataSnapshot(q queryer, subset ...map[string]bool) (map[string][]byte, error) {
	files := map[string][]byte{}
	var rows *sql.Rows
	var e error
	if len(subset) > 0 {
		names := []string{}
		for name := range subset[0] {
			names = append(names, name)
		}
		raw, _ := json.Marshal(names)
		rows, e = q.Query(`SELECT id,slug,COALESCE(parent,''),metadata,published,deleted FROM brands WHERE slug IN(SELECT value FROM json_each(?)) ORDER BY slug`, string(raw))
	} else {
		rows, e = q.Query(`SELECT id,slug,COALESCE(parent,''),metadata,published,deleted FROM brands ORDER BY slug`)
	}
	if e != nil {
		return nil, e
	}
	brands := []Brand{}
	for rows.Next() {
		b, err := scanBrand(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		brands = append(brands, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	for _, b := range brands {
		if !validSlug(b.Slug) {
			return nil, errors.New("无效合集标识")
		}
		m := b.Meta
		m.ID = b.ID
		m.Schema = 1
		m.Parent = b.Parent
		m.Published = publication(b.Published)
		m.Deleted = b.Deleted
		m.Products = []GroupMeta{}
		gr, e := q.Query(`SELECT g.id,g.brand_id,b.slug,g.product_key,g.metadata,g.next_version,g.published,g.deleted,g.updated_at FROM groups g JOIN brands b ON b.id=g.brand_id WHERE g.brand_id=? ORDER BY g.product_key`, b.ID)
		if e != nil {
			return nil, e
		}
		groups := []Group{}
		for gr.Next() {
			g, err := scanGroup(gr)
			if err != nil {
				gr.Close()
				return nil, err
			}
			groups = append(groups, g)
		}
		e = gr.Err()
		gr.Close()
		if e != nil {
			return nil, e
		}
		for _, g := range groups {
			if g.Key != "" && !validSlug(g.Key) {
				return nil, errors.New("无效产品标识")
			}
			gm := cleanGroup(g.Meta)
			gm.ID = g.ID
			gm.Published = publication(g.Published)
			gm.Deleted = g.Deleted
			icons, e := iconsFor(q, g.ID)
			if e != nil {
				return nil, e
			}
			for _, i := range icons {
				if i.Meta.File != filename(b.Slug, g.Key, i.Meta.Version) {
					return nil, errors.New("无效素材文件名")
				}
				im := i.Meta
				im.ID = i.ID
				im.Deleted = i.Deleted
				gm.Icons = append(gm.Icons, im)

			}
			if g.Key == "" {
				m.GroupMeta = gm
				m.Published = publication(b.Published)
				m.Deleted = b.Deleted
			} else {
				m.Products = append(m.Products, gm)
			}
		}
		raw, e := json.MarshalIndent(m, "", "  ")
		if e != nil {
			return nil, e
		}
		files[b.Slug+"/meta.json"] = append(raw, '\n')
	}
	rows, e = q.Query(`SELECT name FROM categories ORDER BY name`)
	if e != nil {
		return nil, e
	}
	names := []string{}
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			rows.Close()
			return nil, e
		}
		names = append(names, name)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	raw, e := json.MarshalIndent(names, "", "  ")
	if e != nil {
		return nil, e
	}
	files["categories.json"] = append(raw, '\n')
	for name, raw := range s.pendingSVG {
		if strings.HasSuffix(name, ".svg") {
			files[name] = raw
		}
	}
	for name := range s.pendingDeletes {
		files[name] = nil
	}
	return files, nil
}

// The disk catalogue is disposable; library files remain the authority. The
// attached runtime database alone contains accounts and operational state.
func openLibraryStore(file, dir, username, password string) (*Store, error) {
	l, e := openLibrary(dir)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			l.Close()
		}
	}()
	dataDir := filepath.Dir(file)
	if e = os.MkdirAll(dataDir, 0700); e != nil {
		return nil, e
	}
	indexFile := filepath.Join(dataDir, "catalog.sqlite3")
	// Build an empty schema, then index exactly once through the normal scanner.
	s, e := openPersistentCatalog(indexFile)
	if e != nil {
		return nil, e
	}
	defer func() {
		if !ok {
			s.close()
		}
	}()
	if _, e = s.db.Exec(`ATTACH DATABASE ? AS runtime`, file); e != nil {
		return nil, e
	}
	if _, e = s.db.Exec(`PRAGMA runtime.journal_mode=WAL; PRAGMA runtime.cache_size=-512;
 CREATE TABLE IF NOT EXISTS runtime.admins(username TEXT PRIMARY KEY,password TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS runtime.sessions(token TEXT PRIMARY KEY,csrf TEXT NOT NULL,expires INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS runtime.attempts(ip TEXT NOT NULL,created INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS runtime.attempts_time ON attempts(created);
 CREATE TABLE IF NOT EXISTS runtime.audit(id INTEGER PRIMARY KEY,action TEXT NOT NULL,target TEXT NOT NULL,created_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS runtime.settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS runtime.downloads(key TEXT PRIMARY KEY,count INTEGER NOT NULL);`); e != nil {
		return nil, e
	}
	var admins int
	if e = s.db.QueryRow(`SELECT COUNT(*) FROM runtime.admins`).Scan(&admins); e != nil {
		return nil, e
	}
	if admins == 0 {
		if len(password) < 8 || len(password) > 256 {
			return nil, errors.New("首次启动需要 8–256 位 ADMIN_PASSWORD")
		}
		if username == "" {
			username = "admin"
		}
		if _, e = s.db.Exec(`INSERT INTO runtime.admins VALUES(?,?)`, username, passwordHash(password)); e != nil {
			return nil, e
		}
	}
	s.library = l
	s.source = l.root.FS()
	var indexedRoot string
	if err := s.db.QueryRow(`SELECT root,fingerprint FROM library_state WHERE id=1`).Scan(&indexedRoot, &l.fingerprint); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if indexedRoot != l.root.Name() {
		l.fingerprint = ""
		if _, e = s.db.Exec(`DELETE FROM library_files; DELETE FROM library_state`); e != nil {
			return nil, e
		}
	}
	if records, err := readLibraryRecords(s.db); err == nil {
		l.scanned = recordHashes(records)
		l.records = records
	} else {
		return nil, err
	}
	if e = s.reloadLibrary(); e != nil {
		return nil, e
	}
	// Initial indexing establishes a baseline, not an import batch.
	l.statusMu.Lock()
	l.status.RecentAdded = nil
	l.status.RecentAddedTotal = 0
	l.status.AddedAt = time.Time{}
	l.statusMu.Unlock()
	ok = true
	return s, nil
}

func (s *Store) reloadLibrary() error { return s.scanLibrary(false) }
func (s *Store) reloadLibraryIndex(full bool, run *scanRun) error {
	l := s.library
	old, err := readLibraryRecords(s.db)
	if err != nil {
		return err
	}
	l.mu.Lock()
	if err = l.recover(); err != nil {
		l.mu.Unlock()
		return err
	}
	_, records, stats, err := l.scanFileTree(old, full)
	// Compare to the committed manifest, not a file-write transaction's
	// provisional in-memory fingerprint while that transaction is committing.
	baseline := fingerprintHashes(recordHashes(old))
	l.mu.Unlock()
	if err != nil {
		return err
	}
	run.IO, run.Hashes = stats, recordHashes(records)
	if !full && fingerprintHashes(run.Hashes) == baseline {
		return nil
	}
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if err = l.recover(); err != nil {
		return err
	}
	files, records, _, err := l.scanFileTree(records, false)
	if err != nil {
		return err
	}
	if !full && fingerprintHashes(recordHashes(records)) == l.fingerprint {
		run.Hashes = recordHashes(records)
		return nil
	}
	normalized, err := s.normalizeLibraryFiles(files)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	changed := map[string]bool{}
	for name, r := range records {
		if previous, ok := old[name]; full || !ok || previous.Hash != r.Hash {
			if strings.Contains(name, "/") {
				changed[path.Dir(name)] = true
			}
		}
	}
	for name := range old {
		if _, ok := records[name]; !ok && strings.Contains(name, "/") {
			changed[path.Dir(name)] = true
		}
	}
	for name, raw := range normalized {
		if strings.HasSuffix(name, "/meta.json") && digest(raw) != records[name].Hash {
			changed[path.Dir(name)] = true
		}
	}
	if _, err = tx.Exec(`PRAGMA defer_foreign_keys=ON; DELETE FROM categories`); err != nil {
		return err
	}
	for slug := range changed {
		if _, err = tx.Exec(`DELETE FROM icons WHERE brand_id IN(SELECT id FROM brands WHERE slug=?); DELETE FROM groups WHERE brand_id IN(SELECT id FROM brands WHERE slug=?); DELETE FROM brands WHERE slug=?`, slug, slug, slug); err != nil {
			return err
		}
	}
	if err = loadCatalog(tx, libraryInput{collectionSubset{FS: snapshotFS(normalized), slugs: changed}}, records); err != nil {
		return err
	}
	if raw, ok := normalized["categories.json"]; ok {
		var names []string
		if err = json.Unmarshal(raw, &names); err != nil {
			return err
		}
		for _, name := range names {
			if strings.TrimSpace(name) == "" || len(name) > 240 {
				return errors.New("无效分类名称")
			}
			if _, err = tx.Exec(`INSERT OR IGNORE INTO categories VALUES(?)`, name); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(`INSERT OR IGNORE INTO categories VALUES('其他'); INSERT OR IGNORE INTO categories SELECT DISTINCT json_extract(metadata,'$.category') FROM groups WHERE COALESCE(json_extract(metadata,'$.category'),'')<>''; UPDATE icons SET downloads=COALESCE((SELECT count FROM runtime.downloads WHERE key=(SELECT slug FROM brands WHERE id=icons.brand_id)||'/'||icons.filename),0)`); err != nil {
		return err
	}
	canonical, err := s.metadataSnapshot(tx, changed)
	if err != nil {
		return err
	}
	previous := l.fingerprint
	previousRecords := l.records
	published := false
	defer func() {
		if !published {
			l.fingerprint = previous
			l.records = previousRecords
		}
	}()
	// The source has just been scanned; publish only canonical metadata.
	l.fingerprint = fingerprintHashes(recordHashes(records))
	l.records = records
	if err = l.saveLocked(canonical); err != nil {
		return err
	}
	_, records, _, err = l.scanFileTree(records, false)
	if err != nil {
		return err
	}
	if err = s.saveLibraryRecords(tx, records); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	published = true
	l.records = records
	run.Hashes = recordHashes(records)
	run.Updated = true
	return nil
}

func (s *Store) recordDownload(i Icon) error {
	return s.recordDownloadContext(context.Background(), i)
}
func (s *Store) recordDownloadContext(ctx context.Context, i Icon) error {
	if e := lockContext(ctx, &s.writeMu); e != nil {
		return e
	}
	defer s.writeMu.Unlock()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO runtime.downloads(key,count) VALUES(?,1) ON CONFLICT(key) DO UPDATE SET count=count+1`, i.Brand+"/"+i.Meta.File); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE icons SET downloads=downloads+1 WHERE id=?`, i.ID); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) close() {
	s.db.Close()
	if s.library != nil {
		s.library.Close()
	}
}
