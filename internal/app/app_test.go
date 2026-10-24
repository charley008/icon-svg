package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var testSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="red" d="M0 0h24v24H0z"/></svg>`)

const testPassword = "test-password-123456789"

func tinyLibrary() fs.FS {
	i := IconMeta{File: "google.svg", Version: 0, Default: true, SHA: digest(testSVG), Bytes: len(testSVG), Label: "原版", Extra: map[string]json.RawMessage{"gotab_id": json.RawMessage("123")}}
	p := i
	p.File = "google-gmail.svg"
	m := BrandMeta{Schema: 1, GroupMeta: GroupMeta{Title: "Google", Icons: []IconMeta{i}, Next: 1}, Products: []GroupMeta{{Key: "gmail", Title: "Gmail", Icons: []IconMeta{p}, Next: 1}}}
	return fstest.MapFS{"google/meta.json": {Data: []byte(jsonText(m))}, "google/google.svg": {Data: testSVG}, "google/google-gmail.svg": {Data: testSVG}}
}
func testApp(t *testing.T) (*App, http.Handler) {
	t.Helper()
	a, h, _, _ := fileLibraryApp(t)
	return a, h
}
func send(t *testing.T, h http.Handler, method, target string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, target, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func authenticate(t *testing.T, a *App, h http.Handler) (*http.Cookie, string) {
	t.Helper()
	w := send(t, h, "POST", "/admin/login", url.Values{"username": {"admin"}, "password": {testPassword}}, nil)
	if w.Code != 303 {
		t.Fatalf("login: %d %s", w.Code, w.Body.String())
	}
	c := w.Result().Cookies()[0]
	var token string
	if e := a.store.db.QueryRow(`SELECT csrf FROM sessions WHERE token=?`, digest([]byte(c.Value))).Scan(&token); e != nil {
		t.Fatal(e)
	}
	return c, token
}
func uploadTest(t *testing.T, h http.Handler, c *http.Cookie, csrf string, gid int64, svg []byte) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	m := multipart.NewWriter(&body)
	m.WriteField("csrf", csrf)
	part, _ := m.CreateFormFile("file", "user-filename.svg")
	part.Write(svg)
	m.Close()
	r := httptest.NewRequest("POST", fmt.Sprintf("/admin/groups/%d/upload", gid), &body)
	r.Header.Set("Content-Type", m.FormDataContentType())
	r.AddCookie(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func assertCode(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("HTTP %d, want %d: %.1000s", w.Code, want, w.Body.String())
	}
}
func TestFullLibraryLoadAndRestart(t *testing.T) {
	a, h, dir, file := fileLibraryAppFrom(t, os.DirFS(filepath.Join("..", "..", "library")))
	s := a.store
	var e error
	// Derive expectations from the shipped files so adding artwork does not break CI.
	wantBrands, wantGroups, wantIcons, wantDefaults := 0, 0, 0, 0
	entries, err := os.ReadDir(filepath.Join("..", "..", "library"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("..", "..", "library", entry.Name(), "meta.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var meta BrandMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			t.Fatal(err)
		}
		wantBrands++
		for _, group := range append([]GroupMeta{meta.GroupMeta}, meta.Products...) {
			wantGroups++
			wantIcons += len(group.Icons)
			for _, icon := range group.Icons {
				if icon.Default {
					wantDefaults++
				}
			}
		}
	}
	if wantBrands == 0 || wantIcons == 0 {
		t.Fatal("shipped library must not be empty")
	}
	for table, want := range map[string]int{"brands": wantBrands, "groups": wantGroups, "icons": wantIcons} {
		var n int
		s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
		if n != want {
			t.Fatalf("%s=%d want %d", table, n, want)
		}
	}
	var count int
	s.db.QueryRow(`SELECT SUM(is_default) FROM icons`).Scan(&count)
	if count != wantDefaults {
		t.Fatal(count)
	}
	for table, predicate := range map[string]string{
		"audit": "1=1", "sessions": "1=1", "attempts": "1=1",
		"icons": "deleted<>0 OR downloads<>0", "groups": "deleted<>0 OR published<>1", "brands": "deleted<>0 OR published<>1",
	} {
		if e = s.db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE " + predicate).Scan(&count); e != nil || count != 0 {
			t.Fatalf("fresh %s must have no operational state: %d %v", table, count, e)
		}
	}
	for _, target := range []string{"/", "/?q=Gmail", "/detail/1", "/healthz"} {
		assertCode(t, send(t, h, "GET", target, nil, nil), 200)
	}
	c, _ := authenticate(t, a, h)
	exported := send(t, h, "GET", "/admin/export-library", nil, c)
	assertCode(t, exported, 200)
	z, e := zip.NewReader(bytes.NewReader(exported.Body.Bytes()), int64(exported.Body.Len()))
	if e != nil {
		t.Fatal(e)
	}
	fsys := fstest.MapFS{}
	for _, file := range z.File {
		if strings.HasPrefix(file.Name, "library/") {
			stream, _ := file.Open()
			content, _ := io.ReadAll(stream)
			stream.Close()
			fsys[strings.TrimPrefix(file.Name, "library/")] = &fstest.MapFile{Data: content}
		}
	}
	reimport, e := newIndex(libraryInput{fsys})
	if e != nil {
		t.Fatal(e)
	}
	var imported int
	reimport.db.QueryRow(`SELECT COUNT(*) FROM icons`).Scan(&imported)
	reimport.db.Close()
	if imported != wantIcons {
		t.Fatal(imported)
	}
	var raw []byte
	s.db.QueryRow(`SELECT svg FROM icons WHERE id=1`).Scan(&raw)
	if digest(raw) == "" {
		t.Fatal("empty content")
	}
	s.close()
	s, e = openLibraryStore(file, dir, "admin", testPassword)
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	var restarted int
	s.db.QueryRow(`SELECT COUNT(*) FROM icons`).Scan(&restarted)
	if restarted != wantIcons {
		t.Fatal("catalog was not rebuilt from files", restarted)
	}
}

func TestIndexLoadFailureAndRetry(t *testing.T) {
	files := tinyLibrary().(fstest.MapFS)
	delete(files, "google/google-gmail.svg")
	if s, e := newIndex(files); e == nil {
		s.close()
		t.Fatal("expected invalid catalogue error")
	}
	s, e := newIndex(tinyLibrary())
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM icons`).Scan(&count)
	if count != 2 {
		t.Fatal(count)
	}
}

func TestUploadLimitAndPasswordRotation(t *testing.T) {
	a, h := testApp(t)
	c, token := authenticate(t, a, h)
	var gid int64
	a.store.db.QueryRow(`SELECT id FROM groups WHERE product_key=''`).Scan(&gid)
	for n := 0; n < 9; n++ {
		assertCode(t, uploadTest(t, h, c, token, gid, testSVG), 303)
	}
	assertCode(t, uploadTest(t, h, c, token, gid, testSVG), 400)
	form := url.Values{"csrf": {token}, "current": {testPassword}, "password": {"short-7"}}
	assertCode(t, send(t, h, "POST", "/admin/password", form, c), 400)
	form.Set("password", "newpass8")
	form.Set("password_confirm", "newpass8")
	assertCode(t, send(t, h, "POST", "/admin/password", form, c), 303)
	assertCode(t, send(t, h, "GET", "/admin", nil, c), 303)
	response := send(t, h, "POST", "/admin/login", url.Values{"username": {"admin"}, "password": {"newpass8"}}, nil)
	assertCode(t, response, 303)
}
func TestNamingDefaultsTrashAndCaching(t *testing.T) {
	a, h := testApp(t)
	c, token := authenticate(t, a, h)
	var gid int64
	a.store.db.QueryRow(`SELECT id FROM groups WHERE product_key='gmail'`).Scan(&gid)
	for n := 1; n <= 2; n++ {
		assertCode(t, uploadTest(t, h, c, token, gid, testSVG), 303)
	}
	var id int64
	var name string
	a.store.db.QueryRow(`SELECT id,filename FROM icons WHERE group_id=? AND version=2`, gid).Scan(&id, &name)
	if name != "google-gmail-2.svg" {
		t.Fatal(name)
	}
	form := url.Values{"csrf": {token}}
	assertCode(t, send(t, h, "POST", fmt.Sprintf("/admin/icons/%d/default", id), form, c), 303)
	icons, _ := iconsFor(a.store.db, gid)
	for _, i := range icons {
		if i.Meta.Default != (i.ID == id) {
			t.Fatal("default inconsistency")
		}
	}
	assertCode(t, send(t, h, "POST", fmt.Sprintf("/admin/icons/%d/trash", id), form, c), 303)
	assertCode(t, uploadTest(t, h, c, token, gid, testSVG), 303)
	a.store.db.QueryRow(`SELECT filename FROM icons WHERE group_id=? AND version=3`, gid).Scan(&name)
	if name != "google-gmail-3.svg" {
		t.Fatal("reused number", name)
	}
	assertCode(t, send(t, h, "POST", fmt.Sprintf("/admin/icons/%d/restore", id), form, c), 303)
	icons, _ = iconsFor(a.store.db, gid)
	defaults := 0
	for _, i := range icons {
		if i.Meta.Default {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatal(defaults)
	}
	i := icons[0]
	target := fmt.Sprintf("/assets/%d/%s", i.ID, i.Meta.SHA)
	response := send(t, h, "GET", target, nil, nil)
	assertCode(t, response, 200)
	r := httptest.NewRequest("GET", target, nil)
	r.Header.Set("If-None-Match", response.Header().Get("ETag"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assertCode(t, w, 304)
	assertCode(t, send(t, h, "GET", fmt.Sprintf("/admin/brands/%d", 1), nil, c), 200)
}
func TestBrandProductSecurityAndParentCycles(t *testing.T) {
	a, h := testApp(t)
	c, token := authenticate(t, a, h)
	assertCode(t, send(t, h, "POST", "/admin/brands", url.Values{"csrf": {token}, "slug": {"self"}, "title": {"self"}, "parent_brand": {"self"}}, c), 400)
	assertCode(t, send(t, h, "POST", "/admin/brands", url.Values{"slug": {"bad"}, "title": {"bad"}}, c), 403)
	form := url.Values{"csrf": {token}, "slug": {"example"}, "title": {"示例品牌"}, "parent_brand": {"google"}}
	assertCode(t, send(t, h, "POST", "/admin/brands", form, c), 303)
	var bid int64
	a.store.db.QueryRow(`SELECT id FROM brands WHERE slug='example'`).Scan(&bid)
	form = url.Values{"csrf": {token}, "key": {"app"}, "title": {"示例产品"}}
	assertCode(t, send(t, h, "POST", fmt.Sprintf("/admin/brands/%d/products", bid), form, c), 303)
	assertCode(t, send(t, h, "GET", fmt.Sprintf("/admin/brands/%d", bid), nil, c), 200)
	form = url.Values{"csrf": {token}, "title": {"Google"}, "parent_brand": {"example"}, "published": {"on"}}
	assertCode(t, send(t, h, "POST", "/admin/brands/1", form, c), 400)
	r := httptest.NewRequest("POST", "/admin/brands", strings.NewReader("csrf="+token))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.example")
	r.AddCookie(c)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	assertCode(t, w, 403)
}
func TestExportReimportBackupAndCleanup(t *testing.T) {
	a, h := testApp(t)
	c, _ := authenticate(t, a, h)
	response := send(t, h, "GET", "/admin/export-library", nil, c)
	assertCode(t, response, 200)
	z, e := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if e != nil {
		t.Fatal(e)
	}
	catalog := fstest.MapFS{}
	for _, f := range z.File {
		if strings.HasPrefix(f.Name, "library/") {
			r, _ := f.Open()
			content, _ := io.ReadAll(r)
			r.Close()
			catalog[strings.TrimPrefix(f.Name, "library/")] = &fstest.MapFile{Data: content}
		}
	}
	s, e := newIndex(libraryInput{catalog})
	if e != nil {
		t.Fatal(e)
	}
	var metadata string
	s.db.QueryRow(`SELECT metadata FROM icons ORDER BY id LIMIT 1`).Scan(&metadata)
	var restored IconMeta
	if e = json.Unmarshal([]byte(metadata), &restored); e != nil || string(restored.Extra["gotab_id"]) != "123" {
		t.Fatal("lost extension metadata", e)
	}
	s.db.Close()
	backup := send(t, h, "GET", "/admin/backup", nil, c)
	assertCode(t, backup, 200)
	if !bytes.HasPrefix(backup.Body.Bytes(), []byte("PK")) {
		t.Fatal("invalid backup")
	}
	files, _ := os.ReadDir(filepath.Join(a.data, "cache"))
	if len(files) != 0 {
		t.Fatal("temporary files remain")
	}
	a.store.db.Exec(`UPDATE icons SET svg=NULL WHERE id=-1`)
	assertCode(t, send(t, h, "POST", "/download/selection", url.Values{"ids": {"999999"}}, nil), 400)
	files, _ = os.ReadDir(filepath.Join(a.data, "cache"))
	if len(files) != 0 {
		t.Fatal("failed archive left files")
	}
}
func TestSVGSecurity(t *testing.T) {
	for _, fragment := range []string{`<script/>`, `<foreignObject/>`, `<image href="data:image/png,a"/>`, `<path onload="alert(1)"/>`, `<use href="https://evil/x"/>`, `<path fill="url(https://evil/x)"/>`, `<style>@import "https://evil/x";</style>`, `<style><![CDATA[@im]]><![CDATA[port "https://evil/x";]]></style>`, `<animate attributeName="href"/>`} {
		t.Run(fragment, func(t *testing.T) {
			raw := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">` + fragment + `</svg>`)
			if _, e := validateSVG(raw); e == nil {
				t.Fatal("accepted active SVG")
			}
		})
	}
	if _, e := validateSVG(testSVG); e != nil {
		t.Fatal(e)
	}
	if _, e := validateSVG([]byte(`<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg/>`)); e == nil {
		t.Fatal("accepted doctype")
	}
}
