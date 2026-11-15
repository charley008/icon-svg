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

const brandNavigationLimit = 30

func (a *App) searchBrands(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(query) > 300 {
		http.Error(w, "搜索内容过长", 400)
		return
	}
	cards, err := a.store.publicBrands(r.Context(), query, "", brandNavigationLimit, 0, false)
	if err != nil {
		renderError(w, err)
		return
	}
	type item struct {
		Slug  string `json:"slug"`
		Title string `json:"title"`
		Logo  string `json:"logo,omitempty"`
	}
	items := make([]item, 0, len(cards))
	for _, card := range cards {
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
