package app

import (
	"bytes"
	"context"
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
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type App struct {
	store        *Store
	data, origin string
	secure       bool
	version      string
	templates    *template.Template
	files        AppResources
	scanInterval atomic.Int64
	scanChanged  chan struct{}
	publicSlots  chan struct{}
	assetSlots   chan struct{}
	adminSlots   chan struct{}
	archiveSlots chan struct{}
}

type AppResources struct{ Web, Library, Root fs.FS }

func newApp(s *Store, data, origin string, secure bool, resources ...AppResources) (*App, error) {
	files := AppResources{Web: os.DirFS("web"), Library: libraryInput{os.DirFS("library")}, Root: os.DirFS(".")}
	if len(resources) > 0 {
		files = resources[0]
	}
	var a *App
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
	}, "navigationLinks": func() ([]NavigationLink, error) {
		links, err := a.navigationLinks()
		visible := make([]NavigationLink, 0, len(links))
		for _, link := range links {
			if link.Visible {
				visible = append(visible, link)
			}
		}
		return visible, err
	}, "appVersion": func() string { return a.version }, "kb": func(n int) string { return fmt.Sprintf("%.1f", float64(n)/1024) }}).ParseFS(files.Web, "templates/*.html")
	if e != nil {
		return nil, fmt.Errorf("读取 web/templates 失败，请解压完整安装包：%w", e)
	}
	a = &App{store: s, data: data, origin: strings.TrimRight(origin, "/"), secure: secure, templates: t, files: files, version: "dev", scanChanged: make(chan struct{}, 1)}
	a.publicSlots = make(chan struct{}, 4)
	a.assetSlots = make(chan struct{}, 16)
	a.adminSlots = make(chan struct{}, 8)
	a.archiveSlots = make(chan struct{}, 1)
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
	mux.HandleFunc("GET /api/brands", a.searchBrands)
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
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if e := a.store.db.PingContext(ctx); e != nil {
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
	mux.Handle("POST /admin/library/import", a.auth(http.HandlerFunc(a.importLibrary)))
	mux.Handle("GET /admin/library/export", a.auth(http.HandlerFunc(a.exportFullLibrary)))
	mux.Handle("POST /admin/navigation", a.auth(http.HandlerFunc(a.saveNavigation)))
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
		release, ok := a.admit(w, r)
		if !ok {
			return
		}
		defer release()
		if !strings.HasPrefix(r.URL.Path, "/admin") && !strings.HasPrefix(r.URL.Path, "/static/") {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
		}
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
			limit := int64(maxSVG + 128*1024)
			if r.URL.Path == "/admin/library/import" {
				limit = maxLibraryZIP + 128*1024
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
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
			if r.ContentLength > limit {
				http.Error(w, "文件过大", 413)
				return
			}
		}
		if r.URL.Path != "/admin/library/scan" && r.URL.Path != "/admin/library/import" && r.URL.Path != "/healthz" && !strings.HasPrefix(r.URL.Path, "/static/") {
			if !a.store.viewMu.TryRLock() {
				unavailable(w)
				return
			}
			defer a.store.viewMu.RUnlock()
		}
		next.ServeHTTP(w, r)
	})
}
func renderError(w http.ResponseWriter, e error) {
	code := 400
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		unavailable(w)
		return
	}
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
	Brand, Category, Q, Sort              string
	Total, Icons, BrandCount, Page, Pages int
	Links                                 []PageLink
	Hero                                  bool
	BrandDirectory                        bool
	DirectoryCards                        []BrandOverview
	DirectoryCount                        int
	NavBrandCards                         []BrandOverview
	SelectedBrand                         *BrandOverview
}

type BrandOverview struct {
	Brand        Brand
	Logo         *Icon
	Main         *Group
	ProductCount int
	Previews     []Group
	Icons        int
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	p, err := a.store.publicHome(r.Context(), r.URL.Query())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
		} else {
			renderError(w, err)
		}
		return
	}
	a.render(w, "home.html", p)
}
func (a *App) detail(w http.ResponseWriter, r *http.Request) {
	queries := contextQueries{r.Context(), a.store.db}
	g, e := groupByID(queries, requestID(r))
	if errors.Is(e, sql.ErrNoRows) {
		var target int64
		err := queries.QueryRow(`SELECT g.id FROM groups g JOIN brands b ON b.id=g.brand_id JOIN json_each(g.metadata,'$.redirect_from') old WHERE old.value=? AND b.published=1 AND b.deleted=0 AND g.published=1 AND g.deleted=0 AND EXISTS(SELECT 1 FROM icons i WHERE i.group_id=g.id AND i.deleted=0)`, requestID(r)).Scan(&target)
		if err == nil {
			http.Redirect(w, r, "/detail/"+strconv.FormatInt(target, 10), http.StatusPermanentRedirect)
			return
		}
	}
	if e != nil {
		renderError(w, e)
		return
	}
	b, e := brandByID(queries, g.BrandID)
	if e != nil || !b.Published || b.Deleted || !g.Published || g.Deleted {
		http.NotFound(w, r)
		return
	}
	icons, e := iconsFor(queries, g.ID)
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
	file, err := a.store.openSVG(r.Context(), i)
	if err != nil {
		unavailable(w)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Content-Type", "image/svg+xml")
	http.ServeContent(w, r, i.Meta.File, time.Time{}, file)
}

func (a *App) asset(w http.ResponseWriter, r *http.Request) {
	i, e := publicIcon(contextQueries{r.Context(), a.store.db}, requestID(r))
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
	i, e := publicIcon(contextQueries{r.Context(), a.store.db}, requestID(r))
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if e = a.store.recordDownloadContext(r.Context(), i); e != nil {
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
