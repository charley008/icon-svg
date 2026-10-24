package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (a *App) archive(w http.ResponseWriter, r *http.Request, name string, build func(*zip.Writer) error) {
	file, e := os.CreateTemp(filepath.Join(a.data, "cache"), "archive-*.zip")
	if e != nil {
		renderError(w, e)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	z := zip.NewWriter(file)
	e = build(z)
	closeErr := z.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		renderError(w, e)
		return
	}
	if _, e = file.Seek(0, 0); e != nil {
		renderError(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeContent(w, r, name, time.Time{}, file)
}
func zipEntry(z *zip.Writer, name string, content []byte) error {
	f, e := z.Create(name)
	if e != nil {
		return e
	}
	_, e = f.Write(content)
	return e
}
func publicArchive(tx *sql.Tx, z *zip.Writer, ids map[int64]bool) error {
	rows, e := tx.Query(`SELECT i.id,b.slug,i.filename,i.svg FROM icons i JOIN groups g ON g.id=i.group_id JOIN brands b ON b.id=i.brand_id WHERE i.deleted=0 AND g.deleted=0 AND g.published=1 AND b.deleted=0 AND b.published=1 ORDER BY b.slug,i.filename`)
	if e != nil {
		return e
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		var id int64
		var brand, name string
		var content []byte
		if e = rows.Scan(&id, &brand, &name, &content); e != nil {
			return e
		}
		if ids != nil && !ids[id] {
			continue
		}
		if e = zipEntry(z, brand+"/"+name, content); e != nil {
			return e
		}
		found++
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if ids != nil && found != len(ids) {
		return errors.New("部分选择的图标已下架或删除，请刷新重选")
	}
	return nil
}
func (a *App) allDownload(w http.ResponseWriter, r *http.Request) {
	a.archive(w, r, "icon-svg-library.zip", func(z *zip.Writer) error {
		return a.store.transaction(func(tx *sql.Tx) error { return publicArchive(tx, z, nil) })
	})
}
func (a *App) selectionDownload(w http.ResponseWriter, r *http.Request) {
	if e := r.ParseForm(); e != nil {
		renderError(w, e)
		return
	}
	ids := map[int64]bool{}
	for _, value := range strings.Split(r.FormValue("ids"), ",") {
		id, e := strconv.ParseInt(value, 10, 64)
		if e != nil || id <= 0 {
			http.Error(w, "选择无效", 400)
			return
		}
		ids[id] = true
	}
	if len(ids) == 0 || len(ids) > 50 {
		http.Error(w, "一次请选择 1–50 个图标", 400)
		return
	}
	a.archive(w, r, "icon-svg-selection.zip", func(z *zip.Writer) error {
		return a.store.transaction(func(tx *sql.Tx) error { return publicArchive(tx, z, ids) })
	})
}
func (a *App) exportLibrary(w http.ResponseWriter, r *http.Request) {
	prefix := "library/"
	a.archive(w, r, "icon-svg-library.zip", func(z *zip.Writer) error {
		return a.store.transaction(func(tx *sql.Tx) error {
			rows, e := tx.Query(`SELECT id,slug,COALESCE(parent,''),metadata,published,deleted FROM brands WHERE deleted=0 AND published=1 ORDER BY slug`)
			if e != nil {
				return e
			}
			brands := []Brand{}
			for rows.Next() {
				b, e := scanBrand(rows)
				if e != nil {
					rows.Close()
					return e
				}
				brands = append(brands, b)
			}
			if e = rows.Err(); e != nil {
				rows.Close()
				return e
			}
			rows.Close()
			included := map[string]bool{}
			for _, b := range brands {
				included[b.Slug] = true
			}
			for _, b := range brands {
				m := b.Meta
				m.Schema = 1
				m.Published = nil
				m.Deleted = false
				m.Parent = b.Parent
				if !included[m.Parent] {
					m.Parent = ""
				}
				m.Icons = []IconMeta{}
				m.Products = []GroupMeta{}
				grow, e := tx.Query(`SELECT g.id,g.brand_id,b.slug,g.product_key,g.metadata,g.next_version,g.published,g.deleted,g.updated_at FROM groups g JOIN brands b ON b.id=g.brand_id WHERE g.brand_id=? AND g.deleted=0 AND g.published=1 ORDER BY g.product_key`, b.ID)
				if e != nil {
					return e
				}
				groups := []Group{}
				for grow.Next() {
					g, e := scanGroup(grow)
					if e != nil {
						grow.Close()
						return e
					}
					groups = append(groups, g)
				}
				e = grow.Err()
				grow.Close()
				if e != nil {
					return e
				}
				for _, g := range groups {
					gm := cleanGroup(g.Meta)
					gm.Key = g.Key
					gm.Published = nil
					gm.Deleted = false
					icons, e := iconsFor(tx, g.ID)
					if e != nil {
						return e
					}
					for _, i := range icons {
						if i.Deleted {
							continue
						}
						var content []byte
						if e = tx.QueryRow(`SELECT svg FROM icons WHERE id=?`, i.ID).Scan(&content); e != nil {
							return e
						}
						gm.Icons = append(gm.Icons, i.Meta)
						if e = zipEntry(z, prefix+b.Slug+"/"+i.Meta.File, content); e != nil {
							return e
						}
					}
					if g.Key == "" {
						m.GroupMeta = gm
					} else {
						m.Products = append(m.Products, gm)
					}
				}
				content, e := json.MarshalIndent(m, "", "  ")
				if e != nil {
					return e
				}
				if e = zipEntry(z, prefix+b.Slug+"/meta.json", append(content, '\n')); e != nil {
					return e
				}
			}
			notices, e := fs.ReadDir(a.files.Library, "licenses")
			if e != nil {
				return e
			}
			for _, notice := range notices {
				if notice.IsDir() {
					continue
				}
				content, e := fs.ReadFile(a.files.Library, "licenses/"+notice.Name())
				if e != nil {
					return e
				}
				if e = zipEntry(z, "library/licenses/"+notice.Name(), content); e != nil {
					return e
				}
			}
			return zipEntry(z, "README.txt", []byte(fmt.Sprintln("icon-svg 素材导出：仅含当前上架且未删除的合集、产品和图标。完整恢复请使用完整备份。")))
		})
	})
}
