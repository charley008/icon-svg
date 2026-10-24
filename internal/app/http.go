package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type App struct {
	store        *Store
	data, origin string
	secure       bool
	templates    *template.Template
	files        AppResources
	scanInterval atomic.Int64
	scanChanged  chan struct{}
}

type AppResources struct{ Web, Library, Root fs.FS }

func newApp(s *Store, data, origin string, secure bool, resources ...AppResources) (*App, error) {
	files := AppResources{Web: os.DirFS("web"), Library: libraryInput{os.DirFS("library")}, Root: os.DirFS(".")}
	if len(resources) > 0 {
		files = resources[0]
	}
	t, e := template.New("").Funcs(template.FuncMap{"metaFields": func(m GroupMeta, c []string) CategoryField { return CategoryField{m, c} }, "join": strings.Join, "add": func(a, b int) int { return a + b }, "asset": func(value any) string {
		var i Icon
		switch v := value.(type) {
		case Icon:
			i = v
		case *Icon:
			if v == nil {
				return ""
			}
			i = *v
		default:
			return ""
		}
		return fmt.Sprintf("/assets/%d/%s", i.ID, i.Meta.SHA)
	}, "kb": func(n int) string { return fmt.Sprintf("%.1f", float64(n)/1024) }}).ParseFS(files.Web, "templates/*.html")
	if e != nil {
		return nil, fmt.Errorf("读取 web/templates 失败，请解压完整安装包：%w", e)
	}
	a := &App{store: s, data: data, origin: strings.TrimRight(origin, "/"), secure: secure, templates: t, files: files, scanChanged: make(chan struct{}, 1)}
	if err := a.configureScanInterval(5 * time.Minute); err != nil {
		return nil, err
	}
	return a, nil
}
func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(a.files.Web, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("GET /licenses", func(w http.ResponseWriter, r *http.Request) {
		content, _ := fs.ReadFile(a.files.Root, "THIRD_PARTY_NOTICES.txt")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(content)
	})
	mux.HandleFunc("GET /detail/{id}", a.detail)
	mux.HandleFunc("GET /assets/{id}/{hash}", a.asset)
	mux.HandleFunc("GET /download/{id}", a.download)
	mux.HandleFunc("GET /download/all.zip", a.allDownload)
	mux.HandleFunc("POST /download/selection", a.selectionDownload)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if e := a.store.db.Ping(); e != nil {
			http.Error(w, "database unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("GET /admin/login", a.loginPage)
	mux.HandleFunc("POST /admin/login", a.login)
	mux.Handle("GET /admin", a.auth(http.HandlerFunc(a.adminHome)))
	mux.Handle("POST /admin/library/schedule", a.auth(http.HandlerFunc(a.saveScanSchedule)))
	mux.Handle("GET /admin/backup-database", a.auth(http.HandlerFunc(a.backupDatabase)))
	mux.Handle("POST /admin/library/scan", a.auth(http.HandlerFunc(a.scanLibrary)))
	mux.Handle("GET /admin/settings", a.auth(http.HandlerFunc(a.adminSettings)))
	mux.Handle("POST /admin/categories/{action}", a.auth(http.HandlerFunc(a.categoryAction)))
	mux.Handle("GET /admin/brands/{id}", a.auth(http.HandlerFunc(a.adminBrand)))
	mux.Handle("POST /admin/brands", a.auth(http.HandlerFunc(a.createBrand)))
	mux.Handle("POST /admin/brands/{id}", a.auth(http.HandlerFunc(a.editBrand)))
	mux.Handle("POST /admin/brands/{id}/{action}", a.auth(http.HandlerFunc(a.brandAction)))
	mux.Handle("POST /admin/brands/{id}/products", a.auth(http.HandlerFunc(a.createProduct)))
	mux.Handle("POST /admin/groups/{id}", a.auth(http.HandlerFunc(a.editGroup)))
	mux.Handle("POST /admin/groups/{id}/upload", a.auth(http.HandlerFunc(a.upload)))
	mux.Handle("POST /admin/groups/{id}/{action}", a.auth(http.HandlerFunc(a.groupAction)))
	mux.Handle("POST /admin/icons/{id}/{action}", a.auth(http.HandlerFunc(a.iconAction)))
	mux.Handle("GET /admin/preview/{id}", a.auth(http.HandlerFunc(a.adminPreview)))
	mux.Handle("POST /admin/password", a.auth(http.HandlerFunc(a.changePassword)))
	mux.Handle("POST /admin/logout", a.auth(http.HandlerFunc(a.logout)))
	mux.Handle("GET /admin/backup", a.auth(http.HandlerFunc(a.backupLibrary)))
	mux.Handle("GET /admin/export-library", a.auth(http.HandlerFunc(a.exportLibrary)))
	return a.headers(mux)
}
func (a *App) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("request panic: %v", e)
				http.Error(w, "服务器处理失败", 500)
			}
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/admin") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			r.Body = http.MaxBytesReader(w, r.Body, maxSVG+128*1024)
			origin := r.Header.Get("Origin")
			expected := a.origin
			if expected == "" {
				scheme := "http"
				if r.TLS != nil {
					scheme = "https"
				}
				expected = scheme + "://" + r.Host
			}
			if (origin != "" && strings.TrimRight(origin, "/") != expected) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				http.Error(w, "不允许跨站操作", 403)
				return
			}
			if r.ContentLength > maxSVG+128*1024 {
				http.Error(w, "文件过大", 413)
				return
			}
		}
		if r.URL.Path != "/admin/library/scan" {
			a.store.viewMu.RLock()
			defer a.store.viewMu.RUnlock()
		}
		next.ServeHTTP(w, r)
	})
}
func renderError(w http.ResponseWriter, e error) {
	code := 400
	if errors.Is(e, sql.ErrNoRows) {
		code = 404
	}
	http.Error(w, e.Error(), code)
}
func (a *App) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if e := a.templates.ExecuteTemplate(&buf, name, data); e != nil {
		log.Print(e)
		http.Error(w, "页面暂时无法加载", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}
func requestID(r *http.Request) int64 { n, _ := strconv.ParseInt(r.PathValue("id"), 10, 64); return n }

type Category struct {
	Name  string
	Count int
}
type PageLink struct {
	Number int
	URL    string
	Active bool
	Gap    bool
}

func paginationLinks(page, pages int, query url.Values) []PageLink {
	return paginationLinksAt("/", page, pages, query)
}

func paginationLinksAt(path string, page, pages int, query url.Values) []PageLink {
	pages = max(1, pages)
	page = max(1, min(page, pages))
	start := max(1, min(page-4, pages-9))
	end := min(pages, start+9)
	links := []PageLink{}
	add := func(number int) {
		values := url.Values{}
		for key, items := range query {
			values[key] = append([]string(nil), items...)
		}
		values.Set("page", strconv.Itoa(number))
		links = append(links, PageLink{Number: number, URL: path + "?" + values.Encode(), Active: number == page})
	}
	if start > 1 {
		add(1)
		if start > 2 {
			links = append(links, PageLink{Gap: true})
		}
	}
	for n := start; n <= end; n++ {
		add(n)
	}
	if end < pages {
		if end < pages-1 {
			links = append(links, PageLink{Gap: true})
		}
		add(pages)
	}
	return links
}

type HomePage struct {
	HeroIcons                             []Group
	Groups                                []Group
	Categories                            []Category
	Brands                                []Brand
	Brand, Category, Q, Sort              string
	Total, Icons, BrandCount, Page, Pages int
	Links                                 []PageLink
	Hero                                  bool
	BrandDirectory                        bool
	BrandCards, DirectoryCards            []BrandOverview
	SelectedBrand                         *BrandOverview
}

type BrandOverview struct {
	Brand    Brand
	Logo     *Icon
	Main     *Group
	Products []Group
	Previews []Group
	Icons    int
}

func brandOverviews(brands []Brand, groups []Group) []BrandOverview {
	byBrand := map[string][]Group{}
	for _, g := range groups {
		if g.Count > 0 {
			byBrand[g.Brand] = append(byBrand[g.Brand], g)
		}
	}
	result := []BrandOverview{}
	for _, b := range brands {
		if !b.Published || b.Deleted || len(byBrand[b.Slug]) == 0 {
			continue
		}
		card := BrandOverview{Brand: b}
		for _, g := range byBrand[b.Slug] {
			card.Icons += g.Count
			if g.Key == "" {
				main := g
				card.Main = &main
				card.Logo = g.Icon
			} else {
				card.Products = append(card.Products, g)
				if len(card.Previews) < 4 {
					card.Previews = append(card.Previews, g)
				}
			}
		}
		result = append(result, card)
	}
	return result
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	groups, e := a.store.groups(false)
	if e != nil {
		renderError(w, e)
		return
	}
	brands, e := a.store.brands()
	if e != nil {
		renderError(w, e)
		return
	}
	q := r.URL.Query()
	p := HomePage{Q: strings.TrimSpace(q.Get("q")), Category: q.Get("category"), Brand: q.Get("brand"), Sort: q.Get("sort"), Page: 1}
	p.BrandDirectory = q.Get("view") == "brands" && p.Brand == ""
	p.BrandCards = brandOverviews(brands, groups)
	if p.Brand != "" {
		for n := range p.BrandCards {
			if p.BrandCards[n].Brand.Slug == p.Brand {
				p.SelectedBrand = &p.BrandCards[n]
				break
			}
		}
		if p.SelectedBrand == nil {
			http.NotFound(w, r)
			return
		}
	}
	if len(p.Q) > 300 {
		p.Q = p.Q[:300]
	}
	p.Page, _ = strconv.Atoi(q.Get("page"))
	if p.Page < 1 {
		p.Page = 1
	}
	categories := map[string]int{}
	heroCandidates := map[string]Group{}
	filtered := []Group{}
	favorites := map[string]bool{}
	favoriteFilter := q.Has("favorites")
	for _, id := range strings.Split(q.Get("favorites"), ",") {
		favorites[id] = true
	}
	for _, b := range brands {
		if b.Published && !b.Deleted {
			p.BrandCount++
			p.Brands = append(p.Brands, b)
		}
	}
	for _, g := range groups {
		if g.Count == 0 {
			continue
		}
		heroCandidates[g.Brand+"/"+g.Key] = g
		p.Icons += g.Count
		categories[g.Meta.Category]++
		haystack := strings.ToLower(g.Brand + " " + g.Key + " " + g.Meta.Title + " " + strings.Join(g.Meta.Tags, " ") + " " + strings.Join(g.Meta.Aliases, " ") + " " + strings.Join(g.Meta.Domains, " "))
		if p.Q != "" && !strings.Contains(haystack, strings.ToLower(p.Q)) {
			continue
		}
		if p.Category != "" && g.Meta.Category != p.Category {
			continue
		}
		if p.Brand != "" && g.Brand != p.Brand {
			continue
		}
		if favoriteFilter && !favorites[strconv.FormatInt(g.ID, 10)] {
			continue
		}
		filtered = append(filtered, g)
	}
	for _, key := range []string{"google/", "apple/", "github/", "wechat/", "bytedance/douyin", "openai/chatgpt", "bilibili/", "alipay/", "google/youtube", "microsoft/", "figma/", "qq/", "spotify/", "alibaba/taobao"} {
		if g, ok := heroCandidates[key]; ok {
			p.HeroIcons = append(p.HeroIcons, g)
		}
	}
	for name, count := range categories {
		p.Categories = append(p.Categories, Category{name, count})
	}
	sortByName(p.Categories, func(c Category) string { return c.Name })
	switch p.Sort {
	case "popular":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Downloads > filtered[j].Downloads })
	case "recent":
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Updated > filtered[j].Updated })
	}
	p.Total = len(filtered)
	if p.BrandDirectory {
		for _, card := range p.BrandCards {
			b := card.Brand
			haystack := strings.ToLower(b.Slug + " " + b.Meta.Title + " " + strings.Join(b.Meta.Aliases, " ") + " " + strings.Join(b.Meta.Domains, " "))
			if p.Q == "" || strings.Contains(haystack, strings.ToLower(p.Q)) {
				p.DirectoryCards = append(p.DirectoryCards, card)
			}
		}
		p.Total = len(p.DirectoryCards)
	}
	p.Pages = max(1, (p.Total+47)/48)
	p.Page = min(p.Page, p.Pages)
	start := (p.Page - 1) * 48
	if p.BrandDirectory {
		p.DirectoryCards = p.DirectoryCards[start:min(start+48, p.Total)]
	} else {
		p.Groups = filtered[start:min(start+48, p.Total)]
	}
	p.Hero = p.Q == "" && p.Brand == "" && p.Category == "" && !favoriteFilter && !p.BrandDirectory
	p.Links = paginationLinks(p.Page, p.Pages, q)
	a.render(w, "home.html", p)
}
func (a *App) detail(w http.ResponseWriter, r *http.Request) {
	g, e := groupByID(a.store.db, requestID(r))
	if e != nil {
		renderError(w, e)
		return
	}
	b, e := brandByID(a.store.db, g.BrandID)
	if e != nil || !b.Published || b.Deleted || !g.Published || g.Deleted {
		http.NotFound(w, r)
		return
	}
	icons, e := iconsFor(a.store.db, g.ID)
	if e != nil {
		renderError(w, e)
		return
	}
	visible := []Icon{}
	for _, i := range icons {
		if !i.Deleted {
			visible = append(visible, i)
		}
	}
	if len(visible) == 0 {
		http.NotFound(w, r)
		return
	}
	a.render(w, "detail.html", struct {
		Group Group
		Brand Brand
		Icons []Icon
	}{g, b, visible})
}
func (a *App) serveSVG(w http.ResponseWriter, r *http.Request, i Icon) {
	var content []byte
	if e := a.store.db.QueryRow(`SELECT svg FROM icons WHERE id=?`, i.ID).Scan(&content); e != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Content-Type", "image/svg+xml")
	http.ServeContent(w, r, i.Meta.File, time.Time{}, bytes.NewReader(content))
}
func (a *App) asset(w http.ResponseWriter, r *http.Request) {
	i, e := publicIcon(a.store.db, requestID(r))
	if e != nil || r.PathValue("hash") != i.Meta.SHA {
		http.NotFound(w, r)
		return
	}
	etag := `"` + i.Meta.SHA + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(304)
		return
	}
	a.serveSVG(w, r, i)
}
func (a *App) download(w http.ResponseWriter, r *http.Request) {
	i, e := publicIcon(a.store.db, requestID(r))
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if e = a.store.recordDownload(i); e != nil {
		renderError(w, e)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+i.Meta.File+`"`)
	a.serveSVG(w, r, i)
}
func (a *App) adminPreview(w http.ResponseWriter, r *http.Request) {
	i, e := iconByID(a.store.db, requestID(r))
	if e != nil {
		http.NotFound(w, r)
		return
	}
	a.serveSVG(w, r, i)
}
func parseJSON[T any](s string) T { var value T; json.Unmarshal([]byte(s), &value); return value }
