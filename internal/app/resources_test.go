package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestExternalWebResources(t *testing.T) {
	a, _ := testApp(t)
	root := t.TempDir()
	web := filepath.Join(root, "web")
	if err := os.MkdirAll(filepath.Join(web, "static"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(web, "templates"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "templates", "home.html"), []byte("external template"), 0600); err != nil {
		t.Fatal(err)
	}
	css := filepath.Join(web, "static", "probe.css")
	if err := os.WriteFile(css, []byte("first external asset"), 0600); err != nil {
		t.Fatal(err)
	}
	files := AppResources{Web: os.DirFS(web), Library: libraryInput{os.DirFS(filepath.Join("..", "..", "library"))}, Root: fstest.MapFS{"THIRD_PARTY_NOTICES.txt": {Data: []byte("external notice")}}}
	external, err := newApp(a.store, a.data, "", false, files)
	if err != nil {
		t.Fatal(err)
	}
	h := external.routes()
	if got := send(t, h, "GET", "/", nil, nil).Body.String(); got != "external template" {
		t.Fatal(got)
	}
	if err = os.WriteFile(css, []byte("edited external asset"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := send(t, h, "GET", "/static/probe.css", nil, nil).Body.String(); !strings.Contains(got, "edited external asset") {
		t.Fatal(got)
	}
	if got := send(t, h, "GET", "/licenses", nil, nil).Body.String(); got != "external notice" {
		t.Fatal(got)
	}
	files.Web = fstest.MapFS{}
	if _, err = newApp(a.store, a.data, "", false, files); err == nil {
		t.Fatal("missing web must fail, with no embedded fallback")
	}
}
