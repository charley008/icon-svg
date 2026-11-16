package app

import (
	"database/sql"
	"errors"
	"io/fs"
)

// Retain numeric high-water marks, not deleted material. Old URLs and saved
// favourites must never point at a newly created item after physical deletion.
func reserveDeletedIDs(tx *sql.Tx) error {
	for _, table := range []string{"brands", "groups", "icons"} {
		query := `SELECT coalesce(max(id),0) FROM ` + table
		if table == "groups" {
			query = `SELECT coalesce(max(id),0) FROM (SELECT id FROM groups UNION ALL SELECT CAST(value AS INTEGER) FROM groups g,json_each(g.metadata,'$.redirect_from'))`
		}
		if _, err := tx.Exec(`INSERT INTO runtime.settings(key,value) VALUES(?,(`+query+`)) ON CONFLICT(key) DO UPDATE SET value=max(CAST(value AS INTEGER),CAST(excluded.value AS INTEGER))`, "deleted_id_"+table); err != nil {
			return err
		}
	}
	return nil
}

func nextMaterialID(q queryer, table string) (int64, error) {
	if table != "brands" && table != "groups" && table != "icons" {
		return 0, errors.New("无效素材类型")
	}
	var id int64
	err := q.QueryRow(`SELECT max(coalesce((SELECT max(id) FROM `+table+`),0),coalesce((SELECT CAST(value AS INTEGER) FROM runtime.settings WHERE key=?),0))+1`, "deleted_id_"+table).Scan(&id)
	if err == nil && id > 1<<52 {
		err = errors.New("素材 ID 超出范围")
	}
	return id, err
}

func (s *Store) deleteIcon(tx *sql.Tx, i Icon) error {
	name, err := iconPath(i)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM icons WHERE id=?`, i.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM runtime.downloads WHERE key=?`, name); err != nil {
		return err
	}
	s.pendingDeletes[name] = true
	return nil
}

func (s *Store) deleteGroup(tx *sql.Tx, g Group) error {
	icons, err := iconsFor(tx, g.ID)
	if err != nil {
		return err
	}
	for _, i := range icons {
		if err = s.deleteIcon(tx, i); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`DELETE FROM groups WHERE id=?`, g.ID)
	return err
}

func (s *Store) deleteBrand(tx *sql.Tx, b Brand) error {
	// Check every target before changing anything. Shared licences and
	// independently stored child collections are outside this directory.
	s.library.mu.Lock()
	entries, err := fs.ReadDir(s.library.root.FS(), b.Slug)
	if err == nil {
		for _, entry := range entries {
			name := b.Slug + "/" + entry.Name()
			if !libraryTarget(name) || entry.IsDir() {
				err = errors.New("合集目录包含非素材文件，请先移出后再删除合集")
				break
			}
			if err = s.library.check(name); err != nil {
				break
			}
		}
	}
	s.library.mu.Unlock()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE brands SET parent=NULL WHERE parent=?`, b.Slug); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM icons WHERE brand_id=?; DELETE FROM groups WHERE brand_id=?; DELETE FROM brands WHERE id=?`, b.ID, b.ID, b.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM runtime.downloads WHERE instr(key,?)=1`, b.Slug+"/"); err != nil {
		return err
	}
	for _, entry := range entries {
		s.pendingDeletes[b.Slug+"/"+entry.Name()] = true
	}
	return nil
}
