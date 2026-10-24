package main

import (
	"strings"
	"testing"
)

func TestBrandBrowseIncludesMainAndProducts(t *testing.T) {
	a, h := testApp(t)
	// Products in a different category must still be discoverable by brand.
	if _, err := a.store.db.Exec(`UPDATE groups SET metadata=json_set(metadata,'$.category','Mail') WHERE product_key='gmail'`); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/?view=brands", "/?brand=google"} {
		w := send(t, h, "GET", target, nil, nil)
		assertCode(t, w, 200)
		for _, want := range []string{"Google", "Gmail", "1 个产品与网站", "合集总览"} {
			if !strings.Contains(w.Body.String(), want) {
				t.Fatalf("%s missing %q", target, want)
			}
		}
	}
	w := send(t, h, "GET", "/?brand=google", nil, nil)
	if !strings.Contains(w.Body.String(), "查看 Google 主图标") || strings.Count(w.Body.String(), `class="icon-card"`) != 2 {
		t.Fatal("brand collection must include the main logo and its product")
	}
	assertCode(t, send(t, h, "GET", "/?brand=missing", nil, nil), 404)
	search := send(t, h, "GET", "/?view=brands&q=missing", nil, nil)
	assertCode(t, search, 200)
	if !strings.Contains(search.Body.String(), "没有找到这个合集") {
		t.Fatal("brand search must have an empty state")
	}
	if _, err := a.store.db.Exec(`UPDATE groups SET published=0 WHERE product_key='gmail'`); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/?view=brands", "/?brand=google"} {
		w := send(t, h, "GET", target, nil, nil)
		assertCode(t, w, 200)
		if strings.Contains(w.Body.String(), "Gmail") || (target == "/?brand=google" && !strings.Contains(w.Body.String(), "0 个产品与网站")) {
			t.Fatal("hidden products leaked into brand navigation or collection")
		}
	}
	if _, err := a.store.db.Exec(`UPDATE brands SET published=0 WHERE slug='google'`); err != nil {
		t.Fatal(err)
	}
	assertCode(t, send(t, h, "GET", "/?brand=google", nil, nil), 404)
	w = send(t, h, "GET", "/?view=brands", nil, nil)
	assertCode(t, w, 200)
	if strings.Contains(w.Body.String(), "Google") {
		t.Fatal("hidden brand leaked into directory")
	}
}
