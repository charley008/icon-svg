package app

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func fileLibraryApp(t *testing.T) (*App, http.Handler, string, string) {
	return fileLibraryAppFrom(t, tinyLibrary())
}
func fileLibraryAppFrom(t *testing.T, source fs.FS) (*App, http.Handler, string, string) {
	t.Helper()
	dir := t.TempDir()
	library := filepath.Join(dir, "library")
	data := filepath.Join(dir, "data")
	if e := os.MkdirAll(filepath.Join(data, "cache"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := fs.WalkDir(source, ".", func(name string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if name != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		raw, e := fs.ReadFile(source, name)
		if e != nil {
			return e
		}
		dest := filepath.Join(library, name)
		if e = os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
			return e
		}
		return os.WriteFile(dest, raw, 0644)
	}); e != nil {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(library, "licenses"), 0755)
	os.WriteFile(filepath.Join(library, "licenses", "README.md"), []byte("test license"), 0644)
	file := filepath.Join(data, "runtime.sqlite3")
	s, e := openLibraryStore(file, library, "admin", testPassword)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.close)
	a, e := newApp(s, data, "", false, AppResources{Web: os.DirFS(filepath.Join("..", "..", "web")), Library: libraryInput{s.library.root.FS()}, Root: os.DirFS(filepath.Join("..", ".."))})
	if e != nil {
		t.Fatal(e)
	}
	return a, a.routes(), library, file
}

func TestFileLibraryRoundTrip(t *testing.T) {
	a, h, dir, file := fileLibraryApp(t)
	cookie, token := authenticate(t, a, h)
	var bid, gid int64
	a.store.db.QueryRow(`SELECT id FROM brands WHERE slug='google'`).Scan(&bid)
	a.store.db.QueryRow(`SELECT id FROM groups WHERE product_key=''`).Scan(&gid)
	form := url.Values{"csrf": {token}, "title": {"谷歌网站"}, "category": {"其他"}, "url": {"https://google.com"}, "tags": {"搜索,Google"}, "published": {"on"}}
	assertCode(t, send(t, h, "POST", fmt.Sprintf("/admin/brands/%d", bid), form, cookie), 303)
	raw, e := os.ReadFile(filepath.Join(dir, "google", "meta.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m BrandMeta
	if e = json.Unmarshal(raw, &m); e != nil || m.Title != "谷歌网站" {
		t.Fatal(string(raw), e)
	}
	assertCode(t, uploadTest(t, h, cookie, token, gid, testSVG), 303)
	if _, e = os.Stat(filepath.Join(dir, "google", "google-1.svg")); e != nil {
		t.Fatal(e)
	}
	var id int64
	a.store.db.QueryRow(`SELECT id FROM icons WHERE filename='google-1.svg'`).Scan(&id)
	assertCode(t, send(t, h, "POST", fmt.Sprintf("/admin/icons/%d/default", id), url.Values{"csrf": {token}}, cookie), 303)
	assertCode(t, send(t, h, "GET", fmt.Sprintf("/download/%d", id), nil, nil), 200)
	disk, e := sql.Open("sqlite", file)
	if e != nil {
		t.Fatal(e)
	}
	defer disk.Close()
	var n int
	if e = disk.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name IN ('icons','brands','groups','categories')`).Scan(&n); e != nil || n != 0 {
		t.Fatal("catalog persisted to disk", n, e)
	}
	backup := send(t, h, "GET", "/admin/backup", nil, cookie)
	assertCode(t, backup, 200)
	z, e := zip.NewReader(bytes.NewReader(backup.Body.Bytes()), int64(backup.Body.Len()))
	if e != nil {
		t.Fatal(e)
	}
	found := map[string]bool{}
	for _, f := range z.File {
		found[f.Name] = true
	}
	if !found["library/google/meta.json"] || !found["data/runtime.sqlite3"] {
		t.Fatal(found)
	}
	a.store.close()
	a.store.library = nil
	// Cleanup may safely close the already closed Store after restart.
	s, e := openLibraryStore(file, dir, "admin", testPassword)
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	b, e := brandByID(s.db, bid)
	if e != nil || b.Meta.Title != "谷歌网站" {
		t.Fatal(b, e)
	}
	i, e := iconByID(s.db, id)
	if e != nil || !i.Meta.Default || i.Downloads != 1 {
		t.Fatal(i, e)
	}
}

func TestFreshInstallationState(t *testing.T) {
	a, _, _, file := fileLibraryApp(t)
	for _, table := range []string{"audit", "sessions", "attempts", "downloads", "settings"} {
		var count int
		if e := a.store.db.QueryRow("SELECT COUNT(*) FROM runtime." + table).Scan(&count); e != nil || count != 0 {
			t.Fatalf("fresh %s: %d %v", table, count, e)
		}
	}
	disk, e := sql.Open("sqlite", file)
	if e != nil {
		t.Fatal(e)
	}
	defer disk.Close()
	var count int
	if e = disk.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&count); e != nil || count != 6 {
		t.Fatalf("runtime must contain only operational tables: %d %v", count, e)
	}
	if e = disk.QueryRow(`SELECT COUNT(*) FROM admins`).Scan(&count); e != nil || count != 1 {
		t.Fatal("fresh administrator missing", count, e)
	}
}

func TestFileLibraryExternalScanAndConflict(t *testing.T) {
	a, h, dir, _ := fileLibraryApp(t)
	cookie, token := authenticate(t, a, h)
	p := filepath.Join(dir, "google", "meta.json")
	raw, _ := os.ReadFile(p)
	var m BrandMeta
	json.Unmarshal(raw, &m)
	m.Title = "外部编辑"
	os.WriteFile(p, []byte(jsonText(m)), 0644)
	e := a.store.transaction(func(tx *sql.Tx) error {
		_, e := tx.Exec(`UPDATE brands SET metadata=json_set(metadata,'$.title','覆盖')`)
		return e
	})
	if e == nil || !strings.Contains(e.Error(), "外部修改") {
		t.Fatal(e)
	}
	raw, _ = os.ReadFile(p)
	if !strings.Contains(string(raw), "外部编辑") {
		t.Fatal("external edit overwritten")
	}
	assertCode(t, send(t, h, "POST", "/admin/library/scan", url.Values{"csrf": {token}}, cookie), 303)
	assertCode(t, send(t, h, "GET", "/?q="+url.QueryEscape("外部编辑"), nil, nil), 200)
	var title string
	a.store.db.QueryRow(`SELECT json_extract(metadata,'$.title') FROM brands WHERE slug='google'`).Scan(&title)
	if title != "外部编辑" {
		t.Fatal(title)
	}
	os.WriteFile(p, []byte("{broken"), 0644)
	if e = a.store.reloadLibrary(); e == nil {
		t.Fatal("invalid JSON accepted")
	}
	a.store.db.QueryRow(`SELECT json_extract(metadata,'$.title') FROM brands WHERE slug='google'`).Scan(&title)
	if title != "外部编辑" {
		t.Fatal("good index lost")
	}
}

func TestFileLibraryTraversalAndLinks(t *testing.T) {
	a, h, dir, _ := fileLibraryApp(t)
	cookie, token := authenticate(t, a, h)
	for _, slug := range []string{"../../../etc/passwd", `..\..\escape`, "/tmp/pwn", "C:evil", "%2e%2e%2fescape", "con", "nul.txt", "com1", "licenses"} {
		assertCode(t, send(t, h, "POST", "/admin/brands", url.Values{"csrf": {token}, "slug": {slug}, "title": {"bad"}, "category": {"其他"}}, cookie), 400)
	}
	if e := a.store.library.save(map[string][]byte{"../../escape": []byte("bad")}); e == nil {
		t.Fatal("traversal accepted")
	}
	outside := filepath.Join(t.TempDir(), "sentinel.svg")
	os.WriteFile(outside, testSVG, 0644)
	link := filepath.Join(dir, "google", "google-9.svg")
	if e := os.Symlink(outside, link); e != nil {
		t.Log("symlink creation unavailable:", e)
	} else {
		if e = a.store.reloadLibrary(); e == nil {
			t.Fatal("symlink accepted")
		}
		os.Remove(link)
	}
	raw, _ := os.ReadFile(outside)
	if !bytes.Equal(raw, testSVG) {
		t.Fatal("outside file changed")
	}
	if _, e := openLibrary(dir); e == nil {
		t.Fatal("second writer acquired library")
	}
	outsideDir := t.TempDir()
	junction := filepath.Join(dir, "escape")
	if runtime.GOOS == "windows" {
		if out, e := exec.Command("cmd.exe", "/c", "mklink", "/J", junction, outsideDir).CombinedOutput(); e != nil {
			t.Fatalf("create junction: %v %s", e, out)
		}
	} else {
		if e := os.Symlink(outsideDir, junction); e != nil {
			t.Fatal(e)
		}
	}
	defer os.Remove(junction)
	if e := a.store.reloadLibrary(); e == nil {
		t.Fatal("directory junction/symlink accepted")
	}
	if e := a.store.library.atomicFile("escape/meta.json", []byte("escape")); e == nil {
		t.Fatal("write through junction/symlink accepted")
	}
	if _, e := os.Stat(filepath.Join(outsideDir, "meta.json")); !os.IsNotExist(e) {
		t.Fatal("wrote outside library", e)
	}
}

func TestLibraryHardLinkReplacementDoesNotModifyOutside(t *testing.T) {
	a, _, dir, _ := fileLibraryApp(t)
	outside := filepath.Join(t.TempDir(), "outside.svg")
	if e := os.WriteFile(outside, testSVG, 0644); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(dir, "google", "google-8.svg")
	if e := os.Link(outside, target); e != nil {
		t.Skip("hard link unavailable:", e)
	}
	replacement := bytes.ReplaceAll(testSVG, []byte("red"), []byte("blue"))
	if e := a.store.library.atomicFile("google/google-8.svg", replacement); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(outside)
	if !bytes.Equal(raw, testSVG) {
		t.Fatal("hard link truncated outside file")
	}
}

func TestFileLibraryDeletionAndStableIDs(t *testing.T) {
	a, _, dir, _ := fileLibraryApp(t)
	var original int64
	a.store.db.QueryRow(`SELECT id FROM icons WHERE filename='google.svg'`).Scan(&original)
	if e := os.Mkdir(filepath.Join(dir, "aaa"), 0755); e != nil {
		t.Fatal(e)
	}
	m := BrandMeta{Schema: 1, GroupMeta: GroupMeta{Title: "AAA", Category: "其他"}, Products: []GroupMeta{}}
	os.WriteFile(filepath.Join(dir, "aaa", "meta.json"), []byte(jsonText(m)), 0644)
	if e := a.store.reloadLibrary(); e != nil {
		t.Fatal(e)
	}
	var actual int64
	a.store.db.QueryRow(`SELECT id FROM icons WHERE filename='google.svg'`).Scan(&actual)
	if actual != original {
		t.Fatal("existing ID changed")
	}
	if e := os.Remove(filepath.Join(dir, "google", "google.svg")); e != nil {
		t.Fatal(e)
	}
	if e := a.store.reloadLibrary(); e != nil {
		t.Fatal(e)
	}
	var count int
	a.store.db.QueryRow(`SELECT COUNT(*) FROM icons WHERE filename='google.svg'`).Scan(&count)
	if count != 0 {
		t.Fatal("removed file still indexed")
	}
}

func TestFileLibraryConcurrentUploads(t *testing.T) {
	a, h, dir, _ := fileLibraryApp(t)
	cookie, token := authenticate(t, a, h)
	var gid int64
	a.store.db.QueryRow(`SELECT id FROM groups WHERE product_key=''`).Scan(&gid)
	var wg sync.WaitGroup
	for n := 0; n < 3; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := uploadTest(t, h, cookie, token, gid, testSVG)
			if w.Code != 303 {
				t.Errorf("upload %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	raw, _ := os.ReadFile(filepath.Join(dir, "google", "meta.json"))
	var m BrandMeta
	json.Unmarshal(raw, &m)
	if len(m.Icons) != 4 || m.Next != 4 {
		t.Fatalf("lost concurrent upload: icons=%d next=%d", len(m.Icons), m.Next)
	}
}

func TestFileLibraryRecoveryAndDiscovery(t *testing.T) {
	a, _, dir, _ := fileLibraryApp(t)
	os.WriteFile(filepath.Join(dir, "google", "google-4.svg"), testSVG, 0644)
	if e := a.store.reloadLibrary(); e != nil {
		t.Fatal(e)
	}
	var n int
	a.store.db.QueryRow(`SELECT COUNT(*) FROM icons WHERE filename='google-4.svg'`).Scan(&n)
	if n != 1 {
		t.Fatal("new SVG not indexed")
	}
	if e := a.store.transaction(func(*sql.Tx) error { return nil }); e != nil {
		t.Fatal(e)
	}
	// Simulate power loss after the redo journal was synced but before rename.
	raw, _ := os.ReadFile(filepath.Join(dir, "google", "meta.json"))
	var m BrandMeta
	json.Unmarshal(raw, &m)
	m.Title = "恢复写入"
	journal, _ := json.Marshal(map[string][]byte{"google/meta.json": []byte(jsonText(m))})
	os.WriteFile(filepath.Join(dir, ".pending.json"), journal, 0600)
	if e := a.store.library.recover(); e != nil {
		t.Fatal(e)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "google", "meta.json"))
	if !strings.Contains(string(raw), "恢复写入") {
		t.Fatal("journal not replayed")
	}
}
