package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestCategoryManagementAndSelection(t *testing.T) {
	a, h := testApp(t)
	c, token := authenticate(t, a, h)
	post := func(action, name, next string) int {
		return send(t, h, "POST", "/admin/categories/"+action, url.Values{"csrf": {token}, "name": {name}, "next": {next}}, c).Code
	}
	if send(t, h, "POST", "/admin/categories/create", url.Values{"name": {"bad"}}, c).Code != 403 {
		t.Fatal("category writes require CSRF")
	}
	if post("create", "测试分类", "") != 303 {
		t.Fatal("create category failed")
	}
	if post("create", "测试分类", "") == 303 {
		t.Fatal("duplicate category accepted")
	}
	form := url.Values{"csrf": {token}, "title": {"Google"}, "category": {"测试分类"}, "published": {"on"}}
	assertCode(t, send(t, h, "POST", "/admin/brands/1", form, c), 303)
	form.Set("title", "Gmail")
	assertCode(t, send(t, h, "POST", "/admin/groups/2", form, c), 303)
	if post("rename", "测试分类", "重命名") != 303 {
		t.Fatal("rename category failed")
	}
	b, _ := brandByID(a.store.db, 1)
	g, _ := groupByID(a.store.db, 2)
	if b.Meta.Category != "重命名" || g.Meta.Category != "重命名" {
		t.Fatal("rename must update both collection and product metadata")
	}
	page := send(t, h, "GET", "/admin/brands/1", nil, c)
	if strings.Contains(page.Body.String(), `<input name="category"`) || !strings.Contains(page.Body.String(), `<select name="category">`) || !strings.Contains(page.Body.String(), `value="重命名" selected`) {
		t.Fatal("category dropdown must show selected category")
	}
	if post("delete", "重命名", "其他") != 303 {
		t.Fatal("merge category failed")
	}
	b, _ = brandByID(a.store.db, 1)
	g, _ = groupByID(a.store.db, 2)
	if b.Meta.Category != "其他" || g.Meta.Category != "其他" {
		t.Fatal("delete must migrate content")
	}
	if post("delete", "其他", "其他") == 303 {
		t.Fatal("fallback category must remain")
	}
	form.Set("category", "不存在")
	if send(t, h, "POST", "/admin/groups/2", form, c).Code == 303 {
		t.Fatal("unknown category accepted")
	}
	var count int
	a.store.db.QueryRow(`SELECT COUNT(*) FROM icons`).Scan(&count)
	if count != 2 {
		t.Fatal("category management must preserve SVGs")
	}
}
