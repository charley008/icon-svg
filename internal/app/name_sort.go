package app

import (
	"sort"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

type nameKey struct {
	group int
	value string
}

func nameSortKey(name string) nameKey {
	key := nameKey{group: 3}
	var normalized strings.Builder
	first := true
	args := pinyin.NewArgs()
	for _, r := range strings.TrimSpace(name) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		if first {
			switch {
			case unicode.Is(unicode.Latin, r):
				key.group = 0
			case unicode.IsDigit(r):
				key.group = 1
			case unicode.Is(unicode.Han, r):
				key.group = 2
			}
			first = false
		}
		if unicode.Is(unicode.Han, r) {
			if syllables := pinyin.LazyPinyin(string(r), args); len(syllables) > 0 {
				normalized.WriteString(syllables[0])
				continue
			}
		}
		normalized.WriteRune(unicode.ToLower(r))
	}
	key.value = normalized.String()
	return key
}

// Calculate pinyin once per item, then apply the same ordering to every list.
func sortByName[T any](items []T, name func(T) string) {
	entries := make([]struct {
		item T
		key  nameKey
	}, len(items))
	for n, item := range items {
		entries[n].item = item
		entries[n].key = nameSortKey(name(item))
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].key.group != entries[j].key.group {
			return entries[i].key.group < entries[j].key.group
		}
		return entries[i].key.value < entries[j].key.value
	})
	for n, entry := range entries {
		items[n] = entry.item
	}
}
