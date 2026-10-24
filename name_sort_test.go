package main

import (
	"reflect"
	"testing"
)

func TestNameSort(t *testing.T) {
	names := []string{"同花顺", "腾讯", "微信", "百度", "阿里巴巴", "字节跳动", "Google", "apple", "Adobe", "360", "123云盘", "OpenAI"}
	sortByName(names, func(s string) string { return s })
	want := []string{"Adobe", "apple", "Google", "OpenAI", "123云盘", "360", "阿里巴巴", "百度", "腾讯", "同花顺", "微信", "字节跳动"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("name order = %v, want %v", names, want)
	}
	if key := nameSortKey("同花顺"); key.group != 2 || key.value != "tonghuashun" {
		t.Fatalf("同花顺 key = %+v", key)
	}
	if nameSortKey("Apple").value != nameSortKey(" apple ").value {
		t.Fatal("English ordering must ignore case and surrounding whitespace")
	}
}

func TestStoreSortsDisplayNamesInsteadOfSlugs(t *testing.T) {
	a, h := testApp(t)
	if _, err := a.store.db.Exec(`UPDATE groups SET metadata=json_set(metadata,'$.title','同花顺') WHERE product_key=''`); err != nil {
		t.Fatal(err)
	}
	groups, err := a.store.groups(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Meta.Title != "Gmail" || groups[1].Meta.Title != "同花顺" {
		t.Fatal("groups must use display names, not brand/product slugs")
	}
	assertCode(t, send(t, h, "GET", "/?sort=name", nil, nil), 200)
}
