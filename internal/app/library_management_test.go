package app

import (
	"archive/zip"
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLibrarySchedulePersistsAndValidates(t *testing.T) {
	a, h, dir, file := fileLibraryApp(t)
	cookie, token := authenticate(t, a, h)
	assertCode(t, send(t, h, "POST", "/admin/library/schedule", url.Values{"minutes": {"60"}}, nil), 303)
	assertCode(t, send(t, h, "POST", "/admin/library/schedule", url.Values{"minutes": {"60"}}, cookie), 403)
	for _, value := range []string{"-1", "43201", "abc", "1.5"} {
		assertCode(t, send(t, h, "POST", "/admin/library/schedule", url.Values{"csrf": {token}, "minutes": {value}}, cookie), 400)
	}
	if time.Duration(a.scanInterval.Load()) != 5*time.Minute {
		t.Fatal("invalid interval changed schedule")
	}
	assertCode(t, send(t, h, "POST", "/admin/library/schedule", url.Values{"csrf": {token}, "minutes": {"60"}}, cookie), 303)
	if time.Duration(a.scanInterval.Load()) != time.Hour {
		t.Fatal("schedule was not applied")
	}
	page := send(t, h, "GET", "/admin/settings?view=library", nil, cookie)
	assertCode(t, page, 200)
	if !strings.Contains(page.Body.String(), `value="60"`) || strings.Contains(page.Body.String(), `href="/admin/backup-database"`) {
		t.Fatal("library controls are not separate")
	}
	backup := send(t, h, "GET", "/admin/backup-database", nil, cookie)
	assertCode(t, backup, 200)
	z, err := zip.NewReader(bytes.NewReader(backup.Body.Bytes()), int64(backup.Body.Len()))
	if err != nil || len(z.File) != 1 || z.File[0].Name != "data/runtime.sqlite3" {
		t.Fatal("database backup contains material files", err)
	}
	assertCode(t, send(t, h, "POST", "/admin/library/schedule", url.Values{"csrf": {token}, "minutes": {"0"}}, cookie), 303)
	a.store.close()
	store, err := openLibraryStore(file, dir, "admin", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer store.close()
	a.store = store
	if err = a.configureScanInterval(time.Hour); err != nil || a.scanInterval.Load() != 0 {
		t.Fatal("disabled schedule did not survive restart", err)
	}
}

func TestScanPreviewsRetainedAndAuthenticated(t *testing.T) {
	a, h, dir, _ := fileLibraryApp(t)
	cookie, token := authenticate(t, a, h)
	if a.store.library.scanStatus().RecentAddedTotal != 0 {
		t.Fatal("startup index must not be an import batch")
	}
	if err := os.WriteFile(filepath.Join(dir, "google", "google-1.svg"), testSVG, 0644); err != nil {
		t.Fatal(err)
	}
	assertCode(t, send(t, h, "POST", "/admin/library/scan", url.Values{"csrf": {token}}, cookie), 303)
	status := a.store.library.scanStatus()
	if status.RecentAddedTotal != 1 || len(status.RecentAdded) != 1 || status.RecentAdded[0] != "google/google-1.svg" {
		t.Fatalf("wrong added preview: %+v", status)
	}
	page := send(t, h, "GET", "/admin/settings?view=library", nil, cookie)
	assertCode(t, page, 200)
	if !strings.Contains(page.Body.String(), "google-1.svg") || !strings.Contains(page.Body.String(), "/admin/preview/") {
		t.Fatal("new icon missing from library preview")
	}
	var id int64
	a.store.db.QueryRow(`SELECT id FROM icons WHERE filename='google-1.svg'`).Scan(&id)
	target := "/admin/preview/" + strconv.FormatInt(id, 10)
	assertCode(t, send(t, h, "GET", target, nil, nil), 303)
	assertCode(t, send(t, h, "GET", target, nil, cookie), 200)
	if err := a.store.reloadLibrary(); err != nil {
		t.Fatal(err)
	}
	next := a.store.library.scanStatus()
	if next.Added != 0 || next.RecentAddedTotal != 1 || !next.AddedAt.Equal(status.AddedAt) {
		t.Fatal("unchanged scan cleared recent imports")
	}
	os.WriteFile(filepath.Join(dir, "google", "google-2.svg"), []byte("broken"), 0644)
	if err := a.store.reloadLibrary(); err == nil {
		t.Fatal("invalid SVG accepted")
	}
	if got := a.store.library.scanStatus(); len(got.RecentAdded) != 1 || got.RecentAdded[0] != status.RecentAdded[0] {
		t.Fatal("failed scan published a preview")
	}
}

func TestScannerScheduleCanEnableWhileDisabled(t *testing.T) {
	a, _, dir, _ := fileLibraryApp(t)
	a.scanInterval.Store(0)
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() { defer close(stopped); a.runLibraryScanner(done) }()
	defer func() {
		close(done)
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Error("scanner did not stop")
		}
	}()
	os.WriteFile(filepath.Join(dir, "google", "google-1.svg"), testSVG, 0644)
	a.scanInterval.Store(int64(10 * time.Millisecond))
	a.scanChanged <- struct{}{}
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("enabled scanner did not discover added SVG")
		case <-ticker.C:
			if a.store.library.scanStatus().RecentAddedTotal == 1 {
				return
			}
		}
	}
}
