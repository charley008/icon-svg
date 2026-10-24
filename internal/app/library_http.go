package app

import (
	"archive/zip"
	"database/sql"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (a *App) scanLibrary(w http.ResponseWriter, r *http.Request) {
	mode := r.FormValue("mode")
	if mode != "" && mode != "changes" && mode != "full" {
		http.Error(w, "扫描模式无效", 400)
		return
	}
	if e := a.store.scanLibrary(mode == "full"); e != nil {
		if e == errLibraryScanRunning {
			http.Redirect(w, r, "/admin/settings?view=data", 303)
			return
		}
		http.Redirect(w, r, "/admin/settings?view=data", 303)
		return
	}
	if e := a.store.transaction(func(tx *sql.Tx) error { return audit(tx, "library_scanned", "library") }); e != nil {
		renderError(w, e)
		return
	}
	http.Redirect(w, r, "/admin/settings?view=data&scanned=1", 303)
}

func addLibraryFiles(z *zip.Writer, files map[string][]byte) error {
	keys := []string{}
	for name := range files {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		if !libraryTarget(name) {
			return fs.ErrInvalid
		}
		if e := zipEntry(z, "library/"+name, files[name]); e != nil {
			return e
		}
	}
	return nil
}

func (a *App) backupLibrary(w http.ResponseWriter, r *http.Request) {
	a.archive(w, r, "icon-svg-backup.zip", func(z *zip.Writer) error {
		s := a.store
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		s.library.mu.Lock()
		defer s.library.mu.Unlock()
		files, e := s.library.load()
		if e != nil {
			return e
		}
		if e = addLibraryFiles(z, files); e != nil {
			return e
		}
		if e = a.libraryLicenses(z); e != nil {
			return e
		}
		file, e := os.CreateTemp(filepath.Join(a.data, "cache"), "runtime-*.sqlite3")
		if e != nil {
			return e
		}
		name := file.Name()
		file.Close()
		os.Remove(name)
		defer os.Remove(name)
		if _, e = s.db.Exec(`VACUUM runtime INTO ?`, name); e != nil {
			return e
		}
		raw, e := os.ReadFile(name)
		if e != nil {
			return e
		}
		return zipEntry(z, "data/runtime.sqlite3", raw)
	})
}

func (a *App) libraryLicenses(z *zip.Writer) error {
	entries, e := fs.ReadDir(a.files.Library, "licenses")
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.ContainsAny(entry.Name(), "/\\:") {
			continue
		}
		raw, e := fs.ReadFile(a.files.Library, "licenses/"+entry.Name())
		if e != nil {
			return e
		}
		if e = zipEntry(z, "library/licenses/"+entry.Name(), raw); e != nil {
			return e
		}
	}
	return nil
}
