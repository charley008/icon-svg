package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Assign stable IDs to new entries without letting directory ordering change
// existing detail URLs. Newly discovered SVGs use the established naming rule.
func normalizeLibraryFiles(files map[string][]byte) (map[string][]byte, error) {
	out := map[string][]byte{}
	metas := map[string]*BrandMeta{}
	keys := []string{}
	var bid, gid, iid int64
	for name, raw := range files {
		out[name] = raw
		if !strings.HasSuffix(name, "/meta.json") {
			continue
		}
		var m BrandMeta
		if e := json.Unmarshal(raw, &m); e != nil {
			return nil, fmt.Errorf("%s: %w", name, e)
		}
		if m.ID < 0 || m.ID > 1<<52 {
			return nil, errors.New("合集 ID 无效")
		}
		bid = max(bid, m.ID)
		groups := append([]GroupMeta{m.GroupMeta}, m.Products...)
		for _, g := range groups {
			if g.ID < 0 || g.ID > 1<<52 {
				return nil, errors.New("产品 ID 无效")
			}
			gid = max(gid, g.ID)
			for _, alias := range g.RedirectFrom {
				if alias <= 0 || alias > 1<<52 {
					return nil, errors.New("旧详情地址 ID 无效")
				}
				gid = max(gid, alias)
			}
			for _, i := range g.Icons {
				if i.ID < 0 || i.ID > 1<<52 {
					return nil, errors.New("图标 ID 无效")
				}
				iid = max(iid, i.ID)
			}
		}
		metas[name] = &m
		keys = append(keys, name)
	}
	sort.Slice(keys, func(i, j int) bool { return path.Dir(keys[i]) < path.Dir(keys[j]) })
	for _, name := range keys {
		m := metas[name]
		slug := path.Dir(name)
		if m.ID == 0 {
			bid++
			m.ID = bid
		}
		groups := []*GroupMeta{&m.GroupMeta}
		for n := range m.Products {
			groups = append(groups, &m.Products[n])
		}
		used := map[string]bool{}
		for _, g := range groups {
			kept := []IconMeta{}
			for _, i := range g.Icons {
				if i.Version < 0 || i.Version >= 1000000 || i.File != filename(slug, g.Key, i.Version) || !validSVGName(i.File) {
					return nil, fmt.Errorf("无效素材引用: %s/%s", slug, i.File)
				}
				g.Next = max(g.Next, i.Version+1)
				if _, exists := files[slug+"/"+i.File]; !exists {
					continue
				}
				kept = append(kept, i)
				used[i.File] = true
			}
			g.Icons = kept
		}
		svgNames := []string{}
		for file := range files {
			if path.Dir(file) == slug && strings.HasSuffix(file, ".svg") {
				svgNames = append(svgNames, path.Base(file))
			}
		}
		sort.Strings(svgNames)
		for _, file := range svgNames {
			if used[file] {
				continue
			}
			var target *GroupMeta
			version := -1
			prefixLen := -1
			for _, g := range groups {
				stem := strings.TrimSuffix(filename(slug, g.Key, 0), ".svg")
				base := strings.TrimSuffix(file, ".svg")
				n := -1
				if base == stem {
					n = 0
				} else if strings.HasPrefix(base, stem+"-") {
					value, e := strconv.Atoi(strings.TrimPrefix(base, stem+"-"))
					if e == nil && value > 0 && value < 1000000 {
						n = value
					}
				}
				if n >= 0 && len(stem) > prefixLen {
					target = g
					version = n
					prefixLen = len(stem)
				}
			}
			if target == nil {
				return nil, fmt.Errorf("SVG 未在 meta.json 中登记，且不符合合集/产品命名: %s/%s", slug, file)
			}
			target.Icons = append(target.Icons, IconMeta{File: file, Version: version, Label: strings.TrimSuffix(file, ".svg"), Source: "素材库扫描", Method: "library-scan"})
		}
		for _, g := range groups {
			if g.ID == 0 {
				gid++
				g.ID = gid
			}
			active, defaults := 0, 0
			first := -1
			for n := range g.Icons {
				i := &g.Icons[n]
				if i.ID == 0 {
					iid++
					i.ID = iid
				}
				if !i.Deleted {
					active++
					if first < 0 {
						first = n
					}
					if i.Default {
						defaults++
					}
				}
				g.Next = max(g.Next, i.Version+1)
			}
			if active > 0 && defaults == 0 {
				g.Icons[first].Default = true
			}
		}
		raw, e := json.MarshalIndent(m, "", "  ")
		if e != nil {
			return nil, e
		}
		out[name] = append(raw, '\n')
	}
	return out, nil
}
