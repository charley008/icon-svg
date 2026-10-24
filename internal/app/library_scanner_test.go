package app

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIncrementalAndFullLibraryScan(t *testing.T) {
	a, _, dir, _ := fileLibraryApp(t)
	if e := a.store.reloadLibrary(); e != nil {
		t.Fatal(e)
	}
	status := a.store.library.scanStatus()
	if status.Updated || status.Reused != 2 || status.Read != 2 || status.Added+status.Modified+status.Removed != 0 {
		t.Fatalf("unchanged scan must reuse two SVGs: %+v", status)
	}
	file := filepath.Join(dir, "google", "google.svg")
	info, e := os.Stat(file)
	if e != nil {
		t.Fatal(e)
	}
	// An editor can preserve size, timestamp and inode. Incremental scans use
	// these attributes; the explicit full scan must still detect the change.
	modified := bytes.ReplaceAll(testSVG, []byte("red"), []byte("tan"))
	if e = os.WriteFile(file, modified, 0644); e != nil {
		t.Fatal(e)
	}
	if e = os.Chtimes(file, info.ModTime(), info.ModTime()); e != nil {
		t.Fatal(e)
	}
	if e = a.store.reloadLibrary(); e != nil {
		t.Fatal(e)
	}
	var hash string
	a.store.db.QueryRow(`SELECT hash FROM icons WHERE filename='google.svg'`).Scan(&hash)
	if hash != digest(testSVG) {
		t.Fatal("unexpected cache result")
	}
	if e = a.store.scanLibrary(true); e != nil {
		t.Fatal(e)
	}
	status = a.store.library.scanStatus()
	if !status.Updated || !status.Full || status.Reused != 0 || status.Read != 4 || status.Modified != 2 {
		t.Fatalf("full scan must refresh SVG and metadata hash: %+v", status)
	}
	a.store.db.QueryRow(`SELECT hash FROM icons WHERE filename='google.svg'`).Scan(&hash)
	if hash != digest(modified) {
		t.Fatal("full scan failed to update SVG hash")
	}
	if e = os.Remove(file); e != nil {
		t.Fatal(e)
	}
	if e = a.store.reloadLibrary(); e != nil {
		t.Fatal(e)
	}
	if status = a.store.library.scanStatus(); status.Removed != 1 || status.Icons != 1 {
		t.Fatalf("removed SVG not counted: %+v", status)
	}
}

func TestMetadataRereadAndScanFailureStatus(t *testing.T) {
	a, h, dir, _ := fileLibraryApp(t)
	cookie, token := authenticate(t, a, h)
	p := filepath.Join(dir, "google", "meta.json")
	info, _ := os.Stat(p)
	raw, _ := os.ReadFile(p)
	raw = bytes.Replace(raw, []byte(`"Google"`), []byte(`"Search"`), 1)
	os.WriteFile(p, raw, 0644)
	os.Chtimes(p, info.ModTime(), info.ModTime())
	if e := a.store.reloadLibrary(); e != nil {
		t.Fatal(e)
	}
	var title string
	a.store.db.QueryRow(`SELECT json_extract(metadata,'$.title') FROM brands`).Scan(&title)
	if title != "Search" {
		t.Fatal("metadata must not rely on timestamp cache", title)
	}
	good := a.store.library.scanStatus().LastSuccess
	os.WriteFile(p, []byte("{invalid"), 0644)
	result := send(t, h, "POST", "/admin/library/scan", url.Values{"csrf": {token}}, cookie)
	assertCode(t, result, 303)
	status := a.store.library.scanStatus()
	if status.Error == "" || !status.LastSuccess.Equal(good) || status.Running {
		t.Fatalf("failed scan status incorrect: %+v", status)
	}
	page := send(t, h, "GET", "/admin/settings?view=library", nil, cookie)
	assertCode(t, page, 200)
	if !strings.Contains(page.Body.String(), "扫描失败") || !strings.Contains(page.Body.String(), "继续使用上次可用索引") {
		t.Fatal("scan error was not visible in settings")
	}
	a.store.db.QueryRow(`SELECT json_extract(metadata,'$.title') FROM brands`).Scan(&title)
	if title != "Search" {
		t.Fatal("failed scan replaced good index")
	}
	var m BrandMeta
	json.Unmarshal(raw, &m)
	m.Title = "恢复扫描"
	os.WriteFile(p, []byte(jsonText(m)), 0644)
	assertCode(t, send(t, h, "POST", "/admin/library/scan", url.Values{"csrf": {token}, "mode": {"full"}}, cookie), 303)
	if status = a.store.library.scanStatus(); status.Error != "" || !status.Full || status.LastSuccess.Equal(good) {
		t.Fatalf("successful retry not recorded: %+v", status)
	}
}

func TestScanRequestsCoalesce(t *testing.T) {
	a, _, _, _ := fileLibraryApp(t)
	a.store.library.scanMu.Lock()
	if e := a.store.reloadLibrary(); e != errLibraryScanRunning {
		t.Fatal("overlapping periodic scan was not coalesced", e)
	}
	a.store.library.scanMu.Unlock()
	if e := a.store.reloadLibrary(); e != nil {
		t.Fatal(e)
	}
}
