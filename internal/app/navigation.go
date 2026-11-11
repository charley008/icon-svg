package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxNavigationLinks = 12

type NavigationLink struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Visible bool   `json:"visible"`
	NewTab  bool   `json:"new_tab"`
}

func (a *App) navigationLinks() ([]NavigationLink, error) {
	var raw string
	err := a.store.db.QueryRow(`SELECT value FROM runtime.settings WHERE key='navigation_links'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var links []NavigationLink
	err = json.Unmarshal([]byte(raw), &links)
	return links, err
}

func validNavigationURL(value string) bool {
	if len(value) > 2048 || strings.ContainsAny(value, "\\") || strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return u.Scheme == "" && u.Host == ""
	}
	return (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}

func (a *App) saveNavigation(w http.ResponseWriter, r *http.Request) {
	titles, addresses := r.PostForm["title"], r.PostForm["url"]
	visible, newTab := r.PostForm["visible"], r.PostForm["new_tab"]
	if len(titles) > maxNavigationLinks || len(titles) != len(addresses) || len(titles) != len(visible) || len(titles) != len(newTab) {
		http.Error(w, "导航表单无效，最多添加 12 条链接", http.StatusBadRequest)
		return
	}
	links := make([]NavigationLink, 0, len(titles))
	for i, title := range titles {
		title, address := strings.TrimSpace(title), strings.TrimSpace(addresses[i])
		if !utf8.ValidString(title) || utf8.RuneCountInString(title) == 0 || utf8.RuneCountInString(title) > 40 || strings.IndexFunc(title, unicode.IsControl) >= 0 || !validNavigationURL(address) || (visible[i] != "0" && visible[i] != "1") || (newTab[i] != "0" && newTab[i] != "1") {
			http.Error(w, fmt.Sprintf("第 %d 条链接无效：名称为 1–40 字，地址须为 http(s) 链接或以 / 开头的站内路径", i+1), http.StatusBadRequest)
			return
		}
		links = append(links, NavigationLink{title, address, visible[i] == "1", newTab[i] == "1"})
	}
	raw, err := json.Marshal(links)
	if err == nil {
		err = a.store.runtimeTransaction(func(tx *sql.Tx) error {
			if _, err := tx.Exec(`INSERT INTO runtime.settings(key,value) VALUES('navigation_links',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw)); err != nil {
				return err
			}
			return audit(tx, "navigation_changed", fmt.Sprintf("%d links", len(links)))
		})
	}
	if err != nil {
		renderError(w, err)
		return
	}
	http.Redirect(w, r, "/admin/settings?view=navigation&saved=1", http.StatusSeeOther)
}
