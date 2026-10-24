package app

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestAdminSearchFindsProductsAndVersions(t *testing.T) {
	a, h := testApp(t)
	cookie, _ := authenticate(t, a, h)
	product, err := groupByID(a.store.db, 2)
	if err != nil {
		t.Fatal(err)
	}
	product.Meta.Title = "淘宝"
	product.Meta.Aliases = []string{"taobao"}
	product.Meta.Domains = []string{"taobao.com"}
	product.Meta.Tags = []string{"购物"}
	product.Meta.URL = "https://www.taobao.com"
	if _, err = a.store.db.Exec(`UPDATE groups SET metadata=?,published=0 WHERE id=2`, jsonText(product.Meta)); err != nil {
		t.Fatal(err)
	}
	version := IconMeta{File: "google-gmail-1.svg", Version: 1, Label: "彩色新版", SHA: digest(testSVG), Bytes: len(testSVG), Provenance: map[string]any{"original_file": "source-mail.svg"}}
	if _, err = a.store.db.Exec(`INSERT INTO icons(brand_id,group_id,filename,version,metadata,hash,svg,is_default) VALUES(1,2,?,1,?,?,?,0)`, version.File, jsonText(version), version.SHA, testSVG); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"淘宝", " TAOBAO ", "taobao.com", "购物", "Google 淘宝", "google-gmail-1.svg", "彩色新版", "source-mail.svg"} {
		w := send(t, h, "GET", "/admin?q="+url.QueryEscape(query), nil, cookie)
		assertCode(t, w, 200)
		body := w.Body.String()
		for _, want := range []string{`href="/admin/brands/1#group-2"`, "所属合集：Google", "未上架"} {
			if !strings.Contains(body, want) {
				t.Fatalf("query %q missing %q", query, want)
			}
		}
		if strings.Count(body, `class="brand-card admin-result"`) != 1 {
			t.Fatalf("query %q should produce one product result", query)
		}
	}
	versionSearch := send(t, h, "GET", "/admin?q=source-mail.svg", nil, cookie)
	if !strings.Contains(versionSearch.Body.String(), `src="/admin/preview/3?`) {
		t.Fatal("a filename search must preview the matching non-default version")
	}
	brandSearch := send(t, h, "GET", "/admin?q=google", nil, cookie)
	if strings.Count(brandSearch.Body.String(), `class="brand-card admin-result"`) != 2 {
		t.Fatal("brand search must include its main logo and product")
	}
	editor := send(t, h, "GET", "/admin/brands/1", nil, cookie)
	if !strings.Contains(editor.Body.String(), `id="group-2"`) {
		t.Fatal("search management links need a product section anchor")
	}
	assertCode(t, send(t, h, "GET", "/admin?q=淘宝", nil, nil), 303)
}

func TestAdminSearchHonorsTrashAndExcludesRemovedVersions(t *testing.T) {
	a, h := testApp(t)
	cookie, _ := authenticate(t, a, h)
	if _, err := a.store.db.Exec(`UPDATE icons SET deleted=1 WHERE group_id=2`); err != nil {
		t.Fatal(err)
	}
	w := send(t, h, "GET", "/admin?q=google-gmail.svg", nil, cookie)
	if strings.Contains(w.Body.String(), `class="brand-card admin-result"`) {
		t.Fatal("removed version filenames must not match active search")
	}
	if _, err := a.store.db.Exec(`UPDATE groups SET deleted=1 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	w = send(t, h, "GET", "/admin?q=gmail", nil, cookie)
	if strings.Contains(w.Body.String(), `class="brand-card admin-result"`) {
		t.Fatal("removed products must not match brand-library search")
	}
	if _, err := a.store.db.Exec(`UPDATE brands SET deleted=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	w = send(t, h, "GET", "/admin?q=google", nil, cookie)
	if strings.Contains(w.Body.String(), `class="brand-card admin-result"`) {
		t.Fatal("trashed brands must not match brand-library search")
	}
	w = send(t, h, "GET", "/admin?state=trash&q=gmail", nil, cookie)
	if !strings.Contains(w.Body.String(), `href="/admin/brands/1#group-2"`) {
		t.Fatal("brand trash search must still locate its removed products")
	}
}

func TestAdminSearchPaginatesProducts(t *testing.T) {
	a, h := testApp(t)
	cookie, _ := authenticate(t, a, h)
	for n := 1; n <= 31; n++ {
		key := fmt.Sprintf("batch-%02d", n)
		meta := GroupMeta{Key: key, Title: fmt.Sprintf("Batch %02d", n)}
		if _, err := a.store.db.Exec(`INSERT INTO groups(brand_id,product_key,metadata,next_version,updated_at) VALUES(1,?,?,0,?)`, key, jsonText(meta), stamp()); err != nil {
			t.Fatal(err)
		}
	}
	w := send(t, h, "GET", "/admin?q=Batch&page=2", nil, cookie)
	assertCode(t, w, 200)
	body := w.Body.String()
	if strings.Count(body, `class="brand-card admin-result"`) != 1 || !strings.Contains(body, "Batch 31") || !strings.Contains(body, "31 个匹配条目") || !strings.Contains(body, "q=Batch") {
		t.Fatal("search pagination must count products and preserve the query")
	}
	w = send(t, h, "GET", "/admin?q=missing", nil, cookie)
	if !strings.Contains(w.Body.String(), "没有匹配的合集、产品或图标") {
		t.Fatal("missing search needs a clear empty state")
	}
	w = send(t, h, "GET", "/admin?q=+++", nil, cookie)
	if strings.Contains(w.Body.String(), `class="brand-card admin-result"`) || !strings.Contains(w.Body.String(), "1 个合集") {
		t.Fatal("blank search must keep the normal brand library")
	}
}
