package app

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type IconMeta struct {
	ID         int64                      `json:"id,omitempty"`
	Deleted    bool                       `json:"deleted,omitempty"`
	Extra      map[string]json.RawMessage `json:"-"`
	File       string                     `json:"file"`
	Version    int                        `json:"version"`
	Label      string                     `json:"label"`
	Default    bool                       `json:"is_default"`
	SHA        string                     `json:"sha256"`
	Bytes      int                        `json:"bytes"`
	Source     string                     `json:"source"`
	License    string                     `json:"license"`
	Collection string                     `json:"collection,omitempty"`
	Method     string                     `json:"method,omitempty"`
	Provenance map[string]any             `json:"provenance,omitempty"`
}

func (i *IconMeta) UnmarshalJSON(data []byte) error {
	type plain IconMeta
	var value plain
	if e := json.Unmarshal(data, &value); e != nil {
		return e
	}
	var fields map[string]json.RawMessage
	if e := json.Unmarshal(data, &fields); e != nil {
		return e
	}
	known, _ := json.Marshal(value)
	var keys map[string]json.RawMessage
	json.Unmarshal(known, &keys)
	for k := range keys {
		delete(fields, k)
	}
	*i = IconMeta(value)
	i.Extra = fields
	return nil
}
func (i IconMeta) MarshalJSON() ([]byte, error) {
	type plain IconMeta
	raw, e := json.Marshal(plain(i))
	if e != nil {
		return nil, e
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	for k, v := range i.Extra {
		if _, exists := fields[k]; !exists {
			fields[k] = v
		}
	}
	return json.Marshal(fields)
}

type GroupMeta struct {
	ID           int64      `json:"id,omitempty"`
	RedirectFrom []int64    `json:"redirect_from,omitempty"`
	Published    *bool      `json:"published,omitempty"`
	Deleted      bool       `json:"deleted,omitempty"`
	Key          string     `json:"key,omitempty"`
	Title        string     `json:"title"`
	URL          string     `json:"url"`
	Domains      []string   `json:"domains"`
	Description  string     `json:"description"`
	Category     string     `json:"category"`
	Tags         []string   `json:"tags"`
	Aliases      []string   `json:"aliases"`
	Notes        []string   `json:"notes"`
	Icons        []IconMeta `json:"icons"`
	Next         int        `json:"next_version"`
}
type BrandMeta struct {
	ID     int64  `json:"collection_id,omitempty"`
	Schema int    `json:"schema_version"`
	Parent string `json:"parent_brand,omitempty"`
	GroupMeta
	Products []GroupMeta `json:"products"`
}
type Brand struct {
	ID                 int64
	Slug, Parent       string
	Meta               BrandMeta
	Published, Deleted bool
	Count              int
}
type Group struct {
	ID, BrandID        int64
	Brand, Key         string
	Meta               GroupMeta
	Published, Deleted bool
	Count, Downloads   int
	Icon               *Icon
	Updated            string
}
type Icon struct {
	ID, GroupID    int64
	Brand, Product string
	Meta           IconMeta
	Deleted        bool
	Downloads      int
	SVG            []byte
}
type Store struct {
	db      *sql.DB
	library *Library
	writeMu sync.Mutex
	viewMu  sync.RWMutex
}
type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	Exec(string, ...any) (sql.Result, error)
}

var slugRE = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)

func validSlug(s string) bool {
	return len(s) > 0 && len(s) <= 80 && slugRE.MatchString(s) && !reservedName(s)
}
func stamp() string          { return time.Now().UTC().Format(time.RFC3339) }
func jsonText(v any) string  { b, _ := json.Marshal(v); return string(b) }
func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func filename(brand, key string, n int) string {
	s := brand
	if key != "" {
		s += "-" + key
	}
	if n > 0 {
		s += fmt.Sprintf("-%d", n)
	}
	return s + ".svg"
}
func cleanGroup(m GroupMeta) GroupMeta {
	m.Icons = []IconMeta{}
	if m.Domains == nil {
		m.Domains = []string{}
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	if m.Aliases == nil {
		m.Aliases = []string{}
	}
	if m.Notes == nil {
		m.Notes = []string{}
	}
	return m
}

// newIndex creates the rebuildable catalogue in memory. Persistent state is
// attached separately by openLibraryStore and never contains catalogue tables.
func newIndex(library fs.FS) (*Store, error) {
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	ok := false
	defer func() {
		if !ok {
			db.Close()
		}
	}()
	if _, e = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=10000; PRAGMA temp_store=MEMORY;
 CREATE TABLE IF NOT EXISTS categories(name TEXT PRIMARY KEY);
 CREATE TABLE IF NOT EXISTS brands(id INTEGER PRIMARY KEY,slug TEXT UNIQUE NOT NULL,parent TEXT REFERENCES brands(slug),metadata TEXT NOT NULL,published INTEGER NOT NULL DEFAULT 1,deleted INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS groups(id INTEGER PRIMARY KEY,brand_id INTEGER NOT NULL REFERENCES brands(id),product_key TEXT NOT NULL DEFAULT '',metadata TEXT NOT NULL,next_version INTEGER NOT NULL DEFAULT 0,published INTEGER NOT NULL DEFAULT 1,deleted INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL,UNIQUE(brand_id,product_key),UNIQUE(id,brand_id));
 CREATE TABLE IF NOT EXISTS icons(id INTEGER PRIMARY KEY,brand_id INTEGER NOT NULL REFERENCES brands(id),group_id INTEGER NOT NULL,filename TEXT NOT NULL COLLATE NOCASE,version INTEGER NOT NULL,metadata TEXT NOT NULL,hash TEXT NOT NULL,svg BLOB NOT NULL,is_default INTEGER NOT NULL CHECK(is_default IN(0,1)),deleted INTEGER NOT NULL DEFAULT 0,downloads INTEGER NOT NULL DEFAULT 0,FOREIGN KEY(group_id,brand_id) REFERENCES groups(id,brand_id),UNIQUE(brand_id,filename),UNIQUE(group_id,version));
 CREATE UNIQUE INDEX IF NOT EXISTS group_default ON icons(group_id) WHERE is_default=1 AND deleted=0;
 CREATE INDEX IF NOT EXISTS icon_group ON icons(group_id,deleted,is_default);
`); e != nil {
		return nil, e
	}
	tx, e := db.Begin()
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	if e = loadCatalog(tx, library); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(`INSERT OR IGNORE INTO categories VALUES('其他');
 INSERT OR IGNORE INTO categories SELECT DISTINCT json_extract(metadata,'$.category') FROM groups WHERE COALESCE(json_extract(metadata,'$.category'),'')<>'';`); e != nil {
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	ok = true
	return s, nil
}

func loadCatalog(tx *sql.Tx, library fs.FS) error {
	dirs, err := fs.ReadDir(library, ".")
	if err != nil {
		return err
	}
	type catalogBrand struct {
		slug string
		meta BrandMeta
	}
	items := []catalogBrand{}
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		if !validSlug(dir.Name()) {
			return fmt.Errorf("无效合集目录: %s", dir.Name())
		}
		raw, err := fs.ReadFile(library, path.Join(dir.Name(), "meta.json"))
		if err != nil {
			return err
		}
		var m BrandMeta
		if err = json.Unmarshal(raw, &m); err != nil {
			return err
		}
		if m.Schema != 1 || strings.TrimSpace(m.Title) == "" {
			return fmt.Errorf("无效元数据: %s", dir.Name())
		}
		items = append(items, catalogBrand{dir.Name(), m})
	}
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		m := item.meta
		m.GroupMeta = cleanGroup(m.GroupMeta)
		m.Products = []GroupMeta{}
		m.Parent = ""
		if _, err = tx.Exec(`INSERT INTO brands(id,slug,metadata,published,deleted,updated_at) VALUES(?,?,?,?,?,?)`, recordID(item.meta.ID), item.slug, jsonText(m), isPublished(item.meta.Published), item.meta.Deleted, stamp()); err != nil {
			return err
		}
	}
	parents := map[string]string{}
	for _, item := range items {
		parents[item.slug] = item.meta.Parent
	}
	for _, item := range items {
		seen := map[string]bool{item.slug: true}
		for p := item.meta.Parent; p != ""; p = parents[p] {
			if _, ok := parents[p]; !ok || seen[p] {
				return fmt.Errorf("上级合集缺失或循环: %s", item.slug)
			}
			seen[p] = true
		}
		if item.meta.Parent != "" {
			if _, err = tx.Exec(`UPDATE brands SET parent=? WHERE slug=?`, item.meta.Parent, item.slug); err != nil {
				return err
			}
		}
		var bid int64
		if err = tx.QueryRow(`SELECT id FROM brands WHERE slug=?`, item.slug).Scan(&bid); err != nil {
			return err
		}
		groups := append([]GroupMeta{item.meta.GroupMeta}, item.meta.Products...)
		groups[0].Key = ""
		groups[0].Published = nil
		groups[0].Deleted = false
		for _, g := range groups {
			if g.Key != "" && !validSlug(g.Key) {
				return errors.New("产品标识无效")
			}
			meta := cleanGroup(g)
			res, err := tx.Exec(`INSERT INTO groups(id,brand_id,product_key,metadata,next_version,published,deleted,updated_at) VALUES(?,?,?,?,?,?,?,?)`, recordID(g.ID), bid, g.Key, jsonText(meta), g.Next, isPublished(g.Published), g.Deleted, stamp())
			if err != nil {
				return err
			}
			gid, _ := res.LastInsertId()
			defaults, maxVersion, active := 0, -1, 0
			for _, i := range g.Icons {
				if i.Version < 0 || i.File != filename(item.slug, g.Key, i.Version) {
					return fmt.Errorf("文件命名不符: %s", i.File)
				}
				content, err := fs.ReadFile(library, path.Join(item.slug, i.File))
				if err != nil {
					return err
				}
				i.SHA, i.Bytes = digest(content), len(content)
				if _, err = validateSVG(content); err != nil {
					return fmt.Errorf("%s: %w", i.File, err)
				}
				if i.Default && !i.Deleted {
					defaults++
				}
				if !i.Deleted {
					active++
				}
				if i.Version > maxVersion {
					maxVersion = i.Version
				}
				if _, err = tx.Exec(`INSERT INTO icons(id,brand_id,group_id,filename,version,metadata,hash,svg,is_default,deleted) VALUES(?,?,?,?,?,?,?,?,?,?)`, recordID(i.ID), bid, gid, i.File, i.Version, jsonText(i), i.SHA, content, i.Default, i.Deleted); err != nil {
					return err
				}
			}
			if (active > 0 && defaults != 1) || g.Next <= maxVersion {
				return fmt.Errorf("默认版本或计数不符: %s/%s", item.slug, g.Key)
			}
		}
	}
	return validateGroupRedirects(tx)
}

// Retired detail IDs share the same namespace as live groups. Reject collisions
// so an old bookmark can never resolve ambiguously or replace another product.
func validateGroupRedirects(q queryer) error {
	rows, err := q.Query(`SELECT id,metadata FROM groups`)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids, aliases := map[int64]bool{}, map[int64]bool{}
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		var meta GroupMeta
		if err := json.Unmarshal([]byte(raw), &meta); err != nil {
			return err
		}
		ids[id] = true
		if len(meta.RedirectFrom) > 1000 {
			return errors.New("旧详情地址过多")
		}
		for _, alias := range meta.RedirectFrom {
			if alias <= 0 || alias > 1<<52 || aliases[alias] {
				return errors.New("旧详情地址 ID 无效或重复")
			}
			aliases[alias] = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for alias := range aliases {
		if ids[alias] {
			return errors.New("旧详情地址与现有产品 ID 冲突")
		}
	}
	return nil
}

func nextGroupID(q queryer) (int64, error) {
	var id int64
	err := q.QueryRow(`SELECT COALESCE(MAX(id),0)+1 FROM (SELECT id FROM groups UNION ALL SELECT CAST(old.value AS INTEGER) FROM groups g JOIN json_each(g.metadata,'$.redirect_from') old)`).Scan(&id)
	if err == nil && id > 1<<52 {
		err = errors.New("产品 ID 超出范围")
	}
	return id, err
}

func (s *Store) transaction(fn func(*sql.Tx) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(tx); err != nil {
		return err
	}
	files, e := librarySnapshot(tx)
	if e != nil {
		return e
	}
	if e = s.library.save(files); e != nil {
		return e
	}
	return tx.Commit()
}
func audit(q queryer, action, target string) error {
	_, err := q.Exec(`INSERT INTO audit(action,target,created_at) VALUES(?,?,?)`, action, target, stamp())
	return err
}
func scanBrand(row interface{ Scan(...any) error }) (Brand, error) {
	var b Brand
	var raw string
	err := row.Scan(&b.ID, &b.Slug, &b.Parent, &raw, &b.Published, &b.Deleted)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &b.Meta)
	}
	b.Meta.Parent = b.Parent
	return b, err
}
func brandByID(q queryer, id int64) (Brand, error) {
	return scanBrand(q.QueryRow(`SELECT id,slug,COALESCE(parent,''),metadata,published,deleted FROM brands WHERE id=?`, id))
}
func (s *Store) brands() ([]Brand, error) {
	rows, err := s.db.Query(`SELECT id,slug,COALESCE(parent,''),metadata,published,deleted FROM brands ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Brand{}
	for rows.Next() {
		b, err := scanBrand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	sortByName(out, func(b Brand) string { return b.Meta.Title })
	return out, rows.Err()
}
func scanGroup(row interface{ Scan(...any) error }) (Group, error) {
	var g Group
	var raw string
	var next int
	err := row.Scan(&g.ID, &g.BrandID, &g.Brand, &g.Key, &raw, &next, &g.Published, &g.Deleted, &g.Updated)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &g.Meta)
	}
	g.Meta.Next = next
	g.Meta.Key = g.Key
	return g, err
}
func groupByID(q queryer, id int64) (Group, error) {
	return scanGroup(q.QueryRow(`SELECT g.id,g.brand_id,b.slug,g.product_key,g.metadata,g.next_version,g.published,g.deleted,g.updated_at FROM groups g JOIN brands b ON b.id=g.brand_id WHERE g.id=?`, id))
}
func (s *Store) groups(admin bool) ([]Group, error) {
	where := `WHERE b.published=1 AND b.deleted=0 AND g.published=1 AND g.deleted=0`
	if admin {
		where = ""
	}
	rows, err := s.db.Query(`SELECT g.id,g.brand_id,b.slug,g.product_key,g.metadata,g.next_version,g.published,g.deleted,g.updated_at FROM groups g JOIN brands b ON b.id=g.brand_id ` + where + ` ORDER BY b.slug,g.product_key`)
	if err != nil {
		return nil, err
	}
	out := []Group{}
	for rows.Next() {
		g, e := scanGroup(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	// One metadata query for the whole listing; never read SVG BLOBs here.
	previews, err := s.db.Query(`SELECT i.id,i.group_id,i.metadata,i.deleted,i.downloads,b.slug,g.product_key FROM icons i JOIN groups g ON g.id=i.group_id JOIN brands b ON b.id=i.brand_id ORDER BY i.is_default DESC,i.version`)
	if err != nil {
		return nil, err
	}
	defer previews.Close()
	index := map[int64]int{}
	for n, g := range out {
		index[g.ID] = n
	}
	for previews.Next() {
		i, e := scanIcon(previews)
		if e != nil {
			return nil, e
		}
		if n, ok := index[i.GroupID]; ok && !i.Deleted {
			out[n].Count++
			out[n].Downloads += i.Downloads
			if out[n].Icon == nil {
				copy := i
				out[n].Icon = &copy
			}
		}
	}
	sortByName(out, func(g Group) string { return g.Meta.Title })
	return out, previews.Err()
}
func scanIcon(row interface{ Scan(...any) error }) (Icon, error) {
	var i Icon
	var raw string
	err := row.Scan(&i.ID, &i.GroupID, &raw, &i.Deleted, &i.Downloads, &i.Brand, &i.Product)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &i.Meta)
	}
	return i, err
}
func iconsFor(q queryer, gid int64) ([]Icon, error) {
	rows, err := q.Query(`SELECT i.id,i.group_id,i.metadata,i.deleted,i.downloads,b.slug,g.product_key FROM icons i JOIN brands b ON b.id=i.brand_id JOIN groups g ON g.id=i.group_id WHERE i.group_id=? ORDER BY i.deleted,i.is_default DESC,i.version`, gid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Icon{}
	for rows.Next() {
		i, e := scanIcon(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func iconByID(q queryer, id int64) (Icon, error) {
	i, err := scanIcon(q.QueryRow(`SELECT i.id,i.group_id,i.metadata,i.deleted,i.downloads,b.slug,g.product_key FROM icons i JOIN brands b ON b.id=i.brand_id JOIN groups g ON g.id=i.group_id WHERE i.id=?`, id))
	return i, err
}
func publicIcon(q queryer, id int64) (Icon, error) {
	i, err := iconByID(q, id)
	if err != nil {
		return i, err
	}
	var n int
	err = q.QueryRow(`SELECT COUNT(*) FROM icons i JOIN groups g ON g.id=i.group_id JOIN brands b ON b.id=i.brand_id WHERE i.id=? AND i.deleted=0 AND g.deleted=0 AND g.published=1 AND b.deleted=0 AND b.published=1`, id).Scan(&n)
	if err == nil && n == 0 {
		err = sql.ErrNoRows
	}
	return i, err
}
