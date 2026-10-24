package app

import (
	"net/url"
	"reflect"
	"testing"
)

func TestPaginationWindowAndFilters(t *testing.T) {
	for _, tt := range []struct {
		page, pages int
		want        []int
	}{
		{1, 13, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 13}},
		{7, 20, []int{1, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 20}},
		{13, 13, []int{1, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}},
		{2, 3, []int{1, 2, 3}},
		{1, 1, []int{1}},
	} {
		query := url.Values{"q": {"腾讯"}, "category": {"自用工具与设备"}, "brand": {"tencent"}, "sort": {"recent"}, "view": {"brands"}, "page": {"99"}}
		original := query.Encode()
		var got []int
		active := 0
		for _, link := range paginationLinks(tt.page, tt.pages, query) {
			if link.Gap {
				if link.URL != "" || link.Active {
					t.Fatal("gap must not be clickable or active")
				}
				continue
			}
			got = append(got, link.Number)
			if link.Active {
				active++
				if link.Number != tt.page {
					t.Fatal("incorrect active page")
				}
			}
			parsed, err := url.Parse(link.URL)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"q", "category", "brand", "sort", "view"} {
				if parsed.Query().Get(key) != query.Get(key) {
					t.Fatalf("lost filter %s", key)
				}
			}
		}
		if !reflect.DeepEqual(got, tt.want) || active != 1 || query.Encode() != original {
			t.Fatalf("page %d/%d: got %v, want %v; active=%d", tt.page, tt.pages, got, tt.want, active)
		}
	}
}

func TestAdminPaginationUsesFullWindowAndKeepsFilters(t *testing.T) {
	query := url.Values{"q": {"淘宝"}, "state": {"trash"}}
	var pages []int
	for _, link := range paginationLinksAt("/admin", 1, 16, query) {
		if link.Gap {
			continue
		}
		pages = append(pages, link.Number)
		parsed, err := url.Parse(link.URL)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Path != "/admin" || parsed.Query().Get("q") != "淘宝" || parsed.Query().Get("state") != "trash" {
			t.Fatalf("wrong admin pagination URL: %s", link.URL)
		}
	}
	want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16}
	if !reflect.DeepEqual(pages, want) {
		t.Fatalf("admin pages = %v, want %v", pages, want)
	}
}
