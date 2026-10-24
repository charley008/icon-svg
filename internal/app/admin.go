package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const cookieName = "icon_svg_session"

type csrfKey struct{}

func csrf(r *http.Request) string { s, _ := r.Context().Value(csrfKey{}).(string); return s }
func (a *App) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie(cookieName)
		if e != nil {
			http.Redirect(w, r, "/admin/login", 303)
			return
		}
		var token string
		if e = a.store.db.QueryRow(`SELECT csrf FROM sessions WHERE token=? AND expires>?`, digest([]byte(c.Value)), time.Now().Unix()).Scan(&token); e != nil {
			http.Redirect(w, r, "/admin/login", 303)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				e = r.ParseMultipartForm(maxSVG + 128*1024)
				if r.MultipartForm != nil {
					defer r.MultipartForm.RemoveAll()
				}
			} else {
				e = r.ParseForm()
			}
			if e != nil {
				http.Error(w, "表单或文件过大", 400)
				return
			}
			if !equalToken(r.FormValue("csrf"), token) {
				http.Error(w, "会话校验失败，请刷新重试", 403)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), csrfKey{}, token)))
	})
}
func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "login.html", struct{ Error string }{""})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if e := r.ParseForm(); e != nil {
		http.Error(w, "表单无效", 400)
		return
	}
	username, password := r.FormValue("username"), r.FormValue("password")
	if len(username) > 100 || len(password) > 256 {
		http.Error(w, "输入过长", 400)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now().Unix()
	var allowed bool
	var session, token string
	e := a.store.transaction(func(tx *sql.Tx) error {
		if _, e := tx.Exec(`DELETE FROM attempts WHERE created<?`, now-900); e != nil {
			return e
		}
		var count int
		if e := tx.QueryRow(`SELECT COUNT(*) FROM attempts WHERE ip=?`, ip).Scan(&count); e != nil {
			return e
		}
		if count >= 8 {
			return errors.New("尝试过于频繁，请 15 分钟后重试")
		}
		encoded := "pbkdf2:600000:invalid:invalid"
		err := tx.QueryRow(`SELECT password FROM admins WHERE username=?`, username).Scan(&encoded)
		allowed = passwordMatches(password, encoded) && err == nil
		if !allowed {
			_, e := tx.Exec(`INSERT INTO attempts VALUES(?,?)`, ip, now)
			return e
		}
		if _, e := tx.Exec(`DELETE FROM attempts WHERE ip=?`, ip); e != nil {
			return e
		}
		if _, e := tx.Exec(`DELETE FROM sessions WHERE expires<?`, now); e != nil {
			return e
		}
		session = randomToken()
		token = randomToken()
		_, e := tx.Exec(`INSERT INTO sessions VALUES(?,?,?)`, digest([]byte(session)), token, now+43200)
		return e
	})
	if e != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(429)
		a.render(w, "login.html", struct{ Error string }{e.Error()})
		return
	}
	if !allowed {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(401)
		a.render(w, "login.html", struct{ Error string }{"用户名或密码不正确"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: session, Path: "/admin", MaxAge: 43200, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/admin", 303)
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie(cookieName)
	if c != nil {
		a.store.db.Exec(`DELETE FROM sessions WHERE token=?`, digest([]byte(c.Value)))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/admin", MaxAge: -1, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/admin/login", 303)
}
func (a *App) changePassword(w http.ResponseWriter, r *http.Request) {
	p := r.FormValue("password")
	if len(p) < 8 || len(p) > 256 {
		http.Error(w, "新密码需要 8–256 位", 400)
		return
	}
	if p != r.FormValue("password_confirm") {
		http.Error(w, "两次输入的新密码不一致", 400)
		return
	}
	e := a.store.transaction(func(tx *sql.Tx) error {
		var username, encoded string
		if e := tx.QueryRow(`SELECT username,password FROM admins LIMIT 1`).Scan(&username, &encoded); e != nil {
			return e
		}
		if !passwordMatches(r.FormValue("current"), encoded) {
			return errors.New("当前密码错误")
		}
		if _, e := tx.Exec(`UPDATE admins SET password=? WHERE username=?`, passwordHash(p), username); e != nil {
			return e
		}
		if _, e := tx.Exec(`DELETE FROM sessions`); e != nil {
			return e
		}
		return audit(tx, "password_changed", username)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	os.Remove(filepath.Join(a.data, "initial-admin.txt"))
	a.logout(w, r)
}

type AuditRow struct{ Action, Target, Time string }
type AdminHome struct {
	EmptyMeta          GroupMeta
	CSRF, Q, State     string
	Categories         []string
	Brands             []Brand
	BrandPreviews      map[int64][]Group
	Results            []AdminSearchResult
	Total, Page, Pages int
	Links              []PageLink
}

func (a *App) adminHome(w http.ResponseWriter, r *http.Request) {
	brands, e := a.store.brands()
	if e != nil {
		renderError(w, e)
		return
	}
	groups, e := a.store.groups(true)
	if e != nil {
		renderError(w, e)
		return
	}
	counts := map[int64]int{}
	for _, g := range groups {
		if !g.Deleted {
			counts[g.BrandID] += g.Count
		}
	}
	p := AdminHome{CSRF: csrf(r), Q: strings.TrimSpace(r.URL.Query().Get("q")), State: r.URL.Query().Get("state")}
	p.Categories, e = a.store.categoryNames()
	if e != nil {
		renderError(w, e)
		return
	}
	all := []Brand{}
	for _, b := range brands {
		b.Count = counts[b.ID]
		if (p.State == "trash") != b.Deleted {
			continue
		}
		all = append(all, b)
	}
	p.Total = len(all)
	if p.Q != "" {
		p.Results, e = a.store.searchAdminGroups(brands, groups, p.Q, p.State)
		if e != nil {
			renderError(w, e)
			return
		}
		p.Total = len(p.Results)
	}
	p.Pages = max(1, (p.Total+29)/30)
	p.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	p.Page = max(1, min(p.Page, p.Pages))
	start := (p.Page - 1) * 30
	if p.Q != "" {
		p.Results = p.Results[start:min(start+30, len(p.Results))]
	} else {
		p.Brands = all[start:min(start+30, len(all))]
		p.BrandPreviews = adminBrandPreviews(groups)
	}
	p.Links = paginationLinksAt("/admin", p.Page, p.Pages, url.Values{"q": {p.Q}, "state": {p.State}})
	a.render(w, "admin.html", p)
}

type AdminSettings struct {
	Scan               LibraryScanStatus
	ScanSchedule       string
	ScanMinutes        int
	ScanPreviews       []Icon
	CSRF, View         string
	Categories         []string
	ManagedCategories  []ManagedCategory
	Audit              []AuditRow
	Total, Page, Pages int
	Links              []PageLink
}

func (a *App) adminSettings(w http.ResponseWriter, r *http.Request) {
	p := AdminSettings{CSRF: csrf(r), View: r.URL.Query().Get("view")}
	if p.View != "data" && p.View != "account" && p.View != "logs" && p.View != "categories" && p.View != "library" {
		p.View = ""
	}
	if p.View == "library" {
		p.Scan = a.store.library.scanStatus()
		interval := time.Duration(a.scanInterval.Load())
		p.ScanSchedule = scanSchedule(interval)
		p.ScanMinutes = int(interval / time.Minute)
		if interval%time.Minute != 0 {
			p.ScanMinutes++
		}
		for _, filename := range p.Scan.RecentAdded {
			brand, file, ok := strings.Cut(filename, "/")
			if !ok {
				continue
			}
			var id int64
			if err := a.store.db.QueryRow(`SELECT i.id FROM icons i JOIN brands b ON b.id=i.brand_id WHERE b.slug=? AND i.filename=? AND i.deleted=0`, brand, file).Scan(&id); err != nil {
				continue
			}
			if icon, err := iconByID(a.store.db, id); err == nil {
				p.ScanPreviews = append(p.ScanPreviews, icon)
			}
		}
	}
	if p.View == "categories" {
		var e error
		p.Categories, e = a.store.categoryNames()
		if e != nil {
			renderError(w, e)
			return
		}
		for _, name := range p.Categories {
			var count int
			if e = a.store.db.QueryRow(`SELECT COUNT(*) FROM groups WHERE json_extract(metadata,'$.category')=?`, name).Scan(&count); e != nil {
				renderError(w, e)
				return
			}
			p.ManagedCategories = append(p.ManagedCategories, ManagedCategory{name, count})
		}
	}
	if p.View != "logs" {
		a.render(w, "settings.html", p)
		return
	}
	if e := a.store.db.QueryRow(`SELECT COUNT(*) FROM audit`).Scan(&p.Total); e != nil {
		renderError(w, e)
		return
	}
	p.Pages = max(1, (p.Total+29)/30)
	p.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	p.Page = max(1, min(p.Page, p.Pages))
	rows, e := a.store.db.Query(`SELECT action,target,created_at FROM audit ORDER BY id DESC LIMIT 30 OFFSET ?`, (p.Page-1)*30)
	if e != nil {
		renderError(w, e)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var row AuditRow
		if e = rows.Scan(&row.Action, &row.Target, &row.Time); e != nil {
			renderError(w, e)
			return
		}
		p.Audit = append(p.Audit, row)
	}
	if e = rows.Err(); e != nil {
		renderError(w, e)
		return
	}
	p.Links = paginationLinksAt("/admin/settings", p.Page, p.Pages, url.Values{"view": {"logs"}})
	a.render(w, "settings.html", p)
}

func adminBrandPreviews(groups []Group) map[int64][]Group {
	previews := map[int64][]Group{}
	// Keep the main brand mark first, followed by up to two different products.
	for _, g := range groups {
		if g.Key == "" && !g.Deleted && g.Icon != nil {
			previews[g.BrandID] = append(previews[g.BrandID], g)
		}
	}
	for _, g := range groups {
		if g.Key != "" && !g.Deleted && g.Icon != nil && len(previews[g.BrandID]) < 3 {
			previews[g.BrandID] = append(previews[g.BrandID], g)
		}
	}
	return previews
}

type ManagedGroup struct {
	Group
	Icons []Icon
}
type AdminBrand struct {
	EmptyMeta  GroupMeta
	CSRF       string
	Categories []string
	Brand      Brand
	Brands     []Brand
	Groups     []ManagedGroup
	Children   []Brand
}

func (a *App) adminBrand(w http.ResponseWriter, r *http.Request) {
	b, e := brandByID(a.store.db, requestID(r))
	if e != nil {
		renderError(w, e)
		return
	}
	p := AdminBrand{CSRF: csrf(r), Brand: b}
	p.Categories, e = a.store.categoryNames()
	if e != nil {
		renderError(w, e)
		return
	}
	p.Brands, e = a.store.brands()
	if e != nil {
		renderError(w, e)
		return
	}
	groups, e := a.store.groups(true)
	if e != nil {
		renderError(w, e)
		return
	}
	for _, g := range groups {
		if g.BrandID == b.ID {
			icons, e := iconsFor(a.store.db, g.ID)
			if e != nil {
				renderError(w, e)
				return
			}
			p.Groups = append(p.Groups, ManagedGroup{g, icons})
		}
	}
	for _, child := range p.Brands {
		if child.Parent == b.Slug {
			p.Children = append(p.Children, child)
		}
	}
	a.render(w, "brand.html", p)
}
func listField(s string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range strings.FieldsFunc(s, func(c rune) bool { return c == ',' || c == '，' || c == '\n' }) {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
func formMeta(r *http.Request, previous GroupMeta) GroupMeta {
	previous.Title = strings.TrimSpace(r.FormValue("title"))
	previous.URL = strings.TrimSpace(r.FormValue("url"))
	previous.Description = strings.TrimSpace(r.FormValue("description"))
	previous.Category = strings.TrimSpace(r.FormValue("category"))
	if previous.Category == "" {
		previous.Category = "其他"
	}
	previous.Tags = listField(r.FormValue("tags"))
	previous.Domains = listField(r.FormValue("domains"))
	return cleanGroup(previous)
}
func redirectBrand(w http.ResponseWriter, r *http.Request, id int64) {
	http.Redirect(w, r, fmt.Sprintf("/admin/brands/%d", id), 303)
}
func (a *App) createBrand(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.FormValue("slug"))
	m := formMeta(r, GroupMeta{})
	if !validSlug(slug) {
		http.Error(w, "标识需要小写字母、数字或连字符/点，最长 80 字符", 400)
		return
	}
	if e := validateMeta(m); e != nil {
		renderError(w, e)
		return
	}
	parent := r.FormValue("parent_brand")
	var id int64
	e := a.store.transaction(func(tx *sql.Tx) error {
		var parentArg any
		if e := validateCategory(tx, m.Category); e != nil {
			return e
		}
		if parent != "" {
			if parent == slug {
				return errors.New("合集不能以自身作为上级合集")
			}
			var exists int
			if e := tx.QueryRow(`SELECT COUNT(*) FROM brands WHERE slug=?`, parent).Scan(&exists); e != nil {
				return e
			}
			if exists != 1 {
				return errors.New("上级合集不存在")
			}
			parentArg = parent
		}
		b := BrandMeta{Schema: 1, GroupMeta: m, Products: []GroupMeta{}}
		res, e := tx.Exec(`INSERT INTO brands(slug,parent,metadata,published,updated_at) VALUES(?,?,?,?,?)`, slug, parentArg, jsonText(b), false, stamp())
		if e != nil {
			return errors.New("合集标识已存在，或上级合集不存在")
		}
		id, _ = res.LastInsertId()
		if _, e = tx.Exec(`INSERT INTO groups(brand_id,metadata,published,updated_at) VALUES(?,?,?,?)`, id, jsonText(m), true, stamp()); e != nil {
			return e
		}
		return audit(tx, "brand_created", slug)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	redirectBrand(w, r, id)
}
func (a *App) editBrand(w http.ResponseWriter, r *http.Request) {
	id := requestID(r)
	e := a.store.transaction(func(tx *sql.Tx) error {
		b, e := brandByID(tx, id)
		if e != nil {
			return e
		}
		m := formMeta(r, b.Meta.GroupMeta)
		if e = validateCategory(tx, m.Category); e != nil {
			return e
		}
		if e = validateMeta(m); e != nil {
			return e
		}
		parent := r.FormValue("parent_brand")
		seen := map[string]bool{b.Slug: true}
		for p := parent; p != ""; {
			if seen[p] {
				return errors.New("上级合集不能形成循环")
			}
			seen[p] = true
			var next string
			if e = tx.QueryRow(`SELECT COALESCE(parent,'') FROM brands WHERE slug=?`, p).Scan(&next); e != nil {
				return errors.New("上级合集不存在")
			}
			p = next
		}
		var parentArg any
		if parent != "" {
			parentArg = parent
		}
		b.Meta.GroupMeta = m
		published := r.FormValue("published") == "on"
		if _, e = tx.Exec(`UPDATE brands SET parent=?,metadata=?,published=?,updated_at=? WHERE id=?`, parentArg, jsonText(b.Meta), published, stamp(), id); e != nil {
			return e
		}
		if _, e = tx.Exec(`UPDATE groups SET metadata=?,updated_at=? WHERE brand_id=? AND product_key=''`, jsonText(m), stamp(), id); e != nil {
			return e
		}
		return audit(tx, "brand_updated", b.Slug)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	redirectBrand(w, r, id)
}
func (a *App) brandAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if action != "trash" && action != "restore" {
		http.NotFound(w, r)
		return
	}
	id := requestID(r)
	e := a.store.transaction(func(tx *sql.Tx) error {
		b, e := brandByID(tx, id)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(`UPDATE brands SET deleted=?,updated_at=? WHERE id=?`, action == "trash", stamp(), id); e != nil {
			return e
		}
		return audit(tx, "brand_"+action, b.Slug)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	http.Redirect(w, r, "/admin", 303)
}
func (a *App) createProduct(w http.ResponseWriter, r *http.Request) {
	bid := requestID(r)
	m := formMeta(r, GroupMeta{})
	m.Key = r.FormValue("key")
	if !validSlug(m.Key) {
		http.Error(w, "产品标识无效", 400)
		return
	}
	if e := validateMeta(m); e != nil {
		renderError(w, e)
		return
	}
	e := a.store.transaction(func(tx *sql.Tx) error {
		if e := validateCategory(tx, m.Category); e != nil {
			return e
		}
		if _, e := brandByID(tx, bid); e != nil {
			return e
		}
		if _, e := tx.Exec(`INSERT INTO groups(brand_id,product_key,metadata,published,updated_at) VALUES(?,?,?,?,?)`, bid, m.Key, jsonText(m), true, stamp()); e != nil {
			return errors.New("产品标识已存在，包括回收站记录")
		}
		return audit(tx, "product_created", m.Key)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	redirectBrand(w, r, bid)
}
func (a *App) editGroup(w http.ResponseWriter, r *http.Request) {
	var bid int64
	e := a.store.transaction(func(tx *sql.Tx) error {
		g, e := groupByID(tx, requestID(r))
		if e != nil {
			return e
		}
		bid = g.BrandID
		if g.Key == "" {
			return errors.New("请在合集资料中编辑合集自身信息")
		}
		m := formMeta(r, g.Meta)
		if e = validateCategory(tx, m.Category); e != nil {
			return e
		}
		if e = validateMeta(m); e != nil {
			return e
		}
		if _, e = tx.Exec(`UPDATE groups SET metadata=?,published=?,updated_at=? WHERE id=?`, jsonText(m), r.FormValue("published") == "on", stamp(), g.ID); e != nil {
			return e
		}
		return audit(tx, "product_updated", g.Brand+"/"+g.Key)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	redirectBrand(w, r, bid)
}
func (a *App) groupAction(w http.ResponseWriter, r *http.Request) {
	action := r.PathValue("action")
	if action != "trash" && action != "restore" {
		http.NotFound(w, r)
		return
	}
	var bid int64
	e := a.store.transaction(func(tx *sql.Tx) error {
		g, e := groupByID(tx, requestID(r))
		if e != nil {
			return e
		}
		if g.Key == "" {
			return errors.New("主图标不能作为产品移除")
		}
		bid = g.BrandID
		if _, e = tx.Exec(`UPDATE groups SET deleted=?,updated_at=? WHERE id=?`, action == "trash", stamp(), g.ID); e != nil {
			return e
		}
		return audit(tx, "product_"+action, g.Brand+"/"+g.Key)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	redirectBrand(w, r, bid)
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	file, header, e := r.FormFile("file")
	if e != nil {
		http.Error(w, "请选择 SVG", 400)
		return
	}
	defer file.Close()
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".svg") {
		http.Error(w, "只接受 SVG 文件", 400)
		return
	}
	content, e := io.ReadAll(io.LimitReader(file, maxSVG+1))
	if e != nil {
		renderError(w, e)
		return
	}
	content, e = prepareUploadedSVG(content)
	if e != nil {
		renderError(w, e)
		return
	}
	var bid int64
	e = a.store.transaction(func(tx *sql.Tx) error {
		g, e := groupByID(tx, requestID(r))
		if e != nil {
			return e
		}
		bid = g.BrandID
		if g.Deleted {
			return errors.New("请先恢复产品")
		}
		replace, _ := strconv.ParseInt(r.FormValue("replace_id"), 10, 64)
		label := strings.TrimSpace(r.FormValue("label"))
		if len([]rune(label)) > 80 {
			return errors.New("版本说明过长")
		}
		m := IconMeta{Label: label, SHA: digest(content), Bytes: len(content), Source: "管理员上传", License: r.FormValue("license"), Method: "uploaded-svg"}
		if len(m.License) > 1000 {
			return errors.New("许可说明过长")
		}
		if replace > 0 {
			old, e := iconByID(tx, replace)
			if e != nil {
				return e
			}
			if old.GroupID != g.ID || old.Deleted {
				return errors.New("替换版本不属于当前分组")
			}
			if label == "" {
				m.Label = old.Meta.Label
			}
			m.File = old.Meta.File
			m.Version = old.Meta.Version
			m.Default = old.Meta.Default
			m.Provenance = old.Meta.Provenance
			if _, e = tx.Exec(`UPDATE icons SET metadata=?,hash=?,svg=? WHERE id=?`, jsonText(m), m.SHA, content, replace); e != nil {
				return e
			}
		} else {
			var count int
			if e = tx.QueryRow(`SELECT COUNT(*) FROM icons WHERE group_id=? AND deleted=0`, g.ID).Scan(&count); e != nil {
				return e
			}
			if count >= 10 {
				return errors.New("每组最多 10 个活动版本，请替换或移除旧版本")
			}
			m.File = filename(g.Brand, g.Key, g.Meta.Next)
			m.Version = g.Meta.Next
			m.Default = count == 0
			if m.Label == "" {
				m.Label = strings.TrimSuffix(m.File, ".svg")
			}
			if _, e = tx.Exec(`INSERT INTO icons(brand_id,group_id,filename,version,metadata,hash,svg,is_default) VALUES(?,?,?,?,?,?,?,?)`, g.BrandID, g.ID, m.File, m.Version, jsonText(m), m.SHA, content, m.Default); e != nil {
				return errors.New("文件名冲突；请检查其他产品标识是否与版本编号重叠")
			}
			if _, e = tx.Exec(`UPDATE groups SET next_version=next_version+1 WHERE id=?`, g.ID); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(`UPDATE groups SET updated_at=? WHERE id=?`, stamp(), g.ID); e != nil {
			return e
		}
		return audit(tx, "icon_uploaded", g.Brand+"/"+g.Key)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	redirectBrand(w, r, bid)
}
func setDefault(q queryer, gid, id int64) error {
	if _, e := q.Exec(`UPDATE icons SET is_default=0,metadata=json_set(metadata,'$.is_default',json('false')) WHERE group_id=?`, gid); e != nil {
		return e
	}
	_, e := q.Exec(`UPDATE icons SET is_default=1,metadata=json_set(metadata,'$.is_default',json('true')) WHERE id=? AND deleted=0`, id)
	return e
}
func (a *App) iconAction(w http.ResponseWriter, r *http.Request) {
	var bid int64
	action := r.PathValue("action")
	e := a.store.transaction(func(tx *sql.Tx) error {
		i, e := iconByID(tx, requestID(r))
		if e != nil {
			return e
		}
		g, e := groupByID(tx, i.GroupID)
		if e != nil {
			return e
		}
		bid = g.BrandID
		switch action {
		case "default":
			if i.Deleted {
				return errors.New("请先恢复版本")
			}
			e = setDefault(tx, g.ID, i.ID)
		case "trash":
			_, e = tx.Exec(`UPDATE icons SET deleted=1,is_default=0,metadata=json_set(metadata,'$.is_default',json('false')) WHERE id=?`, i.ID)
			if e == nil && i.Meta.Default {
				var other int64
				if err := tx.QueryRow(`SELECT id FROM icons WHERE group_id=? AND deleted=0 ORDER BY version LIMIT 1`, g.ID).Scan(&other); err == nil {
					e = setDefault(tx, g.ID, other)
				} else if err != sql.ErrNoRows {
					return err
				}
			}
		case "restore":
			if i.Deleted {
				var count int
				if e = tx.QueryRow(`SELECT COUNT(*) FROM icons WHERE group_id=? AND deleted=0`, g.ID).Scan(&count); e != nil {
					return e
				}
				if count >= 10 {
					return errors.New("每组最多 10 个活动版本")
				}
				_, e = tx.Exec(`UPDATE icons SET deleted=0 WHERE id=?`, i.ID)
				if e == nil && count == 0 {
					e = setDefault(tx, g.ID, i.ID)
				}
			}
		case "edit":
			m := i.Meta
			m.Label = strings.TrimSpace(r.FormValue("label"))
			m.Source = r.FormValue("source")
			m.License = r.FormValue("license")
			if len([]rune(m.Label)) > 80 || m.Label == "" || len(m.Source) > 2000 || len(m.License) > 1000 {
				return errors.New("版本说明、来源或许可长度无效")
			}
			_, e = tx.Exec(`UPDATE icons SET metadata=? WHERE id=?`, jsonText(m), i.ID)
		default:
			return errors.New("未知操作")
		}
		if e != nil {
			return e
		}
		if _, e = tx.Exec(`UPDATE groups SET updated_at=? WHERE id=?`, stamp(), g.ID); e != nil {
			return e
		}
		return audit(tx, "icon_"+action, i.Meta.File)
	})
	if e != nil {
		renderError(w, e)
		return
	}
	redirectBrand(w, r, bid)
}
