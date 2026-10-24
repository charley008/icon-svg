package app

import "strings"

type AdminSearchResult struct {
	Brand Brand
	Group Group
	Icon  *Icon
}

func adminMetaText(m GroupMeta) string {
	return strings.Join([]string{m.Key, m.Title, m.URL, m.Description, m.Category,
		strings.Join(m.Domains, " "), strings.Join(m.Aliases, " "), strings.Join(m.Tags, " ")}, " ")
}

func adminTextMatches(text, query string) bool {
	text = strings.ToLower(text)
	for _, term := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(text, term) {
			return false
		}
	}
	return true
}

// Match product and version metadata without loading SVG BLOBs or issuing
// a separate query for every brand. Return one result per managed icon group.
func (s *Store) searchAdminGroups(brands []Brand, groups []Group, query, state string) ([]AdminSearchResult, error) {
	byBrand := map[int64]Brand{}
	for _, b := range brands {
		if (state == "trash") == b.Deleted {
			byBrand[b.ID] = b
		}
	}
	texts := map[int64]string{}
	matches := map[int64]bool{}
	versions := map[int64]*Icon{}
	for _, g := range groups {
		b, ok := byBrand[g.BrandID]
		if !ok || (g.Deleted && state != "trash") {
			continue
		}
		text := b.Slug + " " + adminMetaText(b.Meta.GroupMeta) + " " + adminMetaText(g.Meta)
		texts[g.ID] = text
		matches[g.ID] = adminTextMatches(text, query)
	}
	rows, err := s.db.Query(`SELECT i.id,i.group_id,i.metadata,i.deleted,i.downloads,b.slug,g.product_key FROM icons i JOIN groups g ON g.id=i.group_id JOIN brands b ON b.id=i.brand_id WHERE i.deleted=0 ORDER BY i.is_default DESC,i.version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		i, err := scanIcon(rows)
		if err != nil {
			return nil, err
		}
		text, eligible := texts[i.GroupID]
		if !eligible || matches[i.GroupID] {
			continue
		}
		original, _ := i.Meta.Provenance["original_file"].(string)
		if adminTextMatches(text+" "+i.Meta.File+" "+i.Meta.Label+" "+original, query) {
			matches[i.GroupID] = true
			icon := i
			versions[i.GroupID] = &icon
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	results := []AdminSearchResult{}
	for _, g := range groups {
		if !matches[g.ID] {
			continue
		}
		icon := g.Icon
		if version := versions[g.ID]; version != nil {
			icon = version
		}
		results = append(results, AdminSearchResult{Brand: byBrand[g.BrandID], Group: g, Icon: icon})
	}
	return results, nil
}
