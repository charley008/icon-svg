package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
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
	cache       map[string]cachedLibraryFile
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
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil, errors.New("素材文件类型或大小无效")
	}
	content, e := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if len(content) > 16<<20 {
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
		if e = l.atomicFile(name, files[name]); e != nil {
			return e
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
	current, e := l.load()
	if e != nil {
		return e
	}
	if l.fingerprint != "" && fingerprint(current) != l.fingerprint {
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
		if !bytes.Equal(old, content) {
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
	updated, e := l.load()
	if e != nil {
		return e
	}
	l.fingerprint = fingerprint(updated)
	return nil
}

// Load a bounded, validated snapshot before touching the live index. A bad
// external edit leaves the last good index available and never partially loads.
func (l *Library) load() (map[string][]byte, error) { return l.loadFiles(false) }
func (l *Library) loadFiles(useCache bool) (map[string][]byte, error) {
	l.loadStats = scanIOStats{}
	nextCache := map[string]cachedLibraryFile{}
	entries, e := fs.ReadDir(l.root.FS(), ".")
	if e != nil {
		return nil, e
	}
	files := map[string][]byte{}
	total := 0
	for _, entry := range entries {
		if e = l.check(entry.Name()); e != nil {
			return nil, e
		}
		if entry.Name() == "licenses" || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, errors.New("素材库不允许符号链接")
		}
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		if !validSlug(slug) {
			return nil, fmt.Errorf("无效合集目录: %s", slug)
		}
		children, e := fs.ReadDir(l.root.FS(), slug)
		if e != nil {
			return nil, e
		}
		for _, child := range children {
			if e = l.check(slug + "/" + child.Name()); e != nil {
				return nil, e
			}
			if child.IsDir() || strings.HasPrefix(child.Name(), ".") {
				continue
			}
			if child.Name() != "meta.json" && !strings.HasSuffix(child.Name(), ".svg") {
				continue
			}
			name := slug + "/" + child.Name()
			if !libraryTarget(name) {
				return nil, fmt.Errorf("无效素材文件名: %s", name)
			}
			content, info, reused, e := l.readScannedFile(name, useCache)
			if e != nil {
				return nil, e
			}
			if reused {
				l.loadStats.Reused++
			} else {
				l.loadStats.Read++
			}
			if strings.HasSuffix(name, ".svg") {
				nextCache[name] = cachedLibraryFile{Info: info, Content: content}
			}

			total += len(content)
			if total > 256<<20 || len(files) > 50000 {
				return nil, errors.New("素材库超出扫描限制")
			}
			files[name] = content
		}
		if _, ok := files[slug+"/meta.json"]; !ok {
			return nil, fmt.Errorf("合集缺少 meta.json: %s", slug)
		}
	}
	categories, e := l.read("categories.json")
	if e == nil {
		l.loadStats.Read++
		files["categories.json"] = categories
	} else if !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	l.cache = nextCache
	return files, nil
}
func fingerprint(files map[string][]byte) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for _, k := range keys {
		buf.WriteString(k)
		buf.WriteByte(0)
		buf.WriteString(digest(files[k]))
		buf.WriteByte(0)
	}
	return digest(buf.Bytes())
}

func librarySnapshot(q queryer) (map[string][]byte, error) {
	files := map[string][]byte{}
	rows, e := q.Query(`SELECT id,slug,COALESCE(parent,''),metadata,published,deleted FROM brands ORDER BY slug`)
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
				var content []byte
				if e = q.QueryRow(`SELECT svg FROM icons WHERE id=?`, i.ID).Scan(&content); e != nil {
					return nil, e
				}
				im := i.Meta
				im.ID = i.ID
				im.Deleted = i.Deleted
				gm.Icons = append(gm.Icons, im)
				files[b.Slug+"/"+im.File] = content
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
	return files, nil
}

// Catalog tables live only in :memory:. Operational tables are resolved from
// the attached disk database; no catalog bytes are written to that database.
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
	input, e := l.input()
	if e != nil {
		return nil, e
	}
	s, e := newIndex(input)
	if e != nil {
		return nil, e
	}
	defer func() {
		if !ok {
			s.db.Close()
		}
	}()
	if _, e = s.db.Exec(`ATTACH DATABASE ? AS runtime`, file); e != nil {
		return nil, e
	}
	if _, e = s.db.Exec(`PRAGMA runtime.journal_mode=WAL; PRAGMA temp_store=MEMORY;
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
	l.fingerprint = ""
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

func (l *Library) input() (libraryInput, error) {
	files, e := l.load()
	if e != nil {
		return libraryInput{}, e
	}
	files, e = normalizeLibraryFiles(files)
	if e != nil {
		return libraryInput{}, e
	}
	return libraryInput{snapshotFS(files)}, nil
}

func (s *Store) reloadLibrary() error { return s.scanLibrary(false) }
func (s *Store) reloadLibraryIndex(full bool, run *scanRun) error {
	// Most periodic scans find no changes. Read files without blocking page
	// requests; take the view lock only when publishing a changed index.
	l := s.library
	l.mu.Lock()
	probe, probeErr := l.loadFiles(!full)
	run.Files = probe
	run.IO = l.loadStats
	unchanged := probeErr == nil && fingerprint(probe) == l.fingerprint
	pending := false
	if _, e := l.root.Lstat(".pending.json"); e == nil {
		unchanged = false
		pending = true
	}
	l.mu.Unlock()
	if probeErr != nil && !pending {
		return probeErr
	}
	if unchanged {
		return nil
	}
	s.viewMu.Lock()
	defer s.viewMu.Unlock()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.recover(); e != nil {
		return e
	}
	files, e := l.load()
	if e != nil {
		return e
	}
	fp := fingerprint(files)
	if fp == l.fingerprint {
		return nil
	}
	normalized, e := normalizeLibraryFiles(files)
	if e != nil {
		return e
	}
	input := libraryInput{snapshotFS(normalized)}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`DELETE FROM icons;DELETE FROM groups;UPDATE brands SET parent=NULL;DELETE FROM brands;DELETE FROM categories;`); e != nil {
		return e
	}
	if len(files) > 0 {
		if e = loadCatalog(tx, input); e != nil {
			return e
		}
	}
	if raw, ok := files["categories.json"]; ok {
		var names []string
		if e = json.Unmarshal(raw, &names); e != nil {
			return e
		}
		for _, name := range names {
			if strings.TrimSpace(name) == "" || len(name) > 240 {
				return errors.New("无效分类名称")
			}
			if _, e = tx.Exec(`INSERT OR IGNORE INTO categories(name) VALUES(?)`, name); e != nil {
				return e
			}
		}
	}
	if _, e = tx.Exec(`INSERT OR IGNORE INTO categories VALUES('其他'); INSERT OR IGNORE INTO categories SELECT DISTINCT json_extract(metadata,'$.category') FROM groups WHERE COALESCE(json_extract(metadata,'$.category'),'')<>''; UPDATE icons SET downloads=COALESCE((SELECT count FROM runtime.downloads WHERE key=(SELECT slug FROM brands WHERE id=icons.brand_id)||'/'||icons.filename),0)`); e != nil {
		return e
	}
	previous := l.fingerprint
	published := false
	defer func() {
		if !published {
			l.fingerprint = previous
		}
	}()
	l.fingerprint = fp
	canonical, e := librarySnapshot(tx)
	if e != nil {
		return e
	}
	if e = l.saveLocked(canonical); e != nil {
		return e
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	published = true
	run.Files = canonical
	run.Updated = true
	return nil
}

func (s *Store) recordDownload(i Icon) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`INSERT INTO runtime.downloads(key,count) VALUES(?,1) ON CONFLICT(key) DO UPDATE SET count=count+1`, i.Brand+"/"+i.Meta.File); e != nil {
		return e
	}
	if _, e = tx.Exec(`UPDATE icons SET downloads=downloads+1 WHERE id=?`, i.ID); e != nil {
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
