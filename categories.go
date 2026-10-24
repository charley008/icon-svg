package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

type CategoryField struct {
	GroupMeta
	Categories []string
}

type ManagedCategory struct {
	Name  string
	Count int
}

func (s *Store) categoryNames() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM categories`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	sortByName(names, func(name string) string { return name })
	return names, rows.Err()
}

func validateCategory(q queryer, name string) error {
	var exists int
	if err := q.QueryRow(`SELECT COUNT(*) FROM categories WHERE name=?`, name).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return errors.New("分类不存在，请刷新后从下拉框选择，或先在设置中添加分类")
	}
	return nil
}

func (a *App) categoryAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if action != "create" && action != "rename" && action != "delete" {
		http.NotFound(w, r)
		return
	}
	name, next := strings.TrimSpace(r.FormValue("name")), strings.TrimSpace(r.FormValue("next"))
	valid := func(value string) bool {
		return utf8.RuneCountInString(value) > 0 && utf8.RuneCountInString(value) <= 60 && !strings.ContainsAny(value, "\r\n\t")
	}
	if !valid(name) || (action == "rename" && !valid(next)) {
		http.Error(w, "分类名称需要 1–60 个字符，不能包含换行", 400)
		return
	}
	err := a.store.transaction(func(tx *sql.Tx) error {
		if action == "create" {
			if _, e := tx.Exec(`INSERT INTO categories(name) VALUES(?)`, name); e != nil {
				return errors.New("分类已存在")
			}
			return audit(tx, "category_created", name)
		}
		if name == "其他" {
			return errors.New("“其他”是默认分类，需要保留")
		}
		if e := validateCategory(tx, name); e != nil {
			return e
		}
		if action == "delete" {
			if next == "" {
				next = "其他"
			}
			if next == name {
				return errors.New("请选择其他分类作为迁移目标")
			}
			if e := validateCategory(tx, next); e != nil {
				return e
			}
		} else {
			if next == name {
				return nil
			}
			if _, e := tx.Exec(`INSERT INTO categories(name) VALUES(?)`, next); e != nil {
				return errors.New("目标分类已存在，可通过移除并迁移合并分类")
			}
		}
		for _, table := range []string{"brands", "groups"} {
			if _, e := tx.Exec(`UPDATE `+table+` SET metadata=json_set(metadata,'$.category',?),updated_at=? WHERE json_extract(metadata,'$.category')=?`, next, stamp(), name); e != nil {
				return e
			}
		}
		if _, e := tx.Exec(`DELETE FROM categories WHERE name=?`, name); e != nil {
			return e
		}
		return audit(tx, "category_"+action, fmt.Sprintf("%s → %s", name, next))
	})
	if err != nil {
		renderError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/settings?view=categories", 303)
}
