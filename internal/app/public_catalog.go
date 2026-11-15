package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

const visibleGroups = `b.published=1 AND b.deleted=0 AND g.published=1 AND g.deleted=0 AND g.icon_count>0`
const visibleBrands = `b.published=1 AND b.deleted=0 AND EXISTS(SELECT 1 FROM groups v WHERE v.brand_id=b.id AND v.published=1 AND v.deleted=0 AND v.icon_count>0)`

// Only card fields are read. Full version metadata belongs to the detail/admin
// pages, not every catalogue request.
const publicGroupsSelect = `SELECT g.id,g.brand_id,b.slug,g.product_key,json_extract(g.metadata,'$.title'),
 coalesce(json_extract(g.metadata,'$.domains'),'[]'),coalesce(json_extract(g.metadata,'$.redirect_from'),'[]'),
 g.updated_at,g.icon_count,g.download_count,coalesce(i.id,0),coalesce(i.hash,'')
 FROM groups g JOIN brands b ON b.id=g.brand_id LEFT JOIN icons i ON i.id=g.preview_id `

func (s *Store) publicGroups(ctx context.Context, where, order string, args ...any) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, publicGroupsSelect+` WHERE `+visibleGroups+where+` ORDER BY `+order, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Group{}
	for rows.Next() {
		var g Group
		var domains, redirects, hash string
		var iconID int64
		if err = rows.Scan(&g.ID, &g.BrandID, &g.Brand, &g.Key, &g.Meta.Title, &domains, &redirects, &g.Updated, &g.Count, &g.Downloads, &iconID, &hash); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(domains), &g.Meta.Domains); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(redirects), &g.Meta.RedirectFrom); err != nil {
			return nil, err
		}
		g.Published = true
		g.Icon = &Icon{ID: iconID, Meta: IconMeta{SHA: hash}}
		result = append(result, g)
	}
	return result, rows.Err()
}

// Directory/sidebar reads are bounded before any metadata is decoded in Go.
// Only directory cards request four product previews; sidebars need just logos.
func (s *Store) publicBrands(ctx context.Context, query, slug string, limit, offset int, previews bool) ([]BrandOverview, error) {
	where, args := "", []any{}
	if query != "" {
		where += ` AND instr(lower(b.slug||' '||b.search_text),?)>0`
		args = append(args, strings.ToLower(query))
	}
	if slug != "" {
		where += ` AND b.slug=?`
		args = append(args, slug)
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT b.id,b.slug,
 json_object('title',json_extract(b.metadata,'$.title'),'aliases',json_extract(b.metadata,'$.aliases'),
 'domains',json_extract(b.metadata,'$.domains'),'url',json_extract(b.metadata,'$.url'),
 'description',CASE WHEN b.slug=? THEN json_extract(b.metadata,'$.description') ELSE '' END),
 (SELECT coalesce(sum(v.icon_count),0) FROM groups v WHERE v.brand_id=b.id AND v.published=1 AND v.deleted=0),
 (SELECT count(*) FROM groups v WHERE v.brand_id=b.id AND v.product_key<>'' AND v.published=1 AND v.deleted=0 AND v.icon_count>0),
 coalesce(main.id,0),coalesce(logo.id,0),coalesce(logo.hash,'')
 FROM brands b LEFT JOIN groups main ON main.brand_id=b.id AND main.product_key='' AND main.published=1 AND main.deleted=0 AND main.icon_count>0
 LEFT JOIN icons logo ON logo.id=main.preview_id WHERE `+visibleBrands+where+`
 ORDER BY b.name_group,b.name_key,b.slug LIMIT ? OFFSET ?`, append([]any{slug}, args...)...)
	if err != nil {
		return nil, err
	}
	result := []BrandOverview{}
	for rows.Next() {
		var c BrandOverview
		var raw, hash string
		var mainID, logoID int64
		if err = rows.Scan(&c.Brand.ID, &c.Brand.Slug, &raw, &c.Icons, &c.ProductCount, &mainID, &logoID, &hash); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &c.Brand.Meta); err != nil {
			rows.Close()
			return nil, err
		}
		c.Brand.Published = true
		if mainID != 0 {
			c.Logo = &Icon{ID: logoID, Meta: IconMeta{SHA: hash}}
			c.Main = &Group{ID: mainID, Icon: c.Logo}
		}
		result = append(result, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if previews {
		for n := range result {
			result[n].Previews, err = s.publicGroups(ctx, ` AND g.brand_id=? AND g.product_key<>''`, `g.name_group,g.name_key,g.id LIMIT 4`, result[n].Brand.ID)
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (s *Store) publicHome(ctx context.Context, q url.Values) (HomePage, error) {
	p := HomePage{Q: strings.TrimSpace(q.Get("q")), Category: q.Get("category"), Brand: q.Get("brand"), Sort: q.Get("sort"), Page: 1}
	if len(p.Q) > 300 {
		p.Q = p.Q[:300]
	}
	p.Page, _ = strconv.Atoi(q.Get("page"))
	p.BrandDirectory = q.Get("view") == "brands" && p.Brand == ""
	favorites := q.Has("favorites")
	p.Hero = p.Q == "" && p.Category == "" && p.Brand == "" && !favorites && !p.BrandDirectory
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM brands b WHERE `+visibleBrands).Scan(&p.DirectoryCount); err != nil {
		return p, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM brands WHERE published=1 AND deleted=0),coalesce(sum(g.icon_count),0) FROM groups g JOIN brands b ON b.id=g.brand_id WHERE `+visibleGroups).Scan(&p.BrandCount, &p.Icons); err != nil {
		return p, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT g.category,count(*) FROM groups g JOIN brands b ON b.id=g.brand_id WHERE `+visibleGroups+` GROUP BY g.category`)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var c Category
		if err = rows.Scan(&c.Name, &c.Count); err != nil {
			rows.Close()
			return p, err
		}
		p.Categories = append(p.Categories, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	sortByName(p.Categories, func(c Category) string { return c.Name })
	if p.Brand != "" {
		cards, err := s.publicBrands(ctx, "", p.Brand, 1, 0, false)
		if err != nil {
			return p, err
		}
		if len(cards) == 0 {
			return p, sql.ErrNoRows
		}
		p.SelectedBrand = &cards[0]
	}
	if p.BrandDirectory || p.Brand != "" {
		p.NavBrandCards, err = s.publicBrands(ctx, "", "", brandNavigationLimit, 0, false)
		if err != nil {
			return p, err
		}
		if p.SelectedBrand != nil {
			found := false
			for _, c := range p.NavBrandCards {
				found = found || c.Brand.Slug == p.Brand
			}
			if !found {
				if len(p.NavBrandCards) == brandNavigationLimit {
					p.NavBrandCards = p.NavBrandCards[:len(p.NavBrandCards)-1]
				}
				p.NavBrandCards = append(p.NavBrandCards, *p.SelectedBrand)
			}
		}
	}
	where, args := "", []any{}
	if p.Q != "" {
		where += ` AND instr(lower(b.slug||' '||g.product_key||' '||g.search_text),?)>0`
		args = append(args, strings.ToLower(p.Q))
	}
	if p.Category != "" {
		where += ` AND g.category=?`
		args = append(args, p.Category)
	}
	if p.Brand != "" {
		where += ` AND b.slug=?`
		args = append(args, p.Brand)
	}
	if favorites {
		ids := []int64{}
		for _, value := range strings.Split(q.Get("favorites"), ",") {
			if id, err := strconv.ParseInt(value, 10, 64); err == nil && id > 0 {
				ids = append(ids, id)
			}
			if len(ids) >= 2000 {
				break
			}
		}
		raw, _ := json.Marshal(ids)
		where += ` AND (g.id IN(SELECT value FROM json_each(?)) OR EXISTS(SELECT 1 FROM json_each(g.metadata,'$.redirect_from') old JOIN json_each(?) saved ON old.value=saved.value))`
		args = append(args, string(raw), string(raw))
	}
	if p.BrandDirectory {
		brandWhere, brandArgs := "", []any{}
		if p.Q != "" {
			brandWhere = ` AND instr(lower(b.slug||' '||b.search_text),?)>0`
			brandArgs = append(brandArgs, strings.ToLower(p.Q))
		}
		err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM brands b WHERE `+visibleBrands+brandWhere, brandArgs...).Scan(&p.Total)
	} else {
		err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM groups g JOIN brands b ON b.id=g.brand_id WHERE `+visibleGroups+where, args...).Scan(&p.Total)
	}
	if err != nil {
		return p, err
	}
	p.Pages = max(1, (p.Total+47)/48)
	p.Page = max(1, min(p.Page, p.Pages))
	if p.BrandDirectory {
		p.DirectoryCards, err = s.publicBrands(ctx, p.Q, "", 48, (p.Page-1)*48, true)
	} else {
		order := `g.name_group,g.name_key,b.slug,g.product_key,g.id`
		if p.Sort == "popular" {
			order = `g.download_count DESC,` + order
		} else if p.Sort == "recent" {
			order = `g.updated_at DESC,` + order
		}
		p.Groups, err = s.publicGroups(ctx, where, order+` LIMIT ? OFFSET ?`, append(args, 48, (p.Page-1)*48)...)
	}
	if err != nil {
		return p, err
	}
	if p.Hero {
		keys := []string{"google/", "apple/", "github/", "wechat/", "bytedance/douyin", "openai/chatgpt", "bilibili/", "alipay/", "google/youtube", "microsoft/", "figma/", "qq/", "spotify/", "alibaba/taobao"}
		marks, values := []string{}, []any{}
		for _, key := range keys {
			marks, values = append(marks, "?"), append(values, key)
		}
		groups, err := s.publicGroups(ctx, ` AND (b.slug||'/'||g.product_key) IN (`+strings.Join(marks, ",")+`)`, `g.id`, values...)
		if err != nil {
			return p, err
		}
		for _, key := range keys {
			for _, g := range groups {
				if g.Brand+"/"+g.Key == key {
					p.HeroIcons = append(p.HeroIcons, g)
				}
			}
		}
	}
	p.Links = paginationLinks(p.Page, p.Pages, q)
	return p, nil
}
