package app

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"modernc.org/sqlite"
)

const catalogVersion = 2

func openPersistentCatalog(file string) (*Store, error) {
	for attempt := 0; attempt < 2; attempt++ {
		s, err := newIndexFile(libraryInput{snapshotFS(nil)}, file)
		if err == nil {
			var result string
			err = s.db.QueryRow(`PRAGMA quick_check(1)`).Scan(&result)
			if err == nil && result == "ok" {
				return s, nil
			}
			s.db.Close()
			if err == nil {
				err = fmt.Errorf("索引校验失败: %s", result)
			}
		} else {
			var sqliteErr *sqlite.Error
			if !errors.As(err, &sqliteErr) || (sqliteErr.Code()&255 != 11 && sqliteErr.Code()&255 != 26) {
				return nil, err
			}
		}
		if attempt != 0 {
			return nil, err
		}
		// Only the catalogue is disposable. Never remove the runtime database.
		for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
			if removeErr := os.Remove(file + suffix); removeErr != nil && !os.IsNotExist(removeErr) {
				return nil, removeErr
			}
		}
	}
	return nil, errors.New("无法重建素材索引")
}

func init() {
	functions := map[string]func(string) driver.Value{
		"icon_name_group": func(s string) driver.Value { return int64(nameSortKey(s).group) },
		"icon_name_key":   func(s string) driver.Value { return nameSortKey(s).value },
		"icon_search": func(s string) driver.Value {
			var m GroupMeta
			_ = json.Unmarshal([]byte(s), &m)
			return strings.ToLower(m.Title + " " + strings.Join(m.Tags, " ") + " " + strings.Join(m.Aliases, " ") + " " + strings.Join(m.Domains, " "))
		},
		"icon_brand_search": func(s string) driver.Value {
			var m BrandMeta
			_ = json.Unmarshal([]byte(s), &m)
			return strings.ToLower(m.Title + " " + strings.Join(m.Aliases, " ") + " " + strings.Join(m.Domains, " "))
		},
	}
	for name, fn := range functions {
		if err := sqlite.RegisterDeterministicScalarFunction(name, 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			s, _ := args[0].(string)
			return fn(s), nil
		}); err != nil {
			panic(err)
		}
	}
}

// This database is a disposable index. Operational data lives in runtime.sqlite3.
// Generated keys and per-group summaries are updated by SQLite when metadata or
// icons change; reads do not parse or sort the complete library in Go.
const catalogSchema = `
CREATE TABLE IF NOT EXISTS categories(name TEXT PRIMARY KEY);
CREATE TABLE IF NOT EXISTS brands(
 id INTEGER PRIMARY KEY,slug TEXT UNIQUE NOT NULL,parent TEXT REFERENCES brands(slug),
 metadata TEXT NOT NULL,published INTEGER NOT NULL DEFAULT 1,deleted INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL,
 name_group INTEGER GENERATED ALWAYS AS (icon_name_group(json_extract(metadata,'$.title'))) STORED,
 name_key TEXT GENERATED ALWAYS AS (icon_name_key(json_extract(metadata,'$.title'))) STORED,
 search_text TEXT GENERATED ALWAYS AS (icon_brand_search(metadata)) STORED);
CREATE TABLE IF NOT EXISTS groups(
 id INTEGER PRIMARY KEY,brand_id INTEGER NOT NULL REFERENCES brands(id),product_key TEXT NOT NULL DEFAULT '',
 metadata TEXT NOT NULL,next_version INTEGER NOT NULL DEFAULT 0,published INTEGER NOT NULL DEFAULT 1,deleted INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL,
 name_group INTEGER GENERATED ALWAYS AS (icon_name_group(json_extract(metadata,'$.title'))) STORED,
 name_key TEXT GENERATED ALWAYS AS (icon_name_key(json_extract(metadata,'$.title'))) STORED,
 search_text TEXT GENERATED ALWAYS AS (icon_search(metadata)) STORED,
 category TEXT GENERATED ALWAYS AS (coalesce(json_extract(metadata,'$.category'),'')) STORED,
 icon_count INTEGER NOT NULL DEFAULT 0,download_count INTEGER NOT NULL DEFAULT 0,preview_id INTEGER,
 UNIQUE(brand_id,product_key),UNIQUE(id,brand_id));
CREATE TABLE IF NOT EXISTS icons(
 id INTEGER PRIMARY KEY,brand_id INTEGER NOT NULL REFERENCES brands(id),group_id INTEGER NOT NULL,
 filename TEXT NOT NULL COLLATE NOCASE,version INTEGER NOT NULL,metadata TEXT NOT NULL,hash TEXT NOT NULL,
 is_default INTEGER NOT NULL CHECK(is_default IN(0,1)),deleted INTEGER NOT NULL DEFAULT 0,downloads INTEGER NOT NULL DEFAULT 0,
 FOREIGN KEY(group_id,brand_id) REFERENCES groups(id,brand_id),UNIQUE(brand_id,filename),UNIQUE(group_id,version));
CREATE UNIQUE INDEX IF NOT EXISTS group_default ON icons(group_id) WHERE is_default=1 AND deleted=0;
CREATE INDEX IF NOT EXISTS icon_group ON icons(group_id,deleted,is_default DESC,version,id);
CREATE INDEX IF NOT EXISTS public_group_order ON groups(published,deleted,name_group,name_key,id);
CREATE INDEX IF NOT EXISTS public_group_category ON groups(published,deleted,category,name_group,name_key,id);
CREATE INDEX IF NOT EXISTS public_groups_by_brand ON groups(brand_id,published,deleted,icon_count,product_key);
CREATE INDEX IF NOT EXISTS public_brand_order ON brands(published,deleted,name_group,name_key,id);
CREATE TABLE IF NOT EXISTS library_files(path TEXT PRIMARY KEY,hash TEXT NOT NULL,size INTEGER NOT NULL,modified INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS library_state(id INTEGER PRIMARY KEY CHECK(id=1),root TEXT NOT NULL,fingerprint TEXT NOT NULL);
CREATE TRIGGER IF NOT EXISTS icon_insert AFTER INSERT ON icons BEGIN
 UPDATE groups SET icon_count=(SELECT count(*) FROM icons WHERE group_id=NEW.group_id AND deleted=0),
 download_count=(SELECT coalesce(sum(downloads),0) FROM icons WHERE group_id=NEW.group_id AND deleted=0),
 preview_id=(SELECT id FROM icons WHERE group_id=NEW.group_id AND deleted=0 ORDER BY is_default DESC,version,id LIMIT 1) WHERE id=NEW.group_id;
END;
CREATE TRIGGER IF NOT EXISTS icon_update AFTER UPDATE ON icons BEGIN
 UPDATE groups SET icon_count=(SELECT count(*) FROM icons WHERE group_id=groups.id AND deleted=0),
 download_count=(SELECT coalesce(sum(downloads),0) FROM icons WHERE group_id=groups.id AND deleted=0),
 preview_id=(SELECT id FROM icons WHERE group_id=groups.id AND deleted=0 ORDER BY is_default DESC,version,id LIMIT 1) WHERE id IN(OLD.group_id,NEW.group_id);
END;
CREATE TRIGGER IF NOT EXISTS icon_delete AFTER DELETE ON icons BEGIN
 UPDATE groups SET icon_count=(SELECT count(*) FROM icons WHERE group_id=OLD.group_id AND deleted=0),
 download_count=(SELECT coalesce(sum(downloads),0) FROM icons WHERE group_id=OLD.group_id AND deleted=0),
 preview_id=(SELECT id FROM icons WHERE group_id=OLD.group_id AND deleted=0 ORDER BY is_default DESC,version,id LIMIT 1) WHERE id=OLD.group_id;
END;
PRAGMA user_version=2;
`
