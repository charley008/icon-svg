package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Admission happens before database/view locks. There is no application queue:
// disconnected crawlers must not leave an unbounded number of waiting handlers.
func (a *App) admit(w http.ResponseWriter, r *http.Request) (func(), bool) {
	if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/licenses" {
		return func() {}, true
	}
	slots := a.publicSlots
	if strings.HasPrefix(r.URL.Path, "/assets/") {
		// Small, cacheable SVG reads must not compete with expensive pages.
		slots = a.assetSlots
	} else if strings.HasPrefix(r.URL.Path, "/admin") {
		slots = a.adminSlots
	}
	select {
	case <-r.Context().Done():
		return nil, false
	case slots <- struct{}{}:
		return func() { <-slots }, true
	default:
		unavailable(w)
		return nil, false
	}
}

func unavailable(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "1")
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "服务繁忙，请稍后重试", http.StatusServiceUnavailable)
}

func lockContext(ctx context.Context, mu *sync.Mutex) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if mu.TryLock() {
			return nil
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Existing query helpers can use a request context without duplicating their SQL.
type contextQueries struct {
	context.Context
	db interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
		QueryRowContext(context.Context, string, ...any) *sql.Row
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	}
}

func (q contextQueries) Query(query string, args ...any) (*sql.Rows, error) {
	return q.db.QueryContext(q.Context, query, args...)
}
func (q contextQueries) QueryRow(query string, args ...any) *sql.Row {
	return q.db.QueryRowContext(q.Context, query, args...)
}
func (q contextQueries) Exec(query string, args ...any) (sql.Result, error) {
	return q.db.ExecContext(q.Context, query, args...)
}

type publicCatalog struct {
	changes, dataVersion int64
	built                time.Time
	groups               []Group
	brands               []Brand
	cards                []BrandOverview
	search               map[int64]string
}

// Cache only metadata, never SVGs or arbitrary URL responses. SQLite's counters
// detect committed edits (including direct SQL) without modifying the schema.
// SetMaxOpenConns(1) keeps total_changes on the same connection; data_version
// also detects writes made through another connection. The age bound covers a
// recycled connection, and a single owner rebuilds the immutable snapshot.
func (a *App) publicCatalog(ctx context.Context) (*publicCatalog, error) {
	if err := lockContext(ctx, &a.catalogMu); err != nil {
		return nil, err
	}
	defer a.catalogMu.Unlock()
	var changes, version int64
	readVersion := func() error {
		return a.store.db.QueryRowContext(ctx, `SELECT total_changes(), data_version FROM pragma_data_version`).Scan(&changes, &version)
	}
	if err := readVersion(); err != nil {
		return nil, err
	}
	if c := a.catalog; c != nil && c.changes == changes && c.dataVersion == version && time.Since(c.built) < time.Minute {
		return c, nil
	}
	if err := lockContext(ctx, &a.store.writeMu); err != nil {
		return nil, err
	}
	defer a.store.writeMu.Unlock()
	if err := readVersion(); err != nil {
		return nil, err
	}
	groups, err := a.store.groupsContext(ctx, false)
	if err != nil {
		return nil, err
	}
	brands, err := a.store.brandsContext(ctx)
	if err != nil {
		return nil, err
	}
	c := &publicCatalog{changes: changes, dataVersion: version, built: time.Now(), groups: groups, brands: brands, cards: brandOverviews(brands, groups), search: make(map[int64]string, len(groups))}
	for _, g := range groups {
		c.search[g.ID] = strings.ToLower(g.Brand + " " + g.Key + " " + g.Meta.Title + " " + strings.Join(g.Meta.Tags, " ") + " " + strings.Join(g.Meta.Aliases, " ") + " " + strings.Join(g.Meta.Domains, " "))
	}
	a.catalog = c
	return c, nil
}

const brandNavigationLimit = 30

func brandNavigation(cards []BrandOverview, query, selected string) []BrandOverview {
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]BrandOverview, 0, brandNavigationLimit)
	var current *BrandOverview
	for n := range cards {
		card := &cards[n]
		if card.Brand.Slug == selected {
			current = card
		}
		text := card.Brand.Slug + " " + card.Brand.Meta.Title + " " + strings.Join(card.Brand.Meta.Aliases, " ") + " " + strings.Join(card.Brand.Meta.Domains, " ")
		if len(result) < brandNavigationLimit && strings.Contains(strings.ToLower(text), query) {
			result = append(result, *card)
		}
	}
	if current != nil && query == "" {
		for _, card := range result {
			if card.Brand.Slug == selected {
				return result
			}
		}
		if len(result) == brandNavigationLimit {
			result = result[:len(result)-1]
		}
		result = append(result, *current)
	}
	return result
}

func (a *App) searchBrands(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if len(query) > 300 {
		http.Error(w, "搜索内容过长", http.StatusBadRequest)
		return
	}
	c, err := a.publicCatalog(r.Context())
	if err != nil {
		renderError(w, err)
		return
	}
	type item struct {
		Slug  string `json:"slug"`
		Title string `json:"title"`
		Logo  string `json:"logo,omitempty"`
	}
	items := make([]item, 0, brandNavigationLimit)
	for _, card := range brandNavigation(c.cards, query, "") {
		entry := item{Slug: card.Brand.Slug, Title: card.Brand.Meta.Title}
		if card.Logo != nil {
			entry.Logo = "/assets/" + strconv.FormatInt(card.Logo.ID, 10) + "/" + card.Logo.Meta.SHA
		}
		items = append(items, entry)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(items)
}
